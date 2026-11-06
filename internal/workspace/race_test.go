package workspace

import (
	"context"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRacingSourceHasFiniteRetriesAndNoOldBaseFallback(t *testing.T) {
	m, repo, _ := fixture(t)
	sink := &fixtureSink{rows: map[string]evidence.Metadata{}}
	a, e := evidence.New(m.lock, sink)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	m.artifacts = a
	count := 0
	sink.hook = func(v evidence.Metadata) {
		if v.Kind == "source_file" && v.Closed {
			count++
			os.WriteFile(filepath.Join(repo, "file.bin"), []byte(strings.Repeat("changed", count)), 0644)
		}
	}
	_, e = m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
	if !errors.Is(e, ErrBlocked) || !strings.Contains(e.Error(), "three capture") {
		t.Fatalf("racing source: %v", e)
	}
	entries, e := os.ReadDir(filepath.Join(m.lock.Root(), "workspaces"))
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 0 {
		t.Fatal("racing source materialized an old clean base")
	}
	if count < 3 || count > 20 {
		t.Fatalf("unbounded/absent local retry: %d", count)
	}
}

func TestSparseUnmergedSubmoduleAndUnbornBlock(t *testing.T) {
	t.Run("sparse flags", func(t *testing.T) {
		m, repo, _ := fixture(t)
		runGit(t, repo, nil, "update-index", "--skip-worktree", "file.bin")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "Sparse") {
			t.Fatalf("unexpected: %v", e)
		}
	})
	t.Run("unmerged", func(t *testing.T) {
		m, repo, _ := fixture(t)
		obj := strings.TrimSpace(string(runGit(t, repo, []byte("conflict"), "hash-object", "-w", "--stdin")))
		runGit(t, repo, []byte("0 0000000000000000000000000000000000000000\tfile.bin\n100644 "+obj+" 1\tfile.bin\n100644 "+obj+" 2\tfile.bin\n"), "update-index", "--index-info")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "Unmerged") {
			t.Fatalf("unexpected: %v", e)
		}
	})
	t.Run("submodule", func(t *testing.T) {
		m, repo, _ := fixture(t)
		head := strings.TrimSpace(string(runGit(t, repo, nil, "rev-parse", "HEAD")))
		runGit(t, repo, nil, "update-index", "--add", "--cacheinfo", "160000,"+head+",submodule")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "Submodules") {
			t.Fatalf("unexpected: %v", e)
		}
	})
	t.Run("unborn", func(t *testing.T) {
		m, _, _ := fixture(t)
		repo := t.TempDir()
		runGit(t, repo, nil, "init", "-b", "main")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "Unborn") {
			t.Fatalf("unexpected: %v", e)
		}
	})
}
