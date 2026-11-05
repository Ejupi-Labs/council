package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type fakeExecutor struct{ calls int }

type panicExecutor struct{}

func (panicExecutor) Execute(context.Context, string, app.Params) domain.Result {
	panic("simulated post-commit acknowledgement failure")
}

func TestMutationPanicPreservesRetryIdentity(t *testing.T) {
	id := "11111111111111111111111111111111"
	h := New(panicExecutor{}, Config{Host: "127.0.0.1:12345", Token: "secret"})
	r := httptest.NewRequest("POST", "http://127.0.0.1:12345/api/v1/drafts", strings.NewReader(`{"requestId":"`+id+`"}`))
	r.Header.Set("Authorization", "Bearer secret")
	r.Header.Set("Origin", "http://127.0.0.1:12345")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"write_uncertain"`) || !strings.Contains(w.Body.String(), id) || strings.Contains(w.Body.String(), "simulated") {
		t.Fatalf("lost mutation identity: %s", w.Body.String())
	}
}

func (f *fakeExecutor) Execute(_ context.Context, _ string, _ app.Params) domain.Result {
	f.calls++
	return domain.Success(200, domain.NewID(), map[string]string{"status": "ok"}, nil)
}

func TestAuthorityBeforePrivateDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, method, route, host, origin, token string
		status                                   int
	}{
		{"private read", "GET", "/api/v1/projects", "127.0.0.1:12345", "", "", 401},
		{"foreign host", "GET", "/api/v1/projects", "evil.example", "", "secret", 403},
		{"foreign read origin", "GET", "/api/v1/projects", "127.0.0.1:12345", "https://evil.example", "secret", 403},
		{"null mutation", "POST", "/api/v1/drafts", "127.0.0.1:12345", "null", "secret", 403},
		{"missing mutation origin", "POST", "/api/v1/drafts", "127.0.0.1:12345", "", "secret", 403},
		{"read", "GET", "/api/v1/projects", "127.0.0.1:12345", "", "secret", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeExecutor{}
			h := New(service, Config{Host: "127.0.0.1:12345", Token: "secret", OwnerInstanceID: domain.NewID()})
			r := httptest.NewRequest(tc.method, "http://127.0.0.1:12345"+tc.route, strings.NewReader("{}"))
			r.Host = tc.host
			if tc.method == "GET" {
				r = httptest.NewRequest(tc.method, "http://127.0.0.1:12345"+tc.route, nil)
				r.Host = tc.host
			}
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status != 200 && service.calls != 0 {
				t.Fatal("unauthorized request reached core")
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("token disclosed")
			}
		})
	}
}

func TestTransportRejectsAmbiguousRequests(t *testing.T) {
	for _, tc := range []struct {
		method, route, body, content string
		status                       int
	}{
		{"GET", "/api/v1/projects?limit=1&limit=2", "", "", 400},
		{"GET", "/api/v1/projects?unknown=x", "", "", 400},
		{"GET", "/api/v1/projects?limit=", "", "", 400},
		{"GET", "/api/v1/projects", "{}", "", 400},
		{"POST", "/api/v1/drafts", "{}", "text/plain", 415},
		{"POST", "/api/v1/drafts", "{}", "application/json; foo=bar", 415},
		{"POST", "/api/v1/drafts", "{\"requestId\":\"a\",\"requestId\":\"b\"}", "application/json", 400},
		{"POST", "/api/v1/drafts", "{\"x\":\"\\ud800\"}", "application/json", 400},
		{"POST", "/api/v1/drafts", "{} {}", "application/json", 400},
		{"DELETE", "/api/v1/drafts", "", "", 405},
		{"GET", "/api/v1/unknown", "", "", 404},
		{"GET", "/api/v1/projects%2fno", "", "", 400},
	} {
		service := &fakeExecutor{}
		h := New(service, Config{Host: "127.0.0.1:12345", Token: "secret"})
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:12345"+tc.route, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer secret")
		r.Header.Set("Origin", "http://127.0.0.1:12345")
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d expected%d %s", tc.method, tc.route, w.Code, tc.status, w.Body.String())
		}
		if service.calls != 0 {
			t.Errorf("invalid request reached core: %s", tc.route)
		}
	}
}
