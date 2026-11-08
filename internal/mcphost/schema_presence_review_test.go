package mcphost

import (
	"encoding/json"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
)

func TestExecutionSchemaAndOwnerRejectPresentEmptySelectors(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name, operation, field string
		base                   map[string]any
	}{
		{"council_tasks_start", "tasks.start", "expectedPolicyRevision", map[string]any{"requestId": id, "projectId": id, "originalText": "fixture"}},
		{"council_tasks_start", "tasks.start", "executionMode", map[string]any{"requestId": id, "projectId": id, "originalText": "fixture"}},
		{"council_tasks_start", "tasks.start", "fakeScenario", map[string]any{"requestId": id, "projectId": id, "originalText": "fixture"}},
		{"council_tasks_start", "tasks.start", "policyOverride", map[string]any{"requestId": id, "projectId": id, "originalText": "fixture"}},
		{"council_tasks_guidance", "tasks.guidance", "policyOverride", map[string]any{"requestId": id, "expectedRevision": "1", "guidanceText": "fixture"}},
		{"council_tasks_attempts_start", "tasks.attempts.start", "executionMode", map[string]any{"requestId": id, "expectedRevision": "1", "taskVersion": "1"}},
		{"council_tasks_attempts_start", "tasks.attempts.start", "fakeScenario", map[string]any{"requestId": id, "expectedRevision": "1", "taskVersion": "1"}},
	}
	for _, tc := range tests {
		t.Run(tc.operation+"/"+tc.field, func(t *testing.T) {
			args := map[string]any{"input": tc.base}
			if tc.operation != "tasks.start" {
				args["taskId"] = id
			}
			body, _ := json.Marshal(tc.base)
			if !workspaceSchemaAccepts(t, tc.name, args) {
				t.Fatal("schema rejected omitted selector")
			}
			if err, ok := schemas.ValidateOwnerBody(tc.operation, id, body); !ok || err != nil {
				t.Fatalf("owner validator rejected omitted selector: %v", err)
			}
			tc.base[tc.field] = ""
			body, _ = json.Marshal(tc.base)
			if workspaceSchemaAccepts(t, tc.name, args) {
				t.Fatal("schema accepted explicitly empty selector")
			}
			if err, ok := schemas.ValidateOwnerBody(tc.operation, id, body); !ok || err == nil {
				t.Fatal("owner validator accepted explicitly empty selector")
			}
		})
	}
}
