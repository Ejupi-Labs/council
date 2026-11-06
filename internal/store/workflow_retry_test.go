package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"strings"
	"testing"
)

// Controlled DAO metadata only: these fixtures do not claim an actual model
// review, process, file inspection or native provider qualification.
func workflowBlockedRetryFixture(t *testing.T, role string) (*Store, string, domain.Task, domain.Workflow, domain.WorkflowStep, domain.WorkflowStepAdmissionFacts) {
	t.Helper()
	ctx := context.Background()
	s := testStore(t)
	a, owner, author := workflowAuthorSettlementFixture(t, s, func(in *domain.StartTaskInput, _ *domain.WorkflowAdmissionFacts) { in.Workflow.Review = "required" })
	var w domain.Workflow
	var current domain.Attempt
	if role == "author" {
		t.Fatal("use the never-prepared author fixture")
	}
	w, e := s.SettleWorkflowStep(ctx, a.Attempt.ID, author)
	if e != nil {
		t.Fatal(e)
	}
	step, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil {
		t.Fatal(e)
	}
	admitNext := func(role string, cycle int) domain.WorkflowStepAdmission {
		t.Helper()
		input := w.SelectedCandidateID
		key, invalid := domain.WorkflowStageKey(role, cycle, "", *input)
		if invalid != nil {
			t.Fatal(invalid)
		}
		plan := domain.WorkflowStagePlan{Role: role, ExecutorKind: "native", Cycle: cycle, LogicalKey: key, InputCandidateID: input, PredecessorStepID: &step.ID}
		detail, e := s.Task(ctx, a.Task.ID)
		if e != nil {
			t.Fatal(e)
		}
		next, e := s.ContinueWorkflow(ctx, owner, step.ID, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: detail.Task.Revision, ExpectedWorkflowRevision: w.Revision, Routing: wrapperRoutingFacts("fake")})
		if e != nil || next == nil {
			t.Fatal(next, e)
		}
		return *next
	}
	review := admitNext("review", 0)
	w = review.Workflow
	current = review.Attempt
	step = review.Step
	if role == "correction" {
		launchFixture(t, s, domain.Admission{Attempt: current}, owner)
		raw := closedArtifact(t, s, current.ID, "review_result")
		if _, e = s.RecordAttempt(ctx, current.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &raw.ID}); e != nil {
			t.Fatal(e)
		}
		app := domain.Applicability{ScopeKind: "whole_candidate", Paths: []string{}, ConditionKeys: []string{"task-instructions"}, DefinitionSHA256: strings.Repeat("b", 64), BasisSHA256: strings.Repeat("c", 64)}
		supplied := []domain.ReviewInspection{{Ref: "candidate-manifest", Scope: "whole_candidate", Paths: []string{}}}
		finding := domain.ReviewFinding{Key: "controlled-blocker", Severity: "blocking", ConditionKeys: []string{"task-instructions"}, Locations: []domain.ReviewLocation{}, Summary: "Controlled metadata-only blocking review.", EvidenceRefs: []string{"candidate-manifest"}}
		report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: domain.ReviewSubject{Kind: "candidate", ID: author.CandidateBinding.CandidateID, SHA256: author.CandidateBinding.ManifestSHA256}, ConditionsSHA256: w.Authority.ConditionsSHA256, Outcome: "changes_required", InspectedEvidence: supplied, Findings: []domain.ReviewFinding{finding}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
		record := domain.ReviewRecord{ID: domain.NewID(), WorkflowID: w.ID, StepID: step.ID, AttemptID: current.ID, CandidateBinding: author.CandidateBinding, ReviewContract: domain.ReviewContract, ResultArtifactID: raw.ID, ResultSHA256: *raw.SHA256, Applicability: app, Report: report, SuppliedEvidence: supplied, Outcome: "changes_required", CreatedAt: domain.Now()}
		record.Findings = []domain.WorkflowFinding{{ID: domain.NewID(), ModelKey: finding.Key, ReviewID: record.ID, Finding: finding}}
		proofs := append(append([]domain.VerifiedResponse{}, author.VerifiedArtifacts...), domain.VerifiedResponse{ArtifactID: raw.ID, SHA256: *raw.SHA256, ByteLength: raw.ByteLength, PhysicalIdentity: raw.PhysicalIdentity})
		detail, e := s.Task(ctx, a.Task.ID)
		if e != nil {
			t.Fatal(e)
		}
		exit := 0
		w, e = s.SettleWorkflowStep(ctx, current.ID, domain.WorkflowStepFacts{StepID: step.ID, OwnerInstanceID: owner, TaskRevision: detail.Task.Revision, WorkflowRevision: w.Revision, CandidateBinding: author.CandidateBinding, Applicability: &app, Report: &record, VerifiedArtifacts: proofs, Settlement: domain.Settlement{Outcome: "succeeded", RootTerminal: true, InputComplete: true, ExitCode: &exit, UsableOutput: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"}})
		if e != nil {
			t.Fatal(e)
		}
		step, e = s.WorkflowStep(ctx, step.ID)
		if e != nil {
			t.Fatal(e)
		}
		correction := admitNext("correction", 1)
		w, current, step = correction.Workflow, correction.Attempt, correction.Step
	}
	no := false
	disabled := s.UpdateRoutingContext(ctx, current.ConnectionID, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
	if disabled.Status != 200 {
		t.Fatal(disabled)
	}
	blocked, e := s.DispatchRouting(ctx, current.ID, wrapperRoutingFacts("fake"))
	if e != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" {
		t.Fatal(blocked, e)
	}
	yes := true
	if restored := s.UpdateRoutingContext(ctx, current.ConnectionID, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "2", Enabled: &yes}); restored.Status != 200 {
		t.Fatal(restored)
	}
	view, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil {
		t.Fatal(e)
	}
	step, e = s.WorkflowStep(ctx, step.ID)
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	plan := domain.WorkflowStagePlan{Role: step.Role, ExecutorKind: step.ExecutorKind, Cycle: step.Cycle, LogicalKey: step.LogicalKey, DefinitionID: step.DefinitionID, InputCandidateID: step.InputCandidateID, InputResponseBinding: step.InputResponseBinding, PredecessorStepID: &step.ID, ReplacementOfStepID: &step.ID, ReasonCodes: []string{"explicit_current_blocked_retry"}}
	return s, owner, task.Task, *view.Workflow, step, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: task.Task.Revision, ExpectedWorkflowRevision: view.Workflow.Revision, Routing: wrapperRoutingFacts("fake")}
}

func workflowBlockedAuthorRetryFixture(t *testing.T) (*Store, string, domain.Task, domain.Workflow, domain.WorkflowStep, domain.WorkflowStepAdmissionFacts) {
	t.Helper()
	ctx := context.Background()
	s := testStore(t)
	owner, in, capture := workflowAdmissionFixture(t, s)
	created := s.CreateTaskWithWorkflow(ctx, owner, in, capture)
	if created.Status != 201 {
		t.Fatal(created)
	}
	a := created.Envelope.Data.(domain.Admission)
	no := false
	if disabled := s.UpdateRoutingContext(ctx, a.Attempt.ConnectionID, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no}); disabled.Status != 200 {
		t.Fatal(disabled)
	}
	blocked, e := s.DispatchRouting(ctx, a.Attempt.ID, capture.Routing)
	if e != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" {
		t.Fatal(blocked, e)
	}
	yes := true
	if restored := s.UpdateRoutingContext(ctx, a.Attempt.ConnectionID, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "2", Enabled: &yes}); restored.Status != 200 {
		t.Fatal(restored)
	}
	task, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	view, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil {
		t.Fatal(e)
	}
	step, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil {
		t.Fatal(e)
	}
	plan := domain.WorkflowStagePlan{Role: "author", ExecutorKind: "native", Cycle: 0, LogicalKey: step.LogicalKey, PredecessorStepID: &step.ID, ReplacementOfStepID: &step.ID, ReasonCodes: []string{"explicit_current_blocked_retry"}}
	return s, owner, task.Task, *view.Workflow, step, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: task.Task.Revision, ExpectedWorkflowRevision: view.Workflow.Revision, Routing: capture.Routing}
}

func TestWorkflowBlockedNativeRetryExplicitAuthor(t *testing.T) {
	ctx := context.Background()
	s, owner, task, w, old, facts := workflowBlockedAuthorRetryFixture(t)
	request := domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Revision, WorkflowRevision: w.Revision, TaskVersion: w.TaskVersion}
	result := s.AdvanceWorkflow(ctx, owner, task.ID, request, facts)
	if result.Status != 201 {
		t.Fatal("explicit blocked author retry unavailable", result)
	}
	next := result.Envelope.Data.(domain.WorkflowStepAdmission)
	if next.Step.Role != "author" || next.Step.ID == old.ID || next.Step.ExecutionOrdinal != "2" || next.Step.LogicalKey != old.LogicalKey || next.Attempt.PreviousAttemptID == nil || *next.Attempt.PreviousAttemptID != *old.AttemptID || next.RoutingDecision.Kind != "manual" || next.Budget.ConsumedAdmissions != "2" {
		t.Fatal(next)
	}
	unchanged, e := s.WorkflowStep(ctx, old.ID)
	if e != nil || !reflect.DeepEqual(unchanged, old) {
		t.Fatal("explicit author retry rewrote terminal", unchanged, e)
	}
	stale := facts
	stale.ExpectedTaskRevision = next.Attempt.Revision
	stale.ExpectedWorkflowRevision = next.Workflow.Revision
	staleRequest := domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: stale.ExpectedTaskRevision, WorkflowRevision: stale.ExpectedWorkflowRevision, TaskVersion: w.TaskVersion}
	if duplicate := s.AdvanceWorkflow(ctx, owner, task.ID, staleRequest, stale); duplicate.Status == 201 {
		t.Fatal("historical execution created a competing successor", duplicate)
	}
}

func TestWorkflowBlockedNativeRetryRejectsLaunchedStoppedUncertainProofOrOldControl(t *testing.T) {
	for _, change := range []string{"launched", "stop", "uncertain", "proof", "old_control", "missing_replacement"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			s, owner, task, w, old, facts := workflowBlockedRetryFixture(t, "review")
			attempt, e := s.Attempt(ctx, *old.AttemptID)
			if e != nil {
				t.Fatal(e)
			}
			switch change {
			case "launched":
				attempt.LaunchIntent = true
			case "stop":
				attempt.Stop = &domain.AcceptedStop{}
			case "uncertain":
				attempt.EvidenceStatus = "uncertain"
			case "proof":
				old.AdvanceProof = &domain.WorkflowAdvanceProof{}
				if _, e = s.db.Exec("UPDATE workflow_steps SET data=? WHERE id=?", encode(old), old.ID); e != nil {
					t.Fatal(e)
				}
			case "old_control":
				facts.ExpectedTaskRevision = "9223372036854775807"
			case "missing_replacement":
				facts.Plan.ReplacementOfStepID = nil
			}
			if change != "proof" && change != "old_control" {
				if _, e = s.db.Exec("UPDATE attempts SET data=? WHERE id=?", encode(attempt), attempt.ID); e != nil {
					t.Fatal(e)
				}
			}
			budget, e := s.TaskBudget(ctx, task.ID)
			if e != nil {
				t.Fatal(e)
			}
			request := domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: facts.ExpectedTaskRevision, WorkflowRevision: w.Revision, TaskVersion: w.TaskVersion}
			result := s.AdvanceWorkflow(ctx, owner, task.ID, request, facts)
			if result.Status == 201 {
				t.Fatal("noncurrent/unsafe execution admitted as manual retry", change, result)
			}
			after, e := s.TaskBudget(ctx, task.ID)
			if e != nil || !reflect.DeepEqual(budget, after) {
				t.Fatal("rejected retry changed finite authority", budget, after, e)
			}
		})
	}
}

func TestWorkflowBlockedNativeRetryExplicitReviewAndCorrection(t *testing.T) {
	for _, role := range []string{"review", "correction"} {
		t.Run(role, func(t *testing.T) {
			ctx := context.Background()
			s, owner, task, w, old, facts := workflowBlockedRetryFixture(t, role)
			before, e := s.TaskBudget(ctx, task.ID)
			if e != nil {
				t.Fatal(e)
			}
			request := domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Revision, WorkflowRevision: w.Revision, TaskVersion: w.TaskVersion}
			result := s.AdvanceWorkflow(ctx, owner, task.ID, request, facts)
			if result.Status != 201 {
				t.Fatal("current blocked native slot cannot be retried manually", result)
			}
			next := result.Envelope.Data.(domain.WorkflowStepAdmission)
			if next.Step.ID == old.ID || next.Step.LogicalKey != old.LogicalKey || next.Step.Role != old.Role || next.Step.Cycle != old.Cycle || next.Step.ExecutionOrdinal != "2" || next.Step.ReplacementOfStepID == nil || *next.Step.ReplacementOfStepID != old.ID || next.Attempt.PreviousAttemptID == nil || old.AttemptID == nil || *next.Attempt.PreviousAttemptID != *old.AttemptID || next.RoutingDecision == nil || next.RoutingDecision.Kind != "manual" || next.Attempt.ExecutionMode != "fake" || next.Workflow.CorrectionCyclesUsed != w.CorrectionCyclesUsed {
				t.Fatal("manual retry rebound or escalated its logical authority", next)
			}
			consumed, invalid := domain.AddRoutingCounts(before.Budget.ConsumedAdmissions, "1")
			if invalid != nil || next.Budget.ConsumedAdmissions != consumed || *next.Budget.CurrentWindow.DeadlineAt != *before.Budget.CurrentWindow.DeadlineAt {
				t.Fatal("manual retry lost finite debit/window", next)
			}
			unchanged, e := s.WorkflowStep(ctx, old.ID)
			if e != nil || !reflect.DeepEqual(unchanged, old) {
				t.Fatal("retry rewrote predecessor/proof", unchanged, e)
			}
			if repeated := s.AdvanceWorkflow(ctx, domain.NewID(), task.ID, request, domain.WorkflowStepAdmissionFacts{}); repeated.Status != 201 {
				t.Fatal("exact retry receipt replay required fresh authority", repeated)
			}
		})
	}
}
