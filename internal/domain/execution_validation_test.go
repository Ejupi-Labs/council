package domain

import "testing"

func TestExecutionStrictPresenceAndNestedNull(t *testing.T) {
	for _, raw := range []string{`{"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"text","pins":{"model":null}}`, `{"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"text","pins":{"model":""}}`, `{"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"text","acceptance":{"requiredEvidence":[null]}}`, `{"requestId":"11111111111111111111111111111111","projectId":"22222222222222222222222222222222","originalText":"text","expectedPolicyRevision":""}`} {
		var in StartTaskInput
		if e := StrictDecode([]byte(raw), &in); e == nil {
			t.Fatal("accepted invalid nested member", raw)
		}
	}
	var in GuidanceInput
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1"}`), &in); e == nil {
		t.Fatal("accepted missing mandatory guidanceText")
	}
	if e := StrictDecode([]byte(`{"requestId":"11111111111111111111111111111111","expectedRevision":"1","guidanceText":""}`), &in); e != nil {
		t.Fatal(e)
	}
}
