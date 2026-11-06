package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandoffMaterializesRetainedCandidateNotCurrentProjectOrOldWorkspace(t *testing.T) {
	m, repo, _ := fixture(t)
	ctx := context.Background()
	put(t, repo, "notes.txt", []byte("original\r\n"))
	runGit(t, repo, nil, "add", "notes.txt")
	runGit(t, repo, nil, "commit", "-m", "document")
	source, err := m.Capture(ctx, Request{AttemptID: attemptID, ProjectPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	exact := []byte("\xef\xbb\xbfretained candidate café 東京 �\r\n")
	put(t, source.WorkspacePath, "notes.txt", exact)
	runGit(t, source.WorkspacePath, nil, "add", "notes.txt")
	candidate, err := m.Candidate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.RetainIndex(ctx, attemptID, source)
	if err != nil {
		t.Fatal(err)
	}
	expectedIndex, err := m.index(ctx, source.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	// Neither this later original edit nor a mutable old workspace is authority.
	put(t, repo, "notes.txt", []byte("later user edit\n"))
	put(t, source.WorkspacePath, "notes.txt", []byte("old workspace tampered\n"))
	next, err := m.Handoff(ctx, HandoffRequest{AttemptID: "abcdefabcdefabcdefabcdefabcdefab", Source: source, CandidateManifestArtifactID: candidate.ManifestArtifactID, IndexArtifactID: index.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(next.WorkspacePath, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, exact) {
		t.Fatalf("handoff selected current/old source instead of retained candidate: %q", got)
	}
	actualIndex, err := m.index(ctx, next.WorkspacePath)
	if err != nil || !indexEqual(expectedIndex, actualIndex) {
		t.Fatalf("handoff staging differs: %+v / %+v (%v)", expectedIndex, actualIndex, err)
	}
	if next.ID == source.ID || next.WorkspacePath == source.WorkspacePath || next.AttemptID == source.AttemptID {
		t.Fatal("handoff reused author context")
	}
	current, err := os.ReadFile(filepath.Join(repo, "notes.txt"))
	if err != nil || string(current) != "later user edit\n" {
		t.Fatal("handoff changed user's later work", err)
	}
}
