package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Ordinary accounting debits do not grant authority. Graph proofs bind the
// explicit graph authority epoch/window; standalone proofs retain revision binding.
func (s *Store) graphBudgetProofCurrent(ctx context.Context, q queryer, taskID, observedRevision, graphEpoch, window string) (bool, error) {
	b, e := s.taskBudget(ctx, q, taskID)
	if e != nil {
		return false, e
	}
	if b == nil || b.CurrentWindow.Number != window {
		return false, nil
	}
	member, e := s.graphMembership(ctx, q, taskID)
	if e != nil {
		return false, e
	}
	if member == nil {
		return graphEpoch == "" && b.Revision == observedRevision, nil
	}
	h, e := s.graphHeader(ctx, q, member.GraphID)
	if e != nil {
		return false, e
	}
	return graphEpoch != "" && graphEpoch == h.Summary.BudgetAuthorityEpoch, nil
}
func (s *Store) graphAttemptBudgetEpoch(ctx context.Context, q queryer, a domain.Attempt) (string, error) {
	member, e := s.graphMembership(ctx, q, a.TaskID)
	if e != nil {
		return "", e
	}
	if member == nil {
		return "", nil
	}
	record, e := s.graphDispatchForAttempt(ctx, q, a.ID)
	if e != nil {
		return "", e
	}
	if record.Public.Dispatch.GraphID != member.GraphID || record.Public.Dispatch.TaskID != a.TaskID || record.Public.Dispatch.TaskVersion != a.TaskVersion {
		return "", evidenceUnavailable()
	}
	return record.Public.Dispatch.BudgetAuthorityEpoch, nil
}
