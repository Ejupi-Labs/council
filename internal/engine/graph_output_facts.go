package engine

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Subject eligibility is decided by the existing workflow reducer. This helper
// independently checks graph dispatch, role and exact retained source ownership.
type graphStepOutputRequest struct {
	Attempt                 domain.Attempt
	Snapshot                *workspace.Snapshot
	CandidateBinding        *domain.CandidateBinding
	EligibleResponseBinding *domain.ResponseBinding
}

func (e *Engine) captureGraphStepOutput(ctx context.Context, request graphStepOutputRequest) (facts *domain.GraphStepOutputFacts, captureErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := e.db.Attempt(ctx, request.Attempt.ID)
	if err != nil {
		return nil, err
	}
	a := request.Attempt
	if current.TaskID != a.TaskID || current.TaskVersion != a.TaskVersion || current.OwnerInstanceID != a.OwnerInstanceID ||
		current.WorkflowID != a.WorkflowID || current.StepID != a.StepID || current.Role != a.Role || current.ExecutorKind != a.ExecutorKind {
		return nil, domain.Fail("graph_output_stale", "Capture graph output only for the exact registered producing attempt.")
	}
	task, err := e.db.Task(ctx, current.TaskID)
	if err != nil {
		return nil, err
	}
	ref := task.Task.GraphRef
	if ref == nil {
		return nil, nil
	}
	if ref.Kind != "node" || ref.NodeID == nil || current.OwnerInstanceID != e.config.OwnerInstanceID || current.Phase == "settled" || current.SettledAt != nil {
		return nil, domain.Fail("graph_output_unavailable", "Only an unsettled owned node dispatch can retain graph output.")
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	d := record.Public.Dispatch
	if d.AttemptID != current.ID || d.GraphID != ref.GraphID || d.NodeID != *ref.NodeID || d.TaskID != current.TaskID || d.TaskVersion != current.TaskVersion ||
		d.WorkflowID != current.WorkflowID || d.StepID != current.StepID || d.Role != current.Role || d.ExecutorKind != current.ExecutorKind ||
		d.OwnerInstanceID != current.OwnerInstanceID || d.BudgetOwnerTaskID != ref.RootTaskID || record.Public.OutputObservation != nil {
		return nil, domain.Fail("graph_output_stale", "The producing graph dispatch does not match its exact captured node and workflow step.")
	}
	facts = &domain.GraphStepOutputFacts{Observation: domain.GraphOutputObservation{ProducerAttemptID: current.ID}, VerifiedArtifacts: []domain.VerifiedResponse{}}
	if current.CandidateID != nil {
		facts.Observation.Candidate = &domain.GraphCandidateOutputObservation{CandidateID: *current.CandidateID, State: "incomplete"}
	}
	if current.ResponseArtifactID != nil {
		facts.Observation.Response = &domain.GraphResponseOutputObservation{ResponseArtifactID: *current.ResponseArtifactID, State: "incomplete"}
	}
	candidateRole := current.ExecutorKind == "native" && (current.Role == "author" || current.Role == "correction") || current.ExecutorKind == "local_check" && current.Role == "check"
	responseRole := current.ExecutorKind == "native" && (current.Role == "author" || current.Role == "correction")
	if request.CandidateBinding != nil && !candidateRole || request.EligibleResponseBinding != nil && !responseRole {
		return facts, domain.Fail("graph_output_unavailable", "Supporting review or local action output is not a deliverable node result.")
	}
	if request.CandidateBinding == nil && request.EligibleResponseBinding == nil {
		return facts, nil
	}

	if request.CandidateBinding != nil {
		if !domain.ValidID(request.CandidateBinding.CandidateID) || current.CandidateID == nil || *current.CandidateID != request.CandidateBinding.CandidateID {
			return facts, evidence.ErrCorrupt
		}
		facts.Observation.Candidate = &domain.GraphCandidateOutputObservation{CandidateID: request.CandidateBinding.CandidateID, State: "incomplete"}
	}
	if request.EligibleResponseBinding != nil {
		if !domain.ValidID(request.EligibleResponseBinding.ResponseArtifactID) || current.ResponseArtifactID == nil || *current.ResponseArtifactID != request.EligibleResponseBinding.ResponseArtifactID {
			return facts, evidence.ErrCorrupt
		}
		facts.Observation.Response = &domain.GraphResponseOutputObservation{ResponseArtifactID: request.EligibleResponseBinding.ResponseArtifactID, State: "incomplete"}
	}
	defer func() {
		if captureErr == nil {
			return
		}
		state := "incomplete"
		if uncertainEvidence(captureErr) {
			state = "uncertain"
		}
		// Preserve actual closed prefix references for inspection, but an errored
		// capture must never return a qualifying complete result association.
		if facts.Observation.Candidate != nil {
			facts.Observation.Candidate.State = state
		}
		if facts.Observation.Response != nil {
			facts.Observation.Response.State = state
		}
	}()
	receipt, input, err := e.graphOutputSource(ctx, current, request.Snapshot, record, facts)
	if err != nil {
		return facts, err
	}
	view, err := e.db.Workflow(ctx, current.TaskID, domain.WorkflowSelector{WorkflowID: current.WorkflowID})
	if err != nil || view.Workflow == nil {
		if err == nil {
			err = evidence.ErrCorrupt
		}
		return facts, err
	}
	w := *view.Workflow
	if w.TaskID != current.TaskID || w.TaskVersion != current.TaskVersion {
		return facts, evidence.ErrCorrupt
	}
	base := workspace.GraphOutputCaptureRequest{GraphID: d.GraphID, NodeID: d.NodeID, ProducerAttemptID: current.ID, ReceivedInputID: input.ID, Source: request.Snapshot}
	if binding := request.CandidateBinding; binding != nil {
		candidate, readErr := e.db.Candidate(ctx, binding.CandidateID)
		if readErr != nil {
			return facts, readErr
		}
		if candidate.AttemptID != current.ID || candidate.WorkflowID != current.WorkflowID || candidate.Completeness != "complete" ||
			candidate.SourceSnapshotID != request.Snapshot.ID || binding.SourceSnapshotID != request.Snapshot.ID || candidate.RootSourceSnapshotID != binding.RootSourceSnapshotID ||
			candidate.ManifestArtifactID != binding.ManifestArtifactID || candidate.IndexArtifactID != binding.IndexArtifactID ||
			candidate.ConditionsSHA256 != binding.ConditionsSHA256 || binding.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
			return facts, evidence.ErrCorrupt
		}
		manifest, readErr := e.graphOutputReference(ctx, facts, binding.ManifestArtifactID, current.ID, binding.ManifestSHA256, "candidate_manifest")
		if readErr != nil {
			return facts, readErr
		}
		index, readErr := e.graphOutputReference(ctx, facts, binding.IndexArtifactID, current.ID, binding.IndexSHA256, "index_snapshot", "candidate_index")
		if readErr != nil {
			return facts, readErr
		}
		candidateRequest := base
		candidateRequest.Kind, candidateRequest.CandidateBinding, candidateRequest.Manifest, candidateRequest.Index = "candidate", binding, manifest, index
		captured, captureErr := e.workspace.CaptureGraphOutput(ctx, candidateRequest)
		if captureErr != nil {
			return facts, captureErr
		}
		facts.CandidateClosure = &captured.Closure
		facts.Observation.Candidate.ClosureRef = &captured.ClosureArtifact
		if err = e.graphOutputClosureAnchors(ctx, facts, captured); err != nil {
			return facts, err
		}
		if err = e.captureGraphFinalVerification(ctx, current, d, input, *binding, captured, facts); err != nil {
			return facts, err
		}
		facts.Observation.Candidate.State = "complete"
	}
	if binding := request.EligibleResponseBinding; binding != nil {
		if binding.ProducerAttemptID != current.ID || binding.SourceSnapshotID != request.Snapshot.ID || current.ResponseArtifactID == nil || *current.ResponseArtifactID != binding.ResponseArtifactID ||
			binding.ManifestSHA256 != receipt.ObservedManifest.SHA256 || binding.ConditionsSHA256 != w.Authority.ConditionsSHA256 {
			return facts, evidence.ErrCorrupt
		}
		response, readErr := e.graphOutputReference(ctx, facts, binding.ResponseArtifactID, current.ID, binding.ResponseSHA256, "final_response")
		if readErr != nil {
			return facts, readErr
		}
		if response.ByteLength != binding.ResponseByteLength {
			return facts, evidence.ErrCorrupt
		}
		index, readErr := e.graphOutputReference(ctx, facts, binding.IndexArtifactID, current.ID, binding.IndexSHA256, "index_snapshot", "candidate_index")
		if readErr != nil {
			return facts, readErr
		}
		responseRequest := base
		responseRequest.Kind, responseRequest.ResponseBinding, responseRequest.Manifest, responseRequest.Index, responseRequest.SourceMaterialization =
			"response", binding, *receipt.ObservedManifest, index, &receipt
		captured, captureErr := e.workspace.CaptureGraphOutput(ctx, responseRequest)
		if captureErr != nil {
			return facts, captureErr
		}
		facts.ResponseClosure = &captured.Closure
		facts.Observation.Response.ClosureRef = &captured.ClosureArtifact
		if err = e.graphOutputClosureAnchors(ctx, facts, captured); err != nil {
			return facts, err
		}
		facts.Observation.Response.State = "complete"
	}
	if err = project.ValidateIdentity(request.Snapshot.WorkspacePath, record.PrivateMaterialization.WorkspaceIdentity); err != nil {
		return facts, err
	}
	return facts, nil
}

// This verifies the persisted pre-command source without comparing mutable
// worker bytes to their old state. The after-state candidate is checked separately.
func (e *Engine) graphOutputSource(ctx context.Context, a domain.Attempt, snapshot *workspace.Snapshot, record domain.GraphDispatchRecord, facts *domain.GraphStepOutputFacts) (domain.GraphMaterializationReceipt, domain.GraphInputBinding, error) {
	var zeroReceipt domain.GraphMaterializationReceipt
	var zeroInput domain.GraphInputBinding
	d := record.Public.Dispatch
	receipt := record.Public.Materialization
	if receipt == nil || record.MaterializationArtifact == nil || record.MaterializationArtifact.ProducerAttemptID != a.ID || record.PrivateMaterialization == nil || snapshot == nil ||
		receipt.State != "complete" || !receipt.EffectsKnown || receipt.EvidenceStatus != "complete" || receipt.Failure != nil ||
		receipt.GraphID != d.GraphID || receipt.InputID != d.InputID || receipt.InputSHA256 != d.InputSHA256 || receipt.ReceivingAttemptID != a.ID || receipt.OperationAttemptID != a.ID ||
		receipt.SourceSnapshotID == nil || *receipt.SourceSnapshotID != snapshot.ID || receipt.ObservedManifest == nil || receipt.ObservedIndex == nil || receipt.OperationEffects == nil ||
		receipt.ObservedWorkingEntriesSHA256 == nil || receipt.ObservedIndexEntriesSHA256 == nil || receipt.StagedBlobsSHA256 == nil || !reflect.DeepEqual(receipt.SourceBasis, d.SourceBasis) ||
		snapshot.AttemptID != a.ID || snapshot.WorkspacePath != filepath.Join(e.lock.Root(), "workspaces", snapshot.ID) ||
		snapshot.GitControlSHA256 != record.PrivateMaterialization.GitControlSHA256 || record.PrivateMaterialization.WorkspaceIdentity == "" {
		return zeroReceipt, zeroInput, domain.Fail("graph_materialization_unavailable", "Graph output requires the exact complete receiving materialization and private workspace identity.")
	}
	body, err := e.readGraphArtifact(ctx, *record.MaterializationArtifact, "graph_materialization", 64<<20)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	var stored domain.GraphMaterializationReceipt
	if err = json.Unmarshal(body, &stored, json.RejectUnknownMembers(true)); err != nil || !reflect.DeepEqual(stored, *receipt) {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	if _, err = e.graphOutputAnchor(ctx, facts, *record.MaterializationArtifact, "graph_materialization"); err != nil {
		return zeroReceipt, zeroInput, err
	}
	inputRecord, err := e.db.GraphInputRecord(ctx, d.GraphID, d.InputID)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	input := inputRecord.Public.InputBinding
	if input == nil || inputRecord.PrivateRoot == nil || input.ID != d.InputID || input.InputSHA256 != d.InputSHA256 || input.ReceivingNodeID != d.NodeID || input.ReceivingTaskID != d.TaskID || input.ReceivingTaskVersion != d.TaskVersion {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	verified, err := e.workspace.VerifyGraphInput(ctx, *input)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	for _, ref := range verified.ArtifactRefs {
		if _, err = e.graphOutputAnchor(ctx, facts, ref); err != nil {
			return zeroReceipt, zeroInput, err
		}
	}
	source, err := e.db.SourceSnapshot(ctx, snapshot.ID)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	root := *inputRecord.PrivateRoot
	if a.SourceSnapshotID == nil || *a.SourceSnapshotID != source.ID || source.AttemptID != a.ID || source.ManifestArtifactID != snapshot.ManifestArtifactID || source.IndexArtifactID != snapshot.IndexArtifactID ||
		receipt.ObservedManifest.ArtifactID != source.ManifestArtifactID || receipt.ObservedManifest.SHA256 != source.ManifestSHA256 || receipt.ObservedIndex.ArtifactID != source.IndexArtifactID ||
		source.BaseCommit != root.BaseCommit || source.ProjectIdentity != root.ProjectIdentity || source.SelectedRelativePath != root.SelectedRelativePath || !reflect.DeepEqual(source.Branch, root.Branch) ||
		snapshot.BaseCommit != source.BaseCommit || snapshot.SelectedSubfolder != source.SelectedRelativePath {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	if err = project.ValidateIdentity(snapshot.ProjectPath, root.ProjectIdentity); err != nil {
		return zeroReceipt, zeroInput, err
	}
	if err = project.ValidateIdentity(snapshot.RepositoryPath, root.RepositoryIdentity); err != nil {
		return zeroReceipt, zeroInput, err
	}
	if err = project.ValidateIdentity(snapshot.WorkspacePath, record.PrivateMaterialization.WorkspaceIdentity); err != nil {
		return zeroReceipt, zeroInput, err
	}
	manifestBody, err := e.readGraphArtifact(ctx, *receipt.ObservedManifest, "source_manifest", 64<<20)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	var manifest workspace.Manifest
	if err = json.Unmarshal(manifestBody, &manifest, json.RejectUnknownMembers(true)); err != nil {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	if err = e.verifyWorkflowClosure(ctx, manifest); err != nil {
		return zeroReceipt, zeroInput, err
	}
	working, index, err := workspace.GraphManifestDigests(manifest)
	if err != nil || working != *receipt.ObservedWorkingEntriesSHA256 || index != *receipt.ObservedIndexEntriesSHA256 ||
		working != d.SourceBasis.ExpectedWorkingEntriesSHA256 || index != d.SourceBasis.ExpectedIndexEntriesSHA256 {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	if _, err = e.graphOutputAnchor(ctx, facts, d.SourceBasis.ExpectedManifest, "source_manifest", "candidate_manifest", "graph_input_manifest"); err != nil {
		return zeroReceipt, zeroInput, err
	}
	if _, err = e.graphOutputAnchor(ctx, facts, d.SourceBasis.ExpectedIndex, "index_snapshot", "candidate_index"); err != nil {
		return zeroReceipt, zeroInput, err
	}
	for _, item := range []struct {
		ref  domain.GraphArtifactRef
		kind string
	}{{*receipt.ObservedManifest, "source_manifest"}, {*receipt.ObservedIndex, "index_snapshot"}, {*receipt.OperationEffects, "graph_operation_effects"}} {
		if item.ref.ProducerAttemptID != a.ID {
			return zeroReceipt, zeroInput, evidence.ErrCorrupt
		}
		if _, err = e.graphOutputAnchor(ctx, facts, item.ref, item.kind); err != nil {
			return zeroReceipt, zeroInput, err
		}
	}
	effectsBody, err := e.readGraphArtifact(ctx, *receipt.OperationEffects, "graph_operation_effects", 64<<20)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	var effects workspace.GraphMaterializationEffects
	if err = json.Unmarshal(effectsBody, &effects, json.RejectUnknownMembers(true)); err != nil || effects.Contract != "006-materialization-effects-v1" || effects.Registration != "registered" || !effects.EffectsKnown ||
		effects.WorkspaceID != snapshot.ID || effects.ObjectEffects == nil || effects.ObjectEffects.ProducerAttemptID != a.ID || effects.StagedBlobs.SHA256 != *receipt.StagedBlobsSHA256 {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	if _, err = e.graphOutputAnchor(ctx, facts, *effects.ObjectEffects, "graph_operation_effects"); err != nil {
		return zeroReceipt, zeroInput, err
	}
	stagedBody, err := e.readGraphArtifact(ctx, effects.StagedBlobs, "graph_index_blobs", 64<<20)
	if err != nil {
		return zeroReceipt, zeroInput, err
	}
	var staged workspace.GraphIndexBlobManifest
	if err = json.Unmarshal(stagedBody, &staged, json.RejectUnknownMembers(true)); err != nil || staged.Contract != "006-index-blobs-v1" || len(staged.Entries) > workspace.MaxEntries {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	objects := map[string]bool{}
	for _, entry := range manifest.Index {
		objects[entry.Object] = true
	}
	if len(objects) != len(staged.Entries) {
		return zeroReceipt, zeroInput, evidence.ErrCorrupt
	}
	previous := ""
	for _, blob := range staged.Entries {
		if !objects[blob.ObjectID] || blob.ObjectID <= previous || blob.ContentSHA256 != blob.ArtifactRef.SHA256 || blob.ByteLength != blob.ArtifactRef.ByteLength {
			return zeroReceipt, zeroInput, evidence.ErrCorrupt
		}
		previous = blob.ObjectID
		meta, verifyErr := e.evidence.VerifyClosed(ctx, blob.ArtifactRef.ArtifactID)
		if verifyErr != nil {
			return zeroReceipt, zeroInput, verifyErr
		}
		if meta.Kind != "graph_index_blob" && meta.Kind != "source_file" || !validGraphArtifactReference(blob.ArtifactRef) || !graphArtifactMatches(meta, blob.ArtifactRef, meta.Kind) {
			return zeroReceipt, zeroInput, evidence.ErrCorrupt
		}
	}
	if _, err = e.graphOutputAnchor(ctx, facts, effects.StagedBlobs, "graph_index_blobs"); err != nil {
		return zeroReceipt, zeroInput, err
	}
	return *receipt, *input, nil
}

func (e *Engine) graphOutputReference(ctx context.Context, facts *domain.GraphStepOutputFacts, id, owner, hash string, kinds ...string) (domain.GraphArtifactRef, error) {
	meta, err := e.evidence.VerifyClosed(ctx, id)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	if meta.SHA256 == nil || meta.AttemptID != owner || *meta.SHA256 != hash {
		return domain.GraphArtifactRef{}, evidence.ErrCorrupt
	}
	ref := domain.GraphArtifactRef{ArtifactID: id, ProducerAttemptID: owner, SHA256: hash, ByteLength: meta.ByteLength}
	if _, err = e.graphOutputAnchor(ctx, facts, ref, kinds...); err != nil {
		return domain.GraphArtifactRef{}, err
	}
	return ref, nil
}

func (e *Engine) graphOutputAnchor(ctx context.Context, facts *domain.GraphStepOutputFacts, ref domain.GraphArtifactRef, kinds ...string) (evidence.Metadata, error) {
	meta, err := e.evidence.VerifyClosed(ctx, ref.ArtifactID)
	if err != nil {
		return meta, err
	}
	allowed := len(kinds) == 0
	for _, kind := range kinds {
		allowed = allowed || meta.Kind == kind
	}
	if !allowed || !validGraphArtifactReference(ref) || !graphArtifactMatches(meta, ref, meta.Kind) {
		return meta, evidence.ErrCorrupt
	}
	proof := domain.VerifiedResponse{ArtifactID: meta.ID, SHA256: *meta.SHA256, PhysicalIdentity: meta.Identity, ByteLength: meta.ByteLength}
	for _, previous := range facts.VerifiedArtifacts {
		if previous.ArtifactID == proof.ArtifactID {
			if previous != proof {
				return meta, evidence.ErrCorrupt
			}
			return meta, nil
		}
	}
	if len(facts.VerifiedArtifacts) >= 256 {
		return meta, evidence.ErrLimit
	}
	facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, proof)
	return meta, nil
}

func (e *Engine) graphOutputClosureAnchors(ctx context.Context, facts *domain.GraphStepOutputFacts, captured workspace.GraphOutputCapture) error {
	for _, item := range []struct {
		ref   domain.GraphArtifactRef
		kinds []string
	}{{captured.Closure.Manifest, []string{"source_manifest", "candidate_manifest"}}, {captured.Closure.Index, []string{"index_snapshot", "candidate_index"}},
		{captured.Closure.StagedBlobs, []string{"graph_index_blobs"}}, {captured.ClosureArtifact, []string{"graph_output_closure"}}} {
		if _, err := e.graphOutputAnchor(ctx, facts, item.ref, item.kinds...); err != nil {
			return err
		}
	}
	return nil
}
