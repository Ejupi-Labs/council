package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) StopAttempt(ctx context.Context, attemptID, requestID string) domain.Result {
	return s.mutate(ctx, requestID, "attempts.stop", Fingerprint("attempts.stop", struct {
		AttemptID string
		RequestID string
	}{attemptID, requestID}), func(tx *sql.Tx) (int, any, error) {
		a, e := s.attempt(ctx, tx, attemptID)
		if e != nil {
			return 0, nil, e
		}
		if a.Phase == "settled" {
			return 200, map[string]any{"attemptId": attemptID, "stopAccepted": false, "reason": "already_settled", "attemptRevision": a.Revision, "cleanupState": a.Cleanup.State}, nil
		}
		if a.Stop == nil {
			seq, e := s.appendEvent(ctx, tx, attemptID, "stop_accepted", nil)
			if e != nil {
				return 0, nil, e
			}
			a.Stop = &domain.AcceptedStop{RequestID: requestID, AcceptedAt: domain.Now(), EventSequence: seq}
			a.Phase = "cancelling"
			t, e := s.task(ctx, tx, a.TaskID)
			if e != nil {
				return 0, nil, e
			}
			t.Revision, e = increment(t.Revision)
			if e != nil {
				return 0, nil, e
			}
			t.UpdatedAt = domain.Now()
			if e = saveTask(ctx, tx, t); e != nil {
				return 0, nil, e
			}
			if e = s.bumpState(ctx, tx, &a); e != nil {
				return 0, nil, e
			}
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if !a.LaunchIntent && (g.ActiveAttemptID == nil || *g.ActiveAttemptID != a.ID) {
			a, e = s.settle(ctx, tx, a, domain.Settlement{Outcome: "cancelled", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"})
			if e != nil {
				return 0, nil, e
			}
		}
		return 202, map[string]any{"attemptId": attemptID, "stopAccepted": true, "attemptRevision": a.Revision, "cleanupState": a.Cleanup.State}, nil
	})
}
func (s *Store) Shutdown(ctx context.Context, ownerID, requestID string) domain.Result {
	return s.mutate(ctx, requestID, "service.stop", Fingerprint("service.stop", struct {
		OwnerInstanceID string
		RequestID       string
	}{ownerID, requestID}), func(tx *sql.Tx) (int, any, error) {
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The requested owner instance is no longer current. Contact the current owner explicitly.")
		}
		state := "closing"
		if g.State == "fenced" {
			state = "fenced"
		}
		if _, e = tx.ExecContext(ctx, "UPDATE execution_control SET state=? WHERE singleton=1", state); e != nil {
			return 0, nil, storageError()
		}
		return 202, map[string]any{"ownerInstanceId": ownerID, "shutdownAccepted": true, "admission": "closing"}, nil
	})
}
