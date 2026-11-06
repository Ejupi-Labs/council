package httpapi

import "strings"

func workflowRoute(p, verb string) (op, id, methods string, queries []string) {
	const prefix = "/api/v1/"
	if !strings.HasPrefix(p, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(p, prefix), "/")
	if len(parts) == 2 && parts[0] == "workflow" && parts[1] == "preferences" {
		if verb == "PATCH" {
			return "workflow.preferences.update", "", "GET, PATCH", nil
		}
		return "workflow.preferences.get", "", "GET, PATCH", nil
	}
	if len(parts) == 3 && parts[0] == "candidates" && parts[1] != "" && parts[2] == "changes" {
		return "candidates.changes.list", parts[1], "GET", []string{"limit", "cursor"}
	}
	if len(parts) < 3 || parts[0] != "tasks" || parts[1] == "" || parts[2] != "workflow" {
		return
	}
	if len(parts) == 3 {
		return "tasks.workflow.get", parts[1], "GET", []string{"workflowId", "taskVersion"}
	}
	if len(parts) == 4 {
		switch parts[3] {
		case "steps", "reviews", "checks", "findings", "deliveries", "candidates":
			return "tasks.workflow." + parts[3] + ".list", parts[1], "GET", []string{"workflowId", "taskVersion", "limit", "cursor"}
		case "adopt", "advance", "check":
			return "tasks.workflow." + parts[3], parts[1], "POST", nil
		}
	}
	return "", "", "", nil
}
