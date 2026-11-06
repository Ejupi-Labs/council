package httpapi

import "strings"

func executionRoute(p, verb string) (op, id, methods string, queries []string) {
	const prefix = "/api/v1/"
	if !strings.HasPrefix(p, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(p, prefix), "/")
	if len(parts) == 1 && parts[0] == "tasks" {
		if verb == "POST" {
			return "tasks.start", "", "GET, POST", nil
		}
		return "tasks.list", "", "GET, POST", []string{"projectId", "limit", "cursor"}
	}
	if len(parts) == 2 {
		if parts[0] == "execution" && parts[1] == "capabilities" {
			return "execution.capabilities", "", "GET", nil
		}
		if parts[0] == "service" && parts[1] == "stop" {
			return "service.stop", "", "POST", nil
		}
		if parts[1] == "" {
			return
		}
		switch parts[0] {
		case "tasks":
			return "tasks.get", parts[1], "GET", nil
		case "attempts":
			return "attempts.get", parts[1], "GET", nil
		case "artifacts":
			return "artifacts.get", parts[1], "GET", nil
		}
	}
	if len(parts) == 3 && parts[1] != "" {
		id = parts[1]
		switch parts[0] + "/" + parts[2] {
		case "tasks/guidance":
			return "tasks.guidance", id, "PUT", nil
		case "tasks/attempts":
			if verb == "POST" {
				return "tasks.attempts.start", id, "GET, POST", nil
			}
			return "tasks.attempts.list", id, "GET, POST", []string{"limit", "cursor"}
		case "tasks/response-acceptance":
			return "tasks.response.accept", id, "POST", nil
		case "attempts/stop":
			return "attempts.stop", id, "POST", nil
		case "attempts/wait":
			return "attempts.wait", id, "GET", []string{"afterRevision", "timeoutMs"}
		case "attempts/events":
			return "attempts.events.list", id, "GET", []string{"limit", "cursor"}
		case "attempts/artifacts":
			return "attempts.artifacts.list", id, "GET", []string{"limit", "cursor"}
		case "artifacts/bytes":
			return "artifacts.bytes", id, "GET", []string{"offset", "through", "limit"}
		}
	}
	if len(parts) == 4 && parts[0] == "tasks" && parts[1] != "" && parts[2] == "versions" && parts[3] != "" {
		return "tasks.version.get", parts[1] + "/" + parts[3], "GET", nil
	}
	return "", "", "", nil
}
