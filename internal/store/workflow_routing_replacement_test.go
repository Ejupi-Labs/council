package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"testing"
)

func TestWorkflowRoutingReplacementCreatesNewExecutionOfSameLogicalSlot(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, in, f := workflowAdmissionFixture(t, s)
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	yes := true
	categories := []string{"pre_launch_ineligible"}
	in.Routing = &domain.RoutingOptions{Suitability: "routine", Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "1", Categories: &categories, AllowReplacement: &yes, AllowEscalation: &yes}}
	created := s.CreateTaskWithWorkflow(ctx, owner, in, f)
	if created.Status != 201 {
		t.Fatal(created)
	}
	a := created.Envelope.Data.(domain.Admission)
	no := false
	disabled := s.UpdateRoutingContext(ctx, "codex-native", domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
	if disabled.Status != 200 {
		t.Fatal(disabled)
	}
	blocked, e := s.DispatchRouting(ctx, a.Attempt.ID, f.Routing)
	if e != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" {
		t.Fatal(blocked, e)
	}
	predecessor, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil || predecessor.AdvanceProof != nil || predecessor.Status != "blocked" {
		t.Fatal("blocked target produced 005 authority", predecessor, e)
	}
	replacement, e := s.ContinueRouting(ctx, owner, a.Attempt.ID, f.Routing)
	if e != nil || replacement == nil {
		t.Fatal("004A replacement unavailable", replacement, e)
	}
	if replacement.Workflow == nil || replacement.Step == nil || replacement.Attempt.WorkflowID != a.Workflow.ID || replacement.Attempt.StepID != replacement.Step.ID || replacement.Attempt.Role != a.Attempt.Role || replacement.Step.ID == a.Step.ID || replacement.Step.ReplacementOfStepID == nil || *replacement.Step.ReplacementOfStepID != a.Step.ID || replacement.Step.LogicalKey != a.Step.LogicalKey || replacement.Step.ExecutionOrdinal != "2" || replacement.Step.Ordinal != "2" {
		t.Fatal("routing replacement escaped or rebound its logical workflow slot", replacement)
	}
	if replacement.Attempt.PreviousAttemptID == nil || *replacement.Attempt.PreviousAttemptID != a.Attempt.ID || replacement.RoutingDecision.Kind != "automatic" || replacement.Budget.ConsumedAdmissions != "2" {
		t.Fatal("replacement lost causality/debit", replacement)
	}
	old, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil || !reflect.DeepEqual(old, predecessor) {
		t.Fatal("replacement rewrote immutable blocked predecessor", old, e)
	}
	current, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil || current.Workflow.CurrentStepID == nil || *current.Workflow.CurrentStepID != replacement.Step.ID {
		t.Fatal("replacement did not exclusively become current", current, e)
	}
	next, e := s.ContinueWorkflow(ctx, owner, a.Step.ID, domain.WorkflowStepAdmissionFacts{})
	if e != nil || next != nil {
		t.Fatal("blocked predecessor also advanced 005", next, e)
	}
	repeated, e := s.ContinueRouting(ctx, owner, a.Attempt.ID, f.Routing)
	if e != nil || repeated != nil {
		t.Fatal("004A predecessor admitted twice", repeated, e)
	}
	if stopped := s.StopAttempt(ctx, a.Attempt.ID, domain.NewID()); stopped.Status != 200 {
		t.Fatal(stopped)
	}
	still, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil || still.Workflow.CurrentStepID == nil || *still.Workflow.CurrentStepID != replacement.Step.ID || still.Workflow.State == "cancelled" {
		t.Fatal("old Stop cancelled successor", still, e)
	}
}
