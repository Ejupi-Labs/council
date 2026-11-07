package engine

import (
	"context"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// Explicit intent is replayed before filesystem inspection. Only the current
// owner constructs facts; the transaction allocates receiving operation IDs.
func (e *Engine) advanceGraph(ctx context.Context, graphID string, in domain.GraphAdvanceInput) domain.Result {
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	if prior, err := e.db.Replay(ctx, in.RequestID, "graphs.advance", store.GraphAdvanceFingerprint(graphID, in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if prior != nil {
		return *prior
	}
	facts, err := e.captureGraphAdvanceFacts(ctx, graphID, in)
	if err != nil {
		if uncertainEvidence(err) {
			e.fence("", "graph_advance_evidence_uncertain", "The exact retained graph inputs cannot be verified. Preserve their evidence before continuing.")
		}
		return e.failure(in.RequestID, err)
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Resolve retained ownership or persistence uncertainty before advancing graph work."))
	}
	result := e.db.AdvanceGraph(ctx, e.config.OwnerInstanceID, graphID, in, facts)
	e.guardMutationLocked(result)
	e.control.Unlock()
	if result.Envelope.Error == nil {
		e.signal()
	}
	return result
}

func (e *Engine) captureGraphAdvanceFacts(ctx context.Context, graphID string, in domain.GraphAdvanceInput) (domain.GraphAdvanceFacts, error) {
	facts := domain.GraphAdvanceFacts{}
	view, err := e.db.Graph(ctx, graphID)
	if err != nil {
		return facts, err
	}
	if view.Graph.GraphVersion != in.GraphVersion || view.Graph.ControlRevision != in.ExpectedGraphControlRevision {
		return facts, domain.Fail("graph_control_conflict", "Capture the current graph controls before explicit Advance.")
	}
	if view.Graph.BudgetAuthorityEpoch != in.ExpectedBudgetAuthorityEpoch || view.Budget.Budget.CurrentWindow.Number != in.ExpectedWindowNumber {
		return facts, domain.Fail("budget_conflict", "Capture the current graph budget authority and window before explicit Advance.")
	}
	if len(in.NodeAuthorities) != len(view.Nodes) {
		return facts, domain.Invalid("the complete captured graph node authority vector")
	}
	for i, node := range view.Nodes {
		authority := in.NodeAuthorities[i]
		if authority.NodeID != node.ID {
			return facts, domain.Fail("graph_control_conflict", "The captured node identities no longer match the immutable graph declaration.")
		}
	}
	// A still-owned initial root operation is never recreated by Advance.
	if view.Graph.RootInputID == nil || view.Graph.Stop != nil {
		return facts, nil
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, graphID)
	if err != nil {
		return facts, err
	}
	record, err := e.db.GraphInputRecord(ctx, graphID, *view.Graph.RootInputID)
	if err != nil {
		return facts, err
	}
	if record.PrivateRoot == nil || record.Public.Summary.State != "ready" || !record.Public.Summary.Current {
		return facts, domain.Fail("graph_input_stale", "The graph root must be retained and current before preparing another input.")
	}
	root := *record.PrivateRoot
	source, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		return facts, err
	}
	rootProofs, err := e.verifyGraphRootClosure(ctx, root, source)
	if err != nil {
		return facts, err
	}
	generation := "1"
	if view.DispatchAuthority != nil {
		generation = view.DispatchAuthority.Generation
	}
	rebind := map[string]bool{}
	if in.RebindNodeIDs != nil {
		for _, id := range *in.RebindNodeIDs {
			rebind[id] = true
		}
	}
	for _, node := range view.Nodes {
		if node.Stop != nil {
			continue
		}
		// Fresh intent cannot reopen an unlaunched descendant of a stopped
		// ancestor. Admitted workflows retain their separate first-GO rules.
		if node.NodeAdmissionID == nil && graphAdvanceStoppedAncestor(view, definition, node) {
			continue
		}
		nodeID, taskID, version, revision := node.ID, node.TaskID, node.TaskVersion, node.ControlRevision
		control := domain.GraphDispatchBinding{GraphID: graphID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID,
			OwnerInstanceID: e.config.OwnerInstanceID, Generation: generation, GraphControlRevision: view.Graph.ControlRevision,
			BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
			NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision}
		if rebind[node.ID] || node.InputID == nil && node.NodeAdmissionID == nil && (node.State == "waiting" || node.State == "blocked") {
			if err := e.checkGraphAdvanceScope(ctx, view, definition, node, in, false); err != nil {
				return facts, err
			}
			selected, selectErr := e.selectGraphContributions(ctx, definition, root, node)
			if selectErr != nil {
				var refusal *domain.Error
				if !rebind[node.ID] && errors.As(selectErr, &refusal) && refusal.Code == "graph_dependency_unaccepted" {
					continue
				}
				return facts, selectErr
			}
			proofs, err := mergeGraphAcceptanceProofs(rootProofs, selected.VerifiedArtifacts)
			if err != nil {
				return facts, err
			}
			contributions := []domain.AcceptedContribution{}
			for _, source := range selected.Sources {
				contributions = append(contributions, source.Binding)
			}
			control.ContributorVector = selected.ContributorVector
			facts.Preparations = append(facts.Preparations, domain.GraphInputPreparationFacts{Trigger: domain.GraphFreshTrigger{Kind: "explicit", ID: in.RequestID, OwnerGeneration: generation},
				Control: control, RootBinding: root, Contributions: contributions, ContributorVector: selected.ContributorVector, VerifiedArtifacts: proofs})
		} else if node.InputID != nil && node.NodeAdmissionID == nil && (node.State == "ready" || node.State == "blocked") {
			// A fresh explicit intent may recheck a retained initial input after a
			// known pre-admission block. Grants and release callbacks cannot enter
			// this path; current scope, complete input and Store guards still bind.
			if err := e.checkGraphAdvanceScope(ctx, view, definition, node, in, false); err != nil {
				return facts, err
			}
			admission, err := e.graphInitialAdmissionFacts(ctx, definition, node, control)
			if err != nil {
				if uncertainEvidence(err) || ctx.Err() != nil {
					return facts, err
				}
				admission.PreparationFailure = domain.SafeError(err)
			}
			facts.Admissions = append(facts.Admissions, admission)
		} else if node.InputID != nil && node.NodeAdmissionID != nil && node.State != "completed" && node.State != "failed" && node.State != "interrupted" && node.State != "cancelled" {
			stage, err := e.captureGraphAdvanceStage(ctx, view, definition, node, in)
			if err != nil {
				return facts, err
			}
			if stage != nil {
				facts.Stages = append(facts.Stages, *stage)
			}
		}
	}
	return facts, nil
}

func graphAdvanceStoppedAncestor(view domain.GraphDetail, definition domain.GraphDefinition, selected domain.GraphNodeSummary) bool {
	ancestors := map[string]bool{selected.Key: true}
	if definition.Original.Dependencies != nil {
		for range view.Nodes {
			for _, edge := range *definition.Original.Dependencies {
				if ancestors[edge.To] {
					ancestors[edge.From] = true
				}
			}
		}
	}
	for _, node := range view.Nodes {
		if node.ID != selected.ID && ancestors[node.Key] && node.Stop != nil {
			return true
		}
	}
	return false
}

// An explicit resume may name a healthy retained predecessor of an older
// owner. Its receipt authorizes only the next stage of this same workflow;
// Failed checks may authorize a correction through their retained canonical
// proof; failed native or uncertain work cannot acquire replay authority.
func (e *Engine) captureGraphAdvanceStage(ctx context.Context, view domain.GraphDetail, definition domain.GraphDefinition, node domain.GraphNodeSummary, in domain.GraphAdvanceInput) (*domain.WorkflowStepAdmissionFacts, error) {
	anchor, err := e.db.GraphDispatch(ctx, node.GraphID, *node.NodeAdmissionID)
	if err != nil {
		return nil, err
	}
	d := anchor.Dispatch
	if d.NodeID != node.ID || d.TaskID != node.TaskID || d.TaskVersion != node.TaskVersion || d.InputID != *node.InputID {
		return nil, domain.Fail("graph_input_stale", "Explicit resume requires this node's unchanged logical admission and receiving input.")
	}
	selected, err := e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{WorkflowID: d.WorkflowID})
	if err != nil {
		return nil, err
	}
	if selected.Workflow == nil || selected.Workflow.TaskVersion != node.TaskVersion || selected.Workflow.CurrentStepID == nil || selected.Workflow.State == "completed" {
		return nil, nil
	}
	w := *selected.Workflow
	step, err := e.db.WorkflowStep(ctx, *w.CurrentStepID)
	if err != nil {
		return nil, err
	}
	if step.AttemptID == nil || step.AdvanceProof == nil {
		return nil, nil
	}
	previous, err := e.db.Attempt(ctx, *step.AttemptID)
	if err != nil {
		return nil, err
	}
	if previous.Phase != "settled" || previous.Outcome == nil || previous.Cleanup.State != "verified" || previous.EvidenceStatus != "complete" || previous.PrimaryFailure != nil || previous.Stop != nil {
		return nil, nil
	}
	progress, err := e.inspectWorkflowProgress(ctx, node.TaskID, w)
	if err != nil {
		return nil, err
	}
	if progress.decision.Role == "" {
		return nil, nil
	}
	if *previous.Outcome != "succeeded" {
		// A complete failed check is evidence for a correction, not an attempt
		// to replay that command or retry failed native work. Store revalidates
		// the unchanged predecessor's exact canonical next-stage proof.
		qualifying := false
		if *previous.Outcome == "failed" && previous.Role == "check" && previous.ExecutorKind == "local_check" && progress.decision.Role == "correction" {
			for _, check := range progress.checks {
				if check.AttemptID == previous.ID && check.Outcome == "failed" && !check.SourceChanged && check.RootTerminal && check.InputComplete && check.OutputComplete && check.ExitCode != nil && check.Cleanup.State == "verified" {
					qualifying = true
				}
			}
		}
		if !qualifying {
			return nil, nil
		}
	}
	known, err := e.db.GraphDispatchForAttempt(ctx, d.AttemptID)
	if err != nil {
		return nil, err
	}
	started := known.Public.Dispatch.ID == d.ID && known.KnownFirstGOAt != nil && known.KnownFirstGOAttemptID != nil
	if err := e.checkGraphAdvanceScope(ctx, view, definition, node, in, started); err != nil {
		return nil, err
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, step.ID, &in)
	if err != nil {
		return nil, err
	}
	return &facts, nil
}

// Unrelated retained/stopped tuples remain captured history, not authority for
// their newer versions. Only a selected node and its declared ancestors guard
// this action. Store repeats these comparisons in the mutation transaction.
func (e *Engine) checkGraphAdvanceScope(ctx context.Context, view domain.GraphDetail, definition domain.GraphDefinition, selected domain.GraphNodeSummary, in domain.GraphAdvanceInput, knownFirstGO bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	relevant := map[string]bool{selected.Key: true}
	if definition.Original.Dependencies != nil {
		for range view.Nodes {
			for _, edge := range *definition.Original.Dependencies {
				if relevant[edge.To] {
					relevant[edge.From] = true
				}
			}
		}
	}
	for i, node := range view.Nodes {
		if !relevant[node.Key] {
			continue
		}
		captured := in.NodeAuthorities[i]
		if captured.TaskVersion == node.TaskVersion && captured.NodeControlRevision == node.ControlRevision {
			continue
		}
		if node.ID != selected.ID && knownFirstGO && node.Stop != nil && node.Stop.NodeControlRevision != nil && *node.Stop.NodeControlRevision == node.ControlRevision && captured.TaskVersion == node.TaskVersion {
			beforeStop, invalid := domain.SubtractRoutingCounts(node.ControlRevision, "1")
			if invalid == nil && captured.NodeControlRevision == beforeStop {
				continue
			}
		}
		return domain.Fail("graph_control_conflict", "A selected node or relevant predecessor changed after this graph intent was captured.")
	}
	return nil
}
