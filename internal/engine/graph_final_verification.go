package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// The cumulative basis is distinct from the node's unchanged authored subject.
// Loading this read-only context neither accepts output nor grants execution.
type graphFinalVerification struct {
	BindingRef          domain.GraphArtifactRef
	Binding             domain.GraphFinalVerificationBinding
	Root                domain.GraphRootBinding
	RootSource          *workspace.Snapshot
	RootProof           workspace.GraphRootProof
	OutputProof         workspace.GraphOutputProof
	OutputClosure       domain.GraphOutputClosure
	Input               domain.GraphInputBinding
	Conditions          domain.GraphConditionsBinding
	Changes             []workspace.Change
	ArtifactRefs        []domain.GraphArtifactRef
	VerifiedArtifacts   []domain.VerifiedResponse
	SourceControlSHA256 string
	subject             workflowSubject
	version             domain.TaskVersionDetail
}

func (e *Engine) loadGraphFinalVerification(ctx context.Context, subject workflowSubject) (*graphFinalVerification, error) {

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	detail, err := e.db.Task(ctx, subject.Workflow.TaskID)
	if err != nil {
		return nil, err
	}
	member := detail.Task.GraphRef
	if member == nil {
		return nil, nil
	}
	stale := func() error {
		return domain.Fail("graph_input_stale", "Cumulative qualification requires this current selected file-final candidate and accepted input.")
	}
	if member.Kind != "node" || member.NodeID == nil {
		return nil, stale()
	}
	graph, err := e.db.Graph(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	if graph.Graph.FinalNodeID == nil || *graph.Graph.FinalNodeID != *member.NodeID {
		return nil, nil
	}
	version, err := e.db.TaskVersion(ctx, detail.Task.ID, subject.Workflow.TaskVersion)
	if err != nil {
		return nil, err
	}
	if version.Version.Acceptance.Deliverable == "response_only" {
		return nil, nil
	}
	if detail.Task.CurrentVersion != subject.Workflow.TaskVersion || subject.Binding == nil || subject.Candidate == nil || subject.Response != nil || graph.Graph.Stop != nil {
		return nil, stale()
	}
	var receiver *domain.GraphNodeSummary
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == *member.NodeID {
			receiver = &graph.Nodes[i]
		}
	}
	if receiver == nil || receiver.Stop != nil || receiver.TaskID != detail.Task.ID || receiver.TaskVersion != subject.Workflow.TaskVersion || receiver.InputID == nil || receiver.NodeAdmissionID == nil {
		return nil, stale()
	}
	current, err := e.db.Workflow(ctx, detail.Task.ID, domain.WorkflowSelector{})
	if err != nil {
		return nil, err
	}
	if current.Workflow == nil || current.Workflow.ID != subject.Workflow.ID || current.Workflow.TaskVersion != subject.Workflow.TaskVersion || current.Workflow.SelectedCandidateID == nil || *current.Workflow.SelectedCandidateID != subject.Binding.CandidateID {
		return nil, stale()
	}
	fresh, err := e.selectedWorkflowSubject(ctx, *current.Workflow)
	if err != nil {
		return nil, err
	}
	if fresh.Candidate == nil || !reflect.DeepEqual(fresh.Binding, subject.Binding) || fresh.Candidate.ID != subject.Candidate.ID {
		return nil, stale()
	}
	producer, err := e.db.Attempt(ctx, fresh.Candidate.AttemptID)
	if err != nil {
		return nil, err
	}
	if producer.TaskID != detail.Task.ID || producer.TaskVersion != receiver.TaskVersion || producer.WorkflowID != fresh.Workflow.ID || producer.CandidateID == nil || *producer.CandidateID != fresh.Binding.CandidateID ||
		producer.Phase != "settled" || producer.Outcome == nil || *producer.Outcome != "succeeded" || producer.PrimaryFailure != nil || producer.Stop != nil || producer.Cleanup.State != "verified" || producer.EvidenceStatus != "complete" ||
		!(producer.ExecutorKind == "native" && (producer.Role == "author" || producer.Role == "correction") || producer.ExecutorKind == "local_check" && producer.Role == "check") {
		return nil, stale()
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, producer.ID)
	if err != nil {
		return nil, err
	}
	d, observation := record.Public.Dispatch, record.Public.OutputObservation
	if d.GraphID != member.GraphID || d.GraphVersion != graph.Graph.GraphVersion || d.NodeID != receiver.ID || d.TaskID != producer.TaskID || d.TaskVersion != producer.TaskVersion || d.WorkflowID != producer.WorkflowID || d.StepID != producer.StepID || d.AttemptID != producer.ID ||
		d.BudgetOwnerTaskID != member.RootTaskID || d.NodeAdmissionID != *receiver.NodeAdmissionID || d.InputID != *receiver.InputID ||
		observation == nil || observation.ProducerAttemptID != producer.ID || observation.Candidate == nil || observation.Candidate.CandidateID != fresh.Binding.CandidateID || observation.Candidate.State != "complete" ||
		observation.Candidate.ClosureRef == nil || observation.Candidate.FinalVerificationRef == nil || record.CandidateClosure == nil || record.FinalVerification == nil {
		return nil, domain.Fail("graph_final_verification_unavailable", "The selected file-final has no complete same-terminal cumulative basis.")
	}
	ref := *observation.Candidate.FinalVerificationRef
	body, err := e.readGraphArtifact(ctx, ref, "graph_final_verification", workspace.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	var binding domain.GraphFinalVerificationBinding
	if json.Unmarshal(body, &binding, json.RejectUnknownMembers(true)) != nil || !reflect.DeepEqual(binding, *record.FinalVerification) || ref.ProducerAttemptID != producer.ID ||
		binding.GraphID != d.GraphID || binding.GraphVersion != d.GraphVersion || binding.FinalNodeID != d.NodeID || binding.FinalTaskID != d.TaskID || binding.FinalTaskVersion != d.TaskVersion || binding.FinalWorkflowID != d.WorkflowID ||
		!reflect.DeepEqual(binding.FinalCandidateBinding, *fresh.Binding) || binding.OutputClosureRef != *observation.Candidate.ClosureRef || binding.FinalInputID != d.InputID || !reflect.DeepEqual(binding.ContributorVector, d.ContributorVector) {
		return nil, evidence.ErrCorrupt
	}
	digest, err := domain.GraphFinalVerificationDigest(binding)
	if err != nil || digest != binding.BindingSHA256 {
		return nil, evidence.ErrCorrupt
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, d.GraphID)
	if err != nil {
		return nil, err
	}
	root, err := e.db.GraphRoot(ctx, d.GraphID)
	if err != nil {
		return nil, err
	}
	if root.RootTaskID != member.RootTaskID || !reflect.DeepEqual(binding.RootBinding, domain.GraphRootPublic(root)) {
		return nil, evidence.ErrCorrupt
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		return nil, err
	}
	rootProof, err := e.workspace.VerifyGraphRoot(ctx, root, rootSource)
	if err != nil {
		return nil, err
	}
	output, err := e.workspace.VerifyCapturedGraphOutput(ctx, workspace.GraphOutputCapture{Closure: *record.CandidateClosure, ClosureArtifact: *observation.Candidate.ClosureRef})
	if err != nil {
		return nil, err
	}
	closure := *record.CandidateClosure
	if closure.GraphID != d.GraphID || closure.NodeID != d.NodeID || closure.ReceivedInputID != d.InputID || closure.ProducerAttemptID != producer.ID || closure.Kind != "candidate" || closure.CandidateID == nil || *closure.CandidateID != fresh.Binding.CandidateID ||
		closure.Manifest.ArtifactID != fresh.Binding.ManifestArtifactID || closure.Manifest.SHA256 != fresh.Binding.ManifestSHA256 || closure.Index.ArtifactID != fresh.Binding.IndexArtifactID || closure.Index.SHA256 != fresh.Binding.IndexSHA256 {
		return nil, evidence.ErrCorrupt
	}
	inputRecord, err := e.db.GraphInputRecord(ctx, d.GraphID, d.InputID)
	if err != nil {
		return nil, err
	}
	if inputRecord.PrivateRoot == nil || !reflect.DeepEqual(*inputRecord.PrivateRoot, root) || inputRecord.Public.Summary.State != "ready" || !inputRecord.Public.Summary.Current ||
		inputRecord.Public.InputBinding == nil || inputRecord.Public.ConditionsRef == nil || inputRecord.ConditionsBinding == nil {
		return nil, stale()
	}
	input := *inputRecord.Public.InputBinding
	if input.InputSHA256 != d.InputSHA256 || input.ReceivingNodeID != d.NodeID || input.ReceivingTaskID != d.TaskID || input.ReceivingTaskVersion != d.TaskVersion ||
		!reflect.DeepEqual(input.ContributorVector, binding.ContributorVector) || binding.GraphConditionsRef != *inputRecord.Public.ConditionsRef {
		return nil, stale()
	}
	verifiedInput, err := e.workspace.VerifyGraphInput(ctx, input)
	if err != nil {
		return nil, err
	}
	conditionBytes, err := e.readGraphArtifact(ctx, binding.GraphConditionsRef, "graph_conditions", workspace.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	var conditions domain.GraphConditionsBinding
	if json.Unmarshal(conditionBytes, &conditions, json.RejectUnknownMembers(true)) != nil || domain.ValidateGraphConditionsBinding(conditions) != nil ||
		!reflect.DeepEqual(conditions, *inputRecord.ConditionsBinding) || conditions.GraphID != d.GraphID || conditions.GraphVersion != d.GraphVersion || conditions.ReceiverTaskID != d.TaskID || conditions.ReceiverTaskVersion != d.TaskVersion {
		return nil, evidence.ErrCorrupt
	}
	digest, err = domain.GraphConditionsDigest(conditions)
	if err != nil || digest != conditions.ConditionsSHA256 || digest != binding.GraphConditionsSHA256 {
		return nil, evidence.ErrCorrupt
	}
	_, own, err := workflowpolicy.ConditionDigests(version.OriginalText, version.Version, fresh.Workflow.Authority.Conditions, fresh.Workflow.Authority.Checks)
	if err != nil {
		return nil, err
	}
	effective, err := graphEffectiveConditionsDigest(own, digest)
	if err != nil || effective != fresh.Workflow.Authority.ConditionsSHA256 || effective != fresh.Binding.ConditionsSHA256 {
		return nil, evidence.ErrCorrupt
	}
	access, err := graphFinalOriginalAccess(conditions, verifiedInput.ContextPacket)
	if err != nil || !reflect.DeepEqual(access, binding.OriginalAccess) {
		return nil, evidence.ErrCorrupt
	}
	changes := workspace.CompareManifests(rootProof.Manifest, output.Manifest)
	working, index := workflowChangeFlags(changes)
	deltaBytes, err := e.readGraphArtifact(ctx, binding.CumulativeDeltaRef, "graph_cumulative_delta", workspace.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	var delta graphCumulativeDelta
	if binding.CumulativeDeltaRef.ProducerAttemptID != producer.ID || json.Unmarshal(deltaBytes, &delta, json.RejectUnknownMembers(true)) != nil ||
		delta.Contract != "006-cumulative-delta-v1" || delta.GraphID != d.GraphID || delta.GraphVersion != d.GraphVersion || delta.FinalNodeID != d.NodeID || delta.FinalTaskID != d.TaskID || delta.FinalTaskVersion != d.TaskVersion || delta.FinalWorkflowID != d.WorkflowID ||
		!reflect.DeepEqual(delta.RootBinding, binding.RootBinding) || !reflect.DeepEqual(delta.FinalCandidateBinding, *fresh.Binding) || delta.OutputClosureRef != binding.OutputClosureRef || delta.FinalInputID != input.ID ||
		!reflect.DeepEqual(delta.ContributorVector, input.ContributorVector) || !reflect.DeepEqual(delta.Changes, changes) || working != binding.HasCumulativeWorkingDelta || index != binding.HasCumulativeIndexDelta {
		return nil, evidence.ErrCorrupt
	}
	selection, err := e.selectGraphContributions(ctx, definition, root, *receiver)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(selection.ContributorVector, input.ContributorVector) || len(selection.Sources) != len(input.Contributions) {
		return nil, stale()
	}
	all := append(append(append([]domain.GraphArtifactRef{}, rootProof.Artifacts...), output.Artifacts...), verifiedInput.ArtifactRefs...)
	for _, source := range selection.Sources {
		found := false
		for _, captured := range input.Contributions {
			if reflect.DeepEqual(source.Binding, captured) {
				found = true
			}
		}
		if !found {
			return nil, stale()
		}
		// Preserve every physical source member, not only final-tree members.
		physical, err := e.workspace.VerifyGraphOutput(ctx, workspace.GraphOutputVerifyRequest{RootBinding: root, Contribution: source.Binding, InputBinding: source.InputBinding})
		if err != nil {
			return nil, err
		}
		all = append(all, physical.Artifacts...)
	}
	anchors := []domain.GraphArtifactRef{ref, binding.CumulativeDeltaRef, binding.OutputClosureRef, closure.Manifest, closure.Index, closure.StagedBlobs,
		root.Manifest, root.Index, root.StagedBlobs, input.Manifest, input.Index, input.StagedBlobs, input.Provenance, input.ContextPacket, binding.GraphConditionsRef}
	for _, item := range binding.OriginalAccess {
		if len(item.Paths) != 0 || item.Mode != "file" && item.Mode != "inline" {
			return nil, evidence.ErrCorrupt
		}
		anchors = append(anchors, item.ArtifactRef)
	}
	all, err = e.graphNativeContextRefs(ctx, append(all, anchors...))
	if err != nil {
		return nil, err
	}
	proofs, err := e.verifyGraphClosureAnchors(ctx, all, anchors)
	if err != nil {
		return nil, err
	}
	proofs, err = mergeGraphAcceptanceProofs(proofs, selection.VerifiedArtifacts)
	if err != nil {
		return nil, err
	}
	// Close with authority observed after the last physical read. Clock/owner
	// admission checks remain separate; reading never extends their allowance.
	latest, err := e.db.Graph(ctx, d.GraphID)
	if err != nil {
		return nil, err
	}
	currentReceiver := false
	for _, node := range latest.Nodes {
		if node.ID == receiver.ID && node.TaskID == receiver.TaskID && node.TaskVersion == receiver.TaskVersion && node.ControlRevision == receiver.ControlRevision && node.Stop == nil &&
			node.InputID != nil && *node.InputID == input.ID && node.NodeAdmissionID != nil && *node.NodeAdmissionID == d.NodeAdmissionID {
			currentReceiver = true
		}
	}
	latestWorkflow, err := e.db.Workflow(ctx, d.TaskID, domain.WorkflowSelector{})
	if err != nil {
		return nil, err
	}
	if !currentReceiver || latest.Graph.Stop != nil || latest.Graph.GraphVersion != d.GraphVersion || latest.Graph.ControlRevision != graph.Graph.ControlRevision ||
		latestWorkflow.Workflow == nil || latestWorkflow.Workflow.ID != fresh.Workflow.ID || latestWorkflow.Workflow.SelectedCandidateID == nil || *latestWorkflow.Workflow.SelectedCandidateID != fresh.Binding.CandidateID {
		return nil, stale()
	}
	control, err := json.Marshal(struct {
		Contract        string                  `json:"contract"`
		VerificationRef domain.GraphArtifactRef `json:"verificationRef"`
		BindingSHA256   string                  `json:"bindingSha256"`
	}{"006-final-applicability-v1", ref, binding.BindingSHA256})
	if err != nil {
		return nil, err
	}
	return &graphFinalVerification{BindingRef: ref, Binding: binding, Root: root, RootSource: rootSource, RootProof: rootProof, OutputProof: output, OutputClosure: closure, Input: input, Conditions: conditions,
		Changes: changes, ArtifactRefs: all, VerifiedArtifacts: proofs, SourceControlSHA256: workflowHash(control), subject: fresh, version: version}, nil
}

func (e *Engine) graphFinalPlanFacts(ctx context.Context, verified *graphFinalVerification, original string, version domain.TaskVersion) (*workflowpolicy.GraphFinalPlanFacts, error) {
	if verified == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if original != verified.version.OriginalText || !reflect.DeepEqual(version, verified.version.Version) || verified.subject.Binding == nil || !reflect.DeepEqual(*verified.subject.Binding, verified.Binding.FinalCandidateBinding) {
		return nil, evidence.ErrCorrupt
	}
	return &workflowpolicy.GraphFinalPlanFacts{VerificationBindingRef: verified.BindingRef, DeclaredFileFinal: true, CurrentSubject: verified.Binding.FinalCandidateBinding,
		HasCumulativeWorkingDelta: verified.Binding.HasCumulativeWorkingDelta, HasCumulativeIndexDelta: verified.Binding.HasCumulativeIndexDelta,
		CumulativeDirectProven: e.graphFinalDirectProof(ctx, verified, original, version) == nil}, nil
}

func (v *graphFinalVerification) applicability(scope domain.CheckScope, keys []string, definition string) (domain.Applicability, error) {
	if v == nil || v.subject.Binding == nil || !reflect.DeepEqual(*v.subject.Binding, v.Binding.FinalCandidateBinding) || !domain.ValidSHA256(v.SourceControlSHA256) {
		return domain.Applicability{}, evidence.ErrCorrupt
	}
	files, index := workflowProofEntries(v.OutputProof.Manifest)
	result, invalid := workflowpolicy.Basis(workflowpolicy.ApplicabilityInput{Scope: scope, ConditionKeys: keys, DefinitionSHA256: definition,
		ConditionsSHA256: v.subject.Workflow.Authority.ConditionsSHA256, SourceControlSHA256: v.SourceControlSHA256, Files: files, Index: index})
	if invalid != nil {
		return result, invalid
	}
	return result, nil
}

// This proof consumes actual graph-root before bytes directly. It never changes
// the node's workflowSubject root/source or its recorded authored delta.
func (e *Engine) graphFinalDirectProof(ctx context.Context, v *graphFinalVerification, original string, version domain.TaskVersion) *domain.Error {
	return e.graphCumulativeDirectProof(ctx, v.subject.Workflow, v.RootProof.Manifest, v.OutputProof.Manifest, original, version)
}

// Capture can call this before terminal association, using the same actual
// verified manifests. It supplies no selected-subject or admission authority.
func (e *Engine) graphCumulativeDirectProof(ctx context.Context, workflow domain.Workflow, rootManifest, selectedManifest workspace.Manifest, original string, version domain.TaskVersion) *domain.Error {
	if invalid := e.graphDirectProofGuard(ctx, workflow, original, version); invalid != nil {
		return invalid
	}
	op, recognized := workflowpolicy.RecognizeDocument(original, version)
	if !recognized {
		return domain.Fail("evidence_unavailable", "This cumulative task has no supported complete direct proof.")
	}
	before, beforeIndex := workflowProofEntries(rootManifest)
	after, afterIndex := workflowProofEntries(selectedManifest)
	for _, group := range []struct {
		manifest workspace.Manifest
		files    []workflowpolicy.ProofFile
	}{{rootManifest, before}, {selectedManifest, after}} {
		for i, entry := range group.manifest.Entries {
			if entry.Path != op.Path {
				continue
			}
			var content bytes.Buffer
			meta, err := e.evidence.CopyTo(ctx, entry.ArtifactID, &content, 1<<20)
			if err != nil {
				return domain.SafeError(err)
			}
			if meta.Kind != "source_file" || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 || meta.ByteLength != domain.Decimal(entry.Size) {
				return domain.Fail("evidence_unavailable", "The exact cumulative document bytes changed or are unavailable.")
			}
			group.files[i].Bytes = content.Bytes()
		}
	}
	return workflowpolicy.ProveDocument(workflowpolicy.DocumentProofInput{Original: original, Version: version, Before: before, After: after, BeforeIndex: beforeIndex, AfterIndex: afterIndex})
}
