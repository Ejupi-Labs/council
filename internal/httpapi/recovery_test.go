package httpapi

import (
	"context"
	json "encoding/json/v2"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRecoveryRoutesSelectExactIdentitiesAndMethods(t *testing.T) {
	id := strings.Repeat("1", 32)
	for _, tc := range []struct{ path, method, op, allowed string }{
		{"/api/v1/tasks/" + id + "/pause", "POST", "tasks.pause", "POST"},
		{"/api/v1/graphs/" + id + "/pause", "POST", "graphs.pause", "POST"},
		{"/api/v1/checkpoints/" + id, "GET", "checkpoints.get", "GET"},
		{"/api/v1/checkpoints/" + id + "/resume", "POST", "checkpoints.resume", "POST"},
		{"/api/v1/incidents/" + id, "GET", "incidents.get", "GET"},
		{"/api/v1/incidents/" + id + "/reconciliations", "POST", "incidents.reconcile", "POST"},
	} {
		op, gotID, allowed, query := route(tc.path, tc.method)
		if op != tc.op || gotID != id || allowed != tc.allowed || len(query) != 0 {
			t.Fatalf("wrong recovery route: %s %s %s %v", op, gotID, allowed, query)
		}
	}
	for _, path := range []string{"/api/v1/checkpoints/", "/api/v1/incidents//reconciliations", "/api/v1/tasks//pause", "/api/v1/graphs/" + id + "/pause/extra"} {
		if op, _, _, _ := route(path, "POST"); op != "" {
			t.Fatalf("malformed path selected operation %s: %s", op, path)
		}
	}
}

type recoveryHTTPSpy struct{ calls int }

func (s *recoveryHTTPSpy) Execute(_ context.Context, _ string, _ app.Params) domain.Result {
	s.calls++
	return domain.Success(200, domain.NewID(), "unexpected", nil)
}

func TestRecoveryHTTPRejectsUnauthenticatedAndInvalidRequests(t *testing.T) {
	id := strings.Repeat("1", 32)
	spy := &recoveryHTTPSpy{}
	handler := New(spy, Config{Host: "127.0.0.1:7001", Token: "controlled-recovery-test"})
	for _, tc := range []struct{ method, path, token, origin, body string }{
		{"POST", "/api/v1/tasks/" + id + "/pause", "wrong", "http://127.0.0.1:7001", `{}`},
		{"POST", "/api/v1/graphs/" + id + "/pause", "controlled-recovery-test", "http://elsewhere", `{}`},
		{"GET", "/api/v1/checkpoints/" + id + "?extra=yes", "controlled-recovery-test", "", ""},
		{"GET", "/api/v1/incidents/" + id + "/reconciliations", "controlled-recovery-test", "", ""},
	} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1:7001"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.method == "POST" {
			req.Header.Set("Content-Type", "application/json")
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		var envelope domain.Envelope
		if err := json.Unmarshal(out.Body.Bytes(), &envelope); err != nil || envelope.Error == nil || spy.calls != 0 {
			t.Fatalf("invalid request reached service: %s %s %#v %v", tc.method, tc.path, envelope, err)
		}
	}
}
