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
			_, e = s.settle(ctx, tx, a, domain.Settlement{Outcome: "interrupted", Cleanup: cleanup, EvidenceStatus: "incomplete", Failure: nil})
			if e != nil {
				return e
			}
		}
		var incidents, claims int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents").Scan(&incidents); e != nil {
			return storageError()
		}
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&claims); e != nil {
			return storageError()
		}
		state := "open"
		if incidents > 0 || claims > 0 || g.State == "fenced" {
			state = "fenced"
		}
		if _, e = tx.ExecContext(ctx, "UPDATE execution_control SET owner_instance_id=?,state=? WHERE singleton=1", newOwnerID, state); e != nil {
			return storageError()
		}
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
