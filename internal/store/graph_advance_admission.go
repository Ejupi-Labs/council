package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) advanceGraphInitialAdmissions(ctx context.Context, tx *sql.Tx, ownerID string, h graphHeader, in domain.GraphAdvanceInput, facts []domain.GraphNodeAdmissionFacts) ([]domain.GraphDispatchView, error) {
	out := []domain.GraphDispatchView{}
	if len(facts) > domain.MaxGraphNodes {
		return nil, domain.Invalid("bounded graph initial admissions")
	}
	seen := map[string]bool{}
	for _, fact := range facts {
		if seen[fact.NodeID] {
			return nil, domain.Invalid("one initial admission per captured node")
		}
		seen[fact.NodeID] = true
		authorized := false
		for _, authority := range in.NodeAuthorities {
			if authority.NodeID == fact.NodeID && authority.TaskVersion == fact.TaskVersion && fact.Control.NodeControlRevision != nil && *fact.Control.NodeControlRevision == authority.NodeControlRevision {
				authorized = true
			}
		}
		if !authorized {
			return nil, graphControlError()
		}
		fact.Control.Generation = h.DispatchAuthority.Generation
		trigger := domain.GraphFreshTrigger{Kind: "explicit", ID: in.RequestID, OwnerGeneration: h.DispatchAuthority.Generation}
		var registered domain.GraphDispatchView
		if err := s.admitGraphNodeTx(ctx, tx, ownerID, h.Summary.ID, trigger, fact, &registered); err != nil {
			return nil, err
		}
		if registered.Dispatch.ID != "" {
			out = append(out, registered)
		}
	}
	return out, nil
}
