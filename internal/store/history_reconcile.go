package store

import (
	"context"
	"database/sql"
	"errors"

	json "encoding/json/v2"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type historyIncidentBinding struct {
	IncidentID, Kind, OperationID, RequestID, Fingerprint, OwnerID string
	Generation                                                     int64
}

func historyBinding(ctx context.Context, q queryer, incidentID string) (historyIncidentBinding, bool, error) {
	var b historyIncidentBinding
	var operation sql.NullString
	err := q.QueryRowContext(ctx, "SELECT incident_id,kind,operation_id,request_id,fingerprint,owner_instance_id,owner_generation FROM history_incident_bindings WHERE incident_id=?", incidentID).Scan(&b.IncidentID, &b.Kind, &operation, &b.RequestID, &b.Fingerprint, &b.OwnerID, &b.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return b, false, nil
	}
	if err != nil {
		return b, false, storageError()
	}
	if operation.Valid {
		b.OperationID = operation.String
	}
	if b.IncidentID != incidentID || !domain.ValidID(b.RequestID) || !domain.ValidSHA256(b.Fingerprint) || !domain.ValidID(b.OwnerID) || b.Generation < 1 || (b.Kind == "operation") != domain.ValidID(b.OperationID) {
		return historyIncidentBinding{}, false, storageError()
	}
	return b, true, nil
}

func historyMutationKind(operation string) string {
	switch operation {
	case historyCaptureOperation:
		return "capture"
	case HistoryPreviewOperation:
		return "preview"
	case HistoryCreateOperation:
		return "create"
	default:
		return ""
	}
}
func historyKindOperation(kind string) string {
	switch kind {
	case "capture":
		return historyCaptureOperation
	case "preview":
		return HistoryPreviewOperation
	case "create", "operation":
		return HistoryCreateOperation
	default:
		return ""
	}
}

func (s *Store) fenceHistoryBinding(ctx context.Context, tx *sql.Tx, b historyIncidentBinding) error {
	incident := domain.ExecutionIncident{ID: b.IncidentID, Code: "history_persistence_uncertain", Message: "A history persistence acknowledgement is uncertain. Reconcile its exact retained history intent.", CreatedAt: domain.Now()}
	if err := s.fence(ctx, tx, incident); err != nil {
		return err
	}
	var operation any
	if b.OperationID != "" {
		operation = b.OperationID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO history_incident_bindings(incident_id,kind,operation_id,request_id,fingerprint,owner_instance_id,owner_generation) VALUES(?,?,?,?,?,?,?)`, b.IncidentID, b.Kind, operation, b.RequestID, b.Fingerprint, b.OwnerID, b.Generation)
	if err != nil {
		return storageError()
	}
	return nil
}

// FenceHistoryOperation atomically identifies the exact operation responsible
// for a history-only persistence incident and closes the durable global gate.
// The owner must set its in-memory fence before calling and retain a dirty epoch
// if this transaction or its acknowledgement is uncertain.
func (s *Store) FenceHistoryOperation(ctx context.Context, ownerID, operationID string) (string, error) {
	if !domain.ValidID(operationID) {
		return "", domain.Invalid("history operation identity")
	}
	id := domain.NewID()
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		rec, err := historyReadOperation(ctx, tx, operationID)
		if err != nil {
			return err
		}
		if rec.Owner != ownerID || rec.Generation != epoch.Generation {
			return domain.Fail("owner_unavailable", "Only the originating owner can bind this history incident.")
		}
		var operation, fingerprint string
		if err = tx.QueryRowContext(ctx, "SELECT operation,fingerprint FROM receipts WHERE request_id=?", rec.Operation.RequestID).Scan(&operation, &fingerprint); err != nil || operation != HistoryCreateOperation || !domain.ValidSHA256(fingerprint) {
			return storageError()
		}
		return s.fenceHistoryBinding(ctx, tx, historyIncidentBinding{IncidentID: id, Kind: "operation", OperationID: operationID, RequestID: rec.Operation.RequestID, Fingerprint: fingerprint, OwnerID: ownerID, Generation: epoch.Generation})
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// FenceHistoryMutation binds a failure before an output operation is known.
// The caller must be the current owner, with no history publisher admitted.
func (s *Store) FenceHistoryMutation(ctx context.Context, ownerID, operation, requestID, fingerprint string) (string, error) {
	kind := historyMutationKind(operation)
	if kind == "" || !domain.ValidID(requestID) || !domain.ValidSHA256(fingerprint) {
		return "", domain.Invalid("history request identity")
	}
	id := domain.NewID()
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		return s.fenceHistoryBinding(ctx, tx, historyIncidentBinding{IncidentID: id, Kind: kind, RequestID: requestID, Fingerprint: fingerprint, OwnerID: ownerID, Generation: epoch.Generation})
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func historyIncidentOwnerProven(ctx context.Context, q queryer, b historyIncidentBinding, ownerID string) (bool, error) {
	epoch, err := historyCurrentOwner(ctx, q, ownerID)
	if err != nil {
		return false, err
	}
	var begin int
	if err = q.QueryRowContext(ctx, "SELECT count(*) FROM recovery_owner_epoch_events WHERE generation=? AND owner_instance_id=? AND kind='begin'", b.Generation, b.OwnerID).Scan(&begin); err != nil {
		return false, storageError()
	}
	if begin != 1 {
		return false, nil
	}
	if b.OwnerID == ownerID {
		return b.Generation == epoch.Generation, nil
	}
	if b.Generation >= epoch.Generation {
		return false, nil
	}
	return recoveryCleanOwnerChain(ctx, q, b.OwnerID, ownerID)
}

func (s *Store) historyResolveIncident(ctx context.Context, tx *sql.Tx, ownerID, incidentID, requestID string) error {
	view, err := s.recoveryIncidentView(ctx, tx, incidentID)
	if err != nil {
		return err
	}
	if view.State != "unresolved" {
		return domain.Fail("revision_conflict", "The history incident has already been reconciled.")
	}
	observation := domain.ReconciliationObservation{ID: domain.NewID(), IncidentID: incidentID, OwnerInstanceID: ownerID, Outcome: "reconciled", Reason: "The exact history intent was verified without replay or a new effect.", CreatedAt: domain.Now()}
	decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: incidentID, RequestID: requestID, ObservationID: observation.ID, Outcome: "reconciled", CreatedAt: domain.Now()}
	if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_observations(id,incident_id,data) VALUES(?,?,?)", observation.ID, incidentID, encode(observation)); err != nil {
		return storageError()
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, incidentID, requestID, encode(decision)); err != nil {
		return storageError()
	}
	var state string
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT state,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &current); err != nil {
		return storageError()
	}
	if state == "fenced" && current.Valid && current.String == incidentID {
		var next string
		err = tx.QueryRowContext(ctx, `SELECT i.id FROM execution_incidents i WHERE NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled') ORDER BY i.rowid LIMIT 1`).Scan(&next)
		if err == nil {
			if _, err = tx.ExecContext(ctx, "UPDATE execution_control SET incident_id=? WHERE singleton=1 AND state='fenced' AND incident_id=?", next, incidentID); err != nil {
				return storageError()
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storageError()
		}
		return s.recoveryReleaseResolvedFence(ctx, tx)
	}
	return nil
}

// historyReceiptNoEffect examines only the exact Council receipt and, for a
// committed create receipt, its immutable operation and complete journal.
// The caller holds the owner's history actor barrier through this transaction.
func historyReceiptNoEffect(ctx context.Context, tx *sql.Tx, b historyIncidentBinding) (bool, error) {
	operation := historyKindOperation(b.Kind)
	if operation == "" || b.Kind == "operation" {
		return false, nil
	}
	var savedOperation, fingerprint, raw string
	err := tx.QueryRowContext(ctx, "SELECT operation,fingerprint,envelope FROM receipts WHERE request_id=?", b.RequestID).Scan(&savedOperation, &fingerprint, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		if b.Kind == "create" {
			var count int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM history_export_operations WHERE request_id=?", b.RequestID).Scan(&count); err != nil {
				return false, storageError()
			}
			return count == 0, nil
		}
		return true, nil
	}
	if err != nil {
		return false, storageError()
	}
	if savedOperation != operation || fingerprint != b.Fingerprint {
		return false, nil
	}
	if b.Kind != "create" {
		return true, nil
	}
	if len(raw) > 32768 {
		return false, storageError()
	}
	var envelope domain.Envelope
	if json.Unmarshal([]byte(raw), &envelope) != nil || envelope.Error != nil {
		return false, storageError()
	}
	data, err := json.Marshal(envelope.Data)
	if err != nil {
		return false, storageError()
	}
	var saved struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	if json.Unmarshal(data, &saved) != nil || !domain.ValidID(saved.Operation.OperationID) {
		return false, storageError()
	}
	rec, err := historyReadOperation(ctx, tx, saved.Operation.OperationID)
	if err != nil {
		return false, err
	}
	if rec.Operation.RequestID != b.RequestID || rec.Owner != b.OwnerID || rec.Generation != b.Generation {
		return false, nil
	}
	permits, err := historyCheckJournal(ctx, tx, rec)
	if err != nil {
		return false, err
	}
	if permits != 0 || (rec.Operation.State != "prepared" && rec.Operation.State != "no_publication") {
		return false, nil
	}
	if rec.Operation.State == "prepared" && !rec.Frozen {
		rec.Frozen = true
		rec.Operation.Blockers = []string{"request_interrupted"}
		rec.Operation.NextAction = "reconcile"
		if err = historySaveOperation(ctx, tx, &rec); err != nil {
			return false, err
		}
	}
	return true, nil
}

// TryReconcileHistoryIncident handles only typed, owner-created history incident
// bindings. The caller holds the history barrier and e.control, and must pass
// actorsDrained=false if any actor/handle or in-memory fence is unproven.
func (s *Store) TryReconcileHistoryIncident(ctx context.Context, ownerID, incidentID string, in domain.ReconciliationInput, actorsDrained bool) (domain.Result, bool) {
	if invalid := domain.ValidateReconciliationInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid), true
	}
	if !domain.ValidID(ownerID) || !domain.ValidID(incidentID) {
		return domain.Failure(in.RequestID, domain.Invalid("history incident identity")), true
	}
	s.mu.Lock()
	_, found, err := historyBinding(ctx, s.db, incidentID)
	s.mu.Unlock()
	if err != nil {
		return domain.Failure(in.RequestID, domain.SafeError(err)), true
	}
	if !found {
		return domain.Result{}, false
	}
	result := s.mutate(ctx, in.RequestID, "incidents.reconcile", RecoveryReconcileFingerprint(incidentID, in), func(tx *sql.Tx) (int, any, error) {
		b, found, err := historyBinding(ctx, tx, incidentID)
		if err != nil || !found {
			return 0, nil, storageError()
		}
		view, err := s.recoveryIncidentView(ctx, tx, incidentID)
		if err != nil {
			return 0, nil, err
		}
		if view.Revision != in.ExpectedIncidentRevision {
			return 0, nil, domain.RevisionConflict(view.Revision)
		}
		proven, err := historyIncidentOwnerProven(ctx, tx, b, ownerID)
		if err != nil {
			return 0, nil, err
		}
		if !actorsDrained || !proven {
			return 0, nil, domain.Fail("dispatch_fenced", "History actor drainage or the owner chain cannot be proven.")
		}
		if b.Kind == "operation" {
			if view.State != "reconciled" {
				return 0, nil, domain.Fail("dispatch_fenced", "Reconcile the exact history operation before rechecking its incident.")
			}
			rec, err := historyReadOperation(ctx, tx, b.OperationID)
			if err != nil {
				return 0, nil, err
			}
			permits, err := historyCheckJournal(ctx, tx, rec)
			if err != nil {
				return 0, nil, err
			}
			if rec.Operation.State != "no_publication" || permits != 0 || rec.Operation.RequestID != b.RequestID {
				return 0, nil, domain.Fail("dispatch_fenced", "The exact no-publication proof is unavailable.")
			}
		} else {
			safe, err := historyReceiptNoEffect(ctx, tx, b)
			if err != nil {
				return 0, nil, err
			}
			if !safe {
				return 0, nil, domain.Fail("dispatch_fenced", "The exact history receipt or effect state remains uncertain.")
			}
		}
		if view.State == "reconciled" {
			// The original exact decision is immutable. Recheck the remaining
			// shared gate without changing any independent incident or claim.
			if err = s.recoveryReleaseResolvedFence(ctx, tx); err != nil {
				return 0, nil, err
			}
			obs := domain.ReconciliationObservation{ID: domain.NewID(), IncidentID: incidentID, OwnerInstanceID: ownerID, Outcome: "rechecked", Reason: "Rechecked the exact history incident and remaining shared fences.", CreatedAt: domain.Now()}
			decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: incidentID, RequestID: in.RequestID, ObservationID: obs.ID, Outcome: "rechecked", CreatedAt: domain.Now()}
			if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_observations(id,incident_id,data) VALUES(?,?,?)", obs.ID, incidentID, encode(obs)); err != nil {
				return 0, nil, storageError()
			}
			if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, incidentID, in.RequestID, encode(decision)); err != nil {
				return 0, nil, storageError()
			}
			view, err = s.recoveryIncidentView(ctx, tx, incidentID)
			if err != nil {
				return 0, nil, err
			}
			return 200, domain.RecoveryReconciliationResult{Incident: view, Decision: decision}, nil
		}
		if err = s.historyResolveIncident(ctx, tx, ownerID, incidentID, in.RequestID); err != nil {
			return 0, nil, err
		}
		var raw string
		if err = tx.QueryRowContext(ctx, "SELECT data FROM recovery_decisions WHERE incident_id=? AND request_id=?", incidentID, in.RequestID).Scan(&raw); err != nil {
			return 0, nil, storageError()
		}
		decision, err := decode[domain.ReconciliationDecision](raw)
		if err != nil {
			return 0, nil, storageError()
		}
		view, err = s.recoveryIncidentView(ctx, tx, incidentID)
		if err != nil {
			return 0, nil, err
		}
		return 200, domain.RecoveryReconciliationResult{Incident: view, Decision: decision}, nil
	})
	return result, true
}

// ReconcileHistoryExport must be called while the owner retains its
// per-operation admission barrier and has established that every publisher and
// effect-capable handle is drained. actorsDrained is an internal owner fact,
// never input accepted from an API client. It also must be false if an
// unrecorded in-memory fence prevents proof.
func (s *Store) ReconcileHistoryExport(ctx context.Context, ownerID, operationID string, in domain.ReconcileHistoryOperationInput, actorsDrained bool) domain.Result {
	if err := domain.ValidateReconcileHistoryOperationInput(in); err != nil {
		return domain.Failure(in.RequestID, err)
	}
	if !domain.ValidID(operationID) {
		return domain.Failure(in.RequestID, domain.Invalid("history operation identity"))
	}
	return s.mutate(ctx, in.RequestID, HistoryReconcileOperation, HistoryReconcileFingerprint(operationID, in), func(tx *sql.Tx) (int, any, error) {
		epoch, err := historyCurrentOwner(ctx, tx, ownerID)
		if err != nil {
			return 0, nil, err
		}
		rec, err := historyReadOperation(ctx, tx, operationID)
		if err != nil {
			return 0, nil, err
		}
		if rec.Operation.Revision != in.ExpectedRevision {
			return 0, nil, domain.Fail("revision_conflict", "The history operation changed. Inspect its current revision before reconciling.")
		}
		if rec.Operation.State == "published" || rec.Operation.State == "no_publication" || rec.Operation.State == "failed" {
			return 0, nil, domain.Fail("revision_conflict", "The history operation is already terminal. Inspect its recorded outcome.")
		}
		// The freeze and the proof/decision commit together while the owner holds
		// the admission barrier. This prevents any future publication permit.
		rec.Frozen = true
		codes := []string{}
		permits, journalErr := historyCheckJournal(ctx, tx, rec)
		if journalErr != nil {
			codes = append(codes, "journal_unproven")
		}
		if journalErr == nil && permits != 0 {
			codes = append(codes, "permit_present_or_uncertain")
		}
		if !actorsDrained {
			codes = append(codes, "actor_not_drained")
		}
		if epoch.OwnerID != rec.Owner || epoch.Generation != rec.Generation {
			clean, chainErr := recoveryCleanOwnerChain(ctx, tx, rec.Owner, ownerID)
			var originalGeneration int64
			if chainErr != nil || tx.QueryRowContext(ctx, "SELECT generation FROM recovery_owner_epoch_events WHERE owner_instance_id=? AND kind='begin'", rec.Owner).Scan(&originalGeneration) != nil || originalGeneration != rec.Generation || !clean {
				codes = append(codes, "owner_history_unproven")
			}
		}
		decision := "unresolved"
		if len(codes) == 0 {
			decision = "no_publication"
			codes = []string{"no_permit_proven"}
			outcome := "no_publication"
			rec.Operation.State = outcome
			rec.Operation.Outcome = &outcome
			rec.Operation.Durability = nil
			rec.Operation.Blockers = []string{}
			rec.Operation.NextAction = "none"
		} else {
			// Preserve the actual effect state; no assertion about target bytes.
			rec.Operation.Blockers = []string{}
			for _, code := range codes {
				switch code {
				case "journal_unproven":
					rec.Operation.Blockers = append(rec.Operation.Blockers, "journal_unproven")
				case "permit_present_or_uncertain":
					rec.Operation.Blockers = append(rec.Operation.Blockers, "history_effect_uncertain")
				case "actor_not_drained":
					rec.Operation.Blockers = append(rec.Operation.Blockers, "actor_not_drained")
				case "owner_history_unproven":
					rec.Operation.Blockers = append(rec.Operation.Blockers, "owner_epoch_uncertain")
				}
			}
			rec.Operation.NextAction = "reconcile"
		}
		if journalErr == nil {
			entry := struct {
				Decision   string   `json:"decision"`
				Codes      []string `json:"codes"`
				Owner      string   `json:"owner"`
				Generation int64    `json:"generation"`
				At         string   `json:"at"`
			}{decision, codes, ownerID, epoch.Generation, domain.Now()}
			if err = historyAppendJournal(ctx, tx, &rec, domain.NewID(), "decision", entry); err != nil {
				return 0, nil, err
			}
		}
		if err = historySaveOperation(ctx, tx, &rec); err != nil {
			return 0, nil, err
		}
		if decision == "no_publication" {
			var linkedID string
			err = tx.QueryRowContext(ctx, "SELECT incident_id FROM history_incident_bindings WHERE kind='operation' AND operation_id=?", operationID).Scan(&linkedID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return 0, nil, storageError()
			}
			if err == nil {
				binding, found, bindErr := historyBinding(ctx, tx, linkedID)
				if bindErr != nil || !found || binding.OperationID != operationID || binding.RequestID != rec.Operation.RequestID {
					return 0, nil, storageError()
				}
				proven, proofErr := historyIncidentOwnerProven(ctx, tx, binding, ownerID)
				if proofErr != nil {
					return 0, nil, proofErr
				}
				if !proven {
					return 0, nil, domain.Fail("dispatch_fenced", "The history incident owner chain is not proven.")
				}
				var op, fp string
				if err = tx.QueryRowContext(ctx, "SELECT operation,fingerprint FROM receipts WHERE request_id=?", binding.RequestID).Scan(&op, &fp); err != nil || op != HistoryCreateOperation || fp != binding.Fingerprint {
					return 0, nil, storageError()
				}
				if err = s.historyResolveIncident(ctx, tx, ownerID, linkedID, in.RequestID); err != nil {
					return 0, nil, err
				}
			}
		}
		result := domain.HistoryReconciliationResult{Operation: rec.Operation, Decision: decision, ObservationCodes: codes}
		if domain.ValidateHistoryReconciliationResult(result) != nil {
			return 0, nil, storageError()
		}
		return 200, result, nil
	})
}
