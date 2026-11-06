package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRoutingProfileLaunchesOwnedClaudeFixtureWithoutChangingIntent(t *testing.T) {
	e, project, _ := testEngine(t)
	ctx := context.Background()
	in := domain.StartTaskInput{
		RequestID: domain.NewID(), ProjectID: project,
		OriginalText:  "\ufeffKeep the exact task 東京\r\n$(literal text)",
		ExecutionMode: "fake", FakeScenario: "response",
		Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"},
		Routing:    &domain.RoutingOptions{TargetProfileID: "claude-haiku-economy"},
	}
	first := e.StartTask(ctx, in)
	admission := decodeAdmission(t, first)
	if admission.RoutingDecision == nil || admission.Budget == nil || admission.Attempt.ResolvedTarget == nil || admission.Attempt.RoutingDecisionID == nil {
		t.Fatal("admission omitted captured routing or finite authority", first)
	}
	if admission.Attempt.ConnectionID != "claude-native" || admission.Attempt.ResolvedTarget.ModelSelector != "haiku" || admission.Version.Pins != (domain.ExecutionPins{}) {
		t.Fatal("the route changed original pins or selected a different native target", admission)
	}
	if admission.Budget.GrantedAdmissions != "2" || admission.Budget.ConsumedAdmissions != "1" || admission.Budget.RemainingAdmissions != "1" || admission.Budget.CurrentWindow.FirstAdmissionAt == nil {
		t.Fatal("the first admission did not open and debit one shared budget", admission.Budget)
	}
	a := settled(t, e, admission.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "succeeded" || a.Cleanup.State != "verified" || a.ResponseArtifactID == nil {
		t.Fatal("the resolved fixture did not reach verified authoring settlement", a)
	}
	if a.TargetObservations.Model.Status != "unknown" || a.TargetObservations.Account.Status != "unknown" || a.TargetObservations.NativeSession.Status != "unknown" {
		t.Fatal("a local fixture invented live provider identity", a.TargetObservations)
	}
	detail, err := e.db.Task(ctx, admission.Task.ID)
	if err != nil || detail.Task.OriginalText != in.OriginalText || detail.Version.Pins != (domain.ExecutionPins{}) {
		t.Fatal("dispatch rewrote retained instructions or literal pins", detail, err)
	}
	replayed := e.StartTask(ctx, in)
	var originalAdmission domain.Admission
	if replayed.Status != first.Status || replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *first.Envelope.Receipt || decodeInteropData(replayed.Envelope.Data, &originalAdmission) != nil || !reflect.DeepEqual(admission, originalAdmission) {
		t.Fatal("exact replay refreshed a historical route or debited again", replayed)
	}
}

func TestRoutingConflictsHaveNoReceiptTaskOrWorker(t *testing.T) {
	for _, pins := range []domain.ExecutionPins{
		{HarnessID: "codex"},
		{Model: "sonnet"},
		{PaymentPath: "paid_api"},
		{AccountID: "0123456789abcdef0123456789abcdef"},
	} {
		t.Run(pins.HarnessID+pins.Model+pins.PaymentPath+pins.AccountID, func(t *testing.T) {
			e, project, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			result := e.StartTask(context.Background(), domain.StartTaskInput{
				RequestID: domain.NewID(), ProjectID: project, OriginalText: "Perform this bounded local fixture task.",
				Pins: &pins, Routing: &domain.RoutingOptions{TargetProfileID: "claude-haiku-economy"}, ExecutionMode: "fake",
			})
			if result.Envelope.Error == nil || result.Envelope.Error.Code != "pin_unavailable" || result.Envelope.Receipt != nil {
				t.Fatal("an incompatible pin acquired authoring authority", result)
			}
			tasks, err := e.db.Tasks(context.Background(), project, 100, "")
			if err != nil || len(tasks.Items) != 0 {
				t.Fatal("a rejected route retained a task or admitted worker", tasks, err)
			}
		})
	}
}

func TestRoutingManagedOccupancyQueuesWithoutInventingIneligibility(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	first := admit(t, e, project, "response")
	if _, err := e.db.ClaimAttempt(ctx, first.Attempt.ID, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = e.db.SettleAttempt(context.Background(), first.Attempt.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"})
	})
	second := decodeAdmission(t, e.StartTask(ctx, domain.StartTaskInput{
		RequestID: domain.NewID(), ProjectID: project, OriginalText: "Perform another explicitly queued local task.",
		Routing: &domain.RoutingOptions{TargetProfileID: "claude-haiku-economy"}, ExecutionMode: "fake",
	}))
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != first.Attempt.ID || second.Attempt.Phase != "queued" || second.Attempt.ConnectionID != "claude-native" {
		t.Fatal("normal global occupancy was rejected or bypassed", gate, second, err)
	}
	if second.Budget == nil || second.Budget.ConsumedAdmissions != "1" || second.Budget.CurrentWindow.DeadlineAt == nil {
		t.Fatal("queue waiting did not consume finite admission authority", second.Budget)
	}
}

func TestRoutingSameBytesPhysicalReplacementAfterAssessmentBlocksBeforePreparation(t *testing.T) {
	e, projectID, root := testEngine(t)
	textPath := filepath.Join(root, "source.txt")
	if err := os.WriteFile(textPath, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "controlled-invalid-native.exe")
	if err := os.WriteFile(launcher, []byte("Controlled nonexecutable fixture; never a vendor launcher."), 0700); err != nil {
		t.Fatal(err)
	}
	var replaced atomic.Bool
	e.config.Resolve = func(_ context.Context, pins domain.ExecutionPins) (string, error) {
		// Context metadata checks have no model. The selected-model resolver is
		// reached after run's first original-source proof check and before Capture.
		if pins.Model != "" && replaced.CompareAndSwap(false, true) {
			replacement := filepath.Join(root, "replacement.txt")
			if err := os.WriteFile(replacement, []byte("old\n"), 0600); err != nil {
				return "", err
			}
			if err := os.Rename(replacement, textPath); err != nil {
				return "", err
			}
		}
		// A failed source guard reaches only a controlled invalid executable;
		// it cannot recursively run this test binary or contact a vendor.
		return launcher, nil
	}
	in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: `Replace "old" with "new" in source.txt only`, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
	admission := decodeAdmission(t, e.StartTask(context.Background(), in))
	a := settled(t, e, admission.Attempt.ID)
	if !replaced.Load() || a.Outcome == nil || *a.Outcome != "blocked" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "source_changed" || a.LaunchIntent || a.StartedAt != nil || a.Cleanup.Scope != "not_launched" {
		t.Fatal("a stale physical source proof reached native preparation", a)
	}
}

func TestRoutingProjectReplacementAfterInitialIdentityCheckBlocksBeforePreparation(t *testing.T) {
	e, projectID, root := testEngine(t)
	_, _, replacement := testEngine(t)
	saveRoot := t.TempDir()
	base := filepath.Dir(root)
	// All moved directories were created by this test under its one TempDir
	// namespace; never move a path supplied by a user or native configuration.
	if filepath.Dir(replacement) != base || filepath.Dir(saveRoot) != base {
		t.Fatal("project replacement fixture escaped its temporary namespace")
	}
	launcher := filepath.Join(t.TempDir(), "controlled-invalid-native.exe")
	if err := os.WriteFile(launcher, []byte("Controlled invalid native fixture."), 0700); err != nil {
		t.Fatal(err)
	}
	var replaced atomic.Bool
	e.config.Resolve = func(_ context.Context, pins domain.ExecutionPins) (string, error) {
		if pins.Model != "" && replaced.CompareAndSwap(false, true) {
			if err := os.Rename(root, filepath.Join(saveRoot, "saved-project")); err != nil {
				return "", err
			}
			if err := os.Rename(replacement, root); err != nil {
				return "", err
			}
		}
		return launcher, nil
	}
	in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "CONTROLLED LOCAL TEST: inspect this project without a derived file proof.", Routing: &domain.RoutingOptions{TargetProfileID: "codex-luna-economy"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
	a := settled(t, e, decodeAdmission(t, e.StartTask(context.Background(), in)).Attempt.ID)
	if !replaced.Load() || a.Outcome == nil || *a.Outcome != "blocked" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "project_identity_changed" || a.LaunchIntent || a.SourceSnapshotID != nil || a.Cleanup.Scope != "not_launched" {
		t.Fatal("a different project was captured under the saved physical identity", a)
	}
}
