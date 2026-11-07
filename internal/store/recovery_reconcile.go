package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// recoveryCleanupProof is written only by the current process-owning controller,
// from the actual Owned.Cleanup result before its native handle is closed.
// It is attempt-bound because the corresponding incident may not yet exist.
type recoveryCleanupProof struct {
	AttemptID               string `json:"attemptId"`
	OriginalOwnerInstanceID string `json:"originalOwnerInstanceId"`
	ClaimSlot               string `json:"claimSlot"`
	Scope                   string `json:"scope"`
}

// RecordRecoveryCleanup is an internal controller method, never a public input
// decoder. Cleanup must be the actual result from this attempt's held owner.
func (s *Store) RecordRecoveryCleanup(ctx context.Context, ownerID, attemptID string, cleanup process.CleanupResult) (string, error) {
	if !domain.ValidID(ownerID) || !domain.ValidID(attemptID) {
		return "", domain.Invalid("owned cleanup identity")
	}
	if !cleanup.Verified || cleanup.Root == nil || !cleanup.Root.Known || !cleanup.StdoutDrained || !cleanup.StderrDrained || cleanup.StdoutError || cleanup.StderrError || (cleanup.Scope != "managed_process_group_empty" && cleanup.Scope != "managed_windows_job_empty") {
		return "", domain.Fail("cleanup_unknown", "Owned process cleanup or retained streams could not be verified.")
	}
	var sequence string
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return err
		}
		if gate.OwnerInstanceID != ownerID {
			return domain.Fail("owner_unavailable", "The current owner cannot attest this cleanup.")
		}
		a, err := s.attempt(ctx, tx, attemptID)
		if err != nil {
			return err
		}
		if a.OwnerInstanceID != ownerID || !a.LaunchIntent || attemptExecutor(a) != "native" {
			return domain.Fail("dispatch_fenced", "The exact native attempt owner is unavailable.")
		}
		claims, err := s.graphClaimRecords(ctx, tx)
		if err != nil {
			return err
		}
		slot := ""
		for _, c := range claims {
			if c.View.AttemptID == attemptID && c.OwnerInstanceID == ownerID {
				slot = c.View.Slot
			}
		}
		if slot == "" {
			return domain.Fail("dispatch_fenced", "The exact owned claim is unavailable.")
		}
		proof := recoveryCleanupProof{AttemptID: attemptID, OriginalOwnerInstanceID: ownerID, ClaimSlot: slot, Scope: cleanup.Scope}
		sequence, err = s.appendEvent(ctx, tx, attemptID, "recovery_cleanup_proof", jsontext.Value(encode(proof)))
		return err
	})
	return sequence, err
}

// RecoveryEvidenceInventory obtains the exact current bounded owner-only
// metadata inventory. Callers must independently VerifyClosed each artifact.
func (s *Store) RecoveryEvidenceInventory(ctx context.Context, attemptID string) (domain.RecoveryEvidenceInventory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoveryEvidenceInventory(ctx, s.db, attemptID)
}

func (s *Store) recoveryEvidenceInventory(ctx context.Context, q queryer, attemptID string) (domain.RecoveryEvidenceInventory, error) {
	var out domain.RecoveryEvidenceInventory
	if !domain.ValidID(attemptID) {
		return out, domain.Invalid("attempt identity")
	}
	a, err := s.attempt(ctx, q, attemptID)
	if err != nil {
		return out, err
	}
	if a.Phase != "settled" {
		return out, domain.Fail("effect_uncertain", "The attempt has not settled.")
	}
	claims, err := s.graphClaimRecords(ctx, q)
	if err != nil {
		return out, err
	}
	slot := ""
	for _, claim := range claims {
		if claim.View.AttemptID == a.ID && claim.OwnerInstanceID == a.OwnerInstanceID {
			slot = claim.View.Slot
		}
	}
	if slot == "" {
		return out, domain.Fail("effect_uncertain", "The exact retained claim is unavailable.")
	}
	events, err := q.QueryContext(ctx, "SELECT sequence FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='recovery_cleanup_proof' ORDER BY sequence LIMIT 2", a.ID)
	if err != nil {
		return out, storageError()
	}
	var sequences []string
	for events.Next() {
		var seq int64
		if err = events.Scan(&seq); err != nil {
			events.Close()
			return out, storageError()
		}
		sequences = append(sequences, domain.Decimal(seq))
	}
	err = events.Err()
	events.Close()
	if err != nil {
		return out, storageError()
	}
	if len(sequences) != 1 {
		return out, domain.Fail("effect_uncertain", "A unique exact cleanup observation is unavailable.")
	}
	valid, err := recoveryCleanupEvent(ctx, q, a.ID, sequences[0], a.OwnerInstanceID, slot)
	if err != nil {
		return out, err
	}
	if !valid {
		return out, domain.Fail("effect_uncertain", "The cleanup observation does not match the retained claim.")
	}
	var terminal string
	if err = q.QueryRowContext(ctx, "SELECT data FROM attempt_terminals WHERE attempt_id=?", attemptID).Scan(&terminal); err != nil {
		return out, storageError()
	}
	sum := sha256.Sum256([]byte(terminal))
	out = domain.RecoveryEvidenceInventory{AttemptID: attemptID, AttemptRevision: a.Revision, CleanupEventSequence: sequences[0], TerminalSHA256: hex.EncodeToString(sum[:]), Artifacts: []domain.ArtifactRecord{}}
	rows, err := q.QueryContext(ctx, "SELECT id FROM artifacts WHERE attempt_id=? ORDER BY sequence LIMIT 4097", attemptID)
	if err != nil {
		return out, storageError()
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, storageError()
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(ids) > 4096 {
		return out, domain.Fail("evidence_unavailable", "The complete recovery inventory is unavailable or exceeds its bound.")
	}
	for _, id := range ids {
		artifact, e := s.artifact(ctx, q, id)
		if e != nil {
			return out, e
		}
		if artifact.AttemptID != attemptID || !artifact.Closed || artifact.Completeness != "complete" || artifact.SHA256 == nil {
			return out, domain.Fail("evidence_unavailable", "A retained artifact is not closed and complete.")
		}
		out.Artifacts = append(out.Artifacts, artifact)
	}
	return out, nil
}

func recoveryCleanupEvent(ctx context.Context, q queryer, attemptID, sequence, ownerID, slot string) (bool, error) {
	n, e := domain.ParseRevision(sequence)
	if e != nil {
		return false, nil
	}
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT data FROM attempt_events WHERE sequence=? AND attempt_id=?", n, attemptID).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, storageError()
	}
	var event domain.AttemptEvent
	if err := json.Unmarshal([]byte(raw), &event, json.RejectUnknownMembers(true)); err != nil {
		return false, storageError()
	}
	if event.Sequence != sequence || event.AttemptID != attemptID || event.Kind != "recovery_cleanup_proof" {
		return false, nil
	}
	var proof recoveryCleanupProof
	if err := json.Unmarshal(event.Metadata, &proof, json.RejectUnknownMembers(true)); err != nil {
		return false, storageError()
	}
	return proof.AttemptID == attemptID && proof.OriginalOwnerInstanceID == ownerID && proof.ClaimSlot == slot && (proof.Scope == "managed_process_group_empty" || proof.Scope == "managed_windows_job_empty"), nil
}

func (s *Store) recoveryIncidentView(ctx context.Context, q queryer, id string) (domain.RecoveryIncidentView, error) {
	var out domain.RecoveryIncidentView
	if !domain.ValidID(id) {
		return out, domain.Invalid("incident identity")
	}
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT data FROM execution_incidents WHERE id=?", id).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return out, notFound()
	} else if err != nil {
		return out, storageError()
	}
	incident, err := decode[domain.ExecutionIncident](raw)
	if err != nil || incident.ID != id {
		return out, storageError()
	}
	rows, err := q.QueryContext(ctx, "SELECT data FROM recovery_decisions WHERE incident_id=? ORDER BY rowid LIMIT 10001", id)
	if err != nil {
		return out, storageError()
	}
	count := int64(0)
	state := "unresolved"
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			rows.Close()
			return out, storageError()
		}
		decision, e := decode[domain.ReconciliationDecision](data)
		if e != nil || decision.IncidentID != id || (state != "reconciled" && decision.Outcome != "unresolved" && decision.Outcome != "reconciled") || (state == "reconciled" && decision.Outcome != "rechecked") {
			rows.Close()
			return out, storageError()
		}
		if decision.Outcome != "rechecked" {
			state = decision.Outcome
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil || count > 10000 {
		return out, storageError()
	}
	out = domain.RecoveryIncidentView{Incident: incident, Revision: domain.Decimal(count + 1), State: state, Blockers: []string{}, NextAction: "Inspect retained ownership and evidence, then request verification."}
	if state == "unresolved" {
		out.Blockers = append(out.Blockers, "Exact cleanup, durable effects and complete retained evidence must be verified.")
	} else {
		out.NextAction = "Review remaining controls before explicitly starting unfinished work."
		var gateState string
		if err := q.QueryRowContext(ctx, "SELECT state FROM execution_control WHERE singleton=1").Scan(&gateState); err != nil {
			return out, storageError()
		}
		if gateState == "fenced" {
			out.Blockers = append(out.Blockers, "Shared dispatch remains fenced; inspect remaining claims, incidents and effect evidence.")
			out.NextAction = "Resolve remaining blockers, then explicitly recheck this incident if it owns the gate."
		}
		if incident.AttemptID != nil {
			var held int
			if err := q.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims WHERE attempt_id=?", *incident.AttemptID).Scan(&held); err != nil {
				return out, storageError()
			}
			if held != 0 {
				out.Blockers = append(out.Blockers, "The attempt claim remains held by another unresolved incident or control.")
			}
		}
	}
	return out, nil
}

// RecoveryIncident returns immutable incident history with derived revision.
func (s *Store) RecoveryIncident(ctx context.Context, id string) (domain.RecoveryIncidentView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoveryIncidentView(ctx, s.db, id)
}

// RecoveryReconcileFingerprint lets the owner replay a saved receipt before
// observing physical state or accessing retained evidence.
func RecoveryReconcileFingerprint(incidentID string, in domain.ReconciliationInput) string {
	return Fingerprint("incidents.reconcile", struct {
		IncidentID string
		Input      domain.ReconciliationInput
	}{incidentID, in})
}

// ReconcileIncident only consumes controller-supplied facts; they are never
// part of a client request, and no work is dispatched by this operation.
func (s *Store) ReconcileIncident(ctx context.Context, ownerID, incidentID string, in domain.ReconciliationInput, fresh *domain.RecoveryFreshFacts) domain.Result {
	if invalid := domain.ValidateReconciliationInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !domain.ValidID(ownerID) || !domain.ValidID(incidentID) {
		return domain.Failure(in.RequestID, domain.Invalid("reconciliation owner and incident"))
	}
	fp := RecoveryReconcileFingerprint(incidentID, in)
	return s.mutate(ctx, in.RequestID, "incidents.reconcile", fp, func(tx *sql.Tx) (int, any, error) {
		view, err := s.recoveryIncidentView(ctx, tx, incidentID)
		if err != nil {
			return 0, nil, err
		}
		if view.Revision != in.ExpectedIncidentRevision {
			return 0, nil, domain.RevisionConflict(view.Revision)
		}
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		if gate.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The current owner must inspect this incident.")
		}
		if view.State == "reconciled" {
			return s.recheckRecoveryFence(ctx, tx, ownerID, gate, view, in)
		}
		safe, slot, err := s.recoveryReconciliationSafe(ctx, tx, gate, view.Incident, fresh)
		if err != nil {
			return 0, nil, err
		}
		outcome := "unresolved"
		reason := "Complete current ownership, effect and retained evidence proof is unavailable."
		if safe {
			// Multiple independent incidents may own the same attempt claim.
			// This incident can be verified, but the claim remains until the
			// final same-attempt incident has independently been reconciled.
			var otherUnresolved int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents i WHERE i.attempt_id=? AND i.id!=? AND NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')", *view.Incident.AttemptID, incidentID).Scan(&otherUnresolved); err != nil {
				return 0, nil, storageError()
			}
			if otherUnresolved == 0 {
				attempt, e := s.attempt(ctx, tx, *view.Incident.AttemptID)
				if e != nil {
					return 0, nil, e
				}
				result, e := tx.ExecContext(ctx, "DELETE FROM capacity_claims WHERE slot=? AND attempt_id=? AND owner_instance_id=?", intRevision(slot), attempt.ID, attempt.OwnerInstanceID)
				if e != nil {
					return 0, nil, storageError()
				}
				n, e := result.RowsAffected()
				if e != nil || n != 1 {
					return 0, nil, storageError()
				}
			}
			outcome = "reconciled"
			reason = "Exact retained ownership, effects and evidence were verified."
		}
		observation := domain.ReconciliationObservation{ID: domain.NewID(), IncidentID: incidentID, OwnerInstanceID: ownerID, Outcome: outcome, Reason: reason, CreatedAt: domain.Now()}
		if view.Incident.AttemptID != nil {
			observation.AttemptID = *view.Incident.AttemptID
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_observations(id,incident_id,data) VALUES(?,?,?)", observation.ID, incidentID, encode(observation)); err != nil {
			return 0, nil, storageError()
		}
		decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: incidentID, RequestID: in.RequestID, ObservationID: observation.ID, Outcome: outcome, CreatedAt: domain.Now()}
		if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, incidentID, in.RequestID, encode(decision)); err != nil {
			return 0, nil, storageError()
		}
		if safe {
			var unresolved, claims int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM execution_incidents i WHERE NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled')").Scan(&unresolved); err != nil {
				return 0, nil, storageError()
			}
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&claims); err != nil {
				return 0, nil, storageError()
			}
			// A fence with unknown provenance is never cleared just because counts are zero.
			if gate.State == "fenced" && gate.IncidentID != nil && *gate.IncidentID == incidentID {
				if unresolved == 0 && claims == 0 {
					if err = s.recoveryReleaseResolvedFence(ctx, tx); err != nil {
						return 0, nil, err
					}
				} else if unresolved > 0 {
					var nextID string
					if err = tx.QueryRowContext(ctx, "SELECT i.id FROM execution_incidents i WHERE NOT EXISTS (SELECT 1 FROM recovery_decisions d WHERE d.incident_id=i.id AND json_extract(d.data,'$.outcome')='reconciled') ORDER BY i.rowid LIMIT 1").Scan(&nextID); err != nil {
						return 0, nil, storageError()
					}
					if _, err = tx.ExecContext(ctx, "UPDATE execution_control SET incident_id=? WHERE singleton=1 AND state='fenced' AND incident_id=?", nextID, incidentID); err != nil {
						return 0, nil, storageError()
					}
				}
			}
		}
		// A checkpoint blocked by this exact retained claim must see the
		// reconciled decision and final claim disposition in the same commit.
		if safe && view.Incident.AttemptID != nil {
			if err = s.refreshRecoveryCheckpointsForAttempt(ctx, tx, *view.Incident.AttemptID); err != nil {
				return 0, nil, err
			}
		}
		view, err = s.recoveryIncidentView(ctx, tx, incidentID)
		if err != nil {
			return 0, nil, err
		}
		return 200, domain.RecoveryReconciliationResult{Incident: view, Decision: decision}, nil
	})
}

// recheckRecoveryFence is entered only after receipt replay, revision and
// current-owner checks. It never replays process, delivery or admission work.
func (s *Store) recheckRecoveryFence(ctx context.Context, tx *sql.Tx, ownerID string, gate domain.ExecutionGate, view domain.RecoveryIncidentView, in domain.ReconciliationInput) (int, any, error) {
	if view.Incident.AttemptID == nil {
		return 0, nil, domain.Fail("dispatch_fenced", "The original attempt ownership cannot be established.")
	}
	attempt, err := s.attempt(ctx, tx, *view.Incident.AttemptID)
	if err != nil {
		return 0, nil, err
	}
	if attempt.OwnerInstanceID != ownerID {
		clean, err := recoveryCleanOwnerChain(ctx, tx, attempt.OwnerInstanceID, ownerID)
		if err != nil {
			return 0, nil, err
		}
		if !clean {
			return 0, nil, domain.Fail("dispatch_fenced", "The prior owner chain does not establish a clean handoff; retain the fence.")
		}
	}
	if gate.State == "fenced" && gate.IncidentID != nil && *gate.IncidentID == view.Incident.ID {
		if err = s.recoveryReleaseResolvedFence(ctx, tx); err != nil {
			return 0, nil, err
		}
	}
	observation := domain.ReconciliationObservation{ID: domain.NewID(), IncidentID: view.Incident.ID, OwnerInstanceID: ownerID, AttemptID: attempt.ID, Outcome: "rechecked", Reason: "Rechecked remaining shared fences and claims without starting work.", CreatedAt: domain.Now()}
	if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_observations(id,incident_id,data) VALUES(?,?,?)", observation.ID, view.Incident.ID, encode(observation)); err != nil {
		return 0, nil, storageError()
	}
	decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: view.Incident.ID, RequestID: in.RequestID, ObservationID: observation.ID, Outcome: "rechecked", CreatedAt: domain.Now()}
	if _, err = tx.ExecContext(ctx, "INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, view.Incident.ID, in.RequestID, encode(decision)); err != nil {
		return 0, nil, storageError()
	}
	view, err = s.recoveryIncidentView(ctx, tx, view.Incident.ID)
	if err != nil {
		return 0, nil, err
	}
	return 200, domain.RecoveryReconciliationResult{Incident: view, Decision: decision}, nil
}

func recoveryProofFatal(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var failure *domain.Error
	return errors.As(err, &failure) && (failure.Code == "storage_failed" || failure.Code == "write_uncertain" || failure.Code == "operation_interrupted")
}

// The only positive scope currently supported is a native graph producer with
// a closed, identity-bound graph output journal and complete fresh inventory.
// Other ownership/effect scopes remain unresolved until independently supported.
func (s *Store) recoveryReconciliationSafe(ctx context.Context, tx *sql.Tx, gate domain.ExecutionGate, incident domain.ExecutionIncident, fresh *domain.RecoveryFreshFacts) (bool, string, error) {
	if fresh == nil || incident.AttemptID == nil || *incident.AttemptID != fresh.AttemptID || gate.State != "fenced" {
		return false, "", nil
	}
	// Cleanup and closed graph-output proof resolve only watchdog expiry.
	// Storage, context, publication or other incidents require their own
	// proof contracts and must not be cleared by unrelated observations.
	if incident.Code != "stop_budget_elapsed" && incident.Code != "settlement_budget_elapsed" {
		return false, "", nil
	}
	a, err := s.attempt(ctx, tx, fresh.AttemptID)
	if err != nil {
		return false, "", err
	}
	if a.OwnerInstanceID != gate.OwnerInstanceID {
		clean, err := recoveryCleanOwnerChain(ctx, tx, a.OwnerInstanceID, gate.OwnerInstanceID)
		if err != nil {
			return false, "", err
		}
		if !clean {
			return false, "", nil
		}
	}
	if a.Revision != fresh.AttemptRevision || a.Phase != "settled" || a.EvidenceStatus != "complete" || attemptExecutor(a) != "native" || !graphOutputRole(a) {
		return false, "", nil
	}
	claims, err := s.graphClaimRecords(ctx, tx)
	if err != nil {
		return false, "", err
	}
	slot := ""
	for _, c := range claims {
		if c.View.AttemptID == a.ID && c.OwnerInstanceID == a.OwnerInstanceID {
			slot = c.View.Slot
		}
	}
	if slot == "" {
		return false, "", nil
	}
	valid, err := recoveryCleanupEvent(ctx, tx, a.ID, fresh.CleanupEventSequence, a.OwnerInstanceID, slot)
	if err != nil {
		return false, "", err
	}
	if !valid {
		return false, "", nil
	}
	inventory, err := s.recoveryEvidenceInventory(ctx, tx, a.ID)
	if err != nil {
		if recoveryProofFatal(err) {
			return false, "", err
		}
		return false, "", nil
	}
	if inventory.AttemptRevision != fresh.AttemptRevision || inventory.TerminalSHA256 != fresh.TerminalSHA256 || inventory.CleanupEventSequence != fresh.CleanupEventSequence || len(inventory.Artifacts) != len(fresh.VerifiedArtifacts) {
		return false, "", nil
	}
	for i, artifact := range inventory.Artifacts {
		proof := fresh.VerifiedArtifacts[i]
		if proof.ArtifactID != artifact.ID || artifact.SHA256 == nil || proof.SHA256 != *artifact.SHA256 || proof.ByteLength != artifact.ByteLength || proof.PhysicalIdentity != artifact.PhysicalIdentity {
			return false, "", nil
		}
	}
	var raw string
	if err = tx.QueryRowContext(ctx, "SELECT data FROM attempt_terminals WHERE attempt_id=?", a.ID).Scan(&raw); err != nil {
		return false, "", storageError()
	}
	var terminal struct {
		Attempt    domain.Attempt    `json:"attempt"`
		Settlement domain.Settlement `json:"settlement"`
	}
	if err = json.Unmarshal([]byte(raw), &terminal, json.RejectUnknownMembers(true)); err != nil {
		return false, "", storageError()
	}
	if terminal.Attempt.ID != a.ID || terminal.Attempt.Revision != a.Revision || terminal.Settlement.EvidenceStatus != "complete" {
		return false, "", nil
	}
	member, err := s.graphMembership(ctx, tx, a.TaskID)
	if err != nil {
		return false, "", err
	}
	if member == nil {
		// Standalone native authors use a controller-sealed, freshly
		// reverified complete source/candidate closure. The old standalone
		// terminal's EffectsKnown bit was never a closure witness.
		if a.Role != "author" || fresh.StandaloneBinding == nil || fresh.StandaloneSealSequence == "" {
			return false, "", nil
		}
		sequence, seal, sealErr := s.recoveryStandaloneSeal(ctx, tx, a.ID)
		if sealErr != nil {
			if recoveryProofFatal(sealErr) {
				return false, "", sealErr
			}
			return false, "", nil
		}
		if sequence != fresh.StandaloneSealSequence || seal.AttemptID != a.ID || seal.OwnerInstanceID != a.OwnerInstanceID || seal.WorkflowID != a.WorkflowID || seal.StepID != a.StepID || !reflect.DeepEqual(seal.Binding, *fresh.StandaloneBinding) {
			return false, "", nil
		}
		if err = s.recoveryStandaloneBinding(ctx, tx, a, seal.Binding, fresh.VerifiedArtifacts); err != nil {
			if recoveryProofFatal(err) {
				return false, "", err
			}
			return false, "", nil
		}
		var effects int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM delivery_operations WHERE attempt_id=? AND (active_effect_id IS NOT NULL OR status='uncertain')", a.ID).Scan(&effects); err != nil {
			return false, "", storageError()
		}
		if effects != 0 {
			return false, "", nil
		}
		return true, slot, nil
	}
	if member.Kind != "node" || !terminal.Settlement.EffectsKnown {
		return false, "", nil
	}
	record, err := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if err != nil {
		return false, "", err
	}
	obs := record.Public.OutputObservation
	if obs == nil || obs.ProducerAttemptID != a.ID || obs.Candidate == nil && obs.Response == nil {
		return false, "", nil
	}
	if obs.Candidate != nil {
		if obs.Candidate.ClosureRef == nil || record.CandidateClosure == nil {
			return false, "", nil
		}
		if err = s.graphOutputClosure(ctx, tx, a, record.Public.Dispatch, "candidate", "complete", obs.Candidate.ClosureRef, record.CandidateClosure, fresh.VerifiedArtifacts); err != nil {
			if recoveryProofFatal(err) {
				return false, "", err
			}
			return false, "", nil
		}
	}
	if obs.Response != nil {
		if obs.Response.ClosureRef == nil && record.ResponseClosure == nil {
			// A file-producing author can retain a raw response as explicitly
			// ineligible context. It is still in the complete verified inventory;
			// it must not be promoted to a response dependency or final result.
			if obs.Candidate == nil || a.ResponseArtifactID == nil || obs.Response.ResponseArtifactID != *a.ResponseArtifactID || obs.Response.State != "incomplete" {
				return false, "", nil
			}
			foundResponse := false
			for _, artifact := range inventory.Artifacts {
				if artifact.ID == *a.ResponseArtifactID && artifact.Kind == "final_response" {
					foundResponse = true
				}
			}
			if !foundResponse {
				return false, "", nil
			}
		} else {
			if obs.Response.ClosureRef == nil || record.ResponseClosure == nil {
				return false, "", nil
			}
			if err = s.graphOutputClosure(ctx, tx, a, record.Public.Dispatch, "response", "complete", obs.Response.ClosureRef, record.ResponseClosure, fresh.VerifiedArtifacts); err != nil {
				if recoveryProofFatal(err) {
					return false, "", err
				}
				return false, "", nil
			}
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM delivery_operations WHERE attempt_id=? AND (active_effect_id IS NOT NULL OR status='uncertain')", a.ID).Scan(&count); err != nil {
		return false, "", storageError()
	}
	if count != 0 {
		return false, "", nil
	}
	return true, slot, nil
}
