package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// A's accepted response feeds C's actual waiting author. D keeps a not-yet-launched
// descendant in scope, so stopping the already accepted A is a meaningful Stop.
// This uses owned offline processes, never manufactured execution observations.
func graphAncestorLiveFixture(t *testing.T, beforeGO bool) (graphParallelFixture, <-chan struct{}, func()) {
	t.Helper()
	f := graphParallelStart(t, "1", false, func(in *domain.GraphStartInput) {
		in.Nodes = []domain.GraphNodeInput{
			{Key: "a", OriginalText: "Return the complete controlled original as a response without changing project files.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
			{Key: "c", OriginalText: "Receive the accepted predecessor response and author branch A after the fixed release, then independently inspect the complete candidate.", FakeScenario: "graph-wait", Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}},
			{Key: "d", OriginalText: "Remain an unstarted dependent until C is actually accepted.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		}
		edges := []domain.GraphDependencyInput{{From: "a", To: "c", Kind: "context"}, {From: "c", To: "d", Kind: "files"}}
		in.Dependencies = &edges
	})
	prepared, release := make(chan struct{}), make(chan struct{})
	var signalOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if beforeGO {
		f.preparations.mu.Lock()
		f.preparations.hook = func(ctx context.Context, spec process.NativeSpec, owned *process.Owned, err error) (*process.Owned, error) {
			if err == nil && slices.Contains(spec.Args, "graph-wait") {
				signalOnce.Do(func() { close(prepared) })
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return owned, err
		}
		f.preparations.mu.Unlock()
	}
	// Observe the three complete prerequisite jobs with fixed, non-renewing
	// deadlines. Actual ACK and the follower's original limits remain separate.
	a, actual := graphAncestorWaitPrerequisites(t, f)
	in := exactResponseAcceptanceInput(t, f.e, a.TaskID, actual.ID)
	assertOriginalResponseAcceptanceWire(t, f.e.AcceptResponse(context.Background(), a.TaskID, in), in)
	return f, prepared, unblock
}

func graphAncestorNode(t *testing.T, view domain.GraphDetail, key string) domain.GraphNodeSummary {
	t.Helper()
	for _, node := range view.Nodes {
		if node.Key == key {
			return node
		}
	}
	t.Fatal("The declared ancestor fixture node is absent", key)
	return domain.GraphNodeSummary{}
}

func graphAncestorControl(t *testing.T, f graphParallelFixture, guidance bool) (domain.GraphDetail, string) {
	t.Helper()
	ctx := context.Background()
	view, err := f.e.db.Graph(ctx, f.graphID)
	if err != nil {
		t.Fatal(err)
	}
	a := graphAncestorNode(t, view, "a")
	if a.AcceptedResult == nil || a.State != "completed" {
		t.Fatal("The actual predecessor has not been explicitly accepted", a)
	}
	requestID := domain.NewID()
	var input any = domain.GraphNodeStopInput{RequestID: requestID, ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: a.ID, ExpectedNodeControlRevision: a.ControlRevision}
	op := "graphs.nodes.stop"
	if guidance {
		op = "graphs.nodes.guidance"
		input = domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: requestID, ExpectedRevision: a.TaskRevision, GuidanceText: "New explicit ancestor guidance; retain the previous accepted evidence as history."}, ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: a.ID, ExpectedNodeControlRevision: a.ControlRevision, TaskVersion: a.TaskVersion}
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result := f.e.MutateGraph(ctx, op, f.graphID+"/a", body)
	if result.Envelope.Error != nil || result.Status != 200 && result.Status != 202 {
		t.Fatal("The exact ancestor control was not accepted", result)
	}
	if !guidance {
		var stopped domain.GraphStopResult
		encoded, encodeErr := json.Marshal(result.Envelope.Data)
		decodeErr := json.Unmarshal(encoded, &stopped, json.RejectUnknownMembers(true))
		if encodeErr != nil || decodeErr != nil || !stopped.StopAccepted || stopped.Reason != "accepted" || stopped.Node == nil || stopped.Node.ID != a.ID || stopped.Node.Stop == nil || stopped.Node.Stop.RequestID != requestID {
			t.Fatal("The ancestor fixture accepted a late no-op instead of an actual scoped Stop", result, stopped, encodeErr, decodeErr)
		}
	}
	after, err := f.e.db.Graph(ctx, f.graphID)
	if err != nil || after.Graph.ControlRevision != view.Graph.ControlRevision || after.Graph.Stop != nil {
		t.Fatal("The ancestor control changed the group Stop scope", after, err)
	}
	return after, requestID
}

func graphAncestorAssertPreserved(t *testing.T, f graphParallelFixture, view domain.GraphDetail, admissions string) {
	t.Helper()
	d := graphAncestorNode(t, view, "d")
	dispatches, err := f.e.db.GraphDispatches(context.Background(), f.graphID, "", d.Key, 100, "")
	if err != nil || len(dispatches.Items) != 0 || d.AcceptedResult != nil || view.Graph.State == "completed" || view.Budget.Budget.ConsumedAdmissions != admissions || view.Budget.Budget.GrantedAdmissions != "4" || view.Graph.BudgetAuthorityEpoch != "1" {
		t.Fatal("Ancestor control launched a waiting descendant, inferred success or refilled native allowance", view, dispatches, err)
	}
	gate, err := f.e.db.Gate(context.Background())
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("Known actual cleanup retained a claim or fenced", gate, err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, "source.txt")); err != nil || !bytes.Equal(body, f.working) {
		t.Fatal("The actual ancestor race changed user working bytes", err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(body, f.stagedIndex) {
		t.Fatal("The actual ancestor race changed the exact user index", err)
	}
}

func TestGraphAncestorStopBeforeFirstGOPreventsPreparedFollowerStart(t *testing.T) {
	f, prepared, release := graphAncestorLiveFixture(t, true)
	select {
	case <-prepared:
	case <-time.After(15 * time.Second):
		t.Fatalf("The controlled follower did not reach actual process Prepare: %s", graphParallelDiagnostics(f))
	}
	before, _ := graphAncestorControl(t, f, false)
	c := graphAncestorNode(t, before, "c")
	if c.LatestAttemptID == nil {
		t.Fatal("The genuinely prepared follower has no exact attempt", c)
	}
	release()
	deadline := time.Now().Add(8 * time.Second)
	for {
		actual, err := f.e.db.Attempt(context.Background(), *c.LatestAttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if actual.Phase == "settled" {
			if actual.StartedAt != nil || actual.Cleanup.State != "verified" || actual.Outcome == nil || *actual.Outcome == "succeeded" || actual.EvidenceStatus == "uncertain" {
				var incident domain.ExecutionIncident
				if actual.Cleanup.IncidentID != nil {
					incident, _ = f.e.db.Incident(context.Background(), *actual.Cleanup.IncidentID)
				}
				t.Fatalf("The ancestor-stopped prepared follower did not retain known no-GO: started=%v outcome=%v failure=%+v cleanup=%+v evidence=%s incident=%+v", actual.StartedAt, actual.Outcome, actual.PrimaryFailure, actual.Cleanup, actual.EvidenceStatus, incident)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("The pre-GO ancestor Stop did not settle its actual prepared process: %s", graphParallelDiagnostics(f))
		}
		time.Sleep(50 * time.Millisecond)
	}
	view, err := f.e.db.Graph(context.Background(), f.graphID)
	if err != nil || graphAncestorNode(t, view, "c").AcceptedResult != nil {
		t.Fatal("The unstarted follower became accepted", view, err)
	}
	graphAncestorAssertPreserved(t, f, view, "2")
}

func TestGraphAncestorStopAfterKnownGOPreservesOnlyAdmittedFollowerWorkflow(t *testing.T) {
	f, _, _ := graphAncestorLiveFixture(t, false)
	ready := graphParallelAwait(t, f, 15*time.Second, "The dependent author did not actually reach known GO", func(workers []graphParallelWorker, view domain.GraphDetail) bool {
		return len(workers) == 1 && workers[0].node.Key == "c"
	})
	before, requestID := graphAncestorControl(t, f, false)
	if a := graphAncestorNode(t, before, "a"); a.Stop == nil || a.Stop.RequestID != requestID || a.AcceptedResult == nil {
		t.Fatal("The stopped accepted ancestor lost its immutable result or exact Stop", a)
	}
	if err := graphParallelRelease(f, ready[0].cwd); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		view, err := f.e.db.Graph(context.Background(), f.graphID)
		if err != nil {
			t.Fatal(err)
		}
		c := graphAncestorNode(t, view, "c")
		if c.State == "completed" && c.AcceptedResult != nil {
			if c.InputID == nil || ready[0].node.InputID == nil || *c.InputID != *ready[0].node.InputID || c.TaskVersion != ready[0].node.TaskVersion || c.Stop != nil {
				t.Fatal("Known-GO continuation replaced the admitted input or inherited another node's Stop", c)
			}
			graphAncestorAssertPreserved(t, f, view, "3")
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("The known-GO follower lost its unchanged admitted review continuation: %s", graphParallelDiagnostics(f))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestGraphAncestorNewVersionAfterKnownGORejectsOldInputContinuation(t *testing.T) {
	f, _, _ := graphAncestorLiveFixture(t, false)
	ready := graphParallelAwait(t, f, 15*time.Second, "The dependent author did not actually reach known GO", func(workers []graphParallelWorker, view domain.GraphDetail) bool {
		return len(workers) == 1 && workers[0].node.Key == "c"
	})
	after, _ := graphAncestorControl(t, f, true)
	if a := graphAncestorNode(t, after, "a"); a.TaskVersion != "2" || a.AcceptedResult != nil || a.InputID != nil || a.LatestAttemptID == nil {
		t.Fatal("The new ancestor guidance did not retain a fresh unlaunched version", a)
	}
	if err := graphParallelRelease(f, ready[0].cwd); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		actual, err := f.e.db.Attempt(context.Background(), ready[0].attempt.ID)
		gate, gateErr := f.e.db.Gate(context.Background())
		if err != nil || gateErr != nil {
			t.Fatal(err, gateErr)
		}
		if actual.Phase == "settled" && len(gate.ActiveClaims) == 0 {
			view, err := f.e.db.Graph(context.Background(), f.graphID)
			if err != nil {
				t.Fatal(err)
			}
			c := graphAncestorNode(t, view, "c")
			if c.AcceptedResult != nil || c.LatestAttemptID == nil || *c.LatestAttemptID != actual.ID || actual.StartedAt == nil || actual.Outcome == nil || *actual.Outcome != "succeeded" || actual.Cleanup.State != "verified" {
				t.Fatal("Changed ancestor input qualified a stale receiver or lost its actual completed author evidence", c, actual)
			}
			graphAncestorAssertPreserved(t, f, view, "2")
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("The actual stale-input author failed to retain a known settled outcome: %s", graphParallelDiagnostics(f))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
