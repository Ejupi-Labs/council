package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This is called only by the actual current-owner settlement callback. It
// verifies the retained input again and can admit only its registered member.
func (e *Engine) continueRecoveryGraphAdmission(ctx context.Context, sourceAttemptID string) {
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	op, scoped, err := e.db.RecoveryScopedInput(ctx, e.config.OwnerInstanceID, sourceAttemptID)
	if err != nil || !scoped || op.Control.NodeID == nil {
		return
	}
	view, err := e.db.Graph(ctx, op.GraphID)
	if err != nil || view.Graph.Stop != nil {
		return
	}
	var selected domain.GraphNodeSummary
	for _, node := range view.Nodes {
		if node.ID == *op.Control.NodeID {
			selected = node
			break
		}
	}
	if selected.ID == "" || selected.InputID == nil || *selected.InputID != op.ID || selected.NodeAdmissionID != nil || selected.Stop != nil || selected.State != "ready" {
		return
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, op.GraphID)
	if err != nil {
		return
	}
	facts, err := e.graphInitialAdmissionFacts(ctx, definition, selected, op.Control)
	if err != nil {
		if uncertainEvidence(err) {
			e.fence(sourceAttemptID, "graph_admission_evidence_uncertain", "The resumed member's exact input evidence cannot be verified.")
		}
		return
	}
	trigger := domain.GraphFreshTrigger{Kind: "completion", ID: sourceAttemptID, OwnerGeneration: op.Control.Generation}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return
	}
	dispatch, err := e.db.AdmitGraphNode(ctx, e.config.OwnerInstanceID, op.GraphID, trigger, facts)
	if persistenceFault(err) {
		e.fenceLocked(sourceAttemptID, "graph_admission_uncertain", "The resumed member admission lost acknowledgement; preserve its exact identity.")
	}
	e.control.Unlock()
	if err == nil && dispatch.Dispatch.AttemptID != "" {
		e.signal()
	}
}

// A later verified claim release can wake a scoped member which was waiting for
// capacity. Every registration is rechecked in the admission transaction.
func (e *Engine) continueRecoveryGraphReleases(ctx context.Context, sourceAttemptID string) {
	cursor := ""
	for ctx.Err() == nil {
		page, err := e.db.RecoveryReleaseScopes(ctx, e.config.OwnerInstanceID, cursor)
		if err != nil || len(page) == 0 {
			return
		}
		for _, scope := range page {
			cursor = scope.CheckpointID
			e.prepareGate.Lock()
			e.continueRecoveryGraphRelease(ctx, scope.GraphID, scope.NodeID, sourceAttemptID)
			e.prepareGate.Unlock()
		}
	}
}

func (e *Engine) continueRecoveryGraphRelease(ctx context.Context, graphID, nodeID, sourceAttemptID string) {
	view, err := e.db.Graph(ctx, graphID)
	if err != nil || view.Graph.Stop != nil || view.DispatchAuthority == nil {
		return
	}
	var selected domain.GraphNodeSummary
	for _, node := range view.Nodes {
		if node.ID == nodeID {
			selected = node
			break
		}
	}
	if selected.ID == "" || selected.InputID == nil || selected.NodeAdmissionID != nil || selected.Stop != nil || selected.State != "ready" {
		return
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, graphID)
	if err != nil {
		return
	}
	node, task, version, revision := selected.ID, selected.TaskID, selected.TaskVersion, selected.ControlRevision
	control := domain.GraphDispatchBinding{GraphID: graphID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID,
		OwnerInstanceID: e.config.OwnerInstanceID, Generation: view.DispatchAuthority.Generation, GraphControlRevision: view.Graph.ControlRevision,
		BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
		NodeID: &node, TaskID: &task, TaskVersion: &version, NodeControlRevision: &revision}
	facts, err := e.graphInitialAdmissionFacts(ctx, definition, selected, control)
	if err != nil {
		if uncertainEvidence(err) {
			e.fence(sourceAttemptID, "graph_admission_evidence_uncertain", "The resumed member's exact input evidence cannot be verified.")
		}
		return
	}
	trigger := domain.GraphFreshTrigger{Kind: "claim_release", ID: sourceAttemptID, OwnerGeneration: view.DispatchAuthority.Generation}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return
	}
	dispatch, err := e.db.AdmitGraphNode(ctx, e.config.OwnerInstanceID, graphID, trigger, facts)
	if persistenceFault(err) {
		e.fenceLocked(sourceAttemptID, "graph_admission_uncertain", "The resumed member admission lost acknowledgement; preserve its exact identity.")
	}
	e.control.Unlock()
	if err == nil && dispatch.Dispatch.AttemptID != "" {
		e.signal()
	}
}
