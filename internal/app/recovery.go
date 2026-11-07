package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// RecoveryController is implemented by the lifetime-locked execution owner.
// The caller supplies captured intent, never controller or process proof.
type RecoveryController interface {
	PauseTask(context.Context, string, domain.PauseTaskInput) domain.Result
	PauseGraph(context.Context, string, domain.PauseGraphInput) domain.Result
	ResumeCheckpoint(context.Context, string, domain.ResumeCheckpointInput) domain.Result
	ReconcileIncident(context.Context, string, domain.ReconciliationInput) domain.Result
}

func recoveryOperation(op string) bool {
	switch op {
	case "tasks.pause", "graphs.pause", "checkpoints.get", "checkpoints.resume", "incidents.get", "incidents.reconcile":
		return true
	default:
		return false
	}
}

func (s *Service) executeRecovery(ctx context.Context, op string, p Params) domain.Result {
	id := domain.MutationIdentity(p.Body)
	if !domain.ValidID(p.ID) {
		return domain.Failure(id, domain.Invalid("resource identity"))
	}
	if invalid := domain.ValidateQuery(p.Query); invalid != nil {
		return domain.Failure(id, invalid)
	}
	if op == "checkpoints.get" || op == "incidents.get" {
		if len(p.Body) != 0 {
			return domain.Failure(id, domain.Invalid("empty GET body"))
		}
		return s.Execution.Read(ctx, op, p.ID, p.Query)
	}
	controller, ok := s.Execution.(RecoveryController)
	if !ok {
		return domain.Failure(id, domain.Fail("capability_unavailable", "Recovery controls require the current persistent Council owner."))
	}
	switch op {
	case "tasks.pause":
		var in domain.PauseTaskInput
		if invalid := domain.StrictDecode(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidatePauseTaskInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.PauseTask(ctx, p.ID, in)
	case "graphs.pause":
		var in domain.PauseGraphInput
		if invalid := domain.StrictDecode(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidatePauseGraphInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.PauseGraph(ctx, p.ID, in)
	case "checkpoints.resume":
		var in domain.ResumeCheckpointInput
		if invalid := domain.StrictDecode(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidateResumeCheckpointInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.ResumeCheckpoint(ctx, p.ID, in)
	case "incidents.reconcile":
		var in domain.ReconciliationInput
		if invalid := domain.StrictDecode(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidateReconciliationInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.ReconcileIncident(ctx, p.ID, in)
	}
	return domain.Failure(id, domain.Fail("not_found", "The recovery operation does not exist."))
}
