package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestWorkflowExplicitBlockedAuthorRetryUsesFreshManualExecution(t *testing.T) {
	ctx := context.Background()
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Controlled local response, without a harness pin.", ExecutionMode: "fake", FakeScenario: "response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
	first := decodeAdmission(t, e.StartTask(ctx, input))
	claimed, err := e.db.ClaimAttempt(ctx, first.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	disableRoutingCodex(t, e)
	e.run(ctx, claimed)
	old := settled(t, e, claimed.ID)
	if old.Outcome == nil || *old.Outcome != "blocked" || old.LaunchIntent || old.Cleanup.State != "verified" || old.Cleanup.Scope != "not_launched" {
		t.Fatal("fixture did not produce an actual clean pre-launch block", old)
	}
	oldStep, err := e.db.WorkflowStep(ctx, old.StepID)
	if err != nil || oldStep.AdvanceProof != nil {
		t.Fatal("blocked execution invented forward proof", oldStep, err)
	}
	e.continueRouting(old.ID)
	e.continueWorkflow(old.ID)
	before, err := e.db.Attempts(ctx, first.Task.ID, 100, "")
	if err != nil || len(before.Items) != 1 {
		t.Fatal("blocked execution automatically retried", before, err)
	}
	advance := workflowControlAdvanceInput(t, e, first.Task.ID)
	raw, err := json.Marshal(advance)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateWorkflow(ctx, "tasks.workflow.advance", first.Task.ID, raw)
	if result.Status != 201 || result.Envelope.Error != nil {
		t.Fatal("current explicit advance cannot retry blocked author", result)
	}
	var next domain.WorkflowStepAdmission
	encoded, err := json.Marshal(result.Envelope.Data)
	if err != nil || json.Unmarshal(encoded, &next) != nil {
		t.Fatal("invalid retry admission", result, err)
	}
	if next.Step.ID == oldStep.ID || next.Step.ReplacementOfStepID == nil || *next.Step.ReplacementOfStepID != oldStep.ID || next.Step.LogicalKey != oldStep.LogicalKey || next.Step.Role != "author" || next.Step.Cycle != 0 || next.Attempt.ConnectionID != "claude-native" || next.Attempt.PreviousAttemptID == nil || *next.Attempt.PreviousAttemptID != old.ID {
		t.Fatal("retry rebound or retargeted the logical predecessor", next)
	}
	if next.Budget.ConsumedAdmissions != "2" || next.Budget.AutomaticAdmissions != "0" || !reflect.DeepEqual(next.Budget.CurrentWindow, first.Budget.CurrentWindow) {
		t.Fatal("manual retry reset the window or avoided finite debit", next.Budget, first.Budget)
	}
	replay := e.MutateWorkflow(ctx, "tasks.workflow.advance", first.Task.ID, raw)
	firstJSON, firstErr := json.Marshal(result)
	replayJSON, replayErr := json.Marshal(replay)
	var firstValue, replayValue any
	if firstErr != nil || replayErr != nil || json.Unmarshal(firstJSON, &firstValue) != nil || json.Unmarshal(replayJSON, &replayValue) != nil || !reflect.DeepEqual(firstValue, replayValue) {
		t.Fatal("retry acknowledgement changed its complete JSON envelope", firstErr, replayErr)
	}
	restartFixtureScheduler(e)
	finished := settled(t, e, next.Attempt.ID)
	if finished.Outcome == nil || *finished.Outcome != "succeeded" || finished.ResponseArtifactID == nil {
		t.Fatal("replacement did not run its real fixed local author", finished)
	}
	retained, err := e.db.WorkflowStep(ctx, oldStep.ID)
	if err != nil || !reflect.DeepEqual(retained, oldStep) {
		t.Fatal("manual retry rewrote old blocked step", retained, oldStep, err)
	}
	retainedAttempt, err := e.db.Attempt(ctx, old.ID)
	if err != nil || retainedAttempt.Outcome == nil || *retainedAttempt.Outcome != "blocked" || retainedAttempt.LaunchIntent || !reflect.DeepEqual(retainedAttempt.PrimaryFailure, old.PrimaryFailure) || !reflect.DeepEqual(retainedAttempt.SettledAt, old.SettledAt) {
		t.Fatal("old blocked terminal was revived", retainedAttempt, old, err)
	}
}
