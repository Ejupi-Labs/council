package mcphost

import (
	"encoding/json"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
)

func TestConditionalMutationSchemasMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	profile := func(extra map[string]any) map[string]any {
		in := map[string]any{"requestId": id, "expectedRevision": "1"}
		for k, v := range extra {
			in[k] = v
		}
		return in
	}
	report := func(extra map[string]any) map[string]any {
		in := map[string]any{"requestId": id, "expectedRevision": "1", "subject": map[string]any{"kind": "context", "id": "codex-native"}, "dimension": "occupancy", "state": "busy"}
		for k, v := range extra {
			in[k] = v
		}
		return in
	}
	start := func(extra map[string]any) map[string]any {
		in := map[string]any{"requestId": id, "projectId": id, "originalText": "fixture"}
		for k, v := range extra {
			in[k] = v
		}
		return in
	}
	guidance := func(routing map[string]any) map[string]any {
		return map[string]any{"requestId": id, "expectedRevision": "1", "guidanceText": "fixture", "routing": routing}
	}
	adopt := func(routing map[string]any) map[string]any {
		return map[string]any{"requestId": id, "expectedRevision": "1", "action": "adopt", "additionalAdmissions": "1", "newWindowSeconds": 3600, "routing": routing}
	}
	price := map[string]any{"currency": "USD", "sourceLabel": "fixture", "observedAt": "2026-01-02T03:04:05Z"}
	draft := map[string]any{"draftId": id, "revision": "1"}
	tests := []struct {
		name, tool, operation, pathKey, pathID string
		input                                  map[string]any
		want                                   bool
	}{
		{"profile unchanged", "council_routing_profile_update", "routing.profile.update", "id", "codex-native-default", profile(nil), true},
		{"profile price", "council_routing_profile_update", "routing.profile.update", "id", "codex-native-default", profile(map[string]any{"referencePrice": price}), true},
		{"profile clear", "council_routing_profile_update", "routing.profile.update", "id", "codex-native-default", profile(map[string]any{"clearReferencePrice": true}), true},
		{"profile conflict", "council_routing_profile_update", "routing.profile.update", "id", "codex-native-default", profile(map[string]any{"referencePrice": price, "clearReferencePrice": true}), false},
		{"correction absent", "council_routing_reports_create", "routing.reports.create", "", "", report(nil), true},
		{"correction recover", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"correctionOf": id, "correctionAction": "recover"}), true},
		{"correction of only", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"correctionOf": id}), false},
		{"correction action only", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"correctionAction": "recover"}), false},
		{"withdraw unknown", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"correctionOf": id, "correctionAction": "withdraw", "state": "unknown"}), true},
		{"withdraw busy", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"correctionOf": id, "correctionAction": "withdraw"}), false},
		{"retry throttle", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"dimension": "service", "state": "temporary_throttle", "retryAfter": "2026-01-02T03:04:05Z"}), true},
		{"retry overload", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"dimension": "service", "state": "temporary_overload", "retryAfter": "2026-01-02T03:04:05Z"}), true},
		{"retry occupancy", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"retryAfter": "2026-01-02T03:04:05Z"}), false},
		{"retry permanent", "council_routing_reports_create", "routing.reports.create", "", "", report(map[string]any{"dimension": "service", "state": "authentication_denied", "retryAfter": "2026-01-02T03:04:05Z"}), false},
		{"start draft source", "council_tasks_start", "tasks.start", "", "", start(map[string]any{"sourceDraftRef": draft}), true},
		{"start policy source", "council_tasks_start", "tasks.start", "", "", start(map[string]any{"expectedPolicyRevision": "1"}), true},
		{"start conflicting sources", "council_tasks_start", "tasks.start", "", "", start(map[string]any{"sourceDraftRef": draft, "expectedPolicyRevision": "1"}), false},
		{"start routing without current policy", "council_tasks_start", "tasks.start", "", "", start(map[string]any{"routing": map[string]any{"suitability": "routine"}}), true},
		{"start routing current policy false", "council_tasks_start", "tasks.start", "", "", start(map[string]any{"routing": map[string]any{"useCurrentPolicy": false}}), false},
		{"start routing current policy true", "council_tasks_start", "tasks.start", "", "", start(map[string]any{"routing": map[string]any{"useCurrentPolicy": true}}), false},
		{"guidance routing budget", "council_tasks_guidance", "tasks.guidance", "taskId", id, guidance(map[string]any{"budget": map[string]any{"admissionLimit": "1"}}), false},
		{"guidance revision no policy", "council_tasks_guidance", "tasks.guidance", "taskId", id, guidance(map[string]any{"expectedPolicyRevision": "1"}), false},
		{"guidance revision false policy", "council_tasks_guidance", "tasks.guidance", "taskId", id, guidance(map[string]any{"expectedPolicyRevision": "1", "useCurrentPolicy": false}), false},
		{"guidance revision true policy", "council_tasks_guidance", "tasks.guidance", "taskId", id, guidance(map[string]any{"expectedPolicyRevision": "1", "useCurrentPolicy": true}), true},
		{"adopt routing", "council_tasks_budget_amend", "tasks.budget.amend", "taskId", id, adopt(map[string]any{"suitability": "routine"}), true},
		{"adopt routing budget", "council_tasks_budget_amend", "tasks.budget.amend", "taskId", id, adopt(map[string]any{"budget": map[string]any{"admissionLimit": "1"}}), false},
		{"adopt routing current policy", "council_tasks_budget_amend", "tasks.budget.amend", "taskId", id, adopt(map[string]any{"useCurrentPolicy": true}), false},
	}
	// Reference prices are strings containing an exact nonnegative JSON number.
	for _, tc := range []struct {
		value string
		valid bool
	}{{"0", true}, {"1.25e+2", true}, {"-1", false}, {"abc", false}, {"01", false}} {
		priceValue := map[string]any{"currency": "USD", "sourceLabel": "fixture", "observedAt": "2026-01-02T03:04:05Z", "inputPerMillion": tc.value}
		tests = append(tests, struct {
			name, tool, operation, pathKey, pathID string
			input                                  map[string]any
			want                                   bool
		}{"price " + tc.value, "council_routing_profile_update", "routing.profile.update", "id", "codex-native-default", profile(map[string]any{"referencePrice": priceValue}), tc.valid})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody(tc.operation, tc.pathID, body)
			if !known || (ownerErr == nil) != tc.want {
				t.Fatalf("owner validity: %v (known=%v)", ownerErr, known)
			}
			args := map[string]any{"input": tc.input}
			if tc.pathKey != "" {
				args[tc.pathKey] = tc.pathID
			}
			if got := workspaceSchemaAccepts(t, tc.tool, args); got != tc.want {
				t.Fatalf("schema validity %v, expected %v", got, tc.want)
			}
		})
	}
}

func TestWorkflowCheckScopeAndKeySchemasMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name  string
		check map[string]any
		want  bool
	}{
		{"ordinary key", map[string]any{"key": "run_checks", "executable": "go", "arguments": []string{"test"}}, true},
		{"uppercase key", map[string]any{"key": "Run", "executable": "go", "arguments": []string{}}, false},
		{"punctuation key", map[string]any{"key": "run.check", "executable": "go", "arguments": []string{}}, false},
		{"reserved prefix", map[string]any{"key": "task-check", "executable": "go", "arguments": []string{}}, false},
		{"reserved exact", map[string]any{"key": "independent-review", "executable": "go", "arguments": []string{}}, false},
		{"covered reserved allowed", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "coveredConditionKeys": []string{"task-condition"}}, true},
		{"covered invalid characters", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "coveredConditionKeys": []string{"bad.key"}}, false},
		{"covered duplicate", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "coveredConditionKeys": []string{"condition", "condition"}}, false},
		{"whole candidate empty", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "scope": map[string]any{"kind": "whole_candidate", "paths": []string{}}}, true},
		{"whole candidate nonempty", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "scope": map[string]any{"kind": "whole_candidate", "paths": []string{"src"}}}, false},
		{"declared subset nonempty", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "scope": map[string]any{"kind": "declared_subset", "paths": []string{"src"}}}, true},
		{"declared subset empty", map[string]any{"key": "check", "executable": "go", "arguments": []string{}, "scope": map[string]any{"kind": "declared_subset", "paths": []string{}}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"requestId": id, "projectId": id, "originalText": "fixture", "workflow": map[string]any{"mode": "auto", "checks": []any{tc.check}}}
			body, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			ownerErr, known := schemas.ValidateOwnerBody("tasks.start", "", body)
			if !known || (ownerErr == nil) != tc.want {
				t.Fatalf("owner validity: %v (known=%v)", ownerErr, known)
			}
			if got := workspaceSchemaAccepts(t, "council_tasks_start", map[string]any{"input": input}); got != tc.want {
				t.Fatalf("schema validity %v, expected %v", got, tc.want)
			}
		})
	}
}
