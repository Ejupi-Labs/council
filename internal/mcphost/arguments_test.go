package mcphost

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

func TestOwnerRequestCatalogPathAndQueryInventory(t *testing.T) {
	catalog := ToolCatalog()
	if len(catalog) != 91 || len(toolPathArguments) != len(catalog) {
		t.Fatalf("catalog=%d path mappings=%d", len(catalog), len(toolPathArguments))
	}
	for _, mapping := range catalog {
		t.Run(mapping.Name, func(t *testing.T) {
			route, ok := ownerclient.RouteFor(mapping.Operation)
			if !ok {
				t.Fatal("missing fixed owner route")
			}
			pathKeys, ok := toolPathArguments[mapping.Name]
			if !ok || len(pathKeys) != len(route.PathKinds) {
				t.Fatalf("path keys %v, route %v", pathKeys, route.PathKinds)
			}
			raw, ok := schemas.DraftLookup(mapping.Name)
			if !ok {
				t.Fatal("missing schema")
			}
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			}
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			expected := map[string]bool{}
			required := map[string]bool{}
			for _, key := range schema.Required {
				required[key] = true
			}
			for _, key := range pathKeys {
				expected[key] = true
				if !required[key] {
					t.Errorf("path %q is not required", key)
				}
			}
			for _, key := range route.QueryKeys {
				if expected[key] {
					t.Errorf("query/path collision %q", key)
				}
				expected[key] = true
			}
			if route.Method != "GET" {
				expected["input"] = true
				if !required["input"] {
					t.Error("mutation input not required")
				}
			}
			if len(expected) != len(schema.Properties) {
				t.Fatalf("schema members %v; expected %v", schema.Properties, expected)
			}
			for key := range schema.Properties {
				if !expected[key] {
					t.Errorf("unexpected schema member %q", key)
				}
			}
			args := map[string]any{}
			wantIDs := make([]string, 0, len(pathKeys))
			for i, key := range pathKeys {
				v := fmt.Sprintf("id-%d", i)
				args[key] = v
				wantIDs = append(wantIDs, v)
			}
			for _, key := range route.QueryKeys {
				args[key] = "query-" + key
			}
			if route.Method != "GET" {
				args["input"] = map[string]any{"requestId": "0123456789abcdef0123456789abcdef"}
			}
			encoded, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			req, invalid := OwnerRequest(mapping, encoded)
			if invalid != nil {
				t.Fatalf("mapping: %v", invalid)
			}
			if req.Operation != mapping.Operation || !reflect.DeepEqual(req.PathIDs, wantIDs) && !(len(req.PathIDs) == 0 && len(wantIDs) == 0) {
				t.Fatalf("got operation %s paths %v, expected %s %v", req.Operation, req.PathIDs, mapping.Operation, wantIDs)
			}
			for _, key := range route.QueryKeys {
				if req.Query[key] != "query-"+key {
					t.Errorf("query %q missing", key)
				}
			}
		})
	}
}

func TestOwnerRequestCompositeIdentityOrderAndExactInput(t *testing.T) {
	const id1 = "0123456789abcdef0123456789abcdef"
	const id2 = "fedcba9876543210fedcba9876543210"
	for _, tc := range []struct {
		mapping ToolMapping
		raw     string
		ids     []string
		query   map[string]string
	}{
		{ToolMapping{"council_tasks_version_get", "tasks.version.get"}, `{"taskId":"` + id1 + `","version":"2"}`, []string{id1, "2"}, nil},
		{ToolMapping{"council_graphs_nodes_guidance", "graphs.nodes.guidance"}, `{"nodeKey":"worker","graphId":"` + id1 + `","input": { "requestId" : "` + id2 + `", "guidanceText" : "\u0061" } }`, []string{id1, "worker"}, nil},
		{ToolMapping{"council_graphs_inputs_members", "graphs.inputs.members"}, `{"inputId":"` + id2 + `","graphId":"` + id1 + `","cursor":"ABC","limit":"5"}`, []string{id1, id2}, map[string]string{"cursor": "ABC", "limit": "5"}},
		{ToolMapping{"council_graphs_dispatches_get", "graphs.dispatches.get"}, `{"dispatchId":"` + id2 + `","graphId":"` + id1 + `"}`, []string{id1, id2}, nil},
	} {
		t.Run(tc.mapping.Name, func(t *testing.T) {
			req, invalid := OwnerRequest(tc.mapping, []byte(tc.raw))
			if invalid != nil {
				t.Fatal(invalid)
			}
			if !reflect.DeepEqual(req.PathIDs, tc.ids) {
				t.Fatalf("paths %v want %v", req.PathIDs, tc.ids)
			}
			for k, v := range tc.query {
				if req.Query[k] != v {
					t.Fatalf("query %q=%q", k, req.Query[k])
				}
			}
			if tc.mapping.Operation == "graphs.nodes.guidance" {
				want := `{ "requestId" : "` + id2 + `", "guidanceText" : "\u0061" }`
				if string(req.Body) != want || req.RequestID != id2 {
					t.Fatalf("input bytes %q identity %q", req.Body, req.RequestID)
				}
			}
		})
	}
}

func TestOwnerRequestRejectsMalformedArguments(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	mapping := ToolMapping{"council_graphs_nodes_guidance", "graphs.nodes.guidance"}
	for _, raw := range []string{
		"", "null", "[]", `{"graphId":"` + id + `","nodeKey":"x"}`,
		`{"graphId":null,"nodeKey":"x","input":{"requestId":"` + id + `"}}`,
		`{"graphId":1,"nodeKey":"x","input":{"requestId":"` + id + `"}}`,
		`{"graphId":"` + id + `","nodeKey":"x","extra":true,"input":{"requestId":"` + id + `"}}`,
		`{"graphId":"` + id + `","graphId":"` + id + `","nodeKey":"x","input":{"requestId":"` + id + `"}}`,
		`{"graphId":"` + id + `","nodeKey":"x","input":{"requestId":null}}`,
		`{"graphId":"` + id + `","nodeKey":"x","input":{"requestId":"bad"}}`,
		`{"graphId":"` + id + `","nodeKey":"x","input":{"requestId":"` + id + `"}} {}`,
	} {
		if _, invalid := OwnerRequest(mapping, []byte(raw)); invalid == nil {
			t.Errorf("accepted malformed %s", raw)
		}
	}
	if _, invalid := OwnerRequest(ToolMapping{"council_graphs_nodes_guidance", "attempts.stop"}, []byte(`{}`)); invalid == nil {
		t.Fatal("mismatched name/operation accepted")
	}
	if _, invalid := OwnerRequest(ToolMapping{"arbitrary", "graphs.nodes.guidance"}, []byte(`{}`)); invalid == nil {
		t.Fatal("unknown tool accepted")
	}
	for _, raw := range [][]byte{nil, {}} {
		req, invalid := OwnerRequest(ToolMapping{"council_owner_meta", "meta"}, raw)
		if invalid != nil || req.Operation != "meta" {
			t.Fatalf("empty read: %+v %v", req, invalid)
		}
		if _, invalid := OwnerRequest(ToolMapping{"council_routing_capacity_get", "routing.capacity.get"}, raw); invalid == nil {
			t.Fatal("omission accepted for required selector")
		}
	}
	if _, invalid := OwnerRequest(ToolMapping{"council_owner_meta", "meta"}, []byte("  ")); invalid == nil {
		t.Fatal("whitespace is not omitted JSON")
	}
}
