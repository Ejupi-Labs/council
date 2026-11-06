package workspace

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type fixtureSink struct {
	mu   sync.Mutex
	rows map[string]evidence.Metadata
	hook func(evidence.Metadata)
}

func (s *fixtureSink) SaveArtifact(_ context.Context, m evidence.Metadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[m.ID] = m
	if s.hook != nil {
		s.hook(m)
	}
	return nil
}
func (s *fixtureSink) LoadArtifact(_ context.Context, id string) (evidence.Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.rows[id]
	if !ok {
		return m, os.ErrNotExist
	}
	return m, nil
}

const attemptID = "1234567890abcdef1234567890abcdef"

func runGit(t *testing.T, dir string, input []byte, args ...string) []byte {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + filepath.Join(t.TempDir(), "missing-hooks")}, args...)...)
	c.Dir = dir
	c.Stdin = bytes.NewReader(input)
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %v %s", args, e, b)
	}
	return b
}
func put(t *testing.T, dir, name string, b []byte) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, b, 0644); e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T) (*Manager, string, *localstate.Lock) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "source")
	if e := os.Mkdir(repo, 0700); e != nil {
		t.Fatal(e)
	}
	runGit(t, repo, nil, "init", "-b", "main")
	runGit(t, repo, nil, "config", "user.name", "Local fixture")
	runGit(t, repo, nil, "config", "user.email", "fixture@example.invalid")
	runGit(t, repo, nil, "config", "core.autocrlf", "false")
	put(t, repo, "file.bin", []byte("base\x00\xff\n"))
	put(t, repo, ".gitignore", []byte("ignored/\n"))
	runGit(t, repo, nil, "add", ".")
	runGit(t, repo, nil, "commit", "-m", "fixture")
	l, e := localstate.Acquire(filepath.Join(base, "private"))
	if e != nil {
		t.Fatal(e)
	}
	a, e := evidence.New(l, &fixtureSink{rows: map[string]evidence.Metadata{}})
	if e != nil {
		t.Fatal(e)
	}
	m, e := New(l, a)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Close(); a.Close(); l.Close() })
	return m, repo, l
}

func TestCurrentBytesStagingCandidateAndOriginalPreserved(t *testing.T) {
	m, repo, _ := fixture(t)
	ctx := context.Background()
	staged := []byte("staged\x00\xfe\r\n")
	dirty := []byte("dirty🙂\x00\xff\n")
	put(t, repo, "file.bin", staged)
	runGit(t, repo, nil, "add", "file.bin")
	put(t, repo, "file.bin", dirty)
	put(t, repo, "sub/Grüße🙂.txt", []byte("untracked exact\r\n"))
	put(t, repo, "ignored/omit.txt", []byte("ignored"))
	put(t, repo, ".codex/auth.json", []byte("FAKE DO NOT COPY"))
	indexPath := filepath.Join(repo, ".git", "index")
	originalIndex, e := os.ReadFile(indexPath)
	if e != nil {
		t.Fatal(e)
	}
	head := runGit(t, repo, nil, "rev-parse", "HEAD")
	branch := runGit(t, repo, nil, "symbolic-ref", "HEAD")
	s, e := m.Capture(ctx, Request{AttemptID: attemptID, ProjectPath: repo, SelectedSubfolder: "sub"})
	if e != nil {
		t.Fatal(e)
	}
	if s.ExecutionPath != filepath.Join(s.WorkspacePath, "sub") {
		t.Fatalf("selected mapping %+v", s)
	}
	current, e := os.ReadFile(filepath.Join(s.WorkspacePath, "file.bin"))
	if e != nil || !bytes.Equal(current, dirty) {
		t.Fatalf("working bytes %x %v", current, e)
	}
	got := runGit(t, s.WorkspacePath, nil, "show", ":file.bin")
	if !bytes.Equal(got, staged) {
		t.Fatalf("staged bytes %x != %x", got, staged)
	}
	if _, e = os.Stat(filepath.Join(s.WorkspacePath, "ignored")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("ignored copied: %v", e)
	}
	if _, e = os.Stat(filepath.Join(s.WorkspacePath, ".codex")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("vendor state copied: %v", e)
	}
	c, e := m.Candidate(ctx, s)
	if e != nil || len(c.Changes) != 0 {
		t.Fatalf("pre-existing source became candidate: %+v %v", c, e)
	}
	// Change only the managed index, keeping current working bytes untouched.
	object := strings.TrimSpace(string(runGit(t, s.WorkspacePath, []byte("authored staged bytes\x00"), "hash-object", "-w", "--stdin", "--no-filters")))
	runGit(t, s.WorkspacePath, nil, "update-index", "--cacheinfo", "100644,"+object+",file.bin")
	c, e = m.Candidate(ctx, s)
	if e != nil || len(c.Changes) != 1 || c.Changes[0].Kind != "index_only" {
		t.Fatalf("index-only candidate: %+v %v", c, e)
	}
	put(t, s.WorkspacePath, "new.txt", []byte("worker authored"))
	put(t, s.WorkspacePath, "file.bin", []byte("worker dirty"))
	c, e = m.Candidate(ctx, s)
	if e != nil || len(c.Changes) != 2 {
		t.Fatalf("candidate: %+v %v", c, e)
	}
	after, _ := os.ReadFile(indexPath)
	original, _ := os.ReadFile(filepath.Join(repo, "file.bin"))
	if !bytes.Equal(originalIndex, after) || !bytes.Equal(original, dirty) || !bytes.Equal(head, runGit(t, repo, nil, "rev-parse", "HEAD")) || !bytes.Equal(branch, runGit(t, repo, nil, "symbolic-ref", "HEAD")) {
		t.Fatal("original source/index/branch changed")
	}
}

func TestCaptureDoesNotInvokeFiltersHooksOrFsmonitor(t *testing.T) {
	m, repo, _ := fixture(t)
	marker := filepath.Join(t.TempDir(), "invoked")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	trap := `"` + filepath.ToSlash(exe) + `" -test.run=^TestGitTrap$ -- "` + filepath.ToSlash(marker) + `"`
	put(t, repo, ".gitattributes", []byte("*.bin filter=trap diff=trap\n"))
	runGit(t, repo, nil, "add", ".gitattributes")
	runGit(t, repo, nil, "commit", "-m", "attributes")
	put(t, repo, "file.bin", []byte("staged bytes\x00"))
	runGit(t, repo, nil, "add", "file.bin")
	put(t, repo, "file.bin", []byte("unstaged exact bytes\x00\xff"))
	for _, key := range []string{"filter.trap.clean", "filter.trap.smudge", "diff.trap.command", "diff.trap.textconv", "core.fsmonitor"} {
		runGit(t, repo, nil, "config", key, trap)
	}
	// An active local post-checkout hook is a second independent trap.
	hook := filepath.Join(repo, ".git", "hooks", "post-checkout")
	body := "#!/bin/sh\n" + trap + "\n"
	if e = os.WriteFile(hook, []byte(body), 0755); e != nil {
		t.Fatal(e)
	}
	s, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(marker); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("repository code ran during capture: %v", e)
	}
	got, _ := os.ReadFile(filepath.Join(s.WorkspacePath, "file.bin"))
	if !bytes.Equal(got, []byte("unstaged exact bytes\x00\xff")) {
		t.Fatalf("filter changed bytes: %x", got)
	}
}

func TestGitTrap(t *testing.T) {
	for i, v := range os.Args {
		if v == "--" && i+1 < len(os.Args) {
			os.WriteFile(os.Args[i+1], []byte("invoked"), 0600)
			os.Exit(0)
		}
	}
}

func TestUnsupportedIndexAndSourceStatesBlock(t *testing.T) {
	t.Run("intent-to-add", func(t *testing.T) {
		m, repo, _ := fixture(t)
		put(t, repo, "new.txt", []byte("new"))
		runGit(t, repo, nil, "add", "--intent-to-add", "new.txt")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "Intent-to-add") {
			t.Fatalf("unexpected %v", e)
		}
	})
	t.Run("split", func(t *testing.T) {
		m, repo, _ := fixture(t)
		runGit(t, repo, nil, "update-index", "--split-index")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "Split") {
			t.Fatalf("unexpected %v", e)
		}
	})
	t.Run("file limit", func(t *testing.T) {
		m, repo, _ := fixture(t)
		f, e := os.Create(filepath.Join(repo, "oversize.bin"))
		if e != nil {
			t.Fatal(e)
		}
		e = f.Truncate(MaxFileBytes + 1)
		f.Close()
		if e != nil {
			t.Fatal(e)
		}
		_, e = m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "64 MiB") {
			t.Fatalf("unexpected %v", e)
		}
	})
	t.Run("tracked vendor state", func(t *testing.T) {
		m, repo, _ := fixture(t)
		put(t, repo, ".codex/auth.json", []byte("fake"))
		runGit(t, repo, nil, "add", ".codex/auth.json")
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "excluded private") {
			t.Fatalf("unexpected %v", e)
		}
	})
	t.Run("outside link", func(t *testing.T) {
		m, repo, _ := fixture(t)
		outside := filepath.Join(t.TempDir(), "outside")
		put(t, filepath.Dir(outside), "outside", []byte("private"))
		if e := os.Symlink(outside, filepath.Join(repo, "link")); e != nil {
			t.Skip("symlink privilege unavailable")
		}
		_, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
		if e == nil || !strings.Contains(e.Error(), "symlink") {
			t.Fatalf("unexpected %v", e)
		}
	})
}

func TestSupportedInRootSymlinkAndNoHardlink(t *testing.T) {
	m, repo, _ := fixture(t)
	if e := os.Symlink("file.bin", filepath.Join(repo, "link")); e != nil {
		t.Skip("symlink privilege unavailable")
	}
	s, e := m.Capture(context.Background(), Request{AttemptID: attemptID, ProjectPath: repo})
	if e != nil {
		t.Fatal(e)
	}
	target, e := os.Readlink(filepath.Join(s.WorkspacePath, "link"))
	if e != nil || target != "file.bin" {
		t.Fatalf("symlink: %s %v", target, e)
	}
	a, _ := os.Stat(filepath.Join(repo, "file.bin"))
	b, _ := os.Stat(filepath.Join(s.WorkspacePath, "file.bin"))
	if os.SameFile(a, b) {
		t.Fatal("source and worker share an inode")
	}
}

func TestRawIndexFlagsAndMalformedMetadata(t *testing.T) {
	for _, v := range []uint32{1, 5} {
		b := make([]byte, 32)
		copy(b, "DIRC")
		binary.BigEndian.PutUint32(b[4:8], v)
		if validateRawIndex(b) == nil {
			t.Fatalf("accepted index version %d", v)
		}
	}
	for _, p := range []string{"../escape", "/abs", "a\\b", ".git/index", "a/./b", "a//b"} {
		if safePath(p) {
			t.Fatalf("accepted %s", p)
		}
	}
	_ = runtime.GOOS
}
