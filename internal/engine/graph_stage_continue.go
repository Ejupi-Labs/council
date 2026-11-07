package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Only a fresh real claim release reaches this path. Pending stage identity
// comes from the current admitted workflow, never a historical attempt scan.
func (e *Engine) continueGraphWaitingStages(ctx context.Context, graphID, sourceAttemptID string) {
	view, err := e.db.Graph(ctx, graphID)
	if err != nil || view.Graph.Stop != nil || view.DispatchAuthority == nil || view.DispatchAuthority.OwnerInstanceID != e.config.OwnerInstanceID {
		return
	}
	for _, node := range view.Nodes {
		if ctx.Err() != nil {
			return
		}
		if node.Stop != nil || node.NodeAdmissionID == nil || node.InputID == nil {
			continue
		}
		anchor, err := e.db.GraphDispatch(ctx, graphID, *node.NodeAdmissionID)
		if err != nil || anchor.Dispatch.NodeID != node.ID || anchor.Dispatch.TaskID != node.TaskID || anchor.Dispatch.TaskVersion != node.TaskVersion || anchor.Dispatch.InputID != *node.InputID {
			continue
		}
		current, err := e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{WorkflowID: anchor.Dispatch.WorkflowID})
		if err != nil || current.Workflow == nil {
			continue
		}
		w := *current.Workflow
		if w.TaskVersion != node.TaskVersion || w.CurrentStepID == nil || w.NextOwner != "council" || w.NextAction != "capacity_wait" {
			continue
		}
		step, err := e.db.WorkflowStep(ctx, *w.CurrentStepID)
		if err != nil || step.AdvanceProof == nil || step.AttemptID == nil || step.WorkflowID != w.ID {
			continue
		}
		predecessor, err := e.db.Attempt(ctx, *step.AttemptID)
		if err != nil || predecessor.Phase != "settled" || predecessor.Cleanup.State != "verified" || predecessor.OwnerInstanceID != e.config.OwnerInstanceID {
			continue
		}
		progress, err := e.inspectWorkflowProgress(ctx, node.TaskID, w)
		if err != nil {
			if uncertainEvidence(err) {
				e.fence(sourceAttemptID, "graph_stage_evidence_uncertain", "The exact waiting workflow subject cannot be verified. Preserve its retained evidence before continuing.")
				return
			}
			continue
		}
		if progress.decision.Role == "" {
			continue
		}
		facts, err := e.workflowAdmissionFacts(ctx, progress, step.ID)
		if err != nil || facts.Graph == nil {
			if uncertainEvidence(err) {
				e.fence(sourceAttemptID, "graph_stage_preparation_uncertain", "The waiting workflow's exact graph input or source cannot be verified. Preserve its current ownership.")
				return
			}
			continue
		}
		facts.Graph.Trigger = domain.GraphFreshTrigger{Kind: "claim_release", ID: sourceAttemptID, OwnerGeneration: facts.Graph.Control.Generation}
		e.prepareGate.Lock()
		e.control.Lock()
		if e.fenced {
			e.control.Unlock()
			e.prepareGate.Unlock()
			return
		}
		admission, err := e.db.ContinueWorkflow(ctx, e.config.OwnerInstanceID, step.ID, facts)
		if persistenceFault(err) {
			e.fenceLocked(sourceAttemptID, "graph_stage_admission_uncertain", "The waiting graph stage lost its admission acknowledgement. Preserve its exact identity and do not replay work.")
		}
		e.control.Unlock()
		e.prepareGate.Unlock()
		if err == nil && admission != nil {
			e.signal()
		}
	}
}
