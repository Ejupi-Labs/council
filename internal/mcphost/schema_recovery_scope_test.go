package mcphost

import (
	"encoding/json"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/google/jsonschema-go/jsonschema"
)

// Every checkpoint scope branch is exercised independently: the owner remains
// authoritative for relationships not expressible as JSON Schema constraints.
func TestRecoveryScopeBranchParity(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	authority := func(node string) map[string]any {
		return map[string]any{"nodeId": node, "taskVersion": "1", "nodeControlRevision": "1"}
	}
	cases := []struct {
		kind      string
		scope     map[string]any
		required  []string
		forbidden map[string]any
	}{
		{"task", map[string]any{"kind": "task", "taskId": id, "taskVersion": "1", "expectedTaskRevision": "1"},
			[]string{"kind", "taskId", "taskVersion", "expectedTaskRevision"},
			map[string]any{"graphId": id, "graphVersion": "1", "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1", "nodeAuthorities": []any{authority(id)}}},
		{"member", map[string]any{"kind": "member", "taskId": id, "taskVersion": "1", "expectedTaskRevision": "1", "graphId": id, "graphVersion": "1", "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1"},
			[]string{"kind", "taskId", "taskVersion", "expectedTaskRevision", "graphId", "graphVersion", "expectedGraphControlRevision", "nodeId", "expectedNodeControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber"},
			map[string]any{"nodeAuthorities": []any{authority(id)}}},
		{"graph", map[string]any{"kind": "graph", "graphId": id, "graphVersion": "1", "expectedGraphControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1", "nodeAuthorities": []any{authority(id)}},
			[]string{"kind", "graphId", "graphVersion", "expectedGraphControlRevision", "expectedBudgetAuthorityEpoch", "expectedWindowNumber", "nodeAuthorities"},
			map[string]any{"taskId": id, "taskVersion": "1", "expectedTaskRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1"}},
	}
	rawSchema, ok := schemas.DraftLookup("council_checkpoints_resume")
	if !ok {
		t.Fatal("missing checkpoint schema")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatal(err)
	}
	compiled, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, scope map[string]any, want bool) {
		t.Helper()
		input := map[string]any{"requestId": id, "expectedCheckpointRevision": "1", "scope": scope}
		args := map[string]any{"id": id, "input": input}
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		ownerErr, known := schemas.ValidateOwnerBody("checkpoints.resume", id, body)
		if !known || (ownerErr == nil) != want {
			t.Fatalf("owner valid=%v; expected %v: %v", ownerErr == nil, want, ownerErr)
		}
		schemaErr := compiled.Validate(args)
		if (schemaErr == nil) != want {
			t.Fatalf("schema valid=%v; expected %v: %v", schemaErr == nil, want, schemaErr)
		}
	}
	copyScope := func(original map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range original {
			out[k] = v
		}
		return out
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			t.Run("complete", func(t *testing.T) { check(t, tc.scope, true) })
			for _, key := range tc.required {
				t.Run("missing_"+key, func(t *testing.T) { scope := copyScope(tc.scope); delete(scope, key); check(t, scope, false) })
			}
			for key, value := range tc.forbidden {
				t.Run("forbidden_"+key, func(t *testing.T) { scope := copyScope(tc.scope); scope[key] = value; check(t, scope, false) })
			}
			t.Run("unknown", func(t *testing.T) {
				scope := copyScope(tc.scope)
				scope["notCouncilAuthority"] = true
				check(t, scope, false)
			})
			for _, key := range tc.required {
				switch key {
				case "kind", "nodeAuthorities":
					continue
				case "taskId", "graphId", "nodeId":
					t.Run("bad_id_"+key, func(t *testing.T) { scope := copyScope(tc.scope); scope[key] = "bad"; check(t, scope, false) })
				default:
					t.Run("max_revision_"+key, func(t *testing.T) {
						scope := copyScope(tc.scope)
						scope[key] = "9223372036854775807"
						check(t, scope, true)
					})
					t.Run("overflow_revision_"+key, func(t *testing.T) {
						scope := copyScope(tc.scope)
						scope[key] = "9223372036854775808"
						check(t, scope, false)
					})
					t.Run("number_revision_"+key, func(t *testing.T) { scope := copyScope(tc.scope); scope[key] = 1; check(t, scope, false) })
				}
			}
			if tc.kind == "graph" {
				t.Run("empty_authorities", func(t *testing.T) {
					scope := copyScope(tc.scope)
					scope["nodeAuthorities"] = []any{}
					check(t, scope, false)
				})
				t.Run("too_many_authorities", func(t *testing.T) {
					scope := copyScope(tc.scope)
					nodes := make([]any, 17)
					for i := range nodes {
						nodes[i] = authority(id)
					}
					scope["nodeAuthorities"] = nodes
					check(t, scope, false)
				})
				t.Run("missing_authority_field", func(t *testing.T) {
					scope := copyScope(tc.scope)
					scope["nodeAuthorities"] = []any{map[string]any{"nodeId": id, "taskVersion": "1"}}
					check(t, scope, false)
				})
				t.Run("null_authority", func(t *testing.T) {
					scope := copyScope(tc.scope)
					scope["nodeAuthorities"] = []any{nil}
					check(t, scope, false)
				})
			}
		})
	}
}
