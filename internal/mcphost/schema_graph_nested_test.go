package mcphost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/google/jsonschema-go/jsonschema"
)

func graphNestedSchema(t *testing.T, tool string) *jsonschema.Resolved {
	t.Helper()
	raw, ok := schemas.DraftLookup(tool)
	if !ok {
		t.Fatalf("missing schema %q", tool)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Shape validation precedes forwarding; the owner still checks canonical
// encoding, contents and binding to the particular graph list and filter.
func TestGraphListCursorSchemaBounds(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		tool string
		path map[string]any
	}{
		{"council_graphs_list", nil},
		{"council_graphs_budget_ledger", map[string]any{"graphId": id}},
		{"council_graphs_inputs_list", map[string]any{"graphId": id}},
		{"council_graphs_inputs_members", map[string]any{"graphId": id, "inputId": id}},
		{"council_graphs_dispatches_list", map[string]any{"graphId": id}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			schema := graphNestedSchema(t, tc.tool)
			for _, c := range []struct {
				value string
				valid bool
			}{
				{"", false}, {"aZ09_-", true}, {strings.Repeat("a", 2048), true},
				{strings.Repeat("a", 2049), false}, {"a=b", false}, {"a!b", false}, {"é", false},
			} {
				args := map[string]any{"cursor": c.value}
				for k, v := range tc.path {
					args[k] = v
				}
				if err := schema.Validate(args); (err == nil) != c.valid {
					t.Errorf("cursor length %d: error %v; expected valid=%v", len(c.value), err, c.valid)
				}
			}
		})
	}
}

func TestGraphModelSelectorCharacterAndByteBounds(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, kind := range []string{"start", "guidance"} {
		for _, tc := range []struct {
			name, model             string
			schemaValid, ownerValid bool
		}{
			{"ascii_limit", strings.Repeat("a", 256), true, true},
			{"ascii_over", strings.Repeat("a", 257), false, false},
			{"unicode_byte_limit", strings.Repeat("é", 128), true, true},
			{"unicode_byte_over", strings.Repeat("é", 129), true, false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				var tool, operation string
				var input, args map[string]any
				pins := map[string]any{"model": tc.model}
				if kind == "start" {
					tool, operation = "council_graphs_start", "graphs.start"
					input = map[string]any{"requestId": id, "projectId": id, "nodes": []any{map[string]any{"key": "worker", "originalText": "Work", "pins": pins}}}
					args = map[string]any{"input": input}
				} else {
					tool, operation = "council_graphs_nodes_guidance", "graphs.nodes.guidance"
					input = map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "taskVersion": "1", "expectedRevision": "1", "guidanceText": "", "pins": pins}
					args = map[string]any{"graphId": id, "nodeKey": "worker", "input": input}
				}
				body, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				ownerErr, known := schemas.ValidateOwnerBody(operation, id, body)
				if !known || (ownerErr == nil) != tc.ownerValid {
					t.Fatalf("owner error %v", ownerErr)
				}
				if schemaErr := graphNestedSchema(t, tool).Validate(args); (schemaErr == nil) != tc.schemaValid {
					t.Fatalf("schema error %v", schemaErr)
				}
			})
		}
	}
}

func TestGraphFakeScenarioEnumMatchesOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		scenario string
		valid    bool
	}{
		{"candidate", true}, {"graph-join-no-edit", true}, {"malformed", true}, {"not-a-scenario", false}, {"", false},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			input := map[string]any{"requestId": id, "projectId": id, "executionMode": "fake", "nodes": []any{map[string]any{"key": "worker", "originalText": "Work", "fakeScenario": tc.scenario}}}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody("graphs.start", id, body)
			if !known || (ownerErr == nil) != tc.valid {
				t.Fatalf("owner error %v", ownerErr)
			}
			if schemaErr := graphNestedSchema(t, "council_graphs_start").Validate(map[string]any{"input": input}); (schemaErr == nil) != tc.valid {
				t.Fatalf("schema error %v", schemaErr)
			}
		})
	}
}

func TestGraphFakeScenarioRequiresFakeExecutionMode(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name, mode string
		valid      bool
	}{
		{"fake", "fake", true},
		{"native", "native", false},
		{"omitted", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"requestId": id, "projectId": id, "nodes": []any{map[string]any{"key": "worker", "originalText": "Work", "fakeScenario": "candidate"}}}
			if tc.mode != "" {
				input["executionMode"] = tc.mode
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody("graphs.start", id, body)
			if !known || (ownerErr == nil) != tc.valid {
				t.Fatalf("owner error %v", ownerErr)
			}
			if schemaErr := graphNestedSchema(t, "council_graphs_start").Validate(map[string]any{"input": input}); (schemaErr == nil) != tc.valid {
				t.Fatalf("schema error %v", schemaErr)
			}
		})
	}
}

func TestGraphDeliveryRequiresFinalNode(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name                   string
		delivery, final, valid bool
	}{
		{"no_delivery", false, false, true},
		{"delivery_with_final", true, true, true},
		{"delivery_without_final", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"requestId": id, "projectId": id, "nodes": []any{map[string]any{"key": "worker", "originalText": "Work"}}}
			if tc.delivery {
				input["delivery"] = map[string]any{}
			}
			if tc.final {
				input["finalNodeKey"] = "worker"
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody("graphs.start", id, body)
			if !known || (ownerErr == nil) != tc.valid {
				t.Fatalf("owner error %v", ownerErr)
			}
			if schemaErr := graphNestedSchema(t, "council_graphs_start").Validate(map[string]any{"input": input}); (schemaErr == nil) != tc.valid {
				t.Fatalf("schema error %v", schemaErr)
			}
		})
	}
}

func TestGraphRoutingPolicyContextMatchesOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name, kind string
		routing    map[string]any
		valid      bool
	}{
		{"top_start_policy", "top", map[string]any{"useCurrentPolicy": true}, false},
		{"node_start_policy", "node", map[string]any{"useCurrentPolicy": true}, false},
		{"guidance_revision_without_policy", "guidance", map[string]any{"expectedPolicyRevision": "1"}, false},
		{"guidance_revision_false_policy", "guidance", map[string]any{"expectedPolicyRevision": "1", "useCurrentPolicy": false}, false},
		{"guidance_revision_true_policy", "guidance", map[string]any{"expectedPolicyRevision": "1", "useCurrentPolicy": true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tool, operation string
			var input, args map[string]any
			if tc.kind == "guidance" {
				tool, operation = "council_graphs_nodes_guidance", "graphs.nodes.guidance"
				input = map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "taskVersion": "1", "expectedRevision": "1", "guidanceText": "", "routing": tc.routing}
				args = map[string]any{"graphId": id, "nodeKey": "worker", "input": input}
			} else {
				tool, operation = "council_graphs_start", "graphs.start"
				node := map[string]any{"key": "worker", "originalText": "Work"}
				input = map[string]any{"requestId": id, "projectId": id, "nodes": []any{node}}
				if tc.kind == "top" {
					input["routing"] = tc.routing
				} else {
					node["routing"] = tc.routing
				}
				args = map[string]any{"input": input}
			}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody(operation, id, body)
			if !known || (ownerErr == nil) != tc.valid {
				t.Fatalf("owner error %v", ownerErr)
			}
			if schemaErr := graphNestedSchema(t, tool).Validate(args); (schemaErr == nil) != tc.valid {
				t.Fatalf("schema error %v", schemaErr)
			}
		})
	}
}

func TestGraphWorkflowCheckShapesMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, kind := range []string{"start", "guidance"} {
		for _, tc := range []struct {
			name   string
			change func(map[string]any)
			valid  bool
		}{
			{"valid", func(map[string]any) {}, true},
			{"key_limit", func(c map[string]any) { c["key"] = strings.Repeat("a", 64) }, true},
			{"key_over", func(c map[string]any) { c["key"] = strings.Repeat("a", 65) }, false},
			{"key_uppercase", func(c map[string]any) { c["key"] = "Check" }, false},
			{"key_reserved_prefix", func(c map[string]any) { c["key"] = "task-check" }, false},
			{"key_reserved_name", func(c map[string]any) { c["key"] = "independent-review" }, false},
			{"condition_key_limit", func(c map[string]any) { c["coveredConditionKeys"] = []any{strings.Repeat("a", 64)} }, true},
			{"condition_key_over", func(c map[string]any) { c["coveredConditionKeys"] = []any{strings.Repeat("a", 65)} }, false},
			{"condition_key_uppercase", func(c map[string]any) { c["coveredConditionKeys"] = []any{"Condition"} }, false},
			{"argument_over", func(c map[string]any) { c["arguments"] = []any{strings.Repeat("a", 4097)} }, false},
			{"directory_over", func(c map[string]any) { c["relativeWorkingDirectory"] = strings.Repeat("a", 4097) }, false},
			{"scope_path_over", func(c map[string]any) {
				c["scope"] = map[string]any{"kind": "declared_subset", "paths": []any{strings.Repeat("a", 4097)}}
			}, false},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				check := map[string]any{"key": "verify", "executable": "go", "arguments": []any{}}
				tc.change(check)
				workflow := map[string]any{"checks": []any{check}}
				var tool, operation string
				var input, args map[string]any
				if kind == "start" {
					tool, operation = "council_graphs_start", "graphs.start"
					input = map[string]any{"requestId": id, "projectId": id, "nodes": []any{map[string]any{"key": "worker", "originalText": "Work", "workflow": workflow}}}
					args = map[string]any{"input": input}
				} else {
					tool, operation = "council_graphs_nodes_guidance", "graphs.nodes.guidance"
					input = map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "taskVersion": "1", "expectedRevision": "1", "guidanceText": "", "workflow": workflow}
					args = map[string]any{"graphId": id, "nodeKey": "worker", "input": input}
				}
				body, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				ownerErr, known := schemas.ValidateOwnerBody(operation, id, body)
				if !known || (ownerErr == nil) != tc.valid {
					t.Fatalf("owner error %v", ownerErr)
				}
				if schemaErr := graphNestedSchema(t, tool).Validate(args); (schemaErr == nil) != tc.valid {
					t.Fatalf("schema error %v", schemaErr)
				}
			})
		}
	}
}

func TestGraphDeliveryCommandBoundsMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name   string
		change func(map[string]any, map[string]any)
		valid  bool
	}{
		{"valid", func(map[string]any, map[string]any) {}, true},
		{"action_over", func(a, _ map[string]any) { a["action"] = strings.Repeat("a", 1025) }, false},
		{"destination_over", func(a, _ map[string]any) { a["destination"] = strings.Repeat("a", 1025) }, false},
		{"executable_over", func(_, c map[string]any) { c["executable"] = strings.Repeat("a", 4097) }, false},
		{"argument_over", func(_, c map[string]any) { c["arguments"] = []any{strings.Repeat("a", 4097)} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := map[string]any{"executable": "go", "arguments": []any{}}
			action := map[string]any{"action": "publish", "destination": "target", "acceptanceBasis": "command_completion", "command": command}
			tc.change(action, command)
			input := map[string]any{"requestId": id, "projectId": id, "nodes": []any{map[string]any{"key": "worker", "originalText": "Work"}}, "finalNodeKey": "worker", "delivery": map[string]any{"externalAction": action}}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody("graphs.start", id, body)
			if !known || (ownerErr == nil) != tc.valid {
				t.Fatalf("owner error %v", ownerErr)
			}
			if schemaErr := graphNestedSchema(t, "council_graphs_start").Validate(map[string]any{"input": input}); (schemaErr == nil) != tc.valid {
				t.Fatalf("schema error %v", schemaErr)
			}
		})
	}
}

func TestGraphNestedSchemasMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, kind := range []string{"start", "guidance"} {
		for _, variant := range []string{"valid_auto", "node_budget", "node_delivery", "node_author_only", "overflow_revision", "overflow_expected_revision"} {
			t.Run(kind+"/"+variant, func(t *testing.T) {
				if kind == "start" && variant == "overflow_expected_revision" {
					t.Skip("expectedRevision is a guidance authority")
				}
				var tool, operation string
				var input, node, args map[string]any
				if kind == "start" {
					tool, operation = "council_graphs_start", "graphs.start"
					node = map[string]any{"key": "worker", "originalText": "Do the work"}
					input = map[string]any{"requestId": id, "projectId": id, "nodes": []any{node}}
					args = map[string]any{"input": input}
				} else {
					tool, operation = "council_graphs_nodes_guidance", "graphs.nodes.guidance"
					input = map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1", "taskVersion": "1", "expectedRevision": "1", "guidanceText": ""}
					node = input
					args = map[string]any{"graphId": id, "nodeKey": "worker", "input": input}
				}
				switch variant {
				case "valid_auto":
					node["workflow"] = map[string]any{"mode": "auto"}
				case "node_budget":
					node["routing"] = map[string]any{"budget": map[string]any{}}
				case "node_delivery":
					node["workflow"] = map[string]any{"delivery": map[string]any{}}
				case "node_author_only":
					node["workflow"] = map[string]any{"mode": "author_only"}
				case "overflow_revision":
					node["routing"] = map[string]any{"expectedPolicyRevision": "9223372036854775808", "useCurrentPolicy": true}
				case "overflow_expected_revision":
					input["expectedRevision"] = "9223372036854775808"
				}
				raw, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				ownerErr, known := schemas.ValidateOwnerBody(operation, id, raw)
				if !known {
					t.Fatal("missing pure owner validator")
				}
				schemaErr := graphNestedSchema(t, tool).Validate(args)
				want := variant == "valid_auto"
				if (ownerErr == nil) != want || (schemaErr == nil) != want {
					t.Fatalf("schema %v; owner %v; valid=%v", schemaErr, ownerErr, want)
				}
			})
		}
	}
}

// JSON Schema's uniqueItems compares whole objects, so same-node authority
// entries with different revisions need the owner's identity-aware check.
func TestRecoveryAuthorityDuplicateNodeRejectedBeforeDispatch(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	authorities := []any{map[string]any{"nodeId": id, "taskVersion": "1", "nodeControlRevision": "1"}, map[string]any{"nodeId": id, "taskVersion": "2", "nodeControlRevision": "2"}}
	common := map[string]any{"graphVersion": "1", "expectedGraphControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1", "nodeAuthorities": authorities}
	for _, tc := range []struct{ tool, operation string }{
		{"council_graphs_advance", "graphs.advance"},
		{"council_graphs_pause", "graphs.pause"},
		{"council_checkpoints_resume", "checkpoints.resume"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			input := map[string]any{"requestId": id}
			for k, v := range common {
				input[k] = v
			}
			args := map[string]any{"graphId": id, "input": input}
			if tc.operation == "checkpoints.resume" {
				scope := map[string]any{"kind": "graph", "graphId": id}
				for k, v := range common {
					scope[k] = v
				}
				input = map[string]any{"requestId": id, "expectedCheckpointRevision": "1", "scope": scope}
				args = map[string]any{"id": id, "input": input}
			}
			if err := graphNestedSchema(t, tc.tool).Validate(args); err != nil {
				t.Fatalf("schema should allow distinct objects: %v", err)
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			got, known := schemas.ValidateOwnerBody(tc.operation, id, raw)
			if !known || got == nil {
				t.Fatalf("duplicate node authority not rejected: known=%v error=%v", known, got)
			}
		})
	}
}
