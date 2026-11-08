package store

import (
	"context"
	"database/sql"
	"runtime"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) fence(ctx context.Context, tx *sql.Tx, in domain.ExecutionIncident) error {
	if in.ID == "" {
		in.ID = domain.NewID()
	}
	if in.CreatedAt == "" {
		in.CreatedAt = domain.Now()
	}
	if !domain.ValidID(in.ID) || len(in.Message) > 512 || len(in.Code) > 64 || in.Code == "" {
		return domain.Invalid("bounded incident")
	}
	if _, e := tx.ExecContext(ctx, "INSERT INTO execution_incidents(id,attempt_id,data) VALUES(?,?,?)", in.ID, in.AttemptID, encode(in)); e != nil {
		return storageError()
	}
	if _, e := tx.ExecContext(ctx, "UPDATE execution_control SET state='fenced',incident_id=? WHERE singleton=1", in.ID); e != nil {
		return storageError()
	}
	return nil
}
func (s *Store) Fence(ctx context.Context, in domain.ExecutionIncident) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error { return s.fence(ctx, tx, in) })
}

// RecoverOwner never dispatches retained work. Unknown launched scopes retain capacity and a fence.
func (s *Store) RecoverOwner(ctx context.Context, newOwnerID string) error {
	if !domain.ValidID(newOwnerID) {
		return domain.Invalid("owner identity")
	}
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, "SELECT data FROM attempts WHERE phase!='settled' ORDER BY sequence")
		if e != nil {
			return storageError()
		}
		pending := []domain.Attempt{}
		for rows.Next() {
			var raw string
			if rows.Scan(&raw) != nil {
				rows.Close()
				return storageError()
			}
			a, e := decode[domain.Attempt](raw)
			if e != nil {
				rows.Close()
				return storageError()
			}
			pending = append(pending, a)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return storageError()
		}
		for _, a := range pending {
			if e := s.recoverWorkflowDelivery(ctx, tx, a); e != nil {
				return e
			}
			cleanup := domain.Cleanup{State: "verified", Scope: "not_launched"}
			if a.LaunchIntent {
				cleanup = domain.Cleanup{State: "unknown", Scope: a.Cleanup.Scope}
				if cleanup.Scope == "not_launched" {
					cleanup.Scope = "unix_process_group"
					if runtime.GOOS == "windows" {
						cleanup.Scope = "windows_job"
					}
				}
			}
			effectsUncertain, err := s.workflowEffectUncertain(ctx, tx, a)
			if err != nil {
				return err
			}
			settled, err := s.settleCoreWithRecovery(ctx, tx, a, domain.Settlement{Outcome: "interrupted", Cleanup: cleanup, EvidenceStatus: "incomplete", Failure: nil, EffectsKnown: a.ExecutorKind == "local_application" && !effectsUncertain}, true)
			if err != nil {
				return err
			}
			if err = s.syncWorkflowTerminal(ctx, tx, settled); err != nil {
				return err
			}
		}
		var incidents, claims int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents i WHERE NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')").Scan(&incidents); e != nil {
			return storageError()
		}
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&claims); e != nil {
			return storageError()
		}
		state := "open"
		if incidents > 0 || claims > 0 || g.State == "fenced" {
			state = "fenced"
		}
		// The recovery loop may have appended attempt-specific incidents after
		// BeginOwnerEpoch recorded an unknown prior epoch. Keep that global
		// uncertainty discoverable through the gate without erasing any of the
		// attempt incidents or releasing their claims.
		var epochIncident sql.NullString
		e = tx.QueryRowContext(ctx, `SELECT i.id FROM execution_incidents i
			WHERE json_extract(i.data,'$.code')='owner_epoch_uncertain'
			AND NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')
			ORDER BY i.rowid DESC LIMIT 1`).Scan(&epochIncident)
		if e != nil && e != sql.ErrNoRows {
			return storageError()
		}
		if _, e = tx.ExecContext(ctx, "UPDATE execution_control SET owner_instance_id=?,state=?,incident_id=coalesce(?,incident_id) WHERE singleton=1", newOwnerID, state, epochIncident); e != nil {
			return storageError()
		}
		// Restart never opens a previously fenced gate. Only a current-owner
		// receipted reconciliation may recheck an already reconciled incident.
		return nil
	})
}
func (s *Store) PendingAttempts(ctx context.Context, ownerID string, limit int) ([]domain.Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit < 1 || limit > 100 {
		return nil, domain.Invalid("pending limit")
	}
	rows, e := s.db.QueryContext(ctx, "SELECT data FROM attempts WHERE phase!='settled' ORDER BY sequence LIMIT ?", limit)
	if e != nil {
		return nil, storageError()
	}
	defer rows.Close()
	out := []domain.Attempt{}
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			return nil, storageError()
		}
		a, e := decode[domain.Attempt](raw)
		if e != nil {
			return nil, storageError()
		}
		if ownerID == "" || a.OwnerInstanceID == ownerID {
			out = append(out, observe(a))
		}
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return out, nil
}
func (s *Store) Incident(ctx context.Context, id string) (domain.ExecutionIncident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var raw string
	e := s.db.QueryRowContext(ctx, "SELECT data FROM execution_incidents WHERE id=?", id).Scan(&raw)
	if e != nil {
		return domain.ExecutionIncident{}, notFound()
	}
	return decode[domain.ExecutionIncident](raw)
}

// recoveryReleaseResolvedFence is called only by an explicit owner-mediated
// reconciliation transaction. Unknown fence provenance never authorizes reopening.
func (s *Store) recoveryReleaseResolvedFence(ctx context.Context, tx *sql.Tx) error {
	var state string
	var incidentID sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT state,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &incidentID); err != nil {
		return storageError()
	}
	if state != "fenced" || !incidentID.Valid {
		return nil
	}
	var resolved, unresolved, claims, effects, inputs, historyEffects int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM recovery_decisions WHERE incident_id=? AND json_extract(data,'$.outcome')='reconciled'", incidentID.String).Scan(&resolved); err != nil {
		return storageError()
	}
	if resolved != 1 {
		return nil
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents i WHERE NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')").Scan(&unresolved); err != nil {
		return storageError()
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&claims); err != nil {
		return storageError()
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM delivery_operations WHERE active_effect_id IS NOT NULL OR status IN ('preparing','applying','uncertain')").Scan(&effects); err != nil {
		return storageError()
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM graph_inputs WHERE state IN ('preparing','uncertain')").Scan(&inputs); err != nil {
		return storageError()
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM history_export_operations WHERE state IN ('effect_started','uncertain')").Scan(&historyEffects); err != nil {
		return storageError()
	}
	if unresolved != 0 || claims != 0 || effects != 0 || inputs != 0 || historyEffects != 0 {
		return nil
	}
	result, err := tx.ExecContext(ctx, "UPDATE execution_control SET state='open',incident_id=NULL WHERE singleton=1 AND state='fenced' AND incident_id=?", incidentID.String)
	if err != nil {
		return storageError()
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return storageError()
	}
	return nil
}
