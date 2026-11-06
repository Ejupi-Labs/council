package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func knownWorkflowAdmissionBlock(err error) *domain.Error {
	var fault *domain.Error
	if !errors.As(err, &fault) || fault == nil {
		return nil
	}
	switch fault.Code {
	case "budget_exhausted", "budget_expired", "routing_unavailable":
		return fault
	}
	return nil
}

// Call only after the failed admission transaction rolled back. No partially
// mutated budget/decision/attempt can leak into this separate passive projection.
// Changing workflow control revision consumes freshness without rewriting the
// predecessor's immutable terminal/proof or admitting a successor.
func (s *Store) projectWorkflowAdmissionBlock(ctx context.Context, ownerID, predecessorID string, f domain.WorkflowStepAdmissionFacts, cause *domain.Error) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		step, e := s.workflowStep(ctx, tx, predecessorID)
		if e != nil {
			return e
		}
		p := step.AdvanceProof
		if p == nil || p.OwnerInstanceID != ownerID || p.WorkflowRevision != f.ExpectedWorkflowRevision {
			return nil
		}
		w, e := s.workflowByID(ctx, tx, step.WorkflowID)
		if e != nil {
			return e
		}
		t, e := s.task(ctx, tx, w.TaskID)
		if e != nil {
			return e
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		if checkGate(g) != nil || g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID || w.CurrentStepID == nil || *w.CurrentStepID != step.ID || w.Revision != f.ExpectedWorkflowRevision || w.TaskVersion != t.CurrentVersion || t.LatestAttemptID == nil || step.AttemptID == nil || *t.LatestAttemptID != *step.AttemptID || w.State == "completed" || w.State == "cancelled" {
			return nil
		}
		action := cause.Code
		if t.Revision != p.TaskRevision {
			action = "advance_current_workflow"
		}
		w.State = "blocked"
		w.NextAction = action
		w.NextOwner = "user"
		w.UpdatedAt = domain.Now()
		w.Revision, e = increment(w.Revision)
		if e != nil {
			return e
		}
		w.StateRevision, e = increment(w.StateRevision)
		if e != nil {
			return e
		}
		if e = saveWorkflow(ctx, tx, w); e != nil {
			return e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return e
		}
		t.UpdatedAt = w.UpdatedAt
		return saveTask(ctx, tx, t)
	})
}
