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

func TestRecoveryCLIParsesExactScopeAndRejectsInvalidAuthority(t *testing.T) {
	id, request, node := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	for _, tc := range []struct {
		args             []string
		stdin, op, route string
	}{
		{[]string{"tasks", "pause", id, "--task-version", "1", "--expected-revision", "2", "--request-id", request}, "", "tasks.pause", "/tasks/" + id + "/pause"},
		{[]string{"graphs", "pause", id, "--graph-version", "1", "--graph-control-revision", "2", "--budget-authority-epoch", "3", "--window-number", "4", "--node-authorities-file", "-", "--request-id", request}, `[{"nodeId":"` + node + `","taskVersion":"1","nodeControlRevision":"2"}]`, "graphs.pause", "/graphs/" + id + "/pause"},
		{[]string{"checkpoints", "get", id}, "", "checkpoints.get", "/checkpoints/" + id},
		{[]string{"checkpoints", "resume", id, "--checkpoint-revision", "2", "--scope-file", "-", "--request-id", request}, `{"kind":"task","taskId":"` + id + `","taskVersion":"1","expectedTaskRevision":"2"}`, "checkpoints.resume", "/checkpoints/" + id + "/resume"},
		{[]string{"incidents", "get", id}, "", "incidents.get", "/incidents/" + id},
		{[]string{"incidents", "reconcile", id, "--incident-revision", "2", "--request-id", request}, "", "incidents.reconcile", "/incidents/" + id + "/reconciliations"},
	} {
		c, err := parse(tc.args, strings.NewReader(tc.stdin))
		if err != nil || c.op != tc.op || c.params.ID != id || c.route != tc.route || !requiresExecutionOwner(c.op) {
			t.Fatalf("wrong recovery CLI operation: %#v %v", c, err)
		}
		if recoveryMutation(c.op) && (c.requestID != request || validateFrozenGraphBody(c) != nil) {
			t.Fatalf("invalid frozen recovery body: %#v", c)
		}
	}
	for _, tc := range []struct {
		args  []string
		stdin string
	}{
		{[]string{"tasks", "pause", id, "--task-version", "1"}, ""},
		{[]string{"graphs", "pause", id, "--graph-version", "1"}, ""},
		{[]string{"incidents", "reconcile", id, "--incident-revision", "01"}, ""},
		{[]string{"checkpoints", "resume", id, "--checkpoint-revision", "1", "--scope-file", "-"}, `{"kind":"task","taskId":"` + id + `","taskVersion":"1","expectedTaskRevision":"2","proof":true}`},
		{[]string{"incidents", "get", id, "--request-id", request}, ""},
	} {
		if _, err := parse(tc.args, strings.NewReader(tc.stdin)); err == nil {
			t.Fatalf("invalid authority accepted: %v", tc.args)
		}
	}
}

func TestRecoveryCLIRetainsFrozenBodyAfterLostAcknowledgement(t *testing.T) {
	resource, request := strings.Repeat("1", 32), strings.Repeat("2", 32)
	lock, err := localstate.Acquire(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	var bodies [][]byte
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Error(readErr)
			return
		}
		mu.Lock()
		bodies = append(bodies, append([]byte{}, body...))
		firstRequest := len(bodies) == 1
		mu.Unlock()
		if firstRequest {
			connection, _, hijackErr := w.(http.Hijacker).Hijack()
			if hijackErr != nil {
				t.Error(hijackErr)
				return
			}
			_ = connection.Close()
			return
		}
		receipt := &domain.Receipt{RequestID: request, Operation: "tasks.pause", CommittedAt: domain.Now()}
		_ = json.MarshalWrite(w, domain.Success(200, request, map[string]string{"checkpointId": resource}, receipt).Envelope)
	}))
	defer server.Close()
	desc := localstate.Descriptor{OwnerInstanceID: domain.NewID(), Origin: server.URL, Token: "fake"}
	args := []string{"tasks", "pause", resource, "--task-version", "1", "--expected-revision", "2", "--request-id", request}
	c, parseErr := parse(args, nil)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	first := forwardGraphIntent(context.Background(), lock.Root(), desc, c)
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatalf("expected uncertain response: %#v", first)
	}
	second := forwardGraphIntent(context.Background(), lock.Root(), desc, c)
	mu.Lock()
	matched := len(bodies) == 2 && bytes.Equal(bodies[0], bodies[1])
	mu.Unlock()
	if second.Envelope.Error != nil || second.Envelope.Receipt == nil || second.Envelope.Receipt.RequestID != request || !matched {
		t.Fatalf("retry did not preserve the exact body: %#v", second)
	}
	changed := append([]string{}, args...)
	changed[6] = "3"
	different, parseErr := parse(changed, nil)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	conflict := forwardGraphIntent(context.Background(), lock.Root(), desc, different)
	mu.Lock()
	numberOfRequests := len(bodies)
	mu.Unlock()
	if conflict.Envelope.Error == nil || conflict.Envelope.Error.Code != "request_conflict" || numberOfRequests != 2 {
		t.Fatalf("changed input reused the request identity: %#v count %d", conflict, numberOfRequests)
	}
}
