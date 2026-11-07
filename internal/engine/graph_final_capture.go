package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// This comparison never rewrites node-local authorship or candidate roots.
type graphCumulativeDelta struct {
	Contract              string                           `json:"contract"`
	GraphID               string                           `json:"graphId"`
	GraphVersion          string                           `json:"graphVersion"`
	FinalNodeID           string                           `json:"finalNodeId"`
	FinalTaskID           string                           `json:"finalTaskId"`
	FinalTaskVersion      string                           `json:"finalTaskVersion"`
	FinalWorkflowID       string                           `json:"finalWorkflowId"`
	RootBinding           domain.GraphRootView             `json:"rootBinding"`
	FinalCandidateBinding domain.CandidateBinding          `json:"finalCandidateBinding"`
	OutputClosureRef      domain.GraphArtifactRef          `json:"outputClosureRef"`
	FinalInputID          string                           `json:"finalInputId"`
	ContributorVector     []domain.GraphContributorBinding `json:"contributorVector"`
	Changes               []workspace.Change               `json:"changes"`
}

func (e *Engine) retainGraphFinalJSON(ctx context.Context, attemptID, kind string, value any) (domain.GraphArtifactRef, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	if int64(len(body)) > workspace.MaxFileBytes {
		return domain.GraphArtifactRef{}, domain.Fail("input_too_large", "Complete final verification evidence exceeds 64 MiB; no content was clipped.")
	}
	meta, err := e.evidence.Put(ctx, attemptID, kind, body)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	if meta.SHA256 == nil || !meta.Closed || meta.Completeness != "complete" {
		return domain.GraphArtifactRef{}, evidence.ErrCorrupt
	}
	return domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}, nil
}

func graphFinalOriginalAccess(conditions domain.GraphConditionsBinding, packet domain.GraphContextPacket) ([]domain.GraphProvidedAccess, error) {
	required := map[string]domain.GraphArtifactRef{}
	for _, source := range conditions.Sources {
		for _, ref := range []*domain.GraphArtifactRef{&source.OriginalRef, source.GuidanceRef, source.AcceptanceRef} {
			if ref != nil {
				if old, found := required[ref.ArtifactID]; found && old != *ref {
					return nil, evidence.ErrCorrupt
				}
				required[ref.ArtifactID] = *ref
			}
		}
	}
	originalAccess := []domain.GraphProvidedAccess{}
	for _, access := range packet.ProvidedAccess {
		if ref, found := required[access.ArtifactRef.ArtifactID]; found {
			if access.ArtifactRef != ref {
				return nil, evidence.ErrCorrupt
			}
			originalAccess = append(originalAccess, access)
			delete(required, ref.ArtifactID)
		}
	}
	if len(required) != 0 || len(originalAccess) == 0 {
		return nil, domain.Fail("graph_context_unavailable", "Final verification requires complete access to every scoped original, guidance and acceptance source.")
	}
	return originalAccess, nil
}

func (e *Engine) captureGraphFinalVerification(ctx context.Context, a domain.Attempt, d domain.GraphDispatch, input domain.GraphInputBinding, binding domain.CandidateBinding, captured workspace.GraphOutputCapture, facts *domain.GraphStepOutputFacts) error {
	graph, err := e.db.Graph(ctx, d.GraphID)
	if err != nil {
		return err
	}
	if graph.Graph.FinalNodeID == nil || *graph.Graph.FinalNodeID != d.NodeID {
		return nil
	}
	version, err := e.db.TaskVersion(ctx, d.TaskID, d.TaskVersion)
	if err != nil {
		return err
	}
	if version.Version.Acceptance.Deliverable == "response_only" {
		return nil
	}
	if facts == nil || facts.CandidateClosure == nil || facts.Observation.Candidate == nil || facts.Observation.Candidate.ClosureRef == nil ||
		!reflect.DeepEqual(*facts.CandidateClosure, captured.Closure) || *facts.Observation.Candidate.ClosureRef != captured.ClosureArtifact ||
		graph.Graph.GraphVersion != d.GraphVersion || graph.Graph.RootTaskID != d.BudgetOwnerTaskID ||
		input.ID != d.InputID || input.InputSHA256 != d.InputSHA256 || input.ReceivingNodeID != d.NodeID || input.ReceivingTaskID != d.TaskID || input.ReceivingTaskVersion != d.TaskVersion || !reflect.DeepEqual(input.ContributorVector, d.ContributorVector) ||
		captured.Closure.GraphID != d.GraphID || captured.Closure.NodeID != d.NodeID || captured.Closure.ProducerAttemptID != a.ID || captured.Closure.ReceivedInputID != input.ID ||
		captured.Closure.CandidateID == nil || *captured.Closure.CandidateID != binding.CandidateID ||
		captured.Closure.Manifest.ArtifactID != binding.ManifestArtifactID || captured.Closure.Manifest.SHA256 != binding.ManifestSHA256 || captured.Closure.Index.ArtifactID != binding.IndexArtifactID || captured.Closure.Index.SHA256 != binding.IndexSHA256 {
		return evidence.ErrCorrupt
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, d.GraphID)
	if err != nil {
		return err
	}
	root, err := e.db.GraphRoot(ctx, d.GraphID)
	if err != nil {
		return err
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		return err
	}
	rootProof, err := e.workspace.VerifyGraphRoot(ctx, root, rootSource)
	if err != nil {
		return err
	}
	output, err := e.workspace.VerifyCapturedGraphOutput(ctx, captured)
	if err != nil {
		return err
	}
	record, err := e.db.GraphInputRecord(ctx, d.GraphID, input.ID)
	if err != nil {
		return err
	}
	if record.PrivateRoot == nil || !reflect.DeepEqual(*record.PrivateRoot, root) || record.Public.InputBinding == nil || !reflect.DeepEqual(*record.Public.InputBinding, input) || record.Public.ConditionsRef == nil || record.ConditionsBinding == nil {
		return evidence.ErrCorrupt
	}
	verifiedInput, err := e.workspace.VerifyGraphInput(ctx, input)
	if err != nil {
		return err
	}
	conditionsBody, err := e.readGraphArtifact(ctx, *record.Public.ConditionsRef, "graph_conditions", 64<<20)
	if err != nil {
		return err
	}
	var conditions domain.GraphConditionsBinding
	if json.Unmarshal(conditionsBody, &conditions, json.RejectUnknownMembers(true)) != nil || domain.ValidateGraphConditionsBinding(conditions) != nil ||
		!reflect.DeepEqual(conditions, *record.ConditionsBinding) || conditions.GraphID != d.GraphID || conditions.GraphVersion != d.GraphVersion || conditions.ReceiverTaskID != d.TaskID || conditions.ReceiverTaskVersion != d.TaskVersion {
		return evidence.ErrCorrupt
	}
	seal, err := domain.GraphConditionsDigest(conditions)
	if err != nil || seal != conditions.ConditionsSHA256 {
		return evidence.ErrCorrupt
	}
	originalAccess, err := graphFinalOriginalAccess(conditions, verifiedInput.ContextPacket)
	if err != nil {
		return err
	}
	changes := workspace.CompareManifests(rootProof.Manifest, output.Manifest)
	workingDelta, indexDelta := workflowChangeFlags(changes)
	delta := graphCumulativeDelta{Contract: "006-cumulative-delta-v1", GraphID: d.GraphID, GraphVersion: d.GraphVersion, FinalNodeID: d.NodeID, FinalTaskID: d.TaskID, FinalTaskVersion: d.TaskVersion, FinalWorkflowID: d.WorkflowID,
		RootBinding: domain.GraphRootPublic(root), FinalCandidateBinding: binding, OutputClosureRef: captured.ClosureArtifact, FinalInputID: input.ID, ContributorVector: input.ContributorVector, Changes: changes}
	deltaRef, err := e.retainGraphFinalJSON(ctx, a.ID, "graph_cumulative_delta", delta)
	if err != nil {
		return err
	}
	final := domain.GraphFinalVerificationBinding{GraphID: d.GraphID, GraphVersion: d.GraphVersion, FinalNodeID: d.NodeID, FinalTaskID: d.TaskID, FinalTaskVersion: d.TaskVersion, FinalWorkflowID: d.WorkflowID,
		RootBinding: domain.GraphRootPublic(root), FinalCandidateBinding: binding, OutputClosureRef: captured.ClosureArtifact, FinalInputID: input.ID, ContributorVector: input.ContributorVector,
		GraphConditionsRef: *record.Public.ConditionsRef, GraphConditionsSHA256: conditions.ConditionsSHA256, OriginalAccess: originalAccess, CumulativeDeltaRef: deltaRef, HasCumulativeWorkingDelta: workingDelta, HasCumulativeIndexDelta: indexDelta}
	final.BindingSHA256, err = domain.GraphFinalVerificationDigest(final)
	if err != nil {
		return err
	}
	finalRef, err := e.retainGraphFinalJSON(ctx, a.ID, "graph_final_verification", final)
	if err != nil {
		return err
	}
	// Verify every retained member after the final metadata save. Only bounded
	// anchors enter SQL; none of these reads prepares a workspace or model call.
	all := append(append(append([]domain.GraphArtifactRef{}, rootProof.Artifacts...), output.Artifacts...), verifiedInput.ArtifactRefs...)
	anchors := []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs, input.Manifest, input.Index, input.StagedBlobs, input.Provenance, input.ContextPacket, *record.Public.ConditionsRef,
		captured.ClosureArtifact, captured.Closure.Manifest, captured.Closure.Index, captured.Closure.StagedBlobs, deltaRef, finalRef}
	for _, access := range originalAccess {
		anchors = append(anchors, access.ArtifactRef)
	}
	proofs, err := e.verifyGraphClosureAnchors(ctx, append(all, anchors...), anchors)
	if err != nil {
		return err
	}
	facts.VerifiedArtifacts, err = mergeGraphAcceptanceProofs(facts.VerifiedArtifacts, proofs)
	if err != nil {
		return err
	}
	facts.FinalVerification, facts.Observation.Candidate.FinalVerificationRef = &final, &finalRef
	return nil
}
