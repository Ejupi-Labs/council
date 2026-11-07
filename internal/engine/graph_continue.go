package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Admission consumes an actual current-owner completion or claim release. It
// must never be called by observation, restart recovery or allowance changes.
// Only persisted live registrations are enumerated; old graph history is never
// a source of new dispatch authority.
func (e *Engine) continueGraphAdmissions(sourceAttemptID string) {
	ctx := e.ctx
	if ctx.Err() != nil {
		return
	}
	source, err := e.db.Attempt(ctx, sourceAttemptID)
	if err != nil || source.OwnerInstanceID != e.config.OwnerInstanceID || source.Phase != "settled" || source.Cleanup.State != "verified" {
		return
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || gate.OwnerInstanceID != e.config.OwnerInstanceID {
		return
	}
	for _, claim := range gate.ActiveClaims {
		if claim.AttemptID == source.ID {
			return
		}
	}
	cursor := ""
	for ctx.Err() == nil {
		page, err := e.db.LiveGraphRegistrations(ctx, e.config.OwnerInstanceID, 32, cursor)
		if err != nil {
			return
		}
		for _, registration := range page.Items {
			if ctx.Err() != nil {
				return
			}
			e.prepareGate.Lock()
			e.admitReadyGraphNodes(ctx, registration.Graph.ID, source.ID)
			e.prepareGate.Unlock()
			e.continueGraphWaitingStages(ctx, registration.Graph.ID, source.ID)
		}
		if page.NextCursor == nil || *page.NextCursor == cursor {
			return
		}
		cursor = *page.NextCursor
	}
}

func (e *Engine) admitReadyGraphNodes(ctx context.Context, graphID, sourceAttemptID string) {
	view, err := e.db.Graph(ctx, graphID)
	if err != nil || view.Graph.Stop != nil || view.DispatchAuthority == nil {
		return
	}
	registration := *view.DispatchAuthority
	if registration.OwnerInstanceID != e.config.OwnerInstanceID || registration.State == "suspended" || registration.GraphControlRevision != view.Graph.ControlRevision ||
		registration.BudgetAuthorityEpoch != view.Graph.BudgetAuthorityEpoch || registration.WindowNumber != view.Budget.Budget.CurrentWindow.Number {
		return
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, graphID)
	if err != nil {
		return
	}
	queued := false
	for _, node := range view.Nodes {
		if ctx.Err() != nil {
			break
		}
		if node.InputID == nil || node.NodeAdmissionID != nil || node.Stop != nil || node.State != "ready" {
			continue
		}
		authorized := false
		for _, authority := range registration.NodeAuthorities {
			if authority.NodeID == node.ID && authority.TaskVersion == node.TaskVersion && authority.NodeControlRevision == node.ControlRevision {
				authorized = true
				break
			}
		}
		if !authorized {
			continue
		}
		nodeID, taskID, version, revision := node.ID, node.TaskID, node.TaskVersion, node.ControlRevision
		control := domain.GraphDispatchBinding{GraphID: graphID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID,
			OwnerInstanceID: e.config.OwnerInstanceID, Generation: registration.Generation, GraphControlRevision: view.Graph.ControlRevision,
			BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
			NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision}
		facts, captureErr := e.graphInitialAdmissionFacts(ctx, definition, node, control)
		if captureErr != nil {
			if uncertainEvidence(captureErr) {
				e.fence(sourceAttemptID, "graph_admission_evidence_uncertain", "The exact receiving input or preparation evidence cannot be verified. Preserve its current ownership before continuing.")
				break
			}
			if ctx.Err() != nil {
				break
			}
			facts.PreparationFailure = domain.SafeError(captureErr)
		}
		trigger := domain.GraphFreshTrigger{Kind: "claim_release", ID: sourceAttemptID, OwnerGeneration: registration.Generation}
		e.control.Lock()
		if e.fenced {
			e.control.Unlock()
			break
		}
		dispatch, admissionErr := e.db.AdmitGraphNode(ctx, e.config.OwnerInstanceID, graphID, trigger, facts)
		if persistenceFault(admissionErr) {
			e.fenceLocked(sourceAttemptID, "graph_admission_uncertain", "A graph node admission lost its durable acknowledgement. Preserve its exact identity; do not replay the model call.")
		}
		e.control.Unlock()
		if admissionErr == nil && dispatch.Dispatch.AttemptID != "" {
			queued = true
		}
	}
	if queued {
		e.signal()
	}
}
