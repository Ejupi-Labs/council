package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphClaimedLocalInputRetainsScopeAcrossExplicitAdvance(t *testing.T) {
	s, op, completion, owner := graphDependentCompletionFixture(t, "")
	ctx := context.Background()
	before, e := s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	advance := s.AdvanceGraph(ctx, owner, op.GraphID, graphAdvanceInput(before), domain.GraphAdvanceFacts{})
	if advance.Status != 200 {
		t.Fatal(advance)
	}
	after, e := s.Graph(ctx, op.GraphID)
	if e != nil || after.DispatchAuthority == nil || after.DispatchAuthority.Generation == op.Control.Generation {
		t.Fatal("fixture did not create a separate future-work registration", after, e)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, op.Attempt.ID); e != nil {
		t.Fatal("unchanged exact claimed local scope was revoked by a new admission registration", e)
	}
	if e = s.CompleteGraphInput(ctx, owner, op.Attempt.ID, completion); e != nil {
		t.Fatal(e)
	}
	input, e := s.GraphInput(ctx, op.GraphID, op.ID)
	if e != nil || input.Summary.State != "ready" || !input.Summary.Current || input.InputBinding == nil || input.InputBinding.ID != completion.InputBinding.ID {
		t.Fatal("independent explicit Advance made actual completed input stale", input, e)
	}
	budget, e := s.GraphBudget(ctx, op.GraphID)
	if e != nil || budget.Budget.ConsumedAdmissions != before.Budget.Budget.ConsumedAdmissions || budget.Budget.CurrentWindow.Number != before.Budget.Budget.CurrentWindow.Number {
		t.Fatal("local completion or registration changed shared model authority", budget, e)
	}
}

func TestGraphClaimedLocalInputStillObeysAcceptedStopAfterAdvance(t *testing.T) {
	s, op, completion, owner := graphDependentCompletionFixture(t, "")
	ctx := context.Background()
	view, e := s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	if advance := s.AdvanceGraph(ctx, owner, op.GraphID, graphAdvanceInput(view), domain.GraphAdvanceFacts{}); advance.Status != 200 {
		t.Fatal(advance)
	}
	current, e := s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	stop := s.StopGraph(ctx, owner, op.GraphID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: current.Graph.ControlRevision})
	if stop.Status != 202 {
		t.Fatal(stop)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, op.Attempt.ID); e == nil {
		t.Fatal("preserved local authority ignored an accepted group Stop")
	}
	if e = s.CompleteGraphInput(ctx, owner, op.Attempt.ID, completion); e != nil {
		t.Fatal("late actual prefix could not settle after Stop", e)
	}
	input, e := s.GraphInput(ctx, op.GraphID, op.ID)
	if e != nil || input.Summary.Current || input.Summary.State == "ready" {
		t.Fatal("late actual completion rearmed stopped input", input, e)
	}
	current, e = s.Graph(ctx, op.GraphID)
	if e != nil || current.Graph.Stop == nil || current.Graph.State != "cancelled" || current.Nodes[1].State != "cancelled" {
		t.Fatal("late settlement replaced accepted cancellation scope", current, e)
	}
}
