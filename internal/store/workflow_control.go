package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"time"
)

func workflowStepTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "cancelled", "blocked", "interrupted", "not_required":
		return true
	}
	return false
}
func (s *Store) saveWorkflowStep(ctx context.Context, tx *sql.Tx, step domain.WorkflowStep) error {
	old, e := s.workflowStep(ctx, tx, step.ID)
	if e != nil {
		return e
	}
	if old.WorkflowID != step.WorkflowID || old.Ordinal != step.Ordinal || old.ExecutionOrdinal != step.ExecutionOrdinal || old.Role != step.Role || old.ExecutorKind != step.ExecutorKind || old.Cycle != step.Cycle || old.LogicalKey != step.LogicalKey || !reflect.DeepEqual(old.AttemptID, step.AttemptID) || !reflect.DeepEqual(old.DefinitionID, step.DefinitionID) || !reflect.DeepEqual(old.InputCandidateID, step.InputCandidateID) || !reflect.DeepEqual(old.InputResponseBinding, step.InputResponseBinding) || !reflect.DeepEqual(old.ReplacementOfStepID, step.ReplacementOfStepID) {
		return domain.Fail("workflow_revision_conflict", "A step execution cannot be rebound. Reload its current workflow.")
	}
	if workflowStepTerminal(old.Status) {
		if reflect.DeepEqual(old, step) {
			return nil
		}
		return domain.Fail("workflow_revision_conflict", "A terminal step execution is immutable. Retain the new observation separately.")
	}
	if _, e = tx.ExecContext(ctx, "UPDATE workflow_steps SET status=?,data=? WHERE id=?", step.Status, encode(step), step.ID); e != nil {
		return storageError()
	}
	return nil
}
func (s *Store) workflowAttemptControls(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	if a.WorkflowID == "" {
		return nil
	}
	w, e := s.workflowByID(ctx, tx, a.WorkflowID)
	if e != nil {
		return e
	}
	step, e := s.workflowStep(ctx, tx, a.StepID)
	if e != nil {
		return e
	}
	if w.TaskID != a.TaskID || w.TaskVersion != a.TaskVersion || w.CurrentStepID == nil || *w.CurrentStepID != step.ID || step.WorkflowID != w.ID || step.AttemptID == nil || *step.AttemptID != a.ID || workflowStepTerminal(step.Status) {
		return domain.Fail("workflow_revision_conflict", "This attempt is no longer the current owned workflow execution.")
	}
	if a.Role != step.Role || a.ExecutorKind != step.ExecutorKind || w.State == "cancelled" || w.State == "cancelling" || w.State == "completed" {
		return domain.Fail("workflow_revision_conflict", "The current workflow no longer permits this execution.")
	}
	budget, e := s.taskBudget(ctx, tx, a.TaskID)
	if e != nil {
		return e
	}
	if budget == nil || budget.CurrentWindow.DeadlineAt == nil {
		return domain.Fail("budget_expired", "No open captured task window permits this stage.")
	}
	deadline, validationErr := domain.ParseRoutingTime(*budget.CurrentWindow.DeadlineAt)
	if validationErr != nil {
		return storageError()
	}
	if !time.Now().UTC().Before(deadline) {
		return domain.Fail("budget_expired", "The common task window expired before this stage effect.")
	}
	return nil
}
func (s *Store) syncWorkflowClaim(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	if a.WorkflowID == "" {
		return nil
	}
	step, e := s.workflowStep(ctx, tx, a.StepID)
	if e != nil {
		return e
	}
	if workflowStepTerminal(step.Status) {
		return domain.Fail("workflow_revision_conflict", "The workflow execution is already settled.")
	}
	now := domain.Now()
	step.Status = "active"
	if step.StartedAt == nil {
		step.StartedAt = &now
	}
	if e = s.saveWorkflowStep(ctx, tx, step); e != nil {
		return e
	}
	w, e := s.workflowByID(ctx, tx, a.WorkflowID)
	if e != nil {
		return e
	}
	w.State = "running"
	w.NextAction = "observe_attempt"
	w.NextOwner = "council"
	w.UpdatedAt = now
	w.StateRevision, e = increment(w.StateRevision)
	if e != nil {
		return e
	}
	return saveWorkflow(ctx, tx, w)
}

// Internal queue/Stop/deadline/recovery settlement has no fresh continuation proof.
func (s *Store) syncWorkflowTerminal(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	if a.WorkflowID == "" {
		return nil
	}
	step, e := s.workflowStep(ctx, tx, a.StepID)
	if e != nil {
		return e
	}
	if workflowStepTerminal(step.Status) {
		return nil
	}
	if a.Outcome == nil {
		return storageError()
	}
	step.Status = *a.Outcome
	step.SettledAt = a.SettledAt
	step.AdvanceProof = nil
	if e = s.saveWorkflowStep(ctx, tx, step); e != nil {
		return e
	}
	w, e := s.workflowByID(ctx, tx, a.WorkflowID)
	if e != nil {
		return e
	}
	if w.CurrentStepID != nil && *w.CurrentStepID == step.ID {
		w.State = *a.Outcome
		if w.State == "succeeded" {
			w.State = "blocked"
		}
		w.NextAction = "inspect_retained_stage"
		w.NextOwner = "user"
		w.UpdatedAt = domain.Now()
		w.StateRevision, e = increment(w.StateRevision)
		if e != nil {
			return e
		}
		return saveWorkflow(ctx, tx, w)
	}
	return nil
}
func (s *Store) suppressWorkflowAdvance(ctx context.Context, tx *sql.Tx, a domain.Attempt) (bool, error) {
	if a.WorkflowID == "" {
		return false, nil
	}
	w, e := s.workflowByID(ctx, tx, a.WorkflowID)
	if e != nil {
		return false, e
	}
	step, e := s.workflowStep(ctx, tx, a.StepID)
	if e != nil {
		return false, e
	}
	if step.AdvanceProof == nil || w.CurrentStepID == nil || *w.CurrentStepID != step.ID {
		return false, nil
	}
	t, e := s.task(ctx, tx, a.TaskID)
	if e != nil {
		return false, e
	}
	p := step.AdvanceProof
	if t.CurrentVersion != a.TaskVersion || t.LatestAttemptID == nil || *t.LatestAttemptID != a.ID || t.Revision != p.TaskRevision || w.Revision != p.WorkflowRevision {
		return false, nil
	}
	t.Revision, e = increment(t.Revision)
	if e != nil {
		return false, e
	}
	t.StateRevision, e = increment(t.StateRevision)
	if e != nil {
		return false, e
	}
	t.UpdatedAt = domain.Now()
	if e = saveTask(ctx, tx, t); e != nil {
		return false, e
	}
	w.Revision, e = increment(w.Revision)
	if e != nil {
		return false, e
	}
	w.StateRevision, e = increment(w.StateRevision)
	if e != nil {
		return false, e
	}
	w.State = "cancelled"
	w.NextAction = "inspect_retained_stage"
	w.NextOwner = "user"
	w.UpdatedAt = t.UpdatedAt
	if e = saveWorkflow(ctx, tx, w); e != nil {
		return false, e
	}
	return true, nil
}
