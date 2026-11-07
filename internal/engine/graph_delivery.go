package engine

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Graph destination authority is separate from the candidate's node source.
// Only ApplyRequest.Source uses the independently verified original graph root.
type graphDeliveryPreparation struct {
	Facts        domain.GraphDeliveryFacts
	Final        *graphFinalVerification
	RootSource   *workspace.Snapshot
	EffectsKnown bool
	artifacts    []domain.GraphArtifactRef
	anchors      []domain.GraphArtifactRef
	subjectIDs   []string
}

func (e *Engine) prepareGraphDelivery(ctx context.Context, a domain.Attempt, subject workflowSubject) (*graphDeliveryPreparation, error) {
	task, err := e.db.Task(ctx, a.TaskID)
	if err != nil || task.Task.GraphRef == nil {
		return nil, err
	}
	member := task.Task.GraphRef
	if member.Kind != "node" || member.NodeID == nil || a.Role != "local_delivery" || a.ExecutorKind != "local_application" {
		return nil, domain.Fail("graph_final_required", "Only the explicitly scoped final node can prepare graph delivery.")
	}
	progress, err := e.inspectWorkflowProgress(ctx, a.TaskID, subject.Workflow)
	if err != nil {
		return nil, err
	}
	if progress.final == nil || progress.decision.Role != "local_delivery" || subject.Binding == nil || subject.Candidate == nil || progress.subject.Binding == nil ||
		*progress.subject.Binding != *subject.Binding || progress.subject.Workflow.ID != a.WorkflowID || progress.subject.Workflow.TaskVersion != a.TaskVersion {
		return nil, domain.Fail("graph_final_unqualified", "Current cumulative verification must qualify this exact selected final before delivery.")
	}
	final := progress.final
	definition, err := e.db.GraphDefinitionPrivate(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	view, err := e.db.Graph(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	if view.Graph.Stop != nil || view.Graph.FinalNodeID == nil || *view.Graph.FinalNodeID != *member.NodeID || final.Binding.FinalNodeID != *member.NodeID ||
		definition.Delivery.LocalMode != "apply_verified_local" || !reflect.DeepEqual(definition.Delivery, subject.Workflow.Authority.Delivery) || len(final.Input.ContributorVector) != len(view.Nodes)-1 {
		return nil, domain.Fail("graph_final_unqualified", "The original final destination scope or complete required accepted cohort is unavailable.")
	}
	fileAncestors, err := graphMemberFileAncestors(definition, view.Nodes, view.Graph.GraphVersion, *member.NodeID)
	if err != nil {
		return nil, err
	}
	all := append([]domain.GraphArtifactRef{}, final.ArtifactRefs...)
	anchors := []domain.GraphArtifactRef{final.BindingRef, final.Binding.CumulativeDeltaRef, final.Binding.OutputClosureRef,
		final.OutputClosure.Manifest, final.OutputClosure.Index, final.OutputClosure.StagedBlobs,
		final.Root.Manifest, final.Root.Index, final.Root.StagedBlobs, final.Input.Manifest, final.Input.Index, final.Input.StagedBlobs,
		final.Input.Provenance, final.Input.ContextPacket, final.Binding.GraphConditionsRef}
	for _, access := range final.Binding.OriginalAccess {
		anchors = append(anchors, access.ArtifactRef)
	}
	subjectIDs := append([]string{}, subject.artifactIDs()...)
	others := append([]domain.GraphContributorBinding{}, final.Input.ContributorVector...)
	for _, contribution := range others {
		var node *domain.GraphNodeSummary
		for i := range view.Nodes {
			if view.Nodes[i].ID == contribution.NodeID {
				node = &view.Nodes[i]
			}
		}
		if node == nil || node.ID == *member.NodeID || node.Stop != nil || node.TaskVersion != contribution.TaskVersion || node.AcceptedResult == nil ||
			node.AcceptedResult.AcceptanceID != contribution.AcceptanceID || node.AcceptedResult.ProducerAttemptID != contribution.ProducerAttemptID {
			return nil, domain.Fail("graph_input_stale", "Every required other node must retain its exact current accepted contribution.")
		}
		if contribution.CandidateBinding != nil && !fileAncestors[node.ID] {
			return nil, domain.Fail("graph_file_ancestry_unproven", "Required file results must reach the final through declared file dependencies.")
		}
		source, refs, err := e.graphDeliveryContribution(ctx, definition, final.Root, view.Nodes, *node)
		if err != nil {
			return nil, err
		}
		b := source.Binding
		actual := domain.GraphContributorBinding{NodeID: b.FromNodeID, TaskID: b.TaskID, TaskVersion: b.TaskVersion, AcceptanceKind: b.AcceptanceKind,
			AcceptanceID: b.AcceptanceID, ProducerAttemptID: b.ProducerAttemptID, InputID: b.InputID, CandidateBinding: b.CandidateBinding, ResponseBinding: b.ResponseBinding, OutputClosureRef: b.OutputClosureRef}
		if !reflect.DeepEqual(actual, contribution) {
			return nil, domain.Fail("graph_input_stale", "A current complete accepted binding differs from the selected final input.")
		}
		ids, err := e.graphDeliverySourceIDs(ctx, b)
		if err != nil {
			return nil, err
		}
		subjectIDs = append(subjectIDs, ids...)
		physical, err := e.workspace.VerifyGraphOutput(ctx, workspace.GraphOutputVerifyRequest{RootBinding: final.Root, Contribution: b, InputBinding: source.InputBinding})
		if err != nil {
			return nil, err
		}
		// Controller-owned receiving receipts are separate from workspace output
		// members. Retain both inventories so every SQL anchor is freshly read.
		all = append(append(all, physical.Artifacts...), refs...)
		anchors = append(anchors, refs...)
	}
	uses, _, _, ids, err := e.currentWorkflowEvidence(ctx, progress.subject, final)
	if err != nil {
		return nil, err
	}
	scopeSHA, err := domain.GraphDeliveryScopeDigest(definition.Delivery)
	if err != nil {
		return nil, err
	}
	binding := domain.GraphDeliveryBinding{GraphID: view.Graph.ID, GraphVersion: view.Graph.GraphVersion, GraphControlRevision: view.Graph.ControlRevision,
		RootBinding: domain.GraphRootPublic(final.Root), FinalNodeID: *member.NodeID, FinalTaskID: a.TaskID, FinalTaskVersion: a.TaskVersion,
		FinalWorkflowID: a.WorkflowID, FinalCandidateBinding: *subject.Binding, FinalInputID: final.Input.ID,
		ContributorVector: append([]domain.GraphContributorBinding{}, final.Input.ContributorVector...), FinalVerificationRef: final.BindingRef,
		OtherNodeAcceptances: others, PreDeliveryEvidenceUses: append([]domain.EvidenceUse{}, uses...), DeliveryScopeSHA256: scopeSHA}
	binding.BindingSHA256, err = domain.GraphDeliveryDigest(binding)
	if err != nil {
		return nil, err
	}
	prepared := &graphDeliveryPreparation{Facts: domain.GraphDeliveryFacts{Binding: binding}, Final: final, RootSource: final.RootSource, EffectsKnown: true, subjectIDs: subjectIDs}
	ref, err := e.retainGraphFinalJSON(ctx, a.ID, "graph_delivery", binding)
	if err != nil {
		prepared.EffectsKnown = !uncertainEvidence(err)
		return prepared, err
	}
	prepared.Facts.BindingRef = ref
	prepared.artifacts, prepared.anchors = append(all, ref), append(anchors, ref)
	// Rehash every retained member after the last metadata save.
	proofs, err := e.verifyGraphClosureAnchors(ctx, prepared.artifacts, prepared.anchors)
	if err != nil {
		return prepared, err
	}
	// Qualification bytes must also be fresh after the binding save; a closed
	// review/check envelope alone is not proof its retained payload survived it.
	qualification, err := e.verifyWorkflowArtifacts(ctx, append(append([]string{}, prepared.subjectIDs...), ids...)...)
	if err != nil {
		return prepared, err
	}
	prepared.Facts.VerifiedArtifacts, err = mergeGraphAcceptanceProofs(proofs, qualification)
	return prepared, err
}

// Registered node-local source snapshots remain part of the workflow subject,
// separately from graph input/root snapshots. Rehash their exact headers after
// the last save and before each effect, including a correction's earlier root.
func (e *Engine) graphDeliverySourceIDs(ctx context.Context, contribution domain.AcceptedContribution) ([]string, error) {
	var sourceID, rootID string
	if binding := contribution.CandidateBinding; binding != nil {
		sourceID, rootID = binding.SourceSnapshotID, binding.RootSourceSnapshotID
	} else if binding := contribution.ResponseBinding; binding != nil {
		producer, err := e.db.Attempt(ctx, binding.ProducerAttemptID)
		if err != nil {
			return nil, err
		}
		if producer.CandidateID == nil {
			return nil, evidence.ErrCorrupt
		}
		candidate, err := e.db.Candidate(ctx, *producer.CandidateID)
		if err != nil {
			return nil, err
		}
		sourceID, rootID = binding.SourceSnapshotID, candidate.RootSourceSnapshotID
	} else {
		return nil, evidence.ErrCorrupt
	}
	ids := []string{}
	for _, id := range []string{sourceID, rootID} {
		snapshot, err := e.db.SourceSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, snapshot.ManifestArtifactID, snapshot.IndexArtifactID)
	}
	return ids, nil
}

func (e *Engine) graphDeliveryContribution(ctx context.Context, definition domain.GraphDefinition, root domain.GraphRootBinding, nodes []domain.GraphNodeSummary, node domain.GraphNodeSummary) (workspace.GraphContributionSource, []domain.GraphArtifactRef, error) {
	// Reuse the accepted-source verifier with one actual outgoing edge. Never
	// manufacture a direct edge from a transitive ancestor to the final.
	if definition.Original.Dependencies != nil {
		for _, edge := range *definition.Original.Dependencies {
			if edge.From != node.Key {
				continue
			}
			kind := edge.Kind
			if kind == "" {
				kind = "files"
			}
			for _, receiver := range nodes {
				if receiver.Key == edge.To {
					id, err := domain.GraphEdgeID(definition.GraphID, definition.GraphVersion, node.ID, receiver.ID, kind)
					if err != nil {
						return workspace.GraphContributionSource{}, nil, err
					}
					return e.graphCurrentAcceptedContribution(ctx, root, node, id, kind)
				}
			}
		}
	}
	return workspace.GraphContributionSource{}, nil, evidence.ErrCorrupt
}

// Rehash retained members outside control/SQL before each readiness or permit
// transaction. Store independently checks current registered authority.
func (e *Engine) refreshGraphDelivery(ctx context.Context, a domain.Attempt, subject workflowSubject, prepared *graphDeliveryPreparation) (*domain.GraphDeliveryFacts, error) {
	if prepared == nil {
		return nil, nil
	}
	progress, err := e.inspectWorkflowProgress(ctx, a.TaskID, subject.Workflow)
	if err != nil {
		return nil, err
	}
	if progress.final == nil || progress.subject.Binding == nil || *progress.subject.Binding != prepared.Facts.Binding.FinalCandidateBinding ||
		progress.final.BindingRef != prepared.Facts.Binding.FinalVerificationRef {
		return nil, domain.Fail("graph_input_stale", "The exact cumulative final changed before its next delivery permit.")
	}
	proofs, err := e.verifyGraphClosureAnchors(ctx, prepared.artifacts, prepared.anchors)
	if err != nil {
		return nil, err
	}
	proofs, err = mergeGraphAcceptanceProofs(proofs, progress.final.VerifiedArtifacts)
	if err != nil {
		return nil, err
	}
	qualification, err := e.verifyWorkflowArtifacts(ctx, append(append([]string{}, prepared.subjectIDs...), progress.artifactIDs...)...)
	if err != nil {
		return nil, err
	}
	prepared.Final = progress.final
	prepared.Facts.VerifiedArtifacts, err = mergeGraphAcceptanceProofs(proofs, qualification)
	if err != nil {
		return nil, err
	}
	return &prepared.Facts, nil
}
