package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphLateLocalCompletionPreservesAcceptedGroupStop(t *testing.T) {
	s, g, _, owner := graphLocalFixture(t)
	ctx := context.Background()
	if _, e := s.ClaimAttempt(ctx, g.PreparationAttemptID, owner); e != nil {
		t.Fatal(e)
	}
	stopID := domain.NewID()
	r := s.StopGraph(ctx, owner, g.Graph.ID, domain.GraphStopInput{RequestID: stopID, ExpectedGraphControlRevision: "1"})
	if r.Status != 202 {
		t.Fatal(r)
	}
	if e := s.CompleteGraphInput(ctx, owner, g.PreparationAttemptID, domain.GraphInputCompletionFacts{State: "blocked", Failure: domain.Fail("preparation_cancelled", "The controlled local operation observed accepted Stop before any filesystem I/O."), EffectCount: "0", EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}}); e != nil {
		t.Fatal(e)
	}
	view, e := s.Graph(ctx, g.Graph.ID)
	if e != nil || view.Graph.Stop == nil || view.Graph.Stop.RequestID != stopID || view.Graph.State != "cancelled" || view.Graph.ControlRevision != "2" {
		t.Fatal("late local completion retconned accepted group Stop", view, e)
	}
	for _, n := range view.Nodes {
		if n.State != "cancelled" {
			t.Fatal("late local completion rewrote stopped node", n)
		}
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("known zero-I/O cancellation lost cleanup truth", gate, e)
	}
}
