package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

func TestRecoveryReconcileUnknownProofReplayAndOtherIncident(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, _ := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	firstID, secondID := domain.NewID(), domain.NewID()
	first := domain.ExecutionIncident{ID: firstID, AttemptID: &admission.Attempt.ID, Code: "first_uncertain", Message: "Retain exact ownership.", CreatedAt: domain.Now()}
	second := domain.ExecutionIncident{ID: secondID, AttemptID: &admission.Attempt.ID, Code: "second_uncertain", Message: "Retain independent blocker.", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.Fence(ctx, second); err != nil {
		t.Fatal(err)
	}
	before, err := s.Gate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.RecoveryIncident(ctx, firstID)
	if err != nil || view.Revision != "1" || view.State != "unresolved" {
		t.Fatal(view, err)
	}
	input := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "1", Action: "verify_and_reconcile"}
	firstResult := s.ReconcileIncident(ctx, owner, firstID, input, nil)
	if firstResult.Status != 200 || firstResult.Envelope.Receipt == nil {
		t.Fatal(firstResult)
	}
	result, err := decode[domain.RecoveryReconciliationResult](encode(firstResult.Envelope.Data))
	if err != nil || result.Incident.State != "unresolved" || result.Incident.Revision != "2" || result.Decision.Outcome != "unresolved" {
		t.Fatal(result, err)
	}
	after, err := s.Gate(ctx)
	if err != nil || after.State != "fenced" || !reflect.DeepEqual(before.ActiveClaims, after.ActiveClaims) || after.IncidentID == nil || *after.IncidentID != secondID {
		t.Fatal("missing proof released ownership or another incident", before, after, err)
	}
	replay := s.ReconcileIncident(ctx, domain.NewID(), firstID, input, nil)
	replayed, err := decode[domain.RecoveryReconciliationResult](encode(replay.Envelope.Data))
	if err != nil || replay.Status != firstResult.Status || !reflect.DeepEqual(result, replayed) || !reflect.DeepEqual(firstResult.Envelope.Receipt, replay.Envelope.Receipt) {
		t.Fatal("exact retry repeated or reevaluated operation", firstResult, replay, err)
	}
	stale := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "1", Action: "verify_and_reconcile"}
	if r := s.ReconcileIncident(ctx, owner, firstID, stale, nil); r.Envelope.Error == nil {
		t.Fatal("stale incident revision accepted", r)
	}
	changed := input
	changed.ExpectedIncidentRevision = "2"
	if r := s.ReconcileIncident(ctx, owner, firstID, changed, nil); r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal("changed request reused", r)
	}
	var decisions, observations int
	if err = s.db.QueryRow("SELECT count(*) FROM recovery_decisions WHERE incident_id=?", firstID).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM recovery_observations WHERE incident_id=?", firstID).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 || observations != 1 {
		t.Fatal("replay duplicated audit records", decisions, observations)
	}
}

func TestRecoveryCleanupRejectsUnverifiedProcess(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, _ := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	before, err := s.Gate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordRecoveryCleanup(ctx, owner, admission.Attempt.ID, process.CleanupResult{Verified: false}); err == nil {
		t.Fatal("unverified process was accepted")
	}
	after, err := s.Gate(ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("bad proof changed ownership", before, after, err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='recovery_cleanup_proof'", admission.Attempt.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid cleanup persisted proof", count, err)
	}
}

// A watchdog's cleanup proof must never resolve a different same-attempt
// storage or context incident, even when presented with controller facts.
func TestRecoveryReconcileRejectsUnrelatedIncidentKinds(t *testing.T) {
	s := testStore(t)
	attemptID := domain.NewID()
	facts := &domain.RecoveryFreshFacts{AttemptID: attemptID}
	gate := domain.ExecutionGate{State: "fenced"}
	for _, code := range []string{"storage_failed", "graph_context_close_uncertain", "effect_uncertain", "cleanup_proof_unavailable"} {
		incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: &attemptID, Code: code, Message: "unrelated failure", CreatedAt: domain.Now()}
		safe, slot, err := s.recoveryReconciliationSafe(context.Background(), nil, gate, incident, facts)
		if err != nil || safe || slot != "" {
			t.Fatalf("watchdog proof cleared unrelated incident %q: safe=%t slot=%q err=%v", code, safe, slot, err)
		}
	}
}

// This metadata-level fixture tests gate accounting and receipts; physical
// process proof is exercised separately by the real Engine watchdog journey.
func TestRecoveryReconcileExplicitRecheckAfterClaimDrains(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	id := domain.NewID()
	incident := domain.ExecutionIncident{ID: id, AttemptID: &admission.Attempt.ID, Code: "settlement_budget_elapsed", Message: "retained", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: id, RequestID: domain.NewID(), ObservationID: domain.NewID(), Outcome: "reconciled", CreatedAt: domain.Now()}
	if _, err := s.db.Exec("INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, id, decision.RequestID, encode(decision)); err != nil {
		t.Fatal(err)
	}
	first := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "2", Action: "verify_and_reconcile"}
	blocked := s.ReconcileIncident(ctx, owner, id, first, nil)
	value, err := decode[domain.RecoveryReconciliationResult](encode(blocked.Envelope.Data))
	if err != nil || blocked.Envelope.Error != nil || value.Decision.Outcome != "rechecked" || value.Incident.State != "reconciled" || value.Incident.Revision != "3" || len(value.Incident.Blockers) == 0 {
		t.Fatal("held claim recheck was not retained as a blocker", blocked, value, err)
	}
	if _, err = s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal(err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || len(gate.ActiveClaims) != 0 {
		t.Fatal("settlement reopened the gate without explicit owner recheck", gate, err)
	}
	stale := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "2", Action: "verify_and_reconcile"}
	if r := s.ReconcileIncident(ctx, owner, id, stale, nil); r.Envelope.Error == nil || r.Envelope.Error.Code != "revision_conflict" {
		t.Fatal("stale recheck accepted", r)
	}
	next := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "3", Action: "verify_and_reconcile"}
	opened := s.ReconcileIncident(ctx, owner, id, next, nil)
	current, err := decode[domain.RecoveryReconciliationResult](encode(opened.Envelope.Data))
	if err != nil || opened.Envelope.Error != nil || current.Decision.Outcome != "rechecked" || current.Incident.Revision != "4" || len(current.Incident.Blockers) != 0 {
		t.Fatal("safe explicit recheck did not clear gate", opened, current, err)
	}
	gate, err = s.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal(gate, err)
	}
	replay := s.ReconcileIncident(ctx, domain.NewID(), id, next, nil)
	replayValue, err := decode[domain.RecoveryReconciliationResult](encode(replay.Envelope.Data))
	if err != nil || !reflect.DeepEqual(current, replayValue) || !reflect.DeepEqual(opened.Envelope.Receipt, replay.Envelope.Receipt) {
		t.Fatal("recheck replay changed the receipt", opened, replay, err)
	}
}

func TestRecoveryReconcileOwnerRestartCannotAutoReopen(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	id := domain.NewID()
	incident := domain.ExecutionIncident{ID: id, AttemptID: &admission.Attempt.ID, Code: "settlement_budget_elapsed", Message: "retained", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	decision := domain.ReconciliationDecision{ID: domain.NewID(), IncidentID: id, RequestID: domain.NewID(), ObservationID: domain.NewID(), Outcome: "reconciled", CreatedAt: domain.Now()}
	if _, err := s.db.Exec("INSERT INTO recovery_decisions(id,incident_id,request_id,data) VALUES(?,?,?,?)", decision.ID, id, decision.RequestID, encode(decision)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal(err)
	}
	nextOwner := domain.NewID()
	if err := s.RecoverOwner(ctx, nextOwner); err != nil {
		t.Fatal(err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.OwnerInstanceID != nextOwner {
		t.Fatal("owner restart silently reopened gate", gate, err)
	}
	in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "2", Action: "verify_and_reconcile"}
	result := s.ReconcileIncident(ctx, nextOwner, id, in, nil)
	if result.Envelope.Error == nil || result.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal("new owner used old-owner proof to clear fence", result)
	}
	gate, err = s.Gate(ctx)
	if err != nil || gate.State != "fenced" {
		t.Fatal("old-owner recheck opened gate", gate, err)
	}
}
