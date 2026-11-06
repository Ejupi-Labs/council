package store

import (
	"context"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"testing"
)

func workflowRecaptureGuidance(t *testing.T, version domain.TaskVersion, original, guidance string, facts *domain.WorkflowAdmissionFacts) {
	t.Helper()
	version.GuidanceText = guidance
	a := &facts.Capture.Authority
	registry, invalid := workflowpolicy.Conditions(original, version, a.OptionSnapshot, a.Checks)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instructions, conditions, err := workflowpolicy.ConditionDigests(original, version, registry, a.Checks)
	if err != nil {
		t.Fatal(err)
	}
	a.Conditions, a.InstructionSHA256, a.ConditionsSHA256 = registry, instructions, conditions
}

func TestWorkflowAdoptionIsProspectiveAndDoesNotRefillOrRun(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, in, f := workflowAdmissionFixture(t, s)
	in.Workflow = &domain.WorkflowInput{Mode: "author_only"}
	created := s.CreateTaskWithWorkflow(ctx, owner, in, f)
	if created.Status != 201 {
		t.Fatal(created)
	}
	a := created.Envelope.Data.(domain.Admission)
	if _, e := s.SettleAttempt(ctx, a.Attempt.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
		t.Fatal(e)
	}
	before, e := s.TaskBudget(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	detail, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	request := domain.WorkflowAdoptInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: detail.Version.Version, Workflow: &domain.WorkflowInput{Mode: "auto"}}
	result := s.AdoptWorkflow(ctx, owner, a.Task.ID, request, f)
	if result.Status != 200 {
		t.Fatal(result)
	}
	view, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil || view.Workflow == nil {
		t.Fatal(view, e)
	}
	if view.TaskVersion == a.Version.Version || view.Workflow.CurrentStepID != nil || view.Workflow.SelectedCandidateID != nil || view.Workflow.Authority.ExecutionMode != "fake" {
		t.Fatal("adoption reused old authority/work or lost fake provenance", view)
	}
	after, e := s.TaskBudget(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	if before.Budget.Revision != after.Budget.Revision || before.Budget.GrantedAdmissions != after.Budget.GrantedAdmissions || before.Budget.ConsumedAdmissions != after.Budget.ConsumedAdmissions {
		t.Fatal("adoption silently granted/debited model authority", before, after)
	}
	if _, attempts, steps, workflows := workflowCounts(t, s); attempts != 1 || steps != 0 || workflows != 1 {
		t.Fatal("adoption ran or rewrote historical work", attempts, steps, workflows)
	}
	replay := s.AdoptWorkflow(ctx, domain.NewID(), a.Task.ID, request, domain.WorkflowAdmissionFacts{})
	if replay.Status != 200 {
		t.Fatal("exact replay required current captured facts", replay)
	}
	current, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	start := domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: current.Task.Revision, TaskVersion: current.Version.Version}
	run := s.StartAttemptWithWorkflow(ctx, owner, a.Task.ID, start, domain.WorkflowAdmissionFacts{Routing: wrapperRoutingFacts("fake")})
	if run.Status != 201 {
		t.Fatal("separate explicit author start unavailable", run)
	}
	newAttempt := run.Envelope.Data.(domain.Admission).Attempt
	if newAttempt.ExecutionMode != "fake" || newAttempt.Role != "author" || newAttempt.WorkflowID != view.Workflow.ID {
		t.Fatal("restart-safe provenance lost", newAttempt)
	}
}

func TestWorkflowGuidanceCapturesNewAuthorityWithoutExecutionOrRefill(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, in, f := workflowAdmissionFixture(t, s)
	created := s.CreateTaskWithWorkflow(ctx, owner, in, f)
	if created.Status != 201 {
		t.Fatal(created)
	}
	a := created.Envelope.Data.(domain.Admission)
	if _, e := s.SettleAttempt(ctx, a.Attempt.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
		t.Fatal(e)
	}
	before, e := s.TaskBudget(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil {
		t.Fatal(e)
	}
	request := domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", GuidanceText: "Preserve this new exact additional instruction."}
	workflowRecaptureGuidance(t, a.Version, in.OriginalText, request.GuidanceText, &f)
	result := s.UpdateGuidanceWithWorkflow(ctx, a.Task.ID, request, f)
	if result.Status != 200 {
		t.Fatal(result)
	}
	current, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{})
	if e != nil || current.Workflow == nil {
		t.Fatal(current, e)
	}
	if current.Workflow.ID == old.Workflow.ID || current.Workflow.CurrentStepID != nil || current.Workflow.Authority.InstructionSHA256 == old.Workflow.Authority.InstructionSHA256 || current.Workflow.Authority.ExecutionMode != "fake" {
		t.Fatal("guidance rewrote, autoran or lost captured facts", current)
	}
	historical, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{WorkflowID: old.Workflow.ID})
	if e != nil || historical.Workflow.Authority.InstructionSHA256 != old.Workflow.Authority.InstructionSHA256 {
		t.Fatal("guidance rewrote old authority", historical, e)
	}
	after, e := s.TaskBudget(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	if before.Budget.Revision != after.Budget.Revision || before.Budget.ConsumedAdmissions != after.Budget.ConsumedAdmissions || before.Budget.GrantedAdmissions != after.Budget.GrantedAdmissions {
		t.Fatal("guidance replenished budget", before, after)
	}
	if _, attempts, steps, workflows := workflowCounts(t, s); attempts != 1 || steps != 1 || workflows != 2 {
		t.Fatal(attempts, steps, workflows)
	}
	if replay := s.UpdateGuidanceWithWorkflow(ctx, a.Task.ID, request, domain.WorkflowAdmissionFacts{}); replay.Status != 200 {
		t.Fatal(replay)
	}
}

func TestWorkflowAuthorityCaptureRollbackPreservesCurrentVersion(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, in, f := workflowAdmissionFixture(t, s)
	created := s.CreateTaskWithWorkflow(ctx, owner, in, f)
	if created.Status != 201 {
		t.Fatal(created)
	}
	a := created.Envelope.Data.(domain.Admission)
	request := domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", GuidanceText: "Retain transaction rollback."}
	workflowRecaptureGuidance(t, a.Version, in.OriginalText, request.GuidanceText, &f)
	s.BeforeCommit = func() error { return errors.New("workflow authority transaction rejected") }
	result := s.UpdateGuidanceWithWorkflow(ctx, a.Task.ID, request, f)
	s.BeforeCommit = nil
	if result.Envelope.Error == nil {
		t.Fatal("injected rollback disappeared", result)
	}
	current, e := s.Task(ctx, a.Task.ID)
	if e != nil || current.Version.Version != "1" || current.Task.Revision != "1" {
		t.Fatal("partial authority/version committed", current, e)
	}
	if _, attempts, steps, workflows := workflowCounts(t, s); attempts != 1 || steps != 1 || workflows != 1 {
		t.Fatal(attempts, steps, workflows)
	}
}
