package ownerclient

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
)

type routeSpy struct{ operation string }

func (s *routeSpy) Execute(_ context.Context, op string, _ app.Params) domain.Result {
	s.operation = op
	return domain.Success(200, domain.NewID(), map[string]any{"ok": true}, nil)
}

func TestRoutesMatchActualHTTPRouter(t *testing.T) {
	seen, methods := map[string]bool{}, map[string]bool{}
	for _, r := range Routes() {
		if seen[r.Operation] || methods[r.Method+" "+r.Template] {
			t.Fatalf("duplicate fixed operation or route: %+v", r)
		}
		seen[r.Operation] = true
		methods[r.Method+" "+r.Template] = true
		ids := make([]string, len(r.PathKinds))
		for i, k := range r.PathKinds {
			switch k {
			case "id":
				ids[i] = strings.Repeat("a", 32)
			case "revision":
				ids[i] = "1"
			case "graphkey":
				ids[i] = "node_A"
			case "context":
				ids[i] = "codex-native"
			case "profile":
				ids[i] = "codex-luna-economy"
			default:
				t.Fatalf("unknown kind %q", k)
			}
		}
		path, invalid := resolveRoute(r, ids, nil)
		if invalid != nil {
			t.Fatalf("%s: %v", r.Operation, invalid)
		}
		var body []byte
		if r.Method != "GET" {
			body = []byte(`{"requestId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
		}
		req := httptest.NewRequest(r.Method, "http://127.0.0.1:12345"+path, bytes.NewReader(body))
		req.Host = "127.0.0.1:12345"
		req.Header.Set("Authorization", "Bearer fixture")
		req.Header.Set("Origin", "http://127.0.0.1:12345")
		if r.Method != "GET" {
			req.Header.Set("Content-Type", "application/json")
		}
		spy := &routeSpy{}
		w := httptest.NewRecorder()
		httpapi.New(spy, httpapi.Config{Host: req.Host, Token: "fixture"}).ServeHTTP(w, req)
		if r.Operation == "meta" {
			if w.Code != 200 {
				t.Fatalf("metadata route %d", w.Code)
			}
			continue
		}
		if spy.operation != r.Operation {
			t.Fatalf("%s resolved to %q (HTTP %d)", r.Operation, spy.operation, w.Code)
		}
	}
	if len(seen) < 65 {
		t.Fatalf("catalog unexpectedly incomplete: %d", len(seen))
	}
	copy := Routes()
	if len(copy) == 0 {
		t.Fatal("empty registry")
	}
	copy[0].Operation = "changed"
	if _, ok := RouteFor("changed"); ok {
		t.Fatal("registry is externally mutable")
	}
}

func TestRouteRejectsUnknownQueriesAndPathInjection(t *testing.T) {
	r, _ := RouteFor("tasks.get")
	for _, ids := range [][]string{{"../meta"}, {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/stop"}, {"%2f"}, {}} {
		if _, err := resolveRoute(r, ids, nil); err == nil {
			t.Fatalf("accepted unsafe ID %q", ids)
		}
	}
	if _, err := resolveRoute(r, []string{strings.Repeat("a", 32)}, map[string]string{"cursor": "x"}); err == nil {
		t.Fatal("undocumented query accepted")
	}
	if _, ok := RouteFor("../service.stop"); ok {
		t.Fatal("unknown operation accepted")
	}
}
