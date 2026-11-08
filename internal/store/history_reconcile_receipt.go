package store

import (
	"context"
	"database/sql"
	"errors"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// VerifyHistoryReconciliationReceipt resolves only a reconciliation write's
// acknowledgement, never an effect. The owner retains historyMu and control.
// A nil result means the exact receipt is absent and its pre-write operation
// revision is still present. Errors do not establish either outcome.
func (s *Store) VerifyHistoryReconciliationReceipt(ctx context.Context, ownerID, operationID string, in domain.ReconcileHistoryOperationInput) (*domain.Result, error) {
	if !domain.ValidID(operationID) {
		return nil, domain.Invalid("history operation identity")
	}
	if invalid := domain.ValidateReconcileHistoryOperationInput(in); invalid != nil {
		return nil, invalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, storageError()
	}
	defer tx.Rollback()
	if _, err = historyCurrentOwner(ctx, tx, ownerID); err != nil {
		return nil, err
	}
	previous, err := s.replay(ctx, tx, in.RequestID, HistoryReconcileOperation, HistoryReconcileFingerprint(operationID, in))
	if err != nil {
		return nil, err
	}
	rec, err := historyReadOperation(ctx, tx, operationID)
	if err != nil {
		return nil, err
	}
	if _, err = historyCheckJournal(ctx, tx, rec); err != nil {
		return nil, err
	}
	if previous == nil {
		if rec.Operation.Revision != in.ExpectedRevision || rec.Operation.State == "published" || rec.Operation.State == "no_publication" || rec.Operation.State == "failed" {
			return nil, domain.Fail("effect_uncertain", "The original history reconciliation state cannot be proven.")
		}
		return nil, nil
	}
	if previous.Status != 200 || previous.Envelope.Error != nil || previous.Envelope.Receipt == nil || previous.Envelope.Receipt.RequestID != in.RequestID || previous.Envelope.Receipt.Operation != HistoryReconcileOperation {
		return nil, storageError()
	}
	data, err := json.Marshal(previous.Envelope.Data)
	if err != nil {
		return nil, storageError()
	}
	var saved domain.HistoryReconciliationResult
	if json.Unmarshal(data, &saved, json.RejectUnknownMembers(true)) != nil || domain.ValidateHistoryReconciliationResult(saved) != nil || saved.Operation.OperationID != operationID || saved.Operation.RequestID != rec.Operation.RequestID || saved.Operation.PreviewID != rec.Operation.PreviewID || saved.Operation.SnapshotID != rec.Operation.SnapshotID {
		return nil, storageError()
	}
	oldRevision, invalid := domain.ParseRevision(saved.Operation.Revision)
	if invalid != nil || oldRevision > rec.Revision {
		return nil, storageError()
	}
	if saved.Decision == "no_publication" {
		if rec.Operation.State != "no_publication" {
			return nil, storageError()
		}
		var incidentID string
		err = tx.QueryRowContext(ctx, "SELECT incident_id FROM history_incident_bindings WHERE kind='operation' AND operation_id=?", operationID).Scan(&incidentID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, storageError()
		}
		if err == nil {
			var raw string
			if err = tx.QueryRowContext(ctx, "SELECT data FROM recovery_decisions WHERE incident_id=? AND request_id=?", incidentID, in.RequestID).Scan(&raw); err != nil {
				return nil, storageError()
			}
			decision, err := decode[domain.ReconciliationDecision](raw)
			if err != nil || decision.IncidentID != incidentID || decision.RequestID != in.RequestID || decision.Outcome != "reconciled" {
				return nil, storageError()
			}
		}
	}
	return previous, nil
}

// VerifyHistoryIncidentReconciliationReceipt verifies an exact typed-history
// incident receipt and its matching append-only recovery decision. It does not
// clear the gate or resolve any other incident.
func (s *Store) VerifyHistoryIncidentReconciliationReceipt(ctx context.Context, ownerID, incidentID string, in domain.ReconciliationInput) (*domain.Result, error) {
	if !domain.ValidID(incidentID) {
		return nil, domain.Invalid("history incident identity")
	}
	if invalid := domain.ValidateReconciliationInput(in); invalid != nil {
		return nil, invalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, storageError()
	}
	defer tx.Rollback()
	if _, err = historyCurrentOwner(ctx, tx, ownerID); err != nil {
		return nil, err
	}
	_, found, err := historyBinding(ctx, tx, incidentID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.Fail("effect_uncertain", "The exact history incident binding cannot be verified.")
	}
	previous, err := s.replay(ctx, tx, in.RequestID, "incidents.reconcile", RecoveryReconcileFingerprint(incidentID, in))
	if err != nil {
		return nil, err
	}
	view, err := s.recoveryIncidentView(ctx, tx, incidentID)
	if err != nil {
		return nil, err
	}
	if previous == nil {
		if view.Revision != in.ExpectedIncidentRevision {
			return nil, domain.Fail("effect_uncertain", "The original history incident revision cannot be proven.")
		}
		return nil, nil
	}
	if previous.Status != 200 || previous.Envelope.Error != nil || previous.Envelope.Receipt == nil || previous.Envelope.Receipt.RequestID != in.RequestID || previous.Envelope.Receipt.Operation != "incidents.reconcile" {
		return nil, storageError()
	}
	data, err := json.Marshal(previous.Envelope.Data)
	if err != nil {
		return nil, storageError()
	}
	var saved domain.RecoveryReconciliationResult
	if json.Unmarshal(data, &saved, json.RejectUnknownMembers(true)) != nil || saved.Incident.Incident.ID != incidentID || saved.Decision.IncidentID != incidentID || saved.Decision.RequestID != in.RequestID || !domain.ValidID(saved.Decision.ID) {
		return nil, storageError()
	}
	var raw string
	if err = tx.QueryRowContext(ctx, "SELECT data FROM recovery_decisions WHERE id=? AND incident_id=? AND request_id=?", saved.Decision.ID, incidentID, in.RequestID).Scan(&raw); err != nil {
		return nil, storageError()
	}
	decision, err := decode[domain.ReconciliationDecision](raw)
	if err != nil || decision != saved.Decision {
		return nil, storageError()
	}
	savedRevision, invalid := domain.ParseRevision(saved.Incident.Revision)
	if invalid != nil {
		return nil, storageError()
	}
	currentRevision, invalid := domain.ParseRevision(view.Revision)
	if invalid != nil || currentRevision < savedRevision {
		return nil, storageError()
	}
	return previous, nil
}
