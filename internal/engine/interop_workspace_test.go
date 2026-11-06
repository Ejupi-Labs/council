package engine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func interopGit(t *testing.T, repo string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s (%v)", args, output, err)
	}
	return output
}

func TestInteropClaudeCapturedInstructionsAndCandidatePreserveOriginalEditsIndexBranch(t *testing.T) {
	e, project, repo := testEngine(t)
	staged := []byte("staged user source\r\n")
	working := []byte("newer unstaged user source\r\n")
	instructions := []byte("# CLAUDE\r\nPreserve this captured project instruction, café \ufeff \ufffd.\r\n")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), staged, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), instructions, 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, repo, "add", "--", "source.txt", "CLAUDE.md")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), working, 0600); err != nil {
		t.Fatal(err)
	}
	beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	beforeHead := interopGit(t, repo, "rev-parse", "HEAD")
	beforeBranch := interopGit(t, repo, "symbolic-ref", "HEAD")
	beforeStatus := interopGit(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	a := settled(t, e, admitClaude(t, e, project, "candidate").Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" || a.CandidateID == nil || a.SourceSnapshotID == nil || !a.AuthoredDelta {
		t.Fatalf("%+v failure=%+v", a, a.PrimaryFailure)
	}
	candidate, err := e.db.Candidate(context.Background(), *a.CandidateID)
	if err != nil || !candidate.HasDelta || candidate.Verification != "pending" || candidate.Integration != "not_performed" {
		t.Fatal(candidate, err)
	}
	managed := filepath.Join(e.lock.Root(), filepath.FromSlash(candidate.WorkspaceRef))
	for _, tc := range []struct {
		name string
		want []byte
	}{{"source.txt", working}, {"CLAUDE.md", instructions}} {
		got, err := os.ReadFile(filepath.Join(managed, tc.name))
		if err != nil || !bytes.Equal(got, tc.want) {
			t.Fatal("captured working instruction/source changed", tc.name, got, err)
		}
		got, err = os.ReadFile(filepath.Join(repo, tc.name))
		if err != nil || !bytes.Equal(got, tc.want) {
			t.Fatal("original user bytes changed", tc.name, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "council-fixture.txt")); !os.IsNotExist(err) {
		t.Fatal("candidate escaped managed workspace", err)
	}
	if got, err := os.ReadFile(filepath.Join(repo, ".git", "index")); err != nil || !bytes.Equal(got, beforeIndex) {
		t.Fatal("original staging bytes changed", err)
	}
	if !bytes.Equal(beforeHead, interopGit(t, repo, "rev-parse", "HEAD")) || !bytes.Equal(beforeBranch, interopGit(t, repo, "symbolic-ref", "HEAD")) || !bytes.Equal(beforeStatus, interopGit(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")) {
		t.Fatal("original branch/head/staging/worktree state changed")
	}
	detail, err := e.db.Task(context.Background(), a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ResponseArtifactID == nil {
		t.Fatal("fixture response missing")
	}
	meta, err := e.evidence.VerifyClosed(context.Background(), *a.ResponseArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	r := e.AcceptResponse(context.Background(), a.TaskID, domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: a.TaskVersion, AttemptID: a.ID, ResponseArtifactID: meta.ID, ResponseSHA256: *meta.SHA256, Confirmation: "reviewed_response_and_conditions"})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "acceptance_unavailable" {
		t.Fatal("response-only acceptance accepted authored candidate", r)
	}
}
