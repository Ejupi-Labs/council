package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// Acceptance is a nonexecuting acknowledgement. Recheck current content and
// destination facts without asking a settled delivery to reacquire its old
// claim, owner epoch or native time window.
func (s *Store) graphAcceptanceFinal(ctx context.Context, tx *sql.Tx, w domain.Workflow, a domain.Attempt, f domain.WorkflowAcceptanceFacts) (*workflowpolicy.GraphFinalPlanFacts, error) {
	member, err := s.graphMembership(ctx, tx, w.TaskID)
	if err != nil || member == nil || member.Kind != "node" || member.NodeID == nil {
		return nil, err
	}
	h, err := s.graphHeader(ctx, tx, member.GraphID)
	if err != nil {
		return nil, err
	}
	if h.Summary.FinalNodeID == nil || *h.Summary.FinalNodeID != *member.NodeID {
		return nil, nil
	}
	definition, err := s.graphDefinition(ctx, tx, h.Summary.ID, h.Summary.GraphVersion)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(definition.Delivery, w.Authority.Delivery) || w.SelectedCandidateID == nil || w.SelectedResponseBinding != nil {
		return nil, evidenceUnavailable()
	}
	candidate, err := s.candidate(ctx, tx, *w.SelectedCandidateID)
	if err != nil {
		return nil, err
	}
	output, err := s.graphDispatchForAttempt(ctx, tx, candidate.AttemptID)
	if err != nil {
		return nil, err
	}
	current, err := s.graphDispatchForAttempt(ctx, tx, a.ID)
	if err != nil {
		return nil, err
	}
	facts := domain.WorkflowStepFacts{VerifiedArtifacts: f.VerifiedArtifacts}
	// With no review use, the exact current canonical acceptance proof retains
	// the prior cumulative-direct qualification. Fresh sealed content/original
	// proofs below reverify that same basis; this never renews native authority.
	if len(f.Acceptance.ReviewUses) == 0 && w.CurrentStepID != nil {
		step, err := s.workflowStep(ctx, tx, *w.CurrentStepID)
		if err != nil {
			return nil, err
		}
		key, invalid := domain.WorkflowStageKey("accept", w.CorrectionCyclesUsed, "", candidate.ID)
		p := step.AdvanceProof
		facts.CumulativeDirectProven = invalid == nil && step.Status == "succeeded" && step.AttemptID != nil && *step.AttemptID == a.ID && p != nil && p.PredecessorStepID == step.ID && p.TaskVersion == w.TaskVersion && p.WorkflowRevision == w.Revision && p.NextLogicalKey == key && p.CandidateID != nil && *p.CandidateID == candidate.ID && p.ResponseBinding == nil && p.GraphBudgetAuthorityEpoch != ""
	}
	if a.ID == candidate.AttemptID {
		if output.Public.OutputObservation == nil {
			return nil, evidenceUnavailable()
		}
		facts.GraphOutput = &domain.GraphStepOutputFacts{Observation: *output.Public.OutputObservation, CandidateClosure: output.CandidateClosure, ResponseClosure: output.ResponseClosure, FinalVerification: output.FinalVerification, VerifiedArtifacts: f.VerifiedArtifacts}
	}
	final, err := s.graphFinalPlanFacts(ctx, tx, w, a, facts)
	if err != nil {
		return nil, err
	}
	if final == nil || output.FinalVerification == nil || final.CurrentSubject != f.Acceptance.CandidateBinding {
		return nil, domain.Fail("acceptance_unavailable", "The current cumulative graph final basis is unavailable.")
	}
	sealed := *output.FinalVerification
	binding := domain.GraphDeliveryBinding{GraphID: h.Summary.ID, GraphVersion: h.Summary.GraphVersion, GraphControlRevision: h.Summary.ControlRevision, RootBinding: sealed.RootBinding, FinalNodeID: *member.NodeID, FinalTaskID: w.TaskID, FinalTaskVersion: w.TaskVersion, FinalWorkflowID: w.ID, FinalCandidateBinding: sealed.FinalCandidateBinding, FinalInputID: sealed.FinalInputID, ContributorVector: sealed.ContributorVector, FinalVerificationRef: final.VerificationBindingRef, OtherNodeAcceptances: sealed.ContributorVector, PreDeliveryEvidenceUses: w.EvidenceUses}
	if err = s.graphDeliveryAcceptances(ctx, tx, h, definition, current.Public.Dispatch, binding, f.VerifiedArtifacts); err != nil {
		return nil, err
	}
	if w.Authority.Delivery.LocalMode != "apply_verified_local" {
		return final, nil
	}
	if f.Acceptance.DeliveryOperationID == nil {
		return nil, evidenceUnavailable()
	}
	op, err := storedWorkflowRecord[domain.DeliveryOperation](ctx, tx, "delivery_operations", *f.Acceptance.DeliveryOperationID)
	if err != nil {
		return nil, err
	}
	if op.Status != "applied" || op.AfterDestination == nil || op.WorkflowID != w.ID || op.CandidateBinding != sealed.FinalCandidateBinding || !reflect.DeepEqual(op.Scope, w.Authority.Delivery) {
		return nil, evidenceUnavailable()
	}
	cached, err := s.loadGraphDelivery(ctx, tx, op)
	if err != nil {
		return nil, err
	}
	if cached == nil {
		return nil, evidenceUnavailable()
	}
	scope, err := domain.GraphDeliveryScopeDigest(w.Authority.Delivery)
	if err != nil {
		return nil, err
	}
	binding.DeliveryScopeSHA256 = scope
	expected := binding
	expected.OtherNodeAcceptances = cached.Binding.OtherNodeAcceptances
	expected.PreDeliveryEvidenceUses = cached.Binding.PreDeliveryEvidenceUses
	expected.BindingSHA256 = cached.Binding.BindingSHA256
	if !reflect.DeepEqual(cached.Binding, expected) {
		return nil, evidenceUnavailable()
	}
	operationDispatch, err := s.graphDispatchForAttempt(ctx, tx, op.AttemptID)
	if err != nil {
		return nil, err
	}
	d := operationDispatch.Public.Dispatch
	if d.GraphID != h.Summary.ID || d.NodeID != *member.NodeID || d.TaskID != w.TaskID || d.TaskVersion != w.TaskVersion || d.WorkflowID != w.ID || d.StepID != op.StepID || d.ExecutorKind != "local_application" || d.Role != "local_delivery" || d.InputID != sealed.FinalInputID || d.SourceBasis.FinalVerificationRef == nil || *d.SourceBasis.FinalVerificationRef != final.VerificationBindingRef || d.SourceBasis.CandidateBinding == nil || *d.SourceBasis.CandidateBinding != sealed.FinalCandidateBinding {
		return nil, evidenceUnavailable()
	}
	if err = s.verifyGraphRef(ctx, tx, cached.BindingRef, f.VerifiedArtifacts, "graph_delivery"); err != nil {
		return nil, err
	}
	if err = s.graphDeliveryAcceptances(ctx, tx, h, definition, current.Public.Dispatch, cached.Binding, f.VerifiedArtifacts); err != nil {
		return nil, err
	}
	if err = s.graphDeliveryEvidence(ctx, tx, w, cached.Binding, f.VerifiedArtifacts, final); err != nil {
		return nil, err
	}
	return final, nil
}
