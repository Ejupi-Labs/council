package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func TestHistoryCLIParsesExactScopeAndRejectsInvalidInput(t *testing.T) {
	id, req := strings.Repeat("1", 32), strings.Repeat("2", 32)
	for _, tc := range []struct {
		args      []string
		op, route string
	}{
		{[]string{"history", "capture", "--project", id, "--request-id", req}, "history.snapshots.create", "/history/snapshots"},
		{[]string{"history", "show", id}, "history.snapshots.get", "/history/snapshots/" + id},
		{[]string{"history", "records", id, "--kind", "tasks", "--limit", "5"}, "history.snapshots.records", "/history/snapshots/" + id + "/records"},
		{[]string{"history", "export-preview", "--snapshot", id, "--revision", "1", "--filename", "history-a.json", "--request-id", req}, "history.exports.preview", "/history/exports/preview"},
		{[]string{"history", "export", "--preview", id, "--revision", "1", "--request-id", req}, "history.exports.create", "/history/exports"},
		{[]string{"history", "operation", id}, "history.operations.get", "/history/operations/" + id},
		{[]string{"history", "reconcile", id, "--revision", "1", "--request-id", req}, "history.operations.reconcile", "/history/operations/" + id + "/reconciliations"},
	} {
		c, err := parse(tc.args, nil)
		if err != nil || c.op != tc.op || c.route != tc.route || !requiresExecutionOwner(c.op) {
			t.Fatal(tc.args, c, err)
		}
		if historyMutation(c.op) && (c.requestID != req || validateFrozenGraphBody(c) != nil) {
			t.Fatal("invalid frozen history request", c)
		}
	}
	for _, args := range [][]string{
		{"history", "capture", "--project", id, "--filename", "history-a.json"},
		{"history", "records", id, "--kind", "foreign"},
		{"history", "records", id, "--kind", "tasks", "--limit", "101"},
		{"history", "export-preview", "--snapshot", id, "--revision", "1", "--filename", "../x.json"},
		{"history", "export", "--preview", id, "--revision", "01"},
		{"history", "reconcile", id, "--revision", "1", "--filename", "history-a.json"},
	} {
		if _, err := parse(args, nil); err == nil {
			t.Fatal("invalid history input accepted", args)
		}
	}
}
func TestHistoryCLIRetainsExactRequestAfterLostAcknowledgement(t *testing.T) {
	id, req := strings.Repeat("1", 32), strings.Repeat("2", 32)
	lock, err := localstate.Acquire(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		bodies = append(bodies, append([]byte{}, body...))
		first := len(bodies) == 1
		mu.Unlock()
		if first {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		receipt := &domain.Receipt{RequestID: req, Operation: "history.exports.preview", CommittedAt: domain.Now()}
		_ = json.MarshalWrite(w, domain.Success(201, req, map[string]string{"previewId": id}, receipt).Envelope)
	}))
	defer server.Close()
	desc := localstate.Descriptor{OwnerInstanceID: domain.NewID(), Origin: server.URL, Token: "fixture"}
	args := []string{"history", "export-preview", "--snapshot", id, "--revision", "1", "--filename", "history-a.json", "--request-id", req}
	c, invalid := parse(args, nil)
	if invalid != nil {
		t.Fatal(invalid)
	}
	first := forwardGraphIntent(context.Background(), lock.Root(), desc, c)
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(first)
	}
	second := forwardGraphIntent(context.Background(), lock.Root(), desc, c)
	mu.Lock()
	matched := len(bodies) == 2 && bytes.Equal(bodies[0], bodies[1])
	mu.Unlock()
	if second.Envelope.Error != nil || second.Envelope.Receipt == nil || second.Envelope.Receipt.RequestID != req || !matched {
		t.Fatal("retry changed captured body", second)
	}
	changed := append([]string{}, args...)
	changed[7] = "history-b.json"
	different, invalid := parse(changed, nil)
	if invalid != nil {
		t.Fatal(invalid)
	}
	conflict := forwardGraphIntent(context.Background(), lock.Root(), desc, different)
	mu.Lock()
	count := len(bodies)
	mu.Unlock()
	if conflict.Envelope.Error == nil || conflict.Envelope.Error.Code != "request_conflict" || count != 2 {
		t.Fatal("changed request reached owner", conflict, count)
	}
}
func TestHistoryOwnerCompatibilityKeepsPriorStop(t *testing.T) {
	for _, tc := range []struct {
		name, contract   string
		capability, want bool
	}{{"current", "007a-v1", true, true}, {"prior", "", false, false}, {"wrong_version", "007b-v1", true, false}, {"missing_capability", "007a-v1", false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			ownerID := domain.NewID()
			m := graphMetadataFixture(ownerID, 0)
			if tc.contract != "" {
				m["historyContract"] = tc.contract
			}
			if tc.capability {
				m["capabilities"] = append(m["capabilities"].([]string), "metadata_history")
			}
			body, err := json.Marshal(domain.Success(200, domain.NewID(), m, nil).Envelope)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
			defer server.Close()
			desc := localstate.Descriptor{OwnerInstanceID: ownerID, Origin: server.URL, Token: "fixture"}
			if got := verifyHistoryOwner(context.Background(), desc); got != tc.want {
				t.Fatal("history compatibility", got, tc.want)
			}
			if !verifyExecutionOwner(context.Background(), desc) || !verifyMaintenanceOwner(context.Background(), desc) {
				t.Fatal("prior execution/stop compatibility lost")
			}
		})
	}
}
