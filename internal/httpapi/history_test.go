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

type historyHTTPSpy struct{ calls int }

func (s *historyHTTPSpy) Execute(_ context.Context, _ string, _ app.Params) domain.Result {
	s.calls++
	return domain.Success(200, domain.NewID(), "unexpected", nil)
}
func TestHistoryRoutesAndQuery(t *testing.T) {
	id := strings.Repeat("1", 32)
	for _, tc := range []struct {
		path, method, op, id, allowed string
		n                             int
	}{
		{"/api/v1/history/snapshots", "POST", "history.snapshots.create", "", "POST", 0},
		{"/api/v1/history/snapshots/" + id, "GET", "history.snapshots.get", id, "GET", 0},
		{"/api/v1/history/snapshots/" + id + "/records", "GET", "history.snapshots.records", id, "GET", 3},
		{"/api/v1/history/exports/preview", "POST", "history.exports.preview", "", "POST", 0},
		{"/api/v1/history/exports", "POST", "history.exports.create", "", "POST", 0},
		{"/api/v1/history/operations/" + id, "GET", "history.operations.get", id, "GET", 0},
		{"/api/v1/history/operations/" + id + "/reconciliations", "POST", "history.operations.reconcile", id, "POST", 0},
	} {
		op, got, allowed, q := route(tc.path, tc.method)
		if op != tc.op || got != tc.id || allowed != tc.allowed || len(q) != tc.n {
			t.Fatal(tc, op, got, allowed, q)
		}
	}
	for _, p := range []string{"/api/v1/history/snapshots/", "/api/v1/history/operations//reconciliations", "/api/v1/history/exports/preview/extra"} {
		if op, _, _, _ := route(p, "POST"); op != "" {
			t.Fatal("malformed route", p, op)
		}
	}
}
func TestHistoryHTTPRejectsInvalidRequestsBeforeService(t *testing.T) {
	id := strings.Repeat("1", 32)
	spy := &historyHTTPSpy{}
	h := New(spy, Config{Host: "127.0.0.1:7001", Token: "fixture"})
	for _, tc := range []struct{ method, path, token, origin, body string }{
		{"POST", "/api/v1/history/snapshots", "bad", "http://127.0.0.1:7001", `{}`},
		{"POST", "/api/v1/history/exports", "fixture", "http://elsewhere", `{}`},
		{"GET", "/api/v1/history/snapshots/" + id + "/records?kind=tasks&kind=attempts", "fixture", "", ""},
		{"GET", "/api/v1/history/operations/" + id + "/reconciliations", "fixture", "", ""},
		{"POST", "/api/v1/history/snapshots", "fixture", "http://127.0.0.1:7001", `{"padding":"` + strings.Repeat("a", domain.HistoryMaxRequestBytes) + `"}`},
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
		h.ServeHTTP(out, req)
		var env domain.Envelope
		if err := json.Unmarshal(out.Body.Bytes(), &env); err != nil || env.Error == nil || spy.calls != 0 {
			t.Fatal("invalid transport reached service", tc.path, env, err)
		}
	}
}
