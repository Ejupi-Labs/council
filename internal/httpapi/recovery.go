package httpapi

import "strings"

// Recovery paths select one exact retained identity; the owner validates its scope.
func recoveryRoute(path string) (op, id, methods string, queries []string) {
	const prefix = "/api/v1/"
	if !strings.HasPrefix(path, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) < 2 || parts[1] == "" {
		return
	}
	id = parts[1]
	if len(parts) == 2 {
		switch parts[0] {
		case "checkpoints":
			return "checkpoints.get", id, "GET", nil
		case "incidents":
			return "incidents.get", id, "GET", nil
		}
	}
	if len(parts) == 3 {
		switch parts[0] + "/" + parts[2] {
		case "tasks/pause":
			return "tasks.pause", id, "POST", nil
		case "checkpoints/resume":
			return "checkpoints.resume", id, "POST", nil
		case "incidents/reconciliations":
			return "incidents.reconcile", id, "POST", nil
		}
	}
	return "", "", "", nil
}
