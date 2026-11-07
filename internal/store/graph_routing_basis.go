package store

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) graphVersionRoutingBasis(ctx context.Context, q queryer, h graphHeader, options domain.GraphNodeExecutionOptions) (domain.GraphNodeRoutingBasis, error) {
	if options.RoutingBasis != nil {
		return *options.RoutingBasis, nil
	}
	prior, e := s.routingAuthority(ctx, q, options.TaskID, options.TaskVersion)
	if e != nil {
		return domain.GraphNodeRoutingBasis{}, e
	}
	if prior == nil {
		prior, e = s.routingAuthority(ctx, q, h.Summary.RootTaskID, "1")
		if e != nil {
			return domain.GraphNodeRoutingBasis{}, e
		}
	}
	if prior == nil {
		return domain.GraphNodeRoutingBasis{}, evidenceUnavailable()
	}
	basis := domain.GraphNodeRoutingBasis{PolicySnapshot: prior.PolicySnapshot, OptionSnapshot: prior.OptionSnapshot}
	if options.TaskVersion == "1" && options.Routing != nil {
		raw := options.Routing
		if raw.Suitability != "" {
			basis.OptionSnapshot.Suitability = raw.Suitability
		}
		if raw.TargetProfileID != "" {
			basis.OptionSnapshot.TargetProfileID = raw.TargetProfileID
		}
		if raw.Continuation != nil {
			basis.OptionSnapshot.Continuation = raw.Continuation
		}
		if raw.ExpectedPolicyRevision != "" {
			basis.OptionSnapshot.ExpectedPolicyRevision = raw.ExpectedPolicyRevision
		}
	}
	// Every node consumes the root ledger; this is never a second child budget.
	basis.OptionSnapshot.Budget = nil
	return basis, nil
}

func (s *Store) graphGuidanceRoutingBasis(ctx context.Context, q queryer, h graphHeader, options domain.GraphNodeExecutionOptions, patch *domain.RoutingOptions) (domain.GraphNodeRoutingBasis, error) {
	basis, e := s.graphVersionRoutingBasis(ctx, q, h, options)
	if e != nil {
		return basis, e
	}
	if patch != nil {
		basis.OptionSnapshot = *patch
		if patch.UseCurrentPolicy != nil && *patch.UseCurrentPolicy {
			basis.PolicySnapshot, e = s.routingPolicy(ctx, q)
			if e != nil {
				return basis, e
			}
		}
	}
	basis.OptionSnapshot.Budget = nil
	if expected := basis.OptionSnapshot.ExpectedPolicyRevision; expected != "" && expected != basis.PolicySnapshot.Revision {
		return basis, domain.RevisionConflict(basis.PolicySnapshot.Revision)
	}
	effective, invalid := effectiveRoutingPolicy(basis.PolicySnapshot, basis.OptionSnapshot)
	if invalid != nil {
		return basis, invalid
	}
	budget, e := s.rawTaskBudget(ctx, q, h.Summary.RootTaskID)
	if e != nil {
		return basis, e
	}
	if budget == nil {
		return basis, storageError()
	}
	if countInt(effective.Continuation.MaxAutomaticAdmissions) > countInt(budget.GrantedAdmissions) {
		return basis, domain.Invalid("automatic admissions no greater than existing graph authority")
	}
	return basis, nil
}

func graphWorkflowGuidancePatch(base *domain.WorkflowInput, patch *domain.WorkflowInput) domain.WorkflowInput {
	options := domain.WorkflowInput{Mode: "auto", Review: "auto"}
	if base != nil {
		options = *base
	}
	options.Mode, options.Delivery = "auto", nil
	if options.Review == "" {
		options.Review = "auto"
	}
	if patch != nil {
		if patch.Review != "" {
			options.Review = patch.Review
		}
		if patch.Checks != nil {
			options.Checks = patch.Checks
		}
		if patch.MaxCorrectionCycles != nil {
			options.MaxCorrectionCycles = patch.MaxCorrectionCycles
		}
	}
	return options
}
