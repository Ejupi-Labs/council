package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func claudeFixtureInput(project, scenario string) domain.StartTaskInput {
	return domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Perform only this explicit local Claude-shaped fixture task.", Pins: &domain.ExecutionPins{HarnessID: "claude-code"}, ExecutionMode: "fake", FakeScenario: scenario, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
}
func admitClaude(t *testing.T, e *Engine, project, scenario string) domain.Admission {
	t.Helper()
	return decodeAdmission(t, e.StartTask(context.Background(), claudeFixtureInput(project, scenario)))
}

func TestInteropCapturedHarnessDraftPolicyReplayAndVersionedGuidance(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	auto := claudeFixtureInput(project, "response")
	auto.Pins = nil
	defaultAdmission := decodeAdmission(t, e.StartTask(ctx, auto))
	if defaultAdmission.Attempt.ConnectionID != "codex-native" || defaultAdmission.Version.Pins.HarnessID != "" {
		t.Fatal("Auto intent was injected or rerouted", defaultAdmission)
	}
	draftResult := e.db.CreateDraft(ctx, domain.NewID(), "Claude source draft", project, "\ufeff Exact original\r\n café \ufffd instructions")
	if draftResult.Envelope.Error != nil {
		t.Fatal(draftResult)
	}
	draft := draftResult.Envelope.Data.(domain.Draft)
	policy, err := e.db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r := e.db.SetPolicy(ctx, domain.NewID(), "later supervised global policy", policy.Revision, "supervised"); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	in := claudeFixtureInput(project, "response")
	in.OriginalText = draft.OriginalText
	in.SourceDraftRef = &domain.DraftRef{DraftID: draft.ID, Revision: draft.Revision}
	in.Pins.Model = "native-alias"
	first := e.StartTask(ctx, in)
	admission := decodeAdmission(t, first)
	if admission.Attempt.ConnectionID != "claude-native" || admission.Attempt.RequestedTarget.HarnessID != "claude-code" || admission.Version.RequestedPolicy != draft.RequestedPolicy || admission.Version.Pins.Model != "native-alias" {
		t.Fatal(admission)
	}
	if r := e.db.UpdateDraft(ctx, domain.NewID(), "changed draft", draft.ID, draft.Revision, "Newer draft is not the started version", true); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	replayed := e.StartTask(ctx, in)
	if replayed.Envelope.Error != nil || replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *first.Envelope.Receipt {
		t.Fatal("replay lost original receipt", replayed)
	}
	// Replayed data decodes through the persisted envelope, not an in-memory DTO.
	var replayAdmission domain.Admission
	if err := decodeInteropData(replayed.Envelope.Data, &replayAdmission); err != nil || !reflect.DeepEqual(replayAdmission, admission) {
		t.Fatal("replay refreshed captured policy/target", replayAdmission, err)
	}
	changed := in
	changed.Pins = &domain.ExecutionPins{HarnessID: "codex"}
	if r := e.StartTask(ctx, changed); r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal("same identity changed harness", r)
	}
	guidance := domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", GuidanceText: "Use Codex for a later explicit attempt.", Pins: &domain.ExecutionPins{HarnessID: "codex"}}
	if r := e.Guidance(ctx, admission.Task.ID, guidance); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	if _, err = e.db.SweepQueued(ctx, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	old, err := e.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || old.Outcome == nil || *old.Outcome != "blocked" || old.PrimaryFailure == nil || old.PrimaryFailure.Code != "guidance_changed" || old.ConnectionID != "claude-native" || old.LaunchIntent {
		t.Fatal(old, err)
	}
	newAttempt := decodeAdmission(t, e.StartAttempt(ctx, admission.Task.ID, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: "2", TaskVersion: "2", ExecutionMode: "fake"}))
	if newAttempt.Attempt.ConnectionID != "codex-native" || newAttempt.Attempt.Phase != "queued" || newAttempt.Attempt.PreviousAttemptID == nil || *newAttempt.Attempt.PreviousAttemptID != old.ID {
		t.Fatal(newAttempt)
	}
	for _, id := range []string{defaultAdmission.Attempt.ID, old.ID, newAttempt.Attempt.ID} {
		artifacts, err := e.db.Artifacts(ctx, id, 100, "")
		if err != nil || len(artifacts.Items) != 0 {
			t.Fatal("stopped scheduler dispatched evidence", artifacts, err)
		}
	}
}

func TestInteropCapturedConnectionMismatchBlocksBeforeSourceOrOutputReservation(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	admission := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, claudeFixtureInput(project, "response")))
	claimed, err := e.db.ClaimAttempt(ctx, admission.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	claimed.ConnectionID = "codex-native" // Controlled corrupted scheduler handoff.
	e.run(ctx, claimed)
	a, err := e.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || a.Outcome == nil || *a.Outcome != "blocked" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "pin_unavailable" || a.ConnectionID != "claude-native" || a.LaunchIntent || a.SourceSnapshotID != nil {
		t.Fatal("connection mismatch crossed dispatch boundary", a, err)
	}
	artifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil || len(artifacts.Items) != 0 {
		t.Fatal(artifacts, err)
	}
}

func TestInteropClaudeStorageValidPromptBlocksBeforeSourceOrWorker(t *testing.T) {
	e, project, _ := testEngine(t)
	in := claudeFixtureInput(project, "candidate")
	in.OriginalText = "\ufeff" + strings.Repeat("x", domain.MaxPromptBytes)
	admission := decodeAdmission(t, e.StartTask(context.Background(), in))
	a := settled(t, e, admission.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "blocked" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "input_too_large" || a.LaunchIntent || a.StartedAt != nil || a.SourceSnapshotID != nil || a.ConnectionID != "claude-native" {
		t.Fatal(a)
	}
	detail, err := e.db.Task(context.Background(), a.TaskID)
	if err != nil || detail.Task.OriginalText != in.OriginalText {
		t.Fatal("stored original truncated", err)
	}
	artifacts, err := e.db.Artifacts(context.Background(), a.ID, 100, "")
	if err != nil || len(artifacts.Items) != 0 {
		t.Fatal(artifacts, err)
	}
}
