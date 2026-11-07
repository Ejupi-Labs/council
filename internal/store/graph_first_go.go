package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) graphNodeAdmissionAnchor(ctx context.Context, q queryer, d domain.GraphDispatch) (domain.GraphDispatchRecord, error) {
	anchor, e := s.graphDispatchRecord(ctx, q, d.GraphID, d.NodeAdmissionID)
	if e != nil {
		return anchor, e
	}
	a := anchor.Public.Dispatch
	if a.ID != d.NodeAdmissionID || a.NodeAdmissionID != a.ID || a.GraphID != d.GraphID || a.GraphVersion != d.GraphVersion || a.NodeID != d.NodeID || a.TaskID != d.TaskID || a.TaskVersion != d.TaskVersion || a.InputID != d.InputID || a.InputSHA256 != d.InputSHA256 || a.WorkflowID != d.WorkflowID || a.Role != "author" || a.ExecutorKind != "native" {
		return anchor, evidenceUnavailable()
	}
	if e = s.graphDispatchOwnerAuthorization(ctx, q, a, d); e != nil {
		return anchor, e
	}
	if (anchor.KnownFirstGOAt == nil) != (anchor.KnownFirstGOAttemptID == nil) {
		return anchor, storageError()
	}
	if anchor.KnownFirstGOAt != nil {
		if _, invalid := domain.ParseRoutingTime(*anchor.KnownFirstGOAt); invalid != nil {
			return anchor, storageError()
		}
		actual, e := s.graphDispatchForAttempt(ctx, q, *anchor.KnownFirstGOAttemptID)
		if e != nil {
			return anchor, e
		}
		observed := actual.Public.Dispatch
		if observed.NodeAdmissionID != a.ID || observed.NodeID != a.NodeID || observed.TaskVersion != a.TaskVersion || observed.InputID != a.InputID || observed.WorkflowID != a.WorkflowID || observed.ExecutorKind != "native" {
			return anchor, evidenceUnavailable()
		}
		if e = s.graphDispatchOwnerAuthorization(ctx, q, a, observed); e != nil {
			return anchor, e
		}
	}
	return anchor, nil
}

func (s *Store) recordGraphKnownFirstGO(ctx context.Context, tx *sql.Tx, a domain.Attempt, at string) error {
	member, e := s.graphMembership(ctx, tx, a.TaskID)
	if e != nil || member == nil {
		return e
	}
	if member.Kind != "node" || attemptExecutor(a) != "native" {
		return nil
	}
	if _, invalid := domain.ParseRoutingTime(at); invalid != nil {
		return invalid
	}
	if !a.LaunchIntent || a.SourceSnapshotID == nil || a.Phase != "running" || a.Stop != nil {
		return graphControlError()
	}
	if e = s.checkAttemptClaim(ctx, tx, a.OwnerInstanceID, a.ID); e != nil {
		return e
	}
	record, e := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if e != nil {
		return e
	}
	anchor, e := s.graphNodeAdmissionAnchor(ctx, tx, record.Public.Dispatch)
	if e != nil {
		return e
	}
	if anchor.KnownFirstGOAt != nil {
		return nil
	}
	anchor.KnownFirstGOAt = &at
	anchor.KnownFirstGOAttemptID = &a.ID
	return saveGraphDispatch(ctx, tx, anchor)
}
