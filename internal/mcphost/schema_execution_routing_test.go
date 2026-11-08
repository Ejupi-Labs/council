package mcphost

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/google/jsonschema-go/jsonschema"
)

// These cases compare the published argument schema with the same decode and
// domain validation used by the owner. No process, store, or live agent is used.
func TestExecutionRoutingWorkflowSchemasMatchOwner(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name, tool, body string
		path             map[string]any
		decode           func([]byte) *domain.Error
		want             bool
	}{
		{"start_minimal", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello"}`, nil, validateSchemaStart, true},
		{"start_author_only", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello","workflow":{"mode":"author_only"}}`, nil, validateSchemaStart, true},
		{"start_author_only_with_checks", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello","workflow":{"mode":"author_only","checks":[]}}`, nil, validateSchemaStart, false},
		{"start_scenario_without_fake", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello","executionMode":"native","fakeScenario":"candidate"}`, nil, validateSchemaStart, false},
		{"start_bad_revision", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello","expectedPolicyRevision":"9223372036854775808"}`, nil, validateSchemaStart, false},
		{"start_duplicate_evidence", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello","acceptance":{"requiredEvidence":["check","check"]}}`, nil, validateSchemaStart, false},
		{"start_unknown", "council_tasks_start", `{"requestId":"%s","projectId":"%s","originalText":"hello","unexpected":true}`, nil, validateSchemaStart, false},
		{"guidance_empty_text", "council_tasks_guidance", `{"requestId":"%s","expectedRevision":"1","guidanceText":""}`, map[string]any{"taskId": id}, validateSchemaGuidance, true},
		{"guidance_missing_text", "council_tasks_guidance", `{"requestId":"%s","expectedRevision":"1"}`, map[string]any{"taskId": id}, validateSchemaGuidance, false},
		{"guidance_null_text", "council_tasks_guidance", `{"requestId":"%s","expectedRevision":"1","guidanceText":null}`, map[string]any{"taskId": id}, validateSchemaGuidance, false},
		{"guidance_conflicting_override", "council_tasks_guidance", `{"requestId":"%s","expectedRevision":"1","guidanceText":"","clearPolicyOverride":true,"policyOverride":"supervised"}`, map[string]any{"taskId": id}, validateSchemaGuidance, false},
		{"routing_policy", "council_routing_policy_update", `{"requestId":"%s","expectedRevision":"1","economyOrder":[],"defaultBudget":{"admissionLimit":"16","windowSeconds":86400}}`, nil, validateSchemaRoutingPolicy, true},
		{"routing_policy_duplicate_profiles", "council_routing_policy_update", `{"requestId":"%s","expectedRevision":"1","economyOrder":["codex-native-default","codex-native-default"]}`, nil, validateSchemaRoutingPolicy, false},
		{"routing_policy_over_budget", "council_routing_policy_update", `{"requestId":"%s","expectedRevision":"1","defaultBudget":{"admissionLimit":"17"}}`, nil, validateSchemaRoutingPolicy, false},
		{"capacity_context", "council_routing_reports_create", `{"requestId":"%s","expectedRevision":"1","subject":{"kind":"context","id":"codex-native"},"dimension":"occupancy","state":"busy"}`, nil, validateSchemaReport, true},
		{"capacity_account", "council_routing_reports_create", `{"requestId":"%s","expectedRevision":"1","subject":{"kind":"account","id":"%s"},"dimension":"occupancy","state":"available"}`, nil, validateSchemaReport, true},
		{"capacity_invalid_subject", "council_routing_reports_create", `{"requestId":"%s","expectedRevision":"1","subject":{"kind":"context","id":"bad"},"dimension":"occupancy","state":"busy"}`, nil, validateSchemaReport, false},
		{"capacity_invalid_state", "council_routing_reports_create", `{"requestId":"%s","expectedRevision":"1","subject":{"kind":"context","id":"codex-native"},"dimension":"occupancy","state":"exhausted"}`, nil, validateSchemaReport, false},
		{"budget_grant", "council_tasks_budget_amend", `{"requestId":"%s","expectedRevision":"1","action":"grant","expectedBudgetRevision":"1","additionalAdmissions":"1"}`, map[string]any{"taskId": id}, validateSchemaAmendment, true},
		{"budget_grant_missing_revision", "council_tasks_budget_amend", `{"requestId":"%s","expectedRevision":"1","action":"grant","additionalAdmissions":"1"}`, map[string]any{"taskId": id}, validateSchemaAmendment, false},
		{"budget_adopt", "council_tasks_budget_amend", `{"requestId":"%s","expectedRevision":"1","action":"adopt","newWindowSeconds":3600,"additionalAdmissions":"1"}`, map[string]any{"taskId": id}, validateSchemaAmendment, true},
		{"budget_adopt_with_old_revision", "council_tasks_budget_amend", `{"requestId":"%s","expectedRevision":"1","action":"adopt","expectedBudgetRevision":"1","newWindowSeconds":3600,"additionalAdmissions":"1"}`, map[string]any{"taskId": id}, validateSchemaAmendment, false},
		{"workflow_adopt", "council_tasks_workflow_adopt", `{"requestId":"%s","expectedRevision":"1","taskVersion":"1","workflow":{"mode":"auto","checks":[]}}`, map[string]any{"taskId": id}, validateSchemaAdopt, true},
		{"workflow_adopt_author_only", "council_tasks_workflow_adopt", `{"requestId":"%s","expectedRevision":"1","taskVersion":"1","workflow":{"mode":"author_only"}}`, map[string]any{"taskId": id}, validateSchemaAdopt, false},
		{"workflow_missing_authority", "council_tasks_workflow_adopt", `{"requestId":"%s","expectedRevision":"1","taskVersion":"1"}`, map[string]any{"taskId": id}, validateSchemaAdopt, false},
		{"workflow_bad_check", "council_tasks_workflow_adopt", `{"requestId":"%s","expectedRevision":"1","taskVersion":"1","workflow":{"checks":[{"key":"verify","executable":"go","arguments":[],"successExitCodes":[]}]}}`, map[string]any{"taskId": id}, validateSchemaAdopt, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(tc.body, id, id)
			// fmt.Sprintf marks unused arguments; format only the actual placeholders.
			if n := countSchemaPlaceholders(tc.body); n != 2 {
				body = fmt.Sprintf(tc.body, id)
			}
			var input map[string]any
			if err := json.Unmarshal([]byte(body), &input); err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"input": input}
			for k, v := range tc.path {
				args[k] = v
			}
			ownerErr := tc.decode([]byte(body))
			if (ownerErr == nil) != tc.want {
				t.Fatalf("owner validity mismatch: %v", ownerErr)
			}
			schemaErr := reviewedSchema(t, tc.tool).Validate(args)
			if (schemaErr == nil) != tc.want {
				t.Fatalf("schema/owner differ: owner %v schema %v", ownerErr, schemaErr)
			}
		})
	}
}

func countSchemaPlaceholders(s string) int {
	n := 0
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '%' && s[i+1] == 's' {
			n++
		}
	}
	return n
}
func reviewedSchema(t *testing.T, name string) *jsonschema.Resolved {
	t.Helper()
	raw, ok := schemas.DraftLookup(name)
	if !ok {
		t.Fatalf("missing schema %s", name)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
func validateSchemaStart(raw []byte) *domain.Error {
	var x domain.StartTaskInput
	if e := domain.StrictDecode(raw, &x); e != nil {
		return e
	}
	return domain.ValidateStartTask(x)
}
func validateSchemaGuidance(raw []byte) *domain.Error {
	var x domain.GuidanceInput
	if e := domain.StrictDecode(raw, &x); e != nil {
		return e
	}
	return domain.ValidateGuidance(x)
}
func validateSchemaRoutingPolicy(raw []byte) *domain.Error {
	var x domain.RoutingPolicyInput
	if e := domain.StrictDecode(raw, &x); e != nil {
		return e
	}
	return domain.ValidateRoutingPolicyInput(x)
}
func validateSchemaReport(raw []byte) *domain.Error {
	var x domain.CapacityReportInput
	if e := domain.StrictDecode(raw, &x); e != nil {
		return e
	}
	return domain.ValidateCapacityReport(x)
}
func validateSchemaAmendment(raw []byte) *domain.Error {
	var x domain.BudgetAmendmentInput
	if e := domain.StrictDecode(raw, &x); e != nil {
		return e
	}
	return domain.ValidateBudgetAmendment(x)
}
func validateSchemaAdopt(raw []byte) *domain.Error {
	var x domain.WorkflowAdoptInput
	if e := domain.StrictDecode(raw, &x); e != nil {
		return e
	}
	return domain.ValidateWorkflowAdoptInput(x)
}

// The observation predicates use the same owner parsing rules, including the
// separate 100-item page and 65536-byte artifact limits and zero wait/count.
func TestExecutionRoutingWorkflowObservationSchemaBounds(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		tool, key string
		path      map[string]any
		values    []string
		owner     func(string) bool
	}{
		{"council_tasks_list", "limit", nil, []string{"1", "100", "0", "101", "01", "abc"}, func(v string) bool { _, e := domain.ParseLimit(v); return e == nil }},
		{"council_routing_accounts_list", "limit", nil, []string{"1", "100", "0", "101", "01"}, func(v string) bool { _, e := domain.ParseLimit(v); return e == nil }},
		{"council_tasks_workflow_steps", "limit", map[string]any{"taskId": id}, []string{"1", "100", "101", "01"}, func(v string) bool { _, e := domain.ParseLimit(v); return e == nil }},
		{"council_attempts_wait", "afterRevision", map[string]any{"id": id}, []string{"0", "1", "9223372036854775807", "9223372036854775808", "01", "abc"}, func(v string) bool { _, e := domain.ParseCount(v); return e == nil }},
		{"council_attempts_wait", "timeoutMs", map[string]any{"id": id}, []string{"0", "30000", "30001", "01", "abc"}, func(v string) bool {
			n, e := strconv.Atoi(v)
			return e == nil && n >= 0 && n <= 30000 && strconv.Itoa(n) == v
		}},
		{"council_artifacts_bytes", "limit", map[string]any{"id": id}, []string{"1", "100", "65536", "65537", "0", "01"}, func(v string) bool {
			n, e := strconv.Atoi(v)
			return e == nil && n >= 1 && n <= 65536 && strconv.Itoa(n) == v
		}},
		{"council_artifacts_bytes", "offset", map[string]any{"id": id}, []string{"0", "1", "9223372036854775807", "9223372036854775808", "01"}, func(v string) bool { _, e := domain.ParseCount(v); return e == nil }},
	}
	for _, tc := range cases {
		t.Run(tc.tool+"/"+tc.key, func(t *testing.T) {
			schema := reviewedSchema(t, tc.tool)
			for _, v := range tc.values {
				args := map[string]any{tc.key: v}
				for k, p := range tc.path {
					args[k] = p
				}
				err := schema.Validate(args)
				if (err == nil) != tc.owner(v) {
					t.Errorf("value %q: schema=%v owner_valid=%v", v, err, tc.owner(v))
				}
			}
		})
	}
}
