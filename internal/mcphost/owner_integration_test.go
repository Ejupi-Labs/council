package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This uses the real binary and only a disposable home. No provider is called.
func TestHostSubprocessBootstrapsAndReusesOwner(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture binary: %v: %s", err, output)
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	root := filepath.Join(home, ".council", "v2")
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil || desc.OwnerInstanceID == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "service", "stop", "--request-id", domain.NewID(), "--json")
		cmd.Env, cmd.Dir = env, repo
		_ = cmd.Run()
		for ctx.Err() == nil {
			lock, err := localstate.Acquire(root)
			if err == nil {
				_ = lock.Close()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("fixture owner did not release its private lock")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var stderr1, stderr2 bytes.Buffer
	connect := func(stderr *bytes.Buffer) *mcp.ClientSession {
		t.Helper()
		cmd := exec.Command(binary, "mcp")
		cmd.Env, cmd.Dir, cmd.Stderr = env, repo, stderr
		client := mcp.NewClient(&mcp.Implementation{Name: "council-fixture", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
		if err != nil {
			t.Fatalf("SDK could not initialize stdio host: %v", err)
		}
		return session
	}
	call := func(session *mcp.ClientSession) []byte {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "council_owner_meta", Arguments: map[string]any{}})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("owner metadata tool failed: %v", err)
		}
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal("cannot encode tool result")
		}
		return data
	}
	first := connect(&stderr1)
	defer first.Close()
	firstResult := call(first)
	before, err := localstate.ReadDescriptor(root)
	if err != nil || before.OwnerInstanceID == "" {
		t.Fatal("host did not create its private fixture owner")
	}
	second := connect(&stderr2)
	defer second.Close()
	secondResult := call(second)
	after, err := localstate.ReadDescriptor(root)
	if err != nil || after.OwnerInstanceID != before.OwnerInstanceID || after.PID != before.PID {
		t.Fatal("second host did not reuse the first owner")
	}
	if bytes.Contains(firstResult, []byte(before.Token)) || bytes.Contains(secondResult, []byte(before.Token)) {
		t.Fatal("owner token leaked into MCP result")
	}
	if err := first.Close(); err != nil {
		t.Fatal("first host did not close cleanly")
	}
	_ = call(second)
	afterEOF, err := localstate.ReadDescriptor(root)
	if err != nil || afterEOF.OwnerInstanceID != before.OwnerInstanceID || afterEOF.PID != before.PID {
		t.Fatal("host EOF stopped or replaced the shared owner")
	}
	if err := second.Close(); err != nil {
		t.Fatal("second host did not close cleanly")
	}
	if strings.Contains(stderr1.String(), before.Token) || strings.Contains(stderr2.String(), before.Token) {
		t.Fatal("owner token leaked into diagnostics")
	}
}
