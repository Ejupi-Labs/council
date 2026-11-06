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

type routingDispatchCapture struct {
	op    string
	args  app.Params
	calls int
}

func (c *routingDispatchCapture) Execute(_ context.Context, op string, args app.Params) domain.Result {
	c.op, c.args = op, args
	c.calls++
	return domain.Success(200, domain.NewID(), map[string]string{"operation": op}, nil)
}

func TestRoutingHTTPDispatchesNestedHistoryAndSeparateQueryScopes(t *testing.T) {
	id := domain.NewID()
	for _, tc := range []struct {
		method, route, op, identity string
	}{
		{"GET", "/routing/policy", "routing.policy.get", ""},
		{"PUT", "/routing/policy", "routing.policy.update", ""},
		{"GET", "/routing/contexts", "routing.contexts.list", ""},
		{"PUT", "/routing/contexts/claude-native", "routing.context.update", "claude-native"},
		{"PUT", "/routing/contexts/codex-native/account-link", "routing.links.update", "codex-native"},
		{"GET", "/routing/profiles?contextId=claude-native", "routing.profiles.list", ""},
		{"PUT", "/routing/profiles/claude-haiku-economy", "routing.profile.update", "claude-haiku-economy"},
		{"GET", "/routing/accounts?limit=1", "routing.accounts.list", ""},
		{"POST", "/routing/accounts", "routing.accounts.create", ""},
		{"PUT", "/routing/accounts/" + id, "routing.accounts.update", id},
		{"GET", "/routing/capacity?contextId=codex-native", "routing.capacity.get", ""},
		{"GET", "/routing/reports?accountId=" + id + "&limit=1", "routing.reports.list", ""},
		{"GET", "/routing/reports/" + id, "routing.reports.get", id},
		{"POST", "/routing/reports", "routing.reports.create", ""},
		{"GET", "/tasks/" + id + "/routing", "tasks.routing.get", id},
		{"GET", "/tasks/" + id + "/routing/decisions?limit=1", "tasks.routing.decisions.list", id},
		{"GET", "/tasks/" + id + "/budget", "tasks.budget.get", id},
		{"GET", "/tasks/" + id + "/budget/ledger?limit=1", "tasks.budget.ledger.list", id},
		{"GET", "/tasks/" + id + "/usage?limit=1", "tasks.usage.list", id},
		{"POST", "/tasks/" + id + "/budget-amendments", "tasks.budget.amend", id},
	} {
		t.Run(tc.method+tc.route, func(t *testing.T) {
			capture := &routingDispatchCapture{}
			h := New(capture, Config{Host: "127.0.0.1:12345", Token: "routing-transport-token", OwnerInstanceID: domain.NewID()})
			var body *strings.Reader
			if tc.method == "GET" {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader(`{"requestId":"` + domain.NewID() + `"}`)
			}
			r := httptest.NewRequest(tc.method, "http://127.0.0.1:12345/api/v1"+tc.route, body)
			r.Header.Set("Authorization", "Bearer routing-transport-token")
			r.Header.Set("Origin", "http://127.0.0.1:12345")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 || capture.calls != 1 || capture.op != tc.op || capture.args.ID != tc.identity {
				t.Fatalf("Documented routing request did not reach its intended operation: %d %s %+v", w.Code, w.Body.String(), capture)
			}
			if strings.Contains(tc.route, "limit=1") && capture.args.Query["limit"] != "1" {
				t.Fatal("History limit was not forwarded")
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("Routing reads lost private response controls")
			}
		})
	}
}

func TestRoutingHTTPRejectsAuthorityMethodsAndAmbiguousQueriesBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		method, route, host, origin, token, body string
		status                                   int
	}{
		{"GET", "/routing/contexts", "127.0.0.1:12345", "", "", "", 401},
		{"GET", "/routing/contexts", "foreign.invalid", "", "routing-transport-token", "", 403},
		{"GET", "/routing/accounts", "127.0.0.1:12345", "https://foreign.invalid", "routing-transport-token", "", 403},
		{"PUT", "/routing/policy", "127.0.0.1:12345", "", "routing-transport-token", "{}", 403},
		{"DELETE", "/routing/accounts", "127.0.0.1:12345", "http://127.0.0.1:12345", "routing-transport-token", "{}", 405},
		{"GET", "/routing/contexts?limit=1", "127.0.0.1:12345", "", "routing-transport-token", "", 400},
		{"GET", "/routing/accounts?limit=1&limit=2", "127.0.0.1:12345", "", "routing-transport-token", "", 400},
		{"GET", "/routing/capacity?contextId=", "127.0.0.1:12345", "", "routing-transport-token", "", 400},
		{"GET", "/routing/policy", "127.0.0.1:12345", "", "routing-transport-token", "{}", 400},
		{"GET", "/routing/contexts/codex-native/extra", "127.0.0.1:12345", "", "routing-transport-token", "", 404},
		{"GET", "/routing/contexts/", "127.0.0.1:12345", "", "routing-transport-token", "", 404},
		{"POST", "/routing/reports", "127.0.0.1:12345", "http://127.0.0.1:12345", "routing-transport-token", `{"requestId":"a","requestId":"b"}`, 400},
	} {
		t.Run(tc.method+tc.route, func(t *testing.T) {
			capture := &routingDispatchCapture{}
			h := New(capture, Config{Host: "127.0.0.1:12345", Token: "routing-transport-token"})
			r := httptest.NewRequest(tc.method, "http://127.0.0.1:12345/api/v1"+tc.route, strings.NewReader(tc.body))
			r.Host = tc.host
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || capture.calls != 0 {
				t.Fatalf("Rejected routing request reached private dispatch: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRoutingMutationPanicRetainsExactRetryIdentity(t *testing.T) {
	id := domain.NewID()
	h := New(panicExecutor{}, Config{Host: "127.0.0.1:12345", Token: "routing-transport-token"})
	r := httptest.NewRequest("POST", "http://127.0.0.1:12345/api/v1/routing/reports", strings.NewReader(`{"requestId":"`+id+`"}`))
	r.Header.Set("Authorization", "Bearer routing-transport-token")
	r.Header.Set("Origin", "http://127.0.0.1:12345")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var envelope domain.Envelope
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil || w.Code != 503 || envelope.RequestID != id || envelope.Error == nil || envelope.Error.Code != "write_uncertain" || envelope.Receipt != nil {
		t.Fatalf("A routing mutation lost the exact uncertainty target: %d %s", w.Code, w.Body.String())
	}
}
