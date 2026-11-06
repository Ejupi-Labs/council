package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// All processes are the existing fixed local test agents. This counter observes
// actual preparation; it substitutes no process outcome, file proof or telemetry.
func observeAcceptancePreparations(e *Engine) *atomic.Int64 {
	count := &atomic.Int64{}
	e.prepareGate.Lock()
	prior := e.config.Prepare
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		count.Add(1)
		if prior != nil {
			return prior(ctx, spec)
		}
		return process.Prepare(ctx, spec)
	}
	e.prepareGate.Unlock()
	return count
}

func waitAcceptanceResponseReady(t *testing.T, e *Engine, taskID string) domain.WorkflowDetail {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
		if err != nil {
			t.Fatal(err)
		}
		detail, err := e.db.Task(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Workflow != nil && view.Workflow.NextAction == "response_acceptance_pending" && view.Workflow.NextOwner == "user" && view.Workflow.SelectedResponseBinding != nil && detail.LatestAttempt != nil && detail.LatestAttempt.Phase == "settled" && detail.LatestAttempt.Cleanup.State == "verified" && detail.LatestAttempt.Outcome != nil && *detail.LatestAttempt.Outcome == "succeeded" {
			return view
		}
		gate, err := e.db.Gate(ctx)
		if err != nil || gate.State == "fenced" {
			t.Fatalf("real response workflow fenced: view=%+v attempt=%+v gate=%+v err=%v", view, detail.LatestAttempt, gate, err)
		}
		if view.Workflow != nil && (view.Workflow.State == "failed" || view.Workflow.State == "cancelled" || view.Workflow.State == "interrupted") {
			t.Fatalf("real response workflow did not qualify: view=%+v attempt=%+v", view, detail.LatestAttempt)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("real response did not reach explicit user acceptance: view=%+v attempt=%+v", view, detail.LatestAttempt)
		case <-ticker.C:
		}
	}
}

func exactResponseAcceptanceInput(t *testing.T, e *Engine, taskID, attemptID string) domain.ResponseAcceptanceInput {
	t.Helper()
	ctx := context.Background()
	detail, err := e.db.Task(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.db.Attempt(ctx, attemptID)
	if err != nil || attempt.ResponseArtifactID == nil {
		t.Fatal("selected real attempt lacks final response", attempt, err)
	}
	meta, err := e.evidence.VerifyClosed(ctx, *attempt.ResponseArtifactID)
	if err != nil || meta.SHA256 == nil {
		t.Fatal(meta, err)
	}
	return domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: detail.Task.CurrentVersion, AttemptID: attemptID, ResponseArtifactID: meta.ID, ResponseSHA256: *meta.SHA256, Confirmation: "reviewed_response_and_conditions"}
}

func assertOriginalResponseAcceptanceWire(t *testing.T, result domain.Result, in domain.ResponseAcceptanceInput) domain.ResponseAcceptance {
	t.Helper()
	if result.Status != 200 || result.Envelope.Error != nil || result.Envelope.Receipt == nil || result.Envelope.Receipt.Operation != "tasks.response.accept" || result.Envelope.Receipt.RequestID != in.RequestID {
		t.Fatal("existing acceptance envelope/receipt changed", result)
	}
	raw, err := json.Marshal(result.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil || len(data) != 2 || data["task"] == nil || data["acceptance"] == nil {
		t.Fatal("workflow metadata expanded existing acknowledgement data shape", string(raw))
	}
	var decoded struct {
		Task       domain.TaskSummary        `json:"task"`
		Acceptance domain.ResponseAcceptance `json:"acceptance"`
	}
	if json.Unmarshal(raw, &decoded, json.RejectUnknownMembers(true)) != nil {
		t.Fatal("existing typed acknowledgement changed", string(raw))
	}
	acceptanceRaw, err := json.Marshal(data["acceptance"])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if json.Unmarshal(acceptanceRaw, &fields) != nil || len(fields) != 10 {
		t.Fatal("original ResponseAcceptance member set changed", string(acceptanceRaw))
	}
	for _, key := range []string{"id", "requestId", "taskId", "taskVersion", "attemptId", "responseArtifactId", "responseSha256", "basis", "confirmation", "acceptedAt"} {
		if _, ok := fields[key]; !ok {
			t.Fatal("missing original acceptance field", key)
		}
	}
	a := decoded.Acceptance
	if !domain.ValidID(a.ID) || a.RequestID != in.RequestID || a.TaskVersion != in.TaskVersion || a.AttemptID != in.AttemptID || a.ResponseArtifactID != in.ResponseArtifactID || a.ResponseSHA256 != in.ResponseSHA256 || a.Basis != "user_review" || a.Confirmation != in.Confirmation {
		t.Fatal("selected producer/bytes or existing acceptance basis changed", a, in)
	}
	return a
}

func assertAcceptanceReplay(t *testing.T, e *Engine, taskID string, in domain.ResponseAcceptanceInput, first domain.Result) {
	t.Helper()
	replay := e.AcceptResponse(context.Background(), taskID, in)
	if replay.Status != first.Status {
		t.Fatal("replay status changed", replay, first)
	}
	var before, after any
	firstRaw, _ := json.Marshal(first.Envelope)
	replayRaw, _ := json.Marshal(replay.Envelope)
	if json.Unmarshal(firstRaw, &before) != nil || json.Unmarshal(replayRaw, &after) != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("original acceptance receipt/data changed on replay", string(firstRaw), string(replayRaw))
	}
}

func TestWorkflowAcceptancePlainAutoAfterWindowExpiryReplaysExistingWire(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	prepared := observeAcceptancePreparations(e)
	preference, err := e.db.DeliveryPreference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	saved := e.db.UpdateDeliveryPreference(ctx, domain.DeliveryPreferenceInput{RequestID: domain.NewID(), ExpectedRevision: preference.Revision, LocalMode: "apply_verified_local"})
	if saved.Envelope.Error != nil {
		t.Fatal(saved)
	}
	// Fifteen seconds leaves a real native fixture ample launch/cleanup time.
	// We poll the captured wall clock; no SQL or artificial deadline mutation is used.
	window := 15
	timeout := 60
	result := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "CONTROLLED LOCAL RESPONSE TEST: explain the retained project without changing files.", ExecutionMode: "fake", FakeScenario: "response", TimeoutSeconds: &timeout, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}, Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{WindowSeconds: &window}}})
	admission := decodeAdmission(t, result)
	if admission.Workflow == nil || admission.Workflow.Authority.Delivery.LocalMode != "apply_verified_local" {
		t.Fatal("omitted Auto did not capture saved prospective scope", admission)
	}
	ready := waitAcceptanceResponseReady(t, e, admission.Task.ID)
	producer := ready.Workflow.SelectedResponseBinding.ProducerAttemptID
	if producer != admission.Attempt.ID || ready.Workflow.Completion != nil || ready.Acceptance != nil || ready.Workflow.HasTaskDelta || ready.Workflow.HasTaskIndexDelta {
		t.Fatal("plain response invented a change or automatic completion", ready)
	}
	if body := interopArtifactBytes(t, e, ready.Workflow.SelectedResponseBinding.ResponseArtifactID); string(body) != fixture.ResponseText {
		t.Fatalf("real original response bytes changed: %q", body)
	}
	steps, err := e.db.WorkflowSteps(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(steps.Items) != 1 || steps.Items[0].Role != "author" {
		t.Fatal("plain response manufactured supporting work", steps, err)
	}
	before, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil || before.Budget == nil || before.Budget.CurrentWindow.DeadlineAt == nil || before.Budget.ConsumedAdmissions != "1" {
		t.Fatal(before, err)
	}
	capturedDeadline, invalid := domain.ParseRoutingTime(*before.Budget.CurrentWindow.DeadlineAt)
	if invalid != nil {
		t.Fatal(invalid)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for !time.Now().UTC().After(capturedDeadline) {
		select {
		case <-waitCtx.Done():
			t.Fatal("captured common window did not expire within bounded poll", waitCtx.Err())
		case <-ticker.C:
		}
	}
	in := exactResponseAcceptanceInput(t, e, admission.Task.ID, producer)
	accepted := e.AcceptResponse(ctx, admission.Task.ID, in)
	record := assertOriginalResponseAcceptanceWire(t, accepted, in)
	assertAcceptanceReplay(t, e, admission.Task.ID, in, accepted)
	completed, err := e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
	if err != nil || completed.Workflow.State != "completed" || completed.Acceptance != nil || completed.Workflow.Completion == nil || completed.Workflow.Completion.Kind != "response_only_user_review" || completed.Workflow.Completion.ResponseAcceptanceID != record.ID {
		t.Fatal("response acceptance did not use separate exact completion branch", completed, err)
	}
	after, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil || !reflect.DeepEqual(before, after) || prepared.Load() != 1 {
		t.Fatal("human acceptance/replay reopened time, refilled authority or launched work", before, after, prepared.Load(), err)
	}
	deliveries, err := e.db.WorkflowDeliveries(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(deliveries.Items) != 0 {
		t.Fatal("saved local-apply scope invented no-op integration for a plain response", deliveries, err)
	}
}

func TestWorkflowAcceptanceRequiredReviewRetainsAuthorAndRejectsReviewerRawResult(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	prepared := observeAcceptancePreparations(e)
	result := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "CONTROLLED LOCAL RESPONSE TEST: return the exact fixture explanation without modifying project files; independently review that response.", ExecutionMode: "fake", FakeScenario: "response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only", RequiredEvidence: []string{"independent_review"}}, Workflow: &domain.WorkflowInput{Review: "required"}})
	admission := decodeAdmission(t, result)
	ready := waitAcceptanceResponseReady(t, e, admission.Task.ID)
	binding := ready.Workflow.SelectedResponseBinding
	if binding == nil || binding.ProducerAttemptID != admission.Attempt.ID {
		t.Fatal("supporting review replaced selected author response", ready)
	}
	steps, err := e.db.WorkflowSteps(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(steps.Items) != 2 || steps.Items[0].Role != "author" || steps.Items[1].Role != "review" || steps.Items[1].AttemptID == nil {
		t.Fatal("required response review did not retain two actual native roles", steps, err)
	}
	reviewerID := *steps.Items[1].AttemptID
	detail, err := e.db.Task(ctx, admission.Task.ID)
	if err != nil || detail.LatestAttempt == nil || detail.LatestAttempt.ID != reviewerID || reviewerID == binding.ProducerAttemptID {
		t.Fatal("test did not reach a distinct latest supporting reviewer", detail, err)
	}
	reviews, err := e.db.WorkflowReviews(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(reviews.Items) != 1 || reviews.Items[0].Outcome != "approved" || reviews.Items[0].ResponseBinding == nil || !reflect.DeepEqual(*reviews.Items[0].ResponseBinding, *binding) || len(reviews.Items[0].SuppliedEvidence) == 0 || len(reviews.Items[0].Report.InspectedEvidence) == 0 {
		t.Fatal("real review lacks supplied/read response subject proof", reviews, err)
	}
	wrong := exactResponseAcceptanceInput(t, e, admission.Task.ID, reviewerID)
	var report domain.ReviewReport
	if json.Unmarshal(interopArtifactBytes(t, e, wrong.ResponseArtifactID), &report) != nil || report.Subject.ID != binding.ResponseArtifactID {
		t.Fatal("selected reviewer artifact is not its actual structured supporting result", report)
	}
	refused := e.AcceptResponse(ctx, admission.Task.ID, wrong)
	if refused.Envelope.Error == nil || refused.Envelope.Receipt != nil {
		t.Fatal("reviewer raw result was accepted as the user's answer", refused)
	}
	unchanged, err := e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
	if err != nil || unchanged.Workflow.Completion != nil || unchanged.Workflow.State == "completed" || unchanged.Workflow.SelectedResponseBinding.ProducerAttemptID != admission.Attempt.ID {
		t.Fatal("refused reviewer acceptance changed selected answer or completed task", unchanged, err)
	}
	in := exactResponseAcceptanceInput(t, e, admission.Task.ID, binding.ProducerAttemptID)
	if body := interopArtifactBytes(t, e, in.ResponseArtifactID); string(body) != fixture.ResponseText {
		t.Fatalf("deliverable became the reviewer report: %q", body)
	}
	budgetBefore, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	accepted := e.AcceptResponse(ctx, admission.Task.ID, in)
	record := assertOriginalResponseAcceptanceWire(t, accepted, in)
	assertAcceptanceReplay(t, e, admission.Task.ID, in, accepted)
	completed, err := e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
	if err != nil || completed.Workflow.Completion == nil || completed.Workflow.Completion.Kind != "response_only_user_review_with_evidence" || completed.Workflow.Completion.ResponseBinding.ProducerAttemptID != admission.Attempt.ID || completed.Workflow.Completion.ResponseAcceptanceID != record.ID || len(completed.Workflow.Completion.ReviewUses) != 1 || completed.Workflow.Completion.ReviewUses[0].EvidenceID != reviews.Items[0].ID {
		t.Fatal("required evidence or producer lineage lost during user acknowledgement", completed, err)
	}
	budgetAfter, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil || !reflect.DeepEqual(budgetBefore, budgetAfter) || budgetAfter.Budget == nil || budgetAfter.Budget.ConsumedAdmissions != "2" || prepared.Load() != 2 {
		t.Fatal("acceptance/replay admitted work or changed finite budget", budgetBefore, budgetAfter, prepared.Load(), err)
	}
}

func TestWorkflowAcceptanceStandaloneKeepsOriginalScopeAndEnvelope(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	admission := admit(t, e, project, "response")
	attempt := settled(t, e, admission.Attempt.ID)
	view, err := e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
	if err != nil || view.Workflow != nil || attempt.WorkflowID != "" {
		t.Fatal("author_only compatibility invented workflow authority", view, attempt, err)
	}
	in := exactResponseAcceptanceInput(t, e, admission.Task.ID, attempt.ID)
	accepted := e.AcceptResponse(ctx, admission.Task.ID, in)
	assertOriginalResponseAcceptanceWire(t, accepted, in)
	assertAcceptanceReplay(t, e, admission.Task.ID, in, accepted)
	after, err := e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
	if err != nil || after.Workflow != nil {
		t.Fatal("old standalone acknowledgement was reinterpreted as new workflow completion", after, err)
	}
}
