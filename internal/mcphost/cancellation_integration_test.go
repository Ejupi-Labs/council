package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Record only IDs of real SDK wait requests and cancellation notifications;
// never retain task content, owner tokens or arbitrary protocol payloads.
type cancellationWire struct {
	io.WriteCloser
	mu            sync.Mutex
	pending       []byte
	waits         chan string
	cancellations chan string
}

func (w *cancellationWire) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.WriteCloser.Write(p)
	w.pending = append(w.pending, p[:n]...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		var frame struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
			Params struct {
				Name      string          `json:"name"`
				RequestID json.RawMessage `json:"requestId"`
			} `json:"params"`
		}
		if json.Unmarshal(w.pending[:end], &frame) == nil {
			switch {
			case frame.Method == "tools/call" && frame.Params.Name == "council_attempts_wait":
				select {
				case w.waits <- string(frame.ID):
				default:
				}
			case frame.Method == "notifications/cancelled":
				select {
				case w.cancellations <- string(frame.Params.RequestID):
				default:
				}
			}
		}
		w.pending = append(w.pending[:0], w.pending[end+1:]...)
	}
	return n, err
}

type cancellationHost struct {
	session  *mcp.ClientSession
	cmd      *exec.Cmd
	wire     *cancellationWire
	once     sync.Once
	closeErr error
}

func (h *cancellationHost) close() error {
	h.once.Do(func() {
		// Explicit EOF is the disconnect under test. Close stdin before the SDK
		// reader: on Windows, closing a pipe reader with a pending read can wait
		// until the peer closes its write end.
		_ = h.wire.Close()
		done := make(chan error, 1)
		go func() {
			_ = h.session.Close()
			done <- h.cmd.Wait()
		}()
		select {
		case err := <-done:
			h.closeErr = err
		case <-time.After(8 * time.Second):
			_ = h.cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
			h.closeErr = errors.New("fixture host did not exit promptly after EOF")
		}
	})
	return h.closeErr
}

// This exercises the real SDK, stdio host, owner and one controlled fake
// worker. Cancellation and EOF are observational; only explicit Stop mutates.
func TestMCPWaitCancellationAndDisconnectKeepOwnedWork(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home, project := t.TempDir(), t.TempDir()
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture binary: %v: %s", err, output)
	}
	gitConfig := filepath.Join(home, "empty-gitconfig")
	if err := os.WriteFile(gitConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git setup: %v: %s", err, output)
		}
	}
	root := filepath.Join(home, ".council", "v2")
	var ownerID string
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil {
			if ownerID != "" {
				t.Error("fixture owner descriptor disappeared before cleanup")
			}
			return
		}
		if ownerID == "" {
			ownerID = desc.OwnerInstanceID
		}
		if !domain.ValidID(ownerID) || desc.OwnerInstanceID != ownerID {
			t.Error("fixture owner identity changed before cleanup")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "service", "stop", "--owner-instance", ownerID, "--request-id", domain.NewID(), "--json")
		cmd.Env, cmd.Dir = env, project
		if err := cmd.Run(); err != nil {
			t.Errorf("fixture owner stop: %v", err)
			return
		}
		for ctx.Err() == nil {
			lock, err := localstate.Acquire(root)
			if err == nil {
				_ = lock.Close()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("fixture owner did not release lock")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	connect := func() *cancellationHost {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, "mcp")
		cmd.Env, cmd.Dir, cmd.Stderr = env, project, io.Discard
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		wire := &cancellationWire{WriteCloser: stdin, waits: make(chan string, 4), cancellations: make(chan string, 4)}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "council-cancellation-journey", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdout, Writer: wire}, nil)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("SDK stdio connection: %v", err)
		}
		h := &cancellationHost{session: session, cmd: cmd, wire: wire}
		t.Cleanup(func() { _ = h.close() })
		return h
	}
	type response struct {
		Status   int `json:"status"`
		Envelope struct {
			Data    json.RawMessage `json:"data"`
			Receipt *domain.Receipt `json:"receipt"`
			Error   *domain.Error   `json:"error"`
		} `json:"envelope"`
	}
	call := func(host *cancellationHost, name string, args map[string]any, target any) response {
		t.Helper()
		result, err := host.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s transport: %v", name, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal("invalid structured result")
		}
		var out response
		if json.Unmarshal(raw, &out) != nil || result.IsError || out.Envelope.Error != nil || out.Status < 200 || out.Status >= 300 {
			t.Fatalf("%s owner status=%d error=%v", name, out.Status, out.Envelope.Error)
		}
		if target != nil && json.Unmarshal(out.Envelope.Data, target) != nil {
			t.Fatalf("%s data shape", name)
		}
		return out
	}
	first := connect()
	call(first, "council_owner_meta", map[string]any{}, nil)
	desc, err := localstate.ReadDescriptor(root)
	if err != nil || !domain.ValidID(desc.OwnerInstanceID) {
		t.Fatal("fixture owner did not start")
	}
	ownerID = desc.OwnerInstanceID
	var opened domain.ProjectOpen
	call(first, "council_projects_open", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "selectedPath": project, "basePath": project}}, &opened)
	var admitted domain.Admission
	call(first, "council_tasks_start", map[string]any{"input": map[string]any{
		"requestId": domain.NewID(), "projectId": opened.Project.ID, "originalText": "Controlled slow cancellation fixture",
		"executionMode": "fake", "fakeScenario": "slow", "timeoutSeconds": 120,
	}}, &admitted)
	if admitted.Attempt.ExecutionMode != "fake" || admitted.Attempt.FakeScenario != "slow" {
		t.Fatal("the worker is not the explicit slow fake")
	}
	var running domain.Attempt
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		call(first, "council_attempts_get", map[string]any{"id": admitted.Attempt.ID}, &running)
		if running.Phase == "running" {
			break
		}
		if running.Phase == "settled" {
			t.Fatal("slow fake settled before cancellation")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if running.Phase != "running" || running.Stop != nil {
		t.Fatal("slow fake was not independently running")
	}
	type waitResult struct {
		result *mcp.CallToolResult
		err    error
	}
	startWait := func(host *cancellationHost, waitCtx context.Context) <-chan waitResult {
		t.Helper()
		result := make(chan waitResult, 1)
		go func() {
			value, err := host.session.CallTool(waitCtx, &mcp.CallToolParams{Name: "council_attempts_wait", Arguments: map[string]any{"id": admitted.Attempt.ID, "afterRevision": "9223372036854775807", "timeoutMs": "30000"}})
			result <- waitResult{value, err}
		}()
		return result
	}
	waitCtx, cancelWait := context.WithCancel(ctx)
	firstWait := startWait(first, waitCtx)
	var waitID string
	select {
	case waitID = <-first.wire.waits:
	case <-ctx.Done():
		t.Fatal("wait was never sent")
	}
	select {
	case <-firstWait:
		t.Fatal("wait finished before cancellation")
	case <-time.After(150 * time.Millisecond):
	}
	cancelWait()
	select {
	case result := <-firstWait:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("wait did not report cancellation: %v", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled wait did not return")
	}
	select {
	case id := <-first.wire.cancellations:
		if id != waitID {
			t.Fatal("SDK cancelled a different JSON-RPC request")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SDK did not send the cancellation notification")
	}
	call(first, "council_attempts_get", map[string]any{"id": admitted.Attempt.ID}, &running)
	if running.Phase != "running" || running.Stop != nil {
		t.Fatal("cancelling the observer changed owned work")
	}
	secondWait := startWait(first, ctx)
	select {
	case <-first.wire.waits:
	case <-ctx.Done():
		t.Fatal("second wait was never sent")
	}
	select {
	case <-secondWait:
		t.Fatal("second wait finished before disconnect")
	case <-time.After(150 * time.Millisecond):
	}
	if err := first.close(); err != nil {
		t.Fatalf("first host did not close: %v", err)
	}
	select {
	case <-secondWait:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnected wait did not return")
	}
	after, err := localstate.ReadDescriptor(root)
	if err != nil || after.OwnerInstanceID != ownerID || after.PID != desc.PID {
		t.Fatal("host disconnect changed the owner")
	}
	second := connect()
	call(second, "council_attempts_get", map[string]any{"id": admitted.Attempt.ID}, &running)
	if running.Phase != "running" || running.Stop != nil {
		t.Fatal("host disconnect stopped or settled owned work")
	}
	stopID := domain.NewID()
	var stop struct {
		AttemptID    string `json:"attemptId"`
		StopAccepted bool   `json:"stopAccepted"`
	}
	stopResult := call(second, "council_attempts_stop", map[string]any{"id": admitted.Attempt.ID, "input": map[string]any{"requestId": stopID}}, &stop)
	if stopResult.Status != 202 || !stop.StopAccepted || stop.AttemptID != admitted.Attempt.ID || stopResult.Envelope.Receipt == nil || stopResult.Envelope.Receipt.RequestID != stopID {
		t.Fatal("explicit stop was not accepted for the exact attempt")
	}
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		call(second, "council_attempts_get", map[string]any{"id": admitted.Attempt.ID}, &running)
		if running.Phase == "settled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if running.Phase != "settled" || running.Outcome == nil || *running.Outcome != "cancelled" || running.Stop == nil || running.Stop.RequestID != stopID || running.Cleanup.State != "verified" || running.PrimaryFailure != nil || running.EvidenceStatus == "uncertain" {
		t.Fatal("explicit stop did not retain cancelled settlement and verified cleanup")
	}
	var events domain.Page[domain.AttemptEvent]
	call(second, "council_attempts_events", map[string]any{"id": admitted.Attempt.ID, "limit": "100"}, &events)
	if events.NextCursor != nil {
		t.Fatal("fixture stop evidence exceeded one bounded page")
	}
	accepted := 0
	for _, event := range events.Items {
		if event.Kind == "stop_accepted" {
			accepted++
			if event.AttemptID != admitted.Attempt.ID || event.Sequence != running.Stop.EventSequence {
				t.Fatal("stop event does not match retained authority")
			}
		}
	}
	if accepted != 1 {
		t.Fatalf("expected exactly one retained stop event, got %d", accepted)
	}
	if err := second.close(); err != nil {
		t.Fatalf("second host did not close: %v", err)
	}
}
