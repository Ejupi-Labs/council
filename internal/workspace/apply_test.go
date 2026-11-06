package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/project"
)

func applicationFixture(t *testing.T) (*Manager, string, ApplyRequest) {
	t.Helper()
	m, repo, _ := fixture(t)
	ctx := context.Background()
	put(t, repo, "notes.txt", []byte("original text\r\n"))
	put(t, repo, "delete.txt", []byte("retained deletion before bytes\r\n"))
	runGit(t, repo, nil, "add", "notes.txt", "delete.txt")
	runGit(t, repo, nil, "commit", "-m", "application fixture")
	source, err := m.Capture(ctx, Request{AttemptID: attemptID, ProjectPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	put(t, source.WorkspacePath, "notes.txt", []byte("\xef\xbb\xbfcandidate café 東京 �\r\n"))
	put(t, source.WorkspacePath, "added.txt", []byte("new controlled text\r\n"))
	if err = os.Remove(filepath.Join(source.WorkspacePath, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	candidate, err := m.Candidate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := m.artifacts.VerifyClosed(ctx, candidate.ManifestArtifactID)
	if err != nil || metadata.SHA256 == nil {
		t.Fatal(err)
	}
	observation, err := project.Inspect(ctx, source.RepositoryPath, source.RepositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	return m, repo, ApplyRequest{AttemptID: strings.Repeat("e", 32), OperationID: strings.Repeat("f", 32), Source: source, SelectedCandidateID: candidate.ID, SelectedAttemptID: attemptID, CandidateManifestArtifactID: candidate.ManifestArtifactID, CandidateManifestSHA256: *metadata.SHA256, ProjectIdentity: observation.Identity, RepositoryIdentity: observation.Identity}
}

func TestApplicationPublishesExactCumulativeTextDeltaAndPreservesUserIndex(t *testing.T) {
	m, repo, request := applicationFixture(t)
	ctx := context.Background()
	// Later user changes outside the affected task paths are neither reset nor staged.
	put(t, repo, "file.bin", []byte("later staged user bytes\x00\xff"))
	runGit(t, repo, nil, "add", "file.bin")
	put(t, repo, "later-user.txt", []byte("untracked user bytes\r\n"))
	indexPath, err := m.value(ctx, repo, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		t.Fatal(err)
	}
	beforeIndex, err := readIndex(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	application, err := m.PrepareApply(ctx, request)
	if application != nil {
		t.Cleanup(func() { application.Close() })
	}
	if err != nil {
		t.Fatal("supported application did not prepare", err)
	}
	if len(application.Actions()) != 4 {
		t.Fatalf("expected add/delete/quarantine/publish actions: %+v", application.Actions())
	}
	for _, action := range application.Actions() {
		result, e := application.ExecuteAction(ctx, action.ID)
		if e != nil || !result.Known || result.State != "applied" {
			t.Fatalf("supported action %+v returned %+v / %v", action, result, e)
		}
	}
	if err = application.Close(); err != nil {
		t.Fatal("application did not release its checked handles", err)
	}
	exact, err := os.ReadFile(filepath.Join(repo, "notes.txt"))
	if err != nil || string(exact) != "\xef\xbb\xbfcandidate café 東京 �\r\n" {
		t.Fatal("application changed exact text bytes", err)
	}
	if _, err = os.Lstat(filepath.Join(repo, "delete.txt")); !os.IsNotExist(err) {
		t.Fatal("supported deletion did not occur", err)
	}
	if added, err := os.ReadFile(filepath.Join(repo, "added.txt")); err != nil || string(added) != "new controlled text\r\n" {
		t.Fatal("supported addition differs", err)
	}
	if user, err := os.ReadFile(filepath.Join(repo, "later-user.txt")); err != nil || string(user) != "untracked user bytes\r\n" {
		t.Fatal("untracked user work changed", err)
	}
	afterIndex, err := readIndex(indexPath)
	if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatal("user staging was rewritten", err)
	}
}
