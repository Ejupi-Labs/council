package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func waitInteropRunning(t *testing.T, e *Engine, id string) {
	t.Helper()
	awaitFixtureCondition(t, "Claude fixture did not reach running", func() bool {
		a, err := e.db.Attempt(context.Background(), id)
		return err == nil && a.Phase == "running"
	})
}

func TestInteropMixedCapacityStopAndEarlierFailureAuthority(t *testing.T) {
	e, project, _ := testEngine(t)
	ctx := context.Background()
	// Independent harnesses can use both managed slots. Declare one shared
	// account explicitly so this fixture exercises the narrower account limit.
	body, err := json.Marshal(domain.AccountCreateInput{RequestID: domain.NewID(), Label: "Controlled shared account"})
	if err != nil {
		t.Fatal(err)
	}
	created := e.MutateRouting(ctx, "routing.accounts.create", "", body)
	if created.Status != 201 || created.Envelope.Error != nil {
		t.Fatal("shared account setup failed", created)
	}
	account := created.Envelope.Data.(domain.DeclaredAccount)
	contexts, err := e.db.RoutingContexts(ctx)
	if err != nil || len(contexts.Items) != 2 {
		t.Fatal("expected both native contexts", contexts, err)
	}
	for _, native := range contexts.Items {
		body, err = json.Marshal(domain.AccountLinkInput{RequestID: domain.NewID(), ExpectedRevision: native.AccountLink.Revision, DeclaredAccountID: account.ID})
		if err != nil {
			t.Fatal(err)
		}
		linked := e.MutateRouting(ctx, "routing.links.update", native.ID, body)
		if linked.Envelope.Error != nil {
			t.Fatal("shared account link failed", native.ID, linked)
		}
	}
	first := admitClaude(t, e, project, "slow")
	waitInteropRunning(t, e, first.Attempt.ID)
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != first.Attempt.ID || gate.ActiveClaims[0].ContextID == nil || *gate.ActiveClaims[0].ContextID != "claude-native" || gate.ActiveClaims[0].CapturedDeclaredAccountID == nil || *gate.ActiveClaims[0].CapturedDeclaredAccountID != account.ID {
		t.Fatal("Claude did not hold the exact shared account claim", gate, err)
	}
	second := decodeAdmission(t, e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Perform the explicit local fixture task.", Pins: &domain.ExecutionPins{HarnessID: "codex"}, ExecutionMode: "fake", FakeScenario: "response", Workflow: &domain.WorkflowInput{Mode: "author_only"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}))
	if second.Attempt.ConnectionID != "codex-native" || second.Attempt.ResolvedTarget == nil || second.Attempt.ResolvedTarget.DeclaredAccountAssociation == nil || second.Attempt.ResolvedTarget.DeclaredAccountAssociation.AccountID != account.ID {
		t.Fatal("Codex did not capture the same declared account", second)
	}
	queued, err := e.db.Attempt(ctx, second.Attempt.ID)
	if err != nil || queued.Phase != "queued" || queued.LaunchIntent || queued.SourceSnapshotID != nil {
		t.Fatal("other harness bypassed shared account reservation", queued, err)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != first.Attempt.ID {
		t.Fatal("queued work changed the original claim", gate, err)
	}
	// This is an injected controller failure, not an instruction from native prose.
	e.recordFailure(first.Attempt.ID, primary("fixture_primary_failure", "A controlled earlier failure must win over later Stop."))
	request := domain.NewID()
	stop := e.Stop(ctx, first.Attempt.ID, request)
	if stop.Envelope.Error != nil || stop.Envelope.Receipt == nil {
		t.Fatal(stop)
	}
	if replay := e.Stop(ctx, first.Attempt.ID, request); replay.Envelope.Error != nil || replay.Envelope.Receipt == nil || *replay.Envelope.Receipt != *stop.Envelope.Receipt {
		t.Fatal(replay)
	}
	a := settled(t, e, first.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "failed" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "fixture_primary_failure" || a.Stop == nil || a.Cleanup.State != "verified" {
		t.Fatal("late Stop replaced failure or left ownership", a)
	}
	if a := settled(t, e, second.Attempt.ID); a.Outcome == nil || *a.Outcome != "succeeded" || a.ConnectionID != "codex-native" {
		t.Fatal("verified cleanup did not transfer shared resource", a)
	}
}

func TestInteropClaudeClaimLostAcknowledgementFencesBeforeAnyWorker(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	in := claudeFixtureInput(project, "response")
	admission := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, in))
	var commits atomic.Int32
	e.db.AfterCommit = func() error {
		if commits.Add(1) == 2 {
			return errors.New("Claude claim committed but acknowledgement lost")
		}
		return nil
	}
	restartFixtureScheduler(e)
	e.signal()
	t.Cleanup(func() { stopFixtureScheduler(t, e); e.db.AfterCommit = nil })
	awaitFixtureCondition(t, "Claude uncertain claim did not fence", func() bool {
		g, err := e.db.Gate(ctx)
		return err == nil && g.State == "fenced" && g.ActiveAttemptID != nil && *g.ActiveAttemptID == admission.Attempt.ID
	})
	a, err := e.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || a.Phase != "preparing" || a.ConnectionID != "claude-native" || a.LaunchIntent || a.SourceSnapshotID != nil || a.StartedAt != nil {
		t.Fatal("unacknowledged claim launched/replayed", a, err)
	}
	if r := e.StartTask(ctx, in); r.Envelope.Error != nil || r.Envelope.Receipt == nil {
		t.Fatal("exact receipt reconciliation blocked", r)
	}
	artifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil || len(artifacts.Items) != 0 {
		t.Fatal("worker evidence exists after claim fault", artifacts, err)
	}
}

func TestInteropClaudeRunningDeadlinePreservesFailureAndVerifiedCleanup(t *testing.T) {
	e, project, _ := testEngine(t)
	ctx := context.Background()
	in := claudeFixtureInput(project, "slow")
	timeout := 5
	in.TimeoutSeconds = &timeout
	admission := decodeAdmission(t, e.StartTask(ctx, in))
	waitInteropRunning(t, e, admission.Attempt.ID)
	running, err := e.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || running.StartedAt == nil || !running.LaunchIntent {
		t.Fatal("test did not exercise a running native worker", running, err)
	}
	a := settled(t, e, admission.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "failed" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "deadline_exceeded" || a.Stop != nil || a.Cleanup.State != "verified" || a.ResponseArtifactID != nil || a.DeadlineAt != admission.Attempt.DeadlineAt {
		t.Fatalf("Running deadline became cancellation/success or changed budget: %+v failure=%+v", a, a.PrimaryFailure)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || gate.ActiveAttemptID != nil {
		t.Fatal("verified expired worker retained false capacity/fence", gate, err)
	}
	artifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	hasStdout := false
	for _, m := range artifacts.Items {
		if m.Kind == "stdout" {
			hasStdout = true
			if m.ByteLength == "0" {
				t.Fatal("available predeadline init bytes lost")
			}
		}
	}
	if !hasStdout {
		t.Fatal("running deadline did not retain stdout prefix")
	}
}

func TestInteropClaudeOwnedDescendantAndUnknownCleanupKeepHonestCapacity(t *testing.T) {
	for _, scenario := range []string{"owned_descendant", "cleanup_unknown"} {
		t.Run(scenario, func(t *testing.T) {
			e, project, _ := testEngine(t)
			ctx := context.Background()
			in := claudeFixtureInput(project, scenario)
			first := e.StartTask(ctx, in)
			admission := decodeAdmission(t, first)
			a := settled(t, e, admission.Attempt.ID)
			gate, err := e.db.Gate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "cleanup_unknown" {
				if a.Outcome == nil || *a.Outcome == "succeeded" || a.Cleanup.State != "unknown" || a.Cleanup.Scope != cancellationScope() || a.Cleanup.IncidentID == nil || a.ResponseArtifactID != nil || gate.State != "fenced" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != a.ID || gate.IncidentID == nil || *gate.IncidentID != *a.Cleanup.IncidentID {
					t.Fatal("unverified cleanup published success or released capacity", a, gate)
				}
				if !bytes.Contains(interopRaw(t, e, a, "stdout"), []byte(`"type":"result"`)) {
					t.Fatal("unverified cleanup erased obtained terminal bytes")
				}
				if replay := e.StartTask(ctx, in); replay.Envelope.Error != nil || replay.Envelope.Receipt == nil || *replay.Envelope.Receipt != *first.Envelope.Receipt {
					t.Fatal("cleanup uncertainty blocked exact reconciliation", replay)
				}
				fresh := claudeFixtureInput(project, "response")
				fresh.Pins = &domain.ExecutionPins{HarnessID: "codex"}
				if r := e.StartTask(ctx, fresh); r.Envelope.Error == nil || r.Envelope.Error.Code != "dispatch_fenced" {
					t.Fatal("other harness bypassed unknown cleanup", r)
				}
				return
			}
			if a.Outcome == nil || *a.Outcome != "succeeded" || a.Cleanup.State != "verified" || a.Cleanup.Scope != cancellationScope() || a.ResponseArtifactID == nil || gate.State != "open" || gate.ActiveAttemptID != nil {
				t.Fatalf("Owned child cleanup did not permit honest success: %+v failure=%+v gate=%+v", a, a.PrimaryFailure, gate)
			}
			markers, err := filepath.Glob(filepath.Join(e.lock.Root(), "claude-fixture-*", "owned-descendant.marker"))
			if err != nil || len(markers) != 1 {
				t.Fatal("fixture did not prove owned child started", markers, err)
			}
			before, err := os.ReadFile(markers[0])
			if err != nil || len(before) == 0 {
				t.Fatal(err)
			}
			time.Sleep(125 * time.Millisecond)
			after, err := os.ReadFile(markers[0])
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("owned descendant continued writing after verified cleanup", string(before), string(after), err)
			}
		})
	}
}

func TestInteropClaudeQueuedDeadlineAndRestartDoNotDispatch(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	stopped := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, claudeFixtureInput(project, "response")))
	if r := e.Stop(ctx, stopped.Attempt.ID, domain.NewID()); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	if _, err := e.db.SweepQueued(ctx, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	beforeLaunch, err := e.db.Attempt(ctx, stopped.Attempt.ID)
	if err != nil || beforeLaunch.Outcome == nil || *beforeLaunch.Outcome != "cancelled" || beforeLaunch.LaunchIntent || beforeLaunch.StartedAt != nil || beforeLaunch.SourceSnapshotID != nil {
		t.Fatal("queued Stop launched work", beforeLaunch, err)
	}
	in := claudeFixtureInput(project, "response")
	one := 1
	in.TimeoutSeconds = &one
	expiring := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, in))
	deadline, err := time.Parse(time.RFC3339Nano, expiring.Attempt.DeadlineAt)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(max(time.Until(deadline)+10*time.Millisecond, 0))
	if _, err = e.db.SweepQueued(ctx, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	expired, err := e.db.Attempt(ctx, expiring.Attempt.ID)
	if err != nil || expired.Outcome == nil || *expired.Outcome != "failed" || expired.PrimaryFailure == nil || expired.PrimaryFailure.Code != "deadline_exceeded" || expired.LaunchIntent || expired.SourceSnapshotID != nil {
		t.Fatal(expired, err)
	}
	queued := decodeAdmission(t, e.db.CreateTask(ctx, e.config.OwnerInstanceID, claudeFixtureInput(project, "response")))
	if err = e.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err = e.evidence.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	a, err := next.db.Attempt(ctx, queued.Attempt.ID)
	if err != nil || a.Outcome == nil || *a.Outcome != "interrupted" || a.ConnectionID != "claude-native" || a.LaunchIntent || a.StartedAt != nil || a.Cleanup.State != "verified" {
		t.Fatal("restart replayed or reinterpreted queued Claude", a, err)
	}
	revision := a.Revision
	fresh := admitClaude(t, next, project, "response")
	if a := settled(t, next, fresh.Attempt.ID); a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatal(a)
	}
	if retained, err := next.db.Attempt(ctx, a.ID); err != nil || retained.Revision != revision || retained.StartedAt != nil {
		t.Fatal("explicit new work replayed old attempt", retained, err)
	}
}
