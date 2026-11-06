package engine

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (e *Engine) guidanceWorkflowFacts(ctx context.Context, taskID string, in domain.GuidanceInput, routing domain.RoutingFacts) (domain.WorkflowAdmissionFacts, error) {
	plain := domain.WorkflowAdmissionFacts{Routing: routing}
	view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
	if err != nil {
		return plain, err
	}
	if view.Workflow == nil {
		if in.Workflow != nil && in.Workflow.Mode != "author_only" {
			return plain, domain.Fail("authority_required", "Use explicit workflow adoption to enable Auto for this standalone task.")
		}
		return plain, nil
	}
	if in.Workflow != nil && in.Workflow.Mode == "author_only" {
		return plain, nil
	}
	detail, err := e.db.Task(ctx, taskID)
	if err != nil {
		return plain, err
	}
	version := detail.Version
	version.GuidanceText = in.GuidanceText
	if in.Acceptance != nil {
		acceptance, invalid := domain.ResolveAcceptance(in.Acceptance)
		if invalid != nil {
			return plain, invalid
		}
		version.Acceptance = acceptance
	}
	options := view.Workflow.Authority.OptionSnapshot
	if in.Workflow != nil {
		options = *in.Workflow
	}
	return e.captureWorkflow(ctx, detail.Task.ProjectID, detail.Task.OriginalText, version, &options, view.Workflow.Authority.ExecutionMode, view.Workflow.Authority.FakeScenario, routing)
}
