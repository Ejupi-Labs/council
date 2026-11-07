package engine

import (
	"context"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func watchFixture(t *testing.T) (*Engine, domain.Admission) {
	t.Helper()
	e, project, _ := testEngine(t)
	e.control.Lock()
	e.fenced = true
	e.control.Unlock()
	r := e.db.CreateTask(context.Background(), e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Stop budget fixture", ExecutionMode: "fake"})
	if r.Envelope.Error != nil {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	return e, a
}
func waitWatch(t *testing.T, e *Engine, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		e.control.Lock()
		_, exists := e.stopWatches[id]
		e.control.Unlock()
		if !exists {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Stop watchdog did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func TestStopBudgetFencesWithoutFinalizingOrReplacingFailure(t *testing.T) {
	e, admission := watchFixture(t)
	ctx := context.Background()
	id := admission.Attempt.ID
	e.control.Lock()
	e.activeWorkers[id] = func() {}
	e.control.Unlock()
	t.Cleanup(func() { e.control.Lock(); delete(e.activeWorkers, id); e.control.Unlock() })
	if _, err := e.db.ClaimAttempt(ctx, id, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	scope := cancellationScope()
	if _, err := e.db.RecordAttempt(ctx, id, domain.AttemptUpdate{Kind: "failure_observed", Failure: primary("native_failed", "The worker failed before Stop."), Cleanup: &domain.Cleanup{State: "active", Scope: scope}}); err != nil {
		t.Fatal(err)
	}
	r := e.db.StopAttempt(ctx, id, domain.NewID())
	if r.Envelope.Error != nil {
		t.Fatal(r)
	}
	before, err := e.db.Attempt(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	e.watchStopWithLimit(id, before.Stop.AcceptedAt, 5*time.Millisecond)
	e.watchStopWithLimit(id, before.Stop.AcceptedAt, 5*time.Millisecond)
	waitWatch(t, e, id)
	after, err := e.db.Attempt(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Phase == "settled" || after.Outcome != nil || after.Cleanup.State != "unknown" || after.Cleanup.Scope != scope || after.Cleanup.IncidentID == nil {
		t.Fatalf("Watchdog fabricated settlement: %+v", after)
	}
	if after.Stop.RequestID != before.Stop.RequestID || after.PrimaryFailure.Code != before.PrimaryFailure.Code || after.PrimaryFailure.EventSequence != before.PrimaryFailure.EventSequence {
		t.Fatal("Watchdog rewrote accepted authority")
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != *after.Cleanup.IncidentID || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != id {
		t.Fatalf("%+v %v", gate, err)
	}
	incident, err := e.db.Incident(ctx, *after.Cleanup.IncidentID)
	if err != nil || incident.Code != "stop_budget_elapsed" {
		t.Fatal(incident, err)
	}
	events, err := e.db.Events(ctx, id, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events.Items {
		if event.Kind == "cleanup_unknown" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("Duplicate watchdog observations", count)
	}
}
func TestStopBudgetDoesNotFenceSettledQueuedCancellation(t *testing.T) {
	e, admission := watchFixture(t)
	ctx := context.Background()
	if r := e.db.StopAttempt(ctx, admission.Attempt.ID, domain.NewID()); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	a, err := e.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || a.Phase != "settled" {
		t.Fatal(a, err)
	}
	e.watchStopWithLimit(a.ID, a.Stop.AcceptedAt, 5*time.Millisecond)
	waitWatch(t, e, a.ID)
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || gate.IncidentID != nil {
		t.Fatal(gate, err)
	}
}
