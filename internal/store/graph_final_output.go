package store

import (
	"context"
	"reflect"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This validates retained producer facts inside the terminal transaction. It
// does not accept a graph result or authorize delivery.
func (s *Store) graphFinalOutput(ctx context.Context, q queryer, a domain.Attempt, d domain.GraphDispatch, observation domain.GraphCandidateOutputObservation, f *domain.GraphStepOutputFacts, selected *domain.CandidateBinding) error {
	if observation.FinalVerificationRef == nil {
		if f != nil && f.FinalVerification != nil {
			return evidenceUnavailable()
		}
		return nil
	}
	if f == nil || f.FinalVerification == nil || f.CandidateClosure == nil || observation.ClosureRef == nil || selected == nil {
		return evidenceUnavailable()
	}
	final := *f.FinalVerification
	ref := *observation.FinalVerificationRef
	if ref.ProducerAttemptID != a.ID || !graphRefHasBytes(ref, []byte(encode(final))) {
		return evidenceUnavailable()
	}
	digest, err := domain.GraphFinalVerificationDigest(final)
	if err != nil || digest != final.BindingSHA256 {
		return evidenceUnavailable()
	}
	h, err := s.graphHeader(ctx, q, d.GraphID)
	if err != nil {
		return err
	}
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != d.NodeID || h.Summary.RootInputID == nil ||
		d.BudgetOwnerTaskID != h.Summary.RootTaskID || d.AttemptID != a.ID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID ||
		final.GraphID != d.GraphID || final.GraphVersion != d.GraphVersion || final.FinalNodeID != d.NodeID || final.FinalTaskID != d.TaskID || final.FinalTaskVersion != d.TaskVersion || final.FinalWorkflowID != d.WorkflowID ||
		final.FinalInputID != d.InputID || !reflect.DeepEqual(final.ContributorVector, d.ContributorVector) ||
		!reflect.DeepEqual(final.FinalCandidateBinding, *selected) || final.FinalCandidateBinding.CandidateID != observation.CandidateID || final.OutputClosureRef != *observation.ClosureRef {
		return evidenceUnavailable()
	}
	version, err := s.version(ctx, q, d.TaskID, d.TaskVersion)
	if err != nil {
		return err
	}
	if version.Acceptance.Deliverable == "response_only" {
		return evidenceUnavailable()
	}
	w, err := s.workflowByID(ctx, q, d.WorkflowID)
	if err != nil {
		return err
	}
	candidate, err := s.candidate(ctx, q, observation.CandidateID)
	if err != nil {
		return err
	}
	closure := *f.CandidateClosure
	if w.TaskID != d.TaskID || w.TaskVersion != d.TaskVersion || candidate.AttemptID != a.ID || candidate.WorkflowID != d.WorkflowID ||
		candidate.RootSourceSnapshotID != selected.RootSourceSnapshotID || candidate.SourceSnapshotID != selected.SourceSnapshotID ||
		candidate.ManifestArtifactID != selected.ManifestArtifactID || candidate.IndexArtifactID != selected.IndexArtifactID ||
		candidate.ConditionsSHA256 != w.Authority.ConditionsSHA256 || selected.ConditionsSHA256 != w.Authority.ConditionsSHA256 ||
		closure.Manifest.ArtifactID != selected.ManifestArtifactID || closure.Manifest.SHA256 != selected.ManifestSHA256 || closure.Index.ArtifactID != selected.IndexArtifactID || closure.Index.SHA256 != selected.IndexSHA256 {
		return evidenceUnavailable()
	}
	rootRecord, err := s.graphInputRecord(ctx, q, d.GraphID, *h.Summary.RootInputID)
	if err != nil {
		return err
	}
	if rootRecord.PrivateRoot == nil || !reflect.DeepEqual(final.RootBinding, domain.GraphRootPublic(*rootRecord.PrivateRoot)) {
		return evidenceUnavailable()
	}
	root := *rootRecord.PrivateRoot
	record, err := s.graphInputRecord(ctx, q, d.GraphID, d.InputID)
	if err != nil {
		return err
	}
	if record.PrivateRoot == nil || !reflect.DeepEqual(*record.PrivateRoot, root) || record.Public.InputBinding == nil || record.Public.ConditionsRef == nil || record.ConditionsBinding == nil || record.Public.ContextPacket == nil {
		return evidenceUnavailable()
	}
	input, conditions, packet := *record.Public.InputBinding, *record.ConditionsBinding, *record.Public.ContextPacket
	if input.ID != d.InputID || input.GraphID != d.GraphID || input.GraphVersion != d.GraphVersion || input.InputSHA256 != d.InputSHA256 ||
		input.RootBasisSHA256 != root.BasisSHA256 || input.ReceivingNodeID != d.NodeID || input.ReceivingTaskID != d.TaskID || input.ReceivingTaskVersion != d.TaskVersion || !reflect.DeepEqual(input.ContributorVector, d.ContributorVector) ||
		final.GraphConditionsRef != *record.Public.ConditionsRef || final.GraphConditionsSHA256 != conditions.ConditionsSHA256 ||
		conditions.GraphID != d.GraphID || conditions.GraphVersion != d.GraphVersion || conditions.ReceiverTaskID != d.TaskID || conditions.ReceiverTaskVersion != d.TaskVersion ||
		!graphRefHasBytes(final.GraphConditionsRef, []byte(encode(conditions))) || domain.ValidateGraphConditionsBinding(conditions) != nil ||
		packet.GraphID != d.GraphID || packet.InputID != input.ID || packet.InputSHA256 != input.InputSHA256 || packet.ReceivingTaskVersion != d.TaskVersion || !graphRefHasBytes(input.ContextPacket, []byte(encode(packet))) {
		return evidenceUnavailable()
	}
	digest, err = domain.GraphConditionsDigest(conditions)
	if err != nil || digest != conditions.ConditionsSHA256 {
		return evidenceUnavailable()
	}
	digest, err = domain.GraphContextDigest(packet)
	if err != nil || digest != packet.PacketSHA256 {
		return evidenceUnavailable()
	}
	originals, err := graphFinalOutputOriginals(conditions, packet, final.OriginalAccess)
	if err != nil {
		return err
	}
	if final.CumulativeDeltaRef.ProducerAttemptID != a.ID {
		return evidenceUnavailable()
	}
	// These are fresh controller proofs of actual closed bytes. SQL checks their
	// registered identities and the immutable tuple; it performs no filesystem I/O.
	for _, item := range []graphFinalSourceRef{
		{root.Manifest, "source_manifest"}, {root.Index, "index_snapshot"}, {root.StagedBlobs, "graph_index_blobs"},
		{input.Manifest, "graph_input_manifest"}, {input.Index, "index_snapshot"}, {input.StagedBlobs, "graph_index_blobs"},
		{input.Provenance, "graph_input_provenance"}, {input.ContextPacket, "graph_context"}, {final.GraphConditionsRef, "graph_conditions"},
		{final.OutputClosureRef, "graph_output_closure"}, {final.CumulativeDeltaRef, "graph_cumulative_delta"}, {ref, "graph_final_verification"},
	} {
		if err = s.verifyGraphRef(ctx, q, item.ref, f.VerifiedArtifacts, item.kind); err != nil {
			return err
		}
	}
	for _, original := range originals {
		if err = s.verifyGraphRef(ctx, q, original.ref, f.VerifiedArtifacts, original.kind); err != nil {
			return err
		}
	}
	// Historical producer output can be retained after its controls become stale.
	// Currentness, qualification, acceptance and delivery are separate guards.
	return nil
}

type graphFinalSourceRef struct {
	ref  domain.GraphArtifactRef
	kind string
}

// Select the same complete source transports captured in the immutable packet.
// Their scopes are preserved; upstream-local sources gain no receiver authority.
func graphFinalOutputOriginals(conditions domain.GraphConditionsBinding, packet domain.GraphContextPacket, supplied []domain.GraphProvidedAccess) ([]graphFinalSourceRef, error) {
	required := map[string]graphFinalSourceRef{}
	scopes := map[string]map[string]bool{}
	for _, source := range conditions.Sources {
		for _, item := range []struct {
			ref  *domain.GraphArtifactRef
			kind string
		}{{&source.OriginalRef, "graph_original"}, {source.GuidanceRef, "graph_original"}, {source.AcceptanceRef, "graph_acceptance"}} {
			if item.ref == nil {
				continue
			}
			want := graphFinalSourceRef{*item.ref, item.kind}
			if old, found := required[item.ref.ArtifactID]; found && old != want {
				return nil, evidenceUnavailable()
			}
			required[item.ref.ArtifactID] = want
			if scopes[item.ref.ArtifactID] == nil {
				scopes[item.ref.ArtifactID] = map[string]bool{}
			}
			scopes[item.ref.ArtifactID][source.AuthorityScope] = true
		}
	}
	expected := []domain.GraphProvidedAccess{}
	originals := []graphFinalSourceRef{}
	for _, access := range packet.ProvidedAccess {
		if want, found := required[access.ArtifactRef.ArtifactID]; found {
			if access.ArtifactRef != want.ref || len(access.Paths) != 0 || access.Mode != "file" && access.Mode != "inline" || !scopes[access.ArtifactRef.ArtifactID][access.Scope] {
				return nil, evidenceUnavailable()
			}
			expected = append(expected, access)
			originals = append(originals, want)
			delete(required, access.ArtifactRef.ArtifactID)
		}
	}
	if len(required) != 0 || len(expected) == 0 || len(supplied) != len(expected) || len(supplied) > 64 {
		return nil, evidenceUnavailable()
	}
	actual := append([]domain.GraphProvidedAccess{}, supplied...)
	less := func(values []domain.GraphProvidedAccess, i, j int) bool {
		if values[i].Ref == values[j].Ref {
			return values[i].ArtifactRef.ArtifactID < values[j].ArtifactRef.ArtifactID
		}
		return values[i].Ref < values[j].Ref
	}
	sort.Slice(expected, func(i, j int) bool { return less(expected, i, j) })
	sort.Slice(actual, func(i, j int) bool { return less(actual, i, j) })
	if !reflect.DeepEqual(actual, expected) {
		return nil, evidenceUnavailable()
	}
	return originals, nil
}
