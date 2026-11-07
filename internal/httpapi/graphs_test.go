package httpapi

import (
	"context"
	json "encoding/json/v2"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphRoutesPreserveExactTargetsAndAllowedReads(t *testing.T) {
	g, input, dispatch := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	base := "/api/v1/graphs"
	for _, c := range []struct {
		path, verb, op, id, methods string
		query                       []string
	}{
		{base, "POST", "graphs.start", "", "GET, POST", nil},
		{base, "GET", "graphs.list", "", "GET, POST", []string{"projectId", "limit", "cursor"}},
		{base + "/" + g, "GET", "graphs.get", g, "GET", nil},
		{base + "/" + g + "/definition", "GET", "graphs.definition.get", g, "GET", nil},
		{base + "/" + g + "/wait", "GET", "graphs.wait", g, "GET", []string{"afterRevision", "timeoutMs"}},
		{base + "/" + g + "/advance", "POST", "graphs.advance", g, "POST", nil},
		{base + "/" + g + "/stop", "POST", "graphs.stop", g, "POST", nil},
		{base + "/" + g + "/budget", "GET", "graphs.budget.get", g, "GET", nil},
		{base + "/" + g + "/budget-ledger", "GET", "graphs.budget.ledger", g, "GET", []string{"limit", "cursor"}},
		{base + "/" + g + "/budget-amendments", "POST", "graphs.budget.amend", g, "POST", nil},
		{base + "/" + g + "/inputs", "GET", "graphs.inputs.list", g, "GET", []string{"limit", "cursor"}},
		{base + "/" + g + "/inputs/" + input, "GET", "graphs.inputs.get", g + "/" + input, "GET", nil},
		{base + "/" + g + "/inputs/" + input + "/members", "GET", "graphs.inputs.members", g + "/" + input, "GET", []string{"limit", "cursor"}},
		{base + "/" + g + "/dispatches", "GET", "graphs.dispatches.list", g, "GET", []string{"inputId", "nodeKey", "limit", "cursor"}},
		{base + "/" + g + "/dispatches/" + dispatch, "GET", "graphs.dispatches.get", g + "/" + dispatch, "GET", nil},
		{base + "/" + g + "/nodes/Case_1/stop", "POST", "graphs.nodes.stop", g + "/Case_1", "POST", nil},
		{base + "/" + g + "/nodes/Case_1/guidance", "POST", "graphs.nodes.guidance", g + "/Case_1", "POST", nil},
	} {
		t.Run(c.op, func(t *testing.T) {
			op, id, methods, query := route(c.path, c.verb)
			if op != c.op || id != c.id || methods != c.methods || !reflect.DeepEqual(query, c.query) {
				t.Fatalf("Exact graph route changed: %s %s %s %v", op, id, methods, query)
			}
		})
	}
	for _, path := range []string{base + "/", base + "//stop", base + "/" + g + "/nodes//stop", base + "/" + g + "/inputs//members", base + "/" + g + "/unknown"} {
		if op, _, _, _ := route(path, "GET"); op != "" {
			t.Fatal("Malformed graph path acquired an operation", path, op)
		}
	}
}

type graphHTTPSpy struct{ calls int }

func (s *graphHTTPSpy) Execute(_ context.Context, op string, _ app.Params) domain.Result {
	s.calls++
	return domain.Success(200, domain.NewID(), map[string]string{"operation": op}, nil)
}

func TestGraphHTTPBoundsDeclarationBeforeServiceDispatch(t *testing.T) {
	spy := &graphHTTPSpy{}
	handler := New(spy, Config{Host: "127.0.0.1:7001", Token: "controlled-graph-test", OwnerInstanceID: strings.Repeat("1", 32)})
	body := `{"requestId":"` + strings.Repeat("2", 32) + `","projectId":"` + strings.Repeat("3", 32) + `","nodes":[{"key":"first","originalText":"` + strings.Repeat("x", domain.MaxGraphDeclarationBytes) + `"}]}`
	req := httptest.NewRequest("POST", "http://127.0.0.1:7001/api/v1/graphs", strings.NewReader(body))
	req.Header.Set("Origin", "http://127.0.0.1:7001")
	req.Header.Set("Authorization", "Bearer controlled-graph-test")
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, req)
	var envelope domain.Envelope
	if err := json.Unmarshal(out.Body.Bytes(), &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "input_too_large" || spy.calls != 0 {
		t.Fatal("Oversized graph reached the service or lost its precise bound", out.Code, envelope, spy.calls, err)
	}
}
