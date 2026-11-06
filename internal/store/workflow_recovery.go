package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) markWorkflowEffectsUnknown(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT id FROM delivery_operations WHERE attempt_id=?", a.ID).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return storageError()
	}
	var op *domain.DeliveryOperation
	if err == nil {
		record, readErr := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", id)
		if readErr != nil {
			return readErr
		}
		op = &record
		if op.IncidentID != nil {
			return nil
		}
	}
	incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: ptr(a.ID), Code: "effect_uncertain", Message: "The filesystem effects of this local application are unknown. Retain ownership and private quarantine; do not repeat the operation.", CreatedAt: domain.Now()}
	if err = s.fence(ctx, tx, incident); err != nil {
		return err
	}
	if op != nil {
		op.Status = "uncertain"
		op.IncidentID = ptr(incident.ID)
		op.NextAction = "Inspect the retained effect journal and destination; no automatic replay is permitted."
		if _, err = tx.ExecContext(ctx, "UPDATE delivery_operations SET revision=revision+1,status='uncertain',data=? WHERE id=?", encode(*op), op.ID); err != nil {
			return storageError()
		}
	}
	return nil
}

func (s *Store) workflowEffectUncertain(ctx context.Context, q queryer, a domain.Attempt) (bool, error) {
	if a.ExecutorKind != "local_application" {
		return false, nil
	}
	var count int
	if e := q.QueryRowContext(ctx, "SELECT count(*) FROM delivery_operations WHERE attempt_id=? AND (active_effect_id IS NOT NULL OR status='uncertain')", a.ID).Scan(&count); e != nil {
		return false, storageError()
	}
	return count != 0, nil
}

func (s *Store) recoverWorkflowDelivery(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	if a.ExecutorKind != "local_application" {
		return nil
	}
	var id string
	var active sql.NullString
	e := tx.QueryRowContext(ctx, "SELECT id,active_effect_id FROM delivery_operations WHERE attempt_id=?", a.ID).Scan(&id, &active)
	if e == sql.ErrNoRows {
		return nil
	}
	if e != nil {
		return storageError()
	}
	op, e := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", id)
	if e != nil {
		return e
	}
	if op.Status == "applied" || op.Status == "conflict" || op.Status == "partial" || op.Status == "cancelled" || op.Status == "interrupted" {
		if !active.Valid {
			return nil
		}
	}
	now := domain.Now()
	op.FinishedAt = ptr(now)
	if active.Valid || op.Status == "uncertain" {
		op.Status = "uncertain"
		op.NextAction = "Inspect retained filesystem intent, quarantine and destination evidence; do not repeat the effect."
		if op.IncidentID == nil {
			incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: ptr(a.ID), Code: "effect_uncertain", Message: "The prior owner lost acknowledgement of a permitted filesystem effect. Retain its claim and live quarantine; no automatic retry is allowed.", CreatedAt: now}
			if e = s.fence(ctx, tx, incident); e != nil {
				return e
			}
			op.IncidentID = ptr(incident.ID)
		}
	} else {
		op.Status = "interrupted"
		op.NextAction = "Inspect the retained known effects and candidate; no filesystem action is resumed."
	}
	if _, e = tx.ExecContext(ctx, "UPDATE delivery_operations SET revision=revision+1,status=?,data=? WHERE id=?", op.Status, encode(op), op.ID); e != nil {
		return storageError()
	}
	return nil
}
