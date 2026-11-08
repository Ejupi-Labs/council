// Package ownerclient connects trusted local interfaces to the continuing owner.
// It never owns the database, starts a scheduler or exposes a network proxy.
package ownerclient

import (
	"net/url"
	"sort"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Route is the complete public metadata for one fixed owner operation. Returned
// slices are copies; callers cannot change the allowlist.
type Route struct {
	Operation string
	Method    string
	Template  string
	PathKinds []string
	QueryKeys []string
	BodyLimit int
}

func route(op, method, path, kinds, query string) Route {
	split := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	}
	limit := 0
	if method != "GET" {
		limit = domain.MaxBodyBytes
	}
	if strings.HasPrefix(op, "history.") && method != "GET" {
		limit = domain.HistoryMaxRequestBytes
	}
	if op == "graphs.start" {
		limit = domain.MaxGraphDeclarationBytes
	}
	return Route{Operation: op, Method: method, Template: path, PathKinds: split(kinds), QueryKeys: split(query), BodyLimit: limit}
}

// The catalog is keyed by owner operation, not an MCP-supplied URL or method.
var routes = []Route{
	route("meta", "GET", "/meta", "", ""),
	route("projects.open", "POST", "/projects/open", "", ""), route("projects.list", "GET", "/projects", "", "limit,cursor"), route("projects.get", "GET", "/projects/:id", "id", ""),
	route("agents.list", "GET", "/agents", "", ""), route("policy.get", "GET", "/policy", "", ""), route("policy.set", "PUT", "/policy", "", ""),
	route("drafts.create", "POST", "/drafts", "", ""), route("drafts.list", "GET", "/drafts", "", "projectId,limit,cursor"), route("drafts.get", "GET", "/drafts/:id", "id", "revision"), route("drafts.update", "PUT", "/drafts/:id", "id", ""),
	route("execution.capabilities", "GET", "/execution/capabilities", "", ""), route("service.stop", "POST", "/service/stop", "", ""),
	route("tasks.start", "POST", "/tasks", "", ""), route("tasks.list", "GET", "/tasks", "", "projectId,limit,cursor"), route("tasks.get", "GET", "/tasks/:id", "id", ""), route("tasks.version.get", "GET", "/tasks/:id/versions/:version", "id,revision", ""),
	route("tasks.guidance", "PUT", "/tasks/:id/guidance", "id", ""), route("tasks.attempts.start", "POST", "/tasks/:id/attempts", "id", ""), route("tasks.attempts.list", "GET", "/tasks/:id/attempts", "id", "limit,cursor"), route("tasks.response.accept", "POST", "/tasks/:id/response-acceptance", "id", ""),
	route("attempts.get", "GET", "/attempts/:id", "id", ""), route("attempts.stop", "POST", "/attempts/:id/stop", "id", ""), route("attempts.wait", "GET", "/attempts/:id/wait", "id", "afterRevision,timeoutMs"), route("attempts.events.list", "GET", "/attempts/:id/events", "id", "limit,cursor"), route("attempts.artifacts.list", "GET", "/attempts/:id/artifacts", "id", "limit,cursor"), route("artifacts.get", "GET", "/artifacts/:id", "id", ""), route("artifacts.bytes", "GET", "/artifacts/:id/bytes", "id", "offset,through,limit"),
	route("routing.policy.get", "GET", "/routing/policy", "", ""), route("routing.policy.update", "PUT", "/routing/policy", "", ""), route("routing.contexts.list", "GET", "/routing/contexts", "", ""), route("routing.context.update", "PUT", "/routing/contexts/:id", "context", ""), route("routing.profiles.list", "GET", "/routing/profiles", "", "contextId"), route("routing.profile.update", "PUT", "/routing/profiles/:id", "profile", ""),
	route("routing.accounts.list", "GET", "/routing/accounts", "", "limit,cursor"), route("routing.accounts.create", "POST", "/routing/accounts", "", ""), route("routing.accounts.update", "PUT", "/routing/accounts/:id", "id", ""), route("routing.links.update", "PUT", "/routing/contexts/:id/account-link", "context", ""), route("routing.capacity.get", "GET", "/routing/capacity", "", "contextId,accountId"),
	route("routing.reports.list", "GET", "/routing/reports", "", "contextId,accountId,limit,cursor"), route("routing.reports.get", "GET", "/routing/reports/:id", "id", ""), route("routing.reports.create", "POST", "/routing/reports", "", ""), route("tasks.routing.get", "GET", "/tasks/:id/routing", "id", ""), route("tasks.routing.decisions.list", "GET", "/tasks/:id/routing/decisions", "id", "limit,cursor"), route("tasks.budget.get", "GET", "/tasks/:id/budget", "id", ""), route("tasks.budget.ledger.list", "GET", "/tasks/:id/budget/ledger", "id", "limit,cursor"), route("tasks.budget.amend", "POST", "/tasks/:id/budget-amendments", "id", ""), route("tasks.usage.list", "GET", "/tasks/:id/usage", "id", "limit,cursor"),
	route("workflow.preferences.get", "GET", "/workflow/preferences", "", ""), route("workflow.preferences.update", "PATCH", "/workflow/preferences", "", ""), route("tasks.workflow.get", "GET", "/tasks/:id/workflow", "id", "workflowId,taskVersion"),
	route("tasks.workflow.steps.list", "GET", "/tasks/:id/workflow/steps", "id", "workflowId,taskVersion,limit,cursor"), route("tasks.workflow.reviews.list", "GET", "/tasks/:id/workflow/reviews", "id", "workflowId,taskVersion,limit,cursor"), route("tasks.workflow.checks.list", "GET", "/tasks/:id/workflow/checks", "id", "workflowId,taskVersion,limit,cursor"), route("tasks.workflow.findings.list", "GET", "/tasks/:id/workflow/findings", "id", "workflowId,taskVersion,limit,cursor"), route("tasks.workflow.deliveries.list", "GET", "/tasks/:id/workflow/deliveries", "id", "workflowId,taskVersion,limit,cursor"), route("tasks.workflow.candidates.list", "GET", "/tasks/:id/workflow/candidates", "id", "workflowId,taskVersion,limit,cursor"),
	route("candidates.changes.list", "GET", "/candidates/:id/changes", "id", "limit,cursor"), route("tasks.workflow.adopt", "POST", "/tasks/:id/workflow/adopt", "id", ""), route("tasks.workflow.advance", "POST", "/tasks/:id/workflow/advance", "id", ""), route("tasks.workflow.check", "POST", "/tasks/:id/workflow/check", "id", ""),
	route("graphs.start", "POST", "/graphs", "", ""), route("graphs.list", "GET", "/graphs", "", "projectId,limit,cursor"), route("graphs.get", "GET", "/graphs/:id", "id", ""), route("graphs.definition.get", "GET", "/graphs/:id/definition", "id", ""), route("graphs.wait", "GET", "/graphs/:id/wait", "id", "afterRevision,timeoutMs"), route("graphs.advance", "POST", "/graphs/:id/advance", "id", ""), route("graphs.stop", "POST", "/graphs/:id/stop", "id", ""), route("graphs.pause", "POST", "/graphs/:id/pause", "id", ""),
	route("graphs.nodes.stop", "POST", "/graphs/:id/nodes/:node/stop", "id,graphkey", ""), route("graphs.nodes.guidance", "POST", "/graphs/:id/nodes/:node/guidance", "id,graphkey", ""), route("graphs.budget.get", "GET", "/graphs/:id/budget", "id", ""), route("graphs.budget.ledger", "GET", "/graphs/:id/budget-ledger", "id", "limit,cursor"), route("graphs.budget.amend", "POST", "/graphs/:id/budget-amendments", "id", ""), route("graphs.inputs.list", "GET", "/graphs/:id/inputs", "id", "limit,cursor"), route("graphs.inputs.get", "GET", "/graphs/:id/inputs/:input", "id,id", ""), route("graphs.inputs.members", "GET", "/graphs/:id/inputs/:input/members", "id,id", "limit,cursor"), route("graphs.dispatches.list", "GET", "/graphs/:id/dispatches", "id", "inputId,nodeKey,limit,cursor"), route("graphs.dispatches.get", "GET", "/graphs/:id/dispatches/:dispatch", "id,id", ""),
	route("tasks.pause", "POST", "/tasks/:id/pause", "id", ""), route("checkpoints.get", "GET", "/checkpoints/:id", "id", ""), route("checkpoints.resume", "POST", "/checkpoints/:id/resume", "id", ""), route("incidents.get", "GET", "/incidents/:id", "id", ""), route("incidents.reconcile", "POST", "/incidents/:id/reconciliations", "id", ""),
	route("history.snapshots.create", "POST", "/history/snapshots", "", ""), route("history.snapshots.get", "GET", "/history/snapshots/:id", "id", ""), route("history.snapshots.records", "GET", "/history/snapshots/:id/records", "id", "kind,limit,cursor"), route("history.exports.preview", "POST", "/history/exports/preview", "", ""), route("history.exports.create", "POST", "/history/exports", "", ""), route("history.operations.get", "GET", "/history/operations/:id", "id", ""), route("history.operations.reconcile", "POST", "/history/operations/:id/reconciliations", "id", ""),
}

func cloneRoute(r Route) Route {
	r.PathKinds = append([]string(nil), r.PathKinds...)
	r.QueryKeys = append([]string(nil), r.QueryKeys...)
	return r
}
func Routes() []Route {
	out := make([]Route, len(routes))
	for i, r := range routes {
		out[i] = cloneRoute(r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Operation < out[j].Operation })
	return out
}
func RouteFor(op string) (Route, bool) {
	for _, r := range routes {
		if r.Operation == op {
			return cloneRoute(r), true
		}
	}
	return Route{}, false
}

func resolveRoute(r Route, ids []string, query map[string]string) (string, *domain.Error) {
	if len(ids) != len(r.PathKinds) {
		return "", domain.Invalid("exact owner route identities")
	}
	parts := strings.Split(r.Template, "/")
	index := 0
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			value := ids[index]
			kind := r.PathKinds[index]
			valid := false
			switch kind {
			case "id":
				valid = domain.ValidID(value)
			case "revision":
				_, err := domain.ParseRevision(value)
				valid = err == nil
			case "graphkey":
				valid = domain.GraphKey(value)
			case "context":
				valid = domain.ValidContextID(value)
			case "profile":
				valid = domain.ValidProfileID(value)
			}
			if !valid {
				return "", domain.Invalid("exact owner route identity")
			}
			parts[i] = url.PathEscape(value)
			index++
		}
	}
	values := url.Values{}
	for key, value := range query {
		found := false
		for _, allowed := range r.QueryKeys {
			if key == allowed {
				found = true
				break
			}
		}
		if !found || value == "" {
			return "", domain.Invalid("documented nonempty query parameters")
		}
		values.Set(key, value)
	}
	path := "/api/v1" + strings.Join(parts, "/")
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path, nil
}
