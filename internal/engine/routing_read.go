package engine

import (
	"context"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingReadOperation(op string) bool {
	switch op {
	case "routing.policy.get", "routing.contexts.list", "routing.profiles.list",
		"routing.accounts.list", "routing.capacity.get", "routing.reports.list", "routing.reports.get",
		"tasks.routing.get", "tasks.routing.decisions.list", "tasks.budget.get",
		"tasks.budget.ledger.list", "tasks.usage.list":
		return true
	}
	return false
}

// readRouting exposes detached store observations. A read never acquires routing
// authority, renews a report or consumes a managed admission unit.
func (e *Engine) readRouting(ctx context.Context, op, id string, q map[string]string) domain.Result {
	requestID := domain.NewID()
	fail := func(err error) domain.Result { return e.failure(requestID, err) }
	if !routingReadOperation(op) {
		return fail(domain.Fail("not_found", "The requested routing read does not exist."))
	}
	var keys []string
	switch op {
	case "routing.profiles.list":
		keys = []string{"contextId"}
	case "routing.capacity.get":
		keys = []string{"contextId", "accountId"}
	case "routing.reports.list":
		keys = []string{"contextId", "accountId", "limit", "cursor"}
	case "routing.accounts.list", "tasks.routing.decisions.list", "tasks.budget.ledger.list", "tasks.usage.list":
		keys = []string{"limit", "cursor"}
	}
	if err := domain.ValidateQuery(q, keys...); err != nil {
		return fail(err)
	}
	for key, value := range q {
		if value == "" {
			if key == "cursor" {
				return fail(domain.Fail("invalid_cursor", "Supply the complete cursor or omit it to start a routing list."))
			}
			return fail(domain.Invalid("nonempty routing query values"))
		}
	}
	if strings.HasPrefix(op, "tasks.") || op == "routing.reports.get" {
		if !domain.ValidID(id) {
			return fail(domain.Invalid("routing resource identity"))
		}
	} else if id != "" {
		return fail(domain.Invalid("a routing read without a resource identity"))
	}
	// Routing history uses thirty by default; the foundation parser's omitted
	// value has a different default, so invoke it only for an explicit limit.
	limit := 30
	if value, present := q["limit"]; present {
		parsed, err := domain.ParseLimit(value)
		if err != nil {
			return fail(err)
		}
		limit = parsed
	}
	var subject domain.CapacitySubject
	if op == "routing.capacity.get" || op == "routing.reports.list" {
		contextID, hasContext := q["contextId"]
		accountID, hasAccount := q["accountId"]
		if hasContext == hasAccount {
			return fail(domain.Invalid("exactly one contextId or accountId"))
		}
		subject = domain.CapacitySubject{Kind: "context", ID: contextID}
		if hasAccount {
			subject = domain.CapacitySubject{Kind: "account", ID: accountID}
		}
		if err := domain.ValidateCapacitySubject(subject); err != nil {
			return fail(err)
		}
	}
	var data any
	var err error
	switch op {
	case "routing.policy.get":
		data, err = e.db.RoutingPolicy(ctx)
	case "routing.contexts.list":
		data, err = e.db.RoutingContexts(ctx)
	case "routing.profiles.list":
		data, err = e.db.RoutingProfiles(ctx, q["contextId"])
	case "routing.accounts.list":
		data, err = e.db.RoutingAccounts(ctx, limit, q["cursor"])
	case "routing.capacity.get":
		data, err = e.db.Capacity(ctx, subject)
	case "routing.reports.list":
		data, err = e.db.CapacityReports(ctx, subject, limit, q["cursor"])
	case "routing.reports.get":
		data, err = e.db.CapacityReport(ctx, id)
	case "tasks.routing.get":
		data, err = e.db.TaskRouting(ctx, id)
	case "tasks.routing.decisions.list":
		data, err = e.db.RoutingDecisions(ctx, id, limit, q["cursor"])
	case "tasks.budget.get":
		data, err = e.db.TaskBudget(ctx, id)
	case "tasks.budget.ledger.list":
		data, err = e.db.BudgetLedger(ctx, id, limit, q["cursor"])
	case "tasks.usage.list":
		data, err = e.db.Usage(ctx, id, limit, q["cursor"])
	}
	if err != nil {
		return fail(err)
	}
	return domain.Success(200, requestID, data, nil)
}
