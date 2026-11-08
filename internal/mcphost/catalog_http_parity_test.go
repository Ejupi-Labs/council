package mcphost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

// A changed HTTP routing source requires the catalog audit to be repeated,
// including the negative-query checks below. Normalize checkout line endings.
func TestHTTPRouterSourcesRequireCatalogReaudit(t *testing.T) {
	sources := map[string]string{
		"server.go":    "e48f71928b467952b48c8775c3ca7082b8e4345928371171040624becaf1baa0",
		"execution.go": "02129ad2246c4fe63af6fd2a325ed9d0736d2b279f73c76e1f18e2539203c534",
		"routing.go":   "c1b1c798ec476a44f90ec7b316a07ed3c866ac15787ae57449baaabe8820d403",
		"workflow.go":  "ccb911c98395250255d2aca00d16061fc8eb0749e29d2e7a218adba70b55972a",
		"graphs.go":    "7d1d659efd6f7bb588712bdf135df5c7b5ac37479ab078c23d438c35c2d5a47e",
		"recovery.go":  "17b02bd1e0d47bbe7c35b3f33303328b86cb0c27d576c9564978f0fcfdafb201",
	}
	for name, expected := range sources {
		body, err := os.ReadFile(filepath.Join("..", "httpapi", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")))
		if fmt.Sprintf("%x", sum) != expected {
			t.Errorf("%s changed; re-audit the HTTP routes, catalog and query vocabulary", name)
		}
	}
}

type routeRecorder struct {
	operation string
	params    app.Params
}

func (r *routeRecorder) Execute(_ context.Context, op string, params app.Params) domain.Result {
	r.operation, r.params = op, params
	return domain.Success(http.StatusOK, domain.NewID(), map[string]bool{"recorded": true}, nil)
}

// Compare the fixed transport registry with the actual authenticated HTTP
// router, including composite identity order and every individual query name.
// This does not replace full application and MCP protocol integration tests.
func TestFixedRoutesReachMatchingHTTPRouter(t *testing.T) {
	// Independent inventory of the documented query vocabulary. Testing absent
	// keys as well as advertised ones detects a dropped key in the registry.
	allKeys := []string{"projectId", "limit", "cursor", "revision", "afterRevision", "timeoutMs", "offset", "through", "contextId", "accountId", "workflowId", "taskVersion", "inputId", "nodeKey", "kind"}
	for _, route := range ownerclient.Routes() {
		t.Run(route.Operation, func(t *testing.T) {
			parts := strings.Split(route.Template, "/")
			ids := make([]string, 0, len(route.PathKinds))
			index := 0
			for i, part := range parts {
				if !strings.HasPrefix(part, ":") {
					continue
				}
				var value string
				switch route.PathKinds[index] {
				case "revision":
					value = "2"
				case "graphkey":
					value = "node-one"
				case "context":
					value = "codex-native"
				case "profile":
					value = "codex-native-default"
				default:
					value = fmt.Sprintf("%032x", index+1)
				}
				parts[i] = value
				ids = append(ids, value)
				index++
			}
			path := "/api/v1" + strings.Join(parts, "/")
			queries := append([]string{""}, route.QueryKeys...)
			for _, key := range queries {
				recorder := &routeRecorder{}
				handler := httpapi.New(recorder, httpapi.Config{Host: "council.test", Token: "fixture-token"})
				endpoint := "http://council.test" + path
				if key != "" {
					endpoint += "?" + url.QueryEscape(key) + "=1"
				}
				var body []byte
				if route.Method != "GET" {
					body = []byte(`{"requestId":"0123456789abcdef0123456789abcdef"}`)
				}
				req := httptest.NewRequest(route.Method, endpoint, bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer fixture-token")
				req.Header.Set("Origin", "http://council.test")
				if route.Method != "GET" {
					req.Header.Set("Content-Type", "application/json")
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if response.Code != http.StatusOK {
					t.Fatalf("query %q: status %d", key, response.Code)
				}
				if route.Operation == "meta" {
					continue
				}
				if recorder.operation != route.Operation || recorder.params.ID != strings.Join(ids, "/") {
					t.Fatalf("query %q: actual operation %q identity %q, expected %q %q", key, recorder.operation, recorder.params.ID, route.Operation, strings.Join(ids, "/"))
				}
				if key != "" && recorder.params.Query[key] != "1" {
					t.Fatalf("query %q was not forwarded", key)
				}
				if !bytes.Equal(body, recorder.params.Body) {
					t.Fatal("body changed across router")
				}
			}
			for _, key := range allKeys {
				allowed := false
				for _, present := range route.QueryKeys {
					if present == key {
						allowed = true
						break
					}
				}
				if allowed {
					continue
				}
				recorder := &routeRecorder{}
				handler := httpapi.New(recorder, httpapi.Config{Host: "council.test", Token: "fixture-token"})
				req := httptest.NewRequest(route.Method, "http://council.test"+path+"?"+url.QueryEscape(key)+"=1", nil)
				req.Header.Set("Authorization", "Bearer fixture-token")
				req.Header.Set("Origin", "http://council.test")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if response.Code != http.StatusBadRequest || recorder.operation != "" {
					t.Fatalf("unlisted query %q was accepted by HTTP router (status %d)", key, response.Code)
				}
			}
		})
	}
}
