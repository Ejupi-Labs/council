package app

import (
	"context"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// ExecutionController is provided only by a lifetime-locked persistent owner.
type ExecutionController interface {
	StartTask(context.Context, domain.StartTaskInput) domain.Result
	Guidance(context.Context, string, domain.GuidanceInput) domain.Result
	StartAttempt(context.Context, string, domain.StartAttemptInput) domain.Result
	Stop(context.Context, string, string) domain.Result
	AcceptResponse(context.Context, string, domain.ResponseAcceptanceInput) domain.Result
	Shutdown(context.Context, string, string) domain.Result
	Read(context.Context, string, string, map[string]string) domain.Result
	Meta(context.Context) (map[string]any, error)
}

func executionOperation(op string) bool {
	return strings.HasPrefix(op, "workflow.") || strings.HasPrefix(op, "candidates.") || strings.HasPrefix(op, "routing.") || strings.HasPrefix(op, "tasks.") || strings.HasPrefix(op, "attempts.") || strings.HasPrefix(op, "artifacts.") || strings.HasPrefix(op, "execution.") || op == "service.stop"
}

func (s *Service) executeManaged(ctx context.Context, op string, p Params) domain.Result {
	if s.Execution == nil {
		return domain.Failure(domain.MutationIdentity(p.Body), domain.Fail("capability_unavailable", "Managed execution requires a compatible persistent Council owner. Start Council UI or use an execution command."))
	}
	if strings.HasSuffix(op, ".get") || strings.HasSuffix(op, ".list") || op == "attempts.wait" || op == "artifacts.bytes" || op == "execution.capabilities" {
		if len(p.Body) > 0 {
			return domain.Failure(domain.NewID(), domain.Invalid("empty GET body"))
		}
		return s.Execution.Read(ctx, op, p.ID, p.Query)
	}
	id := domain.MutationIdentity(p.Body)
	if e := domain.ValidateQuery(p.Query); e != nil {
		return domain.Failure(id, e)
	}
	if strings.HasPrefix(op, "workflow.") || strings.HasPrefix(op, "tasks.workflow.") {
		return s.executeWorkflowMutation(ctx, op, p)
	}
	if strings.HasPrefix(op, "routing.") || op == "tasks.budget.amend" {
		return s.executeRoutingMutation(ctx, op, p)
	}
	if p.ID != "" && !domain.ValidID(p.ID) {
		return domain.Failure(id, domain.Invalid("resource identity"))
	}
	switch op {
	case "tasks.start":
		var in domain.StartTaskInput
		if e := domain.StrictDecode(p.Body, &in); e != nil {
			return domain.Failure(id, e)
		}
		if e := domain.ValidateStartTask(in); e != nil {
			return domain.Failure(id, e)
		}
		return s.Execution.StartTask(ctx, in)
	case "tasks.guidance":
		var in domain.GuidanceInput
		if e := domain.StrictDecode(p.Body, &in); e != nil {
			return domain.Failure(id, e)
		}
		if e := domain.ValidateGuidance(in); e != nil {
			return domain.Failure(id, e)
		}
		return s.Execution.Guidance(ctx, p.ID, in)
	case "tasks.attempts.start":
		var in domain.StartAttemptInput
		if e := domain.StrictDecode(p.Body, &in); e != nil {
			return domain.Failure(id, e)
		}
		if e := domain.ValidateStartAttempt(in); e != nil {
			return domain.Failure(id, e)
		}
		return s.Execution.StartAttempt(ctx, p.ID, in)
	case "tasks.response.accept":
		var in domain.ResponseAcceptanceInput
		if e := domain.StrictDecode(p.Body, &in); e != nil {
			return domain.Failure(id, e)
		}
		if e := domain.ValidateResponseAcceptance(in); e != nil {
			return domain.Failure(id, e)
		}
		return s.Execution.AcceptResponse(ctx, p.ID, in)
	case "attempts.stop":
		var in struct {
			RequestID string `json:"requestId"`
		}
		if e := domain.StrictDecode(p.Body, &in); e != nil {
			return domain.Failure(id, e)
		}
		if !domain.ValidID(in.RequestID) {
			return domain.Failure(id, domain.Invalid("requestId"))
		}
		return s.Execution.Stop(ctx, p.ID, in.RequestID)
	case "service.stop":
		var in struct {
			RequestID       string `json:"requestId"`
			OwnerInstanceID string `json:"ownerInstanceId"`
		}
		if e := domain.StrictDecode(p.Body, &in); e != nil {
			return domain.Failure(id, e)
		}
		if !domain.ValidID(in.RequestID) || !domain.ValidID(in.OwnerInstanceID) {
			return domain.Failure(id, domain.Invalid("request and owner identities"))
		}
		return s.Execution.Shutdown(ctx, in.OwnerInstanceID, in.RequestID)
	}
	return domain.Failure(id, domain.Fail("not_found", "The managed execution operation does not exist."))
}

func (s *Service) Metadata(ctx context.Context) (map[string]any, error) {
	if s.Execution != nil {
		return s.Execution.Meta(ctx)
	}
	return map[string]any{"scope": "workspace_foundation", "managedExecution": "not_implemented"}, nil
}
