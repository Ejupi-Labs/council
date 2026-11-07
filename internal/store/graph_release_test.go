package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphClaimReleaseRequiresAnActualPriorReservation(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(map[bool]string{false: "never_claimed_stop", true: "known_claimed_release"}[claimed], func(t *testing.T) {
			s, graph, root, proofs, owner := graphReadyBasis(t)
			ctx := context.Background()
			request := taskInput(graph.Graph.ProjectID)
			r := s.CreateTask(ctx, owner, request)
			if r.Status != 201 {
				t.Fatal(r)
			}
			a := r.Envelope.Data.(domain.Admission).Attempt
			if claimed {
				if _, e := s.ClaimAttempt(ctx, a.ID, owner); e != nil {
					t.Fatal(e)
				}
				if _, e := s.SettleAttempt(ctx, a.ID, domain.Settlement{Outcome: "failed", Failure: &domain.PrimaryFailure{Code: "controlled_release", Message: "Controlled failure with known claim release."}, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
					t.Fatal(e)
				}
			} else {
				if stopped := s.StopAttempt(ctx, a.ID, domain.NewID()); stopped.Status != 202 {
					t.Fatal(stopped)
				}
			}
			facts := graphBeginFacts(graph, graph.Nodes[0], root, proofs, owner)
			facts.Trigger.Kind = "claim_release"
			facts.Trigger.ID = a.ID
			facts.Control.OperationAttemptID = a.ID
			_, e := s.BeginGraphInput(ctx, owner, graph.Graph.ID, graph.Nodes[0].ID, facts)
			if claimed && e != nil {
				t.Fatal("known actual release could not wake current authorized graph", e)
			}
			if !claimed && e == nil {
				t.Fatal("a queued never-claimed terminal invented a fresh capacity release")
			}
		})
	}
}
