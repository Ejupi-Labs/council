package mcphost

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise the guide's shell-free executable/argument setup with a real SDK
// client and a path containing both spaces and Unicode. No owner is started.
func TestMCPGuideExecutablePathWithSpacesAndUnicode(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "Council setup 東京")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "mcp")
	cmd.Dir, cmd.Env, cmd.Stderr = repo, append(os.Environ(), "HOME="+home, "USERPROFILE="+home), &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "guide-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("guide setup failed: %v", err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != len(ToolCatalog()) {
		t.Fatalf("guide tool discovery failed: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal("guide client did not close")
	}
	if strings.Contains(strings.ToLower(stderr.String()), "token") {
		t.Fatal("private material in diagnostics")
	}
	if _, err := os.Stat(filepath.Join(home, ".council")); !os.IsNotExist(err) {
		t.Fatal("tool discovery unexpectedly started an owner or touched state")
	}
	invalid := exec.Command(binary, "mcp", "--unexpected")
	invalid.Dir, invalid.Env = repo, append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	var stdout, diagnostic bytes.Buffer
	invalid.Stdout, invalid.Stderr = &stdout, &diagnostic
	err = invalid.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 || stdout.Len() != 0 || !strings.Contains(diagnostic.String(), "Usage: council mcp") {
		t.Fatal("invalid invocation did not produce the expected stderr-only usage")
	}
}
