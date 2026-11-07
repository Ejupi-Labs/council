package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphStageCapacityWaitConsumesOldSourceAndRequiresNewActualRelease(t *testing.T) {
	s, d, author, owner := graphOutputSettlementFixture(t)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, author); e != nil {
		t.Fatal(e)
	}
	facts := graphReviewAdmissionFacts(t, s, d, author)
	held := []domain.Attempt{}
	task, e := s.Task(ctx, d.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	for _, harness := range []string{"codex", "claude-code"} {
		in := taskInput(task.Task.ProjectID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		a := r.Envelope.Data.(domain.Admission).Attempt
		claimed, e := s.ClaimAttempt(ctx, a.ID, owner)
		if e != nil {
			t.Fatal(e)
		}
		held = append(held, claimed)
	}
	before, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	predecessor, e := s.WorkflowStep(ctx, d.Dispatch.StepID)
	if e != nil {
		t.Fatal(e)
	}
	if next, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, facts); e != nil || next != nil {
		t.Fatal("capacity wait did not durably acknowledge zero new work", next, e)
	}
	waiting, e := s.Workflow(ctx, d.Dispatch.TaskID, domain.WorkflowSelector{})
	if e != nil || waiting.Workflow == nil || waiting.Workflow.NextOwner != "council" || waiting.Workflow.NextAction != "capacity_wait" || waiting.Workflow.Revision != facts.ExpectedWorkflowRevision {
		t.Fatal("capacity wait lost bounded continuation authority or promised wrong action", waiting, e)
	}
	if _, e = s.SettleAttempt(ctx, held[0].ID, domain.Settlement{Outcome: "failed", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
		t.Fatal(e)
	}
	if next, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, facts); e != nil || next != nil {
		t.Fatal("old completion callback rearmed after capacity changed", next, e)
	}
	sameEvent := facts
	copy := *facts.Graph
	sameEvent.Graph = &copy
	sameEvent.Graph.Trigger.Kind = "claim_release"
	if next, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, sameEvent); e != nil || next != nil {
		t.Fatal("one source event was reused under another classification", next, e)
	}
	unchanged, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if e != nil || !reflect.DeepEqual(before, unchanged) {
		t.Fatal("wait/duplicate spent model budget", unchanged, e)
	}
	fresh := facts
	copy = *facts.Graph
	fresh.Graph = &copy
	fresh.Graph.Trigger = domain.GraphFreshTrigger{Kind: "claim_release", ID: held[0].ID, OwnerGeneration: facts.Graph.Trigger.OwnerGeneration}
	next, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, fresh)
	if e != nil || next == nil {
		t.Fatal("new actual release could not resume current waiting slot", next, e)
	}
	after, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if e != nil || after.Budget.ConsumedAdmissions != "2" || after.Budget.RemainingAdmissions != "2" || !reflect.DeepEqual(before.Budget.CurrentWindow, after.Budget.CurrentWindow) {
		t.Fatal("fresh release refilled/reset or duplicated shared debit", after, e)
	}
	old, e := s.WorkflowStep(ctx, d.Dispatch.StepID)
	if e != nil || !reflect.DeepEqual(predecessor, old) {
		t.Fatal("waiting rewrote immutable predecessor proof", old, e)
	}
	if duplicate, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, fresh); e != nil || duplicate != nil {
		t.Fatal("new source consumed twice", duplicate, e)
	}
}
