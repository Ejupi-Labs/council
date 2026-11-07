package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// A cumulative direct proof is separate from the receiver's node-local proof.
// Only an exact registered selected final basis may supply planner facts.
func (s *Store) graphFinalPlanFacts(ctx context.Context, tx *sql.Tx, w domain.Workflow, a domain.Attempt, f domain.WorkflowStepFacts) (*workflowpolicy.GraphFinalPlanFacts, error) {
	member, err := s.graphMembership(ctx, tx, w.TaskID)
	if err != nil || member == nil || member.Kind != "node" || member.NodeID == nil || w.SelectedCandidateID == nil {
		return nil, err
	}
	h, err := s.graphHeader(ctx, tx, member.GraphID)
	if err != nil {
		return nil, err
	}
	if h.Summary.Stop != nil || h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != *member.NodeID {
		return nil, nil
	}
	version, err := s.version(ctx, tx, w.TaskID, w.TaskVersion)
	if err != nil {
		return nil, err
	}
	if version.Acceptance.Deliverable == "response_only" {
		return nil, nil
	}
	task, err := s.task(ctx, tx, w.TaskID)
	if err != nil {
		return nil, err
	}
	if task.CurrentVersion != w.TaskVersion {
		return nil, nil
	}
	candidate, err := s.candidate(ctx, tx, *w.SelectedCandidateID)
	if err != nil {
		return nil, err
	}
	producer, err := s.attempt(ctx, tx, candidate.AttemptID)
	if err != nil {
		return nil, err
	}
	if producer.Phase != "settled" || producer.Outcome == nil || *producer.Outcome != "succeeded" || producer.Stop != nil || producer.PrimaryFailure != nil || producer.Cleanup.State != "verified" || producer.EvidenceStatus != "complete" {
		return nil, nil
	}
	output, err := s.graphDispatchForAttempt(ctx, tx, producer.ID)
	if err != nil {
		return nil, err
	}
	observation := output.Public.OutputObservation
	if observation == nil || observation.Candidate == nil || observation.Candidate.CandidateID != candidate.ID || observation.Candidate.State != "complete" || observation.Candidate.FinalVerificationRef == nil || output.FinalVerification == nil || output.CandidateClosure == nil {
		return nil, nil
	}
	final, ref := *output.FinalVerification, *observation.Candidate.FinalVerificationRef
	if final.FinalCandidateBinding.CandidateID != candidate.ID || final.FinalWorkflowID != w.ID || final.FinalTaskID != w.TaskID || final.FinalTaskVersion != w.TaskVersion || output.Public.Dispatch.GraphID != h.Summary.ID || output.Public.Dispatch.NodeID != *member.NodeID {
		return nil, evidenceUnavailable()
	}
	current, err := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if err != nil {
		return nil, err
	}
	d := current.Public.Dispatch
	if d.GraphID != h.Summary.ID || d.GraphVersion != h.Summary.GraphVersion || d.NodeID != *member.NodeID || d.TaskID != w.TaskID || d.TaskVersion != w.TaskVersion || d.WorkflowID != w.ID || d.StepID != a.StepID || d.InputID != final.FinalInputID {
		return nil, evidenceUnavailable()
	}
	if a.ID == producer.ID {
		if f.GraphOutput == nil || f.GraphOutput.FinalVerification == nil || !reflect.DeepEqual(*f.GraphOutput.FinalVerification, final) || f.GraphOutput.Observation.Candidate == nil || f.GraphOutput.Observation.Candidate.FinalVerificationRef == nil || *f.GraphOutput.Observation.Candidate.FinalVerificationRef != ref {
			return nil, evidenceUnavailable()
		}
	} else if d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.CandidateBinding == nil || !reflect.DeepEqual(*d.SourceBasis.CandidateBinding, final.FinalCandidateBinding) || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != ref {
		return nil, nil // An ordinary node-local proof cannot stand in for graph scope.
	}
	input, err := s.graphInputRecord(ctx, tx, h.Summary.ID, d.InputID)
	if err != nil {
		return nil, err
	}
	if !input.Public.Summary.Current || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil || !reflect.DeepEqual(input.Public.InputBinding.ContributorVector, final.ContributorVector) {
		return nil, nil
	}
	if err = s.graphContributorsCurrent(ctx, tx, h.Summary.ID, final.ContributorVector); err != nil {
		return nil, err
	}
	nodes, err := s.graphNodes(ctx, tx, h.Summary.ID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, node := range nodes {
		if node.Summary.ID == d.NodeID {
			found = node.Summary.Stop == nil && node.Summary.TaskVersion == w.TaskVersion && node.Summary.InputID != nil && *node.Summary.InputID == d.InputID && node.Summary.NodeAdmissionID != nil && *node.Summary.NodeAdmissionID == d.NodeAdmissionID
		}
	}
	if !found {
		return nil, nil
	}
	proofs, err := graphFinalPlannerProofs(f)
	if err != nil {
		return nil, err
	}
	if err = s.candidateBinding(ctx, tx, w, final.FinalCandidateBinding, proofs); err != nil {
		return nil, err
	}
	if err = s.graphOutputClosure(ctx, tx, producer, output.Public.Dispatch, "candidate", observation.Candidate.State, observation.Candidate.ClosureRef, output.CandidateClosure, proofs); err != nil {
		return nil, err
	}
	verified := &domain.GraphStepOutputFacts{Observation: *observation, CandidateClosure: output.CandidateClosure, FinalVerification: &final, VerifiedArtifacts: proofs}
	if err = s.graphFinalOutput(ctx, tx, producer, output.Public.Dispatch, *observation.Candidate, verified, &final.FinalCandidateBinding); err != nil {
		return nil, err
	}
	return &workflowpolicy.GraphFinalPlanFacts{VerificationBindingRef: ref, DeclaredFileFinal: true, CurrentSubject: final.FinalCandidateBinding, HasCumulativeWorkingDelta: final.HasCumulativeWorkingDelta, HasCumulativeIndexDelta: final.HasCumulativeIndexDelta, CumulativeDirectProven: f.CumulativeDirectProven}, nil
}

func graphFinalPlannerProofs(f domain.WorkflowStepFacts) ([]domain.VerifiedResponse, error) {
	groups := [][]domain.VerifiedResponse{f.VerifiedArtifacts}
	if f.GraphOutput != nil {
		groups = append(groups, f.GraphOutput.VerifiedArtifacts)
	}
	known := map[string]domain.VerifiedResponse{}
	out := []domain.VerifiedResponse{}
	for _, group := range groups {
		if len(group) > 256 {
			return nil, domain.Invalid("bounded graph final artifact proofs")
		}
		for _, proof := range group {
			if old, found := known[proof.ArtifactID]; found {
				if old != proof {
					return nil, evidenceUnavailable()
				}
				continue
			}
			known[proof.ArtifactID] = proof
			out = append(out, proof)
		}
	}
	if len(out) > 256 {
		return nil, domain.Invalid("bounded graph final artifact proofs")
	}
	return out, nil
}
