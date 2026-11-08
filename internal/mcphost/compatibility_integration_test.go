package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
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
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Fixtures hold the real OS lock and serve the authenticated owner protocol.
// The MCP side is the actual newline-framed host over an in-memory connection.
func compatibilitySession(t *testing.T, root string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	client, err := ownerclient.New(root)
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(client)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	if err = left.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), host, right, right) }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	sdk := mcp.NewClient(&mcp.Implementation{Name: "compatibility-fixture", Version: "1"}, nil)
	session, err := sdk.Connect(ctx, &mcp.IOTransport{Reader: left, Writer: left}, nil)
	if err != nil {
		cancel()
		_ = left.Close()
		t.Fatalf("host handshake: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		_ = left.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("host failed to close")
		}
	})
	return session, ctx
}

func compatibilityCall(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, error) {
	t.Helper()
	return session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}
func compatibilityLockUnchanged(t *testing.T, root string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(filepath.Join(root, "owner.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("owner descriptor was replaced or removed: %v", err)
	}
	lock, err := localstate.Acquire(root)
	if err == nil {
		_ = lock.Close()
		t.Fatal("held owner lock was bypassed or removed")
	}
	if _, err = os.Stat(filepath.Join(root, "owner.lock")); err != nil {
		t.Fatalf("owner lock path removed: %v", err)
	}
}

func TestMCPKnownOlderOwnerAllowsOnlyAuthenticatedStop(t *testing.T) {
	ownerID, requestID := domain.NewID(), domain.NewID()
	token := strings.Repeat("f", 64)
	var metadata, actions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("owner authentication missing")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/meta" {
			metadata.Add(1)
			data := map[string]any{"ownerInstanceId": ownerID, "apiVersion": 1, "appVersion": "2.0.0-dev.2", "scope": "managed_execution", "managedExecution": "native_codex", "executionContract": "002-v1", "capabilities": []string{"workspace_foundation", "managed_execution"}, "admission": map[string]any{"state": "open", "incidentId": nil}, "capacity": map[string]any{"connectionId": "codex-native", "limit": 1, "activeAttemptId": nil}}
			_ = json.NewEncoder(w).Encode(domain.Success(200, domain.NewID(), data, nil).Envelope)
			return
		}
		actions.Add(1)
		if r.URL.Path != "/api/v1/service/stop" || r.Method != "POST" || r.Header.Get("Origin") != "http://"+r.Host {
			t.Errorf("unexpected owner action %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body struct {
			RequestID       string `json:"requestId"`
			OwnerInstanceID string `json:"ownerInstanceId"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.RequestID != requestID || body.OwnerInstanceID != ownerID {
			t.Error("stop changed captured authority")
		}
		_ = json.NewEncoder(w).Encode(domain.Success(200, requestID, map[string]any{"stopped": true}, &domain.Receipt{RequestID: requestID, Operation: "service.stop", CommittedAt: domain.Now()}).Envelope)
	}))
	t.Cleanup(server.Close)
	root := filepath.Join(t.TempDir(), "private")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if err = lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "owner.json"))
	if err != nil {
		t.Fatal(err)
	}
	session, ctx := compatibilitySession(t, root)
	// A current-execution observation must not reach the old owner. The
	// short context avoids waiting for the normal startup deadline.
	blocked, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	result, callErr := compatibilityCall(t, blocked, session, "council_tasks_get", map[string]any{"taskId": domain.NewID()})
	cancel()
	if callErr == nil && (result == nil || !result.IsError) {
		t.Fatal("older owner accepted a current operation")
	}
	if actions.Load() != 0 || metadata.Load() == 0 {
		t.Fatalf("unexpected old owner operations: metadata=%d actions=%d", metadata.Load(), actions.Load())
	}
	compatibilityLockUnchanged(t, root, before)
	result, callErr = compatibilityCall(t, ctx, session, "council_service_stop", map[string]any{"input": map[string]any{"requestId": requestID, "ownerInstanceId": ownerID}})
	if callErr != nil || result == nil || result.IsError || actions.Load() != 1 {
		t.Fatalf("known old owner Stop failed: %v, actions %d", callErr, actions.Load())
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(requestID)) || bytes.Contains(encoded, []byte(token)) {
		t.Fatal("receipt missing or private token exposed")
	}
	compatibilityLockUnchanged(t, root, before)
}

func TestMCPIncompatibleOrMalformedLockedOwnerIsNeverReplaced(t *testing.T) {
	for _, kind := range []string{"incompatible", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private")
			lock, err := localstate.Acquire(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = lock.Close() })
			ownerID, token := domain.NewID(), strings.Repeat("b", 64)
			var actions atomic.Int32
			if kind == "incompatible" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer "+token {
						t.Error("owner authentication missing")
						w.WriteHeader(401)
						return
					}
					if r.URL.Path != "/api/v1/meta" {
						actions.Add(1)
						w.WriteHeader(500)
						return
					}
					data := map[string]any{"ownerInstanceId": ownerID, "apiVersion": 1, "appVersion": "9.9.9", "scope": "unrecognized", "managedExecution": "unsupported"}
					_ = json.NewEncoder(w).Encode(domain.Success(200, domain.NewID(), data, nil).Envelope)
				}))
				t.Cleanup(server.Close)
				if err = lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.WriteFile(filepath.Join(root, "owner.json"), []byte(`{"not-a-descriptor":true}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filepath.Join(root, "owner.json"))
			if err != nil {
				t.Fatal(err)
			}
			session, ctx := compatibilitySession(t, root)
			blocked, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
			result, callErr := compatibilityCall(t, blocked, session, "council_tasks_get", map[string]any{"taskId": domain.NewID()})
			cancel()
			if callErr == nil && (result == nil || !result.IsError) {
				t.Fatal("unrecognized owner accepted execution")
			}
			compatibilityLockUnchanged(t, root, before)
			result, callErr = compatibilityCall(t, ctx, session, "council_service_stop", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "ownerInstanceId": ownerID}})
			if callErr != nil || result == nil || !result.IsError {
				t.Fatalf("unrecognized owner Stop was not refused: %v", callErr)
			}
			if actions.Load() != 0 {
				t.Fatal("an operation reached unrecognized owner")
			}
			encoded, _ := json.Marshal(result)
			if bytes.Contains(encoded, []byte(token)) {
				t.Fatal("private token exposed")
			}
			compatibilityLockUnchanged(t, root, before)
		})
	}
}
