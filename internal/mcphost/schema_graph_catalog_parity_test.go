package mcphost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/google/jsonschema-go/jsonschema"
)

// Each case is independently tied to the real owner validator. A matching DTO
// field inventory alone does not establish required-field or authority parity.
func TestGraphRecoveryHistoryMutationCatalogParity(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	auth := []any{map[string]any{"nodeId": id, "taskVersion": "1", "nodeControlRevision": "1"}}
	graphPath := map[string]any{"graphId": id}
	memberPath := map[string]any{"graphId": id, "nodeKey": "worker"}
	graphAuthority := map[string]any{"requestId": id, "graphVersion": "1", "expectedGraphControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1", "nodeAuthorities": auth}
	taskScope := map[string]any{"kind": "task", "taskId": id, "taskVersion": "1", "expectedTaskRevision": "1"}
	cases := []struct {
		tool, operation string
		path, body      map[string]any
		required        []string
	}{
		{"council_graphs_start", "graphs.start", nil, map[string]any{"requestId": id, "projectId": id, "nodes": []any{map[string]any{"key": "worker", "originalText": "Work"}}}, []string{"requestId", "projectId", "nodes"}},
		{"council_graphs_nodes_guidance", "graphs.nodes.guidance", memberPath, map[string]any{"requestId": id, "expectedRevision": "1", "guidanceText": "", "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "taskVersion": "1"}, []string{"requestId", "expectedRevision", "guidanceText", "expectedGraphControlRevision", "nodeId", "expectedNodeControlRevision", "taskVersion"}},
		{"council_graphs_advance", "graphs.advance", graphPath, graphAuthority, []string{"requestId", "graphVersion", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber", "nodeAuthorities"}},
		{"council_graphs_stop", "graphs.stop", graphPath, map[string]any{"requestId": id, "expectedGraphControlRevision": "1"}, []string{"requestId", "expectedGraphControlRevision"}},
		{"council_graphs_nodes_stop", "graphs.nodes.stop", memberPath, map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1"}, []string{"requestId", "expectedGraphControlRevision", "nodeId", "expectedNodeControlRevision"}},
		{"council_graphs_budget_amend", "graphs.budget.amend", graphPath, map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1", "additionalAdmissions": "1"}, []string{"requestId", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber", "additionalAdmissions"}},
		{"council_graphs_pause", "graphs.pause", graphPath, graphAuthority, []string{"requestId", "graphVersion", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber", "nodeAuthorities"}},
		{"council_checkpoints_resume", "checkpoints.resume", map[string]any{"id": id}, map[string]any{"requestId": id, "expectedCheckpointRevision": "1", "scope": taskScope}, []string{"requestId", "expectedCheckpointRevision", "scope"}},
		{"council_tasks_pause", "tasks.pause", map[string]any{"taskId": id}, map[string]any{"requestId": id, "taskVersion": "1", "expectedTaskRevision": "1"}, []string{"requestId", "taskVersion", "expectedTaskRevision"}},
		{"council_incidents_reconcile", "incidents.reconcile", map[string]any{"id": id}, map[string]any{"requestId": id, "expectedIncidentRevision": "1", "action": "verify_and_reconcile"}, []string{"requestId", "expectedIncidentRevision", "action"}},
		{"council_history_capture", "history.snapshots.create", nil, map[string]any{"requestId": id, "projectId": id}, []string{"requestId", "projectId"}},
		{"council_history_preview", "history.exports.preview", nil, map[string]any{"requestId": id, "snapshotId": id, "snapshotRevision": "1", "filename": "history-example.json"}, []string{"requestId", "snapshotId", "snapshotRevision", "filename"}},
		{"council_history_export", "history.exports.create", nil, map[string]any{"requestId": id, "previewId": id, "previewRevision": "1"}, []string{"requestId", "previewId", "previewRevision"}},
		{"council_history_reconcile", "history.operations.reconcile", map[string]any{"operationId": id}, map[string]any{"requestId": id, "expectedRevision": "1", "action": "verify_no_publication"}, []string{"requestId", "expectedRevision", "action"}},
	}
	copyBody := func(body map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range body {
			out[k] = v
		}
		return out
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			raw, ok := schemas.DraftLookup(tc.tool)
			if !ok {
				t.Fatal("missing draft schema")
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			check := func(t *testing.T, body map[string]any, want bool) {
				t.Helper()
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				ownerErr, known := schemas.ValidateOwnerBody(tc.operation, id, encoded)
				if !known || (ownerErr == nil) != want {
					t.Fatalf("owner valid=%v want=%v: %v", ownerErr == nil, want, ownerErr)
				}
				args := map[string]any{"input": body}
				for k, v := range tc.path {
					args[k] = v
				}
				schemaErr := resolved.Validate(args)
				if (schemaErr == nil) != want {
					t.Fatalf("schema valid=%v want=%v: %v", schemaErr == nil, want, schemaErr)
				}
			}
			t.Run("valid", func(t *testing.T) { check(t, tc.body, true) })
			t.Run("unknown", func(t *testing.T) {
				body := copyBody(tc.body)
				body["unexpectedAuthority"] = true
				check(t, body, false)
			})
			for _, field := range tc.required {
				t.Run("missing_"+field, func(t *testing.T) { body := copyBody(tc.body); delete(body, field); check(t, body, false) })
				t.Run("null_"+field, func(t *testing.T) { body := copyBody(tc.body); body[field] = nil; check(t, body, false) })
				if strings.Contains(field, "Revision") || field == "graphVersion" || field == "taskVersion" || field == "expectedBudgetAuthorityEpoch" || field == "expectedWindowNumber" {
					t.Run("overflow_"+field, func(t *testing.T) {
						body := copyBody(tc.body)
						body[field] = "9223372036854775808"
						check(t, body, false)
					})
					t.Run("wrong_type_"+field, func(t *testing.T) { body := copyBody(tc.body); body[field] = 1; check(t, body, false) })
				}
			}
		})
	}
}
