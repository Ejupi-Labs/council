package workspace

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

type graphIndexCase struct {
	m       *Manager
	repo    string
	source  *Snapshot
	request GraphIndexCaptureRequest
	staged  []byte
	working []byte
	object  string
	index   []byte
}

func graphArtifactRef(t *testing.T, m *Manager, id string) domain.GraphArtifactRef {
	t.Helper()
	meta, err := m.artifacts.VerifyClosed(context.Background(), id)
	if err != nil || meta.SHA256 == nil {
		t.Fatal("closed fixture artifact", err)
	}
	return domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}
}

func graphIndexFixture(t *testing.T) graphIndexCase {
	t.Helper()
	m, repo, _ := fixture(t)
	staged := []byte("\xef\xbb\xbfcontrolled staged 東京\r\n\x00\xfe")
	working := []byte("different working🙂\r\n\x00\xff")
	put(t, repo, "file.bin", staged)
	runGit(t, repo, nil, "add", "file.bin")
	object := strings.TrimSpace(string(runGit(t, repo, nil, "rev-parse", ":file.bin")))
	if len(object) != 40 {
		t.Fatal("fixture did not create a SHA1 staged blob", object)
	}
	put(t, repo, "file.bin", working)
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
	if err != nil {
		t.Fatal("fixture root capture", err)
	}
	return graphIndexCase{m: m, repo: repo, source: source, staged: staged, working: working, object: object, index: index, request: GraphIndexCaptureRequest{
		ProducerAttemptID: attemptID, Source: source,
		WorkingManifest: graphArtifactRef(t, m, source.ManifestArtifactID), RawIndex: graphArtifactRef(t, m, source.IndexArtifactID),
	}}
}

func graphIndexManifest(t *testing.T, f graphIndexCase, ref domain.GraphArtifactRef) GraphIndexBlobManifest {
	t.Helper()
	var raw bytes.Buffer
	meta, err := f.m.artifacts.CopyTo(context.Background(), ref.ArtifactID, &raw, 64<<20)
	if err != nil || meta.Kind != "graph_index_blobs" || meta.AttemptID != ref.ProducerAttemptID || meta.SHA256 == nil || *meta.SHA256 != ref.SHA256 {
		t.Fatal("actual graph staged closure metadata", err)
	}
	var manifest GraphIndexBlobManifest
	if err = json.Unmarshal(raw.Bytes(), &manifest, json.RejectUnknownMembers(true)); err != nil || manifest.Contract != "006-index-blobs-v1" {
		t.Fatal("actual graph staged closure body", err)
	}
	return manifest
}

func graphIndexBlob(t *testing.T, manifest GraphIndexBlobManifest, object string) GraphIndexBlob {
	t.Helper()
	for _, entry := range manifest.Entries {
		if entry.ObjectID == object {
			return entry
		}
	}
	t.Fatal("staged closure omitted actual index object", object)
	return GraphIndexBlob{}
}

func assertGraphIndexOriginalPreserved(t *testing.T, f graphIndexCase) {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || !bytes.Equal(index, f.index) {
		t.Fatal("graph blob operation changed the user's raw index", err)
	}
	working, err := os.ReadFile(filepath.Join(f.repo, "file.bin"))
	if err != nil || !bytes.Equal(working, f.working) {
		t.Fatal("graph blob operation changed the user's working bytes", err)
	}
	if branch := strings.TrimSpace(string(runGit(t, f.repo, nil, "symbolic-ref", "--short", "HEAD"))); branch != "main" {
		t.Fatal("graph blob operation changed the user's branch", branch)
	}
}

func TestGraphIndexCaptureRetainsStagedBytesDistinctFromWorkingContent(t *testing.T) {
	f := graphIndexFixture(t)
	closure, err := f.m.CaptureGraphIndex(context.Background(), f.request)
	if err != nil {
		t.Fatal("real staged-blob capture unavailable", err)
	}
	manifest := graphIndexManifest(t, f, closure.StagedBlobs)
	blob := graphIndexBlob(t, manifest, f.object)
	var raw bytes.Buffer
	meta, err := f.m.artifacts.CopyTo(context.Background(), blob.ArtifactRef.ArtifactID, &raw, MaxFileBytes)
	contentHash := sha256.Sum256(f.staged)
	rawHash := sha1.Sum(f.staged)
	if err != nil || !bytes.Equal(raw.Bytes(), f.staged) || bytes.Equal(raw.Bytes(), f.working) || meta.Kind != "graph_index_blob" || meta.AttemptID != attemptID || blob.ContentSHA256 != hex.EncodeToString(contentHash[:]) || f.object == hex.EncodeToString(rawHash[:]) {
		t.Fatal("closure substituted working bytes/raw hash or lost exact staged bytes", err)
	}
	if closure.IndexEntriesSHA256 == "" || closure.CombinedBytes == "0" || closure.BlobCount != "2" {
		t.Fatalf("closure omitted bounded exact staged facts: %+v", closure)
	}
	assertGraphIndexOriginalPreserved(t, f)
}

func TestGraphIndexRestoresMissingStagedObjectFromClosedBytes(t *testing.T) {
	f := graphIndexFixture(t)
	closure, err := f.m.CaptureGraphIndex(context.Background(), f.request)
	if err != nil {
		t.Fatal("real staged-blob capture unavailable", err)
	}
	// This exact loose object is newly staged, uncommitted and confined to the
	// fixture's fresh Git repository. No real repository or broad cleanup is used.
	objectPath := filepath.Join(f.repo, ".git", "objects", f.object[:2], f.object[2:])
	info, err := os.Lstat(objectPath)
	if err != nil || !info.Mode().IsRegular() || !within(f.repo, objectPath) {
		t.Fatal("fixture did not identify its own ordinary loose object", err)
	}
	if err = os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	result, err := f.m.RestoreGraphIndexObjects(context.Background(), GraphIndexRestoreRequest{
		OperationAttemptID: strings.Repeat("b", 32), Target: f.source,
		WorkingManifest: f.request.WorkingManifest, RawIndex: f.request.RawIndex, StagedBlobs: closure.StagedBlobs,
	})
	if err != nil || !result.EffectsKnown || result.EffectsArtifact == nil || result.EffectCount != "2" {
		t.Fatalf("missing fixture Git object was not actually restored with known evidence: %+v / %v", result, err)
	}
	if actual := runGit(t, f.source.WorkspacePath, nil, "show", ":file.bin"); !bytes.Equal(actual, f.staged) {
		t.Fatalf("restored actual index reads different staged bytes: %q", actual)
	}
	var body bytes.Buffer
	meta, err := f.m.artifacts.CopyTo(context.Background(), result.EffectsArtifact.ArtifactID, &body, 64<<20)
	if err != nil || meta.Kind != "graph_operation_effects" || meta.AttemptID != strings.Repeat("b", 32) {
		t.Fatal("object-store effects lack their actual operation owner", err)
	}
	var effects GraphObjectEffects
	if err = json.Unmarshal(body.Bytes(), &effects, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, effect := range effects.Effects {
		if effect.ObjectID == f.object {
			found = effect.Kind == "git_object" && effect.Action == "restored" && effect.SourceArtifactID != "" && effect.ObservedAt != ""
		}
	}
	if !found {
		t.Fatal("closed effects did not identify the real shared-object restoration")
	}
	assertGraphIndexOriginalPreserved(t, f)
}

func TestGraphIndexCaptureRejectsChangedRawIndex(t *testing.T) {
	f := graphIndexFixture(t)
	runGit(t, f.source.WorkspacePath, nil, "add", "file.bin")
	result, err := f.m.CaptureGraphIndex(context.Background(), f.request)
	if err == nil || result.StagedBlobs.ArtifactID != "" {
		t.Fatal("capture accepted an index different from its retained raw basis", err)
	}
	assertGraphIndexOriginalPreserved(t, f)
}

func TestGraphIndexRestoreRejectsCorruptClosedBlob(t *testing.T) {
	f := graphIndexFixture(t)
	closure, err := f.m.CaptureGraphIndex(context.Background(), f.request)
	if err != nil {
		t.Fatal("real staged-blob capture unavailable", err)
	}
	blob := graphIndexBlob(t, graphIndexManifest(t, f, closure.StagedBlobs), f.object)
	meta, err := f.m.artifacts.VerifyClosed(context.Background(), blob.ArtifactRef.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(f.m.lock.Root(), filepath.FromSlash(meta.RelativePath)), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte("X"), 0)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("controlled retained-blob corruption failed", writeErr, closeErr)
	}
	_, err = f.m.RestoreGraphIndexObjects(context.Background(), GraphIndexRestoreRequest{
		OperationAttemptID: strings.Repeat("c", 32), Target: f.source,
		WorkingManifest: f.request.WorkingManifest, RawIndex: f.request.RawIndex, StagedBlobs: closure.StagedBlobs,
	})
	if !errors.Is(err, evidence.ErrCorrupt) {
		t.Fatal("restore ignored corrupt closed staged bytes because a Git object still existed", err)
	}
	assertGraphIndexOriginalPreserved(t, f)
}
