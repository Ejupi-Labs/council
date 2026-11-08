package mcphost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
)

func TestRoutingAndExecutionTextBoundsMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name, tool, operation, pathKey, pathID string
		input                                  map[string]any
		field                                  string
		max                                    int
	}{
		{"account label", "council_routing_accounts_create", "routing.accounts.create", "", "", map[string]any{"requestId": id, "label": "x"}, "label", 128},
		{"account provider", "council_routing_accounts_create", "routing.accounts.create", "", "", map[string]any{"requestId": id, "label": "x"}, "providerLabel", 128},
		{"update label", "council_routing_accounts_update", "routing.accounts.update", "id", id, map[string]any{"requestId": id, "expectedRevision": "1"}, "label", 128},
		{"context label", "council_routing_context_update", "routing.context.update", "id", "codex-native", map[string]any{"requestId": id, "expectedRevision": "1"}, "label", 128},
		{"report note", "council_routing_reports_create", "routing.reports.create", "", "", map[string]any{"requestId": id, "expectedRevision": "1", "subject": map[string]any{"kind": "context", "id": "codex-native"}, "dimension": "occupancy", "state": "available"}, "note", 512},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, length := range []int{tc.max, tc.max + 1} {
				tc.input[tc.field] = strings.Repeat("x", length)
				body, err := json.Marshal(tc.input)
				if err != nil {
					t.Fatal(err)
				}
				ownerErr, known := schemas.ValidateOwnerBody(tc.operation, tc.pathID, body)
				args := map[string]any{"input": tc.input}
				if tc.pathKey != "" {
					args[tc.pathKey] = tc.pathID
				}
				want := length == tc.max
				if !known || (ownerErr == nil) != want || workspaceSchemaAccepts(t, tc.tool, args) != want {
					t.Fatalf("length %d: owner=%v reviewed=%v want=%v", length, ownerErr, known, want)
				}
			}
		})
	}
}

func TestTaskFakeScenarioEnumMatchesOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	for _, scenario := range []struct {
		value string
		valid bool
	}{{"slow", true}, {"not-a-scenario", false}} {
		input := map[string]any{"requestId": id, "projectId": id, "originalText": "fixture", "executionMode": "fake", "fakeScenario": scenario.value}
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		ownerErr, known := schemas.ValidateOwnerBody("tasks.start", "", body)
		if !known || (ownerErr == nil) != scenario.valid || workspaceSchemaAccepts(t, "council_tasks_start", map[string]any{"input": input}) != scenario.valid {
			t.Fatalf("scenario %q: owner=%v reviewed=%v", scenario.value, ownerErr, known)
		}
	}
}
