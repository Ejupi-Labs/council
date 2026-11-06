package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"testing"
)

// Metadata-only owner-loss ordering. Native no-replace moves, physical file
// retention and old-owner process death have separate platform/engine gates.
func TestWorkflowDeliveryRecoveryRetainsUnacknowledgedEffectAndClaim(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	effect, e := f.store.PermitDeliveryEffect(ctx, f.permit(domain.NewID(), "quarantine_before"))
	if e != nil {
		t.Fatal(e)
	}
	if e = f.store.RecoverOwner(ctx, domain.NewID()); e != nil {
		t.Fatal(e)
	}
	operation, e := f.store.DeliveryOperation(ctx, f.operation.ID)
	if e != nil {
		t.Fatal(e)
	}
	if operation.Status != "uncertain" || operation.IncidentID == nil {
		t.Fatal("restart forgot the unacknowledged filesystem effect", operation)
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 1 || rows[0].State != "intent" || !active.Valid || active.String != effect.EffectID {
		t.Fatal("restart replayed/rewrote or discarded the original permit", rows, active)
	}
	gate, e := f.store.Gate(ctx)
	if e != nil || gate.State != "fenced" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != f.operation.AttemptID {
		t.Fatal("restart released uncertain local ownership", gate, e)
	}
	attempt, e := f.store.Attempt(ctx, f.operation.AttemptID)
	if e != nil || attempt.Outcome == nil || *attempt.Outcome != "interrupted" || attempt.LaunchIntent || attempt.Cleanup.Scope != "not_launched" {
		t.Fatal("restart fabricated a process or successful work", attempt, e)
	}
	if _, attempts, steps, _ := workflowCounts(t, f.store); attempts != 1 || steps != 1 {
		t.Fatal("restart admitted historical work", attempts, steps)
	}
}

func TestWorkflowDeliveryUnknownEffectsRetainClaimWithKnownNoProcessCleanup(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	result, err := f.store.SettleAttempt(ctx, f.operation.AttemptID, domain.Settlement{Outcome: "interrupted", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", EffectsKnown: false})
	if err != nil {
		t.Fatal(err)
	}
	gate, err := f.store.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != f.operation.AttemptID {
		t.Fatal("unknown effects lost capacity/fence", gate, err)
	}
	if result.Cleanup.State != "verified" || result.Cleanup.Scope != "not_launched" || result.LaunchIntent {
		t.Fatal("filesystem uncertainty invented process uncertainty", result)
	}
	operation, err := f.store.DeliveryOperation(ctx, f.operation.ID)
	if err != nil || operation.Status != "uncertain" || operation.IncidentID == nil {
		t.Fatal(operation, err)
	}
}

func TestWorkflowDeliveryUnknownEffectsPreservesOperationIncidentReference(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	finished, readErr := f.store.DeliveryOperation(ctx, f.operation.ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	finished.Status = "uncertain"
	finished.NextAction = "inspect_retained_handles"
	now := domain.Now()
	finished.FinishedAt = &now
	if finished.IncidentID != nil {
		t.Fatal("fixture must supply a stale nil incident before settlement")
	}
	facts := domain.WorkflowStepFacts{StepID: f.operation.StepID, OwnerInstanceID: f.owner, TaskRevision: f.taskRevision, WorkflowRevision: f.workflowRevision, CandidateBinding: &f.operation.CandidateBinding, Operation: &finished, Settlement: domain.Settlement{Outcome: "interrupted", NoProcessCreated: true, EffectsKnown: false, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "uncertain"}}
	if _, err := f.store.SettleWorkflowStep(ctx, f.operation.AttemptID, facts); err != nil {
		t.Fatal(err)
	}
	operation, err := f.store.DeliveryOperation(ctx, f.operation.ID)
	if err != nil || operation.Status != "uncertain" || operation.IncidentID == nil {
		t.Fatal("final operation lost the independently recorded effect incident", operation, err)
	}
	incident, err := f.store.Incident(ctx, *operation.IncidentID)
	if err != nil || incident.Code != "effect_uncertain" || incident.AttemptID == nil || *incident.AttemptID != f.operation.AttemptID {
		t.Fatal("operation references the wrong effect incident", incident, err)
	}
	gate, err := f.store.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != f.operation.AttemptID {
		t.Fatal("unknown effects released ownership", gate, err)
	}
}

func TestWorkflowDeliveryRecoveryWithoutPermitDoesNotInventEffectUncertainty(t *testing.T) {
	ctx := context.Background()
	f := deliveryDAOFixture(t)
	deliveryDAOBegin(t, f)
	if e := f.store.RecoverOwner(ctx, domain.NewID()); e != nil {
		t.Fatal(e)
	}
	operation, e := f.store.DeliveryOperation(ctx, f.operation.ID)
	if e != nil || operation.Status != "interrupted" || operation.IncidentID != nil {
		t.Fatal("nonexecuted local operation lost its truthful interrupted state", operation, e)
	}
	rows, active := deliveryDAORows(t, f)
	if len(rows) != 0 || active.Valid {
		t.Fatal("restart invented a native effect", rows, active)
	}
	gate, e := f.store.Gate(ctx)
	if e != nil || gate.State != "open" || gate.ActiveAttemptID != nil {
		t.Fatal("known no-effect restart retained false uncertainty", gate, e)
	}
}
