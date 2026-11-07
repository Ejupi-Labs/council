package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This is the actual automatic-completion path after owned fake workers and
// real local application. The SQL-only acceptance fixtures are separate.
func TestGraphFinalAcceptanceAfterActualCumulativeApplication(t *testing.T) {
	graphActualFinalAcceptance(t, false)
}

func TestGraphFinalAcceptanceAfterOwnerRecoveryDoesNotReacquireDelivery(t *testing.T) {
	graphActualFinalAcceptance(t, true)
}

func graphActualFinalAcceptance(t *testing.T, recoverOwner bool) {
	t.Helper()
	f := graphDeliveryAttemptFixture(t)
	e, ctx := f.final.engine, context.Background()
	e.run(ctx, f.delivery)
	application, err := e.db.Attempt(ctx, f.delivery.ID)
	if err != nil || application.Phase != "settled" || application.Outcome == nil || *application.Outcome != "succeeded" || application.EvidenceStatus != "complete" {
		t.Fatalf("Actual delivery prerequisite failed: attempt=%+v err=%v details=%s", application, err, graphDeliveryFailureDiagnostics(e, application))
	}
	automaticPredecessor := application.StepID
	if recoverOwner {
		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := next.Close(); err != nil {
				t.Error(err)
			}
		})
		stopFixtureScheduler(t, next)
		e = next
		automaticPredecessor = "" // A fresh user acknowledgement is not old live intent.
		if err := e.db.CheckGraphDispatchPreparation(ctx, e.config.OwnerInstanceID, application.ID); err == nil {
			t.Fatal("Recovery unexpectedly renewed the settled delivery's old execution authority")
		}
	}
	w, err := e.db.Workflow(ctx, application.TaskID, domain.WorkflowSelector{WorkflowID: application.WorkflowID})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, application.TaskID, *w.Workflow)
	if err != nil || progress.decision.Action != "accept" || progress.final == nil || progress.operationID == nil {
		t.Fatal("Actual cumulative delivery has no current nonexecuting acceptance decision", progress.decision, err)
	}
	requestID := domain.NewID()
	accepted := e.acceptCurrentWorkflow(ctx, progress, requestID, automaticPredecessor)
	if accepted.Status != 200 || accepted.Envelope.Error != nil {
		t.Fatal("Actual recorded cumulative delivery could not close its exact final and graph", accepted)
	}
	current, err := e.db.Workflow(ctx, application.TaskID, domain.WorkflowSelector{WorkflowID: application.WorkflowID})
	if err != nil || current.Workflow == nil || current.Workflow.State != "completed" || current.Acceptance == nil || current.Acceptance.AcceptedByRequestID != requestID ||
		current.Acceptance.CandidateBinding != f.binding || current.Acceptance.DeliveryOperationID == nil || *current.Acceptance.DeliveryOperationID != *progress.operationID || current.Acceptance.DeliveryResult != "applied" {
		t.Fatal("Final acceptance lost its actual selected candidate, destination journal or completion", current, err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.GraphID)
	if err != nil || view.Graph.State != "completed" || view.Graph.Counts.Completed != "3" || !reflect.DeepEqual(view.Budget.Budget, f.admitted.Budget.Budget) {
		t.Fatal("All-required graph completion was not backed by the actual unchanged shared allowance", view, err)
	}
	for _, node := range view.Nodes {
		if node.State != "completed" || node.AcceptedResult == nil || node.AcceptedResult.TaskVersion != node.TaskVersion {
			t.Fatal("Graph acceptance hid a required unfinished branch", node)
		}
		if node.ID == f.dispatch.NodeID && (node.AcceptedResult.AcceptanceID != current.Acceptance.ID || node.AcceptedResult.CandidateID == nil || *node.AcceptedResult.CandidateID != f.binding.CandidateID) {
			t.Fatal("The final projection substituted a supporting operation for its actual producer", node)
		}
	}
	candidate, err := e.db.Candidate(ctx, f.candidate.ID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta || candidate.AttemptID != f.candidate.AttemptID || candidate.RootSourceSnapshotID != f.candidate.RootSourceSnapshotID {
		t.Fatal("Accepting the cumulative graph manufactured final-node authorship", candidate, err)
	}
	f.assertUserBasis()
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("Nonexecuting final acknowledgement acquired or stranded a worker claim", gate, err)
	}
}
