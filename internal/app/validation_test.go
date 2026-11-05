package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestStrictJSON(t *testing.T) {
	for _, body := range []string{`{"requestId":"a","mode":"autonomous","mode":"supervised"}`, `{"mode":"\ud800"}`, `{"unknown":true}`, `{"mode":42}`, `{"mode":"autonomous"} {}`, `[]`, string([]byte{'{', '"', 'm', 'o', 'd', 'e', '"', ':', '"', 0xff, '"', '}'})} {
		var value policyRequest
		if e := StrictDecode([]byte(body), &value); e == nil {
			t.Errorf("accepted invalid JSON %q", body)
		}
	}
	var value createDraftRequest
	body := `{"requestId":"0123456789abcdef0123456789abcdef","projectId":"0123456789abcdef0123456789abcdef","originalText":"\ufeff\ufffd A\r\n B\ufeff "}`
	if e := StrictDecode([]byte(body), &value); e != nil {
		t.Fatal(e)
	}
	if value.OriginalText != "\ufeff\ufffd A\r\n B\ufeff " {
		t.Fatalf("changed text %q", value.OriginalText)
	}
}
func TestExactRevisionAndTextBounds(t *testing.T) {
	for _, s := range []string{"1", "9007199254740993", "9223372036854775807"} {
		if _, e := ParseRevision(s); e != nil {
			t.Fatal(s, e)
		}
	}
	for _, s := range []string{"", "0", "01", "-1", "+1", "1.0", "1e2", " 1", "9223372036854775808"} {
		if _, e := ParseRevision(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
	if e := domain.ValidateText(strings.Repeat("a", domain.MaxTextBytes)); e != nil {
		t.Fatal(e)
	}
	if e := domain.ValidateText(strings.Repeat("a", domain.MaxTextBytes+1)); e == nil || e.Code != "input_too_large" {
		t.Fatal(e)
	}
	for _, text := range []string{"", " \r\n\u2003\u00a0"} {
		if e := domain.ValidateText(text); e == nil {
			t.Fatal("accepted whitespace")
		}
	}
	if e := domain.ValidateText("\ufeff"); e != nil {
		t.Fatal("BOM is a scalar, not trimmed", e)
	}
}

func TestStrictJSONRequiredTypesAndIndependentBodyCap(t *testing.T) {
	for _, raw := range []string{
		"{\"requestId\":null}",
		"{\"expectedRevision\":1}",
		"{\"mode\":true}",
		"{\"mode\":\"autonomous\",\"Mode\":\"supervised\"}",
		"{\"mode\":\"\\udfff\"}",
	} {
		var input policyRequest
		if e := StrictDecode([]byte(raw), &input); e == nil {
			t.Fatal("accepted wrong member type", raw)
		}
	}
	var input createDraftRequest
	if e := StrictDecode([]byte("{\"originalText\":\"\\ud83d\\ude00\"}"), &input); e != nil || input.OriginalText != "😀" {
		t.Fatal(input, e)
	}
	body := append([]byte("{\"originalText\":\"x\"}"), []byte(strings.Repeat(" ", domain.MaxBodyBytes))...)
	if e := StrictDecode(body, &input); e == nil || e.Code != "input_too_large" {
		t.Fatal(e)
	}
}
func TestExplicitInvalidListQueryIsRejected(t *testing.T) {
	s := testService(t)
	for _, op := range []string{"projects.list", "drafts.list"} {
		for _, query := range []map[string]string{{"limit": ""}, {"cursor": ""}, {"limit": "101"}, {"limit": "9223372036854775808"}, {"extra": "x"}} {
			r := s.Execute(context.Background(), op, Params{Query: query})
			if r.Status != 400 {
				t.Fatal(op, query, r)
			}
		}
	}
}
