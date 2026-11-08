package mcphost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/google/jsonschema-go/jsonschema"
)

type schemaRecordingExecutor struct{ calls []string }

func (e *schemaRecordingExecutor) Execute(_ context.Context, op string, _ app.Params) domain.Result {
	e.calls = append(e.calls, op)
	return domain.Success(http.StatusOK, "test", nil, nil)
}

// The known-empty schemas are checked against actual owner HTTP routing, not
// merely against the catalog: an unexpected query must never reach the owner.
func TestReviewedEmptySchemasMatchOwnerRoutes(t *testing.T) {
	cases := []struct{ name, path, operation string }{
		{"council_owner_meta", "/meta", "meta"},
		{"council_policy_get", "/policy", "policy.get"},
		{"council_agents_list", "/agents", "agents.list"},
		{"council_execution_capabilities", "/execution/capabilities", "execution.capabilities"},
		{"council_routing_policy_get", "/routing/policy", "routing.policy.get"},
		{"council_routing_contexts_list", "/routing/contexts", "routing.contexts.list"},
		{"council_workflow_preferences_get", "/workflow/preferences", "workflow.preferences.get"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, ok := schemas.DraftLookup(tc.name)
			if !ok {
				t.Fatal("reviewed schema missing")
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(map[string]any{}); err != nil {
				t.Fatalf("empty object rejected: %v", err)
			}
			if err := resolved.Validate(map[string]any{"unexpected": "1"}); err == nil {
				t.Fatal("unknown field accepted")
			}
			if err := resolved.Validate(nil); err == nil {
				t.Fatal("null arguments accepted")
			}

			executor := &schemaRecordingExecutor{}
			handler := httpapi.New(executor, httpapi.Config{Host: "council.test", Token: "test-token"})
			for _, query := range []string{"", "?unexpected=1"} {
				req := httptest.NewRequest(http.MethodGet, "http://council.test/api/v1"+tc.path+query, nil)
				req.Header.Set("Authorization", "Bearer test-token")
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, req)
				if query == "" {
					if res.Code != http.StatusOK || (tc.operation == "meta" && len(executor.calls) != 0) || (tc.operation != "meta" && (len(executor.calls) != 1 || executor.calls[0] != tc.operation)) {
						t.Fatalf("valid route: status %d calls %v", res.Code, executor.calls)
					}
				} else if res.Code != http.StatusBadRequest || (tc.operation == "meta" && len(executor.calls) != 0) || (tc.operation != "meta" && len(executor.calls) != 1) {
					t.Fatalf("unexpected query reached owner: status %d calls %v", res.Code, executor.calls)
				}
			}
		})
	}
}

func TestSchemaRegistryIsClosedAndDefensive(t *testing.T) {
	if schemas.Version != "008-v1" {
		t.Fatal("unexpected schema version")
	}
	if len(schemas.Names()) != len(ToolCatalog()) {
		t.Fatal("every catalog schema must have an independently reviewed digest")
	}
	names := schemas.DraftNames()
	catalog := map[string]bool{}
	for _, mapping := range ToolCatalog() {
		catalog[mapping.Name] = true
	}
	for i, name := range names {
		if !catalog[name] || (i > 0 && names[i-1] >= name) {
			t.Fatalf("unknown or unsorted schema %q", name)
		}
		raw, ok := schemas.DraftLookup(name)
		if !ok || len(raw) == 0 {
			t.Fatalf("missing schema %q", name)
		}
		compact := append(jsontext.Value(nil), raw...)
		if err := compact.Compact(); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(compact)
		digest, ok := schemas.DraftDigest(name)
		if !ok || digest != hex.EncodeToString(sum[:]) {
			t.Fatalf("incorrect digest for %q", name)
		}
		raw[0] = '!'
		fresh, _ := schemas.DraftLookup(name)
		if fresh[0] == '!' {
			t.Fatal("caller mutated registry")
		}
	}
	if raw, ok := schemas.Lookup("not-a-tool"); ok || raw != nil {
		t.Fatal("unknown schema enabled")
	}
	if digest, ok := schemas.Digest("not-a-tool"); ok || digest != "" {
		t.Fatal("unknown digest returned")
	}
}

func TestDraftSchemasResolveAndRemainClosed(t *testing.T) {
	var walk func(*testing.T, any)
	walk = func(t *testing.T, value any) {
		switch v := value.(type) {
		case map[string]any:
			if _, exists := v["default"]; exists {
				t.Fatal("schema must never apply defaults")
			}
			if v["type"] == "object" && v["additionalProperties"] != false {
				t.Fatal("object schema is not explicitly closed")
			}
			for _, child := range v {
				walk(t, child)
			}
		case []any:
			for _, child := range v {
				walk(t, child)
			}
		}
	}
	for _, name := range schemas.DraftNames() {
		t.Run(name, func(t *testing.T) {
			raw, _ := schemas.DraftLookup(name)
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			walk(t, value)
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			if _, err := schema.Resolve(nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHistoryMutationSchemasMatchOwnerValidation(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name   string
		body   map[string]any
		decode func([]byte) *domain.Error
	}{
		{"council_history_capture", map[string]any{"requestId": id, "projectId": id}, func(raw []byte) *domain.Error {
			var in domain.CaptureHistoryInput
			if err := domain.StrictDecodeHistory(raw, &in); err != nil {
				return err
			}
			return domain.ValidateCaptureHistoryInput(in)
		}},
		{"council_history_preview", map[string]any{"requestId": id, "snapshotId": id, "snapshotRevision": "1", "filename": "history-fixture.json"}, func(raw []byte) *domain.Error {
			var in domain.PreviewHistoryExportInput
			if err := domain.StrictDecodeHistory(raw, &in); err != nil {
				return err
			}
			return domain.ValidatePreviewHistoryExportInput(in)
		}},
		{"council_history_export", map[string]any{"requestId": id, "previewId": id, "previewRevision": "1"}, func(raw []byte) *domain.Error {
			var in domain.ExecuteHistoryExportInput
			if err := domain.StrictDecodeHistory(raw, &in); err != nil {
				return err
			}
			return domain.ValidateExecuteHistoryExportInput(in)
		}},
		{"council_history_reconcile", map[string]any{"requestId": id, "expectedRevision": "9223372036854775807", "action": "verify_no_publication"}, func(raw []byte) *domain.Error {
			var in domain.ReconcileHistoryOperationInput
			if err := domain.StrictDecodeHistory(raw, &in); err != nil {
				return err
			}
			return domain.ValidateReconcileHistoryOperationInput(in)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schemaBytes, ok := schemas.DraftLookup(tc.name)
			if !ok {
				t.Fatal("missing history schema")
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(schemaBytes, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"valid", "unknown", "bad_id", "null", "missing"} {
				body := map[string]any{}
				for k, v := range tc.body {
					body[k] = v
				}
				switch kind {
				case "unknown":
					body["unknown"] = true
				case "bad_id":
					body["requestId"] = "bad"
				case "null":
					body["requestId"] = nil
				case "missing":
					delete(body, "requestId")
				}
				args := map[string]any{"input": body}
				if tc.name == "council_history_reconcile" {
					args["operationId"] = id
				}
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				ownerOK := tc.decode(encoded) == nil
				schemaOK := resolved.Validate(args) == nil
				if ownerOK != (kind == "valid") || schemaOK != ownerOK {
					t.Fatalf("%s: owner %v schema %v", kind, ownerOK, schemaOK)
				}
			}
		})
	}
}

func TestGraphAndRecoveryMutationSchemaOwnerValidation(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	authority := map[string]any{"nodeId": id, "taskVersion": "1", "nodeControlRevision": "1"}
	base := map[string]any{"requestId": id, "graphVersion": "1", "expectedGraphControlRevision": "1", "expectedBudgetAuthorityEpoch": "1", "expectedWindowNumber": "1", "nodeAuthorities": []any{authority}}
	cases := []struct {
		name   string
		path   map[string]any
		body   map[string]any
		decode func([]byte) *domain.Error
	}{
		{"council_graphs_advance", map[string]any{"graphId": id}, base, func(b []byte) *domain.Error {
			var in domain.GraphAdvanceInput
			if e := domain.StrictDecode(b, &in); e != nil {
				return e
			}
			return domain.ValidateGraphAdvanceInput(in)
		}},
		{"council_graphs_pause", map[string]any{"graphId": id}, base, func(b []byte) *domain.Error {
			var in domain.PauseGraphInput
			if e := domain.StrictDecode(b, &in); e != nil {
				return e
			}
			return domain.ValidatePauseGraphInput(in)
		}},
		{"council_graphs_stop", map[string]any{"graphId": id}, map[string]any{"requestId": id, "expectedGraphControlRevision": "1"}, func(b []byte) *domain.Error {
			var in domain.GraphStopInput
			if e := domain.StrictDecode(b, &in); e != nil {
				return e
			}
			return domain.ValidateGraphStopInput(in)
		}},
		{"council_graphs_nodes_stop", map[string]any{"graphId": id, "nodeKey": "member"}, map[string]any{"requestId": id, "expectedGraphControlRevision": "1", "nodeId": id, "expectedNodeControlRevision": "1"}, func(b []byte) *domain.Error {
			var in domain.GraphNodeStopInput
			if e := domain.StrictDecode(b, &in); e != nil {
				return e
			}
			return domain.ValidateGraphNodeStopInput(in)
		}},
		{"council_tasks_pause", map[string]any{"taskId": id}, map[string]any{"requestId": id, "taskVersion": "1", "expectedTaskRevision": "1"}, func(b []byte) *domain.Error {
			var in domain.PauseTaskInput
			if e := domain.StrictDecode(b, &in); e != nil {
				return e
			}
			return domain.ValidatePauseTaskInput(in)
		}},
		{"council_incidents_reconcile", map[string]any{"id": id}, map[string]any{"requestId": id, "expectedIncidentRevision": "1", "action": "verify_and_reconcile"}, func(b []byte) *domain.Error {
			var in domain.ReconciliationInput
			if e := domain.StrictDecode(b, &in); e != nil {
				return e
			}
			return domain.ValidateReconciliationInput(in)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, ok := schemas.DraftLookup(tc.name)
			if !ok {
				t.Fatal("missing schema")
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			resolved, err := schema.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"valid", "unknown", "bad_id", "null", "missing"} {
				body := map[string]any{}
				for k, v := range tc.body {
					body[k] = v
				}
				switch kind {
				case "unknown":
					body["unknown"] = true
				case "bad_id":
					body["requestId"] = "bad"
				case "null":
					body["requestId"] = nil
				case "missing":
					delete(body, "requestId")
				}
				args := map[string]any{"input": body}
				for k, v := range tc.path {
					args[k] = v
				}
				encoded, _ := json.Marshal(body)
				ownerOK := tc.decode(encoded) == nil
				schemaOK := resolved.Validate(args) == nil
				if ownerOK != (kind == "valid") || schemaOK != ownerOK {
					t.Fatalf("%s: owner %v schema %v", kind, ownerOK, schemaOK)
				}
			}
		})
	}
}
