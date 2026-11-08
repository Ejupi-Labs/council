package schemas

import "testing"

func TestOwnerBodyValidationFailsClosed(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name, operation, path, body string
		known, valid                bool
	}{
		{"valid task", "tasks.start", "", `{"requestId":"` + id + `","projectId":"` + id + `","originalText":"Hello"}`, true, true},
		{"unknown field", "tasks.start", "", `{"requestId":"` + id + `","projectId":"` + id + `","originalText":"Hello","extra":true}`, true, false},
		{"duplicate field", "tasks.start", "", `{"requestId":"` + id + `","projectId":"` + id + `","originalText":"Hello","originalText":"Other"}`, true, false},
		{"invalid execution mode", "tasks.start", "", `{"requestId":"` + id + `","projectId":"` + id + `","originalText":"Hello","executionMode":"native","fakeScenario":"slow"}`, true, false},
		{"graph cycle", "graphs.start", "", `{"requestId":"` + id + `","projectId":"` + id + `","nodes":[{"key":"a","originalText":"a"},{"key":"b","originalText":"b"}],"dependencies":[{"from":"a","to":"b"},{"from":"b","to":"a"}]}`, true, false},
		{"mismatched context", "routing.context.update", "not-a-context", `{"requestId":"` + id + `","expectedRevision":"1","enabled":true}`, true, false},
		{"valid context", "routing.context.update", "codex-native", `{"requestId":"` + id + `","expectedRevision":"1","enabled":true}`, true, true},
		{"unknown operation", "arbitrary.http.proxy", "", `{}`, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err, known := ValidateOwnerBody(tc.operation, tc.path, []byte(tc.body))
			if known != tc.known || (known && (err == nil) != tc.valid) {
				t.Fatalf("known=%v error=%v; want known=%v valid=%v", known, err, tc.known, tc.valid)
			}
		})
	}
}
