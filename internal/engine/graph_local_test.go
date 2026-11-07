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

func TestGraphLocalRootCaptureRetainsRealClosureWithoutNativeAdmission(t *testing.T) {
	e, projectID, repo := testEngine(t)
	ctx := context.Background()
	dirty := []byte("\xef\xbb\xbfgraph dirty original 東京\r\n")
	if err := os.WriteFile(filepath.Join(repo, "graph-dirty.txt"), dirty, 0600); err != nil {
		t.Fatal(err)
	}
	beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{{Key: "one", OriginalText: "First complete instruction", FakeScenario: "graph-branch-a"}, {Key: "two", OriginalText: "Second complete instruction", FakeScenario: "graph-branch-b"}}}
	var admission domain.GraphAdmission
	var attempt domain.Attempt
	func() {
		e.control.Lock()
		defer e.control.Unlock()
		created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, in, facts)
		if created.Envelope.Error != nil {
			t.Fatal(created)
		}
		var ok bool
		admission, ok = created.Envelope.Data.(domain.GraphAdmission)
		if !ok {
			t.Fatal("Missing typed graph admission", created)
		}
		attempt, err = e.db.ClaimAttempt(ctx, admission.PreparationAttemptID, e.config.OwnerInstanceID)
		if err != nil {
			t.Fatal(err)
		}
	}()
	e.run(ctx, attempt)
	input, err := e.db.GraphInputRecord(ctx, admission.Graph.ID, *admission.Graph.RootInputID)
	if err != nil || input.Public.Summary.State != "ready" || input.PrivateRoot == nil {
		t.Fatal("A real bounded local basis did not become ready", input, err)
	}
	settled, err := e.db.Attempt(ctx, attempt.ID)
	if err != nil || settled.Outcome == nil || *settled.Outcome != "succeeded" || settled.SourceSnapshotID == nil || settled.LaunchIntent || settled.ResolvedTarget != nil {
		t.Fatal("Local basis did not settle without a native launch", settled, err)
	}
	root := *input.PrivateRoot
	manifest, err := e.readGraphArtifact(ctx, root.Manifest, "source_manifest", 64<<20)
	if err != nil || !bytes.Contains(manifest, []byte("graph-dirty.txt")) {
		t.Fatal("Dirty source missing from the retained closure", err)
	}
	if _, err := e.verifyGraphArtifact(ctx, root.Index, "index_snapshot"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.verifyGraphArtifact(ctx, root.StagedBlobs, "graph_index_blobs"); err != nil {
		t.Fatal(err)
	}
	budget, err := e.db.GraphBudget(ctx, admission.Graph.ID)
	if err != nil || budget.Budget.ConsumedAdmissions != "0" || budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("Local basis spent or opened native authority", budget, err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatal("User raw index changed", err)
	}
	afterDirty, err := os.ReadFile(filepath.Join(repo, "graph-dirty.txt"))
	if err != nil || !bytes.Equal(dirty, afterDirty) {
		t.Fatal("User working bytes changed", err)
	}
}

func TestGraphLocalKnownSourceDriftRetainsEffectsWithoutFencing(t *testing.T) {
	for _, change := range []string{"base", "branch"} {
		t.Run(change, func(t *testing.T) {
			e, projectID, repo := testEngine(t)
			ctx := context.Background()
			facts, err := e.captureGraphCreateFacts(ctx, projectID)
			if err != nil {
				t.Fatal(err)
			}
			in := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{{Key: "a", OriginalText: "Keep the admitted source basis"}}}
			var admission domain.GraphAdmission
			var attempt domain.Attempt
			func() {
				e.control.Lock()
				defer e.control.Unlock()
				r := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, in, facts)
				if r.Envelope.Error != nil {
					t.Fatal(r)
				}
				admission = r.Envelope.Data.(domain.GraphAdmission)
				attempt, err = e.db.ClaimAttempt(ctx, admission.PreparationAttemptID, e.config.OwnerInstanceID)
				if err != nil {
					t.Fatal(err)
				}
			}()
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=0")
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatal(string(output), err)
				}
			}
			if change == "base" {
				if err := os.WriteFile(filepath.Join(repo, "user-new-base.txt"), []byte("An explicit user commit after graph admission\n"), 0600); err != nil {
					t.Fatal(err)
				}
				git("add", "user-new-base.txt")
				git("commit", "--quiet", "-m", "User changed the base")
			} else {
				git("switch", "--quiet", "-c", "user-changed-branch")
			}
			e.run(ctx, attempt)
			input, err := e.db.GraphInputRecord(ctx, admission.Graph.ID, *admission.Graph.RootInputID)
			if err != nil || input.Public.Summary.State != "blocked" || input.Public.Summary.Current || input.PrivateRoot != nil || input.Public.OperationEffects == nil {
				t.Fatal("Known source drift did not retain a nonqualifying completion with its actual effects", input, err)
			}
			settled, err := e.db.Attempt(ctx, attempt.ID)
			if err != nil || settled.Outcome == nil || *settled.Outcome != "blocked" {
				t.Fatal("Known source conflict was left unsettled", settled, err)
			}
			e.control.Lock()
			fenced := e.fenced
			e.control.Unlock()
			if fenced {
				t.Fatal("Known source conflict fenced unrelated local work")
			}
			if _, err := e.verifyGraphArtifact(ctx, *input.Public.OperationEffects, "graph_operation_effects"); err != nil {
				t.Fatal("Known capture effects lost their closed evidence", err)
			}
			claims, err := e.db.GraphClaims(ctx)
			if err != nil || len(claims) != 0 {
				t.Fatal("Known completed local work retained managed capacity", claims, err)
			}
		})
	}
}
