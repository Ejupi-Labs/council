package httpapi

import "strings"

// routingRoute describes only authored API paths. Authentication, methods, body
// validation and duplicate/empty query rejection remain in the common server.
func routingRoute(p, verb string) (op, id, methods string, queries []string) {
	const prefix = "/api/v1/"
	if !strings.HasPrefix(p, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(p, prefix), "/")
	for _, part := range parts {
		if part == "" {
			return "", "", "", nil
		}
	}
	if len(parts) == 2 && parts[0] == "routing" {
		switch parts[1] {
		case "policy":
			if verb == "PUT" {
				return "routing.policy.update", "", "GET, PUT", nil
			}
			return "routing.policy.get", "", "GET, PUT", nil
		case "contexts":
			return "routing.contexts.list", "", "GET", nil
		case "profiles":
			return "routing.profiles.list", "", "GET", []string{"contextId"}
		case "accounts":
			if verb == "POST" {
				return "routing.accounts.create", "", "GET, POST", nil
			}
			return "routing.accounts.list", "", "GET, POST", []string{"limit", "cursor"}
		case "capacity":
			return "routing.capacity.get", "", "GET", []string{"contextId", "accountId"}
		case "reports":
			if verb == "POST" {
				return "routing.reports.create", "", "GET, POST", nil
			}
			return "routing.reports.list", "", "GET, POST", []string{"contextId", "accountId", "limit", "cursor"}
		}
	}
	if len(parts) == 3 && parts[0] == "routing" {
		switch parts[1] {
		case "contexts":
			return "routing.context.update", parts[2], "PUT", nil
		case "profiles":
			return "routing.profile.update", parts[2], "PUT", nil
		case "accounts":
			return "routing.accounts.update", parts[2], "PUT", nil
		case "reports":
			return "routing.reports.get", parts[2], "GET", nil
		}
	}
	if len(parts) == 4 && parts[0] == "routing" && parts[1] == "contexts" && parts[3] == "account-link" {
		return "routing.links.update", parts[2], "PUT", nil
	}
	if len(parts) == 3 && parts[0] == "tasks" {
		switch parts[2] {
		case "routing":
			return "tasks.routing.get", parts[1], "GET", nil
		case "budget":
			return "tasks.budget.get", parts[1], "GET", nil
		case "usage":
			return "tasks.usage.list", parts[1], "GET", []string{"limit", "cursor"}
		case "budget-amendments":
			return "tasks.budget.amend", parts[1], "POST", nil
		}
	}
	if len(parts) == 4 && parts[0] == "tasks" {
		switch parts[2] + "/" + parts[3] {
		case "routing/decisions":
			return "tasks.routing.decisions.list", parts[1], "GET", []string{"limit", "cursor"}
		case "budget/ledger":
			return "tasks.budget.ledger.list", parts[1], "GET", []string{"limit", "cursor"}
		}
	}
	return "", "", "", nil
}
