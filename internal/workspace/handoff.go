package workspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

// CompareManifests computes the complete delta against a selected retained basis.
// Final application compares the first author source to the selected full manifest.
func CompareManifests(before, after Manifest) []Change { return delta(before, after) }

// HandoffRequest names registered immutable evidence, never a worker-supplied path.
type HandoffRequest struct {
	AttemptID                   string
	Source                      *Snapshot
	CandidateManifestArtifactID string
	IndexArtifactID             string
}

// RetainIndex snapshots the actual managed candidate's index after authoring.
func (m *Manager) RetainIndex(ctx context.Context, attemptID string, source *Snapshot) (evidence.Metadata, error) {
	var zero evidence.Metadata
	if source == nil || !validManagedID(attemptID) || !validManagedID(source.ID) || source.WorkspacePath != filepath.Join(m.lock.Root(), "workspaces", source.ID) {
		return zero, ErrBlocked
	}
	if err := m.lock.CheckHeld(); err != nil {
		return zero, err
	}
	r, err := m.root.OpenRoot("workspaces/" + source.ID)
	if err != nil {
		return zero, err
	}
	defer r.Close()
	control, err := r.ReadFile(".git")
	if err != nil || hashBytes(control) != source.GitControlSHA256 {
		return zero, ErrBlocked
	}
	indexPath, err := m.value(ctx, source.WorkspacePath, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return zero, err
	}
	before, err := readIndex(indexPath)
	if err != nil {
		return zero, err
	}
	if _, err = m.index(ctx, source.WorkspacePath); err != nil {
		return zero, err
	}
	after, err := readIndex(indexPath)
	if err != nil || !bytes.Equal(before, after) {
		return zero, fmt.Errorf("%w Candidate index changed during retention.", ErrBlocked)
	}
	return m.artifacts.Put(ctx, attemptID, "index_snapshot", before)
}

// Handoff returns an isolated new stage workspace. Its implementation must use
// the selected retained predecessor, rather than recapturing the user's project.
func (m *Manager) Handoff(ctx context.Context, req HandoffRequest) (*Snapshot, error) {
	if req.Source == nil || !validManagedID(req.AttemptID) || !validManagedID(req.Source.ID) || !validManagedID(req.CandidateManifestArtifactID) || !validManagedID(req.IndexArtifactID) {
		return nil, ErrBlocked
	}
	if err := m.lock.CheckHeld(); err != nil {
		return nil, err
	}
	source := req.Source
	if !filepath.IsAbs(source.RepositoryPath) || len(source.BaseCommit) != 40 || !filepath.IsAbs(source.ProjectPath) || !within(source.RepositoryPath, source.ProjectPath) || source.SelectedSubfolder != "." && !safePath(source.SelectedSubfolder) {
		return nil, ErrBlocked
	}
	if _, err := hex.DecodeString(source.BaseCommit); err != nil {
		return nil, ErrBlocked
	}
	var manifestBytes, indexBytes bytes.Buffer
	manifestMeta, err := m.artifacts.CopyTo(ctx, req.CandidateManifestArtifactID, &manifestBytes, 64<<20)
	if err != nil || manifestMeta.AttemptID != source.AttemptID || manifestMeta.Kind != "candidate_manifest" && manifestMeta.Kind != "source_manifest" {
		return nil, evidence.ErrCorrupt
	}
	indexMeta, err := m.artifacts.CopyTo(ctx, req.IndexArtifactID, &indexBytes, 64<<20)
	if err != nil {
		return nil, err
	}
	if indexMeta.AttemptID != source.AttemptID || indexMeta.Kind != "index_snapshot" {
		return nil, evidence.ErrCorrupt
	}
	var manifest Manifest
	if err = json.Unmarshal(manifestBytes.Bytes(), &manifest, json.RejectUnknownMembers(true)); err != nil {
		return nil, evidence.ErrCorrupt
	}
	if err = validateHandoffManifest(manifest); err != nil {
		return nil, err
	}
	rawEntries, err := handoffIndexEntries(indexBytes.Bytes())
	if err != nil || !indexEqual(rawEntries, manifest.Index) {
		return nil, fmt.Errorf("%w Retained candidate manifest/index differ.", ErrBlocked)
	}
	id, err := evidence.NewID()
	if err != nil {
		return nil, err
	}
	dir := "workspaces/" + id
	if err = m.privateDir(dir); err != nil {
		return nil, err
	}
	destination := filepath.Join(m.lock.Root(), filepath.FromSlash(dir))
	if _, err = m.git(ctx, source.RepositoryPath, nil, "worktree", "add", "--no-checkout", "--detach", destination, source.BaseCommit); err != nil {
		return nil, err
	}
	if _, err = m.git(ctx, destination, nil, "read-tree", "--empty"); err != nil {
		return nil, err
	}
	var indexInput bytes.Buffer
	for _, entry := range manifest.Index {
		indexInput.WriteString(entry.Mode + " " + entry.Object + "\t" + entry.Path + "\x00")
	}
	if indexInput.Len() != 0 {
		if _, err = m.git(ctx, destination, indexInput.Bytes(), "update-index", "-z", "--index-info"); err != nil {
			return nil, err
		}
	}
	idx, err := m.index(ctx, destination)
	if err != nil || !indexEqual(idx, manifest.Index) {
		return nil, fmt.Errorf("%w Retained staging reconstruction failed.", ErrBlocked)
	}
	r, err := m.root.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if err = m.materialize(ctx, r, manifest); err != nil {
		return nil, err
	}
	actual, err := m.inventory(ctx, req.AttemptID, destination, r, idx, false)
	actual.Exclusions = manifest.Exclusions
	if err != nil || !sameManifest(actual, manifest) {
		return nil, fmt.Errorf("%w Reconstructed predecessor bytes differ.", ErrBlocked)
	}
	control, err := r.ReadFile(".git")
	if err != nil || len(control) > 4096 {
		return nil, ErrBlocked
	}
	ma, err := m.artifacts.Put(ctx, req.AttemptID, "source_manifest", manifestBytes.Bytes())
	if err != nil {
		return nil, err
	}
	next := &Snapshot{ID: id, AttemptID: req.AttemptID, ProjectPath: source.ProjectPath, RepositoryPath: source.RepositoryPath, SelectedSubfolder: source.SelectedSubfolder, WorkspacePath: destination, ExecutionPath: filepath.Join(destination, filepath.FromSlash(source.SelectedSubfolder)), BaseCommit: source.BaseCommit, Branch: source.Branch, ManifestArtifactID: ma.ID, Manifest: manifest, GitControlSHA256: hashBytes(control)}
	index, err := m.RetainIndex(ctx, req.AttemptID, next)
	if err != nil {
		return nil, err
	}
	next.IndexArtifactID = index.ID
	return next, nil
}

func validateHandoffManifest(manifest Manifest) error {
	if len(manifest.Entries) > MaxEntries || len(manifest.Index) > MaxEntries {
		return ErrBlocked
	}
	seen := map[string]bool{}
	var total int64
	for _, entry := range manifest.Entries {
		if !safePath(entry.Path) || excluded(entry.Path) != "" || seen[entry.Path] || entry.Size < 0 || entry.Size > MaxFileBytes || entry.Type != "file" && entry.Type != "symlink" {
			return ErrBlocked
		}
		seen[entry.Path] = true
		if entry.Type == "file" && (!validManagedID(entry.ArtifactID) || len(entry.SHA256) != 64) {
			return evidence.ErrCorrupt
		}
		if entry.Type == "symlink" {
			resolved := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(entry.Path), entry.Target)))
			if entry.Target == "" || filepath.IsAbs(entry.Target) || resolved != "." && !safePath(resolved) || excluded(resolved) != "" {
				return ErrBlocked
			}
		}
		total += entry.Size
		if total > MaxTotalBytes {
			return ErrBlocked
		}
	}
	previous := ""
	for _, entry := range manifest.Index {
		if !safePath(entry.Path) || excluded(entry.Path) != "" || entry.Path <= previous || len(entry.Object) != 40 || entry.Mode != "100644" && entry.Mode != "100755" && entry.Mode != "120000" {
			return ErrBlocked
		}
		if _, err := hex.DecodeString(entry.Object); err != nil {
			return ErrBlocked
		}
		previous = entry.Path
	}
	return nil
}

// The raw retained index must describe the same staging entries as the manifest.
// Validation handles bounds/extensions/checksum first, then this extracts its basis.
func handoffIndexEntries(raw []byte) ([]IndexEntry, error) {
	if err := validateRawIndex(raw); err != nil {
		return nil, err
	}
	version := binary.BigEndian.Uint32(raw[4:8])
	count := binary.BigEndian.Uint32(raw[8:12])
	offset := 12
	previous := ""
	out := make([]IndexEntry, 0, count)
	for range count {
		start := offset
		mode := binary.BigEndian.Uint32(raw[offset+24 : offset+28])
		object := hex.EncodeToString(raw[offset+40 : offset+60])
		flags := binary.BigEndian.Uint16(raw[offset+60 : offset+62])
		if flags&0x3000 != 0 {
			return nil, ErrBlocked
		}
		offset += 62
		if flags&0x4000 != 0 {
			offset += 2
		}
		removed := 0
		if version == 4 {
			c := raw[offset]
			offset++
			removed = int(c & 0x7f)
			for c&0x80 != 0 {
				removed = (removed + 1) << 7
				c = raw[offset]
				offset++
				removed += int(c & 0x7f)
			}
		}
		end := bytes.IndexByte(raw[offset:len(raw)-20], 0)
		name := string(raw[offset : offset+end])
		if version == 4 {
			name = previous[:len(previous)-removed] + name
		}
		offset += end + 1
		if version != 4 {
			for (offset-start)%8 != 0 {
				offset++
			}
		}
		if !safePath(name) {
			return nil, ErrBlocked
		}
		out = append(out, IndexEntry{Path: name, Mode: strconv.FormatUint(uint64(mode), 8), Object: object})
		previous = name
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	for i := 1; i < len(out); i++ {
		if out[i-1].Path == out[i].Path {
			return nil, errors.New("Duplicate retained index entry.")
		}
	}
	return out, nil
}
