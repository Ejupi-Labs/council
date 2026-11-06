package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// 004A replaces an execution, never its logical workflow slot or immutable
// blocked terminal. Its routing proof/debit/receipt share the caller's SQL tx.
func (s *Store) bindWorkflowRoutingReplacement(ctx context.Context, tx *sql.Tx, previous domain.Attempt, next *domain.Attempt) (*domain.Workflow, *domain.WorkflowStep, error) {
	if previous.WorkflowID == "" {
		return nil, nil, nil
	}
	w, e := s.workflowByID(ctx, tx, previous.WorkflowID)
	if e != nil {
		return nil, nil, e
	}
	old, e := s.workflowStep(ctx, tx, previous.StepID)
	if e != nil {
		return nil, nil, e
	}
	if w.CurrentStepID == nil || *w.CurrentStepID != old.ID || old.WorkflowID != w.ID || old.AttemptID == nil || *old.AttemptID != previous.ID || old.Status != "blocked" || old.AdvanceProof != nil || old.ExecutorKind != "native" || w.State == "cancelled" || w.State == "completed" || w.Authority.ExecutionMode != previous.ExecutionMode || w.Authority.FakeScenario != previous.FakeScenario {
		return nil, nil, noRoutingContinuation
	}
	step := old
	step.ID = domain.NewID()
	step.ReplacementOfStepID = ptr(old.ID)
	step.AttemptID = ptr(next.ID)
	step.Ordinal, e = increment(old.Ordinal)
	if e != nil {
		return nil, nil, e
	}
	step.ExecutionOrdinal, e = increment(old.ExecutionOrdinal)
	if e != nil {
		return nil, nil, e
	}
	step.Status = "ready"
	step.CreatedAt = next.AdmittedAt
	step.StartedAt = nil
	step.SettledAt = nil
	step.AdvanceProof = nil
	step.OutputCandidateID = nil
	step.OutputResponseBinding = nil
	step.ReviewID = nil
	step.CheckID = nil
	step.ExternalResult = nil
	step.ReasonCodes = append(append([]string{}, old.ReasonCodes...), "routing_pre_launch_ineligible")
	next.WorkflowID = w.ID
	next.StepID = step.ID
	next.Role = old.Role
	next.ExecutorKind = old.ExecutorKind
	next.InputCandidateID = old.InputCandidateID
	next.Trigger = old.Trigger
	w.CurrentStepID = ptr(step.ID)
	w.Revision, e = increment(w.Revision)
	if e != nil {
		return nil, nil, e
	}
	w.StateRevision, e = increment(w.StateRevision)
	if e != nil {
		return nil, nil, e
	}
	w.State = "queued"
	w.NextAction = "observe_attempt"
	w.NextOwner = "council"
	w.UpdatedAt = next.AdmittedAt
	return &w, &step, nil
}
