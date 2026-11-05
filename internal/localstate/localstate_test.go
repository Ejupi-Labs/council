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
