package engine

import (
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Only an actual current-owner completion or fresh human acknowledgement can
// prepare a registered receiver. Observation, restart and grants never call this.
func (e *Engine) continueGraphInputs(sourceAttemptID string) {
	ctx := e.ctx
	if ctx.Err() != nil {
		return
	}
	source, err := e.db.Attempt(ctx, sourceAttemptID)
	if err != nil || source.Phase != "settled" || source.OwnerInstanceID != e.config.OwnerInstanceID || source.Outcome == nil || *source.Outcome != "succeeded" || source.Cleanup.State != "verified" {
		return
	}
	task, err := e.db.Task(ctx, source.TaskID)
	if err != nil || task.Task.GraphRef == nil {
		return
	}
	e.continueGraphInputsFromTrigger(task.Task.GraphRef.GraphID, source.ID, domain.GraphFreshTrigger{Kind: "completion", ID: source.ID})
}

// Called only after the exact fresh acknowledgement transaction has committed.
// Its event is distinct from the producer's earlier terminal, including after
// restart; persisted registration and transaction chronology remain mandatory.
func (e *Engine) continueGraphResponseAcknowledgement(ack domain.ResponseAcceptance) {
	ctx := e.ctx
	if ctx.Err() != nil {
		return
	}
	task, err := e.db.Task(ctx, ack.TaskID)
	if err != nil || task.Task.GraphRef == nil || task.Task.GraphRef.Kind != "node" || task.Task.GraphRef.NodeID == nil {
		return
	}
	member := task.Task.GraphRef
	stored, err := e.db.GraphResponseAcceptance(ctx, member.GraphID, *member.NodeID, ack.ID)
	if err != nil || !reflect.DeepEqual(stored, ack) {
		return
	}
	e.continueGraphInputsFromTrigger(member.GraphID, ack.AttemptID, domain.GraphFreshTrigger{Kind: "response_ack", ID: ack.ID})
}

func (e *Engine) continueGraphInputsFromTrigger(graphID, sourceAttemptID string, trigger domain.GraphFreshTrigger) {
	ctx := e.ctx
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	view, err := e.db.Graph(ctx, graphID)
	if err != nil || view.Graph.Stop != nil || view.DispatchAuthority == nil || view.DispatchAuthority.OwnerInstanceID != e.config.OwnerInstanceID || view.DispatchAuthority.State == "suspended" || view.Graph.RootInputID == nil {
		return
	}
	registration := *view.DispatchAuthority
	if registration.GraphControlRevision != view.Graph.ControlRevision || registration.BudgetAuthorityEpoch != view.Graph.BudgetAuthorityEpoch || registration.WindowNumber != view.Budget.Budget.CurrentWindow.Number {
		return
	}
	// A completion with no authorized receiver is observation only. Avoid opening
	// root payloads and holding the preparation gate for work that cannot begin.
	targets := []domain.GraphNodeSummary{}
	for _, node := range view.Nodes {
		if node.InputID != nil || node.Stop != nil || node.State != "waiting" {
			continue
		}
		for _, authority := range registration.NodeAuthorities {
			if authority.NodeID == node.ID && authority.TaskVersion == node.TaskVersion && authority.NodeControlRevision == node.ControlRevision {
				targets = append(targets, node)
				break
			}
		}
	}
	if len(targets) == 0 {
		return
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, view.Graph.ID)
	if err != nil {
		return
	}
	root, err := e.db.GraphInputRecord(ctx, view.Graph.ID, *view.Graph.RootInputID)
	if err != nil || root.PrivateRoot == nil || root.Public.Summary.State != "ready" || !root.Public.Summary.Current {
		return
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, *root.PrivateRoot, definition.Delivery)
	if err != nil {
		if uncertainEvidence(err) {
			e.fence(sourceAttemptID, "graph_root_evidence_uncertain", "The retained root preparation closure cannot be verified. Preserve its evidence before continuing.")
		}
		return
	}
	rootProofs, err := e.verifyGraphRootClosure(ctx, *root.PrivateRoot, rootSource)
	if err != nil {
		if uncertainEvidence(err) {
			e.fence(sourceAttemptID, "graph_root_evidence_uncertain", "The retained root preparation closure cannot be verified. Preserve its evidence before continuing.")
		}
		return
	}
	trigger.OwnerGeneration = registration.Generation
	queued := false
	for _, node := range targets {
		if ctx.Err() != nil {
			break
		}
		dependent := false
		if definition.Original.Dependencies != nil {
			for _, edge := range *definition.Original.Dependencies {
				if edge.To == node.Key {
					dependent = true
					break
				}
			}
		}
		contributions, vector := []domain.AcceptedContribution{}, []domain.GraphContributorBinding{}
		proofs := rootProofs
		if dependent {
			selected, selectErr := e.selectGraphContributions(ctx, definition, *root.PrivateRoot, node)
			if selectErr != nil {
				if uncertainEvidence(selectErr) {
					e.fence(sourceAttemptID, "graph_dependency_evidence_uncertain", "The exact accepted predecessor closure cannot be verified. Preserve its evidence before continuing.")
					break
				}
				continue
			}
			vector = selected.ContributorVector
			for _, selectedSource := range selected.Sources {
				contributions = append(contributions, selectedSource.Binding)
			}
			proofs, err = mergeGraphAcceptanceProofs(rootProofs, selected.VerifiedArtifacts)
			if err != nil {
				continue
			}
		}
		nodeID, taskID, version, revision := node.ID, node.TaskID, node.TaskVersion, node.ControlRevision
		facts := domain.GraphInputPreparationFacts{
			Trigger: trigger, RootBinding: *root.PrivateRoot,
			Control: domain.GraphDispatchBinding{GraphID: view.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID, OperationAttemptID: sourceAttemptID, OwnerInstanceID: e.config.OwnerInstanceID,
				Generation: registration.Generation, GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
				NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision, ContributorVector: vector},
			Contributions: contributions, ContributorVector: vector, VerifiedArtifacts: proofs}
		e.control.Lock()
		if e.fenced {
			e.control.Unlock()
			break
		}
		_, beginErr := e.db.BeginGraphInput(ctx, e.config.OwnerInstanceID, view.Graph.ID, node.ID, facts)
		if persistenceFault(beginErr) {
			e.fenceLocked(sourceAttemptID, "graph_input_registration_uncertain", "Graph input registration lost its durable acknowledgement. Preserve its exact operation; do not replay it.")
		}
		e.control.Unlock()
		if beginErr == nil {
			queued = true
		}
	}
	if queued {
		e.signal()
	}
}
