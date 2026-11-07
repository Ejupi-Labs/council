package store

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Admission verifies fresh controller proofs. Preparation/GO repeat the exact
// immutable association only; physical freshness is rechecked by the controller
// outside SQL and is never inferred from this cached metadata.
func (s *Store) graphStageFinalRef(ctx context.Context, q queryer, w domain.Workflow, basis domain.GraphSourceBasis, proofs []domain.VerifiedResponse, fresh bool) error {
	unexpected := func() error {
		if basis.FinalVerificationRef != nil {
			return evidenceUnavailable()
		}
		return nil
	}
	member, err := s.graphMembership(ctx, q, w.TaskID)
	if err != nil {
		return err
	}
	if member == nil || member.Kind != "node" || member.NodeID == nil || w.SelectedCandidateID == nil || w.SelectedResponseBinding != nil {
		return unexpected()
	}
	h, err := s.graphHeader(ctx, q, member.GraphID)
	if err != nil {
		return err
	}
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != *member.NodeID {
		return unexpected()
	}
	v, err := s.version(ctx, q, w.TaskID, w.TaskVersion)
	if err != nil {
		return err
	}
	if v.Acceptance.Deliverable == "response_only" {
		return unexpected()
	}
	candidate, err := s.candidate(ctx, q, *w.SelectedCandidateID)
	if err != nil {
		return err
	}
	producer, err := s.attempt(ctx, q, candidate.AttemptID)
	if err != nil {
		return err
	}
	output, err := s.graphDispatchForAttempt(ctx, q, producer.ID)
	if err != nil {
		return err
	}
	observation := output.Public.OutputObservation
	if observation == nil || observation.Candidate == nil || observation.Candidate.FinalVerificationRef == nil {
		return unexpected()
	}
	if observation.Candidate.State != "complete" || observation.Candidate.ClosureRef == nil || output.FinalVerification == nil || output.CandidateClosure == nil || producer.Phase != "settled" || producer.Outcome == nil || *producer.Outcome != "succeeded" || producer.Stop != nil || producer.PrimaryFailure != nil || producer.Cleanup.State != "verified" || producer.EvidenceStatus != "complete" {
		return evidenceUnavailable()
	}
	d, final, closure := output.Public.Dispatch, *output.FinalVerification, *output.CandidateClosure
	ref := *observation.Candidate.FinalVerificationRef
	selected := final.FinalCandidateBinding
	if basis.FinalVerificationRef == nil || *basis.FinalVerificationRef != ref || basis.BasisKind != "node_subject" || basis.CandidateBinding == nil || *basis.CandidateBinding != selected || basis.ResponseBinding != nil || basis.InputID != final.FinalInputID || basis.InputSHA256 != d.InputSHA256 || basis.ExpectedManifest != closure.Manifest || basis.ExpectedIndex != closure.Index ||
		ref.ProducerAttemptID != producer.ID || final.GraphID != h.Summary.ID || final.GraphVersion != h.Summary.GraphVersion || final.FinalNodeID != *member.NodeID || final.FinalTaskID != w.TaskID || final.FinalTaskVersion != w.TaskVersion || final.FinalWorkflowID != w.ID || final.FinalInputID != d.InputID || !reflect.DeepEqual(final.ContributorVector, d.ContributorVector) ||
		d.GraphID != h.Summary.ID || d.GraphVersion != h.Summary.GraphVersion || d.NodeID != *member.NodeID || d.TaskID != w.TaskID || d.TaskVersion != w.TaskVersion || d.WorkflowID != w.ID || d.AttemptID != producer.ID ||
		selected.CandidateID != candidate.ID || observation.Candidate.CandidateID != candidate.ID || selected.SourceSnapshotID != candidate.SourceSnapshotID || selected.RootSourceSnapshotID != candidate.RootSourceSnapshotID || selected.ManifestArtifactID != candidate.ManifestArtifactID || selected.IndexArtifactID != candidate.IndexArtifactID || selected.ConditionsSHA256 != w.Authority.ConditionsSHA256 ||
		closure.CandidateID == nil || *closure.CandidateID != candidate.ID || closure.Kind != "candidate" || closure.ProducerAttemptID != producer.ID || closure.GraphID != d.GraphID || closure.NodeID != d.NodeID || closure.ReceivedInputID != d.InputID || closure.Manifest.ArtifactID != selected.ManifestArtifactID || closure.Manifest.SHA256 != selected.ManifestSHA256 || closure.Index.ArtifactID != selected.IndexArtifactID || closure.Index.SHA256 != selected.IndexSHA256 || final.OutputClosureRef != *observation.Candidate.ClosureRef ||
		!graphRefHasBytes(ref, []byte(encode(final))) || !graphRefHasBytes(*observation.Candidate.ClosureRef, []byte(encode(closure))) {
		return evidenceUnavailable()
	}
	digest, err := domain.GraphFinalVerificationDigest(final)
	if err != nil || digest != final.BindingSHA256 {
		return evidenceUnavailable()
	}
	digest, err = domain.GraphOutputDigest(closure)
	if err != nil || digest != closure.ClosureSHA256 {
		return evidenceUnavailable()
	}
	if err = s.graphStoredRef(ctx, q, d.GraphID, ref, "graph_final_verification"); err != nil {
		return err
	}
	if err = s.graphStoredRef(ctx, q, d.GraphID, *observation.Candidate.ClosureRef, "graph_output_closure"); err != nil {
		return err
	}
	if !fresh {
		return nil
	}
	if err = s.candidateBinding(ctx, q, w, selected, proofs); err != nil {
		return err
	}
	if err = s.graphOutputClosure(ctx, q, producer, d, "candidate", observation.Candidate.State, observation.Candidate.ClosureRef, &closure, proofs); err != nil {
		return err
	}
	facts := &domain.GraphStepOutputFacts{Observation: *observation, CandidateClosure: &closure, FinalVerification: &final, VerifiedArtifacts: proofs}
	return s.graphFinalOutput(ctx, q, producer, d, *observation.Candidate, facts, &selected)
}
