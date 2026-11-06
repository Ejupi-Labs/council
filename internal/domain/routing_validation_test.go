package domain

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestRoutingStrictRequestsAndExplicitFalse(t *testing.T) {
	for _, body := range []string{
		`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","enabled":null}`,
		`{"requestId":"11111111111111111111111111111111","expectedRevision":9007199254740993,"enabled":false}`,
		`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","enabled":true,"enabled":false}`,
		`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","executable":"vendor"}`,
	} {
		var in ContextUpdateInput
		if e := StrictDecode([]byte(body), &in); e == nil {
			t.Fatal("invalid routing request accepted", body)
		}
	}
	var in ContextUpdateInput
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","enabled":false}`), &in); e != nil || in.Enabled == nil || *in.Enabled {
		t.Fatal(in, e)
	}
	raw, err := json.Marshal(in)
	if err != nil || !strings.Contains(string(raw), `"enabled":false`) {
		t.Fatal("explicit false lost", string(raw), err)
	}
	for _, v := range []string{"", "0", "00", "9223372036854775808"} {
		in.ExpectedRevision = v
		if e := ValidateContextUpdate("codex-native", in); e == nil {
			t.Fatal("invalid required revision accepted", v)
		}
	}
}

func TestRoutingExactArithmeticAndDecimalStorageBounds(t *testing.T) {
	if n, e := AddRoutingCounts("9007199254740993", "2"); e != nil || n != "9007199254740995" {
		t.Fatal(n, e)
	}
	if _, e := AddRoutingCounts("9223372036854775807", "1"); e == nil {
		t.Fatal("overflow admitted")
	}
	if _, e := SubtractRoutingCounts("1", "2"); e == nil {
		t.Fatal("negative budget admitted")
	}
	for _, v := range []string{"0", "0.0001", "1e999", strings.Repeat("9", 64)} {
		if !ValidRoutingDecimal(v) {
			t.Fatal("valid bounded exact lexeme lost", v)
		}
	}
	for _, v := range []string{"-0", "NaN", "+1", "01", "1 ", strings.Repeat("9", 65)} {
		if ValidRoutingDecimal(v) {
			t.Fatal("invalid estimate/reference lexeme accepted", v)
		}
	}
}

func TestRoutingOptionsCategoryAndBudgetBoundaries(t *testing.T) {
	for _, n := range []string{"0", "17", "9007199254740993"} {
		if e := ValidateRoutingOptions(&RoutingOptions{Budget: &RoutingBudgetInput{AdmissionLimit: n}}, "start"); e == nil {
			t.Fatal("unbounded initial grant", n)
		}
	}
	n := 0
	if e := ValidateRoutingOptions(&RoutingOptions{Budget: &RoutingBudgetInput{WindowSeconds: &n}}, "start"); e == nil {
		t.Fatal("zero window")
	}
	n = 1800
	if e := ValidateRoutingOptions(&RoutingOptions{Budget: &RoutingBudgetInput{WindowSeconds: &n}}, "guidance"); e == nil {
		t.Fatal("guidance refilled budget")
	}
	categories := []string{"terminal_transient"}
	policy := ContinuationPolicy{MaxAutomaticAdmissions: "1", Categories: categories}
	if e := CheckContinuationCapability(policy); e == nil || e.Code != "capability_unavailable" {
		t.Fatal("unsupported B falsely enabled", e)
	}
	categories = []string{"from_peer_prose"}
	if e := ValidateContinuationInput(&ContinuationInput{Categories: &categories}); e == nil {
		t.Fatal("prose category invented")
	}
	if e := ValidateRoutingOptions(&RoutingOptions{TargetProfileID: "invented-profile"}, "start"); e == nil {
		t.Fatal("unknown profile")
	}
}
