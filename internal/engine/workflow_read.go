package engine

import (
	"context"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func workflowReadOperation(op string) bool {
	switch op {
	case "workflow.preferences.get", "tasks.workflow.get", "tasks.workflow.steps.list",
		"tasks.workflow.reviews.list", "tasks.workflow.checks.list", "tasks.workflow.findings.list",
		"tasks.workflow.deliveries.list", "tasks.workflow.candidates.list", "candidates.changes.list":
		return true
	}
	return false
}

func (e *Engine) readWorkflow(ctx context.Context, op, id string, q map[string]string) domain.Result {
	requestID := domain.NewID()
	fail := func(err error) domain.Result { return e.failure(requestID, err) }
	keys := []string{}
	if op == "tasks.workflow.get" {
		keys = []string{"workflowId", "taskVersion"}
	} else if strings.HasPrefix(op, "tasks.workflow.") {
		keys = []string{"workflowId", "taskVersion", "limit", "cursor"}
	} else if op == "candidates.changes.list" {
		keys = []string{"limit", "cursor"}
	}
	if validationErr := domain.ValidateQuery(q, keys...); validationErr != nil {
		return fail(validationErr)
	}
	for key, value := range q {
		if value == "" {
			if key == "cursor" {
				return fail(domain.Fail("invalid_cursor", "Supply the complete cursor or omit it to start the workflow list."))
			}
			return fail(domain.Invalid("nonempty workflow query values"))
		}
	}
	if op == "workflow.preferences.get" {
		if id != "" {
			return fail(domain.Invalid("no resource identity on delivery preference"))
		}
	} else if !domain.ValidID(id) {
		return fail(domain.Invalid("workflow resource identity"))
	}
	selector := domain.WorkflowSelector{WorkflowID: q["workflowId"], TaskVersion: q["taskVersion"]}
	if validationErr := domain.ValidateWorkflowSelector(selector); validationErr != nil {
		return fail(validationErr)
	}
	limit := 30
	if value, present := q["limit"]; present {
		parsed, validationErr := domain.ParseLimit(value)
		if validationErr != nil {
			return fail(validationErr)
		}
		limit = parsed
	}
	var data any
	var readErr error
	switch op {
	case "workflow.preferences.get":
		data, readErr = e.db.DeliveryPreference(ctx)
	case "tasks.workflow.get":
		data, readErr = e.db.Workflow(ctx, id, selector)
	case "tasks.workflow.steps.list":
		data, readErr = e.db.WorkflowSteps(ctx, id, selector, limit, q["cursor"])
	case "tasks.workflow.reviews.list":
		data, readErr = e.db.WorkflowReviews(ctx, id, selector, limit, q["cursor"])
	case "tasks.workflow.checks.list":
		data, readErr = e.db.WorkflowChecks(ctx, id, selector, limit, q["cursor"])
	case "tasks.workflow.findings.list":
		data, readErr = e.db.WorkflowFindings(ctx, id, selector, limit, q["cursor"])
	case "tasks.workflow.deliveries.list":
		data, readErr = e.db.WorkflowDeliveries(ctx, id, selector, limit, q["cursor"])
	case "tasks.workflow.candidates.list":
		data, readErr = e.db.WorkflowCandidates(ctx, id, selector, limit, q["cursor"])
	case "candidates.changes.list":
		data, readErr = e.candidateChanges(ctx, id, limit, q["cursor"])
	default:
		readErr = domain.Fail("not_found", "This workflow read does not exist.")
	}
	if readErr != nil {
		return fail(readErr)
	}
	return domain.Success(200, requestID, data, nil)
}
