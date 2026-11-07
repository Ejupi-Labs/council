package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Real control calls are held at the preparation boundary while Close runs.
// A failed mutation and failed fence write must never cross a clean epoch marker.
func TestRecoveryOwnerLifecycleDrainsControlsBeforeCleanEpoch(t *testing.T) {
	for _, kind := range []string{"pause", "reconcile"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			e, project, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			var invoke func() domain.Result
			if kind == "pause" {
				a := admit(t, e, project, "response")
				detail, err := e.db.Task(ctx, a.Task.ID)
				if err != nil {
					t.Fatal(err)
				}
				input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: detail.Task.CurrentVersion, ExpectedTaskRevision: detail.Task.Revision}
				invoke = func() domain.Result { return e.PauseTask(ctx, detail.Task.ID, input) }
			} else {
				e.fence("", "fixture_incident", "A controlled retained incident.")
				gate, err := e.db.Gate(ctx)
				if err != nil || gate.IncidentID == nil {
					t.Fatal(gate, err)
				}
				id := *gate.IncidentID
				view, err := e.db.RecoveryIncident(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				input := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: view.Revision, Action: "verify_and_reconcile"}
				invoke = func() domain.Result { return e.ReconcileIncident(ctx, id, input) }
			}
			e.prepareGate.Lock()
			resultCh := make(chan domain.Result, 1)
			go func() { resultCh <- invoke() }()
			awaitFixtureCondition(t, "control admission", func() bool {
				e.lifecycleMu.Lock()
				defer e.lifecycleMu.Unlock()
				return e.lifecycleActive == 1
			})
			closed := make(chan error, 1)
			go func() { closed <- e.Close() }()
			awaitFixtureCondition(t, "closing admission", func() bool {
				e.lifecycleMu.Lock()
				defer e.lifecycleMu.Unlock()
				return e.lifecycleClosing
			})
			select {
			case err := <-closed:
				t.Fatalf("close passed an admitted %s: %v", kind, err)
			default:
			}
			late := e.PauseTask(ctx, domain.NewID(), domain.PauseTaskInput{RequestID: domain.NewID()})
			if late.Envelope.Error == nil || late.Envelope.Error.Code != "owner_closing" {
				t.Fatalf("late control was admitted: %+v", late)
			}
			if _, err := e.Meta(ctx); err == nil {
				t.Fatal("late metadata read was admitted")
			}
			var failures atomic.Int32
			e.db.BeforeCommit = func() error {
				if failures.Add(1) <= 2 {
					return errors.New("controlled persistence failure")
				}
				return nil
			}
			e.prepareGate.Unlock()
			select {
			case result := <-resultCh:
				if result.Envelope.Error == nil {
					t.Fatalf("%s unexpectedly committed: %+v", kind, result)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("admitted operation did not finish")
			}
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("close did not drain")
			}
			e.db.BeforeCommit = nil
			e.control.Lock()
			uncertain := e.fenceUncertain
			e.control.Unlock()
			if !uncertain {
				t.Fatal("failed fence persistence was not retained")
			}
			next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			gate, err := next.db.Gate(ctx)
			if err != nil || gate.State != "fenced" {
				t.Fatalf("restart failed closed: %+v %v", gate, err)
			}
		})
	}
}
