package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Acceptance verifies retained selected data. It does not acquire native
// dispatch authority, prepare a workspace or require a fresh execution window.
func (e *Engine) graphAcceptanceArtifactProofs(ctx context.Context, subject workflowSubject) ([]domain.VerifiedResponse, error) {
	detail, err := e.db.Task(ctx, subject.Workflow.TaskID)
	if err != nil {
		return nil, err
	}
	if detail.Task.GraphRef == nil {
		return nil, nil
	}
	member := detail.Task.GraphRef
	stale := func() error {
		return domain.Fail("graph_input_stale", "Acknowledge only the exact current graph node input and selected producer.")
	}
	if member.Kind != "node" || member.NodeID == nil || detail.Task.CurrentVersion != subject.Workflow.TaskVersion {
		return nil, stale()
	}
	view, err := e.db.Workflow(ctx, subject.Workflow.TaskID, domain.WorkflowSelector{})
	if err != nil {
		return nil, err
	}
	if view.Workflow == nil || view.Workflow.ID != subject.Workflow.ID || view.Workflow.TaskVersion != detail.Task.CurrentVersion {
		return nil, stale()
	}
	// This fresh loader reads the actual selected/root manifests and re-runs
	// verifyWorkflowClosure over ALL working payloads. In particular a new
	// unstaged file may not occur in either the input or selected staged index.
	fresh, err := e.selectedWorkflowSubject(ctx, *view.Workflow)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(fresh.Binding, subject.Binding) || !reflect.DeepEqual(fresh.Response, subject.Response) {
		return nil, stale()
	}
	producer := ""
	if fresh.Binding != nil && fresh.Candidate != nil && fresh.Response == nil {
		producer = fresh.Candidate.AttemptID
	}
	if fresh.Response != nil && fresh.Binding == nil {
		producer = fresh.Response.ProducerAttemptID
	}
	if !domain.ValidID(producer) {
		return nil, stale()
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, producer)
	if err != nil {
		return nil, err
	}
	d := record.Public.Dispatch
	if d.AttemptID != producer || d.GraphID != member.GraphID || d.NodeID != *member.NodeID || d.TaskID != detail.Task.ID || d.TaskVersion != detail.Task.CurrentVersion || d.WorkflowID != fresh.Workflow.ID || d.BudgetOwnerTaskID != member.RootTaskID {
		return nil, stale()
	}
	produced, err := e.db.Attempt(ctx, producer)
	if err != nil {
		return nil, err
	}
	if produced.TaskID != d.TaskID || produced.TaskVersion != d.TaskVersion || produced.WorkflowID != d.WorkflowID || produced.StepID != d.StepID || produced.ExecutorKind != d.ExecutorKind || produced.Role != d.Role || produced.Phase != "settled" || produced.Outcome == nil || *produced.Outcome != "succeeded" || produced.Cleanup.State != "verified" || produced.EvidenceStatus != "complete" {
		return nil, stale()
	}
	graph, err := e.db.Graph(ctx, d.GraphID)
	if err != nil {
		return nil, err
	}
	if graph.Graph.RootTaskID != member.RootTaskID || graph.Graph.GraphVersion != d.GraphVersion {
		return nil, stale()
	}
	matched := false
	for _, node := range graph.Nodes {
		if node.ID == d.NodeID && node.TaskID == d.TaskID && node.TaskVersion == d.TaskVersion && node.InputID != nil && *node.InputID == d.InputID && node.NodeAdmissionID != nil && *node.NodeAdmissionID == d.NodeAdmissionID {
			matched = true
			break
		}
	}
	if !matched {
		return nil, stale()
	}
	anchor, err := e.db.GraphDispatch(ctx, d.GraphID, d.NodeAdmissionID)
	if err != nil {
		return nil, err
	}
	a := anchor.Dispatch
	if a.ID != d.NodeAdmissionID || a.NodeAdmissionID != a.ID || a.GraphID != d.GraphID || a.NodeID != d.NodeID || a.TaskID != d.TaskID || a.TaskVersion != d.TaskVersion || a.WorkflowID != d.WorkflowID || a.InputID != d.InputID || a.InputSHA256 != d.InputSHA256 || a.BudgetOwnerTaskID != member.RootTaskID {
		return nil, stale()
	}
	input, err := e.db.GraphInputRecord(ctx, d.GraphID, d.InputID)
	if err != nil {
		return nil, err
	}
	binding := input.Public.InputBinding
	if input.Public.Summary.State != "ready" || !input.Public.Summary.Current || binding == nil || input.PrivateRoot == nil || input.Public.ConditionsRef == nil || input.ConditionsBinding == nil || binding.ID != d.InputID || binding.GraphID != d.GraphID || binding.GraphVersion != d.GraphVersion || binding.InputSHA256 != d.InputSHA256 || binding.ReceivingNodeID != d.NodeID || binding.ReceivingTaskID != d.TaskID || binding.ReceivingTaskVersion != d.TaskVersion || binding.ContextPacket != d.ContextPacket || !reflect.DeepEqual(binding.ContributorVector, d.ContributorVector) || input.PrivateRoot.GraphID != d.GraphID || input.PrivateRoot.RootTaskID != member.RootTaskID || input.PrivateRoot.BasisSHA256 != binding.RootBasisSHA256 {
		return nil, stale()
	}
	verified, err := e.workspace.VerifyGraphInput(ctx, *binding)
	if err != nil {
		return nil, err
	}
	body, err := e.readGraphArtifact(ctx, *input.Public.ConditionsRef, "graph_conditions", 64<<20)
	if err != nil {
		return nil, err
	}
	var conditions domain.GraphConditionsBinding
	if err := json.Unmarshal(body, &conditions, json.RejectUnknownMembers(true)); err != nil || domain.ValidateGraphConditionsBinding(conditions) != nil || !reflect.DeepEqual(conditions, *input.ConditionsBinding) || conditions.GraphID != d.GraphID || conditions.GraphVersion != d.GraphVersion || conditions.ReceiverTaskID != d.TaskID || conditions.ReceiverTaskVersion != d.TaskVersion {
		return nil, evidence.ErrCorrupt
	}
	seal, err := domain.GraphConditionsDigest(conditions)
	if err != nil || seal != conditions.ConditionsSHA256 {
		return nil, evidence.ErrCorrupt
	}
	_, own, err := workflowpolicy.ConditionDigests(detail.Task.OriginalText, detail.Version, fresh.Workflow.Authority.Conditions, fresh.Workflow.Authority.Checks)
	if err != nil {
		return nil, err
	}
	effective, err := graphEffectiveConditionsDigest(own, seal)
	if err != nil || effective != fresh.Workflow.Authority.ConditionsSHA256 {
		return nil, evidence.ErrCorrupt
	}
	manifestID := fresh.Source.ManifestArtifactID
	if fresh.Binding != nil {
		manifestID = fresh.Binding.ManifestArtifactID
	}
	manifest, err := e.evidence.VerifyClosed(ctx, manifestID)
	if err != nil {
		return nil, err
	}
	index, err := e.evidence.VerifyClosed(ctx, fresh.ResponseIndexID())
	if err != nil {
		return nil, err
	}
	if manifest.SHA256 == nil || index.SHA256 == nil {
		return nil, evidence.ErrCorrupt
	}
	workingHash, indexHash, err := workspace.GraphManifestDigests(fresh.Manifest)
	if err != nil {
		return nil, err
	}
	basis := domain.GraphSourceBasis{BasisKind: "node_subject", WorkflowID: d.WorkflowID, InputID: d.InputID, InputSHA256: d.InputSHA256,
		CandidateBinding: fresh.Binding, ResponseBinding: fresh.Response,
		ExpectedManifest:             domain.GraphArtifactRef{ArtifactID: manifest.ID, ProducerAttemptID: manifest.AttemptID, SHA256: *manifest.SHA256, ByteLength: manifest.ByteLength},
		ExpectedIndex:                domain.GraphArtifactRef{ArtifactID: index.ID, ProducerAttemptID: index.AttemptID, SHA256: *index.SHA256, ByteLength: index.ByteLength},
		ExpectedWorkingEntriesSHA256: workingHash, ExpectedIndexEntriesSHA256: indexHash}
	output, err := e.graphSelectedStageOutput(ctx, fresh, d, basis)
	if err != nil {
		return nil, err
	}
	if len(output) != 4 {
		return nil, evidence.ErrCorrupt
	}
	closureBody, err := e.readGraphArtifact(ctx, output[0], "graph_output_closure", 64<<20)
	if err != nil {
		return nil, err
	}
	var closure domain.GraphOutputClosure
	if err := json.Unmarshal(closureBody, &closure, json.RejectUnknownMembers(true)); err != nil {
		return nil, evidence.ErrCorrupt
	}
	if fresh.Binding != nil && (closure.CandidateID == nil || *closure.CandidateID != fresh.Binding.CandidateID || closure.ResponseArtifact != nil) {
		return nil, evidence.ErrCorrupt
	}
	anchors := []domain.GraphArtifactRef{binding.Manifest, binding.Index, binding.StagedBlobs, binding.Provenance, binding.ContextPacket, *input.Public.ConditionsRef}
	anchors = append(anchors, output...)
	if fresh.Response != nil {
		r := fresh.Response
		if closure.CandidateID != nil || closure.ResponseArtifact == nil || closure.ResponseArtifact.ArtifactID != r.ResponseArtifactID || closure.ResponseArtifact.ProducerAttemptID != producer || closure.ResponseArtifact.SHA256 != r.ResponseSHA256 || closure.ResponseArtifact.ByteLength != r.ResponseByteLength {
			return nil, evidence.ErrCorrupt
		}
		anchors = append(anchors, *closure.ResponseArtifact)
	}
	all := append(append([]domain.GraphArtifactRef{}, verified.ArtifactRefs...), anchors...)
	return e.verifyGraphClosureAnchors(ctx, all, anchors)
}

func mergeGraphAcceptanceProofs(ordinary, graph []domain.VerifiedResponse) ([]domain.VerifiedResponse, error) {
	if len(graph) == 0 {
		return ordinary, nil
	}
	result := make([]domain.VerifiedResponse, 0, len(ordinary)+len(graph))
	seen := map[string]domain.VerifiedResponse{}
	for _, group := range [][]domain.VerifiedResponse{ordinary, graph} {
		for _, proof := range group {
			if previous, ok := seen[proof.ArtifactID]; ok {
				if previous != proof {
					return nil, evidence.ErrCorrupt
				}
				continue
			}
			seen[proof.ArtifactID] = proof
			result = append(result, proof)
		}
	}
	if len(result) > 256 {
		return nil, evidence.ErrLimit
	}
	return result, nil
}
