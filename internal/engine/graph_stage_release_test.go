package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphOrdinaryClaimReleaseWakesExactWaitingReviewWithoutAuthorReplay(t *testing.T) {
	f := graphOutputAdmittedNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Produce the controlled file and independently review its exact content.",
		Pins: &domain.ExecutionPins{HarnessID: "codex"}, FakeScenario: "graph-branch-a", Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete controlled source change."}})
	e, author := f.engine, f.attempt
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	e.run(ctx, author)
	actual, err := e.db.Attempt(ctx, author.ID)
	if err != nil || actual.Outcome == nil || *actual.Outcome != "succeeded" {
		t.Fatal("The actual graph author did not settle its review prerequisite", actual, err)
	}
	// A real ordinary reservation occupies the literal native context. Its
	// no-process settlement below releases actual capacity; it is never a fake
	// provider success or an invented graph completion.
	ordinary := decodeAdmission(t, e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: f.rootBinding.ProjectID,
		OriginalText: "Reserve this explicit offline ordinary response fixture.", Pins: &domain.ExecutionPins{HarnessID: "codex"}, ExecutionMode: "fake", FakeScenario: "response",
		Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}))
	blocking, err := e.db.ClaimAttempt(ctx, ordinary.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil || blocking.Phase != "preparing" {
		t.Fatal("The ordinary fixture did not reserve its actual native context", blocking, err)
	}
	t.Cleanup(func() {
		a, err := e.db.Attempt(context.Background(), blocking.ID)
		if err == nil && a.Phase != "settled" {
			_, err = e.db.SettleWorkflowStep(context.Background(), a.ID, graphReleaseNoProcessFacts(e, a))
		}
		if err != nil {
			t.Error(err)
		}
		view, err := e.db.Workflow(context.Background(), author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
		if err != nil || view.Workflow == nil || view.Workflow.CurrentStepID == nil {
			return
		}
		step, err := e.db.WorkflowStep(context.Background(), *view.Workflow.CurrentStepID)
		if err != nil || step.AttemptID == nil || *step.AttemptID == author.ID {
			return
		}
		a, err = e.db.Attempt(context.Background(), *step.AttemptID)
		if err == nil && a.Phase != "settled" {
			_, err = e.db.SettleWorkflowStep(context.Background(), a.ID, graphReleaseNoProcessFacts(e, a))
		}
		if err != nil {
			t.Error(err)
		}
	})
	e.continueWorkflow(author.ID)
	waiting, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || waiting.Workflow == nil || waiting.Workflow.NextAction != "capacity_wait" || waiting.Workflow.NextOwner != "council" || waiting.Workflow.CurrentStepID == nil || *waiting.Workflow.CurrentStepID != author.StepID {
		t.Fatal("The exact review did not enter a passive native-context wait", waiting, err)
	}
	before, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || before.Budget.ConsumedAdmissions != "1" {
		t.Fatal("Waiting consumed a native admission", before, err)
	}
	if _, err = e.db.SettleWorkflowStep(ctx, blocking.ID, graphReleaseNoProcessFacts(e, blocking)); err != nil {
		t.Fatal(err)
	}
	e.continueGraphAdmissions(author.ID)
	unchanged, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || unchanged.Workflow == nil || unchanged.Workflow.CurrentStepID == nil || *unchanged.Workflow.CurrentStepID != author.StepID {
		t.Fatal("Reclassifying an old completion rearmed its consumed wait", unchanged, err)
	}
	e.continueGraphAdmissions(blocking.ID)
	woken, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || woken.Workflow == nil || woken.Workflow.CurrentStepID == nil || *woken.Workflow.CurrentStepID == author.StepID {
		t.Fatal("The real unrelated ordinary claim release did not wake the exact graph review", woken, err)
	}
	step, err := e.db.WorkflowStep(ctx, *woken.Workflow.CurrentStepID)
	if err != nil || step.Role != "review" || step.AttemptID == nil || step.InputCandidateID == nil || actual.CandidateID == nil || *step.InputCandidateID != *actual.CandidateID {
		t.Fatal("Capacity wake replayed the author or changed its selected candidate", step, err)
	}
	d, err := e.db.GraphDispatchForAttempt(ctx, *step.AttemptID)
	if err != nil || d.Public.Dispatch.NodeAdmissionID != f.dispatch.Dispatch.NodeAdmissionID || d.Public.Dispatch.FreshTriggerID != blocking.ID || d.Public.Dispatch.SourceBasis.BasisKind != "node_subject" {
		t.Fatal("The new review lost its exact release/source/admission association", d, err)
	}
	after, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || after.Budget.ConsumedAdmissions != "2" || after.Budget.CurrentWindow.Number != before.Budget.CurrentWindow.Number {
		t.Fatal("One released context created the wrong number of root debits", after, err)
	}
}
