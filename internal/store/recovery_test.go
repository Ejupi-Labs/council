package store

import (
	"context"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"testing"
)

func TestRecoveryNeverReplaysAndPreservesTerminals(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	queued, _ := admitted(t, s)
	if e := s.RecoverOwner(ctx, domain.NewID()); e != nil {
		t.Fatal(e)
	}
	a, e := s.Attempt(ctx, queued.Attempt.ID)
	if e != nil || a.Outcome == nil || *a.Outcome != "interrupted" || a.Cleanup.State != "verified" || a.LaunchIntent {
		t.Fatal(a, e)
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.State != "open" {
		t.Fatal(gate, e)
	}
	revision := a.Revision
	if e = s.RecoverOwner(ctx, domain.NewID()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Attempt(ctx, a.ID)
	if e != nil || a.Revision != revision {
		t.Fatal("terminal changed on restart", a, e)
	}
	gate, e = s.Gate(ctx)
	if e != nil {
		t.Fatal(e)
	}
	in := taskInput(queued.Task.ProjectID)
	r := s.CreateTask(ctx, gate.OwnerInstanceID, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	active := r.Envelope.Data.(domain.Admission)
	launchFixture(t, s, active, active.Attempt.OwnerInstanceID)
	if e = s.RecoverOwner(ctx, domain.NewID()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Attempt(ctx, active.Attempt.ID)
	if e != nil || a.Outcome == nil || *a.Outcome != "interrupted" || a.Cleanup.State != "unknown" {
		t.Fatal(a, e)
	}
	gate, e = s.Gate(ctx)
	if e != nil || gate.State != "fenced" || gate.ActiveAttemptID == nil {
		t.Fatal(gate, e)
	}
}
func TestTaskAdmissionKnownFailureAndLostAckHaveOneRecord(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "fault")
	in := taskInput(p.ID)
	owner := domain.NewID()
	s.BeforeCommit = func() error { return errors.New("known rollback") }
	r := s.CreateTask(ctx, owner, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "storage_failed" {
		t.Fatal(r)
	}
	s.BeforeCommit = nil
	var count int
	s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count)
	if count != 0 {
		t.Fatal(count)
	}
	s.AfterCommit = func() error { return errors.New("lost ack") }
	r = s.CreateTask(ctx, owner, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(r)
	}
	s.AfterCommit = nil
	s.SetPolicy(ctx, domain.NewID(), "changed-policy", "1", "supervised")
	r = s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate attempt", count)
	}
	rows, e := s.Tasks(ctx, p.ID, 100, "")
	if e != nil || len(rows.Items) != 1 || rows.Items[0].RequestedMode != "autonomous" {
		t.Fatal(rows, e)
	}
}
