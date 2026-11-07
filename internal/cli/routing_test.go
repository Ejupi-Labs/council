package cli

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func TestRoutingCLIContractOperationsAndCurrentOwnerGate(t *testing.T) {
	id := strings.Repeat("1", 32)
	for _, tc := range []struct {
		args              []string
		op, method, route string
	}{
		{[]string{"routing", "policy"}, "routing.policy.get", "GET", "/routing/policy"},
		{[]string{"routing", "policy", "set", "--expected-revision", "1", "--prefer-profile", "claude-haiku-economy", "--unknown-availability", "require_positive"}, "routing.policy.update", "PUT", "/routing/policy"},
		{[]string{"routing", "contexts"}, "routing.contexts.list", "GET", "/routing/contexts"},
		{[]string{"routing", "context", "set", "claude-native", "--expected-revision", "1", "--enabled", "false"}, "routing.context.update", "PUT", "/routing/contexts/claude-native"},
		{[]string{"routing", "profiles", "--context", "codex-native"}, "routing.profiles.list", "GET", "/routing/profiles"},
		{[]string{"routing", "profile", "set", "codex-luna-economy", "--expected-revision", "1", "--clear-reference-price", "true"}, "routing.profile.update", "PUT", "/routing/profiles/codex-luna-economy"},
		{[]string{"routing", "accounts", "--limit", "100"}, "routing.accounts.list", "GET", "/routing/accounts"},
		{[]string{"routing", "account", "add", "--label", "Local subscription", "--provider", "Anthropic", "--entitlement", "subscription"}, "routing.accounts.create", "POST", "/routing/accounts"},
		{[]string{"routing", "account", "set", id, "--expected-revision", "1", "--label", "Renamed"}, "routing.accounts.update", "PUT", "/routing/accounts/" + id},
		{[]string{"routing", "link", "--context", "claude-native", "--declared-account", id, "--expected-revision", "1"}, "routing.links.update", "PUT", "/routing/contexts/claude-native/account-link"},
		{[]string{"routing", "capacity", "--account", id}, "routing.capacity.get", "GET", "/routing/capacity"},
		{[]string{"routing", "reports", "--context", "codex-native", "--limit", "30"}, "routing.reports.list", "GET", "/routing/reports"},
		{[]string{"routing", "report", "get", id}, "routing.reports.get", "GET", "/routing/reports/" + id},
		{[]string{"routing", "report", "--context", "codex-native", "--expected-revision", "1", "--dimension", "occupancy", "--state", "busy"}, "routing.reports.create", "POST", "/routing/reports"},
		{[]string{"tasks", "routing", id}, "tasks.routing.get", "GET", "/tasks/" + id + "/routing"},
		{[]string{"tasks", "routing-decisions", id}, "tasks.routing.decisions.list", "GET", "/tasks/" + id + "/routing/decisions"},
		{[]string{"tasks", "budget", id}, "tasks.budget.get", "GET", "/tasks/" + id + "/budget"},
		{[]string{"tasks", "budget-ledger", id}, "tasks.budget.ledger.list", "GET", "/tasks/" + id + "/budget/ledger"},
		{[]string{"tasks", "usage", id}, "tasks.usage.list", "GET", "/tasks/" + id + "/usage"},
		{[]string{"tasks", "budget", "grant", id, "--expected-revision", "1", "--expected-budget-revision", "2", "--additional-admissions", "1"}, "tasks.budget.amend", "POST", "/tasks/" + id + "/budget-amendments"},
		{[]string{"tasks", "adopt", id, "--expected-revision", "1", "--additional-admissions", "2", "--window-seconds", "1800"}, "tasks.budget.amend", "POST", "/tasks/" + id + "/budget-amendments"},
	} {
		t.Run(strings.Join(tc.args[:min(3, len(tc.args))], "_"), func(t *testing.T) {
			c, err := parse(tc.args, nil)
			if err != nil {
				t.Fatal(err)
			}
			if c.op != tc.op || c.method != tc.method || c.route != tc.route || !requiresExecutionOwner(c.op) {
				t.Fatalf("wrong owner dispatch %+v", c)
			}
			if c.method == "GET" {
				if c.requestID != "" || len(c.params.Body) != 0 {
					t.Fatal("observation mutated state")
				}
			} else if !domain.ValidID(c.requestID) {
				t.Fatal("missing retry identity")
			}
		})
	}
}

func TestRoutingCLIExactOptionsFalseAndOmittedRemainDistinct(t *testing.T) {
	id := strings.Repeat("1", 32)
	args := []string{"tasks", "start", "--project", id, "--task", "\ufeffExact café\r\n東京", "--suitability", "routine", "--target-profile", "claude-haiku-economy", "--model", "haiku", "--expected-routing-policy-revision", "9223372036854775807", "--budget-admissions", "2", "--budget-window-seconds", "1800", "--max-automatic-admissions", "1", "--allow-continuation", "pre_launch_ineligible", "--allow-replacement", "false", "--allow-escalation", "false", "--fake"}
	c, err := parse(args, nil)
	if err != nil {
		t.Fatal(err)
	}
	var in domain.StartTaskInput
	if e := domain.StrictDecode(c.params.Body, &in); e != nil {
		t.Fatal(e)
	}
	if in.OriginalText != "\ufeffExact café\r\n東京" || in.Routing == nil || in.Routing.Continuation == nil || in.Routing.Continuation.AllowReplacement == nil || *in.Routing.Continuation.AllowReplacement || in.Routing.Budget.AdmissionLimit != "2" || in.Routing.ExpectedPolicyRevision != "9223372036854775807" || in.Pins.Model != "haiku" {
		t.Fatalf("intent changed %+v", in)
	}
	c, err = parse([]string{"tasks", "start", "--project", id, "--task", "minimal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e := domain.StrictDecode(c.params.Body, &in); e != nil {
		t.Fatal(e)
	}
	// Decode a new value: absent fields must remain absent in the serialized retry intent.
	if strings.Contains(string(c.params.Body), `"routing"`) {
		t.Fatal("injected mutable routing defaults")
	}
	c, err = parse([]string{"tasks", "guidance", id, "--expected-revision", "1", "--guidance", "", "--use-current-routing-policy", "false"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var guidance domain.GuidanceInput
	if e := domain.StrictDecode(c.params.Body, &guidance); e != nil {
		t.Fatal(e)
	}
	if guidance.Routing == nil || guidance.Routing.UseCurrentPolicy == nil || *guidance.Routing.UseCurrentPolicy {
		t.Fatal("explicit false lost")
	}
	c, err = parse([]string{"routing", "context", "set", "codex-native", "--expected-revision", "1", "--enabled", "false"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var update domain.ContextUpdateInput
	if e := domain.StrictDecode(c.params.Body, &update); e != nil || update.Enabled == nil || *update.Enabled {
		t.Fatalf("false became omission %s", c.params.Body)
	}
}

func TestRoutingCLIAllowsExplicitTextClearingButRejectsEmptyRoutingChoices(t *testing.T) {
	id := strings.Repeat("1", 32)
	c, err := parse([]string{"routing", "account", "set", id, "--expected-revision", "1", "--provider", ""}, nil)
	if err != nil {
		t.Fatal("the CLI cannot clear a declared provider label", err)
	}
	var account domain.AccountUpdateInput
	if e := domain.StrictDecode(c.params.Body, &account); e != nil || account.ProviderLabel == nil || *account.ProviderLabel != "" {
		t.Fatal("provider clearing became omission", string(c.params.Body), e)
	}
	if _, err = parse([]string{"routing", "report", "--context", "codex-native", "--dimension", "occupancy", "--state", "busy", "--expected-revision", "1", "--note", ""}, nil); err != nil {
		t.Fatal("an explicitly empty report note is valid text", err)
	}
	for _, key := range []string{"suitability", "target-profile", "expected-routing-policy-revision", "budget-admissions"} {
		if _, err = parse([]string{"tasks", "start", "--project", id, "--task", "bounded local task", "--" + key, ""}, nil); err == nil {
			t.Fatal("an explicit empty routing choice disappeared into defaults", key)
		}
	}
}

func TestRoutingCLIRejectsAmbiguityBeforeContactAndStrictPriceObject(t *testing.T) {
	id := strings.Repeat("1", 32)
	invalid := [][]string{
		{"routing", "context", "set", "codex-native", "--expected-revision", "1", "--enabled"},
		{"routing", "context", "set", "codex-native", "--expected-revision", "1", "--enabled", "0"},
		{"routing", "link", "--context", "codex-native", "--declared-account", id, "--unlink", "true", "--expected-revision", "1"},
		{"routing", "link", "--context", "codex-native", "--unlink", "false", "--expected-revision", "1"},
		{"routing", "capacity", "--context", "codex-native", "--account", id},
		{"routing", "capacity"},
		{"routing", "report", "get", id, "--state", "busy"},
		{"routing", "policy", "set", "--expected-revision", "1", "--prefer-profile", "codex-native-default", "--prefer-profile", "codex-native-default"},
		{"tasks", "start", "--project", id, "--task", "x", "--allow-continuation", "pre_launch_ineligible", "--allow-continuation", "pre_launch_ineligible"},
		{"tasks", "guidance", id, "--expected-revision", "1", "--guidance", "x", "--budget-admissions", "1"},
		{"tasks", "guidance", id, "--expected-revision", "1", "--guidance", "x", "--expected-routing-policy-revision", "1"},
		{"tasks", "run", id, "--expected-revision", "1", "--task-version", "1", "--suitability", "routine"},
		{"tasks", "adopt", id, "--expected-revision", "1", "--additional-admissions", "1"},
		{"tasks", "budget", "grant", id, "--expected-revision", "1", "--additional-admissions", "1"},
		{"tasks", "usage", id, "--limit", "101"},
		{"routing", "reports", "--context", "codex-native", "--cursor", "東京"},
		{"routing", "profile", "set", "claude-haiku-economy", "--expected-revision", "1", "--clear-reference-price", "false"},
	}
	for _, args := range invalid {
		if _, err := parse(args, nil); err == nil {
			t.Errorf("ambiguous command accepted: %v", args)
		}
	}
	base := []string{"routing", "profile", "set", "claude-haiku-economy", "--expected-revision", "1", "--reference-price-file", "-"}
	price := `{"currency":"USD","sourceLabel":"dated declaration","observedAt":"2026-11-06T00:00:00.000Z","inputPerMillion":"1e999"}`
	c, err := parse(base, strings.NewReader(price))
	if err != nil {
		t.Fatal(err)
	}
	var in domain.ProfileUpdateInput
	if e := domain.StrictDecode(c.params.Body, &in); e != nil || in.ReferencePrice == nil || *in.ReferencePrice.InputPerMillion != "1e999" || in.ReferencePrice.Basis != "" {
		t.Fatalf("price declaration changed %s", c.params.Body)
	}
	for _, raw := range []string{`{"currency":"USD","currency":"EUR"}`, `{"currency":"USD","sourceKind":"native"}`, `null`, price + ` {}`} {
		if _, err := parse(base, strings.NewReader(raw)); err == nil {
			t.Errorf("invalid price object accepted %s", raw)
		}
	}
}

func TestKnown003MaintenanceCannotGrant004ExecutionOrRouting(t *testing.T) {
	id := domain.NewID()
	metadata := metadataFixture(id, false)
	metadata["appVersion"], metadata["executionContract"] = "2.0.0-dev.3", "003-v1"
	metadata["capabilities"] = []string{"workspace_foundation", "managed_execution"}
	delete(metadata, "graphContract")
	delete(metadata, "recoveryContract")
	delete(metadata, "workflowContract")
	metadata["capacity"] = map[string]any{"resourceId": "managed-native", "limit": 1, "activeAttemptId": nil}
	body, _ := json.Marshal(domain.Success(200, domain.NewID(), metadata, nil).Envelope)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	desc := localstate.Descriptor{OwnerInstanceID: id, Origin: server.URL, Token: "fixture"}
	if !verifyMaintenanceOwner(context.Background(), desc) || verifyExecutionOwner(context.Background(), desc) {
		t.Fatal("known003 maintenance incorrectly granted004 execution")
	}
	metadata["appVersion"], metadata["executionContract"] = domain.AppVersion, domain.ExecutionContract
	body, _ = json.Marshal(domain.Success(200, domain.NewID(), metadata, nil).Envelope)
	if verifyExecutionOwner(context.Background(), desc) {
		t.Fatal("missing economical routing capability was accepted")
	}
}
