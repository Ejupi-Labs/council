package localstate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExclusiveLockSubprocess(t *testing.T) {
	if root := os.Getenv("COUNCIL_LOCK_TEST_ROOT"); root != "" {
		lock, err := Acquire(root)
		if !errors.Is(err, ErrLocked) {
			if lock != nil {
				lock.Close()
			}
			os.Exit(18)
		}
		os.Exit(0)
	}
	root := filepath.Join(t.TempDir(), "private")
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	before, err := os.Stat(filepath.Join(root, "owner.lock"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestExclusiveLockSubprocess$")
	cmd.Env = append(os.Environ(), "COUNCIL_LOCK_TEST_ROOT="+root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("other process acquired owner lock: %v %s", err, out)
	}
	after, err := os.Stat(filepath.Join(root, "owner.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("lock file replaced")
	}
	if err := verifyPrivate(root, true); err != nil {
		t.Fatal(err)
	}
	if err := verifyPrivate(lock.DatabasePath(), false); err != nil {
		t.Fatal(err)
	}
}

func TestDescriptorAtomicOwnershipAndStaleRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	d := Descriptor{OwnerInstanceID: "11111111111111111111111111111111", PID: os.Getpid(), StartedAt: "2026-11-05T20:00:00.000Z", Origin: "http://127.0.0.1:34567", APIVersion: 1, Token: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err := lock.WriteDescriptor(d); err != nil {
		t.Fatal(err)
	}
	if err := verifyPrivate(filepath.Join(root, "owner.json"), false); err != nil {
		t.Fatal(err)
	}
	if err := lock.RemoveOwnDescriptor("22222222222222222222222222222222"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDescriptor(root)
	if err != nil || got != d {
		t.Fatalf("descriptor removed by other instance: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lock.WriteDescriptor(d); err == nil {
		t.Fatal("closed owner wrote descriptor")
	}
	lock, err = Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	d.OwnerInstanceID = "33333333333333333333333333333333"
	if err := lock.WriteDescriptor(d); err != nil {
		t.Fatal(err)
	}
	got, err = ReadDescriptor(root)
	if err != nil || got != d {
		t.Fatal("new owner could not replace stale descriptor")
	}
	if err := lock.RemoveOwnDescriptor(d.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDescriptor(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descriptor still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "owner.lock")); err != nil {
		t.Fatal("lock file must never be unlinked")
	}
}

func TestRejectRedirectedRoot(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink unavailable")
	}
	if lock, err := Acquire(link); err == nil {
		lock.Close()
		t.Fatal("accepted redirected root")
	}
}

func TestAncestorAliasResolvesToPrivatePhysicalRoot(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "physical-parent")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "ancestor-alias")
	if err := createAncestorAlias(parent, alias); err != nil {
		t.Fatal(err)
	}
	selectedRoot := filepath.Join(alias, "private")
	lock, err := Acquire(selectedRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	canonicalRoot, err := filepath.EvalSymlinks(filepath.Join(parent, "private"))
	if err != nil {
		t.Fatal(err)
	}
	if lock.Root() != canonicalRoot || lock.DatabasePath() != filepath.Join(canonicalRoot, "workspace.sqlite3") {
		t.Fatalf("owner paths are not canonical: root=%q database=%q", lock.Root(), lock.DatabasePath())
	}
	if err := verifyPrivate(canonicalRoot, true); err != nil {
		t.Fatal(err)
	}
	if contender, err := Acquire(canonicalRoot); !errors.Is(err, ErrLocked) {
		if contender != nil {
			contender.Close()
		}
		t.Fatalf("ancestor alias bypassed physical owner lock: %v", err)
	}
	d := Descriptor{OwnerInstanceID: "11111111111111111111111111111111", PID: os.Getpid(), StartedAt: "2026-11-05T20:00:00.000Z", Origin: "http://127.0.0.1:34567", APIVersion: 1, Token: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err := lock.WriteDescriptor(d); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{selectedRoot, canonicalRoot} {
		got, err := ReadDescriptor(root)
		if err != nil || got != d {
			t.Fatalf("descriptor does not identify the same owner through %q: %v", root, err)
		}
	}
	if err := lock.RemoveOwnDescriptor(d.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(selectedRoot, "owner.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical owner cleanup did not remove alias descriptor: %v", err)
	}
}

func TestRejectRedirectedPrivateFileTargets(t *testing.T) {
	base := t.TempDir()
	// This is only a disposable sentinel; no actual credential location is inspected.
	sentinel := filepath.Join(base, "unrelated-private-file")
	original := []byte("private sentinel must stay untouched")
	if err := os.WriteFile(sentinel, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"owner.lock", "workspace.sqlite3"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(sentinel, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			if lock, err := Acquire(root); err == nil {
				lock.Close()
				t.Fatal("accepted redirected private state file")
			}
		})
	}
	if lock, err := Acquire(sentinel); err == nil {
		lock.Close()
		t.Fatal("accepted a regular file as the private directory")
	}
	after, err := os.ReadFile(sentinel)
	if err != nil || string(after) != string(original) {
		t.Fatalf("private sentinel changed: %v", err)
	}
}
