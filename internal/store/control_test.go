package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"testing"
)

func admitted(t *testing.T, s *Store) (domain.Admission, string) {
	t.Helper()
	p := openFixture(t, s, domain.NewID())
	owner := domain.NewID()
	r := s.CreateTask(context.Background(), owner, taskInput(p.ID))
	if r.Status != 201 {
		t.Fatal(r)
	}
	return r.Envelope.Data.(domain.Admission), owner
}
func TestStopAndFailureAcceptedOrderAndTerminalImmutability(t *testing.T) {
	for _, failureFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "failure_first", false: "stop_first"}[failureFirst], func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			admission, owner := admitted(t, s)
			launchFixture(t, s, admission, owner)
			id := admission.Attempt.ID
			fault := func() {
				if _, e := s.RecordAttempt(ctx, id, domain.AttemptUpdate{Kind: "primary_failure", Failure: &domain.PrimaryFailure{Code: "native_failed", Message: "The worker reported a failure."}}); e != nil {
					t.Fatal(e)
				}
			}
			if failureFirst {
				fault()
			}
			request := domain.NewID()
			r := s.StopAttempt(ctx, id, request)
			if r.Status != 202 {
				t.Fatal(r)
			}
			if !failureFirst {
				fault()
			}
			a, e := s.SettleAttempt(ctx, id, domain.Settlement{Outcome: "succeeded", Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "incomplete"})
			expected := "cancelled"
			if failureFirst {
				expected = "failed"
			}
			if e != nil || a.Outcome == nil || *a.Outcome != expected {
				t.Fatal(a, e)
			}
			revision := a.Revision
			repeated, e := s.SettleAttempt(ctx, id, domain.Settlement{Outcome: "blocked"})
			if e != nil || *repeated.Outcome != expected || repeated.Revision != revision {
				t.Fatal(repeated, e)
			}
			if replay := s.StopAttempt(ctx, id, request); replay.Status != 202 {
				t.Fatal(replay)
			}
			if noOp := s.StopAttempt(ctx, id, domain.NewID()); noOp.Status != 200 {
				t.Fatal(noOp)
			}
			var count int
			s.db.QueryRow("SELECT count(*) FROM attempt_terminals WHERE attempt_id=?", id).Scan(&count)
			if count != 1 {
				t.Fatal("terminal rewritten", count)
			}
			task, e := s.Task(ctx, admission.Task.ID)
			if e != nil || task.Task.Revision != "2" {
				t.Fatal(task, e)
			}
		})
	}
}
func TestSharedClaimAndCleanupFence(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	first, owner := admitted(t, s)
	p := openFixture(t, s, "second")
	in := taskInput(p.ID)
	r := s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	second := r.Envelope.Data.(domain.Admission)
	if _, e := s.ClaimAttempt(ctx, first.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ClaimAttempt(ctx, second.Attempt.ID, owner); e == nil {
		t.Fatal("second shared slot claim accepted")
	}
	if _, e := s.SettleAttempt(ctx, first.Attempt.ID, domain.Settlement{Outcome: "failed", Cleanup: domain.Cleanup{State: "unknown", Scope: "windows_job"}, EvidenceStatus: "incomplete"}); e != nil {
		t.Fatal(e)
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.State != "fenced" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != first.Attempt.ID {
		t.Fatal(gate, e)
	}
	r = s.CreateTask(ctx, owner, taskInput(p.ID))
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal(r)
	}
}

func TestSweepFindsExpiredAttemptBeyondFirstHundred(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	first, owner := admitted(t, s)
	if _, e := s.ClaimAttempt(ctx, first.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	var last domain.Admission
	for i := 0; i < 101; i++ {
		r := s.CreateTask(ctx, owner, taskInput(first.Task.ProjectID))
		if r.Status != 201 {
			t.Fatal(r)
		}
		last = r.Envelope.Data.(domain.Admission)
	}
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, last.Attempt.ID)
		if e != nil {
			return e
		}
		a.DeadlineAt = "2000-01-01T00:00:00.000Z"
		return saveAttempt(ctx, tx, a)
	})
	if e != nil {
		t.Fatal(e)
	}
	count, e := s.SweepQueued(ctx, owner)
	if e != nil || count != 1 {
		t.Fatal(count, e)
	}
	a, e := s.Attempt(ctx, last.Attempt.ID)
	if e != nil || a.Outcome == nil || *a.Outcome != "failed" || a.PrimaryFailure.Code != "deadline_exceeded" {
		t.Fatal(a, e)
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != first.Attempt.ID {
		t.Fatal(gate, e)
	}
}
