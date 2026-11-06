package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// WorkflowMutationController belongs to the lifetime-locked execution owner.
// A read-only/bootstrap service cannot authorize workflow stages or effects.
type WorkflowMutationController interface {
	MutateWorkflow(context.Context, string, string, []byte) domain.Result
}

func (s *Service) executeWorkflowMutation(ctx context.Context, op string, p Params) domain.Result {
	controller, ok := s.Execution.(WorkflowMutationController)
	if !ok {
		return domain.Failure(domain.MutationIdentity(p.Body), domain.Fail("capability_unavailable", "Workflow controls require the current persistent Council owner."))
	}
	return controller.MutateWorkflow(ctx, op, p.ID, p.Body)
}
