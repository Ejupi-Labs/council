package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// RoutingMutationController shares the authoritative owner's serialized mutation
// path. A foundation-only service cannot modify live scheduling controls.
type RoutingMutationController interface {
	MutateRouting(context.Context, string, string, []byte) domain.Result
}

func (s *Service) executeRoutingMutation(ctx context.Context, op string, p Params) domain.Result {
	controller, ok := s.Execution.(RoutingMutationController)
	if !ok {
		return domain.Failure(domain.MutationIdentity(p.Body), domain.Fail("capability_unavailable", "Routing controls require the current persistent Council owner."))
	}
	return controller.MutateRouting(ctx, op, p.ID, p.Body)
}
