package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type restartFixtureOwner struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	desc localstate.Descriptor
}

// The owner, both MCP hosts, CLI and fake worker are actual subprocesses. A
// loopback-only fixture proxy drops the first acknowledgement after the actual
// owner has returned a committed response. The restart is orderly, not a crash.
func TestMCPLostAcknowledgementAcrossRealOwnerRestart(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v %s", err, out)
	}
	ownerHome, proxyHome, projectDir := t.TempDir(), t.TempDir(), t.TempDir()
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ownerEnv := append(os.Environ(), "HOME="+ownerHome, "USERPROFILE="+ownerHome, "GIT_CONFIG_GLOBAL="+config, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	proxyEnv := append(os.Environ(), "HOME="+proxyHome, "USERPROFILE="+proxyHome, "GIT_CONFIG_GLOBAL="+config, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		cmd.Env = ownerEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture Git: %v %s", err, out)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	ownerRoot := filepath.Join(ownerHome, ".council", "v2")
	startOwner := func() *restartFixtureOwner {
		t.Helper()
		cmd := exec.Command(binary, "ui", "--no-open", "--json")
		cmd.Env, cmd.Dir, cmd.Stdout, cmd.Stderr = ownerEnv, projectDir, io.Discard, io.Discard
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		owner := &restartFixtureOwner{cmd: cmd, done: make(chan struct{})}
		go func() { owner.err = cmd.Wait(); close(owner.done) }()
		t.Cleanup(func() {
			select {
			case <-owner.done:
				return
			default:
				_ = owner.cmd.Process.Kill()
				<-owner.done
			}
		})
		deadline := time.NewTimer(20 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(25 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-owner.done:
				t.Fatal("fixture owner exited before readiness")
			case <-deadline.C:
				t.Fatal("fixture owner did not become ready")
			case <-tick.C:
				desc, err := localstate.ReadDescriptor(ownerRoot)
				if err == nil && desc.PID == cmd.Process.Pid && domain.ValidID(desc.OwnerInstanceID) {
					owner.desc = desc
					return owner
				}
			}
		}
	}
	cli := func(args ...string) domain.Envelope {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append(args, "--json")...)
		cmd.Env, cmd.Dir = ownerEnv, projectDir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("fixture CLI failed: %v", err)
		}
		var envelope domain.Envelope
		if json.Unmarshal(out, &envelope) != nil || envelope.Error != nil {
			t.Fatal("fixture CLI did not return a successful envelope")
		}
		return envelope
	}
	owner1 := startOwner()
	verifyLoopback := func(desc localstate.Descriptor) {
		t.Helper()
		u, err := url.Parse(desc.Origin)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			t.Fatal("fixture owner did not publish an exact loopback origin")
		}
	}
	verifyLoopback(owner1.desc)
	opened := cli("projects", "open", projectDir)
	projectRaw, _ := json.Marshal(opened.Data)
	var project domain.ProjectOpen
	if json.Unmarshal(projectRaw, &project) != nil || !domain.ValidID(project.Project.ID) {
		t.Fatal("fixture project invalid")
	}
	proxyRoot := filepath.Join(proxyHome, ".council", "v2")
	proxyLock, err := localstate.Acquire(proxyRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxyLock.Close() })
	proxyToken := strings.Repeat("p", 64)
	var upstreamMu sync.Mutex
	upstream := owner1.desc
	allowedOrigins := map[string]bool{owner1.desc.Origin: true}
	seenOrigins := map[string]int{}
	var postCount atomic.Int32
	var savedMu sync.Mutex
	var saved []byte
	upstreamClient := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 25 * time.Second}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+proxyToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		upstreamMu.Lock()
		desc := upstream
		allowed := allowedOrigins[desc.Origin]
		if allowed {
			seenOrigins[desc.Origin]++
		}
		upstreamMu.Unlock()
		if !allowed {
			t.Error("fixture proxy refused an uncaptured origin")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), r.Method, desc.Origin+r.URL.RequestURI(), r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		request.Header.Set("Authorization", "Bearer "+desc.Token)
		request.Header.Set("Origin", desc.Origin)
		if contentType := r.Header.Get("Content-Type"); contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		response, err := upstreamClient.Do(request)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			w.WriteHeader(502)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/v1/tasks" && postCount.Add(1) == 1 {
			if response.StatusCode != 201 {
				t.Errorf("actual owner admission status %d", response.StatusCode)
			}
			savedMu.Lock()
			saved = append([]byte(nil), body...)
			savedMu.Unlock()
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("fixture proxy cannot drop response")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Error("fixture proxy cannot hijack response")
				return
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(body)
	}))
	t.Cleanup(proxy.Close)
	publishProxyDescriptor := func(desc localstate.Descriptor) {
		t.Helper()
		if err := proxyLock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: desc.OwnerInstanceID, PID: desc.PID, StartedAt: desc.StartedAt, Origin: proxy.URL, APIVersion: 1, Token: proxyToken}); err != nil {
			t.Fatal(err)
		}
	}
	publishProxyDescriptor(owner1.desc)
	connect := func() *reconnectHost {
		t.Helper()
		cmd := exec.Command(binary, "mcp")
		cmd.Env, cmd.Dir, cmd.Stderr = proxyEnv, projectDir, io.Discard
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		trace := &reconnectTrace{WriteCloser: stdin}
		client := mcp.NewClient(&mcp.Implementation{Name: "owner-restart-fixture", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdout, Writer: trace}, nil)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("MCP connect: %v", err)
		}
		host := &reconnectHost{cmd: cmd, session: session, trace: trace}
		t.Cleanup(func() {
			if err := host.close(); err != nil {
				t.Errorf("fixture host close: %v", err)
			}
		})
		return host
	}
	requestID := domain.NewID()
	input := map[string]any{"requestId": requestID, "projectId": project.Project.ID, "originalText": "Keep this fake admission across restart", "executionMode": "fake", "fakeScenario": "response", "workflow": map[string]any{"mode": "author_only"}, "acceptance": map[string]any{"deliverable": "response_only"}}
	args := map[string]any{"input": input}
	first := connect()
	result, err := first.session.CallTool(ctx, &mcp.CallToolParams{Name: "council_tasks_start", Arguments: args})
	if err != nil || result == nil || !result.IsError {
		t.Fatal("lost acknowledgement was not uncertain")
	}
	encoded, _ := json.Marshal(result)
	if !bytes.Contains(encoded, []byte("write_uncertain")) || !bytes.Contains(encoded, []byte(requestID)) {
		t.Fatal("uncertainty did not retain request identity")
	}
	if err := first.close(); err != nil {
		t.Fatalf("first host close: %v", err)
	}
	firstIDs := first.trace.callIDs()
	if len(firstIDs) != 1 {
		t.Fatal("unexpected first host call count")
	}
	savedMu.Lock()
	ownerReply := append([]byte(nil), saved...)
	savedMu.Unlock()
	var original domain.Envelope
	if json.Unmarshal(ownerReply, &original) != nil || original.Receipt == nil || original.RequestID != requestID {
		t.Fatal("actual owner receipt missing")
	}
	raw, _ := json.Marshal(original.Data)
	var admission domain.Admission
	if json.Unmarshal(raw, &admission) != nil || !domain.ValidID(admission.Task.ID) || !domain.ValidID(admission.Attempt.ID) {
		t.Fatal("actual owner admission identities missing")
	}
	deadline := time.Now().Add(25 * time.Second)
	var settled domain.Attempt
	for time.Now().Before(deadline) {
		envelope := cli("attempts", "get", admission.Attempt.ID)
		data, _ := json.Marshal(envelope.Data)
		if json.Unmarshal(data, &settled) != nil {
			t.Fatal("attempt data invalid")
		}
		if settled.Phase == "settled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if settled.Phase != "settled" || settled.ExecutionMode != "fake" || settled.Outcome == nil || *settled.Outcome != "succeeded" {
		t.Fatal("fake worker did not settle before restart")
	}
	cli("service", "stop", "--owner-instance", owner1.desc.OwnerInstanceID, "--request-id", domain.NewID())
	select {
	case <-owner1.done:
		if owner1.err != nil {
			t.Fatalf("old owner exited unsuccessfully: %v", owner1.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("old owner did not stop")
	}
	owner2 := startOwner()
	verifyLoopback(owner2.desc)
	if owner2.desc.OwnerInstanceID == owner1.desc.OwnerInstanceID || owner2.desc.PID == owner1.desc.PID {
		t.Fatal("owner subprocess did not restart")
	}
	upstreamMu.Lock()
	allowedOrigins[owner2.desc.Origin] = true
	upstream = owner2.desc
	upstreamMu.Unlock()
	publishProxyDescriptor(owner2.desc)
	before := cli("tasks", "attempts", admission.Task.ID)
	beforeRaw, _ := json.Marshal(before.Data)
	var beforeAttempts domain.Page[domain.Attempt]
	if json.Unmarshal(beforeRaw, &beforeAttempts) != nil || len(beforeAttempts.Items) != 1 || beforeAttempts.Items[0].ID != admission.Attempt.ID {
		t.Fatal("owner restart replayed an attempt")
	}
	second := connect()
	if _, err := second.session.CallTool(ctx, &mcp.CallToolParams{Name: "council_owner_meta", Arguments: map[string]any{}}); err != nil {
		t.Fatal("restarted owner not reachable")
	}
	upstreamMu.Lock()
	bothOwnersObserved := seenOrigins[owner1.desc.Origin] > 0 && seenOrigins[owner2.desc.Origin] > 0 && len(seenOrigins) <= 2
	upstreamMu.Unlock()
	if !bothOwnersObserved {
		t.Fatal("proxy did not use only the captured loopback owner origins")
	}
	retry, err := second.session.CallTool(ctx, &mcp.CallToolParams{Name: "council_tasks_start", Arguments: args})
	if err != nil || retry == nil || retry.IsError {
		t.Fatal("same intent did not replay after owner restart")
	}
	retryRaw, _ := json.Marshal(retry.StructuredContent)
	var replay reconnectPayload
	if json.Unmarshal(retryRaw, &replay) != nil || replay.Envelope.RequestID != requestID || !reflect.DeepEqual(replay.Envelope.Receipt, original.Receipt) {
		t.Fatal("restart changed the retained receipt")
	}
	var retriedAdmission domain.Admission
	if json.Unmarshal(replay.Envelope.Data, &retriedAdmission) != nil || retriedAdmission.Task.ID != admission.Task.ID || retriedAdmission.Attempt.ID != admission.Attempt.ID {
		t.Fatal("restart changed admission identities")
	}
	secondIDs := second.trace.callIDs()
	if len(secondIDs) != 2 || secondIDs[1] == firstIDs[0] {
		t.Fatal("retry did not use a distinct JSON-RPC identity")
	}
	after := cli("tasks", "attempts", admission.Task.ID)
	afterRaw, _ := json.Marshal(after.Data)
	var afterAttempts domain.Page[domain.Attempt]
	if json.Unmarshal(afterRaw, &afterAttempts) != nil || len(afterAttempts.Items) != 1 || afterAttempts.Items[0].ID != admission.Attempt.ID || postCount.Load() != 2 {
		t.Fatal("retry or restart created extra attempts")
	}
	cliReplay := cli("tasks", "start", "--project", project.Project.ID, "--task", "Keep this fake admission across restart", "--fake", "--fake-scenario", "response", "--workflow", "author_only", "--deliverable", "response_only", "--request-id", requestID)
	if !reflect.DeepEqual(cliReplay.Receipt, original.Receipt) {
		t.Fatal("CLI did not retain original receipt after restart")
	}
	if err := second.close(); err != nil {
		t.Fatalf("second host close: %v", err)
	}
	cli("service", "stop", "--owner-instance", owner2.desc.OwnerInstanceID, "--request-id", domain.NewID())
	select {
	case <-owner2.done:
		if owner2.err != nil {
			t.Fatalf("new owner exited unsuccessfully: %v", owner2.err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("new owner did not stop")
	}
}
