package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphActualAuthorBudgetBlockProjectsCurrentNodeWithoutNewWork(t *testing.T) {
	f := graphOutputConfiguredNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Create the controlled file result and require an independent review.",
		FakeScenario: "graph-branch-a", Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}}, func(input *domain.GraphStartInput) {
		input.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "1"}}
	})
	e, author := f.engine, f.attempt
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	e.run(ctx, author)
	terminal, err := e.db.Attempt(ctx, author.ID)
	if err != nil || terminal.Outcome == nil || *terminal.Outcome != "succeeded" || terminal.CandidateID == nil {
		t.Fatal("Actual author did not produce the controlled candidate", terminal, err)
	}
	before, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || before.Budget.Budget.ConsumedAdmissions != "1" || before.Budget.Budget.RemainingAdmissions != "0" || len(before.ActiveClaims) != 0 {
		t.Fatal("Expected the actual single-admission author to exhaust the shared allowance", before, err)
	}
	stepBefore, err := e.db.WorkflowStep(ctx, author.StepID)
	if err != nil || stepBefore.AdvanceProof == nil {
		t.Fatal("Expected the actual terminal continuation proof", stepBefore, err)
	}
	e.continueWorkflow(author.ID)
	after, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || after.Graph.State != "blocked" || after.Graph.NextOwner != "user" || after.Nodes[0].State != "blocked" || after.Nodes[0].NextAction != "budget_exhausted" || after.Nodes[0].NextOwner != "user" {
		t.Fatal("Actual exhausted continuation left the graph promising automatic review", after, err)
	}
	workflow, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || workflow.Workflow == nil || workflow.Workflow.State != "blocked" || workflow.Workflow.NextAction != after.Nodes[0].NextAction || workflow.Workflow.NextOwner != after.Nodes[0].NextOwner {
		t.Fatal("Graph and actual workflow do not identify the same block", workflow, err)
	}
	if !reflect.DeepEqual(before.Budget, after.Budget) || len(after.ActiveClaims) != 0 || before.Graph.ControlRevision != after.Graph.ControlRevision || before.Nodes[0].ControlRevision != after.Nodes[0].ControlRevision || before.Nodes[0].TaskVersion != after.Nodes[0].TaskVersion || !reflect.DeepEqual(before.Nodes[0].InputID, after.Nodes[0].InputID) || after.Nodes[0].AcceptedResult != nil {
		t.Fatal("Passive blocked projection changed allowance, control, input, or acceptance", before, after)
	}
	stepAfter, err := e.db.WorkflowStep(ctx, author.StepID)
	if err != nil || !reflect.DeepEqual(stepBefore, stepAfter) {
		t.Fatal("Blocked projection rewrote the actual predecessor or continuation proof", stepAfter, err)
	}
	e.continueWorkflow(author.ID)
	repeated, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || !reflect.DeepEqual(after, repeated) {
		t.Fatal("Old completion callback changed the blocked graph again", repeated, err)
	}
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatal("Blocked or repeated continuation created new work", pending, err)
	}
}
