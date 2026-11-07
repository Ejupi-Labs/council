package store

import (
	"context"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// recoveryActiveGraphCheckpoints applies one bounded, validated inventory to
// every transactional graph-pause reader. An excess is never silently omitted.
func (s *Store) recoveryActiveGraphCheckpoints(ctx context.Context, q queryer, graphID string) ([]domain.RecoveryCheckpoint, error) {
	rows, err := q.QueryContext(ctx, "SELECT data FROM recovery_checkpoints WHERE graph_id=? AND state IN ('pausing','paused') ORDER BY id LIMIT 18", graphID)
	if err != nil {
		return nil, storageError()
	}
	defer rows.Close()
	out := []domain.RecoveryCheckpoint{}
	seen := map[string]bool{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, storageError()
		}
		cp, err := decode[domain.RecoveryCheckpoint](raw)
		if err != nil || cp.Scope.GraphID != graphID {
			return nil, storageError()
		}
		if err = recoveryTrackActiveCheckpoint(seen, cp); err != nil {
			return nil, err
		}
		out = append(out, cp)
		if len(out) > maxRecoveryActiveCandidates {
			return nil, domain.Fail("checkpoint_scope_overflow", "Too many active pause records affect this graph; inspect recovery state before continuing.")
		}
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return out, nil
}

// RecoveryAncestorsPaused is read-only planning support. Admission repeats
// the check inside its transaction against the then-current graph and pauses.
func (s *Store) RecoveryAncestorsPaused(ctx context.Context, graphID, nodeID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.requireRecoveryAncestors(ctx, s.db, graphID, nodeID)
	var fault *domain.Error
	if errors.As(err, &fault) && fault.Code == "scope_paused" {
		return true, nil
	}
	return false, err
}

// requireRecoveryAncestors rejects new preparation or initial admission when a
// transitive prerequisite is paused. The caller must invoke it inside the same
// transaction as that admission. Already-admitted work retains its authority.
func (s *Store) requireRecoveryAncestors(ctx context.Context, q queryer, graphID, nodeID string) error {
	if !domain.ValidID(graphID) || !domain.ValidID(nodeID) {
		return domain.Invalid("graph and node identity")
	}
	h, err := s.graphHeader(ctx, q, graphID)
	if err != nil {
		return err
	}
	nodes, err := s.graphNodes(ctx, q, graphID)
	if err != nil {
		return err
	}
	keys := make(map[string]string, len(nodes))
	target := ""
	for _, node := range nodes {
		keys[node.Summary.ID] = node.Summary.Key
		if node.Summary.ID == nodeID {
			target = node.Summary.Key
		}
	}
	if target == "" {
		return notFound()
	}
	definition, err := s.graphDefinition(ctx, q, graphID, h.Summary.GraphVersion)
	if err != nil {
		return err
	}
	ancestors := map[string]bool{target: true}
	if definition.Original.Dependencies != nil {
		for range nodes {
			for _, edge := range *definition.Original.Dependencies {
				if ancestors[edge.To] {
					ancestors[edge.From] = true
				}
			}
		}
	}
	pauses, err := s.recoveryActiveGraphCheckpoints(ctx, q, graphID)
	if err != nil {
		return err
	}
	for _, pause := range pauses {
		if pause.Scope.Kind != "member" {
			continue
		}
		pausedID := pause.Scope.NodeID
		key, ok := keys[pausedID]
		if !ok {
			return storageError()
		}
		if pausedID != nodeID && ancestors[key] {
			return domain.Fail("scope_paused", "A prerequisite is paused. Inspect its checkpoint before admitting this dependent work.")
		}
	}
	return nil
}
