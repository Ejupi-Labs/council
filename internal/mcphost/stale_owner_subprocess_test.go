package mcphost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

// A real host subprocess must not send a mutation to an authenticated server
// whose identity differs from the still-locked descriptor, or replace it.
func TestMCPSubprocessRejectsStaleOwnerIdentity(t *testing.T) {
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
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture binary build: %v: %s", err, output)
	}

	home := t.TempDir()
	root := filepath.Join(home, ".council", "v2")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	actualID, staleID := domain.NewID(), domain.NewID()
	if actualID == staleID {
		t.Fatal("test fixture identities unexpectedly collided")
	}
	token := strings.Repeat("d", 64) // Only a generated test token, never vendor credentials.
	spy := &parityExecutor{ownerID: actualID}
	var metadata, forwarded, badAuthentication atomic.Int32
	server := httptest.NewUnstartedServer(nil)
	authenticated := httpapi.New(spy, httpapi.Config{Host: server.Listener.Addr().String(), Token: token, OwnerInstanceID: actualID})
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			badAuthentication.Add(1)
		}
		if r.URL.Path == "/api/v1/meta" {
			metadata.Add(1)
		} else {
			forwarded.Add(1)
		}
		authenticated.ServeHTTP(w, r)
	})
	server.Start()
	t.Cleanup(server.Close)
	desc := localstate.Descriptor{OwnerInstanceID: staleID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}
	if err := lock.WriteDescriptor(desc); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "mcp")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	lines := make(chan []byte, 8)
	scanDone := make(chan error, 1)
	var wire bytes.Buffer
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			wire.Write(line)
			wire.WriteByte('\n')
			lines <- line
		}
		scanDone <- scanner.Err()
	}()
	read := func() map[string]json.RawMessage {
		t.Helper()
		select {
		case line := <-lines:
			var frame map[string]json.RawMessage
			if json.Unmarshal(line, &frame) != nil {
				t.Fatal("stdout emitted a non-protocol frame")
			}
			return frame
		case <-ctx.Done():
			t.Fatal("timed out waiting for MCP frame")
			return nil
		}
	}
	write := func(frame string) {
		t.Helper()
		if _, err := fmt.Fprintln(stdin, frame); err != nil {
			t.Fatal("MCP stdin write", err)
		}
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"stale-owner-fixture","version":"1"}}}`)
	initialized := read()
	if string(initialized["id"]) != "1" || initialized["result"] == nil || metadata.Load() != 0 || forwarded.Load() != 0 {
		t.Fatal("initialization unexpectedly touched the owner")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	requestID, projectID := domain.NewID(), domain.NewID()
	write(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"council_drafts_create","arguments":{"input":{"requestId":%q,"projectId":%q,"originalText":"controlled fixture"}}}}`, requestID, projectID))
	frame := read()
	var result struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status   int `json:"status"`
			Envelope struct {
				RequestID string          `json:"requestId"`
				Error     *domain.Error   `json:"error"`
				Receipt   *domain.Receipt `json:"receipt"`
			} `json:"envelope"`
		} `json:"structuredContent"`
	}
	if string(frame["id"]) != "2" || json.Unmarshal(frame["result"], &result) != nil || !result.IsError || result.StructuredContent.Envelope.Error == nil || result.StructuredContent.Envelope.Error.Code != "owner_unavailable" || result.StructuredContent.Envelope.RequestID != requestID || result.StructuredContent.Envelope.Receipt != nil {
		t.Fatal("stale owner did not yield the expected definite owner_unavailable result")
	}
	if metadata.Load() == 0 || forwarded.Load() != 0 || badAuthentication.Load() != 0 {
		t.Fatal("authenticated identity check did not fence the mutation", metadata.Load(), forwarded.Load(), badAuthentication.Load())
	}
	after, err := localstate.ReadDescriptor(root)
	if err != nil || !reflect.DeepEqual(desc, after) {
		t.Fatal("the locked stale descriptor was replaced")
	}
	other, err := localstate.Acquire(root)
	if err == nil {
		_ = other.Close()
		t.Fatal("the original owner lock was not preserved")
	}
	if !errors.Is(err, localstate.ErrLocked) {
		t.Fatal("unexpected fixture lock failure", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal("MCP EOF", err)
	}
	// StdoutPipe must be completely read before Wait closes the pipe.
	// On a stuck or broken stream, kill and reap only our own fixture host.
	select {
	case err := <-scanDone:
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatal("stdout read failed", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("stdout did not close after EOF")
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			t.Fatal("host did not exit cleanly on EOF", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		select {
		case <-wait:
		case <-time.After(5 * time.Second):
		}
		t.Fatal("host did not exit promptly on EOF")
	}
	if bytes.Contains(wire.Bytes(), []byte(token)) || bytes.Contains(stderr.Bytes(), []byte(token)) {
		t.Fatal("private fixture token appeared in host output")
	}
	if len(lines) != 0 {
		t.Fatal("host emitted an unexpected extra protocol frame")
	}
	afterEOF, err := localstate.ReadDescriptor(root)
	if err != nil || !reflect.DeepEqual(desc, afterEOF) || forwarded.Load() != 0 {
		t.Fatal("host EOF replaced the descriptor or sent work")
	}
}
