package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"reflect"
	"sort"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

type graphUpstreamContext struct {
	Sources             []domain.GraphConditionSource
	ConstraintRefs      []domain.GraphScopedReference
	ContributedEvidence []domain.GraphContributedEvidence
	SupportedAccess     []domain.GraphProvidedAccess
	ArtifactRefs        []domain.GraphArtifactRef
}

// Loading provenance writes no evidence and promotes no historical instruction
// into the receiver's current shared or local authority.
func (e *Engine) loadGraphUpstreamContext(ctx context.Context, op domain.GraphInputOperation) (graphUpstreamContext, error) {
	empty := graphUpstreamContext{}
	result := graphUpstreamContext{Sources: []domain.GraphConditionSource{}, ConstraintRefs: []domain.GraphScopedReference{},
		ContributedEvidence: []domain.GraphContributedEvidence{}, SupportedAccess: []domain.GraphProvidedAccess{}, ArtifactRefs: []domain.GraphArtifactRef{}}
	if err := e.checkGraphContextOperation(ctx, op); err != nil {
		return empty, err
	}
	vector := map[string]domain.GraphContributorBinding{}
	for _, item := range op.ContributorVector {
		if !domain.ValidID(item.NodeID) || item.NodeID == *op.Control.NodeID {
			return empty, evidence.ErrCorrupt
		}
		if _, exists := vector[item.NodeID]; exists {
			return empty, evidence.ErrCorrupt
		}
		vector[item.NodeID] = item
	}
	if len(vector) > domain.MaxGraphNodes || !reflect.DeepEqual(op.ContributorVector, op.Control.ContributorVector) {
		return empty, evidence.ErrCorrupt
	}
	refs := map[string]domain.GraphArtifactRef{}
	remember := func(ref domain.GraphArtifactRef) error {
		if !validGraphArtifactReference(ref) {
			return evidence.ErrCorrupt
		}
		if old, exists := refs[ref.ArtifactID]; exists {
			if old != ref {
				return evidence.ErrCorrupt
			}
			return nil
		}
		refs[ref.ArtifactID] = ref
		result.ArtifactRefs = append(result.ArtifactRefs, ref)
		return nil
	}
	for _, item := range op.ContributorVector {
		input, err := e.db.GraphInputRecord(ctx, op.GraphID, item.InputID)
		if err != nil {
			return empty, err
		}
		binding := input.Public.InputBinding
		if !input.Public.Summary.Current || input.Public.Summary.State != "ready" || binding == nil || input.PrivateRoot == nil ||
			!reflect.DeepEqual(input.PrivateRoot, op.RootBinding) || binding.ID != item.InputID || binding.GraphID != op.GraphID ||
			binding.ReceivingNodeID != item.NodeID || binding.ReceivingTaskID != item.TaskID || binding.ReceivingTaskVersion != item.TaskVersion ||
			input.Public.ConditionsRef == nil || input.ConditionsBinding == nil {
			return empty, evidence.ErrCorrupt
		}
		if invalid := domain.ValidateGraphInputBinding(*binding); invalid != nil {
			return empty, invalid
		}
		seal, err := domain.GraphInputDigest(*binding)
		if err != nil || seal != binding.InputSHA256 {
			return empty, evidence.ErrCorrupt
		}
		body, err := e.readGraphArtifact(ctx, *input.Public.ConditionsRef, "graph_conditions", 64<<20)
		if err != nil {
			return empty, err
		}
		var conditions domain.GraphConditionsBinding
		if err = json.Unmarshal(body, &conditions, json.RejectUnknownMembers(true)); err != nil ||
			domain.ValidateGraphConditionsBinding(conditions) != nil || !reflect.DeepEqual(conditions, *input.ConditionsBinding) ||
			conditions.GraphID != op.GraphID || conditions.GraphVersion != binding.GraphVersion ||
			conditions.ReceiverTaskID != item.TaskID || conditions.ReceiverTaskVersion != item.TaskVersion {
			return empty, evidence.ErrCorrupt
		}
		seal, err = domain.GraphConditionsDigest(conditions)
		if err != nil || seal != conditions.ConditionsSHA256 {
			return empty, evidence.ErrCorrupt
		}
		if err = remember(*input.Public.ConditionsRef); err != nil {
			return empty, err
		}
		source, err := e.graphOwnUpstreamSource(ctx, item, conditions)
		if err != nil {
			return empty, err
		}
		for _, ref := range []*domain.GraphArtifactRef{&source.OriginalRef, source.GuidanceRef, source.AcceptanceRef} {
			if ref != nil {
				if err = remember(*ref); err != nil {
					return empty, err
				}
			}
		}
		result.Sources = append(result.Sources, source)
	}
	for _, selected := range op.Contributions {
		item, exists := vector[selected.FromNodeID]
		if !exists || selected.TaskID != item.TaskID || selected.TaskVersion != item.TaskVersion || selected.AcceptanceKind != item.AcceptanceKind ||
			selected.AcceptanceID != item.AcceptanceID || selected.ProducerAttemptID != item.ProducerAttemptID || selected.InputID != item.InputID ||
			!reflect.DeepEqual(selected.CandidateBinding, item.CandidateBinding) || !reflect.DeepEqual(selected.ResponseBinding, item.ResponseBinding) ||
			selected.OutputClosureRef != item.OutputClosureRef {
			return empty, evidence.ErrCorrupt
		}
		for _, ref := range selected.ConstraintRefs {
			if ref.NodeID == nil || ref.TaskVersion == nil || ref.AuthorityScope != "upstream_local" {
				return empty, evidence.ErrCorrupt
			}
			origin, present := vector[*ref.NodeID]
			if !present || *ref.TaskVersion != origin.TaskVersion {
				return empty, evidence.ErrCorrupt
			}
			result.ConstraintRefs = append(result.ConstraintRefs, ref)
		}
		for _, ref := range selected.EvidenceRefs {
			resultID, kind := "", ""
			if selected.CandidateBinding != nil {
				resultID, kind = selected.CandidateBinding.CandidateID, "candidate"
			} else if selected.ResponseBinding != nil {
				resultID, kind = selected.ResponseBinding.ResponseArtifactID, "response"
			}
			if ref.EdgeID != selected.EdgeID || ref.SourceNodeID != selected.FromNodeID || ref.AcceptanceKind != selected.AcceptanceKind ||
				ref.AcceptanceID != selected.AcceptanceID || ref.ProducerAttemptID != selected.ProducerAttemptID || ref.ResultID != resultID || ref.ResultKind != kind {
				return empty, evidence.ErrCorrupt
			}
			result.ContributedEvidence = append(result.ContributedEvidence, ref)
		}
	}
	var err error
	result.ConstraintRefs, result.ContributedEvidence, err = graphUniqueContributionContext(result.ConstraintRefs, result.ContributedEvidence)
	if err != nil {
		return empty, err
	}
	receiver, err := e.db.TaskVersion(ctx, *op.Control.TaskID, *op.Control.TaskVersion)
	if err != nil {
		return empty, err
	}
	baseCount := 4 // Current shared, receiver original/acceptance and conditions.
	if receiver.Version.GuidanceText != "" {
		baseCount++
	}
	reserved := map[string]bool{"graph-instructions": true, "node-instructions": true, "receiver-guidance": true, "receiver-acceptance": true, "applicable-conditions": true}
	names := map[string]bool{}
	addAccess := func(name, scope string, ref domain.GraphArtifactRef, reuse bool) error {
		if reuse {
			for _, access := range result.SupportedAccess {
				if access.ArtifactRef == ref && access.Scope == scope && len(access.Paths) == 0 {
					return nil
				}
			}
		}
		if names[name] || reserved[name] {
			return domain.Fail("context_capacity", "Complete upstream context has colliding attributed reference names.")
		}
		names[name] = true
		if len(names)+baseCount > 64 {
			return domain.Fail("context_capacity", "Complete attributed context exceeds sixty-four references; none were clipped.")
		}
		meta, err := e.evidence.VerifyClosed(ctx, ref.ArtifactID)
		if err != nil || !validGraphArtifactReference(ref) || !graphArtifactMatches(meta, ref, meta.Kind) {
			return evidence.ErrCorrupt
		}
		length, invalid := domain.ParseCount(ref.ByteLength)
		if invalid != nil || length > 64<<20 {
			return domain.Fail("context_capacity", "An upstream source exceeds the complete local file transport bound.")
		}
		mode := "file"
		if length <= 16<<10 {
			body, err := e.readGraphArtifact(ctx, ref, meta.Kind, 16<<10)
			if err != nil {
				return err
			}
			if utf8.Valid(body) {
				mode = "inline"
			}
		}
		if err = remember(ref); err != nil {
			return err
		}
		result.SupportedAccess = append(result.SupportedAccess, domain.GraphProvidedAccess{Ref: name, ArtifactRef: ref, Scope: scope, Paths: []string{}, Mode: mode})
		return nil
	}
	for _, ref := range result.ConstraintRefs {
		if err = addAccess(ref.Ref, ref.AuthorityScope, ref.ArtifactRef, false); err != nil {
			return empty, err
		}
	}
	for _, ref := range result.ContributedEvidence {
		if err = addAccess(ref.Ref, ref.Scope, ref.ArtifactRef, false); err != nil {
			return empty, err
		}
	}
	for _, source := range result.Sources {
		if err = addAccess(source.Ref+"-original", "upstream_local", source.OriginalRef, true); err != nil {
			return empty, err
		}
		if source.GuidanceRef != nil {
			if err = addAccess(source.Ref+"-guidance", "upstream_local", *source.GuidanceRef, true); err != nil {
				return empty, err
			}
		}
		if err = addAccess(source.Ref+"-acceptance", "upstream_local", *source.AcceptanceRef, true); err != nil {
			return empty, err
		}
	}
	sort.Slice(result.Sources, func(i, j int) bool { return result.Sources[i].Ref < result.Sources[j].Ref })
	sort.Slice(result.SupportedAccess, func(i, j int) bool { return result.SupportedAccess[i].Ref < result.SupportedAccess[j].Ref })
	sort.Slice(result.ArtifactRefs, func(i, j int) bool { return result.ArtifactRefs[i].ArtifactID < result.ArtifactRefs[j].ArtifactID })
	if err = e.db.CheckGraphLocalOperation(ctx, e.config.OwnerInstanceID, op.Attempt.ID); err != nil {
		return empty, err
	}
	return result, nil
}

// Read only the captured source node's own receiver original/guidance/acceptance.
// Its inherited sources are independently obtained through the transitive vector.
func (e *Engine) graphOwnUpstreamSource(ctx context.Context, item domain.GraphContributorBinding, conditions domain.GraphConditionsBinding) (domain.GraphConditionSource, error) {
	var zero domain.GraphConditionSource
	var own *domain.GraphConditionSource
	for _, source := range conditions.Sources {
		if source.AuthorityScope != "receiver" {
			continue
		}
		if own != nil || source.NodeID == nil || *source.NodeID != item.NodeID || source.TaskVersion == nil || *source.TaskVersion != item.TaskVersion {
			return zero, evidence.ErrCorrupt
		}
		copy := cloneGraphConditionSource(source)
		own = &copy
	}
	if own == nil || own.AcceptanceRef == nil {
		return zero, evidence.ErrCorrupt
	}
	captured, err := e.db.TaskVersion(ctx, item.TaskID, item.TaskVersion)
	if err != nil {
		return zero, err
	}
	original, err := e.readGraphArtifact(ctx, own.OriginalRef, "graph_original", 64<<20)
	if err != nil || string(original) != captured.OriginalText {
		return zero, evidence.ErrCorrupt
	}
	if (own.GuidanceRef == nil) != (captured.Version.GuidanceText == "") {
		return zero, evidence.ErrCorrupt
	}
	if own.GuidanceRef != nil {
		guidance, err := e.readGraphArtifact(ctx, *own.GuidanceRef, "graph_original", 64<<20)
		if err != nil || string(guidance) != captured.Version.GuidanceText {
			return zero, evidence.ErrCorrupt
		}
	}
	acceptance, err := e.readGraphArtifact(ctx, *own.AcceptanceRef, "graph_acceptance", 64<<20)
	if err != nil {
		return zero, err
	}
	expected, err := json.Marshal(captured.Version.Acceptance)
	if err != nil || !bytes.Equal(acceptance, expected) {
		return zero, evidence.ErrCorrupt
	}
	own.Ref = "upstream-" + item.NodeID + "-v" + item.TaskVersion
	own.AuthorityScope = "upstream_local"
	return *own, nil
}

func (e *Engine) checkGraphContextOperation(ctx context.Context, op domain.GraphInputOperation) error {
	if op.Kind != "node_input" || !domain.ValidID(op.GraphID) || !domain.ValidID(op.ID) || !domain.ValidID(op.Attempt.ID) ||
		op.Control.NodeID == nil || op.Control.TaskID == nil || op.Control.TaskVersion == nil || op.RootBinding == nil {
		return domain.Invalid("exact current graph context operation")
	}
	actual, err := e.db.GraphInputOperationForAttempt(ctx, e.config.OwnerInstanceID, op.Attempt.ID)
	if err != nil {
		return err
	}
	if actual.Kind != op.Kind || actual.ID != op.ID || actual.GraphID != op.GraphID || actual.RootTaskID != op.RootTaskID ||
		actual.Attempt.ID != op.Attempt.ID || actual.Attempt.TaskID != op.Attempt.TaskID || actual.Attempt.TaskVersion != op.Attempt.TaskVersion ||
		actual.Attempt.OwnerInstanceID != op.Attempt.OwnerInstanceID || actual.Attempt.OwnerInstanceID != e.config.OwnerInstanceID ||
		actual.Attempt.Phase != "preparing" || op.Attempt.Phase != "preparing" || actual.Attempt.ExecutorKind != "local_graph" ||
		actual.Attempt.ExecutorKind != op.Attempt.ExecutorKind || actual.Attempt.Role != op.Attempt.Role || actual.DeadlineAt != op.DeadlineAt ||
		!reflect.DeepEqual(actual.Control, op.Control) || !reflect.DeepEqual(actual.Definition, op.Definition) ||
		!reflect.DeepEqual(actual.RootBinding, op.RootBinding) || !reflect.DeepEqual(actual.Contributions, op.Contributions) ||
		!reflect.DeepEqual(actual.ContributorVector, op.ContributorVector) || !reflect.DeepEqual(actual.InputRecord.ExpectedContributions, op.InputRecord.ExpectedContributions) {
		return domain.Fail("graph_input_stale", "Use only the exact persisted current graph preparation tuple before retaining sources.")
	}
	if invalid := domain.ValidateGraphRootBinding(*op.RootBinding); invalid != nil {
		return invalid
	}
	digest, err := domain.GraphRootDigest(*op.RootBinding)
	if err != nil || digest != op.RootBinding.BasisSHA256 {
		return evidence.ErrCorrupt
	}
	return e.db.CheckGraphLocalOperation(ctx, e.config.OwnerInstanceID, op.Attempt.ID)
}
