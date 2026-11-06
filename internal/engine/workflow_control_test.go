package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type workflowControlObservation struct {
	Attempt domain.Attempt
	Action  workspace.ApplyAction
	Result  workspace.ApplyResult
	At      time.Time
}

// These barriers observe real native actions; they never replace an action or
// manufacture an effect result. Cleanup releases them before Engine.Close.
type workflowControlBarrier struct {
	entered chan workflowControlObservation
	release chan struct{}
	once    sync.Once
}

func newWorkflowControlBarrier(t *testing.T) *workflowControlBarrier {
	t.Helper()
	b := &workflowControlBarrier{entered: make(chan workflowControlObservation, 1), release: make(chan struct{})}
	t.Cleanup(b.Release)
	return b
}

func (b *workflowControlBarrier) Release() { b.once.Do(func() { close(b.release) }) }

func (b *workflowControlBarrier) Hold(o workflowControlObservation) error {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case b.entered <- o:
	case <-timer.C:
		return errors.New("controlled application barrier was not observed")
	}
	// Deliberately wait for the test's release after Stop or clock expiry. The
	// production action/settlement still receives its original captured context.
	select {
	case <-b.release:
		return nil
	case <-timer.C:
		return errors.New("controlled application barrier was not released")
	}
}

func awaitWorkflowControlBarrier(t *testing.T, e *Engine, taskID string, b *workflowControlBarrier) workflowControlObservation {
	t.Helper()
	select {
	case o := <-b.entered:
		return o
	case <-time.After(35 * time.Second):
		view, viewErr := e.db.Workflow(context.Background(), taskID, domain.WorkflowSelector{})
		task, taskErr := e.db.Task(context.Background(), taskID)
		gate, gateErr := e.db.Gate(context.Background())
		t.Fatalf("real local application did not reach controlled boundary: workflow=%+v err=%v task=%+v err=%v gate=%+v err=%v", view, viewErr, task, taskErr, gate, gateErr)
		return workflowControlObservation{}
	}
}

func workflowControlDocument(t *testing.T) (*Engine, string, string, []byte) {
	t.Helper()
	e, projectID, projectPath := testEngine(t)
	if err := os.Mkdir(filepath.Join(projectPath, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("\ufeffControlled native application\r\nold\r\n東京\r\n")
	if err := os.WriteFile(filepath.Join(projectPath, "docs", "note.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	return e, projectID, projectPath, original
}

func workflowControlDocumentInput(projectID, mode string) domain.StartTaskInput {
	return domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Replace \"old\" with \"new\" in docs/note.md only", ExecutionMode: "fake", FakeScenario: "workflow-document", Workflow: &domain.WorkflowInput{Delivery: &domain.WorkflowDeliveryInput{LocalMode: mode}}}
}

func workflowControlOperation(t *testing.T, e *Engine, taskID string) domain.DeliveryOperation {
	t.Helper()
	page, err := e.db.WorkflowDeliveries(context.Background(), taskID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "local_application" || page.Items[0].Operation == nil || page.NextCursor != nil {
		t.Fatal("expected exactly one real local operation and no acceptance", page, err)
	}
	return *page.Items[0].Operation
}

func assertWorkflowControlKnownSettlement(t *testing.T, e *Engine, taskID, attemptID, outcome string) domain.WorkflowDetail {
	t.Helper()
	a := settled(t, e, attemptID)
	if a.Outcome == nil || *a.Outcome != outcome || a.ExecutorKind != "local_application" || a.LaunchIntent || a.Cleanup.State != "verified" || a.Cleanup.Scope != "not_launched" || a.EvidenceStatus != "complete" || a.Cleanup.IncidentID != nil {
		t.Fatal("known no-process application became uncertain or lost its actual outcome", a)
	}
	view, err := e.db.Workflow(context.Background(), taskID, domain.WorkflowSelector{})
	if err != nil || view.Workflow == nil || view.Workflow.State != outcome || view.Acceptance != nil || view.Workflow.Completion != nil {
		t.Fatal("local control produced false workflow completion", view, err)
	}
	gate, err := e.db.Gate(context.Background())
	e.control.Lock()
	fenced := e.fenced
	e.control.Unlock()
	if err != nil || gate.State != "open" || gate.ActiveAttemptID != nil || gate.IncidentID != nil || fenced {
		t.Fatal("a known controlled result falsely retained uncertainty/ownership", gate, fenced, err)
	}
	return view
}

func TestWorkflowControlStopBeforeApplicationReadyHasNoDestinationEffectOrFalseUncertainty(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath, original := workflowControlDocument(t)
	b := newWorkflowControlBarrier(t)
	var actions atomic.Int64
	e.config.BeforeApplicationReady = func(_ context.Context, a domain.Attempt) error {
		return b.Hold(workflowControlObservation{Attempt: a, At: time.Now()})
	}
	e.config.BeforeApplicationAction = func(context.Context, domain.Attempt, workspace.ApplyAction) { actions.Add(1) }
	admitted := decodeAdmission(t, e.StartTask(ctx, workflowControlDocumentInput(projectID, "apply_verified_local")))
	o := awaitWorkflowControlBarrier(t, e, admitted.Task.ID, b)
	before := workflowControlOperation(t, e, admitted.Task.ID)
	if before.AttemptID != o.Attempt.ID || before.Status != "preparing" || before.LastEffectSequence != "0" || before.PlanArtifactID != "" || before.BeforeDestination != nil {
		t.Fatal("pre-Ready boundary already granted or rebound preparation", before, o.Attempt)
	}
	requestID := domain.NewID()
	if stopped := e.Stop(ctx, o.Attempt.ID, requestID); stopped.Envelope.Error != nil {
		t.Fatal(stopped)
	}
	b.Release()
	assertWorkflowControlKnownSettlement(t, e, admitted.Task.ID, o.Attempt.ID, "cancelled")
	after := workflowControlOperation(t, e, admitted.Task.ID)
	if after.Status != "interrupted" || after.LastEffectSequence != "0" || after.PlanArtifactID != "" || after.BeforeDestination != nil || after.AfterDestination != nil || after.IncidentID != nil || actions.Load() != 0 {
		t.Fatal("rejected Ready fabricated effects or an unacknowledged preparation binding", after, actions.Load())
	}
	a, err := e.db.Attempt(ctx, o.Attempt.ID)
	if err != nil || a.Stop == nil || a.Stop.RequestID != requestID || a.PrimaryFailure != nil {
		t.Fatal("later preparation rejection replaced the accepted Stop", a, err)
	}
	actual, err := os.ReadFile(filepath.Join(projectPath, "docs", "note.md"))
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("Stop before Ready changed original destination bytes", actual, err)
	}
}

func TestWorkflowControlStopAfterQuarantineRetainsOriginalWithoutPublicationPermit(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath, original := workflowControlDocument(t)
	b := newWorkflowControlBarrier(t)
	var publications atomic.Int64
	e.config.BeforeApplicationAction = func(_ context.Context, _ domain.Attempt, action workspace.ApplyAction) {
		if action.Action == "publish_after" {
			publications.Add(1)
		}
	}
	e.config.AfterApplicationAction = func(_ context.Context, a domain.Attempt, action workspace.ApplyAction, result workspace.ApplyResult) {
		if action.Action == "quarantine_before" {
			_ = b.Hold(workflowControlObservation{Attempt: a, Action: action, Result: result, At: time.Now()})
		}
	}
	admitted := decodeAdmission(t, e.StartTask(ctx, workflowControlDocumentInput(projectID, "apply_verified_local")))
	o := awaitWorkflowControlBarrier(t, e, admitted.Task.ID, b)
	op := workflowControlOperation(t, e, admitted.Task.ID)
	if !o.Result.Known || o.Result.State != "applied" || o.Action.Action != "quarantine_before" || o.Action.QuarantineRef == "" || o.Result.ObservedEvidence == nil || op.LastEffectSequence != "2" {
		t.Fatal("barrier did not observe actual quarantine and its durable intent/result", o, op)
	}
	quarantine := filepath.Join(e.lock.Root(), filepath.FromSlash(o.Action.QuarantineRef))
	if stopped := e.Stop(ctx, o.Attempt.ID, domain.NewID()); stopped.Envelope.Error != nil {
		t.Fatal(stopped)
	}
	b.Release()
	assertWorkflowControlKnownSettlement(t, e, admitted.Task.ID, o.Attempt.ID, "cancelled")
	op = workflowControlOperation(t, e, admitted.Task.ID)
	if op.Status != "partial" || op.LastEffectSequence != "2" || op.AfterDestination != nil || op.IncidentID != nil || publications.Load() != 0 {
		t.Fatal("Stop after quarantine admitted publication or hid the retained partial effect", op, publications.Load())
	}
	// Application.Close precedes settlement. Read the actual retained original
	// only after its native exclusive handle has closed.
	retained, err := os.ReadFile(quarantine)
	if err != nil || !bytes.Equal(retained, original) {
		t.Fatal("settlement removed or rewrote the actual quarantined original", retained, err)
	}
	if _, err = os.Stat(filepath.Join(projectPath, "docs", "note.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a separate publication ran after accepted Stop", err)
	}
}

func TestWorkflowControlAppliedPublicationRetainedWhenRequiredSettlementCrossesCommonDeadline(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath, original := workflowControlDocument(t)
	prepared := observeAcceptancePreparations(e)
	b := newWorkflowControlBarrier(t)
	var actions atomic.Int64
	e.config.BeforeApplicationAction = func(context.Context, domain.Attempt, workspace.ApplyAction) { actions.Add(1) }
	e.config.AfterApplicationAction = func(_ context.Context, a domain.Attempt, action workspace.ApplyAction, result workspace.ApplyResult) {
		if action.Action == "publish_after" {
			_ = b.Hold(workflowControlObservation{Attempt: a, Action: action, Result: result, At: time.Now().UTC()})
		}
	}
	window, timeout := 15, 60
	input := workflowControlDocumentInput(projectID, "apply_verified_local")
	input.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{WindowSeconds: &window}}
	input.TimeoutSeconds = &timeout
	admitted := decodeAdmission(t, e.StartTask(ctx, input))
	o := awaitWorkflowControlBarrier(t, e, admitted.Task.ID, b)
	before, err := e.db.TaskBudget(ctx, admitted.Task.ID)
	if err != nil || before.Budget == nil || before.Budget.CurrentWindow.DeadlineAt == nil {
		t.Fatal(before, err)
	}
	deadline, invalid := domain.ParseRoutingTime(*before.Budget.CurrentWindow.DeadlineAt)
	if invalid != nil || !o.At.Before(deadline) || !o.Result.Known || o.Result.State != "applied" || o.Result.Identity == "" || o.Result.ObservedEvidence == nil {
		t.Fatal("actual final publication did not complete before the captured deadline", o, deadline, invalid)
	}
	op := workflowControlOperation(t, e, admitted.Task.ID)
	if op.LastEffectSequence != "4" || actions.Load() != 2 {
		t.Fatal("final replacement action/result was not already durably recorded", op, actions.Load())
	}
	expected := bytes.ReplaceAll(original, []byte("old"), []byte("new"))
	// The native application still owns its exclusive destination handle. Read
	// its actual observed destination_file artifact, not the intended plan or
	// the destination path, until Application.Close has completed.
	observed, err := e.evidence.VerifyClosed(ctx, o.Result.ObservedEvidence.ID)
	if err != nil || observed.Kind != "destination_file" || observed.AttemptID != o.Attempt.ID || !observed.Closed || observed.Completeness != "complete" || observed.SHA256 == nil || *observed.SHA256 != o.Result.SHA256 {
		t.Fatal("actual publication lacks a complete matching observed artifact", observed, o.Result, err)
	}
	observedBytes, err := e.readResponse(ctx, observed)
	if err != nil || !bytes.Equal(observedBytes, expected) {
		t.Fatal("barrier did not retain the actual published bytes", observedBytes, err)
	}
	// Only required settlement is delayed. This does not claim the native rename
	// physically completed after expiry, and no captured timestamp is rewritten.
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for !time.Now().UTC().After(deadline) {
		select {
		case <-waitCtx.Done():
			t.Fatal("captured common window did not expire within bounded observation", waitCtx.Err())
		case <-ticker.C:
		}
	}
	b.Release()
	assertWorkflowControlKnownSettlement(t, e, admitted.Task.ID, o.Attempt.ID, "failed")
	a, err := e.db.Attempt(ctx, o.Attempt.ID)
	if err != nil || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "deadline_exceeded" || a.Stop != nil {
		t.Fatal("expired required settlement became successful or a fabricated Stop", a, err)
	}
	op = workflowControlOperation(t, e, admitted.Task.ID)
	if op.Status != "applied" || op.LastEffectSequence != "4" || op.AfterDestination == nil || op.IncidentID != nil || actions.Load() != 2 {
		t.Fatal("failed task rewrote the acknowledged applied result or ran another action", op, actions.Load())
	}
	actual, err := os.ReadFile(filepath.Join(projectPath, "docs", "note.md"))
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatal("deadline settlement reverted actual published bytes", actual, err)
	}
	after, err := e.db.TaskBudget(ctx, admitted.Task.ID)
	if err != nil || !reflect.DeepEqual(before, after) || after.Budget.ConsumedAdmissions != "1" || prepared.Load() != 1 {
		t.Fatal("expiry settlement refilled budget or launched a model unit", before, after, prepared.Load(), err)
	}
}

func setWorkflowControlMemoryFence(e *Engine, fenced bool) {
	e.control.Lock()
	e.fenced = fenced // Internal memory-only fault injection; do not persist a DB fence.
	e.control.Unlock()
}

func assertWorkflowControlDatabaseGateOpen(t *testing.T, e *Engine) {
	t.Helper()
	gate, err := e.db.Gate(context.Background())
	if err != nil || gate.State != "open" || gate.IncidentID != nil {
		t.Fatal("fixture accidentally persisted a fence instead of testing memory-only refusal", gate, err)
	}
}

func TestWorkflowControlMemoryFenceBlocksCommandBeforeOwnedPreparation(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath := testEngine(t)
	stopFixtureScheduler(t, e)
	prepared := observeAcceptancePreparations(e)
	if err := os.WriteFile(filepath.Join(projectPath, fixture.WorkflowFile), []byte(fixture.WorkflowValid), 0600); err != nil {
		t.Fatal(err)
	}
	admitted := decodeAdmission(t, e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Controlled actual local command dispatch guard.", ExecutionMode: "fake", Workflow: &domain.WorkflowInput{Mode: "author_only"}}))
	a, err := e.db.ClaimAttempt(ctx, admitted.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.workspace.Capture(ctx, workspace.Request{AttemptID: a.ID, ProjectPath: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	_, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.registerWorkflowSnapshot(ctx, a, snapshot, identity); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	program, err := workflowProgram(executable)
	if err != nil {
		t.Fatal(err)
	}
	command := domain.CommandDefinition{Executable: program.Executable, PhysicalIdentity: program.PhysicalIdentity, Arguments: []string{"__fixture", "check", "--case", "workflow-validation"}, RelativeWorkingDirectory: ".", TimeoutSeconds: 10, SuccessExitCodes: []int{0}}
	setWorkflowControlMemoryFence(e, true)
	assertWorkflowControlDatabaseGateOpen(t, e)
	observed, err := e.executeWorkflowCommand(ctx, a, snapshot, command)
	var problem *domain.Error
	if !errors.As(err, &problem) || problem.Code != "dispatch_fenced" || observed.Started || prepared.Load() != 0 || observed.Settlement.RootTerminal || observed.Settlement.ExitCode != nil {
		t.Fatal("memory-only fence reached native preparation or fabricated a command observation", observed, err, prepared.Load())
	}
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || current.LaunchIntent || current.StartedAt != nil || current.PrimaryFailure != nil {
		t.Fatal("fenced command wrote a launch observation", current, err)
	}
	assertWorkflowControlDatabaseGateOpen(t, e)
	if _, err = e.db.SettleAttempt(ctx, a.ID, domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); err != nil {
		t.Fatal(err)
	}
}

func runWorkflowControlAttempt(t *testing.T, e *Engine, attemptID string) domain.Attempt {
	t.Helper()
	ctx := context.Background()
	a, err := e.db.ClaimAttempt(ctx, attemptID, e.config.OwnerInstanceID)
	if err != nil || a.Phase != "preparing" {
		t.Fatal("manual fixture did not acquire its real owned claim", a, err)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" {
		t.Fatal("real manually dispatched fixture did not qualify", current, err)
	}
	return current
}

func workflowControlAdvanceInput(t *testing.T, e *Engine, taskID string) domain.WorkflowAdvanceInput {
	t.Helper()
	ctx := context.Background()
	view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	task, err := e.db.Task(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	return domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Task.CurrentVersion, WorkflowRevision: view.Workflow.Revision}
}

func TestWorkflowControlMemoryFenceBlocksOtherwiseQualifiedCandidateAcceptance(t *testing.T) {
	ctx := context.Background()
	e, projectID, _, _ := workflowControlDocument(t)
	stopFixtureScheduler(t, e)
	prepared := observeAcceptancePreparations(e)
	admitted := decodeAdmission(t, e.StartTask(ctx, workflowControlDocumentInput(projectID, "review_only")))
	runWorkflowControlAttempt(t, e, admitted.Attempt.ID)
	advanced := e.advanceCurrentWorkflow(ctx, admitted.Task.ID, workflowControlAdvanceInput(t, e, admitted.Task.ID), nil)
	if advanced.Status != 201 || advanced.Envelope.Error != nil {
		t.Fatal("real direct candidate did not admit its builtin proof", advanced)
	}
	current, err := e.db.Task(ctx, admitted.Task.ID)
	if err != nil || current.LatestAttempt == nil || current.LatestAttempt.Role != "check" {
		t.Fatal(current, err)
	}
	runWorkflowControlAttempt(t, e, current.LatestAttempt.ID)
	before, err := e.db.Workflow(ctx, admitted.Task.ID, domain.WorkflowSelector{})
	if err != nil || before.Workflow == nil || before.Acceptance != nil {
		t.Fatal(before, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, admitted.Task.ID, *before.Workflow)
	if err != nil || progress.decision.Action != "accept" || progress.subject.Binding == nil || len(progress.checkUses) != 1 {
		t.Fatal("memory-fence test lacks genuinely qualified retained candidate facts", progress, err)
	}
	budgetBefore, err := e.db.TaskBudget(ctx, admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	in := workflowControlAdvanceInput(t, e, admitted.Task.ID)
	setWorkflowControlMemoryFence(e, true)
	assertWorkflowControlDatabaseGateOpen(t, e)
	refused := e.advanceCurrentWorkflow(ctx, admitted.Task.ID, in, nil)
	if refused.Envelope.Error == nil || refused.Envelope.Error.Code != "dispatch_fenced" || refused.Envelope.Receipt != nil {
		t.Fatal("memory-only fence accepted a newly qualified candidate", refused)
	}
	after, err := e.db.Workflow(ctx, admitted.Task.ID, domain.WorkflowSelector{})
	budgetAfter, budgetErr := e.db.TaskBudget(ctx, admitted.Task.ID)
	if err != nil || budgetErr != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(budgetBefore, budgetAfter) || prepared.Load() != 1 {
		t.Fatal("fenced candidate acceptance changed evidence, budget or admitted work", before, after, budgetBefore, budgetAfter, prepared.Load(), err, budgetErr)
	}
	assertWorkflowControlDatabaseGateOpen(t, e)
	// Clearing only the injected memory fault proves the same retained candidate
	// and request were valid; no SQL fixture manufactures qualification.
	setWorkflowControlMemoryFence(e, false)
	accepted := e.advanceCurrentWorkflow(ctx, admitted.Task.ID, in, nil)
	if accepted.Status != 200 || accepted.Envelope.Error != nil {
		t.Fatal("same real candidate was not otherwise acceptable", accepted)
	}
	completed, err := e.db.Workflow(ctx, admitted.Task.ID, domain.WorkflowSelector{})
	if err != nil || completed.Workflow.State != "completed" || completed.Acceptance == nil || prepared.Load() != 1 {
		t.Fatal(completed, prepared.Load(), err)
	}
}

func TestWorkflowControlMemoryFenceBlocksOtherwiseQualifiedResponseAcceptance(t *testing.T) {
	for _, mode := range []string{"auto", "author_only"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			e, projectID, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			prepared := observeAcceptancePreparations(e)
			input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Controlled plain response acknowledgement without modifying project files.", ExecutionMode: "fake", FakeScenario: "response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}
			if mode == "author_only" {
				input.Workflow = &domain.WorkflowInput{Mode: "author_only"}
			}
			admitted := decodeAdmission(t, e.StartTask(ctx, input))
			a := runWorkflowControlAttempt(t, e, admitted.Attempt.ID)
			before, err := e.db.Workflow(ctx, admitted.Task.ID, domain.WorkflowSelector{})
			if err != nil || mode == "auto" && (before.Workflow == nil || before.Workflow.NextAction != "response_acceptance_pending") || mode == "author_only" && before.Workflow != nil {
				t.Fatal("response fixture lacks its real intended authority", before, err)
			}
			budgetBefore, err := e.db.TaskBudget(ctx, admitted.Task.ID)
			if err != nil {
				t.Fatal(err)
			}
			in := exactResponseAcceptanceInput(t, e, admitted.Task.ID, a.ID)
			setWorkflowControlMemoryFence(e, true)
			assertWorkflowControlDatabaseGateOpen(t, e)
			refused := e.AcceptResponse(ctx, admitted.Task.ID, in)
			if refused.Envelope.Error == nil || refused.Envelope.Error.Code != "dispatch_fenced" || refused.Envelope.Receipt != nil {
				t.Fatal("memory-only fence acknowledged a new response", refused)
			}
			after, err := e.db.Workflow(ctx, admitted.Task.ID, domain.WorkflowSelector{})
			budgetAfter, budgetErr := e.db.TaskBudget(ctx, admitted.Task.ID)
			if err != nil || budgetErr != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(budgetBefore, budgetAfter) || prepared.Load() != 1 {
				t.Fatal("fenced response acknowledgement changed task authority or launched work", before, after, budgetBefore, budgetAfter, prepared.Load(), err, budgetErr)
			}
			assertWorkflowControlDatabaseGateOpen(t, e)
			setWorkflowControlMemoryFence(e, false)
			accepted := e.AcceptResponse(ctx, admitted.Task.ID, in)
			assertOriginalResponseAcceptanceWire(t, accepted, in)
			assertAcceptanceReplay(t, e, admitted.Task.ID, in, accepted)
		})
	}
}
