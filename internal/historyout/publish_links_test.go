package historyout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A target introduced after preview must never make the publisher follow an
// outside symlink/reparse point or request publication authority.
func TestHistoryPublisherRejectsTargetSymlinkAfterPreview(t *testing.T) {
	root, path := fixtureRoot(t)
	preview, err := root.Preview("history-link-test.json")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	const original = "unrelated fixture content\n"
	if err = os.WriteFile(outside, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(path, preview.Name)
	if err = os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows fixture cannot create a symlink: %v", err)
		}
		t.Fatal(err)
	}
	permits := 0
	result, err := root.Publish(context.Background(), preview, []byte("must not be written"), func(context.Context) error {
		permits++
		return nil
	})
	if err == nil || permits != 0 || result.CreateAttempted {
		t.Fatalf("symlink reached publication authority: %+v, permits=%d, err=%v", result, permits, err)
	}
	if _, err = root.Preview(preview.Name); err == nil {
		t.Fatal("preview accepted the existing symlink")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != original {
		t.Fatalf("outside fixture changed: %q, %v", data, err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was modified or removed: %v", err)
	}
}

func TestHistoryPublisherUnsupportedSyncNeverClaimsDurability(t *testing.T) {
	root, path := fixtureRoot(t)
	preview, err := root.Preview("history-sync-test.json")
	if err != nil {
		t.Fatal(err)
	}
	root.testHook = func(stage string) error {
		if stage == "before_directory_sync" {
			return ErrUnsupported
		}
		return nil
	}
	result, err := root.Publish(context.Background(), preview, []byte("captured"), func(context.Context) error { return nil })
	if !errors.Is(err, ErrUnsupported) || !result.PermitAccepted || !result.Created || !result.FileFlushed || !result.NameVerified || result.DirectorySync != "unknown" {
		t.Fatalf("unsupported sync was reported as complete: %+v %v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(path, preview.Name)); err != nil || string(got) != "captured" {
		t.Fatalf("uncertain file not retained: %q %v", got, err)
	}
}
