package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

// Acknowledgement rechecks the retained cumulative result and recorded effect.
// It does not acquire a claim, reconstruct a workspace or renew an allowance.
func (e *Engine) graphFinalAcceptanceProofs(ctx context.Context, subject workflowSubject, operationID *string) ([]domain.VerifiedResponse, error) {
	final, err := e.loadGraphFinalVerification(ctx, subject)
	if err != nil || final == nil {
		return nil, err
	}
	ids := append([]string{}, subject.artifactIDs()...)
	for _, contributor := range final.Binding.ContributorVector {
		// These are the exact current vector's immutable node-local source
		// headers, including an earlier author root after a correction.
		sourceIDs, err := e.graphDeliverySourceIDs(ctx, domain.AcceptedContribution{CandidateBinding: contributor.CandidateBinding, ResponseBinding: contributor.ResponseBinding})
		if err != nil {
			return nil, err
		}
		ids = append(ids, sourceIDs...)
	}
	if operationID != nil {
		operation, err := e.db.DeliveryOperation(ctx, *operationID)
		if err != nil {
			return nil, err
		}
		if operation.GraphDeliveryRef == nil || operation.WorkflowID != subject.Workflow.ID || subject.Binding == nil || operation.CandidateBinding != *subject.Binding || operation.Status != "applied" {
			return nil, evidence.ErrCorrupt
		}
		if _, err = e.verifyGraphArtifact(ctx, *operation.GraphDeliveryRef, "graph_delivery"); err != nil {
			return nil, err
		}
		ids = append(ids, operation.GraphDeliveryRef.ArtifactID)
	}
	proofs, err := e.verifyWorkflowArtifacts(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return mergeGraphAcceptanceProofs(proofs, final.VerifiedArtifacts)
}
