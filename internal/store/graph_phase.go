package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// projectGraphAttemptPhase records progress with the observation that caused it.
// It neither qualifies an output nor changes dispatch authority. Historical or
// stopped executions may retain observations without repainting current work.
func (s *Store) projectGraphAttemptPhase(ctx context.Context, tx *sql.Tx, a domain.Attempt) error {
	if a.WorkflowID == "" || a.StepID == "" || a.Stop != nil || a.PrimaryFailure != nil {
		return nil
	}
	switch a.Phase {
	case "preparing", "running", "cancelling":
	default:
		return nil
	}
	member, err := s.graphMembership(ctx, tx, a.TaskID)
	if err != nil || member == nil {
		return err
	}
	if member.Kind != "node" || member.NodeID == nil {
		return nil
	}
	task, err := s.task(ctx, tx, a.TaskID)
	if err != nil {
		return err
	}
	if task.CurrentVersion != a.TaskVersion || task.LatestAttemptID == nil || *task.LatestAttemptID != a.ID {
		return nil
	}
	w, err := s.workflowByID(ctx, tx, a.WorkflowID)
	if err != nil {
		return err
	}
	if w.TaskID != a.TaskID || w.TaskVersion != a.TaskVersion || w.CurrentStepID == nil || *w.CurrentStepID != a.StepID || w.NextOwner != "council" || (w.State != "queued" && w.State != "running") {
		return nil
	}
	step, err := s.workflowStep(ctx, tx, a.StepID)
	if err != nil {
		return err
	}
	if step.WorkflowID != w.ID || step.AttemptID == nil || *step.AttemptID != a.ID || step.Role != a.Role || step.ExecutorKind != a.ExecutorKind || workflowStepTerminal(step.Status) {
		return nil
	}
	h, err := s.graphHeader(ctx, tx, member.GraphID)
	if err != nil {
		return err
	}
	gate, err := s.gate(ctx, tx)
	if err != nil {
		return err
	}
	if gate.OwnerInstanceID != a.OwnerInstanceID || h.DispatchAuthority == nil || h.DispatchAuthority.OwnerInstanceID != a.OwnerInstanceID || h.Summary.Stop != nil || h.Summary.State == "completed" || h.Summary.State == "cancelled" {
		return nil
	}
	nodes, err := s.graphNodes(ctx, tx, member.GraphID)
	if err != nil {
		return err
	}
	index := -1
	for i := range nodes {
		if nodes[i].Summary.ID == *member.NodeID {
			index = i
			break
		}
	}
	if index < 0 {
		return notFound()
	}
	n := &nodes[index]
	if n.Summary.TaskID != a.TaskID || n.Summary.TaskVersion != a.TaskVersion || n.Summary.LatestAttemptID == nil || *n.Summary.LatestAttemptID != a.ID || n.Summary.Stop != nil || n.Summary.AcceptedResult != nil || n.Blocker != nil || n.Summary.NextOwner != "council" || graphRetainedTerminal(n.Summary.State) || n.Summary.State == "blocked" || n.Summary.State == a.Phase {
		return nil
	}
	n.Summary.State = a.Phase
	n.Summary.StateRevision, err = increment(n.Summary.StateRevision)
	if err != nil {
		return err
	}
	if err = saveGraphNode(ctx, tx, *n); err != nil {
		return err
	}
	h.Summary.Counts, err = graphCounts(nodes)
	if err != nil {
		return err
	}
	// Preserve exceptional group outcomes and their actionable next steps. An
	// ordinary active group follows its current nodes without touching controls.
	if h.Summary.State == "queued" || h.Summary.State == "preparing" || h.Summary.State == "running" {
		h.Summary.State = "queued"
		if h.Summary.Counts.Running != "0" {
			h.Summary.State = "running"
		} else if h.Summary.Counts.Preparing != "0" {
			h.Summary.State = "preparing"
		}
	}
	h.Summary.StateRevision, err = increment(h.Summary.StateRevision)
	if err != nil {
		return err
	}
	h.Summary.UpdatedAt = domain.Now()
	return saveGraphHeader(ctx, tx, h)
}
