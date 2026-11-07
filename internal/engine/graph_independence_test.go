package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphKnownSiblingInputFailurePreservesAdmittedWorkflowStageAuthority(t *testing.T) {
	f := graphOutputAdmittedNodeFixture(t,
		domain.GraphNodeInput{Key: "producer", OriginalText: "Create the controlled file and obtain its independent review.", FakeScenario: "graph-branch-a", Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete controlled file change."}},
		domain.GraphNodeInput{Key: "sibling", OriginalText: "Prepare an independent sibling without changing the producer's authority."})
	e, a := f.engine, f.attempt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Keep the actor stopped while invoking only named real transitions.
	e.ctx, e.cancel = ctx, cancel
	e.continueGraphInputs(f.rootBinding.OperationAttemptID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var sibling *domain.Attempt
	for i := range pending {
		if pending[i].ExecutorKind == "local_graph" && pending[i].Phase == "queued" {
			if sibling != nil {
				t.Fatal("The fixture registered an unexpected additional local operation")
			}
			sibling = &pending[i]
		}
	}
	if sibling == nil {
		t.Fatal("The real root completion did not register the independent sibling input", pending)
	}
	b, err := e.db.ClaimAttempt(ctx, sibling.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := e.db.GraphInputOperationForAttempt(ctx, e.config.OwnerInstanceID, b.ID)
	if err != nil || op.Kind != "node_input" {
		t.Fatal(op, err)
	}
	// Cancellation occurs before the first task-version read and any artifact
	// intent/I/O. The actual local runner must settle this known zero-effect
	// failure, rather than manufacturing global process/evidence uncertainty.
	interrupted, interrupt := context.WithCancel(ctx)
	interrupt()
	e.runGraphNodeInput(interrupted, b, op)
	failed, err := e.db.Attempt(ctx, b.ID)
	if err != nil || failed.Phase != "settled" || failed.Outcome == nil || *failed.Outcome == "succeeded" || failed.Cleanup.State != "verified" || failed.LaunchIntent || failed.EvidenceStatus == "uncertain" {
		t.Fatal("The actual known sibling failure was not retained faithfully", failed, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != a.ID {
		t.Fatal("A known sibling preparation failure fenced or stole the admitted producer", gate, err)
	}
	e.run(ctx, a)
	producer, err := e.db.Attempt(ctx, a.ID)
	if err != nil || producer.Outcome == nil || *producer.Outcome != "succeeded" {
		t.Fatal("The unrelated known failure prevented the actual admitted worker", producer, err)
	}
	w, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, a.TaskID, *w.Workflow)
	if err != nil || progress.decision.Role != "review" {
		t.Fatal("The retained exact producer no longer has its review plan", progress.decision, err)
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, a.StepID)
	if err != nil || facts.Graph == nil || facts.Graph.Control.OperationAttemptID != a.ID || facts.Graph.NodeAdmissionID != f.dispatch.Dispatch.NodeAdmissionID {
		t.Fatal("Independent sibling failure stranded unchanged admitted workflow authority", facts, err)
	}
}
