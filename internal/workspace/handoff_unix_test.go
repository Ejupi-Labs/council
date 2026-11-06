//go:build linux || darwin

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandoffPreservesCapturedRepositoryRootSymlink(t *testing.T) {
	m, repo, _ := fixture(t)
	if err := os.Symlink(".", filepath.Join(repo, "repository-link")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	source, err := m.Capture(ctx, Request{AttemptID: attemptID, ProjectPath: repo})
	if err != nil {
		t.Fatal("valid existing source link did not capture", err)
	}
	candidate, err := m.Candidate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.RetainIndex(ctx, source.AttemptID, source)
	if err != nil {
		t.Fatal(err)
	}
	next, err := m.Handoff(ctx, HandoffRequest{AttemptID: strings.Repeat("b", 32), Source: source, CandidateManifestArtifactID: candidate.ManifestArtifactID, IndexArtifactID: index.ID})
	if err != nil {
		t.Fatal("handoff rejected a symlink already accepted and retained by source capture", err)
	}
	target, err := os.Readlink(filepath.Join(next.WorkspacePath, "repository-link"))
	if err != nil || target != "." {
		t.Fatalf("retained root-relative source link changed: %q / %v", target, err)
	}
}
