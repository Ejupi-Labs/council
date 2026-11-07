package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

func graphRootRequest(t *testing.T, repo, selected string) GraphRootCaptureRequest {
	t.Helper()
	ctx := context.Background()
	projectState, err := project.Inspect(ctx, selected, selected)
	if err != nil {
		t.Fatal(err)
	}
	repositoryState, err := project.Inspect(ctx, repo, repo)
	if err != nil {
		t.Fatal(err)
	}
	return GraphRootCaptureRequest{GraphID: strings.Repeat("1", 32), RootTaskID: strings.Repeat("2", 32), OperationAttemptID: strings.Repeat("3", 32), ProjectID: strings.Repeat("4", 32), ProjectPath: projectState.Observation.ResolvedPath, ProjectIdentity: projectState.Identity, RepositoryIdentity: repositoryState.Identity}
}

func TestGraphRootCaptureRetainsExactDirtyWorkingAndStagedBasis(t *testing.T) {
	m, repo, _ := fixture(t)
	staged := []byte("\xef\xbb\xbfgraph root staged 東京\r\n\x00\xfd")
	working := []byte("graph root different working🙂\r\n\x00\xfe")
	put(t, repo, "file.bin", staged)
	runGit(t, repo, nil, "add", "file.bin")
	object := strings.TrimSpace(string(runGit(t, repo, nil, "rev-parse", ":file.bin")))
	put(t, repo, "file.bin", working)
	put(t, repo, "untracked.txt", []byte("untracked original\r\n"))
	beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	request := graphRootRequest(t, repo, repo)
	result, err := m.CaptureGraphRoot(context.Background(), request)
	if err != nil {
		t.Fatal("actual graph root capture unavailable", err)
	}
	binding := result.Binding
	if result.Snapshot == nil || binding.GraphID != request.GraphID || binding.RootTaskID != request.RootTaskID || binding.OperationAttemptID != request.OperationAttemptID || result.Snapshot.AttemptID != request.OperationAttemptID || binding.SourceSnapshotID != result.Snapshot.ID || binding.Manifest.ArtifactID != result.Snapshot.ManifestArtifactID || binding.Index.ArtifactID != result.Snapshot.IndexArtifactID || binding.ProjectID != request.ProjectID || binding.ProjectIdentity != request.ProjectIdentity || binding.RepositoryIdentity != request.RepositoryIdentity || binding.SelectedRelativePath != "." || binding.Branch == nil || *binding.Branch != "refs/heads/main" {
		t.Fatalf("root capture invented/lost operation, source or scope identity: %+v", binding)
	}
	digest, err := domain.GraphRootDigest(binding)
	if err != nil || digest != binding.BasisSHA256 || !domain.ValidSHA256(digest) {
		t.Fatal("actual root binding is not privately sealed", err)
	}
	var raw bytes.Buffer
	if _, err = m.artifacts.CopyTo(context.Background(), binding.Index.ArtifactID, &raw, 64<<20); err != nil || !bytes.Equal(raw.Bytes(), beforeIndex) {
		t.Fatal("root did not retain exact user's raw index", err)
	}
	var manifestBody bytes.Buffer
	if _, err = m.artifacts.CopyTo(context.Background(), binding.Manifest.ArtifactID, &manifestBody, 64<<20); err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err = json.Unmarshal(manifestBody.Bytes(), &manifest, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	var member Entry
	for _, entry := range manifest.Entries {
		if entry.Path == "file.bin" {
			member = entry
		}
	}
	var retainedWorking bytes.Buffer
	if _, err = m.artifacts.CopyTo(context.Background(), member.ArtifactID, &retainedWorking, MaxFileBytes); err != nil || !bytes.Equal(retainedWorking.Bytes(), working) {
		t.Fatal("root substituted staged bytes for actual working bytes", err)
	}
	indexManifest := graphIndexManifest(t, graphIndexCase{m: m}, binding.StagedBlobs)
	stageRef := graphIndexBlob(t, indexManifest, object)
	var retainedStaged bytes.Buffer
	if _, err = m.artifacts.CopyTo(context.Background(), stageRef.ArtifactRef.ArtifactID, &retainedStaged, MaxFileBytes); err != nil || !bytes.Equal(retainedStaged.Bytes(), staged) {
		t.Fatal("root did not retain its distinct actual staged blob", err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(afterIndex, beforeIndex) {
		t.Fatal("root preparation changed the user's original raw index", err)
	}
	// Later mutable source/workspace changes cannot rewrite the retained basis.
	put(t, repo, "file.bin", []byte("later user change\r\n"))
	put(t, result.Snapshot.WorkspacePath, "file.bin", []byte("later old-workspace change\r\n"))
	retainedWorking.Reset()
	if _, err = m.artifacts.CopyTo(context.Background(), member.ArtifactID, &retainedWorking, MaxFileBytes); err != nil || !bytes.Equal(retainedWorking.Bytes(), working) {
		t.Fatal("retained graph basis followed mutable source/workspace", err)
	}
}

func TestGraphRootCaptureRetainsActualSelectedSubfolder(t *testing.T) {
	m, repo, _ := fixture(t)
	selected := filepath.Join(repo, "docs-東京")
	put(t, selected, "note.txt", []byte("selected controlled data\r\n"))
	request := graphRootRequest(t, repo, selected)
	result, err := m.CaptureGraphRoot(context.Background(), request)
	if err != nil || result.Snapshot == nil {
		t.Fatal("selected graph-root capture unavailable", err)
	}
	if result.Binding.SelectedRelativePath != "docs-東京" || result.Snapshot.ExecutionPath != filepath.Join(result.Snapshot.WorkspacePath, "docs-東京") || result.Binding.ProjectIdentity != request.ProjectIdentity || result.Binding.RepositoryIdentity != request.RepositoryIdentity {
		t.Fatalf("selected project scope or physical identities changed: %+v", result.Binding)
	}
	if body, err := os.ReadFile(filepath.Join(result.Snapshot.ExecutionPath, "note.txt")); err != nil || string(body) != "selected controlled data\r\n" {
		t.Fatal("actual selected graph root files are unavailable", err)
	}
}

func TestGraphRootCaptureRejectsChangedPhysicalProjectIdentity(t *testing.T) {
	m, repo, _ := fixture(t)
	request := graphRootRequest(t, repo, repo)
	request.ProjectIdentity = "a-different-physical-project"
	result, err := m.CaptureGraphRoot(context.Background(), request)
	if err == nil || result.Snapshot != nil || result.Binding.SourceSnapshotID != "" {
		t.Fatal("root capture ignored captured project identity", err)
	}
}
