package mcphost

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

// toolPathArguments is the reviewed outer MCP path-key sequence, in exact
// ownerclient.Route.PathKinds order. It contains every catalog tool explicitly.
var toolPathArguments = map[string][]string{
	"council_owner_meta":                  {},
	"council_projects_open":               {},
	"council_projects_list":               {},
	"council_projects_get":                {"id"},
	"council_policy_get":                  {},
	"council_policy_set":                  {},
	"council_agents_list":                 {},
	"council_drafts_create":               {},
	"council_drafts_list":                 {},
	"council_drafts_get":                  {"id"},
	"council_drafts_update":               {"id"},
	"council_service_stop":                {},
	"council_execution_capabilities":      {},
	"council_tasks_start":                 {},
	"council_tasks_list":                  {},
	"council_tasks_get":                   {"taskId"},
	"council_tasks_version_get":           {"taskId", "version"},
	"council_tasks_guidance":              {"taskId"},
	"council_tasks_attempts_start":        {"taskId"},
	"council_tasks_attempts_list":         {"taskId"},
	"council_tasks_response_accept":       {"taskId"},
	"council_attempts_get":                {"id"},
	"council_attempts_stop":               {"id"},
	"council_attempts_wait":               {"id"},
	"council_attempts_events":             {"id"},
	"council_attempts_artifacts":          {"id"},
	"council_artifacts_get":               {"id"},
	"council_artifacts_bytes":             {"id"},
	"council_routing_policy_get":          {},
	"council_routing_policy_update":       {},
	"council_routing_contexts_list":       {},
	"council_routing_context_update":      {"id"},
	"council_routing_profiles_list":       {},
	"council_routing_profile_update":      {"id"},
	"council_routing_accounts_list":       {},
	"council_routing_accounts_create":     {},
	"council_routing_accounts_update":     {"id"},
	"council_routing_links_update":        {"id"},
	"council_routing_capacity_get":        {},
	"council_routing_reports_list":        {},
	"council_routing_reports_get":         {"id"},
	"council_routing_reports_create":      {},
	"council_tasks_routing_get":           {"taskId"},
	"council_tasks_routing_decisions":     {"taskId"},
	"council_tasks_budget_get":            {"taskId"},
	"council_tasks_budget_ledger":         {"taskId"},
	"council_tasks_budget_amend":          {"taskId"},
	"council_tasks_usage":                 {"taskId"},
	"council_workflow_preferences_get":    {},
	"council_workflow_preferences_update": {},
	"council_tasks_workflow_get":          {"taskId"},
	"council_tasks_workflow_steps":        {"taskId"},
	"council_tasks_workflow_reviews":      {"taskId"},
	"council_tasks_workflow_checks":       {"taskId"},
	"council_tasks_workflow_findings":     {"taskId"},
	"council_tasks_workflow_deliveries":   {"taskId"},
	"council_tasks_workflow_candidates":   {"taskId"},
	"council_candidates_changes":          {"id"},
	"council_tasks_workflow_adopt":        {"taskId"},
	"council_tasks_workflow_advance":      {"taskId"},
	"council_tasks_workflow_check":        {"taskId"},
	"council_graphs_start":                {},
	"council_graphs_list":                 {},
	"council_graphs_get":                  {"graphId"},
	"council_graphs_definition":           {"graphId"},
	"council_graphs_wait":                 {"graphId"},
	"council_graphs_advance":              {"graphId"},
	"council_graphs_stop":                 {"graphId"},
	"council_graphs_nodes_stop":           {"graphId", "nodeKey"},
	"council_graphs_nodes_guidance":       {"graphId", "nodeKey"},
	"council_graphs_budget_get":           {"graphId"},
	"council_graphs_budget_ledger":        {"graphId"},
	"council_graphs_budget_amend":         {"graphId"},
	"council_graphs_inputs_list":          {"graphId"},
	"council_graphs_inputs_get":           {"graphId", "inputId"},
	"council_graphs_inputs_members":       {"graphId", "inputId"},
	"council_graphs_dispatches_list":      {"graphId"},
	"council_graphs_dispatches_get":       {"graphId", "dispatchId"},
	"council_tasks_pause":                 {"taskId"},
	"council_graphs_pause":                {"graphId"},
	"council_checkpoints_get":             {"id"},
	"council_checkpoints_resume":          {"id"},
	"council_incidents_get":               {"id"},
	"council_incidents_reconcile":         {"id"},
	"council_history_capture":             {},
	"council_history_snapshot_get":        {"snapshotId"},
	"council_history_records":             {"snapshotId"},
	"council_history_preview":             {},
	"council_history_export":              {},
	"council_history_operation_get":       {"operationId"},
	"council_history_reconcile":           {"operationId"},
}

// OwnerRequest converts one already selected fixed tool into an owner request.
// Input JSON is retained byte-for-byte; this function never attaches to an
// owner. The dispatcher must additionally validate the approved schema and
// owner semantic validators before calling the owner client.
func OwnerRequest(mapping ToolMapping, arguments []byte) (ownerclient.Request, *domain.Error) {
	req := ownerclient.Request{Operation: mapping.Operation}
	pathKeys, known := toolPathArguments[mapping.Name]
	if !known {
		return req, domain.Invalid("known Council tool")
	}
	found := false
	for _, item := range ToolCatalog() {
		if item == mapping {
			found = true
			break
		}
	}
	if !found {
		return req, domain.Invalid("matching Council tool and operation")
	}
	route, ok := ownerclient.RouteFor(mapping.Operation)
	if !ok || len(pathKeys) != len(route.PathKinds) {
		return req, domain.Invalid("fixed owner route")
	}
	if len(arguments) == 0 {
		// Omission has the same meaning as {} only for a read whose own
		// stateless owner rules permit all arguments to be absent.
		if route.Method != "GET" || len(pathKeys) != 0 {
			return req, domain.Invalid("required tool arguments")
		}
		if invalid, reviewed := schemas.ValidateOwnerQuery(mapping.Operation, nil, nil); !reviewed || invalid != nil {
			return req, domain.Invalid("required tool arguments")
		}
		arguments = []byte("{}")
	}
	if len(arguments) > MaxFrameBytes {
		return req, domain.Fail("input_too_large", "Tool arguments exceed the protocol limit.")
	}
	dec := jsontext.NewDecoder(bytes.NewReader(arguments))
	value, err := dec.ReadValue()
	if err != nil || value.Kind() != '{' {
		return req, domain.Invalid("tool argument object")
	}
	// Decoder values alias its buffer and are invalidated by the next read.
	value = append(jsontext.Value(nil), value...)
	if _, err = dec.ReadValue(); err != io.EOF {
		return req, domain.Invalid("single tool argument object")
	}
	var members map[string]jsontext.Value
	if err := json.Unmarshal(value, &members); err != nil {
		return req, domain.Invalid("unique tool argument members")
	}
	allowed := make(map[string]bool, len(pathKeys)+len(route.QueryKeys)+1)
	for _, key := range pathKeys {
		allowed[key] = true
	}
	for _, key := range route.QueryKeys {
		allowed[key] = true
	}
	if route.Method != "GET" {
		allowed["input"] = true
	}
	for key := range members {
		if !allowed[key] {
			return req, domain.Invalid("documented tool argument members")
		}
	}
	stringValue := func(key string, required bool) (string, *domain.Error) {
		raw, ok := members[key]
		if !ok {
			if required {
				return "", domain.Invalid("required tool argument")
			}
			return "", nil
		}
		if raw.Kind() != '"' {
			return "", domain.Invalid("string tool argument")
		}
		var result string
		if err := json.Unmarshal(raw, &result); err != nil || result == "" {
			return "", domain.Invalid("nonempty string tool argument")
		}
		return result, nil
	}
	for _, key := range pathKeys {
		id, invalid := stringValue(key, true)
		if invalid != nil {
			return req, invalid
		}
		req.PathIDs = append(req.PathIDs, id)
	}
	req.Query = map[string]string{}
	for _, key := range route.QueryKeys {
		if _, present := members[key]; !present {
			continue
		}
		v, invalid := stringValue(key, false)
		if invalid != nil {
			return req, invalid
		}
		req.Query[key] = v
	}
	if route.Method != "GET" {
		raw, ok := members["input"]
		if !ok || raw.Kind() != '{' {
			return req, domain.Invalid("required input object")
		}
		var body map[string]jsontext.Value
		if err := json.Unmarshal(raw, &body); err != nil {
			return req, domain.Invalid("unique input object members")
		}
		id, ok := body["requestId"]
		if !ok || id.Kind() != '"' || json.Unmarshal(id, &req.RequestID) != nil || !domain.ValidID(req.RequestID) {
			return req, domain.Invalid("durable request identity")
		}
		req.Body = append([]byte(nil), raw...)
	}
	return req, nil
}
