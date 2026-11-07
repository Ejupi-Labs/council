package workspace

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func graphIndexRef(meta evidence.Metadata) domain.GraphArtifactRef {
	ref := domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, ByteLength: meta.ByteLength}
	if meta.SHA256 != nil {
		ref.SHA256 = *meta.SHA256
	}
	return ref
}

func (m *Manager) graphIndexLoad(ctx context.Context, ref domain.GraphArtifactRef, kinds ...string) ([]byte, error) {
	if !validManagedID(ref.ArtifactID) || !validManagedID(ref.ProducerAttemptID) || !domain.ValidSHA256(ref.SHA256) {
		return nil, evidence.ErrCorrupt
	}
	var body bytes.Buffer
	meta, err := m.artifacts.CopyTo(ctx, ref.ArtifactID, &body, MaxFileBytes)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, kind := range kinds {
		allowed = allowed || meta.Kind == kind
	}
	if !allowed || meta.AttemptID != ref.ProducerAttemptID || meta.ByteLength != ref.ByteLength || meta.SHA256 == nil || *meta.SHA256 != ref.SHA256 || !meta.Closed || meta.Completeness != "complete" {
		return nil, evidence.ErrCorrupt
	}
	return body.Bytes(), nil
}

func (m *Manager) graphIndexTarget(source *Snapshot) (*os.Root, error) {
	if source == nil || !validManagedID(source.ID) || !validManagedID(source.AttemptID) || source.WorkspacePath != filepath.Join(m.lock.Root(), "workspaces", source.ID) {
		return nil, ErrBlocked
	}
	if err := m.lock.CheckHeld(); err != nil {
		return nil, err
	}
	dir := "workspaces/" + source.ID
	info, err := m.root.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil, ErrBlocked
	}
	r, err := m.root.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	controlInfo, err := r.Lstat(".git")
	if err != nil || !controlInfo.Mode().IsRegular() || controlInfo.Size() > 4096 {
		r.Close()
		return nil, ErrBlocked
	}
	control, err := r.ReadFile(".git")
	if err != nil || source.GitControlSHA256 == "" || hashBytes(control) != source.GitControlSHA256 {
		r.Close()
		return nil, ErrBlocked
	}
	return r, nil
}

func (m *Manager) graphIndexBasis(ctx context.Context, manifestRef, indexRef domain.GraphArtifactRef) (Manifest, []byte, error) {
	var manifest Manifest
	body, err := m.graphIndexLoad(ctx, manifestRef, "source_manifest", "candidate_manifest", "graph_input_manifest")
	if err != nil {
		return manifest, nil, err
	}
	if err = json.Unmarshal(body, &manifest, json.RejectUnknownMembers(true)); err != nil {
		return manifest, nil, evidence.ErrCorrupt
	}
	if err = validateHandoffManifest(manifest); err != nil {
		return manifest, nil, err
	}
	raw, err := m.graphIndexLoad(ctx, indexRef, "index_snapshot", "candidate_index")
	if err != nil {
		return manifest, nil, err
	}
	entries, err := handoffIndexEntries(raw)
	if err != nil || !indexEqual(entries, manifest.Index) {
		return manifest, nil, fmt.Errorf("%w Retained graph working/index bindings disagree.", ErrBlocked)
	}
	return manifest, raw, nil
}

func graphIndexDigest(entries []IndexEntry) (string, error) {
	type item struct {
		Path     string `json:"path"`
		Mode     string `json:"mode"`
		ObjectID string `json:"objectId"`
	}
	items := make([]item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, item{entry.Path, entry.Mode, entry.Object})
	}
	body, err := json.Marshal(items)
	return hashBytes(body), err
}

func graphBlobObjectID(body []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(body))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func graphObjectID(value string) bool {
	if len(value) != 40 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func graphIndexObjects(entries []IndexEntry) []string {
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Object] = true
	}
	objects := make([]string, 0, len(seen))
	for object := range seen {
		objects = append(objects, object)
	}
	sort.Strings(objects)
	return objects
}

// One binary batch avoids one process per object and never buffers a whole tree.
// Each callback receives at most64MiB. Git hooks/filters/credentials are not used.
func (m *Manager) graphReadBlobs(ctx context.Context, dir string, objects []string, visit func(string, []byte) error) error {
	if len(objects) == 0 {
		return nil
	}
	var input bytes.Buffer
	for _, object := range objects {
		if !graphObjectID(object) {
			return evidence.ErrCorrupt
		}
		input.WriteString(object + "\n")
	}
	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	argv := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + m.hooks, "-c", "core.attributesFile=/dev/null", "cat-file", "--batch"}
	command := exec.CommandContext(batchCtx, "git", argv...)
	configureGitCommand(command)
	command.Dir = dir
	command.Stdin = bytes.NewReader(input.Bytes())
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var stderr cappedBuffer
	command.Stderr = &stderr
	out, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err = command.Start(); err != nil {
		out.Close()
		return fmt.Errorf("%w Git object batch could not start.", ErrBlocked)
	}
	reader := bufio.NewReaderSize(out, 4096)
	readErr := func() error {
		for _, object := range objects {
			if err := ctx.Err(); err != nil {
				return err
			}
			header, err := reader.ReadSlice('\n')
			if err != nil {
				return fmt.Errorf("%w Git object batch header is unavailable.", ErrBlocked)
			}
			parts := strings.Fields(string(header))
			if len(parts) != 3 || parts[0] != object || parts[1] != "blob" {
				return fmt.Errorf("%w A required index blob is unavailable or unsupported.", ErrBlocked)
			}
			size, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || size < 0 || size > MaxFileBytes || strconv.FormatInt(size, 10) != parts[2] {
				return fmt.Errorf("%w An index blob exceeds its supported byte bound.", ErrBlocked)
			}
			body := make([]byte, int(size))
			if _, err = io.ReadFull(reader, body); err != nil {
				return ErrBlocked
			}
			terminator, err := reader.ReadByte()
			if err != nil || terminator != '\n' || graphBlobObjectID(body) != object {
				return evidence.ErrCorrupt
			}
			if err = visit(object, body); err != nil {
				return err
			}
		}
		if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
			return ErrBlocked
		}
		return nil
	}()
	if readErr != nil {
		cancel()
	}
	out.Close()
	waitErr := command.Wait()
	if readErr != nil {
		return readErr
	}
	if waitErr != nil || stderr.overflow {
		return errors.Join(fmt.Errorf("%w Git object batch did not complete.", ErrBlocked), ctx.Err())
	}
	return nil
}
