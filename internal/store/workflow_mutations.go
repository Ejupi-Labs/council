package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) StartAttemptWithWorkflow(ctx context.Context, ownerID, taskID string, in domain.StartAttemptInput, f domain.WorkflowAdmissionFacts) domain.Result {
	if invalid := domain.ValidateStartAttempt(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	fp := AttemptStartFingerprint(taskID, in)
	if old, e := s.Replay(ctx, in.RequestID, "tasks.attempts.start", fp); e != nil {
		return domain.Failure(in.RequestID, asDomain(e))
	} else if old != nil {
		return *old
	}
	view, e := s.Workflow(ctx, taskID, domain.WorkflowSelector{})
	if e != nil {
		return domain.Failure(in.RequestID, asDomain(e))
	}
	if view.Workflow == nil {
		return s.StartAttemptWithRouting(ctx, ownerID, taskID, in, f.Routing)
	}
	return s.mutate(ctx, in.RequestID, "tasks.attempts.start", fp, func(tx *sql.Tx) (int, any, error) {
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.CurrentVersion != in.TaskVersion {
			return 0, nil, domain.Fail("task_version_conflict", "Run only the current captured task version.")
		}
		selected, e := s.selectedWorkflow(ctx, tx, taskID, domain.WorkflowSelector{})
		if e != nil {
			return 0, nil, e
		}
		if selected.Workflow == nil {
			return 0, nil, workflowUnavailable()
		}
		w := *selected.Workflow
		if w.CurrentStepID != nil || w.SelectedCandidateID != nil || w.SelectedResponseBinding != nil {
			return 0, nil, domain.Fail("authority_required", "This Auto workflow already has a retained stage. Use tasks advance to preserve its selected result.")
		}
		if in.ExecutionMode != "" && in.ExecutionMode != w.Authority.ExecutionMode || in.FakeScenario != "" && in.FakeScenario != w.Authority.FakeScenario {
			return 0, nil, domain.Invalid("matching captured workflow execution mode")
		}
		b, e := s.taskBudget(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		plan := domain.WorkflowStagePlan{Role: "author", ExecutorKind: "native", LogicalKey: "author:0", Cycle: 0, ReasonCodes: []string{"explicit_author_start"}}
		admitted, e := s.admitWorkflowStage(ctx, tx, ownerID, &t, &w, b, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: in.ExpectedRevision, ExpectedWorkflowRevision: w.Revision, Routing: f.Routing}, in.RequestID, "")
		if e != nil {
			return 0, nil, e
		}
		v, e := s.version(ctx, tx, taskID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		return 201, domain.Admission{Task: taskSummary(t, v, &admitted.Attempt, false), Version: v, Attempt: admitted.Attempt, Budget: &admitted.Budget, RoutingDecision: admitted.RoutingDecision, Workflow: &admitted.Workflow, Step: &admitted.Step}, nil
	})
}

func (s *Store) AdvanceWorkflow(ctx context.Context, ownerID, taskID string, in domain.WorkflowAdvanceInput, f domain.WorkflowStepAdmissionFacts) domain.Result {
	if invalid := domain.ValidateWorkflowAdvanceInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	return s.workflowExplicitStage(ctx, ownerID, taskID, in, f, "tasks.workflow.advance", WorkflowAdvanceFingerprint(taskID, in))
}
func (s *Store) RecheckWorkflow(ctx context.Context, ownerID, taskID string, in domain.WorkflowCheckInput, f domain.WorkflowStepAdmissionFacts) domain.Result {
	if invalid := domain.ValidateWorkflowCheckInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if f.Plan.Role != "check" || f.Plan.DefinitionID == nil || *f.Plan.DefinitionID != in.CheckID {
		return domain.Failure(in.RequestID, domain.Invalid("requested captured check execution"))
	}
	plain := domain.WorkflowAdvanceInput{RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision, WorkflowRevision: in.WorkflowRevision, TaskVersion: in.TaskVersion}
	return s.workflowExplicitStage(ctx, ownerID, taskID, plain, f, "tasks.workflow.check", WorkflowCheckFingerprint(taskID, in))
}
func (s *Store) workflowExplicitStage(ctx context.Context, ownerID, taskID string, in domain.WorkflowAdvanceInput, f domain.WorkflowStepAdmissionFacts, op, fp string) domain.Result {
	return s.mutate(ctx, in.RequestID, op, fp, func(tx *sql.Tx) (int, any, error) {
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.CurrentVersion != in.TaskVersion {
			return 0, nil, domain.Fail("task_version_conflict", "Only the current task version can admit workflow work.")
		}
		selected, e := s.selectedWorkflow(ctx, tx, taskID, domain.WorkflowSelector{})
		if e != nil {
			return 0, nil, e
		}
		if selected.Workflow == nil {
			return 0, nil, workflowUnavailable()
		}
		w := *selected.Workflow
		if f.ExpectedTaskRevision != in.ExpectedRevision || f.ExpectedWorkflowRevision != in.WorkflowRevision {
			return 0, nil, domain.Fail("workflow_revision_conflict", "The prepared stage no longer matches this explicit request.")
		}
		b, e := s.taskBudget(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		admitted, e := s.admitWorkflowStage(ctx, tx, ownerID, &t, &w, b, f, in.RequestID, "")
		if e != nil {
			return 0, nil, e
		}
		return 201, admitted, nil
	})
}

// asDomain preserves typed store faults for the public pre-transaction replay/read
// boundary; ordinary context interruption remains distinct from uncertain commit.
func asDomain(err error) *domain.Error {
	if e, ok := err.(*domain.Error); ok {
		return e
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return domain.Fail("operation_interrupted", "The operation was interrupted before a new commit.")
	}
	return storageError()
}
