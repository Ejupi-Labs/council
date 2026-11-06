package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func TestJoinedPersistenceFaultWinsOverExpectedCancellation(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		err                    error
		persistence, uncertain bool
	}{
		{"cancelled", context.Canceled, false, false},
		{"deadline", context.DeadlineExceeded, false, false},
		{"known_precommit", domain.Fail("operation_interrupted", "No commit was attempted."), false, false},
		{"durability", errors.Join(evidence.ErrDurability, context.Canceled), false, true},
		{"corruption", errors.Join(context.Canceled, evidence.ErrCorrupt), false, true},
		{"committed", errors.Join(context.Canceled, domain.Fail("write_uncertain", "Committed acknowledgement lost.")), true, true},
		{"multiple_domain_errors", errors.Join(domain.Fail("operation_interrupted", "Another write was cancelled."), context.Canceled, domain.Fail("write_uncertain", "This write committed.")), true, true},
		{"storage_failed", errors.Join(domain.Fail("storage_failed", "Storage failed."), context.Canceled), true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := persistenceFault(tc.err); got != tc.persistence {
				t.Fatalf("persistenceFault=%v, want %v", got, tc.persistence)
			}
			if got := uncertainEvidence(tc.err); got != tc.uncertain {
				t.Fatalf("uncertainEvidence=%v, want %v", got, tc.uncertain)
			}
		})
	}
}

func TestAdmissionLostAcknowledgementFencesBeforeDispatchAndRetainsReceipt(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	commits := 0
	e.db.AfterCommit = func() error {
		commits++
		if commits == 1 {
			return errors.New("admission committed but acknowledgement was lost")
		}
		return nil
	}
	t.Cleanup(func() { e.db.AfterCommit = nil })
	in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Retain this exact admission without launching it.", ExecutionMode: "fake"}
	r := e.StartTask(ctx, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || gate.ActiveAttemptID != nil {
		t.Fatal(gate, err)
	}
	incident, err := e.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.AttemptID != nil {
		t.Fatal("Request identity was used as an attempt foreign key", incident, err)
	}
	page, err := e.db.Tasks(ctx, project, 100, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	a, err := e.db.Attempt(ctx, *page.Items[0].LatestAttemptID)
	if err != nil || a.Phase != "queued" || a.LaunchIntent || a.SourceSnapshotID != nil {
		t.Fatal("Unacknowledged admission launched", a, err)
	}
	r = e.StartTask(ctx, in)
	if r.Envelope.Error != nil || r.Envelope.Receipt == nil || r.Envelope.Receipt.RequestID != in.RequestID {
		t.Fatal("Same-request reconciliation was blocked", r)
	}
	in.RequestID = domain.NewID()
	if r = e.StartTask(ctx, in); r.Envelope.Error == nil || r.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal("New admission crossed uncertainty", r)
	}
}

func TestAcceptedStopBeforePreparationReadDoesNotCreatePersistenceIncident(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	a := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Cancel before source preparation.", ExecutionMode: "fake"})).Attempt
	var err error
	if a, err = e.db.ClaimAttempt(ctx, a.ID, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	if r := e.Stop(ctx, a.ID, domain.NewID()); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	e.run(cancelled, a)
	a, err = e.db.Attempt(ctx, a.ID)
	if err != nil || a.Outcome == nil || *a.Outcome != "cancelled" || a.LaunchIntent || a.Cleanup.State != "verified" {
		t.Fatal(a, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || gate.IncidentID != nil || gate.ActiveAttemptID != nil || e.fenced {
		t.Fatal("Expected cancellation fenced healthy storage", gate, err)
	}
}

func TestShutdownReplaySignalsOnlyTheExactCurrentOwner(t *testing.T) {
	e, _, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	oldOwner, oldRequest := e.config.OwnerInstanceID, domain.NewID()
	if r := e.db.Shutdown(ctx, oldOwner, oldRequest); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	e.config.OwnerInstanceID = domain.NewID()
	if err := e.db.RecoverOwner(ctx, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	if r := e.Shutdown(ctx, oldOwner, oldRequest); r.Envelope.Error != nil || r.Envelope.Receipt == nil {
		t.Fatal(r)
	}
	select {
	case <-e.ShutdownRequested():
		t.Fatal("Old receipt stopped a replacement owner")
	default:
	}
	currentRequest := domain.NewID()
	if r := e.db.Shutdown(ctx, e.config.OwnerInstanceID, currentRequest); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	if r := e.Shutdown(ctx, e.config.OwnerInstanceID, currentRequest); r.Envelope.Error != nil || r.Envelope.Receipt == nil {
		t.Fatal(r)
	}
	select {
	case <-e.ShutdownRequested():
	default:
		t.Fatal("Current-owner receipt did not resume orderly shutdown")
	}
}

func TestShutdownPersistenceUncertaintyStillStopsExactOwner(t *testing.T) {
	e, _, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	commits := 0
	e.db.AfterCommit = func() error {
		commits++
		if commits == 1 {
			return errors.New("shutdown acknowledgement lost")
		}
		return nil
	}
	t.Cleanup(func() { e.db.AfterCommit = nil })
	r := e.Shutdown(context.Background(), e.config.OwnerInstanceID, domain.NewID())
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" || !e.fenced {
		t.Fatal(r)
	}
	select {
	case <-e.ShutdownRequested():
	default:
		t.Fatal("Authorized owner shutdown waited forever for an uncertain receipt")
	}
}
