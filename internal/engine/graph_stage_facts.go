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

// A later role consumes the workflow's selected subject. Its original graph
// input remains provenance; it must never replace an author's newer result.
func (e *Engine) graphWorkflowStageFacts(ctx context.Context, progress workflowProgress, previousStepID string, explicit ...*domain.GraphAdvanceInput) (*domain.GraphStageAdmissionFacts, error) {
	if len(explicit) > 1 {
		return nil, domain.Invalid("one exact explicit graph intent")
	}
	resuming := len(explicit) == 1 && explicit[0] != nil
	member := progress.detail.Task.GraphRef
	if member == nil {
		return nil, nil
	}
	w := progress.subject.Workflow
	if member.Kind != "node" || member.NodeID == nil || progress.detail.Task.CurrentVersion != w.TaskVersion {
		return nil, domain.Fail("graph_input_stale", "Only the exact current node workflow can prepare its next graph stage.")
	}
	step, err := e.db.WorkflowStep(ctx, previousStepID)
	if err != nil {
		return nil, err
	}
	if step.AttemptID == nil || step.WorkflowID != w.ID || w.CurrentStepID == nil || *w.CurrentStepID != step.ID {
		return nil, domain.Fail("graph_input_stale", "The next graph stage requires its exact current predecessor execution.")
	}
	a, err := e.db.Attempt(ctx, *step.AttemptID)
	if err != nil {
		return nil, err
	}
	if a.Phase != "settled" || a.Cleanup.State != "verified" || !resuming && a.OwnerInstanceID != e.config.OwnerInstanceID || a.WorkflowID != w.ID || a.TaskID != w.TaskID || a.TaskVersion != w.TaskVersion || a.StepID != step.ID {
		return nil, domain.Fail("graph_input_stale", "The graph predecessor has not settled under this owner's exact workflow authority.")
	}
	previous, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	d := previous.Public.Dispatch
	view, err := e.db.Graph(ctx, member.GraphID)
	if err != nil {
		return nil, err
	}
	if d.GraphID != member.GraphID || d.NodeID != *member.NodeID || d.TaskID != w.TaskID || d.TaskVersion != w.TaskVersion || d.WorkflowID != w.ID || d.StepID != step.ID || d.AttemptID != a.ID || view.Graph.Stop != nil || view.DispatchAuthority == nil {
		return nil, domain.Fail("graph_input_stale", "The selected graph workflow no longer has live current-owner execution intent.")
	}
	if !resuming && view.DispatchAuthority.OwnerInstanceID != e.config.OwnerInstanceID {
		scoped, scopeErr := e.db.RecoveryScopedPredecessor(ctx, e.config.OwnerInstanceID, a.ID)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if !scoped {
			return nil, domain.Fail("graph_input_stale", "The selected graph workflow has no current scoped owner authority.")
		}
	}
	var node *domain.GraphNodeSummary
	for i := range view.Nodes {
		if view.Nodes[i].ID == d.NodeID {
			node = &view.Nodes[i]
			break
		}
	}
	if node == nil || node.Stop != nil || node.TaskID != a.TaskID || node.TaskVersion != a.TaskVersion || node.InputID == nil || *node.InputID != d.InputID || node.NodeAdmissionID == nil || *node.NodeAdmissionID != d.NodeAdmissionID {
		return nil, domain.Fail("graph_input_stale", "The receiving node's exact version, input or logical admission changed.")
	}
	registration := *view.DispatchAuthority
	if resuming {
		intent := explicit[0]
		authorized := false
		for _, authority := range intent.NodeAuthorities {
			if authority.NodeID == node.ID && authority.TaskVersion == node.TaskVersion && authority.NodeControlRevision == node.ControlRevision {
				authorized = true
			}
		}
		if !authorized || intent.GraphVersion != view.Graph.GraphVersion || intent.ExpectedGraphControlRevision != view.Graph.ControlRevision || intent.ExpectedBudgetAuthorityEpoch != view.Graph.BudgetAuthorityEpoch || intent.ExpectedWindowNumber != view.Budget.Budget.CurrentWindow.Number {
			return nil, domain.Fail("graph_input_stale", "The explicit graph intent no longer captures this exact receiving workflow and shared window.")
		}
	} else if registration.GraphControlRevision != view.Graph.ControlRevision || registration.BudgetAuthorityEpoch != view.Graph.BudgetAuthorityEpoch || registration.WindowNumber != view.Budget.Budget.CurrentWindow.Number {
		return nil, domain.Fail("graph_input_stale", "The graph execution registration no longer matches its current controls and shared window.")
	}
	record, err := e.db.GraphInputRecord(ctx, d.GraphID, d.InputID)
	if err != nil {
		return nil, err
	}
	input := record.Public.InputBinding
	if !record.Public.Summary.Current || record.Public.Summary.State != "ready" || input == nil || record.ConditionsBinding == nil || record.Public.ConditionsRef == nil || input.InputSHA256 != d.InputSHA256 || input.ReceivingNodeID != node.ID || input.ReceivingTaskVersion != node.TaskVersion {
		return nil, domain.Fail("graph_input_stale", "The graph workflow requires its complete current input and receiver-scoped conditions.")
	}
	verified, err := e.workspace.VerifyGraphInput(ctx, *input)
	if err != nil {
		return nil, err
	}
	conditionsBody, err := e.readGraphArtifact(ctx, *record.Public.ConditionsRef, "graph_conditions", 64<<20)
	if err != nil {
		return nil, err
	}
	var conditions domain.GraphConditionsBinding
	if err = json.Unmarshal(conditionsBody, &conditions, json.RejectUnknownMembers(true)); err != nil || !reflect.DeepEqual(conditions, *record.ConditionsBinding) {
		return nil, evidence.ErrCorrupt
	}
	seal, err := domain.GraphConditionsDigest(conditions)
	if err != nil || seal != conditions.ConditionsSHA256 {
		return nil, evidence.ErrCorrupt
	}
	_, own, err := workflowpolicy.ConditionDigests(progress.detail.Task.OriginalText, progress.detail.Version, w.Authority.Conditions, w.Authority.Checks)
	if err != nil {
		return nil, err
	}
	effective, err := graphEffectiveConditionsDigest(own, seal)
	if err != nil || effective != w.Authority.ConditionsSHA256 {
		return nil, evidence.ErrCorrupt
	}
	manifestID, indexID := progress.subject.Source.ManifestArtifactID, progress.subject.ResponseIndexID()
	if progress.subject.Binding != nil {
		manifestID = progress.subject.Binding.ManifestArtifactID
	}
	manifest, err := e.evidence.VerifyClosed(ctx, manifestID)
	if err != nil {
		return nil, err
	}
	index, err := e.evidence.VerifyClosed(ctx, indexID)
	if err != nil {
		return nil, err
	}
	if manifest.SHA256 == nil || index.SHA256 == nil {
		return nil, evidence.ErrCorrupt
	}
	workingHash, indexHash, err := workspace.GraphManifestDigests(progress.subject.Manifest)
	if err != nil {
		return nil, err
	}
	basis := domain.GraphSourceBasis{BasisKind: "node_subject", WorkflowID: w.ID, InputID: input.ID, InputSHA256: input.InputSHA256,
		CandidateBinding: progress.subject.Binding, ResponseBinding: progress.subject.Response,
		ExpectedManifest:             domain.GraphArtifactRef{ArtifactID: manifest.ID, ProducerAttemptID: manifest.AttemptID, SHA256: *manifest.SHA256, ByteLength: manifest.ByteLength},
		ExpectedIndex:                domain.GraphArtifactRef{ArtifactID: index.ID, ProducerAttemptID: index.AttemptID, SHA256: *index.SHA256, ByteLength: index.ByteLength},
		ExpectedWorkingEntriesSHA256: workingHash, ExpectedIndexEntriesSHA256: indexHash}
	outputAnchors, err := e.graphSelectedStageOutput(ctx, progress.subject, d, basis)
	if err != nil {
		return nil, err
	}
	ids := progress.subject.artifactIDs()
	for _, ref := range verified.ArtifactRefs {
		ids = append(ids, ref.ArtifactID)
	}
	ids = append(ids, record.Public.ConditionsRef.ArtifactID)
	for _, ref := range outputAnchors {
		ids = append(ids, ref.ArtifactID)
	}
	proofs, err := e.verifyWorkflowArtifacts(ctx, ids...)
	if err != nil {
		return nil, err
	}
	if progress.final != nil {
		if progress.subject.Binding == nil || progress.final.Binding.FinalCandidateBinding != *progress.subject.Binding || progress.final.Binding.FinalWorkflowID != w.ID {
			return nil, evidence.ErrCorrupt
		}
		ref := progress.final.BindingRef
		basis.FinalVerificationRef = &ref
		proofs, err = mergeGraphAcceptanceProofs(proofs, progress.final.VerifiedArtifacts)
		if err != nil {
			return nil, err
		}
	}
	if len(proofs) > 256 {
		return nil, evidence.ErrLimit
	}
	trigger := domain.GraphFreshTrigger{Kind: "completion", ID: a.ID, OwnerGeneration: registration.Generation}
	if resuming {
		trigger.Kind, trigger.ID = "explicit", explicit[0].RequestID
	}
	nodeID, taskID, version, revision, inputID := node.ID, node.TaskID, node.TaskVersion, node.ControlRevision, input.ID
	return &domain.GraphStageAdmissionFacts{Trigger: trigger, NodeAdmissionID: d.NodeAdmissionID, InputBinding: *input, SourceBasis: basis, VerifiedArtifacts: proofs,
		Control: domain.GraphDispatchBinding{GraphID: d.GraphID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID, OperationAttemptID: a.ID,
			OwnerInstanceID: e.config.OwnerInstanceID, Generation: registration.Generation, GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch,
			WindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision, InputID: &inputID,
			ContributorVector: append([]domain.GraphContributorBinding{}, input.ContributorVector...)}}, nil
}

func (e *Engine) graphSelectedStageOutput(ctx context.Context, subject workflowSubject, receiver domain.GraphDispatch, basis domain.GraphSourceBasis) ([]domain.GraphArtifactRef, error) {
	producer := ""
	if subject.Candidate != nil && subject.Binding != nil {
		producer = subject.Candidate.AttemptID
	} else if subject.Response != nil {
		producer = subject.Response.ProducerAttemptID
	}
	if producer == "" {
		return nil, evidence.ErrCorrupt
	}
	stored, err := e.db.GraphDispatchForAttempt(ctx, producer)
	if err != nil {
		return nil, err
	}
	d, observation := stored.Public.Dispatch, stored.Public.OutputObservation
	if d.GraphID != receiver.GraphID || d.NodeID != receiver.NodeID || d.TaskVersion != receiver.TaskVersion || d.WorkflowID != receiver.WorkflowID || d.NodeAdmissionID != receiver.NodeAdmissionID || d.InputID != receiver.InputID || observation == nil || observation.ProducerAttemptID != producer {
		return nil, evidence.ErrCorrupt
	}
	var closureRef *domain.GraphArtifactRef
	kind := "candidate"
	if subject.Binding != nil {
		if observation.Candidate == nil || observation.Candidate.CandidateID != subject.Binding.CandidateID || observation.Candidate.State != "complete" {
			return nil, evidence.ErrCorrupt
		}
		closureRef = observation.Candidate.ClosureRef
	} else {
		kind = "response"
		if observation.Response == nil || observation.Response.ResponseArtifactID != subject.Response.ResponseArtifactID || observation.Response.State != "complete" {
			return nil, evidence.ErrCorrupt
		}
		closureRef = observation.Response.ClosureRef
	}
	if closureRef == nil || closureRef.ProducerAttemptID != producer {
		return nil, evidence.ErrCorrupt
	}
	body, err := e.readGraphArtifact(ctx, *closureRef, "graph_output_closure", 64<<20)
	if err != nil {
		return nil, err
	}
	var closure domain.GraphOutputClosure
	if err = json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)); err != nil {
		return nil, evidence.ErrCorrupt
	}
	seal, err := domain.GraphOutputDigest(closure)
	if err != nil || domain.ValidateGraphOutputClosure(closure) != nil || seal != closure.ClosureSHA256 || closure.Kind != kind || closure.GraphID != d.GraphID || closure.NodeID != d.NodeID || closure.ProducerAttemptID != producer || closure.ReceivedInputID != d.InputID ||
		closure.Manifest != basis.ExpectedManifest || closure.Index != basis.ExpectedIndex {
		return nil, evidence.ErrCorrupt
	}
	stagedBody, err := e.readGraphArtifact(ctx, closure.StagedBlobs, "graph_index_blobs", 64<<20)
	if err != nil {
		return nil, err
	}
	var staged workspace.GraphIndexBlobManifest
	if err = json.Unmarshal(stagedBody, &staged, json.RejectUnknownMembers(true)); err != nil || staged.Contract != "006-index-blobs-v1" || len(staged.Entries) > workspace.MaxEntries {
		return nil, evidence.ErrCorrupt
	}
	objects := map[string]bool{}
	for _, item := range subject.Manifest.Index {
		objects[item.Object] = true
	}
	if len(objects) != len(staged.Entries) {
		return nil, evidence.ErrCorrupt
	}
	previous := ""
	for _, blob := range staged.Entries {
		if !objects[blob.ObjectID] || blob.ObjectID <= previous || blob.ContentSHA256 != blob.ArtifactRef.SHA256 || blob.ByteLength != blob.ArtifactRef.ByteLength {
			return nil, evidence.ErrCorrupt
		}
		previous = blob.ObjectID
		meta, err := e.evidence.VerifyClosed(ctx, blob.ArtifactRef.ArtifactID)
		if err != nil {
			return nil, err
		}
		if (meta.Kind != "graph_index_blob" && meta.Kind != "source_file") || !validGraphArtifactReference(blob.ArtifactRef) || !graphArtifactMatches(meta, blob.ArtifactRef, meta.Kind) {
			return nil, evidence.ErrCorrupt
		}
	}
	return []domain.GraphArtifactRef{*closureRef, closure.Manifest, closure.Index, closure.StagedBlobs}, nil
}
