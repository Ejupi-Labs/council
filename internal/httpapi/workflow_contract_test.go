package httpapi

import (
	"bytes"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func workflowTransportRequest(method, path, requestID string) *http.Request {
	var body []byte
	if method != "GET" {
		body, _ = json.Marshal(map[string]string{"requestId": requestID})
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:12345/api/v1"+path, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer workflow-fixture-token")
	r.Header.Set("Origin", "http://127.0.0.1:12345")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestWorkflowHTTPDispatchesCurrentAndHistoricalSubjectsWithoutWork(t *testing.T) {
	taskID, workflowID, candidateID := domain.NewID(), domain.NewID(), domain.NewID()
	for _, tc := range []struct{ method, path, operation, id string }{
		{"GET", "/workflow/preferences", "workflow.preferences.get", ""},
		{"PATCH", "/workflow/preferences", "workflow.preferences.update", ""},
		{"GET", "/tasks/" + taskID + "/workflow", "tasks.workflow.get", taskID},
		{"GET", "/tasks/" + taskID + "/workflow?taskVersion=1", "tasks.workflow.get", taskID},
		{"GET", "/tasks/" + taskID + "/workflow/steps?workflowId=" + workflowID + "&limit=1", "tasks.workflow.steps.list", taskID},
		{"GET", "/tasks/" + taskID + "/workflow/reviews?taskVersion=1&limit=1", "tasks.workflow.reviews.list", taskID},
		{"GET", "/tasks/" + taskID + "/workflow/checks?workflowId=" + workflowID, "tasks.workflow.checks.list", taskID},
		{"GET", "/tasks/" + taskID + "/workflow/findings?taskVersion=1", "tasks.workflow.findings.list", taskID},
		{"GET", "/tasks/" + taskID + "/workflow/deliveries?limit=1", "tasks.workflow.deliveries.list", taskID},
		{"GET", "/tasks/" + taskID + "/workflow/candidates?taskVersion=1", "tasks.workflow.candidates.list", taskID},
		{"GET", "/candidates/" + candidateID + "/changes?limit=1", "candidates.changes.list", candidateID},
		{"POST", "/tasks/" + taskID + "/workflow/adopt", "tasks.workflow.adopt", taskID},
		{"POST", "/tasks/" + taskID + "/workflow/advance", "tasks.workflow.advance", taskID},
		{"POST", "/tasks/" + taskID + "/workflow/check", "tasks.workflow.check", taskID},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			capture := &routingDispatchCapture{}
			h := New(capture, Config{Host: "127.0.0.1:12345", Token: "workflow-fixture-token"})
			r := workflowTransportRequest(tc.method, tc.path, domain.NewID())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 || capture.calls != 1 || capture.op != tc.operation || capture.args.ID != tc.id {
				t.Fatalf("Workflow contract did not reach its selected operation: %d %s %+v", w.Code, w.Body.String(), capture)
			}
			for key, values := range r.URL.Query() {
				if capture.args.Query[key] != values[0] {
					t.Fatalf("Historical selector/paging changed: %s %+v", key, capture.args.Query)
				}
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("Workflow evidence lost private response controls")
			}
		})
	}
}

func TestWorkflowHTTPRejectsAmbiguousTransportWithoutDispatch(t *testing.T) {
	id := domain.NewID()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/workflow/preferences?limit=1", 400},
		{"PUT", "/workflow/preferences", 405},
		{"GET", "/tasks/" + id + "/workflow?taskVersion=1&taskVersion=2", 400},
		{"GET", "/tasks/" + id + "/workflow/checks?workflowId=", 400},
		{"GET", "/tasks/" + id + "/workflow/findings?authority=true", 400},
		{"POST", "/tasks/" + id + "/workflow/advance?workflowId=" + id, 400},
		{"DELETE", "/tasks/" + id + "/workflow/checks", 405},
		{"GET", "/tasks/" + id + "/workflow/", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			capture := &routingDispatchCapture{}
			h := New(capture, Config{Host: "127.0.0.1:12345", Token: "workflow-fixture-token"})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, workflowTransportRequest(tc.method, tc.path, domain.NewID()))
			if w.Code != tc.status || capture.calls != 0 {
				t.Fatalf("Rejected workflow transport reached dispatch: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestWorkflowHTTPPanicRetainsExactAdvanceIdentity(t *testing.T) {
	id, requestID := domain.NewID(), domain.NewID()
	h := New(panicExecutor{}, Config{Host: "127.0.0.1:12345", Token: "workflow-fixture-token"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, workflowTransportRequest("POST", "/tasks/"+id+"/workflow/advance", requestID))
	var out domain.Envelope
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || w.Code != 503 || out.RequestID != requestID || out.Error == nil || out.Error.Code != "write_uncertain" || out.Receipt != nil {
		t.Fatalf("Advance uncertainty lost its retry identity: %d %s", w.Code, w.Body.String())
	}
}
