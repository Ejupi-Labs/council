package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type graphReceiverContext struct {
	Request       workspace.GraphContextRequest
	Conditions    domain.GraphConditionsBinding
	ConditionsRef domain.GraphArtifactRef
	ArtifactRefs  []domain.GraphArtifactRef
}

// This retains exact current receiver and attributed upstream sources; it does not register
// input readiness, authorize execution or infer that a model read those sources.
// The local operation guard and completion transaction own those decisions.
func (e *Engine) retainGraphReceiverContext(ctx context.Context, op domain.GraphInputOperation, receiver domain.TaskVersionDetail) (graphReceiverContext, error) {
	var result graphReceiverContext
	c := op.Control
	if op.Kind != "node_input" || c.NodeID == nil || c.TaskID == nil || c.TaskVersion == nil ||
		!domain.ValidID(*c.NodeID) || receiver.TaskID != *c.TaskID || receiver.Version.TaskID != receiver.TaskID || receiver.Version.Version != *c.TaskVersion ||
		op.Definition.GraphID != op.GraphID || op.Definition.GraphVersion != c.GraphVersion || op.Definition.RootTaskID != op.RootTaskID ||
		!domain.ValidID(op.ID) || !domain.ValidID(op.Attempt.ID) || op.Attempt.TaskID != op.RootTaskID || op.Attempt.ExecutorKind != "local_graph" {
		return result, domain.Invalid("exact graph receiver context operation")
	}
	upstream, err := e.loadGraphUpstreamContext(ctx, op)
	if err != nil {
		return result, err
	}
	actual, err := e.db.TaskVersion(ctx, receiver.TaskID, receiver.Version.Version)
	if err != nil {
		return result, err
	}
	if !reflect.DeepEqual(actual, receiver) {
		return result, domain.Fail("graph_input_stale", "Retain only the complete exact captured receiving task version.")
	}
	put := func(kind string, body []byte) (domain.GraphArtifactRef, error) {
		meta, err := e.evidence.Put(ctx, op.Attempt.ID, kind, body)
		if err != nil {
			return domain.GraphArtifactRef{}, err
		}
		if meta.SHA256 == nil || !meta.Closed || meta.Completeness != "complete" {
			return domain.GraphArtifactRef{}, evidence.ErrCorrupt
		}
		return domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}, nil
	}
	shared := ""
	if op.Definition.Original.SharedInstructions != nil {
		shared = *op.Definition.Original.SharedInstructions
	}
	sharedRef, err := put("graph_original", []byte(shared))
	if err != nil {
		return result, err
	}
	originalRef, err := put("graph_original", []byte(receiver.OriginalText))
	if err != nil {
		return result, err
	}
	acceptance, err := json.Marshal(receiver.Version.Acceptance)
	if err != nil {
		return result, err
	}
	acceptanceRef, err := put("graph_acceptance", acceptance)
	if err != nil {
		return result, err
	}
	var guidanceRef *domain.GraphArtifactRef
	if receiver.Version.GuidanceText != "" {
		retained, err := put("graph_original", []byte(receiver.Version.GuidanceText))
		if err != nil {
			return result, err
		}
		guidanceRef = &retained
	}
	nodeID, version := *c.NodeID, *c.TaskVersion
	sources := []domain.GraphConditionSource{
		{Ref: "graph-instructions", AuthorityScope: "graph_shared", OriginalRef: sharedRef},
		{Ref: "node-instructions", AuthorityScope: "receiver", NodeID: &nodeID, TaskVersion: &version, OriginalRef: originalRef, GuidanceRef: guidanceRef, AcceptanceRef: &acceptanceRef},
	}
	sources = append(sources, upstream.Sources...)
	conditions, err := graphScopedConditions(op.GraphID, c.GraphVersion, receiver.TaskID, version, sources)
	if err != nil {
		return result, err
	}
	body, err := json.Marshal(conditions)
	if err != nil {
		return result, err
	}
	conditionsRef, err := put("graph_conditions", body)
	if err != nil {
		return result, err
	}
	request := workspace.GraphContextRequest{GraphID: op.GraphID, InputID: op.ID, ReceivingNodeID: nodeID, ReceivingTaskVersion: version,
		GraphInstructionsRef: sharedRef, NodeInstructionsRef: originalRef, GuidanceRef: guidanceRef, AcceptanceRef: &acceptanceRef,
		ConstraintRefs:      []domain.GraphScopedReference{{Ref: "applicable-conditions", ArtifactRef: conditionsRef, AuthorityScope: "receiver", NodeID: &nodeID, TaskVersion: &version, Paths: []string{}}},
		ContributedEvidence: append([]domain.GraphContributedEvidence{}, upstream.ContributedEvidence...), SupportedAccess: append([]domain.GraphProvidedAccess{}, upstream.SupportedAccess...)}
	request.ConstraintRefs = append(request.ConstraintRefs, upstream.ConstraintRefs...)
	addAccess := func(name, scope string, ref domain.GraphArtifactRef) {
		mode := "file"
		count, _ := domain.ParseCount(ref.ByteLength)
		if count <= 16<<10 {
			mode = "inline"
		}
		request.SupportedAccess = append(request.SupportedAccess, domain.GraphProvidedAccess{Ref: name, ArtifactRef: ref, Scope: scope, Paths: []string{}, Mode: mode})
	}
	addAccess("graph-instructions", "graph_shared", sharedRef)
	addAccess("node-instructions", "receiver", originalRef)
	addAccess("receiver-acceptance", "receiver", acceptanceRef)
	if guidanceRef != nil {
		addAccess("receiver-guidance", "receiver", *guidanceRef)
	}
	addAccess("applicable-conditions", "receiver", conditionsRef)
	return graphReceiverContext{Request: request, Conditions: conditions, ConditionsRef: conditionsRef, ArtifactRefs: upstream.ArtifactRefs}, nil
}
