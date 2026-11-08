package ownerclient

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func ownerMetadataFixture(id string, old bool) map[string]any {
	m := map[string]any{"ownerInstanceId": id, "apiVersion": 1, "appVersion": domain.AppVersion, "scope": "managed_execution", "managedExecution": "native_codex_claude", "executionContract": domain.ExecutionContract, "workflowContract": domain.WorkflowContract, "graphContract": domain.GraphContract, "recoveryContract": "007-v1", "historyContract": "007a-v1", "maintenanceContract": "owner-stop/1", "capabilities": []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery", "task_graphs", "safe_recovery", "metadata_history"}, "admission": map[string]any{"state": "open", "incidentId": nil}, "capacity": map[string]any{"resourceId": domain.ManagedNativeResourceID, "limit": 2, "activeAttemptId": nil, "occupiedSlots": "0", "activeClaims": []any{}}}
	if old {
		m["appVersion"] = "2.0.0-dev.2"
		m["managedExecution"] = "native_codex"
		m["executionContract"] = "002-v1"
		delete(m, "workflowContract")
		delete(m, "graphContract")
		delete(m, "recoveryContract")
		delete(m, "historyContract")
		delete(m, "maintenanceContract")
		m["capabilities"] = []string{"workspace_foundation", "managed_execution"}
		m["capacity"] = map[string]any{"connectionId": "codex-native", "limit": 1, "activeAttemptId": nil}
	}
	return m
}

func fixtureClient(t *testing.T, old bool, handle func(http.ResponseWriter, *http.Request)) (*Client, string, string, *atomic.Int32) {
	t.Helper()
	ownerID := domain.NewID()
	token := strings.Repeat("f", 64)
	var actions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("owner request lacked exact private authentication")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/v1/meta" {
			b, _ := json.Marshal(domain.Success(200, domain.NewID(), ownerMetadataFixture(ownerID, old), nil).Envelope)
			_, _ = w.Write(b)
			return
		}
		actions.Add(1)
		if r.Header.Get("Origin") != serverOrigin(r) {
			t.Error("owner request lacked exact local origin")
		}
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	root := filepath.Join(t.TempDir(), "private")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	desc := localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}
	if err = lock.WriteDescriptor(desc); err != nil {
		t.Fatal(err)
	}
	c, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return c, ownerID, token, &actions
}
func serverOrigin(r *http.Request) string { return "http://" + r.Host }

func TestClientIsLazyAndRejectsInvalidBeforeStateAccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	c, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("construction accessed state: %v", err)
	}
	for _, req := range []Request{{Operation: "unknown"}, {Operation: "tasks.get", PathIDs: []string{"../service/stop"}}, {Operation: "tasks.start", RequestID: domain.NewID(), Body: []byte(`{"requestId":"different"}`)}} {
		if r := c.Call(context.Background(), req); r.Envelope.Error == nil {
			t.Fatal("invalid request accepted", r)
		}
		if _, err = os.Stat(root); !os.IsNotExist(err) {
			t.Fatalf("invalid request touched state: %v", err)
		}
	}
}

func TestClientPreservesExactReceiptAndBinaryPage(t *testing.T) {
	requestID, taskID, artifactID := domain.NewID(), domain.NewID(), domain.NewID()
	c, _, _, actions := fixtureClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		var result domain.Result
		switch r.URL.Path {
		case "/api/v1/tasks":
			if r.Method != "POST" {
				t.Error("unexpected task method")
			}
			result = domain.Success(201, requestID, map[string]any{"taskId": taskID}, &domain.Receipt{RequestID: requestID, Operation: "tasks.start", CommittedAt: domain.Now()})
		case "/api/v1/artifacts/" + artifactID + "/bytes":
			if r.URL.Query().Get("through") != "9007199254740993" || r.URL.Query().Get("offset") != "0" {
				t.Error("byte bounds changed")
			}
			result = domain.Success(200, domain.NewID(), map[string]any{"bytesBase64": "AP+A", "nextOffset": "3", "through": "9007199254740993", "eof": false, "streamClosed": true}, nil)
		default:
			t.Errorf("unexpected owner route %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		b, _ := json.Marshal(result.Envelope)
		w.WriteHeader(result.Status)
		_, _ = w.Write(b)
	})
	body, _ := json.Marshal(map[string]string{"requestId": requestID, "projectId": domain.NewID(), "originalText": "fixture"})
	r := c.Call(context.Background(), Request{Operation: "tasks.start", Body: body, RequestID: requestID})
	if r.Status != 201 || r.Envelope.Receipt == nil || r.Envelope.Receipt.RequestID != requestID || r.Envelope.Receipt.Operation != "tasks.start" {
		t.Fatal(r)
	}
	r = c.Call(context.Background(), Request{Operation: "artifacts.bytes", PathIDs: []string{artifactID}, Query: map[string]string{"offset": "0", "through": "9007199254740993", "limit": "3"}})
	if r.Status != 200 || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
	var data map[string]any
	raw, ok := r.Envelope.Data.(jsontext.Value)
	if !ok || json.Unmarshal(raw, &data) != nil || data["bytesBase64"] != "AP+A" || data["through"] != "9007199254740993" {
		t.Fatal(r)
	}
	if actions.Load() != 2 {
		t.Fatal("unexpected side effect count", actions.Load())
	}
}

func TestClientKnownOldOwnerAllowsOnlyExplicitStop(t *testing.T) {
	requestID := domain.NewID()
	c, ownerID, _, actions := fixtureClient(t, true, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/service/stop" || r.Method != "POST" {
			t.Errorf("old owner received unsupported operation %s", r.URL.Path)
		}
		b, _ := json.Marshal(domain.Success(200, requestID, map[string]bool{"stopped": true}, &domain.Receipt{RequestID: requestID, Operation: "service.stop", CommittedAt: domain.Now()}).Envelope)
		_, _ = w.Write(b)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if r := c.Call(ctx, Request{Operation: "tasks.get", PathIDs: []string{domain.NewID()}}); r.Envelope.Error == nil || r.Envelope.Error.Code != "owner_unavailable" {
		t.Fatal("older owner accepted execution", r)
	}
	if actions.Load() != 0 {
		t.Fatal("incompatible operation reached older owner")
	}
	body, _ := json.Marshal(map[string]string{"requestId": requestID, "ownerInstanceId": ownerID})
	r := c.Call(context.Background(), Request{Operation: "service.stop", Body: body, RequestID: requestID})
	if r.Status != 200 || r.Envelope.Receipt == nil || actions.Load() != 1 {
		t.Fatal(r, actions.Load())
	}
}

func TestClientAmbiguousMutationAndCancellationAreNotStop(t *testing.T) {
	requestID, attempt := domain.NewID(), domain.NewID()
	token := strings.Repeat("f", 64)
	c, _, _, actions := fixtureClient(t, false, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/wait") {
			<-r.Context().Done()
			return
		}
		// An invalid acknowledgement that includes the private token is never
		// surfaced; the mutation is uncertain and must not be resent here.
		_, _ = w.Write([]byte(token))
	})
	body, _ := json.Marshal(map[string]string{"requestId": requestID})
	r := c.Call(context.Background(), Request{Operation: "attempts.stop", PathIDs: []string{attempt}, Body: body, RequestID: requestID})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" || r.Envelope.RequestID != requestID || strings.Contains(r.Envelope.Error.Message, token) {
		t.Fatal(r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	r = c.Call(ctx, Request{Operation: "attempts.wait", PathIDs: []string{attempt}, Query: map[string]string{"timeoutMs": "30000"}})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "observation_interrupted" || actions.Load() != 2 {
		t.Fatal(r, actions.Load())
	}
}
