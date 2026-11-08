//go:build linux

package historyout

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// A bind mount is introduced only inside a disposable user/mount namespace.
// Hosts that prohibit unprivileged namespaces report an explicit skip.
func TestHistoryPublisherRejectsMountReplacement(t *testing.T) {
	if os.Getenv("COUNCIL_HISTORY_MOUNT_FIXTURE") == "1" {
		root, path := fixtureRoot(t)
		preview, err := root.Preview("history-mount-test.json")
		if err != nil {
			t.Fatal(err)
		}
		other := t.TempDir()
		if err = os.WriteFile(filepath.Join(other, "sentinel"), []byte("untouched"), 0600); err != nil {
			t.Fatal(err)
		}
		if err = unix.Mount(other, path, "", unix.MS_BIND, ""); err != nil {
			if errors.Is(err, unix.EPERM) || errors.Is(err, unix.ENOSYS) {
				t.Skipf("bind mounts unavailable in isolated namespace: %v", err)
			}
			t.Fatal(err)
		}
		defer unix.Unmount(path, unix.MNT_DETACH)
		permits := 0
		result, err := root.Publish(context.Background(), preview, []byte("must not be written"), func(context.Context) error { permits++; return nil })
		if err == nil || permits != 0 || result.CreateAttempted {
			t.Fatalf("mount replacement reached permit: %+v %d %v", result, permits, err)
		}
		if body, err := os.ReadFile(filepath.Join(other, "sentinel")); err != nil || string(body) != "untouched" {
			t.Fatal("mounted fixture modified", err)
		}
		if _, err := os.Lstat(filepath.Join(other, preview.Name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("output created under replacement mount", err)
		}
		return
	}
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare is not available")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("unshare", "--user", "--map-root-user", "--mount", "--", exe, "-test.run=^TestHistoryPublisherRejectsMountReplacement$", "-test.v")
	cmd.Env = append(os.Environ(), "COUNCIL_HISTORY_MOUNT_FIXTURE=1")
	output, err := cmd.CombinedOutput()
	if err != nil && (strings.Contains(string(output), "Operation not permitted") || strings.Contains(string(output), "unshare failed: Function not implemented")) {
		t.Skipf("isolated mount namespace unavailable: %s", strings.TrimSpace(string(output)))
	}
	if err != nil {
		t.Fatalf("isolated mount fixture failed: %s: %v", output, err)
	}
	if strings.Contains(string(output), "--- SKIP:") {
		t.Skipf("%s", strings.TrimSpace(string(output)))
	}
}
