package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Only an explicit current advance can retry a clean pre-launch blocked native
// slot. Automatic progression never calls this path or revives an old proof.
func (e *Engine) retryBlockedWorkflow(ctx context.Context, taskID string, w domain.Workflow, in domain.WorkflowAdvanceInput) (domain.Result, bool) {
	if w.CurrentStepID == nil || w.State != "blocked" {
		return domain.Result{}, false
	}
	step, err := e.db.WorkflowStep(ctx, *w.CurrentStepID)
	if err != nil {
		return e.failure(in.RequestID, err), true
	}
	if step.AttemptID == nil {
		return domain.Result{}, false
	}
	attempt, err := e.db.Attempt(ctx, *step.AttemptID)
	if err != nil {
		return e.failure(in.RequestID, err), true
	}
	plan := domain.BlockedNativeRetryPlan(w, step, attempt)
	if plan == nil {
		return domain.Result{}, false
	}
	detail, err := e.db.Task(ctx, taskID)
	if err != nil {
		return e.failure(in.RequestID, err), true
	}
	if detail.Task.Revision != in.ExpectedRevision {
		return e.failure(in.RequestID, domain.RevisionConflict(detail.Task.Revision)), true
	}
	if plan.Role != "author" {
		if _, err = e.selectedWorkflowSubject(ctx, w); err != nil {
			return e.failure(in.RequestID, err), true
		}
	}
	facts := domain.WorkflowStepAdmissionFacts{Plan: *plan, ExpectedTaskRevision: detail.Task.Revision, ExpectedWorkflowRevision: w.Revision}
	facts.Routing, err = e.nativeWorkflowRoutingFacts(ctx, w, detail.Task.ProjectID)
	if err != nil {
		return e.failure(in.RequestID, err), true
	}
	return e.admitExplicitWorkflow(ctx, taskID, in, nil, facts), true
}
