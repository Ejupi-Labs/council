package mcphost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
)

// Compare the remaining mutation bodies with the actual owner decoder and pure
// validator, without claiming any state-dependent authority or dispatch.
func TestRemainingExecutionRoutingWorkflowBodiesAgainstOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		tool, operation, pathKey, pathID, badField string
		valid                                      map[string]any
		bad                                        any
	}{
		{"council_tasks_attempts_start", "tasks.attempts.start", "taskId", id, "taskVersion", map[string]any{"requestId": id, "expectedRevision": "1", "taskVersion": "1"}, "0"},
		{"council_attempts_stop", "attempts.stop", "id", id, "requestId", map[string]any{"requestId": id}, "bad"},
		{"council_tasks_response_accept", "tasks.response.accept", "taskId", id, "confirmation", map[string]any{"requestId": id, "expectedRevision": "1", "taskVersion": "1", "attemptId": id, "responseArtifactId": id, "responseSha256": strings.Repeat("a", 64), "confirmation": "reviewed_response_and_conditions"}, "unreviewed"},
		{"council_routing_context_update", "routing.context.update", "id", "codex-native", "label", map[string]any{"requestId": id, "expectedRevision": "1", "label": "Fixture"}, ""},
		{"council_routing_profile_update", "routing.profile.update", "id", "codex-native-default", "clearReferencePrice", map[string]any{"requestId": id, "expectedRevision": "1", "clearReferencePrice": true}, false},
		{"council_routing_accounts_create", "routing.accounts.create", "", "", "label", map[string]any{"requestId": id, "label": "Fixture"}, ""},
		{"council_routing_accounts_update", "routing.accounts.update", "id", id, "entitlement", map[string]any{"requestId": id, "expectedRevision": "1", "entitlement": "unknown"}, "unsupported"},
		{"council_routing_links_update", "routing.links.update", "id", "codex-native", "unlink", map[string]any{"requestId": id, "expectedRevision": "1", "unlink": true}, false},
		{"council_workflow_preferences_update", "workflow.preferences.update", "", "", "localMode", map[string]any{"requestId": id, "expectedRevision": "1", "localMode": "review_only"}, "automatic"},
		{"council_tasks_workflow_advance", "tasks.workflow.advance", "taskId", id, "workflowRevision", map[string]any{"requestId": id, "expectedRevision": "1", "workflowRevision": "1", "taskVersion": "1"}, "0"},
		{"council_tasks_workflow_check", "tasks.workflow.check", "taskId", id, "checkId", map[string]any{"requestId": id, "expectedRevision": "1", "workflowRevision": "1", "taskVersion": "1", "checkId": id}, "invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.operation, func(t *testing.T) {
			args := map[string]any{"input": tc.valid}
			if tc.pathKey != "" {
				args[tc.pathKey] = tc.pathID
			}
			check := func(want bool) {
				t.Helper()
				body, err := json.Marshal(tc.valid)
				if err != nil {
					t.Fatal(err)
				}
				ownerErr, known := schemas.ValidateOwnerBody(tc.operation, tc.pathID, body)
				if !known || (ownerErr == nil) != want {
					t.Fatalf("owner validity: %v (known=%v)", ownerErr, known)
				}
				if got := workspaceSchemaAccepts(t, tc.tool, args); got != want {
					t.Fatalf("schema validity %v, expected %v", got, want)
				}
			}
			check(true)
			tc.valid[tc.badField] = tc.bad
			check(false)
		})
	}
}
