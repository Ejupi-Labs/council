package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Stop the scheduler before installing a store hook. No test races an unsynchronized
// hook assignment with a writer; restarting below gives the actor exclusive reads.
func stopFixtureScheduler(t *testing.T, e *Engine) {
	t.Helper()
	e.cancel()
	select {
	case <-e.done:
	case <-time.After(10 * time.Second):
		t.Fatal("Fixture scheduler did not stop")
	}
	e.workers.Wait()
}
func restartFixtureScheduler(e *Engine) {
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.done = make(chan struct{})
	go e.schedule()
}
func decodeAdmission(t *testing.T, r domain.Result) domain.Admission {
	t.Helper()
	if r.Envelope.Error != nil {
		t.Fatal(r.Envelope.Error)
	}
	return r.Envelope.Data.(domain.Admission)
}
func awaitFixtureCondition(t *testing.T, message string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSchedulerClaimPersistenceFaultFencesWithoutWorkerReplay(t *testing.T) {
	for _, after := range []bool{false, true} {
		name := "before_commit"
		if after {
			name = "after_commit"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			e, project, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			admitted := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Never automatically replay this claim.", ExecutionMode: "fake"}))
			var commits atomic.Int32
			hook := func() error {
				if commits.Add(1) == 2 {
					return errors.New("deterministic claim persistence fault")
				}
				return nil
			}
			// The first empty queue sweep commits; the second transaction is this claim.
			if after {
				e.db.AfterCommit = hook
			} else {
				e.db.BeforeCommit = hook
			}
			restartFixtureScheduler(e)
			e.signal()
			t.Cleanup(func() { stopFixtureScheduler(t, e); e.db.BeforeCommit = nil; e.db.AfterCommit = nil })
			awaitFixtureCondition(t, "Claim fault did not fence dispatch", func() bool { g, err := e.db.Gate(ctx); return err == nil && g.State == "fenced" && g.IncidentID != nil })
			awaitFixtureCondition(t, "Actor did not continue observing after its claim fault", func() bool { return commits.Load() >= 4 })
			a, err := e.db.Attempt(ctx, admitted.Attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if a.LaunchIntent || a.StartedAt != nil || a.SourceSnapshotID != nil || a.Outcome != nil {
				t.Fatalf("Fault dispatched or fabricated a result: %+v", a)
			}
			gate, err := e.db.Gate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if after {
				if a.Phase != "preparing" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != a.ID {
					t.Fatal("Uncertain committed claim was lost", a, gate)
				}
			} else if a.Phase != "queued" || gate.ActiveAttemptID != nil {
				t.Fatal("Known rollback retained a claim", a, gate)
			}
			incident, err := e.db.Incident(ctx, *gate.IncidentID)
			if err != nil || incident.AttemptID == nil || *incident.AttemptID != a.ID {
				t.Fatal(incident, err)
			}
			artifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
			if err != nil || len(artifacts.Items) != 0 {
				t.Fatal("Worker evidence appeared after failed claim", artifacts, err)
			}
			events, err := e.db.Events(ctx, a.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			preparations := 0
			for _, event := range events.Items {
				if event.Kind == "preparation_started" {
					preparations++
				}
				if event.Kind == "launch_intent" || event.Kind == "worker_started" {
					t.Fatal("Worker was invoked after a claim fault")
				}
			}
			expected := 0
			if after {
				expected = 1
			}
			if preparations != expected {
				t.Fatal("Claim was replayed", preparations, expected)
			}
		})
	}
}

func TestAcceptanceUnknownReferencesDoNotFenceButRegisteredCorruptionDoes(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	admission := admit(t, e, project, "response")
	a := settled(t, e, admission.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" || a.ResponseArtifactID == nil {
		t.Fatalf("No successful fixture response: %+v", a)
	}
	record, err := e.db.Artifact(ctx, *a.ResponseArtifactID)
	if err != nil || record.SHA256 == nil {
		t.Fatal(record, err)
	}
	detail, err := e.db.Task(ctx, a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	// Register another settled task/attempt so mismatches test association, not only missing IDs.
	e.control.Lock()
	otherResult := e.db.CreateTask(ctx, e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Another registered task.", ExecutionMode: "fake"})
	var other domain.Admission
	if otherResult.Envelope.Error == nil {
		other = otherResult.Envelope.Data.(domain.Admission)
		_ = e.db.StopAttempt(ctx, other.Attempt.ID, domain.NewID())
	}
	e.control.Unlock()
	if otherResult.Envelope.Error != nil {
		t.Fatal(otherResult.Envelope.Error)
	}
	base := domain.ResponseAcceptanceInput{ExpectedRevision: detail.Task.Revision, TaskVersion: a.TaskVersion, AttemptID: a.ID, ResponseArtifactID: record.ID, ResponseSHA256: *record.SHA256, Confirmation: "reviewed_response_and_conditions"}
	cases := []struct {
		name   string
		taskID string
		mutate func(*domain.ResponseAcceptanceInput)
	}{
		{"unknown_artifact", a.TaskID, func(in *domain.ResponseAcceptanceInput) { in.ResponseArtifactID = domain.NewID() }},
		{"unknown_attempt", a.TaskID, func(in *domain.ResponseAcceptanceInput) { in.AttemptID = domain.NewID() }},
		{"mismatched_task", other.Task.ID, func(*domain.ResponseAcceptanceInput) {}},
		{"mismatched_registered_attempt", a.TaskID, func(in *domain.ResponseAcceptanceInput) { in.AttemptID = other.Attempt.ID }},
		{"incorrect_client_hash", a.TaskID, func(in *domain.ResponseAcceptanceInput) { in.ResponseSHA256 = strings.Repeat("0", 64) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			in.RequestID = domain.NewID()
			tc.mutate(&in)
			r := e.AcceptResponse(ctx, tc.taskID, in)
			if r.Envelope.Error == nil || r.Envelope.Receipt != nil {
				t.Fatal("Invalid acceptance was admitted", r)
			}
			gate, err := e.db.Gate(ctx)
			if err != nil || gate.State != "open" || gate.IncidentID != nil {
				t.Fatal("Invalid client reference fenced dispatch", gate, err)
			}
			e.control.Lock()
			fenced := e.fenced
			e.control.Unlock()
			if fenced {
				t.Fatal("Invalid client reference latched an in-memory fence")
			}
		})
	}
	raw, err := e.readAllResponse(evidenceMetadata(record))
	if err != nil || len(raw) == 0 {
		t.Fatal("Cannot read fixture response", err)
	}
	raw[0] ^= 1
	// Same file identity and length: only a fresh byte/hash verification catches it.
	if err = os.WriteFile(filepath.Join(e.lock.Root(), filepath.FromSlash(record.PrivateRef)), raw, 0600); err != nil {
		t.Fatal(err)
	}
	in := base
	in.RequestID = domain.NewID()
	r := e.AcceptResponse(ctx, a.TaskID, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "acceptance_unavailable" || r.Envelope.Receipt != nil {
		t.Fatal("Corrupt response was accepted", r)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatal("Registered corruption did not fence", gate, err)
	}
	incident, err := e.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.Code != "evidence_corrupt" || incident.AttemptID == nil || *incident.AttemptID != a.ID {
		t.Fatal(incident, err)
	}
	detail, err = e.db.Task(ctx, a.TaskID)
	if err != nil || detail.Acceptance != nil || detail.LatestAttempt.Outcome == nil || *detail.LatestAttempt.Outcome != "succeeded" {
		t.Fatal("Corruption rewrote authoring or invented acceptance", detail, err)
	}
	blocked := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Do not dispatch over corrupt retained evidence.", ExecutionMode: "fake"})
	if blocked.Envelope.Error == nil || blocked.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal(blocked)
	}
}
