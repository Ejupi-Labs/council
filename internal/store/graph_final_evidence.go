package store

import (
	"context"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Applicability cannot promote node-only or former cumulative evidence to the
// selected final basis. Preserve the immutable records, and remove only their
// uses from the current workflow qualification.
func (s *Store) graphFinalEvidenceUses(ctx context.Context, q queryer, w domain.Workflow, uses []domain.EvidenceUse) ([]domain.EvidenceUse, error) {
	if len(uses) > domain.MaxWorkflowChecks+1 {
		return nil, evidenceUnavailable()
	}
	member, err := s.graphMembership(ctx, q, w.TaskID)
	if err != nil || member == nil || member.Kind != "node" || member.NodeID == nil || w.SelectedCandidateID == nil {
		return uses, err
	}
	h, err := s.graphHeader(ctx, q, member.GraphID)
	if err != nil {
		return nil, err
	}
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != *member.NodeID {
		return uses, nil
	}
	candidate, err := s.candidate(ctx, q, *w.SelectedCandidateID)
	if err != nil {
		return nil, err
	}
	producer, err := s.graphDispatchForAttempt(ctx, q, candidate.AttemptID)
	if err != nil {
		return nil, err
	}
	observation := producer.Public.OutputObservation
	if observation == nil || observation.Candidate == nil || observation.Candidate.State != "complete" || observation.Candidate.FinalVerificationRef == nil || producer.FinalVerification == nil {
		return uses, nil
	}
	final, ref := *producer.FinalVerification, *observation.Candidate.FinalVerificationRef
	if final.GraphID != h.Summary.ID || final.FinalNodeID != *member.NodeID || final.FinalTaskID != w.TaskID || final.FinalTaskVersion != w.TaskVersion || final.FinalWorkflowID != w.ID || final.FinalCandidateBinding.CandidateID != candidate.ID || observation.Candidate.CandidateID != candidate.ID {
		return nil, evidenceUnavailable()
	}
	out := []domain.EvidenceUse{}
	seen := map[string]bool{}
	for _, use := range uses {
		if !useSubjectMatches(w, use) || seen[use.EvidenceID] {
			return nil, evidenceUnavailable()
		}
		seen[use.EvidenceID] = true
		var attemptID, workflowID, stepID string
		switch use.Kind {
		case "review":
			record, err := storedWorkflowRecord[domain.ReviewRecord](ctx, q, "workflow_reviews", use.EvidenceID)
			if err != nil {
				return nil, err
			}
			attemptID, workflowID, stepID = record.AttemptID, record.WorkflowID, record.StepID
		case "check":
			record, err := storedWorkflowRecord[domain.CheckReceipt](ctx, q, "workflow_checks", use.EvidenceID)
			if err != nil {
				return nil, err
			}
			attemptID, workflowID, stepID = record.AttemptID, record.WorkflowID, record.StepID
		default:
			return nil, evidenceUnavailable()
		}
		if workflowID != w.ID {
			return nil, evidenceUnavailable()
		}
		origin, err := s.graphDispatchForAttempt(ctx, q, attemptID)
		if err != nil {
			return nil, err
		}
		d := origin.Public.Dispatch
		if d.GraphID != h.Summary.ID || d.NodeID != *member.NodeID || d.TaskID != w.TaskID || d.TaskVersion != w.TaskVersion || d.WorkflowID != workflowID || d.StepID != stepID || d.AttemptID != attemptID {
			return nil, evidenceUnavailable()
		}
		if d.InputID != final.FinalInputID || !reflect.DeepEqual(d.ContributorVector, final.ContributorVector) || d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != ref || d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != final.FinalCandidateBinding || d.SourceBasis.ResponseBinding != nil {
			continue
		}
		out = append(out, use)
	}
	return out, nil
}
