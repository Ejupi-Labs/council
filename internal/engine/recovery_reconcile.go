package engine

import (
	"context"
	"errors"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// ReconcileIncident replays a known receipt before touching retained evidence.
// The controller, not the request, obtains every physical verification fact.
func (e *Engine) ReconcileIncident(ctx context.Context, id string, in domain.ReconciliationInput) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	if invalid := domain.ValidateReconciliationInput(in); invalid != nil {
		return e.failure(in.RequestID, invalid)
	}
	if !domain.ValidID(id) {
		return e.failure(in.RequestID, domain.Invalid("incident identity"))
	}
	if previous, err := e.db.Replay(ctx, in.RequestID, "incidents.reconcile", store.RecoveryReconcileFingerprint(id, in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if previous != nil {
		return *previous
	}

	// History receipt incidents use their own typed binding and the same
	// synchronous publisher barrier. Non-history incidents keep the original
	// recovery path and never inherit history authority.
	e.historyMu.Lock()
	e.control.Lock()
	historyResult, handled := e.db.TryReconcileHistoryIncident(ctx, e.config.OwnerInstanceID, id, in, !e.fenceUncertain)
	if handled {
		if historyResult.Envelope.Error != nil && persistenceFault(historyResult.Envelope.Error) {
			verifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			saved, verifyErr := e.db.VerifyHistoryIncidentReconciliationReceipt(verifyCtx, e.config.OwnerInstanceID, id, in)
			if verifyErr != nil {
				e.preserveUnrecordedHistoryFenceLocked()
			} else if saved != nil {
				historyResult = *saved
			} else if gate, gateErr := e.db.Gate(verifyCtx); gateErr != nil || gate.State != "fenced" {
				// A missing receipt is safe only while the original incident
				// is still durably fenced and eligible for an exact retry.
				e.preserveUnrecordedHistoryFenceLocked()
			} else {
				e.fenced = true
			}
			cancel()
		}
		if historyResult.Envelope.Error == nil && !e.fenceUncertain {
			if gate, gateErr := e.db.Gate(context.Background()); gateErr == nil && gate.State == "open" {
				e.fenced = false
			}
		}
	}
	e.control.Unlock()
	e.historyMu.Unlock()
	if handled {
		return historyResult
	}

	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	view, err := e.db.RecoveryIncident(ctx, id)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if view.Revision != in.ExpectedIncidentRevision {
		return e.failure(in.RequestID, domain.RevisionConflict(view.Revision))
	}
	var fresh *domain.RecoveryFreshFacts
	if view.Incident.AttemptID != nil {
		inventory, inventoryErr := e.db.RecoveryEvidenceInventory(ctx, *view.Incident.AttemptID)
		if inventoryErr != nil && ctx.Err() != nil {
			return e.failure(in.RequestID, ctx.Err())
		}
		if persistenceFault(inventoryErr) {
			e.fence(*view.Incident.AttemptID, "recovery_storage_unavailable", "Recovery evidence could not be read durably. Preserve the existing fence.")
			return e.failure(in.RequestID, inventoryErr)
		}
		if inventoryErr == nil {
			candidate := &domain.RecoveryFreshFacts{AttemptID: inventory.AttemptID, AttemptRevision: inventory.AttemptRevision, CleanupEventSequence: inventory.CleanupEventSequence, TerminalSHA256: inventory.TerminalSHA256, VerifiedArtifacts: make([]domain.VerifiedResponse, 0, len(inventory.Artifacts))}
			verified := true
			for _, artifact := range inventory.Artifacts {
				m, verifyErr := e.evidence.VerifyClosed(ctx, artifact.ID)
				if ctx.Err() != nil {
					return e.failure(in.RequestID, ctx.Err())
				}
				if persistenceFault(verifyErr) || errors.Is(verifyErr, evidence.ErrDurability) {
					e.fence(*view.Incident.AttemptID, "recovery_evidence_uncertain", "Recovery evidence durability could not be verified. Preserve the existing fence.")
					return e.failure(in.RequestID, verifyErr)
				}
				if verifyErr != nil || m.SHA256 == nil {
					verified = false
					break
				}
				candidate.VerifiedArtifacts = append(candidate.VerifiedArtifacts, domain.VerifiedResponse{ArtifactID: m.ID, SHA256: *m.SHA256, PhysicalIdentity: m.Identity, ByteLength: m.ByteLength})
			}
			if verified {
				sequence, binding, standaloneErr := e.recoveryStandaloneFresh(ctx, inventory.AttemptID)
				if ctx.Err() != nil {
					return e.failure(in.RequestID, ctx.Err())
				}
				if persistenceFault(standaloneErr) || errors.Is(standaloneErr, evidence.ErrDurability) {
					e.fence(inventory.AttemptID, "recovery_evidence_uncertain", "Standalone recovery closure could not be verified durably. Preserve the existing fence.")
					return e.failure(in.RequestID, standaloneErr)
				}
				if standaloneErr == nil {
					candidate.StandaloneSealSequence, candidate.StandaloneBinding = sequence, binding
				}
				fresh = candidate
			}
		}
	}
	e.control.Lock()
	if e.fenceUncertain {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "A fence cause could not be recorded durably. Preserve the fence and inspect retained evidence."))
	}
	result := e.db.ReconcileIncident(ctx, e.config.OwnerInstanceID, id, in, fresh)
	e.guardMutationLocked(result)
	if result.Envelope.Error == nil && !e.fenceUncertain {
		gate, gateErr := e.db.Gate(ctx)
		if gateErr == nil && gate.State == "open" {
			e.fenced = false
		}
	}
	e.control.Unlock()
	return result
}
