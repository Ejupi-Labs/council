package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/engine"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type retryFixtureOwner struct{ *app.Service }

func (o retryFixtureOwner) Metadata(context.Context) (map[string]any, error) {
	return map[string]any{"scope": "managed_execution", "managedExecution": "native_codex_claude", "executionContract": domain.ExecutionContract, "workflowContract": domain.WorkflowContract, "graphContract": domain.GraphContract, "recoveryContract": "007-v1", "historyContract": "007a-v1", "maintenanceContract": "owner-stop/1", "capabilities": []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery", "task_graphs", "safe_recovery", "metadata_history"}, "admission": map[string]any{"state": "open", "incidentId": nil}, "capacity": map[string]any{"resourceId": domain.ManagedNativeResourceID, "limit": 2, "activeAttemptId": nil, "occupiedSlots": "0", "activeClaims": []any{}}}, nil
}

type retryTraceWriter struct {
	conn net.Conn
	mu   sync.Mutex
	data bytes.Buffer
}

func (w *retryTraceWriter) Write(p []byte) (int, error) {
	n, err := w.conn.Write(p)
	w.mu.Lock()
	_, _ = w.data.Write(p[:n])
	w.mu.Unlock()
	return n, err
}
func (w *retryTraceWriter) Close() error { return w.conn.Close() }
func (w *retryTraceWriter) callIDs(t *testing.T) []string {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	var ids []string
	for _, line := range bytes.Split(w.data.Bytes(), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var message struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
		}
		if json.Unmarshal(line, &message) != nil {
			t.Fatal("invalid traced client frame")
		}
		if message.Method == "tools/call" {
			ids = append(ids, string(message.ID))
		}
	}
	return ids
}

// The real application/store commits before a fixture HTTP connection closes
// without a response. The client then retries over actual SDK/MCP framing.
func TestMCPRetryAfterRealOwnerCommitAndLostHTTPAcknowledgement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	db, err := store.Open(filepath.Join(root, "workspace.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	before, err := db.Policy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next, err := strconv.ParseInt(before.Revision, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	next++
	mode := "autonomous"
	if before.Mode == mode {
		mode = "supervised"
	}
	requestID, ownerID := domain.NewID(), domain.NewID()
	token := strings.Repeat("c", 64)
	var policyCalls atomic.Int32
	var retainedMu sync.Mutex
	var dropped []byte
	srv := httptest.NewUnstartedServer(nil)
	handler := httpapi.New(retryFixtureOwner{app.New(db)}, httpapi.Config{Host: srv.Listener.Addr().String(), Token: token, OwnerInstanceID: ownerID})
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/policy" && r.Method == http.MethodPut && policyCalls.Add(1) == 1 {
			record := httptest.NewRecorder()
			handler.ServeHTTP(record, r)
			if record.Code != 200 {
				t.Errorf("fixture commit failed: %d", record.Code)
			}
			retainedMu.Lock()
			dropped = append([]byte(nil), record.Body.Bytes()...)
			retainedMu.Unlock()
			hijack, ok := w.(http.Hijacker)
			if !ok {
				t.Error("fixture transport cannot drop acknowledgement")
				return
			}
			conn, _, err := hijack.Hijack()
			if err != nil {
				t.Errorf("fixture hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		handler.ServeHTTP(w, r)
	})
	srv.Start()
	t.Cleanup(srv.Close)
	if err = lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: srv.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	client, err := ownerclient.New(root)
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(client)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	_ = left.SetDeadline(time.Now().Add(20 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), host, right, right) }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	trace := &retryTraceWriter{conn: left}
	sdk := mcp.NewClient(&mcp.Implementation{Name: "retry-fixture", Version: "1"}, nil)
	session, err := sdk.Connect(ctx, &mcp.IOTransport{Reader: left, Writer: trace}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = session.Close()
		_ = left.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("host failed to close")
		}
	})
	args := map[string]any{"input": map[string]any{"requestId": requestID, "expectedRevision": before.Revision, "mode": mode}}
	call := func(arguments any) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "council_policy_set", Arguments: arguments})
		if err != nil || result == nil {
			t.Fatalf("MCP call failed: %v", err)
		}
		return result
	}
	first := call(args)
	if !first.IsError {
		t.Fatal("missing acknowledgement was reported as success")
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(firstJSON, []byte("write_uncertain")) || !bytes.Contains(firstJSON, []byte(requestID)) || bytes.Contains(firstJSON, []byte(token)) {
		t.Fatal("uncertain result did not preserve the request or leaked authentication")
	}
	committed, err := db.Policy(ctx)
	if err != nil || committed.Revision != strconv.FormatInt(next, 10) || committed.Mode != mode {
		t.Fatalf("effect was not committed before dropped acknowledgement: %+v %v", committed, err)
	}
	replay := call(args)
	if replay.IsError {
		t.Fatal("same request did not replay")
	}
	var payload struct {
		Envelope json.RawMessage `json:"envelope"`
	}
	raw, err := json.Marshal(replay.StructuredContent)
	if err != nil || json.Unmarshal(raw, &payload) != nil {
		t.Fatal("missing structured owner result")
	}
	retainedMu.Lock()
	original := append([]byte(nil), dropped...)
	retainedMu.Unlock()
	var a, b any
	if json.Unmarshal(original, &a) != nil || json.Unmarshal(payload.Envelope, &b) != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("replayed data or receipt differed from committed response")
	}
	changed := map[string]any{"input": map[string]any{"requestId": requestID, "expectedRevision": before.Revision, "mode": before.Mode}}
	conflict := call(changed)
	conflictJSON, _ := json.Marshal(conflict)
	if !conflict.IsError || !bytes.Contains(conflictJSON, []byte("request_conflict")) {
		t.Fatal("changed intent did not conflict")
	}
	final, err := db.Policy(ctx)
	if err != nil || final.Revision != committed.Revision || final.Mode != committed.Mode {
		t.Fatal("replay or conflict applied an extra effect")
	}
	if policyCalls.Load() != 3 {
		t.Fatalf("unexpected owner calls: %d", policyCalls.Load())
	}
	ids := trace.callIDs(t)
	if len(ids) != 3 || ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		t.Fatalf("calls lacked distinct JSON-RPC ids: %v", ids)
	}
}

// The production owner canonicalizes selected omitted task defaults. The
// process preparation seam intentionally prevents any native launch here.
func TestMCPRetryCanonicalTaskDefaultsAgainstRealOwner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	db, err := store.Open(filepath.Join(root, "workspace.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ownerID := domain.NewID()
	e, err := engine.New(db, lock, engine.Config{OwnerInstanceID: ownerID, Prepare: func(context.Context, process.NativeSpec) (*process.Owned, error) {
		return nil, errors.New("fixture prevents process launch")
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Errorf("fixture owner close: %v", err)
		}
	})
	service := app.New(db)
	service.Execution = e
	projectDir := filepath.Join(t.TempDir(), "fixture-project")
	if err = os.Mkdir(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	gitConfig := filepath.Join(t.TempDir(), "empty-gitconfig")
	if err = os.WriteFile(gitConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		command := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		command.Env = env
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("fixture Git: %v %s", err, output)
		}
	}
	ctx := context.Background()
	openBody, err := json.Marshal(map[string]any{"requestId": domain.NewID(), "selectedPath": projectDir, "basePath": projectDir})
	if err != nil {
		t.Fatal(err)
	}
	opened := service.Execute(ctx, "projects.open", app.Params{Body: openBody})
	if opened.Envelope.Error != nil {
		t.Fatalf("open fixture project: %v", opened.Envelope.Error)
	}
	project, ok := opened.Envelope.Data.(domain.ProjectOpen)
	if !ok {
		t.Fatal("unexpected project result")
	}
	policy, err := db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != "autonomous" {
		body, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "expectedRevision": policy.Revision, "mode": "autonomous"})
		result := service.Execute(ctx, "policy.set", app.Params{Body: body})
		if result.Envelope.Error != nil {
			t.Fatalf("fixture policy: %v", result.Envelope.Error)
		}
	}
	token := strings.Repeat("d", 64)
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = httpapi.New(service, httpapi.Config{Host: srv.Listener.Addr().String(), Token: token, OwnerInstanceID: ownerID})
	srv.Start()
	t.Cleanup(srv.Close)
	if err = lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: srv.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	session, callContext := compatibilitySession(t, root)
	requestID := domain.NewID()
	firstInput := map[string]any{"requestId": requestID, "projectId": project.Project.ID, "originalText": "Evaluate a fake fixture task", "executionMode": "fake", "acceptance": map[string]any{"deliverable": "response_only"}}
	explicitInput := map[string]any{"requestId": requestID, "projectId": project.Project.ID, "originalText": "Evaluate a fake fixture task", "executionMode": "fake", "fakeScenario": "response", "timeoutSeconds": 1800, "pins": map[string]any{}, "acceptance": map[string]any{"deliverable": "response_only", "conditionsText": "", "requiredEvidence": []any{}, "allowCandidatePreparation": true}}
	call := func(in map[string]any) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(callContext, &mcp.CallToolParams{Name: "council_tasks_start", Arguments: map[string]any{"input": in}})
		if err != nil || result == nil {
			t.Fatalf("MCP task call: %v", err)
		}
		return result
	}
	first := call(firstInput)
	if first.IsError {
		raw, _ := json.Marshal(first)
		t.Fatalf("fake task admission failed: %s", raw)
	}
	second := call(explicitInput)
	if second.IsError {
		raw, _ := json.Marshal(second)
		t.Fatalf("equivalent explicit defaults failed: %s", raw)
	}
	type resultShape struct {
		Status   int `json:"status"`
		Envelope struct {
			Data    json.RawMessage `json:"data"`
			Receipt *domain.Receipt `json:"receipt"`
		} `json:"envelope"`
	}
	var a, b resultShape
	rawFirst, _ := json.Marshal(first.StructuredContent)
	rawSecond, _ := json.Marshal(second.StructuredContent)
	var dataFirst, dataSecond any
	if json.Unmarshal(rawFirst, &a) != nil || json.Unmarshal(rawSecond, &b) != nil || a.Envelope.Receipt == nil || b.Envelope.Receipt == nil || a.Status != b.Status || !reflect.DeepEqual(a.Envelope.Receipt, b.Envelope.Receipt) || json.Unmarshal(a.Envelope.Data, &dataFirst) != nil || json.Unmarshal(b.Envelope.Data, &dataSecond) != nil || !reflect.DeepEqual(dataFirst, dataSecond) {
		t.Fatal("canonical default retry did not retain the same receipt and admission")
	}
	page, err := db.Tasks(ctx, project.Project.ID, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("default retry produced additional task: %d, %v", len(page.Items), err)
	}
}
