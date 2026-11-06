package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
)

func nextWorkflowCount(value string) (string, error) {
	next, invalid := domain.AddRoutingCounts(value, "1")
	if invalid != nil {
		return "", invalid
	}
	return next, nil
}

func (s *Store) deliveryControls(ctx context.Context, tx *sql.Tx, ownerID string, op domain.DeliveryOperation) (domain.Workflow, domain.WorkflowStep, domain.Attempt, error) {
	w, e := s.workflowByID(ctx, tx, op.WorkflowID)
	if e != nil {
		return w, domain.WorkflowStep{}, domain.Attempt{}, e
	}
	step, e := s.workflowStep(ctx, tx, op.StepID)
	if e != nil {
		return w, step, domain.Attempt{}, e
	}
	a, e := s.attempt(ctx, tx, op.AttemptID)
	if e != nil {
		return w, step, a, e
	}
	g, e := s.gate(ctx, tx)
	if e != nil {
		return w, step, a, e
	}
	if e = checkGate(g); e != nil {
		return w, step, a, e
	}
	t, e := s.task(ctx, tx, w.TaskID)
	if e != nil {
		return w, step, a, e
	}
	if ownerID != a.OwnerInstanceID || g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID || g.ActiveAttemptID == nil || *g.ActiveAttemptID != a.ID || a.WorkflowID != w.ID || a.StepID != step.ID || a.ExecutorKind != "local_application" || a.Stop != nil || a.PrimaryFailure != nil || a.Phase == "settled" || t.CurrentVersion != w.TaskVersion || w.SelectedCandidateID == nil || *w.SelectedCandidateID != op.CandidateBinding.CandidateID || w.Authority.Delivery.LocalMode != "apply_verified_local" || !reflect.DeepEqual(op.Scope, w.Authority.Delivery) {
		return w, step, a, domain.Fail("workflow_revision_conflict", "Current ownership, destination scope and controls do not permit this effect.")
	}
	if e = s.workflowAttemptControls(ctx, tx, a); e != nil {
		return w, step, a, e
	}
	hash, invalid := domain.WorkflowAuthoritySHA256(w.Authority)
	if invalid != nil {
		return w, step, a, invalid
	}
	if op.AuthoritySHA256 != hash {
		return w, step, a, evidenceUnavailable()
	}
	return w, step, a, nil
}
func (s *Store) BeginDelivery(ctx context.Context, ownerID string, in domain.DeliveryOperation) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		if !domain.ValidID(in.ID) || in.Status != "preparing" && in.Status != "ready" || in.LastEffectSequence != "0" || len(in.Conflicts) != 0 || in.ConflictCount != "0" || in.AfterDestination != nil {
			return domain.Invalid("new bounded local delivery operation")
		}
		if _, _, _, e := s.deliveryControls(ctx, tx, ownerID, in); e != nil {
			return e
		}
		if in.PlanArtifactID != "" {
			plan, err := s.artifact(ctx, tx, in.PlanArtifactID)
			if err != nil {
				return err
			}
			if plan.AttemptID != in.AttemptID || plan.Kind != "delivery_plan" || !plan.Closed || plan.Completeness != "complete" || plan.SHA256 == nil {
				return evidenceUnavailable()
			}
		}
		seq, e := nextSequence(ctx, tx, "delivery_operations")
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO delivery_operations VALUES(?,?,?,?,?,?,1,?,NULL,NULL,?)", seq, in.ID, in.WorkflowID, in.StepID, in.AttemptID, in.CandidateBinding.CandidateID, in.Status, encode(in)); e != nil {
			return storageError()
		}
		return nil
	})
}
func (s *Store) ReadyDelivery(ctx context.Context, ownerID string, in domain.DeliveryOperation) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		old, err := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", in.ID)
		if err != nil {
			return err
		}
		if old.Status != "preparing" || old.PlanArtifactID != "" || old.LastEffectSequence != "0" || in.Status != "ready" || !domain.ValidID(in.PlanArtifactID) || in.BeforeDestination == nil || in.AfterDestination != nil || in.LastEffectSequence != "0" || in.WorkflowID != old.WorkflowID || in.StepID != old.StepID || in.AttemptID != old.AttemptID || in.RequestID != old.RequestID || in.CandidateBinding != old.CandidateBinding || in.AuthoritySHA256 != old.AuthoritySHA256 || !reflect.DeepEqual(in.Scope, old.Scope) || !reflect.DeepEqual(in.EvidenceUses, old.EvidenceUses) {
			return domain.Fail("effect_uncertain", "The operation cannot replace or repeat its immutable private preparation.")
		}
		if _, _, _, err = s.deliveryControls(ctx, tx, ownerID, old); err != nil {
			return err
		}
		for _, ref := range []struct{ id, kind string }{{in.PlanArtifactID, "delivery_plan"}, {in.BeforeDestination.ManifestArtifactID, "destination_manifest"}} {
			artifact, err := s.artifact(ctx, tx, ref.id)
			if err != nil {
				return err
			}
			if artifact.AttemptID != in.AttemptID || artifact.Kind != ref.kind || !artifact.Closed || artifact.Completeness != "complete" || artifact.SHA256 == nil {
				return evidenceUnavailable()
			}
			if ref.kind == "destination_manifest" && *artifact.SHA256 != in.BeforeDestination.ManifestSHA256 {
				return evidenceUnavailable()
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE delivery_operations SET revision=revision+1,status='ready',data=? WHERE id=?", encode(in), in.ID); err != nil {
			return storageError()
		}
		return nil
	})
}

func (s *Store) PermitDeliveryEffect(ctx context.Context, f domain.DeliveryPermitFacts) (domain.DeliveryEffect, error) {
	var out domain.DeliveryEffect
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		op, e := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", f.OperationID)
		if e != nil {
			return e
		}
		w, _, _, e := s.deliveryControls(ctx, tx, f.OwnerInstanceID, op)
		if e != nil {
			return e
		}
		t, e := s.task(ctx, tx, w.TaskID)
		if e != nil {
			return e
		}
		if t.Revision != f.TaskRevision || w.Revision != f.WorkflowRevision {
			return domain.Fail("workflow_revision_conflict", "Control changed before the single filesystem-effect permit.")
		}
		var active sql.NullString
		if e = tx.QueryRowContext(ctx, "SELECT active_effect_id FROM delivery_operations WHERE id=?", op.ID).Scan(&active); e != nil {
			return storageError()
		}
		if active.Valid {
			return domain.Fail("attempt_active", "One previously permitted filesystem effect remains unresolved.")
		}
		if op.Status != "ready" && op.Status != "applying" {
			return domain.Fail("effect_uncertain", "The retained operation no longer permits new filesystem actions.")
		}
		effect := f.Effect
		if effect.OperationID != op.ID || !domain.ValidID(effect.EffectID) || effect.State != "intent" || effect.Kind != "add" && effect.Kind != "replace" && effect.Kind != "delete" || effect.Action != "quarantine_before" && effect.Action != "publish_after" || !domain.CanonicalWorkflowPath(effect.RelativePath) {
			return domain.Invalid("one exact filesystem-effect intent")
		}
		if effect.Kind == "add" && effect.Action != "publish_after" || effect.Kind == "delete" && effect.Action != "quarantine_before" {
			return domain.Invalid("matching effect kind and native action")
		}
		if effect.Action == "publish_after" && effect.Kind == "replace" {
			var prior int
			if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM delivery_effects WHERE operation_id=? AND effect_id=? AND action='quarantine_before' AND state='applied'", op.ID, effect.EffectID).Scan(&prior); e != nil {
				return storageError()
			}
			if prior != 1 {
				return domain.Fail("effect_uncertain", "The displaced original is not durably quarantined; do not publish.")
			}
		}
		effect.Sequence, e = nextWorkflowCount(op.LastEffectSequence)
		if e != nil {
			return e
		}
		effect.ObservedAt = domain.Now()
		op.LastEffectSequence = effect.Sequence
		op.Status = "applying"
		if _, e = tx.ExecContext(ctx, "INSERT INTO delivery_effects VALUES(?,?,?,?,?,?,?)", op.ID, intRevision(effect.Sequence), effect.EffectID, effect.Kind, effect.Action, effect.State, encode(effect)); e != nil {
			return storageError()
		}
		if _, e = tx.ExecContext(ctx, "UPDATE delivery_operations SET revision=revision+1,status=?,active_effect_id=?,active_action=?,data=? WHERE id=?", op.Status, effect.EffectID, effect.Action, encode(op), op.ID); e != nil {
			return storageError()
		}
		out = effect
		return nil
	})
	return out, e
}
func (s *Store) RecordDeliveryEffect(ctx context.Context, in domain.DeliveryEffect) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		op, e := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", in.OperationID)
		if e != nil {
			return e
		}
		var active, action sql.NullString
		if e = tx.QueryRowContext(ctx, "SELECT active_effect_id,active_action FROM delivery_operations WHERE id=?", op.ID).Scan(&active, &action); e != nil {
			return storageError()
		}
		if !active.Valid || active.String != in.EffectID || !action.Valid || action.String != in.Action || in.State == "intent" || in.State != "applied" && in.State != "conflict" && in.State != "failed" && in.State != "uncertain" {
			return domain.Fail("effect_uncertain", "No matching unresolved permit can acknowledge this filesystem effect.")
		}
		var raw string
		if e = tx.QueryRowContext(ctx, "SELECT data FROM delivery_effects WHERE operation_id=? AND effect_id=? AND action=? AND state='intent'", op.ID, in.EffectID, in.Action).Scan(&raw); e != nil {
			return storageError()
		}
		intent, e := decode[domain.DeliveryEffect](raw)
		if e != nil {
			return storageError()
		}
		if intent.Kind != in.Kind || intent.RelativePath != in.RelativePath || !reflect.DeepEqual(intent.BeforeEvidence, in.BeforeEvidence) || !reflect.DeepEqual(intent.IntendedAfterEvidence, in.IntendedAfterEvidence) {
			return evidenceUnavailable()
		}
		in.Sequence, e = nextWorkflowCount(op.LastEffectSequence)
		if e != nil {
			return e
		}
		in.ObservedAt = domain.Now()
		op.LastEffectSequence = in.Sequence
		if in.State == "uncertain" {
			incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: ptr(op.AttemptID), Code: "effect_uncertain", Message: "The permitted filesystem effect has an uncertain result; retain quarantine and do not repeat it.", CreatedAt: domain.Now()}
			if e = s.fence(ctx, tx, incident); e != nil {
				return e
			}
			in.IncidentID = ptr(incident.ID)
			op.IncidentID = ptr(incident.ID)
			op.Status = "uncertain"
		} else if in.State == "conflict" {
			op.Status = "conflict"
		} else if in.State == "failed" {
			op.Status = "partial"
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO delivery_effects VALUES(?,?,?,?,?,?,?)", op.ID, intRevision(in.Sequence), in.EffectID, in.Kind, in.Action, in.State, encode(in)); e != nil {
			return storageError()
		}
		if _, e = tx.ExecContext(ctx, "UPDATE delivery_operations SET revision=revision+1,status=?,active_effect_id=NULL,active_action=NULL,data=? WHERE id=?", op.Status, encode(op), op.ID); e != nil {
			return storageError()
		}
		return nil
	})
}
func (s *Store) finishDelivery(ctx context.Context, tx *sql.Tx, w domain.Workflow, step domain.WorkflowStep, a domain.Attempt, in domain.DeliveryOperation, proofs []domain.VerifiedResponse) error {
	old, e := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", in.ID)
	if e != nil {
		return e
	}
	if old.IncidentID != nil {
		if in.IncidentID != nil && *in.IncidentID != *old.IncidentID {
			return evidenceUnavailable()
		}
		incidentID := *old.IncidentID
		in.IncidentID = &incidentID
	}
	if old.PlanArtifactID != in.PlanArtifactID || old.WorkflowID != w.ID || old.StepID != step.ID || old.AttemptID != a.ID || old.CandidateBinding != in.CandidateBinding || old.AuthoritySHA256 != in.AuthoritySHA256 || !reflect.DeepEqual(old.Scope, in.Scope) || !reflect.DeepEqual(old.EvidenceUses, in.EvidenceUses) {
		return evidenceUnavailable()
	}
	var active sql.NullString
	if e = tx.QueryRowContext(ctx, "SELECT active_effect_id FROM delivery_operations WHERE id=?", in.ID).Scan(&active); e != nil {
		return storageError()
	}
	if active.Valid {
		return domain.Fail("effect_uncertain", "An in-flight filesystem effect cannot be declared settled.")
	}
	if old.Status == "uncertain" && in.Status != "uncertain" || old.Status == "conflict" && in.Status == "applied" || old.Status == "partial" && in.Status == "applied" {
		return domain.Fail("effect_uncertain", "A retained nonapplied effect cannot be promoted into applied success.")
	}
	switch in.Status {
	case "applied", "conflict", "partial", "cancelled", "interrupted", "uncertain":
	default:
		return domain.Invalid("settled local application result")
	}
	ids := []string{}
	if in.PlanArtifactID != "" {
		ids = append(ids, in.PlanArtifactID)
	}
	for _, destination := range []*domain.DestinationEvidence{in.BeforeDestination, in.AfterDestination} {
		if destination != nil {
			ids = append(ids, destination.ManifestArtifactID)
		}
	}
	if in.Status == "applied" && (in.BeforeDestination == nil || in.AfterDestination == nil) {
		return evidenceUnavailable()
	}
	if e = s.verifyWorkflowProofs(ctx, tx, proofs, ids...); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE delivery_operations SET revision=revision+1,status=?,data=? WHERE id=?", in.Status, encode(in), in.ID); e != nil {
		return storageError()
	}
	return nil
}
