package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// Resume validates exact receipt identity before any filesystem evidence read.
// Fresh controller facts and the pause transition enter one store transaction.
func (e *Engine) ResumeCheckpoint(ctx context.Context, id string, in domain.ResumeCheckpointInput) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	if invalid := domain.ValidateResumeCheckpointInput(in); invalid != nil {
		return e.failure(in.RequestID, invalid)
	}
	if !domain.ValidID(id) {
		return e.failure(in.RequestID, domain.Invalid("checkpoint identity"))
	}
	if prior, err := e.db.Replay(ctx, in.RequestID, "checkpoints.resume", store.RecoveryResumeFingerprint(id, in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if prior != nil {
		return *prior
	}

	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	checkpoint, err := e.db.Checkpoint(ctx, id)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if checkpoint.Revision != in.ExpectedCheckpointRevision || checkpoint.State != "paused" {
		return e.failure(in.RequestID, domain.Fail("checkpoint_not_paused", "Read the current durable paused checkpoint before resuming."))
	}
	if checkpoint.Scope.Kind != in.Scope.Kind || checkpoint.Scope.TaskID != in.Scope.TaskID || checkpoint.Scope.TaskVersion != in.Scope.TaskVersion || checkpoint.Scope.GraphID != in.Scope.GraphID || checkpoint.Scope.GraphVersion != in.Scope.GraphVersion || checkpoint.Scope.NodeID != in.Scope.NodeID {
		return e.failure(in.RequestID, domain.Fail("checkpoint_scope_conflict", "Resume the exact checkpoint and captured work scope."))
	}
	facts := store.RecoveryResumeFacts{}
	switch in.Scope.Kind {
	case "task":
		view, readErr := e.db.Workflow(ctx, in.Scope.TaskID, domain.WorkflowSelector{})
		if readErr != nil {
			return e.failure(in.RequestID, readErr)
		}
		if view.Workflow != nil && view.Workflow.CurrentStepID != nil {
			progress, progressErr := e.inspectWorkflowProgress(ctx, in.Scope.TaskID, *view.Workflow)
			if progressErr != nil {
				return e.failure(in.RequestID, progressErr)
			}
			if progress.decision.Role != "" && progress.decision.Action != "accept" {
				prepared, prepErr := e.workflowAdmissionFacts(ctx, progress, *view.Workflow.CurrentStepID)
				if prepErr != nil {
					return e.failure(in.RequestID, prepErr)
				}
				facts.Workflow = &prepared
			}
		}
	case "graph", "member":
		view, readErr := e.db.Graph(ctx, in.Scope.GraphID)
		if readErr != nil {
			return e.failure(in.RequestID, readErr)
		}
		if view.Graph.GraphVersion != in.Scope.GraphVersion || view.Graph.ControlRevision != in.Scope.ExpectedGraphControlRevision || view.Graph.BudgetAuthorityEpoch != in.Scope.ExpectedBudgetAuthorityEpoch || view.Budget.Budget.CurrentWindow.Number != in.Scope.ExpectedWindowNumber {
			return e.failure(in.RequestID, domain.Fail("graph_control_conflict", "Capture current graph authority before resuming."))
		}
		authorities := append([]domain.GraphNodeAuthority(nil), in.Scope.NodeAuthorities...)
		if in.Scope.Kind == "member" {
			authorities = make([]domain.GraphNodeAuthority, 0, len(view.Nodes))
			found := false
			for _, node := range view.Nodes {
				authorities = append(authorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
				if node.ID == in.Scope.NodeID && node.TaskID == in.Scope.TaskID && node.TaskVersion == in.Scope.TaskVersion && node.ControlRevision == in.Scope.ExpectedNodeControlRevision {
					found = true
				}
			}
			if !found {
				return e.failure(in.RequestID, domain.Fail("graph_control_conflict", "The selected member's current authority changed."))
			}
		}
		advance := domain.GraphAdvanceInput{RequestID: in.RequestID, GraphVersion: in.Scope.GraphVersion, ExpectedGraphControlRevision: in.Scope.ExpectedGraphControlRevision, ExpectedBudgetAuthorityEpoch: in.Scope.ExpectedBudgetAuthorityEpoch, ExpectedWindowNumber: in.Scope.ExpectedWindowNumber, NodeAuthorities: authorities}
		selected := make(map[string]bool, len(view.Nodes))
		for _, node := range view.Nodes {
			selected[node.ID] = in.Scope.Kind == "graph" || node.ID == in.Scope.NodeID
		}
		pauses, readErr := e.db.ActiveRecoveryCheckpoints(ctx, "", in.Scope.GraphID)
		if readErr != nil {
			return e.failure(in.RequestID, readErr)
		}
		for _, pause := range pauses {
			if pause.ID != id && pause.Scope.Kind == "member" {
				selected[pause.Scope.NodeID] = false
			}
			if pause.ID != id && pause.Scope.Kind == "graph" {
				for key := range selected {
					selected[key] = false
				}
			}
		}
		for _, node := range view.Nodes {
			if !selected[node.ID] {
				continue
			}
			blocked, checkErr := e.db.RecoveryAncestorsPaused(ctx, in.Scope.GraphID, node.ID)
			if checkErr != nil {
				return e.failure(in.RequestID, checkErr)
			}
			if blocked {
				selected[node.ID] = false
			}
		}
		prepared, prepErr := e.captureGraphAdvanceFacts(ctx, in.Scope.GraphID, advance, selected)
		if prepErr != nil {
			return e.failure(in.RequestID, prepErr)
		}
		facts.GraphInput, facts.Graph = &advance, &prepared
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Resolve retained ownership or persistence uncertainty before resuming work."))
	}
	result := e.db.ResumeCheckpoint(ctx, e.config.OwnerInstanceID, id, in, facts)
	e.guardMutationLocked(result)
	e.control.Unlock()
	if result.Envelope.Error == nil {
		e.signal()
	}
	return result
}
