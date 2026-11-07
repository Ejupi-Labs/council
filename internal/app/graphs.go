package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Graph mutations are provided by the lifetime-locked owner. Raw requests stay
// intact until its strict typed decoding; controller facts are never user input.
type GraphMutationController interface {
	MutateGraph(context.Context, string, string, []byte) domain.Result
}

func (s *Service) executeGraph(ctx context.Context, op string, p Params) domain.Result {
	id := domain.MutationIdentity(p.Body)
	switch op {
	case "graphs.list", "graphs.get", "graphs.definition.get", "graphs.wait", "graphs.budget.get", "graphs.budget.ledger", "graphs.inputs.list", "graphs.inputs.get", "graphs.inputs.members", "graphs.dispatches.list", "graphs.dispatches.get":
		if len(p.Body) != 0 {
			return domain.Failure(id, domain.Invalid("empty GET body"))
		}
		return s.Execution.Read(ctx, op, p.ID, p.Query)
	case "graphs.start", "graphs.advance", "graphs.stop", "graphs.nodes.stop", "graphs.nodes.guidance", "graphs.budget.amend":
	default:
		return domain.Failure(id, domain.Fail("not_found", "The graph operation does not exist."))
	}
	if invalid := domain.ValidateQuery(p.Query); invalid != nil {
		return domain.Failure(id, invalid)
	}
	if op == "graphs.start" && len(p.Body) > domain.MaxGraphDeclarationBytes {
		return domain.Failure(id, domain.Fail("input_too_large", "The graph declaration exceeds 2 MiB. Reduce it explicitly."))
	}
	controller, ok := s.Execution.(GraphMutationController)
	if !ok {
		return domain.Failure(id, domain.Fail("capability_unavailable", "Graph controls require the current persistent Council owner."))
	}
	return controller.MutateGraph(ctx, op, p.ID, p.Body)
}
