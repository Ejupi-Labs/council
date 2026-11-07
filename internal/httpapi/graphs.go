package httpapi

import "strings"

// Composite identities preserve the exact graph and path selector together.
// The application validates both components before invoking the controller.
func graphRoute(p, verb string) (op, id, methods string, queries []string) {
	const prefix = "/api/v1/"
	if !strings.HasPrefix(p, prefix) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(p, prefix), "/")
	if len(parts) == 0 || parts[0] != "graphs" {
		return
	}
	if len(parts) == 1 {
		if verb == "POST" {
			return "graphs.start", "", "GET, POST", nil
		}
		return "graphs.list", "", "GET, POST", []string{"projectId", "limit", "cursor"}
	}
	if parts[1] == "" {
		return
	}
	id = parts[1]
	if len(parts) == 2 {
		return "graphs.get", id, "GET", nil
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "definition":
			return "graphs.definition.get", id, "GET", nil
		case "wait":
			return "graphs.wait", id, "GET", []string{"afterRevision", "timeoutMs"}
		case "advance":
			return "graphs.advance", id, "POST", nil
		case "stop":
			return "graphs.stop", id, "POST", nil
		case "pause":
			return "graphs.pause", id, "POST", nil
		case "budget":
			return "graphs.budget.get", id, "GET", nil
		case "budget-ledger":
			return "graphs.budget.ledger", id, "GET", []string{"limit", "cursor"}
		case "budget-amendments":
			return "graphs.budget.amend", id, "POST", nil
		case "inputs":
			return "graphs.inputs.list", id, "GET", []string{"limit", "cursor"}
		case "dispatches":
			return "graphs.dispatches.list", id, "GET", []string{"inputId", "nodeKey", "limit", "cursor"}
		}
	}
	if len(parts) == 4 && parts[3] != "" {
		switch parts[2] {
		case "inputs":
			return "graphs.inputs.get", id + "/" + parts[3], "GET", nil
		case "dispatches":
			return "graphs.dispatches.get", id + "/" + parts[3], "GET", nil
		}
	}
	if len(parts) == 5 && parts[3] != "" {
		if parts[2] == "inputs" && parts[4] == "members" {
			return "graphs.inputs.members", id + "/" + parts[3], "GET", []string{"limit", "cursor"}
		}
		if parts[2] == "nodes" {
			switch parts[4] {
			case "stop":
				return "graphs.nodes.stop", id + "/" + parts[3], "POST", nil
			case "guidance":
				return "graphs.nodes.guidance", id + "/" + parts[3], "POST", nil
			}
		}
	}
	return "", "", "", nil
}
