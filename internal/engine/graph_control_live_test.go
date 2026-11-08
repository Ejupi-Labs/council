package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphNodeStopCancelsLiveBranchAndBlocksWaitingFollowerWhileIndependentSiblingFinishes(t *testing.T) {
	f := graphParallelStart(t, "", false, func(in *domain.GraphStartInput) {
		in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "dependent", OriginalText: "Receive only the accepted A file before running this controlled dependent author.", FakeScenario: "graph-branch-b", Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}})
		edges := []domain.GraphDependencyInput{{From: "a", To: "dependent", Kind: "files"}}
		in.Dependencies = &edges
	})
	e, ctx := f.e, context.Background()
	// Allow independent source preparation before either fake worker starts;
	// the first worker's fixed eight-second release deadline begins only once
	// it is running, so the overlap check below remains separately bounded.
	graphParallelAwait(t, f, 30*time.Second, "The live Stop fixture did not establish its first actual running worker", func(workers []graphParallelWorker, view domain.GraphDetail) bool {
		return len(workers) > 0
	})
	ready := graphParallelAwait(t, f, 6*time.Second, "The live Stop fixture did not establish both actual running workers", func(workers []graphParallelWorker, view domain.GraphDetail) bool {
		return len(workers) == 2
	})
	var a, b graphParallelWorker
	for _, worker := range ready {
		if worker.node.Key == "a" {
			a = worker
		} else if worker.node.Key == "b" {
			b = worker
		}
	}
	if a.attempt.ID == "" || b.attempt.ID == "" || a.attempt.ResolvedTarget == nil || b.attempt.ResolvedTarget == nil || a.attempt.ResolvedTarget.ContextID == b.attempt.ResolvedTarget.ContextID {
		t.Fatal("The Stop fixture did not establish two independently owned contexts", ready)
	}
	view, err := e.db.Graph(ctx, f.graphID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Graph.Counts.Running != "2" {
		t.Errorf("The actual two running workers were projected as running=%s queued=%s group=%s", view.Graph.Counts.Running, view.Graph.Counts.Queued, view.Graph.State)
	}
	requestID := domain.NewID()
	body, err := json.Marshal(domain.GraphNodeStopInput{RequestID: requestID, ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: a.node.ID, ExpectedNodeControlRevision: a.node.ControlRevision})
	if err != nil {
		t.Fatal(err)
	}
	stopped := e.MutateGraph(ctx, "graphs.nodes.stop", f.graphID+"/a", body)
	if stopped.Status != 202 || stopped.Envelope.Error != nil {
		t.Fatal("The actual running node Stop was not accepted", stopped)
	}
	// Release the independent worker immediately. Waiting for A's process
	// cleanup first could consume B's unchanged fixed eight-second barrier.
	if err := graphParallelRelease(f, b.cwd); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		view, err = e.db.Graph(ctx, f.graphID)
		if err != nil {
			t.Fatal(err)
		}
		completed := false
		for _, node := range view.Nodes {
			if node.Key == "b" && node.State == "completed" && node.AcceptedResult != nil {
				completed = true
			}
		}
		actual, readErr := e.db.Attempt(ctx, a.attempt.ID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if completed && actual.Phase == "settled" {
			if actual.Outcome == nil || *actual.Outcome != "cancelled" || actual.Stop == nil || actual.Stop.RequestID != requestID || actual.PrimaryFailure != nil || actual.Cleanup.State != "verified" || actual.EvidenceStatus == "uncertain" {
				t.Fatal("The cancelled live branch lost its accepted Stop or retained false success/uncertainty", actual)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Node Stop stranded the independent admitted sibling: %s", graphParallelDiagnostics(f))
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, node := range view.Nodes {
		switch node.Key {
		case "a":
			if node.Stop == nil || node.Stop.RequestID != requestID || node.AcceptedResult != nil || node.State != "cancelled" {
				t.Fatal("The stopped live branch was qualified or lost its exact Stop", node)
			}
		case "dependent":
			if node.Stop != nil || node.AcceptedResult != nil || node.State != "blocked" || node.NextAction != "dependency_stopped" {
				t.Fatal("The unlaunched follower lost its exact ancestor dependency blocker", node)
			}
		case "b":
			if node.Stop != nil || node.ControlRevision != b.node.ControlRevision || node.TaskVersion != b.node.TaskVersion || node.AcceptedResult == nil {
				t.Fatal("The unrelated running sibling inherited another branch's Stop", node)
			}
		}
		if node.Key == "dependent" {
			dispatches, err := e.db.GraphDispatches(ctx, f.graphID, "", node.Key, 100, "")
			if err != nil || len(dispatches.Items) != 0 {
				t.Fatal("The stopped waiting follower obtained native work", dispatches, err)
			}
		}
	}
	if view.Graph.State == "completed" || view.Graph.Stop != nil || view.Budget.Budget.GrantedAdmissions != "4" || view.Budget.Budget.ConsumedAdmissions != "3" || view.Graph.BudgetAuthorityEpoch != "1" {
		t.Fatal("Branch Stop became group success, group Stop or allowance renewal", view)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("Known branch cleanup did not release only its actual claims", gate, err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, "source.txt")); err != nil || !bytes.Equal(body, f.working) {
		t.Fatal("Branch cancellation changed the user's working source", err)
	}
	if index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(index, f.stagedIndex) {
		t.Fatal("Branch cancellation changed the user's exact staged index", err)
	}
}
