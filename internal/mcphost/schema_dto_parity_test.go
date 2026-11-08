package mcphost

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
)

// This compares the complete nested field inventory independently of the
// handcrafted schema. Validators, required presence and semantics still need
// separate owner-parity tests; a matching Go shape is not schema approval.
func TestMutationDraftsMatchDomainFieldInventory(t *testing.T) {
	types := map[string]any{
		"council_tasks_start":                 domain.StartTaskInput{},
		"council_tasks_guidance":              domain.GuidanceInput{},
		"council_tasks_attempts_start":        domain.StartAttemptInput{},
		"council_tasks_response_accept":       domain.ResponseAcceptanceInput{},
		"council_routing_policy_update":       domain.RoutingPolicyInput{},
		"council_routing_context_update":      domain.ContextUpdateInput{},
		"council_routing_profile_update":      domain.ProfileUpdateInput{},
		"council_routing_accounts_create":     domain.AccountCreateInput{},
		"council_routing_accounts_update":     domain.AccountUpdateInput{},
		"council_routing_links_update":        domain.AccountLinkInput{},
		"council_routing_reports_create":      domain.CapacityReportInput{},
		"council_tasks_budget_amend":          domain.BudgetAmendmentInput{},
		"council_workflow_preferences_update": domain.DeliveryPreferenceInput{},
		"council_tasks_workflow_adopt":        domain.WorkflowAdoptInput{},
		"council_tasks_workflow_advance":      domain.WorkflowAdvanceInput{},
		"council_tasks_workflow_check":        domain.WorkflowCheckInput{},
		"council_graphs_start":                domain.GraphStartInput{},
		"council_graphs_advance":              domain.GraphAdvanceInput{},
		"council_graphs_stop":                 domain.GraphStopInput{},
		"council_graphs_nodes_stop":           domain.GraphNodeStopInput{},
		"council_graphs_nodes_guidance":       domain.GraphGuidanceInput{},
		"council_graphs_budget_amend":         domain.GraphBudgetAmendmentInput{},
		"council_tasks_pause":                 domain.PauseTaskInput{},
		"council_graphs_pause":                domain.PauseGraphInput{},
		"council_checkpoints_resume":          domain.ResumeCheckpointInput{},
		"council_incidents_reconcile":         domain.ReconciliationInput{},
		"council_history_capture":             domain.CaptureHistoryInput{},
		"council_history_preview":             domain.PreviewHistoryExportInput{},
		"council_history_export":              domain.ExecuteHistoryExportInput{},
		"council_history_reconcile":           domain.ReconcileHistoryOperationInput{},
	}
	for name, prototype := range types {
		t.Run(name, func(t *testing.T) {
			raw, ok := schemas.DraftLookup(name)
			if !ok {
				t.Fatal("missing candidate schema")
			}
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatal("missing outer properties")
			}
			input, ok := properties["input"].(map[string]any)
			if !ok {
				t.Fatal("missing input object")
			}
			checkSchemaFields(t, name, "input", reflect.TypeOf(prototype), input)
		})
	}
}

func checkSchemaFields(t *testing.T, tool, path string, typ reflect.Type, schema map[string]any) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		if schema["type"] != "array" {
			t.Errorf("%s: expected array schema", path)
		}
		nested, ok := schema["items"].(map[string]any)
		if !ok {
			t.Errorf("%s: array item schema missing", path)
			return
		}
		checkSchemaFields(t, tool, path+"[]", typ.Elem(), nested)
		return
	}
	if typ.Kind() != reflect.Struct {
		want := ""
		switch typ.Kind() {
		case reflect.String:
			want = "string"
		case reflect.Bool:
			want = "boolean"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			want = "integer"
		}
		if want != "" {
			if actual, exists := schema["type"]; exists && actual != want {
				t.Errorf("%s: expected %s schema, got %v", path, want, actual)
			} else if !exists {
				// A const constrains a scalar to one value of its actual type.
				value, ok := schema["const"]
				if !ok {
					t.Errorf("%s: missing scalar type or const", path)
				} else if want == "string" {
					if _, ok := value.(string); !ok {
						t.Errorf("%s: const is not a string", path)
					}
				} else if want == "boolean" {
					if _, ok := value.(bool); !ok {
						t.Errorf("%s: const is not a boolean", path)
					}
				}
			}
		}
		return
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		// Discriminated recovery scopes enumerate the shared DTO fields across
		// closed oneOf branches; inspect their union for structural drift.
		branches, hasBranches := schema["oneOf"].([]any)
		if !hasBranches {
			t.Errorf("%s: nested object schema missing", path)
			return
		}
		properties = map[string]any{}
		for _, branch := range branches {
			object, _ := branch.(map[string]any)
			members, _ := object["properties"].(map[string]any)
			for name, value := range members {
				properties[name] = value
			}
		}
	}
	fields := map[string]reflect.Type{}
	var collect func(reflect.Type)
	collect = func(current reflect.Type) {
		for i := 0; i < current.NumField(); i++ {
			field := current.Field(i)
			if !field.IsExported() {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if field.Anonymous && name == "" && field.Type.Kind() == reflect.Struct {
				collect(field.Type)
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
	}
	collect(typ)
	var missing, extra []string
	for name, fieldType := range fields {
		value, exists := properties[name]
		if !exists {
			// Graph node declarations intentionally forbid per-node delivery and
			// budget authority even though the shared DTO contains these fields.
			graphNode := tool == "council_graphs_start" && strings.HasPrefix(path, "input.nodes[].") || tool == "council_graphs_nodes_guidance" && strings.HasPrefix(path, "input.")
			if tool == "council_graphs_start" && (path == "input.routing" || path == "input.nodes[].routing") && name == "useCurrentPolicy" {
				continue
			}
			if graphNode && name == "budget" && typ.Name() == "RoutingOptions" {
				continue
			}
			if graphNode && name == "delivery" && typ.Name() == "WorkflowInput" {
				continue
			}
			missing = append(missing, name)
			continue
		}
		if child, ok := value.(map[string]any); ok {
			checkSchemaFields(t, tool, path+"."+name, fieldType, child)
		}
	}
	for name := range properties {
		if _, exists := fields[name]; !exists {
			extra = append(extra, name)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		sort.Strings(missing)
		sort.Strings(extra)
		t.Errorf("%s: missing fields %v; extra fields %v", path, missing, extra)
	}
}
