package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A Stop ordered after blocked settlement still controls its pending automatic
// transition. Advance only the current task control revision, never its immutable
// terminal/proof, and never suppress a newer successor by naming an old attempt.
func (s *Store) suppressFreshContinuation(ctx context.Context, tx *sql.Tx, a domain.Attempt) (bool, error) {
	var raw string
	err := tx.QueryRowContext(ctx, "SELECT data FROM routing_continuation_proofs WHERE attempt_id=?", a.ID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, storageError()
	}
	proof, err := decode[domain.ContinuationProof](raw)
	if err != nil {
		return false, storageError()
	}
	g, err := s.gate(ctx, tx)
	if err != nil {
		return false, err
	}
	t, err := s.task(ctx, tx, a.TaskID)
	if err != nil {
		return false, err
	}
	if !proof.Eligible || proof.AttemptID != a.ID || proof.TaskID != a.TaskID || proof.OwnerInstanceID != g.OwnerInstanceID || proof.TaskRevision != t.Revision || proof.TaskVersion != t.CurrentVersion || t.LatestAttemptID == nil || *t.LatestAttemptID != a.ID {
		return false, nil
	}
	t.Revision, err = increment(t.Revision)
	if err != nil {
		return false, err
	}
	t.StateRevision, err = increment(t.StateRevision)
	if err != nil {
		return false, err
	}
	t.UpdatedAt = domain.Now()
	if err = saveTask(ctx, tx, t); err != nil {
		return false, err
	}
	return true, nil
}
