package engine

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// A controlled local process reaches the real ready marker; the watchdog runs
// while its actual owned process and capacity claim are still live.
func TestRecoveryReconcileActualWatchdogOwnedCleanupAndEvidence(t *testing.T) {
	f := graphOutputAdmittedAttemptForScenarioFixture(t, "graph-wait", "Run the controlled writer and preserve its retained result.")
	e, a := f.engine, f.attempt
	ctx := context.Background()
	cwdReady := make(chan string, 1)
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		owned, err := process.Prepare(ctx, spec)
		if err == nil {
			cwdReady <- spec.Cwd
		}
		return owned, err
	}
	done := make(chan struct{})
	go func() { defer close(done); e.run(ctx, a) }()
	var cwd string
	select {
	case cwd = <-cwdReady:
	case <-time.After(15 * time.Second):
		t.Fatal("owned fake worker did not reach native prepare")
	}
	ready := filepath.Join(cwd, fixture.GraphReadyFile)
	deadline := time.Now().Add(8 * time.Second)
	for {
		body, err := os.ReadFile(ready)
		if err == nil && string(body) == "ready\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned fake worker did not reach its actual ready marker", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	running, err := e.db.Attempt(ctx, a.ID)
	if err != nil || running.Phase != "running" || !running.LaunchIntent {
		t.Fatal("worker was not actually running", running, err)
	}
	task, err := e.db.Task(ctx, a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	pause := e.PauseTask(ctx, a.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision})
	if pause.Envelope.Error != nil || pause.Envelope.Receipt == nil {
		t.Fatal("running member pause failed", pause)
	}
	pauseBody, err := json.Marshal(pause.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var paused domain.RecoveryPauseResult
	if err = json.Unmarshal(pauseBody, &paused, json.RejectUnknownMembers(true)); err != nil || paused.Checkpoint.State != "pausing" {
		t.Fatal("running member did not retain a draining checkpoint", paused, err)
	}
	e.expireStopBudget(a.ID)
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != a.ID {
		t.Fatal("watchdog did not retain exact live claim and incident", gate, err)
	}
	watchdogID := *gate.IncidentID
	temp := filepath.Join(cwd, "."+fixture.GraphReleaseFile+"-"+domain.NewID()+".tmp")
	if err := os.WriteFile(temp, []byte("release\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, filepath.Join(cwd, fixture.GraphReleaseFile)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(35 * time.Second):
		t.Fatal("owned worker did not settle after controlled release")
	}
	settled, err := e.db.Attempt(ctx, a.ID)
	if err != nil || settled.Phase != "settled" || settled.Cleanup.State != "verified" || settled.EvidenceStatus != "complete" {
		t.Fatal("watchdog path did not retain fully verified settlement", settled, err)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != a.ID {
		t.Fatal("unresolved watchdog claim was not retained", gate, err)
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || dispatch.Public.OutputObservation == nil || dispatch.Public.OutputObservation.Candidate == nil || dispatch.Public.OutputObservation.Candidate.ClosureRef == nil {
		t.Fatal("real candidate effect closure unavailable", dispatch, err)
	}
	inventory, err := e.db.RecoveryEvidenceInventory(ctx, a.ID)
	if err != nil || inventory.CleanupEventSequence == "" || len(inventory.Artifacts) == 0 {
		t.Fatal("actual owned cleanup/inventory proof unavailable", inventory, err)
	}
	// Deliberately corrupt a closed required fake response. A failed fresh read
	// must not release ownership; restoring its exact bytes lets the real owner
	// perform a new, independent physical check for the later positive case.
	var response *domain.ArtifactRecord
	for i := range inventory.Artifacts {
		if inventory.Artifacts[i].Kind == "final_response" {
			response = &inventory.Artifacts[i]
			break
		}
	}
	if response == nil {
		t.Fatal("actual response evidence missing")
	}
	path := filepath.Join(e.lock.Root(), filepath.FromSlash(response.PrivateRef))
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(append([]byte{}, original...), byte('!')), 0600); err != nil {
		t.Fatal(err)
	}
	view, err := e.db.RecoveryIncident(ctx, watchdogID)
	if err != nil {
		t.Fatal(err)
	}
	negativeInput := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: view.Revision, Action: "verify_and_reconcile"}
	negative := e.ReconcileIncident(ctx, watchdogID, negativeInput)
	negativeResult := recoveryEngineResult(t, negative)
	if negativeResult.Incident.State != "unresolved" {
		t.Fatal("corrupt required response enabled reconciliation", negativeResult)
	}
	still, err := e.db.Gate(ctx)
	if err != nil || still.State != "fenced" || len(still.ActiveClaims) != 1 {
		t.Fatal("corrupt response released the owned claim", still, err)
	}
	if err = os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	sameAttemptID := domain.NewID()
	if err = e.db.Fence(ctx, domain.ExecutionIncident{ID: sameAttemptID, AttemptID: &a.ID, Code: "stop_budget_elapsed", Message: "A second watchdog observation retains this attempt claim.", CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	otherID := domain.NewID()
	if err = e.db.Fence(ctx, domain.ExecutionIncident{ID: otherID, Code: "other_test_blocker", Message: "Retain independent blocker.", CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	positiveInput := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: negativeResult.Incident.Revision, Action: "verify_and_reconcile"}
	positive := e.ReconcileIncident(ctx, watchdogID, positiveInput)
	positiveResult := recoveryEngineResult(t, positive)
	if positiveResult.Incident.State != "reconciled" || positiveResult.Decision.Outcome != "reconciled" {
		t.Fatal("actual verified watchdog incident did not reconcile", positiveResult)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != otherID || len(gate.ActiveClaims) != 1 || len(positiveResult.Incident.Blockers) == 0 {
		t.Fatal("first same-attempt incident released the still-owned claim or hid its blocker", gate, positiveResult, err)
	}
	blockedCheckpoint, err := e.db.Checkpoint(ctx, paused.Checkpoint.ID)
	if err != nil || blockedCheckpoint.State == "paused" || len(blockedCheckpoint.Blockers) == 0 {
		t.Fatal("checkpoint ignored the remaining same-attempt claim", blockedCheckpoint, err)
	}
	secondView, err := e.db.RecoveryIncident(ctx, sameAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: secondView.Revision, Action: "verify_and_reconcile"}
	second := recoveryEngineResult(t, e.ReconcileIncident(ctx, sameAttemptID, secondInput))
	if second.Incident.State != "reconciled" {
		t.Fatal("the last same-attempt incident could not reconcile", second)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != otherID || len(gate.ActiveClaims) != 0 {
		t.Fatal("last same-attempt decision did not release only its claim or preserve the independent blocker", gate, err)
	}
	resolvedCheckpoint, err := e.db.Checkpoint(ctx, paused.Checkpoint.ID)
	if err != nil || resolvedCheckpoint.State != "paused" || len(resolvedCheckpoint.Blockers) != 0 {
		t.Fatal("final claim reconciliation did not atomically refresh the checkpoint", resolvedCheckpoint, err)
	}
	replay := e.ReconcileIncident(ctx, watchdogID, positiveInput)
	replayed := recoveryEngineResult(t, replay)
	if !reflect.DeepEqual(positiveResult, replayed) || !reflect.DeepEqual(positive.Envelope.Receipt, replay.Envelope.Receipt) {
		t.Fatal("exact reconciliation retry changed its receipt or decision", positive, replay)
	}
	after, err := e.db.Attempt(ctx, a.ID)
	if err != nil || after.Revision != settled.Revision || after.Outcome == nil || settled.Outcome == nil || *after.Outcome != *settled.Outcome {
		t.Fatal("reconciliation replayed or rewrote the terminal attempt", after, settled, err)
	}
}

func recoveryEngineResult(t *testing.T, r domain.Result) domain.RecoveryReconciliationResult {
	t.Helper()
	if r.Status != 200 || r.Envelope.Error != nil || r.Envelope.Receipt == nil {
		t.Fatalf("reconciliation did not return durable result: %+v", r)
	}
	b, err := json.Marshal(r.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var result domain.RecoveryReconciliationResult
	if err = json.Unmarshal(b, &result, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	return result
}

// Force a second independent fence cause to fail before durable commit while
// an earlier gate and a different admitted claim exist. No assertion invents
// process cleanup proof or treats a lost persistence attempt as resolved.
func TestRecoveryUnrecordedFenceCauseSurvivesClaimSettlementAndOwnerRestart(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	created := e.db.CreateTask(ctx, e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Keep the claimed fixture.", ExecutionMode: "fake"})
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission, ok := created.Envelope.Data.(domain.Admission)
	if !ok {
		t.Fatal("unexpected admission", created)
	}
	if _, err := e.db.ClaimAttempt(ctx, admission.Attempt.ID, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	e.fence("", "first_retained_cause", "The original durable incident remains unresolved.")
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || len(gate.ActiveClaims) != 1 {
		t.Fatal("missing durable first fence", gate, err)
	}
	firstID := *gate.IncidentID
	e.db.BeforeCommit = func() error { return errors.New("injected failure recording a second cause") }
	e.fence("", "second_unrecorded_cause", "Another distinct cause failed to commit.")
	e.db.BeforeCommit = nil
	e.control.Lock()
	uncertain := e.fenceUncertain
	e.control.Unlock()
	if !uncertain {
		t.Fatal("failed second incident did not retain in-memory uncertainty")
	}
	settlement := domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}
	if _, err = e.db.SettleAttempt(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal(err)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || len(gate.ActiveClaims) != 0 {
		t.Fatal("claim settlement reopened an uncertain gate", gate, err)
	}
	view, err := e.db.RecoveryIncident(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	request := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: view.Revision, Action: "verify_and_reconcile"}
	if result := e.ReconcileIncident(ctx, firstID, request); result.Envelope.Error == nil || result.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal("unrecorded cause allowed reconciliation", result)
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	newOwner := domain.NewID()
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: newOwner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	gate, err = next.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.OwnerInstanceID != newOwner {
		t.Fatal("owner restart reopened the persisted uncertain gate", gate, err)
	}
	if result := next.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Must remain fenced", ExecutionMode: "fake"}); result.Envelope.Error == nil {
		t.Fatal("restart admitted work through unresolved fence", result)
	}
}
