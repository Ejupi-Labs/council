package domain

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestRoutingWireRejectsExplicitEmptyOptionalValues(t *testing.T) {
	identity := `"requestId":"11111111111111111111111111111111","expectedRevision":"1"`
	start := `"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"Keep the exact task."`
	for _, tc := range []struct {
		name, body string
		target     func() any
	}{
		{"suitability", `{` + start + `,"routing":{"suitability":""}}`, func() any { return &StartTaskInput{} }},
		{"profile", `{` + start + `,"routing":{"targetProfileId":""}}`, func() any { return &StartTaskInput{} }},
		{"policy_revision", `{` + start + `,"routing":{"expectedPolicyRevision":""}}`, func() any { return &StartTaskInput{} }},
		{"initial_count", `{` + start + `,"routing":{"budget":{"admissionLimit":""}}}`, func() any { return &StartTaskInput{} }},
		{"automatic_count", `{` + start + `,"routing":{"continuation":{"maxAutomaticAdmissions":""}}}`, func() any { return &StartTaskInput{} }},
		{"unknown_policy", `{` + identity + `,"unknownAvailability":""}`, func() any { return &RoutingPolicyInput{} }},
		{"default_count", `{` + identity + `,"defaultBudget":{"admissionLimit":""}}`, func() any { return &RoutingPolicyInput{} }},
		{"default_automatic_count", `{` + identity + `,"continuation":{"maxAutomaticAdmissions":""}}`, func() any { return &RoutingPolicyInput{} }},
		{"context_label", `{` + identity + `,"label":""}`, func() any { return &ContextUpdateInput{} }},
		{"account_entitlement", `{` + identity + `,"entitlement":""}`, func() any { return &AccountUpdateInput{} }},
		{"link_unlink_empty_id", `{` + identity + `,"declaredAccountId":"","unlink":true}`, func() any { return &AccountLinkInput{} }},
		{"adoption_empty_budget_revision", `{` + identity + `,"action":"adopt","expectedBudgetRevision":"","additionalAdmissions":"2","newWindowSeconds":1800}`, func() any { return &BudgetAmendmentInput{} }},
		{"adoption_nested_empty", `{` + identity + `,"action":"adopt","additionalAdmissions":"2","newWindowSeconds":1800,"routing":{"suitability":""}}`, func() any { return &BudgetAmendmentInput{} }},
		{"report_observed", `{` + identity + `,"subject":{"kind":"context","id":"codex-native"},"dimension":"occupancy","state":"unknown","observedAt":""}`, func() any { return &CapacityReportInput{} }},
		{"report_retry", `{` + identity + `,"subject":{"kind":"context","id":"codex-native"},"dimension":"service","state":"temporary_throttle","retryAfter":""}`, func() any { return &CapacityReportInput{} }},
		{"report_correction_empty_pair", `{` + identity + `,"subject":{"kind":"context","id":"codex-native"},"dimension":"allowance","state":"available","correctionOf":"","correctionAction":""}`, func() any { return &CapacityReportInput{} }},
		{"reference_basis", `{` + identity + `,"referencePrice":{"currency":"USD","sourceLabel":"User reference","observedAt":"2026-11-06T00:00:00Z","basis":""}}`, func() any { return &ProfileUpdateInput{} }},
		{"reference_expiry", `{` + identity + `,"referencePrice":{"currency":"USD","sourceLabel":"User reference","observedAt":"2026-11-06T00:00:00Z","validUntil":""}}`, func() any { return &ProfileUpdateInput{} }},
		{"missing_subject_kind", `{` + identity + `,"subject":{"id":"codex-native"},"dimension":"occupancy","state":"unknown"}`, func() any { return &CapacityReportInput{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := StrictDecode([]byte(tc.body), tc.target()); e == nil || e.Code != "invalid_request" {
				t.Fatal("wire value disappeared into omission", tc.body, e)
			}
		})
	}
}

func TestRoutingWireKeepsOmissionUnknownFalseAndIntentionalClearing(t *testing.T) {
	var start StartTaskInput
	body := `{"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"Keep \ufeff and \ufffd exactly.","executionMode":"fake","routing":{"suitability":"unknown","budget":{"admissionLimit":"2","windowSeconds":1800},"continuation":{"maxAutomaticAdmissions":"0","categories":[],"allowReplacement":false,"allowEscalation":false}}}`
	if e := StrictDecode([]byte(body), &start); e != nil {
		t.Fatal(e)
	}
	if e := ValidateStartTask(start); e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(start)
	if e != nil || !strings.Contains(string(raw), `"allowReplacement":false`) || !strings.Contains(string(raw), `"allowEscalation":false`) {
		t.Fatal("explicit false was erased", string(raw), e)
	}
	var account AccountUpdateInput
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","providerLabel":""}`), &account); e != nil {
		t.Fatal(e)
	}
	if account.ProviderLabel == nil || *account.ProviderLabel != "" || ValidateAccountUpdate("22222222222222222222222222222222", account) != nil {
		t.Fatal("provider-label clearing rejected", account)
	}
	var report CapacityReportInput
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","subject":{"kind":"context","id":"codex-native"},"dimension":"occupancy","state":"unknown","note":""}`), &report); e != nil {
		t.Fatal(e)
	}
	if e := ValidateCapacityReport(report); e != nil {
		t.Fatal(e)
	}
	var guidance GuidanceInput
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","guidanceText":"","routing":{"useCurrentPolicy":false}}`), &guidance); e != nil {
		t.Fatal(e)
	}
	if e := ValidateGuidance(guidance); e != nil {
		t.Fatal(e)
	}
	var policy RoutingPolicyInput
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1"}`), &policy); e != nil {
		t.Fatal("omitted options rejected", e)
	}
	if e := ValidateRoutingPolicyInput(policy); e != nil {
		t.Fatal(e)
	}
}

func TestRoutingReferencePriceStandaloneSharesNestedPresenceRules(t *testing.T) {
	base := `"currency":"USD","sourceLabel":"User declaration","observedAt":"2026-11-06T00:00:00Z"`
	for _, field := range []string{"unit", "basis", "sourceUrl", "validUntil"} {
		t.Run(field, func(t *testing.T) {
			raw := `{` + base + `,"` + field + `":""}`
			var standalone ReferencePriceInput
			if e := StrictDecode([]byte(raw), &standalone); e == nil || e.Code != "invalid_request" {
				t.Fatal("standalone empty price member was erased", field, e)
			}
			var nested ProfileUpdateInput
			body := `{"requestId":"11111111111111111111111111111111","expectedRevision":"1","referencePrice":` + raw + `}`
			if e := StrictDecode([]byte(body), &nested); e == nil || e.Code != "invalid_request" {
				t.Fatal("nested price presence differs", field, e)
			}
		})
	}
	for _, missing := range []string{`{"sourceLabel":"User declaration","observedAt":"2026-11-06T00:00:00Z"}`, `{"currency":"USD","observedAt":"2026-11-06T00:00:00Z"}`, `{"currency":"USD","sourceLabel":"User declaration"}`} {
		var standalone ReferencePriceInput
		if e := StrictDecode([]byte(missing), &standalone); e == nil {
			t.Fatal("standalone required price member defaulted", missing)
		}
	}
	var valid ReferencePriceInput
	if e := StrictDecode([]byte(`{`+base+`,"inputPerMillion":"1e999"}`), &valid); e != nil {
		t.Fatal("omitted unknown reference members were rejected", e)
	}
	if e := ValidateReferencePrice(&valid); e != nil || valid.InputPerMillion == nil || *valid.InputPerMillion != "1e999" {
		t.Fatal("exact extreme reference was changed", valid, e)
	}
}
