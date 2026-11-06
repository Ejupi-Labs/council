package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// MutateRouting orders mutable eligibility and authority against the native
// launch guard. Public reports cannot bypass an uncertain owner/store fence.
func (e *Engine) MutateRouting(ctx context.Context, op, id string, body []byte) domain.Result {
	requestID := domain.MutationIdentity(body)
	var input any
	switch op {
	case "routing.policy.update":
		input = &domain.RoutingPolicyInput{}
	case "routing.context.update":
		input = &domain.ContextUpdateInput{}
	case "routing.profile.update":
		input = &domain.ProfileUpdateInput{}
	case "routing.accounts.create":
		input = &domain.AccountCreateInput{}
	case "routing.accounts.update":
		input = &domain.AccountUpdateInput{}
	case "routing.links.update":
		input = &domain.AccountLinkInput{}
	case "routing.reports.create":
		input = &domain.CapacityReportInput{}
	case "tasks.budget.amend":
		input = &domain.BudgetAmendmentInput{}
	default:
		return e.failure(requestID, domain.Fail("not_found", "The requested routing mutation does not exist."))
	}
	if err := domain.StrictDecode(body, input); err != nil {
		return e.failure(requestID, err)
	}
	switch op {
	case "routing.policy.update", "routing.accounts.create", "routing.reports.create":
		if id != "" {
			return e.failure(requestID, domain.Invalid("no resource ID on this routing operation"))
		}
	case "routing.context.update", "routing.links.update":
		if !domain.ValidContextID(id) {
			return e.failure(requestID, domain.Invalid("fixed native context ID"))
		}
	case "routing.profile.update":
		if !domain.ValidProfileID(id) {
			return e.failure(requestID, domain.Invalid("fixed routing profile ID"))
		}
	default:
		if !domain.ValidID(id) {
			return e.failure(requestID, domain.Invalid("resource identity"))
		}
	}
	facts := domain.RoutingFacts{Assessment: domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown", ReasonCodes: []string{}}, Contexts: []domain.NativeContextFact{}}
	if in, ok := input.(*domain.BudgetAmendmentInput); ok {
		if err := domain.ValidateBudgetAmendment(*in); err != nil {
			return e.failure(requestID, err)
		}
		if replayed, err := e.db.Replay(ctx, in.RequestID, "tasks.budget.amend", store.BudgetAmendmentFingerprint(id, *in)); err != nil {
			return e.failure(in.RequestID, err)
		} else if replayed != nil {
			return *replayed
		}
		if in.Action == "adopt" {
			detail, err := e.db.Task(ctx, id)
			if err != nil {
				return e.failure(in.RequestID, err)
			}
			declared := ""
			if in.Routing != nil {
				declared = in.Routing.Suitability
			}
			facts, err = e.routingFactsForVersion(ctx, detail.Task.ProjectID, detail.Task.OriginalText, detail.Version, declared, "")
			if err != nil {
				return e.failure(in.RequestID, err)
			}
		}
	}
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	e.control.Lock()
	var result domain.Result
	switch in := input.(type) {
	case *domain.RoutingPolicyInput:
		result = e.db.SetRoutingPolicy(ctx, *in)
	case *domain.ContextUpdateInput:
		result = e.db.UpdateRoutingContext(ctx, id, *in)
	case *domain.ProfileUpdateInput:
		result = e.db.UpdateRoutingProfile(ctx, id, *in)
	case *domain.AccountCreateInput:
		result = e.db.CreateRoutingAccount(ctx, *in)
	case *domain.AccountUpdateInput:
		result = e.db.UpdateRoutingAccount(ctx, id, *in)
	case *domain.AccountLinkInput:
		result = e.db.UpdateAccountLink(ctx, id, *in)
	case *domain.CapacityReportInput:
		result = e.db.CreateCapacityReport(ctx, *in)
	case *domain.BudgetAmendmentInput:
		if e.fenced && (in.Action == "adopt" || in.NewWindowSeconds != nil) {
			result = e.failure(in.RequestID, domain.Fail("dispatch_fenced", "New task windows require resolved ownership and persistence. Inspect the retained incident."))
		} else {
			result = e.db.AmendBudgetWithRouting(ctx, id, *in, facts)
		}
	}
	e.guardMutationLocked(result)
	e.control.Unlock()
	e.signal()
	return result
}
