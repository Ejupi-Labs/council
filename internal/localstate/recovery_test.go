package localstate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedSQLiteCompanionsPreservePhysicalFilesAndBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	root = lock.Root()
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	contents := []byte{0, 255, 83, 81, 76, 105, 116, 101, 0, 13, 10, 128}
	originals := map[string]os.FileInfo{}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		path := filepath.Join(root, "workspace.sqlite3"+suffix)
		// These model SQLite-created crash companions: native default owner and
		// inherited ACL on Windows, private mode on Unix. They are not pre-protected.
		if err := os.WriteFile(path, contents, 0600); err != nil {
			t.Fatal(err)
		}
		originals[path], err = os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	lock, err = Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for path, before := range originals {
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(before, after) {
			t.Fatalf("recovery file replaced: %s: %v", path, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, contents) {
			t.Fatalf("recovery bytes changed: %s: %v", path, err)
		}
		if err := verifyPrivate(path, false); err != nil {
			t.Fatalf("recovery metadata not normalized: %s: %v", path, err)
		}
	}
}

func TestAcquireDoesNotFabricateRecoveryCompanions(t *testing.T) {
	lock, err := Acquire(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if _, err := os.Lstat(lock.DatabasePath() + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing companion was created: %s: %v", suffix, err)
		}
	}
}

func TestAcquireRejectsRecoveryDirectories(t *testing.T) {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		t.Run(suffix, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "private")
			path := filepath.Join(root, "workspace.sqlite3"+suffix)
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			if lock, err := Acquire(root); err == nil {
				lock.Close()
				t.Fatal("accepted a recovery companion directory")
			}
		})
	}
}

func TestAcquireRejectsHardLinksBeforeChangingOutsideMetadata(t *testing.T) {
	for _, name := range []string{"owner.lock", "workspace.sqlite3", "workspace.sqlite3-journal", "workspace.sqlite3-wal", "workspace.sqlite3-shm"} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "private")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(base, "outside-sentinel")
			contents := []byte("disposable outside hard-link sentinel\x00\xff")
			if err := os.WriteFile(outside, contents, 0640); err != nil {
				t.Fatal(err)
			}
			before := protectionFingerprint(t, outside)
			if err := os.Link(outside, filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			if lock, err := Acquire(root); err == nil {
				lock.Close()
				t.Fatal("accepted a multiply linked private file")
			}
			if got := protectionFingerprint(t, outside); got != before {
				t.Fatalf("outside target metadata changed before rejection:\nbefore %s\nafter  %s", before, got)
			}
			got, err := os.ReadFile(outside)
			if err != nil || !bytes.Equal(got, contents) {
				t.Fatalf("outside target bytes changed: %v", err)
			}
		})
	}
}

func TestProtectPrivateFileRejectsHardLinksBeforeChangingMetadata(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside-sentinel")
	if err := os.WriteFile(outside, []byte("unchanged"), 0640); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "linked")
	if err := os.Link(outside, linked); err != nil {
		t.Fatal(err)
	}
	before := protectionFingerprint(t, outside)
	f, err := os.OpenFile(linked, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := ProtectPrivateFile(f, false); err == nil {
		t.Fatal("protected a multiply linked private file")
	}
	if got := protectionFingerprint(t, outside); got != before {
		t.Fatal("protection changed the outside hard-link target")
	}
}

func TestRecoveryPreflightRunsOnlyAfterNativeOwnership(t *testing.T) {
	lock, err := Acquire(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	path := lock.DatabasePath() + "-wal"
	if err := os.WriteFile(path, []byte("retained crash state"), 0640); err != nil {
		t.Fatal(err)
	}
	before := protectionFingerprint(t, path)
	if other, err := Acquire(lock.Root()); !errors.Is(err, ErrLocked) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("contender bypassed native owner: %v", err)
	}
	if got := protectionFingerprint(t, path); got != before {
		t.Fatal("lock contender changed retained recovery metadata")
	}
}
