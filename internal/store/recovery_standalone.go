package store

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// RecoveryStandaloneSeal is a controller-authored witness to the already
// verified complete standalone author workspace closure. It is not an API input.
type RecoveryStandaloneSeal struct {
	AttemptID       string                  `json:"attemptId"`
	OwnerInstanceID string                  `json:"ownerInstanceId"`
	WorkflowID      string                  `json:"workflowId"`
	StepID          string                  `json:"stepId"`
	Binding         domain.CandidateBinding `json:"binding"`
}

// RecordRecoveryStandaloneSeal is called only after the native owner has
// verified the full source/candidate manifests and all retained members. The
// transaction rechecks exact registered anchors, owner, claim and cleanup.
func (s *Store) RecordRecoveryStandaloneSeal(ctx context.Context, ownerID, attemptID string, binding domain.CandidateBinding, proofs []domain.VerifiedResponse) error {
	if !domain.ValidID(ownerID) || !domain.ValidID(attemptID) {
		return domain.Invalid("standalone recovery owner and attempt")
	}
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var incidents int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents WHERE attempt_id=? AND json_extract(data,'$.code') IN ('stop_budget_elapsed','settlement_budget_elapsed')", attemptID).Scan(&incidents); err != nil {
			return storageError()
		}
		if incidents == 0 {
			return nil
		}
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return err
		}
		if gate.OwnerInstanceID != ownerID || gate.State != "fenced" {
			return domain.Fail("dispatch_fenced", "The exact owner and watchdog fence are required.")
		}
		a, err := s.attempt(ctx, tx, attemptID)
		if err != nil {
			return err
		}
		if a.OwnerInstanceID != ownerID || a.Phase == "settled" || a.Role != "author" || attemptExecutor(a) != "native" || a.WorkflowID == "" || a.StepID == "" || a.CandidateID == nil || *a.CandidateID != binding.CandidateID {
			return domain.Fail("evidence_unavailable", "The standalone author subject is unavailable.")
		}
		member, err := s.graphMembership(ctx, tx, a.TaskID)
		if err != nil {
			return err
		}
		if member != nil {
			return domain.Invalid("standalone author scope")
		}
		if err = s.recoveryStandaloneBinding(ctx, tx, a, binding, proofs); err != nil {
			return err
		}
		var cleanup int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='recovery_cleanup_proof'", attemptID).Scan(&cleanup); err != nil {
			return storageError()
		}
		if cleanup != 1 {
			return domain.Fail("evidence_unavailable", "The exact retained process cleanup observation is unavailable.")
		}
		var prior int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='recovery_standalone_seal'", attemptID).Scan(&prior); err != nil {
			return storageError()
		}
		if prior != 0 {
			return domain.Fail("acceptance_conflict", "The standalone recovery seal was already recorded.")
		}
		seal := RecoveryStandaloneSeal{AttemptID: attemptID, OwnerInstanceID: ownerID, WorkflowID: a.WorkflowID, StepID: a.StepID, Binding: binding}
		_, err = s.appendEvent(ctx, tx, attemptID, "recovery_standalone_seal", jsontext.Value(encode(seal)))
		return err
	})
}

func (s *Store) recoveryStandaloneBinding(ctx context.Context, q queryer, a domain.Attempt, binding domain.CandidateBinding, proofs []domain.VerifiedResponse) error {
	view, err := s.selectedWorkflow(ctx, q, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil {
		return err
	}
	if view.Workflow == nil {
		return evidenceUnavailable()
	}
	w := *view.Workflow
	if w.ID != a.WorkflowID || w.TaskID != a.TaskID || w.TaskVersion != a.TaskVersion || a.CandidateID == nil || *a.CandidateID != binding.CandidateID {
		return evidenceUnavailable()
	}
	candidate, err := s.candidate(ctx, q, binding.CandidateID)
	if err != nil {
		return err
	}
	if candidate.AttemptID != a.ID || candidate.WorkflowID != w.ID || candidate.CreatedByStepID != a.StepID || candidate.SourceSnapshotID != binding.SourceSnapshotID || candidate.RootSourceSnapshotID != binding.RootSourceSnapshotID || candidate.RootSourceSnapshotID != candidate.SourceSnapshotID {
		return evidenceUnavailable()
	}
	source, err := s.sourceSnapshot(ctx, q, candidate.SourceSnapshotID)
	if err != nil {
		return err
	}
	if source.AttemptID != a.ID {
		return evidenceUnavailable()
	}
	return s.candidateBinding(ctx, q, w, binding, proofs)
}

// RecoveryStandaloneSealForAttempt reads the sole immutable sealed event.
func (s *Store) RecoveryStandaloneSealForAttempt(ctx context.Context, attemptID string) (string, RecoveryStandaloneSeal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoveryStandaloneSeal(ctx, s.db, attemptID)
}

func (s *Store) recoveryStandaloneSeal(ctx context.Context, q queryer, attemptID string) (string, RecoveryStandaloneSeal, error) {
	var out RecoveryStandaloneSeal
	rows, err := q.QueryContext(ctx, "SELECT sequence,data FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='recovery_standalone_seal' ORDER BY sequence LIMIT 2", attemptID)
	if err != nil {
		return "", out, storageError()
	}
	var seq int64
	var raw string
	count := 0
	for rows.Next() {
		if err = rows.Scan(&seq, &raw); err != nil {
			rows.Close()
			return "", out, storageError()
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", out, storageError()
	}
	if count != 1 {
		return "", out, domain.Fail("evidence_unavailable", "A unique standalone recovery seal is unavailable.")
	}
	var event domain.AttemptEvent
	if err = json.Unmarshal([]byte(raw), &event, json.RejectUnknownMembers(true)); err != nil {
		return "", out, storageError()
	}
	if event.AttemptID != attemptID || event.Sequence != domain.Decimal(seq) || event.Kind != "recovery_standalone_seal" {
		return "", out, storageError()
	}
	if err = json.Unmarshal(event.Metadata, &out, json.RejectUnknownMembers(true)); err != nil {
		return "", out, storageError()
	}
	if out.AttemptID != attemptID || !domain.ValidID(out.OwnerInstanceID) || !domain.ValidID(out.WorkflowID) || !domain.ValidID(out.StepID) {
		return "", out, storageError()
	}
	return event.Sequence, out, nil
}
