package discovery

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFilesystemOnlyFiveOrderedUnknownObservations(t *testing.T) {
	dir := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	sentinel := filepath.Join(dir, "called")
	for _, name := range []string{"codex", "claude", "dsh", "junie", "agy"} {
		if err := os.WriteFile(filepath.Join(dir, name+suffix), []byte("#!/bin/sh\necho called > '"+filepath.ToSlash(sentinel)+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	s := Scanner{Path: dir, Native: func(string) ([]string, error) { return nil, nil }}
	got, err := s.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex", "claude-code", "deepseek-harness", "junie", "antigravity"}
	if len(got.Observations) != len(want) {
		t.Fatal("not exactly five observations")
	}
	for n, e := range got.Observations {
		if e.HarnessID != want[n] || e.Error != nil || e.Observation == nil {
			t.Fatalf("wrong ordered observation: %+v", e)
		}
		o := e.Observation
		managed, action := "not_implemented", "await_managed_execution"
		if e.HarnessID == "codex" {
			managed, action = "native_supported_live_unqualified", "start_explicit_task"
		}
		if o.InstallationStatus != "installed" || o.Authentication != "unknown" || o.Account != "unknown" || o.Provider != "unknown" || o.Model != "unknown" || o.EffectivePermissions != "unknown" || o.ExternalOccupancy != "unknown" || o.Allowance != "unknown" || o.Usage != "unknown" || o.ManagedExecution != managed || o.NextAction != action {
			t.Fatalf("invented readiness: %+v", o)
		}
	}
	if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("discovery invoked a launcher")
	}
}

func TestMissingUnsupportedAndPerHarnessError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude.cmd"), []byte("sensitive launcher contents must not be read"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "codex.sh"), []byte("sensitive launcher contents must not be read"), 0700); err != nil {
		t.Fatal(err)
	}
	s := Scanner{Path: dir, Native: func(id string) ([]string, error) {
		if id == "junie" {
			return nil, fs.ErrPermission
		}
		return nil, nil
	}}
	got, err := s.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range []string{"unsupported_launcher", "unsupported_launcher", "missing", "error", "missing"} {
		e := got.Observations[n]
		if want == "error" {
			if e.Error == nil || e.Observation != nil || e.Error.Code != "discovery_unavailable" {
				t.Fatal("metadata error became false missing")
			}
		} else if e.Error != nil || e.Observation == nil || e.Observation.InstallationStatus != want {
			t.Fatalf("wrong state for %s: %+v", e.HarnessID, e)
		}
	}
}

func TestNativeCandidateAndBounds(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "native.exe")
	if err := os.WriteFile(file, []byte("not executed"), 0700); err != nil {
		t.Fatal(err)
	}
	s := Scanner{Path: "", Native: func(id string) ([]string, error) {
		if id == "codex" {
			return []string{file}, nil
		}
		return nil, nil
	}}
	got, err := s.Discover(context.Background())
	if err != nil || got.Observations[0].Observation.InstallationStatus != "installed" {
		t.Fatalf("native metadata ignored: %v", err)
	}
	s.Path = string(make([]byte, 1024*1024+1))
	if _, err := s.Discover(context.Background()); err == nil {
		t.Fatal("unbounded PATH accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Discover(ctx); err == nil {
		t.Fatal("cancelled scan reported success")
	}
}
