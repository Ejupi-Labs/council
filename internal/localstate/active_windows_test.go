//go:build windows

package localstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/process"
	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

func TestPrivateStateOwnedJobHelper(t *testing.T) {
	if os.Getenv("COUNCIL_PRIVATE_ACTIVE_JOB") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "PRIVATE_STATE_JOB_READY")
	time.Sleep(30 * time.Second) // Self-expiring test worker; no provider is invoked.
	os.Exit(0)
}

type privateJobReadyWriter struct{ ready chan struct{} }

func (w privateJobReadyWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "PRIVATE_STATE_JOB_READY") {
		select {
		case w.ready <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func TestContenderWithLiveWALRootAndOwnedJob(t *testing.T) {
	lock, err := Acquire(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	db, err := sql.Open("sqlite", lock.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("live fixture could not establish WAL: %s %v", mode, err)
	}
	if _, err := db.Exec("PRAGMA synchronous=FULL; CREATE TABLE active_fixture(value TEXT); INSERT INTO active_fixture VALUES('retained owner')"); err != nil {
		t.Fatal(err)
	}
	// Evidence.New retains exactly this kind of root handle in a real execution
	// owner. Go's Windows root handle does not share DELETE access.
	root, err := os.OpenRoot(lock.Root())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 1)
	worker, err := process.Prepare(context.Background(), process.NativeSpec{
		Executable: executable, Args: []string{"-test.run=^TestPrivateStateOwnedJobHelper$"}, Cwd: lock.Root(),
		Env: append(os.Environ(), "COUNCIL_PRIVATE_ACTIVE_JOB=1"), Stdout: privateJobReadyWriter{ready}, Stderr: io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanup := worker.Cleanup(ctx); !cleanup.Verified {
			t.Errorf("test-owned native job cleanup was not verified: %+v", cleanup)
		}
		worker.Close()
	}()
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-worker.RootDone():
		t.Fatal("test-owned native child exited before the acquisition probe")
	case <-time.After(5 * time.Second):
		t.Fatal("test-owned native child did not reach its first-instruction marker")
	}
	if other, err := Acquire(lock.Root()); !errors.Is(err, ErrLocked) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("active execution-owner contender failed before native lock arbitration: %v", err)
	}
	var value string
	if err := db.QueryRow("SELECT value FROM active_fixture").Scan(&value); err != nil || value != "retained owner" {
		t.Fatalf("contender changed the active WAL database: %q %v", value, err)
	}
	select {
	case <-worker.RootDone():
		t.Fatal("contender interrupted the owned native worker")
	default:
	}
}

func TestPrivateDirectoryReuseRequiresEffectiveInheritance(t *testing.T) {
	for _, flags := range []string{"", "OI", "CI", "OICINP", "OICIIO"} {
		t.Run("flags="+flags, func(t *testing.T) {
			lock, err := Acquire(filepath.Join(t.TempDir(), "private"))
			if err != nil {
				t.Fatal(err)
			}
			path := lock.Root()
			lock.Close()
			name, err := windows.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			// Retain restoration rights without DELETE, so the fixture handle
			// itself does not conflict with the subsequent os.OpenRoot probe.
			handle, err := windows.CreateFile(name, windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER|windows.FILE_READ_ATTRIBUTES,
				windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
				windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
			if err != nil {
				t.Fatal(err)
			}
			securityFile := os.NewFile(uintptr(handle), path)
			defer func() {
				if err := protectSecurityHandle(securityFile, true); err != nil {
					t.Errorf("test directory ACL restoration failed: %v", err)
				}
				securityFile.Close()
			}()
			user, err := currentSID()
			if err != nil {
				t.Fatal(err)
			}
			// An allowed-principal protected DACL is insufficient when new SQLite
			// files would receive the token default DACL or lose nested inheritance.
			sd, err := windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;" + user.String() + ")(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)")
			if err != nil {
				t.Fatal(err)
			}
			acl, _, err := sd.DACL()
			if err != nil {
				t.Fatal(err)
			}
			if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
				t.Fatal(err)
			}
			if err := verifyPrivate(path, true); err == nil {
				t.Fatal("directory without effective OICI inheritance passed private verification")
			}
			root, err := os.OpenRoot(path)
			if err != nil {
				// Inherit-only grants intentionally deny effective directory reads.
				// Reuse must fail closed; the retained test handle restores its ACL.
				if err := securePrivate(path, true); err == nil {
					t.Fatal("inaccessible directory bypassed private verification")
				}
				return
			}
			if err := securePrivate(path, true); err == nil {
				root.Close()
				t.Fatal("busy directory needing ACL normalization bypassed the mutation guard")
			}
			root.Close()
			if err := securePrivate(path, true); err != nil {
				t.Fatalf("ordinary unbusy directory could not regain canonical inheritance: %v", err)
			}
			if err := verifyPrivate(path, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}
