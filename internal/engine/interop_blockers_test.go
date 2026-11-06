package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestInteropBothHarnessesRetainUnsupportedSourceWithoutLaunching(t *testing.T) {
	for _, harness := range []string{"codex", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			e, project, repo := testEngine(t)
			interopGit(t, repo, "update-index", "--skip-worktree", "--", "source.txt")
			indexPath := filepath.Join(repo, ".git", "index")
			before, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			in := claudeFixtureInput(project, "candidate")
			in.Pins = &domain.ExecutionPins{HarnessID: harness}
			admitted := decodeAdmission(t, e.StartTask(context.Background(), in))
			a := settled(t, e, admitted.Attempt.ID)
			if a.Outcome == nil || *a.Outcome != "blocked" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "source_capture_blocked" || !strings.Contains(strings.ToLower(a.PrimaryFailure.Message), "sparse") || a.LaunchIntent || a.StartedAt != nil || a.SourceSnapshotID != nil || a.CandidateID != nil || a.Cleanup.State != "verified" || a.Cleanup.Scope != "not_launched" {
				t.Fatalf("unsupported source launched or lost its blocker: %+v failure=%+v", a, a.PrimaryFailure)
			}
			after, err := os.ReadFile(indexPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("blocked source index changed", err)
			}
			body, err := os.ReadFile(filepath.Join(repo, "source.txt"))
			if err != nil || string(body) != "original\n" {
				t.Fatal("blocked source bytes changed", err)
			}
			if _, err := os.Stat(filepath.Join(repo, "council-fixture.txt")); !os.IsNotExist(err) {
				t.Fatal("blocked source received candidate output", err)
			}
		})
	}
}
