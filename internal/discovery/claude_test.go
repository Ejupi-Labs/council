package discovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestClaudeDocumentedNativeMetadataBeatsWrapperWithoutExecution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	native := filepath.Join(home, ".local", "bin", name)
	if err := os.MkdirAll(filepath.Dir(native), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("authored native metadata sentinel; never execute"), 0700); err != nil {
		t.Fatal(err)
	}
	wrappers := t.TempDir()
	if err := os.WriteFile(filepath.Join(wrappers, "claude.cmd"), []byte("launcher contents must not be read"), 0600); err != nil {
		t.Fatal(err)
	}
	scanner := Scanner{Path: wrappers, Native: func(id string) ([]string, error) {
		if id == "claude-code" {
			return nativeCandidates(id)
		}
		return nil, nil
	}}
	result, err := scanner.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	claude := result.Observations[1].Observation
	if claude == nil || claude.LauncherPath != native || claude.InstallationStatus != "installed" || claude.ManagedExecution != "native_supported_live_unqualified" || claude.Authentication != "unknown" || claude.Model != "unknown" || claude.Allowance != "unknown" {
		t.Fatalf("native metadata was confused with wrapper compatibility or authentication: %+v", claude)
	}
}
