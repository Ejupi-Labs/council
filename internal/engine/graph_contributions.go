package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"sort"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type graphContributionSelection struct {
	Sources           []workspace.GraphContributionSource
	ContributorVector []domain.GraphContributorBinding
	ConstraintRefs    []domain.GraphScopedReference
	EvidenceRefs      []domain.GraphContributedEvidence
	VerifiedArtifacts []domain.VerifiedResponse
}

func (e *Engine) selectGraphContributions(ctx context.Context, definition domain.GraphDefinition, root domain.GraphRootBinding, receiver domain.GraphNodeSummary) (graphContributionSelection, error) {
	empty := graphContributionSelection{}
	result := graphContributionSelection{Sources: []workspace.GraphContributionSource{}, ContributorVector: []domain.GraphContributorBinding{},
		ConstraintRefs: []domain.GraphScopedReference{}, EvidenceRefs: []domain.GraphContributedEvidence{}, VerifiedArtifacts: []domain.VerifiedResponse{}}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if definition.GraphID != root.GraphID || definition.RootTaskID != root.RootTaskID || receiver.GraphID != root.GraphID {
		return empty, domain.Fail("graph_input_stale", "Select dependencies only within the exact captured graph root.")
	}
	storedDefinition, err := e.db.GraphDefinitionPrivate(ctx, definition.GraphID)
	if err != nil {
		return empty, err
	}
	storedRoot, err := e.db.GraphRoot(ctx, root.GraphID)
	if err != nil {
		return empty, err
	}
	if !reflect.DeepEqual(storedDefinition, definition) || !reflect.DeepEqual(storedRoot, root) {
		return empty, domain.Fail("graph_input_stale", "The supplied graph definition or private root is not the stored version.")
	}
	view, err := e.db.Graph(ctx, definition.GraphID)
	if err != nil {
		return empty, err
	}
	if view.Graph.GraphVersion != definition.GraphVersion || view.Graph.RootTaskID != root.RootTaskID || view.Graph.Stop != nil {
		return empty, domain.Fail("graph_input_stale", "This graph version is no longer eligible for new dependent input.")
	}
	nodes := map[string]domain.GraphNodeSummary{}
	found := false
	for _, node := range view.Nodes {
		if _, duplicate := nodes[node.Key]; duplicate {
			return empty, evidence.ErrCorrupt
		}
		nodes[node.Key] = node
		if node.ID == receiver.ID && node.Key == receiver.Key && node.TaskID == receiver.TaskID && node.TaskVersion == receiver.TaskVersion && node.ControlRevision == receiver.ControlRevision && node.Stop == nil {
			found = true
		}
	}
	if !found {
		return empty, domain.Fail("graph_input_stale", "The receiving node identity or controls changed before dependency selection.")
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		return empty, err
	}
	if _, err = e.workspace.VerifyGraphRoot(ctx, root, rootSource); err != nil {
		return empty, err
	}
	anchors := []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs}
	vector := map[string]domain.GraphContributorBinding{}
	addVector := func(item domain.GraphContributorBinding) error {
		if previous, present := vector[item.NodeID]; present {
			if !reflect.DeepEqual(previous, item) {
				return domain.Fail("graph_input_stale", "Two paths selected different accepted outcomes for the same ancestor.")
			}
			return nil
		}
		if len(vector) >= domain.MaxGraphNodes {
			return domain.Fail("input_too_large", "The exact accepted contributor vector exceeds sixteen node heads.")
		}
		vector[item.NodeID] = item
		return nil
	}
	edges := []domain.GraphDependencyInput{}
	if definition.Original.Dependencies != nil {
		for _, edge := range *definition.Original.Dependencies {
			if edge.To == receiver.Key {
				edges = append(edges, edge)
			}
		}
	}
	for _, edge := range edges {
		source, present := nodes[edge.From]
		if !present || source.Stop != nil || source.AcceptedResult == nil {
			return empty, domain.Fail("graph_dependency_unaccepted", "A required predecessor has no current accepted result.")
		}
		kind := edge.Kind
		if kind == "" {
			kind = "files"
		}
		edgeID, err := domain.GraphEdgeID(definition.GraphID, definition.GraphVersion, source.ID, receiver.ID, kind)
		if err != nil {
			return empty, err
		}
		selected, refs, err := e.graphCurrentAcceptedContribution(ctx, root, source, edgeID, kind)
		if err != nil {
			return empty, err
		}
		// This verifies all source/input/selected working and staged payloads,
		// without opening an old mutable worker directory.
		physical, err := e.workspace.VerifyGraphOutput(ctx, workspace.GraphOutputVerifyRequest{RootBinding: root, Contribution: selected.Binding, InputBinding: selected.InputBinding})
		if err != nil {
			return empty, err
		}
		_ = physical // Complete physical verification precedes bounded SQL anchors.
		for _, ancestor := range selected.InputBinding.ContributorVector {
			if err = addVector(ancestor); err != nil {
				return empty, err
			}
		}
		c := selected.Binding
		if err = addVector(domain.GraphContributorBinding{NodeID: c.FromNodeID, TaskID: c.TaskID, TaskVersion: c.TaskVersion, AcceptanceKind: c.AcceptanceKind, AcceptanceID: c.AcceptanceID,
			ProducerAttemptID: c.ProducerAttemptID, InputID: c.InputID, CandidateBinding: c.CandidateBinding, ResponseBinding: c.ResponseBinding, OutputClosureRef: c.OutputClosureRef}); err != nil {
			return empty, err
		}
		result.Sources = append(result.Sources, selected)
		result.ConstraintRefs = append(result.ConstraintRefs, c.ConstraintRefs...)
		result.EvidenceRefs = append(result.EvidenceRefs, c.EvidenceRefs...)
		anchors = append(anchors, refs...)
	}
	sort.Slice(result.Sources, func(i, j int) bool { return result.Sources[i].Binding.EdgeID < result.Sources[j].Binding.EdgeID })
	for _, item := range vector {
		// A transitive tuple is not current merely because a descendant retained it.
		current := false
		for _, node := range nodes {
			r := node.AcceptedResult
			if node.ID == item.NodeID && node.TaskID == item.TaskID && node.TaskVersion == item.TaskVersion && node.Stop == nil && r != nil &&
				r.TaskVersion == item.TaskVersion && r.AcceptanceKind == item.AcceptanceKind && r.AcceptanceID == item.AcceptanceID && r.ProducerAttemptID == item.ProducerAttemptID && r.InputID == item.InputID {
				current = true
				break
			}
		}
		if !current {
			return empty, domain.Fail("graph_input_stale", "A retained transitive contributor no longer names its current accepted outcome.")
		}
		result.ContributorVector = append(result.ContributorVector, item)
	}
	sort.Slice(result.ContributorVector, func(i, j int) bool { return result.ContributorVector[i].NodeID < result.ContributorVector[j].NodeID })
	result.ConstraintRefs, result.EvidenceRefs, err = graphUniqueContributionContext(result.ConstraintRefs, result.EvidenceRefs)
	if err != nil {
		return empty, err
	}
	for _, ref := range result.ConstraintRefs {
		anchors = append(anchors, ref.ArtifactRef)
	}
	for _, ref := range result.EvidenceRefs {
		anchors = append(anchors, ref.ArtifactRef)
	}
	result.VerifiedArtifacts, err = e.graphContributionProofs(ctx, anchors)
	if err != nil {
		return empty, err
	}
	return result, nil
}

func (e *Engine) graphCurrentAcceptedContribution(ctx context.Context, root domain.GraphRootBinding, node domain.GraphNodeSummary, edgeID, kind string) (workspace.GraphContributionSource, []domain.GraphArtifactRef, error) {
	var zero workspace.GraphContributionSource
	r := node.AcceptedResult
	if r == nil || r.TaskVersion != node.TaskVersion {
		return zero, nil, domain.Fail("graph_dependency_unaccepted", "The predecessor has no accepted result for its current version.")
	}
	task, err := e.db.Task(ctx, node.TaskID)
	if err != nil {
		return zero, nil, err
	}
	if task.Task.CurrentVersion != node.TaskVersion || task.Task.GraphRef == nil || task.Task.GraphRef.GraphID != root.GraphID || task.Task.GraphRef.NodeID == nil || *task.Task.GraphRef.NodeID != node.ID {
		return zero, nil, domain.Fail("graph_input_stale", "The accepted predecessor no longer belongs to this current node version.")
	}
	detail, err := e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{TaskVersion: node.TaskVersion})
	if err != nil {
		return zero, nil, err
	}
	w := detail.Workflow
	if w == nil || w.State != "completed" || w.TaskID != node.TaskID || w.TaskVersion != node.TaskVersion {
		return zero, nil, domain.Fail("graph_dependency_unaccepted", "The predecessor workflow has no current completed acceptance.")
	}
	c := domain.AcceptedContribution{EdgeID: edgeID, EdgeKind: kind, FromNodeID: node.ID, TaskID: node.TaskID, TaskVersion: node.TaskVersion,
		AcceptanceKind: r.AcceptanceKind, AcceptanceID: r.AcceptanceID, ProducerAttemptID: r.ProducerAttemptID, InputID: r.InputID,
		ConstraintRefs: []domain.GraphScopedReference{}, EvidenceRefs: []domain.GraphContributedEvidence{}}
	switch r.AcceptanceKind {
	case "workflow":
		accepted := detail.Acceptance
		if accepted == nil || accepted.ID != r.AcceptanceID || accepted.TaskID != node.TaskID || accepted.TaskVersion != node.TaskVersion || accepted.WorkflowID != w.ID ||
			w.SelectedCandidateID == nil || r.CandidateID == nil || *w.SelectedCandidateID != *r.CandidateID || accepted.CandidateBinding.CandidateID != *r.CandidateID || r.ResponseArtifactID != nil {
			return zero, nil, domain.Fail("graph_dependency_unaccepted", "The exact current workflow acceptance does not name this selected predecessor candidate.")
		}
		subject, err := e.candidateWorkflowSubject(ctx, *w, accepted.CandidateBinding.CandidateID)
		if err != nil {
			return zero, nil, err
		}
		if subject.Binding == nil || subject.Candidate == nil || !reflect.DeepEqual(*subject.Binding, accepted.CandidateBinding) || subject.Candidate.AttemptID != r.ProducerAttemptID {
			return zero, nil, evidence.ErrCorrupt
		}
		binding := accepted.CandidateBinding
		c.CandidateBinding = &binding
	case "response":
		binding, err := e.graphAcceptedResponseContribution(ctx, root.GraphID, node, *w)
		if err != nil {
			return zero, nil, err
		}
		c.ResponseBinding = &binding
	default:
		return zero, nil, evidence.ErrCorrupt
	}
	producer, err := e.db.Attempt(ctx, r.ProducerAttemptID)
	if err != nil {
		return zero, nil, err
	}
	if producer.TaskID != node.TaskID || producer.TaskVersion != node.TaskVersion || producer.WorkflowID != w.ID || producer.Phase != "settled" || producer.Outcome == nil || *producer.Outcome != "succeeded" ||
		producer.Stop != nil || producer.PrimaryFailure != nil || producer.EvidenceStatus != "complete" || producer.Cleanup.State != "verified" {
		return zero, nil, domain.Fail("graph_dependency_unaccepted", "The exact accepted result producer is not completely settled and usable.")
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, producer.ID)
	if err != nil {
		return zero, nil, err
	}
	d, observation := dispatch.Public.Dispatch, dispatch.Public.OutputObservation
	if d.GraphID != root.GraphID || d.NodeID != node.ID || d.TaskID != node.TaskID || d.TaskVersion != node.TaskVersion || d.AttemptID != producer.ID || d.InputID != r.InputID ||
		d.WorkflowID != producer.WorkflowID || d.StepID != producer.StepID || d.Role != producer.Role || d.ExecutorKind != producer.ExecutorKind || d.OwnerInstanceID != producer.OwnerInstanceID ||
		observation == nil || observation.ProducerAttemptID != producer.ID {
		return zero, nil, evidence.ErrCorrupt
	}
	if c.CandidateBinding != nil {
		out := observation.Candidate
		if out == nil || out.State != "complete" || out.ClosureRef == nil || out.CandidateID != c.CandidateBinding.CandidateID {
			return zero, nil, domain.Fail("graph_dependency_unavailable", "The accepted candidate has no exact complete same-terminal graph output closure.")
		}
		c.OutputClosureRef = *out.ClosureRef
	} else {
		out := observation.Response
		if out == nil || out.State != "complete" || out.ClosureRef == nil || out.ResponseArtifactID != c.ResponseBinding.ResponseArtifactID {
			return zero, nil, domain.Fail("graph_dependency_unavailable", "The accepted response has no exact complete same-terminal graph output closure.")
		}
		c.OutputClosureRef = *out.ClosureRef
	}
	input, err := e.db.GraphInputRecord(ctx, root.GraphID, d.InputID)
	if err != nil {
		return zero, nil, err
	}
	binding := input.Public.InputBinding
	if binding == nil || input.PrivateRoot == nil || input.PrivateRoot.BasisSHA256 != root.BasisSHA256 || binding.ID != d.InputID || binding.InputSHA256 != d.InputSHA256 ||
		binding.ReceivingNodeID != node.ID || binding.ReceivingTaskID != node.TaskID || binding.ReceivingTaskVersion != node.TaskVersion || input.Public.ConditionsRef == nil || input.ConditionsBinding == nil {
		return zero, nil, evidence.ErrCorrupt
	}
	refs := []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs, c.OutputClosureRef, binding.Manifest, binding.Index, binding.StagedBlobs, binding.Provenance, binding.ContextPacket, *input.Public.ConditionsRef}
	conditionsBody, err := e.readGraphArtifact(ctx, *input.Public.ConditionsRef, "graph_conditions", 64<<20)
	if err != nil {
		return zero, nil, err
	}
	var conditions domain.GraphConditionsBinding
	if err = json.Unmarshal(conditionsBody, &conditions, json.RejectUnknownMembers(true)); err != nil || !reflect.DeepEqual(conditions, *input.ConditionsBinding) {
		return zero, nil, evidence.ErrCorrupt
	}
	seal, err := domain.GraphConditionsDigest(conditions)
	if err != nil || seal != conditions.ConditionsSHA256 || conditions.ReceiverTaskID != node.TaskID || conditions.ReceiverTaskVersion != node.TaskVersion {
		return zero, nil, evidence.ErrCorrupt
	}
	addCondition := func(ref domain.GraphArtifactRef, originNode, version, label string) {
		n, v := originNode, version
		name := "up-" + originNode + "-" + workflowHash([]byte(label + ":" + ref.ArtifactID))[:16]
		c.ConstraintRefs = append(c.ConstraintRefs, domain.GraphScopedReference{Ref: name, ArtifactRef: ref, AuthorityScope: "upstream_local", NodeID: &n, TaskVersion: &v, Paths: []string{}})
	}
	for _, source := range conditions.Sources {
		origin, version := node.ID, node.TaskVersion
		if source.NodeID != nil {
			origin = *source.NodeID
		}
		if source.TaskVersion != nil {
			version = *source.TaskVersion
		}
		// Even the ancestor's graph-shared source is historical context here;
		// this receiver's graphShared source is supplied separately by its owner.
		addCondition(source.OriginalRef, origin, version, "original:"+source.Ref)
		if source.GuidanceRef != nil {
			addCondition(*source.GuidanceRef, origin, version, "guidance:"+source.Ref)
		}
		if source.AcceptanceRef != nil {
			addCondition(*source.AcceptanceRef, origin, version, "acceptance:"+source.Ref)
		}
	}
	addCondition(*input.Public.ConditionsRef, node.ID, node.TaskVersion, "conditions")
	closureBody, err := e.readGraphArtifact(ctx, c.OutputClosureRef, "graph_output_closure", 64<<20)
	if err != nil {
		return zero, nil, err
	}
	var closure domain.GraphOutputClosure
	if err = json.Unmarshal(closureBody, &closure, json.RejectUnknownMembers(true)); err != nil {
		return zero, nil, evidence.ErrCorrupt
	}
	resultID, scope, output := "", "whole_candidate", closure.Manifest
	if c.CandidateBinding != nil {
		resultID = c.CandidateBinding.CandidateID
	} else {
		resultID, scope = c.ResponseBinding.ResponseArtifactID, "whole_response"
		if closure.ResponseArtifact == nil {
			return zero, nil, evidence.ErrCorrupt
		}
		output = *closure.ResponseArtifact
	}
	for _, item := range []struct {
		name string
		ref  domain.GraphArtifactRef
	}{{"result", output}, {"closure", c.OutputClosureRef}} {
		c.EvidenceRefs = append(c.EvidenceRefs, domain.GraphContributedEvidence{Ref: "edge-" + edgeID + "-" + item.name, EdgeID: edgeID, SourceNodeID: node.ID, AcceptanceKind: c.AcceptanceKind, AcceptanceID: c.AcceptanceID,
			ProducerAttemptID: producer.ID, ResultKind: closure.Kind, ResultID: resultID, ArtifactRef: item.ref, Scope: scope, Paths: []string{}})
	}
	refs = append(refs, closure.Manifest, closure.Index, closure.StagedBlobs)
	if dispatch.MaterializationArtifact == nil || dispatch.Public.Materialization == nil || dispatch.Public.Materialization.ObservedManifest == nil || dispatch.Public.Materialization.ObservedIndex == nil {
		return zero, nil, evidence.ErrCorrupt
	}
	refs = append(refs, *dispatch.MaterializationArtifact, *dispatch.Public.Materialization.ObservedManifest, *dispatch.Public.Materialization.ObservedIndex)
	if dispatch.Public.Materialization.OperationEffects != nil {
		refs = append(refs, *dispatch.Public.Materialization.OperationEffects)
	}
	return workspace.GraphContributionSource{Binding: c, InputBinding: *binding}, refs, nil
}

// This boundary uses the actual acknowledgement ID, never Task.Acceptance's
// latest-attempt convenience projection or a supporting review's response.
func (e *Engine) graphAcceptedResponseContribution(ctx context.Context, graphID string, node domain.GraphNodeSummary, w domain.Workflow) (domain.ResponseBinding, error) {
	var zero domain.ResponseBinding
	r := node.AcceptedResult
	if r == nil || r.AcceptanceKind != "response" || r.ResponseArtifactID == nil || r.CandidateID != nil || w.Completion == nil || w.SelectedResponseBinding == nil {
		return zero, domain.Fail("graph_dependency_unaccepted", "The predecessor has no exact current response-only acceptance.")
	}
	acknowledgement, err := e.db.GraphResponseAcceptance(ctx, graphID, node.ID, r.AcceptanceID)
	if err != nil {
		return zero, err
	}
	binding := *w.SelectedResponseBinding
	if acknowledgement.ID != r.AcceptanceID || acknowledgement.TaskID != node.TaskID || acknowledgement.TaskVersion != node.TaskVersion ||
		acknowledgement.AttemptID != r.ProducerAttemptID || binding.ProducerAttemptID != r.ProducerAttemptID ||
		acknowledgement.ResponseArtifactID != *r.ResponseArtifactID || binding.ResponseArtifactID != *r.ResponseArtifactID ||
		acknowledgement.ResponseSHA256 != binding.ResponseSHA256 || w.Completion.ResponseAcceptanceID != acknowledgement.ID || !reflect.DeepEqual(w.Completion.ResponseBinding, binding) {
		return zero, evidence.ErrCorrupt
	}
	subject, err := e.selectedWorkflowSubject(ctx, w)
	if err != nil {
		return zero, err
	}
	if subject.Binding != nil || subject.Response == nil || !reflect.DeepEqual(*subject.Response, binding) {
		return zero, evidence.ErrCorrupt
	}
	return binding, nil
}

func graphUniqueContributionContext(constraints []domain.GraphScopedReference, evidence []domain.GraphContributedEvidence) ([]domain.GraphScopedReference, []domain.GraphContributedEvidence, error) {
	outConstraints := []domain.GraphScopedReference{}
	outEvidence := []domain.GraphContributedEvidence{}
	names := map[string]string{}
	for _, source := range constraints {
		key, err := graphScopedContextKeyForSelection(source)
		if err != nil {
			return nil, nil, err
		}
		if previous, exists := names[source.Ref]; exists {
			if previous != key {
				return nil, nil, domain.Fail("input_too_large", "Attributed upstream references collide; none were replaced.")
			}
			continue
		}
		names[source.Ref] = key
		outConstraints = append(outConstraints, source)
	}
	for _, source := range evidence {
		copy := source
		copy.Paths = append([]string{}, source.Paths...)
		sort.Strings(copy.Paths)
		body, err := json.Marshal(copy)
		if err != nil {
			return nil, nil, err
		}
		key := string(body)
		if previous, exists := names[source.Ref]; exists {
			if previous != key {
				return nil, nil, domain.Fail("input_too_large", "Attributed upstream evidence collides; none was replaced.")
			}
			continue
		}
		names[source.Ref] = key
		outEvidence = append(outEvidence, source)
	}
	if len(names) > 64 {
		return nil, nil, domain.Fail("input_too_large", "Complete upstream context exceeds sixty-four supplied references.")
	}
	sort.Slice(outConstraints, func(i, j int) bool { return outConstraints[i].Ref < outConstraints[j].Ref })
	sort.Slice(outEvidence, func(i, j int) bool { return outEvidence[i].Ref < outEvidence[j].Ref })
	return outConstraints, outEvidence, nil
}

func graphScopedContextKeyForSelection(source domain.GraphScopedReference) (string, error) {
	source.Paths = append([]string{}, source.Paths...)
	sort.Strings(source.Paths)
	body, err := json.Marshal(source)
	return string(body), err
}

func (e *Engine) graphContributionProofs(ctx context.Context, refs []domain.GraphArtifactRef) ([]domain.VerifiedResponse, error) {
	result := []domain.VerifiedResponse{}
	seen := map[string]domain.GraphArtifactRef{}
	for _, ref := range refs {
		if previous, present := seen[ref.ArtifactID]; present {
			if previous != ref {
				return nil, evidence.ErrCorrupt
			}
			continue
		}
		if len(seen) >= 256 {
			return nil, evidence.ErrLimit
		}
		meta, err := e.evidence.VerifyClosed(ctx, ref.ArtifactID)
		if err != nil {
			return nil, err
		}
		if !validGraphArtifactReference(ref) || !graphArtifactMatches(meta, ref, meta.Kind) {
			return nil, evidence.ErrCorrupt
		}
		seen[ref.ArtifactID] = ref
		result = append(result, domain.VerifiedResponse{ArtifactID: meta.ID, SHA256: *meta.SHA256, PhysicalIdentity: meta.Identity, ByteLength: meta.ByteLength})
	}
	sort.Slice(result, func(i, j int) bool { return strings.Compare(result[i].ArtifactID, result[j].ArtifactID) < 0 })
	return result, nil
}
