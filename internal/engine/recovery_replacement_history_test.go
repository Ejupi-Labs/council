package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Replacement is a fresh, explicitly requested attempt. These fixtures use
// an actual fake-worker partial failure and a separately accepted queued Stop.
func TestRecoveryExplicitReplacementRetainsFailureAndCancelledHistory(t *testing.T) {
	for _, scenario := range []string{"partial_failure", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			e, project, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			fake := scenario
			if fake == "cancelled" {
				fake = "response"
			}
			input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Preserve predecessor failure and evidence.", ExecutionMode: "fake", FakeScenario: fake, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
			first := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, input))
			if scenario == "cancelled" {
				stopped := e.Stop(ctx, first.Attempt.ID, domain.NewID())
				if stopped.Envelope.Error != nil {
					t.Fatal(stopped)
				}
				if _, err := e.db.SweepQueued(ctx, e.config.OwnerInstanceID); err != nil {
					t.Fatal(err)
				}
			} else {
				claimed, err := e.db.ClaimAttempt(ctx, first.Attempt.ID, e.config.OwnerInstanceID)
				if err != nil {
					t.Fatal(err)
				}
				e.run(ctx, claimed)
			}
			old, err := e.db.Attempt(ctx, first.Attempt.ID)
			if err != nil || old.Outcome == nil || old.Cleanup.State != "verified" {
				t.Fatal(old, err)
			}
			if scenario == "partial_failure" && (*old.Outcome != "failed" || old.PrimaryFailure == nil || old.CandidateID == nil || !old.AuthoredDelta) {
				t.Fatal("missing actual partial failure", old)
			}
			if scenario == "cancelled" && (*old.Outcome != "cancelled" || old.Stop == nil) {
				t.Fatal("missing accepted Stop", old)
			}
			artifacts, err := e.db.Artifacts(ctx, old.ID, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			task, err := e.db.Task(ctx, old.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			result := e.StartAttempt(ctx, old.TaskID, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Task.CurrentVersion, ExecutionMode: "fake", FakeScenario: "response"})
			next := decodeAdmission(t, result)
			if next.Attempt.ID == old.ID || next.Attempt.PreviousAttemptID == nil || *next.Attempt.PreviousAttemptID != old.ID {
				t.Fatal("replacement lost its exact predecessor", next)
			}
			retained, err := e.db.Attempt(ctx, old.ID)
			// Observation clock and countdown are projections, not durable fields.
			retained.ObservedAt, retained.RemainingSeconds = old.ObservedAt, old.RemainingSeconds
			if err != nil || !reflect.DeepEqual(retained, old) {
				t.Fatal("replacement changed predecessor outcome, stop, failure, or candidate", old, retained, err)
			}
			after, err := e.db.Artifacts(ctx, old.ID, 100, "")
			if err != nil || !reflect.DeepEqual(after, artifacts) {
				t.Fatal("replacement changed prior evidence", artifacts, after, err)
			}
		})
	}
}
