package engine

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// recoveryStandaloneFresh re-verifies every member of the original and
// candidate manifests through candidateWorkflowSubject. The returned binding
// must match the controller-only immutable seal from before settlement.
func (e *Engine) recoveryStandaloneFresh(ctx context.Context, attemptID string) (string, *domain.CandidateBinding, error) {
	attempt, err := e.db.Attempt(ctx, attemptID)
	if err != nil {
		return "", nil, err
	}
	detail, err := e.db.Task(ctx, attempt.TaskID)
	if err != nil {
		return "", nil, err
	}
	if detail.Task.GraphRef != nil || attempt.Role != "author" || attempt.WorkflowID == "" || attempt.CandidateID == nil {
		return "", nil, nil
	}
	sequence, seal, err := e.db.RecoveryStandaloneSealForAttempt(ctx, attemptID)
	if err != nil {
		return "", nil, err
	}
	view, err := e.db.Workflow(ctx, attempt.TaskID, domain.WorkflowSelector{WorkflowID: attempt.WorkflowID})
	if err != nil {
		return "", nil, err
	}
	if view.Workflow == nil {
		return "", nil, domain.Fail("evidence_unavailable", "The sealed standalone workflow is unavailable.")
	}
	subject, err := e.candidateWorkflowSubject(ctx, *view.Workflow, *attempt.CandidateID)
	if err != nil {
		return "", nil, err
	}
	if subject.Binding == nil || seal.AttemptID != attempt.ID || seal.OwnerInstanceID != attempt.OwnerInstanceID || seal.WorkflowID != attempt.WorkflowID || seal.StepID != attempt.StepID || !reflect.DeepEqual(*subject.Binding, seal.Binding) {
		return "", nil, domain.Fail("evidence_unavailable", "The sealed standalone subject no longer matches its verified closure.")
	}
	return sequence, subject.Binding, nil
}
