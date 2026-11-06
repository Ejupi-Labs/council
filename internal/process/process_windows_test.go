//go:build windows

package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func platformTestChild(args []string) int {
	if len(args) == 2 && args[0] == "event" {
		n, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil {
			return 2
		}
		err = windows.SetEvent(windows.Handle(n))
		fmt.Fprint(os.Stdout, err == nil)
		return 0
	}
	if len(args) == 2 && args[0] == "restricted_outer" {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return 3
		}
		defer windows.CloseHandle(job)
		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		limits.BasicLimitInformation.ActiveProcessLimit = 1
		if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
			return 4
		}
		if windows.AssignProcessToJobObject(job, windows.CurrentProcess()) != nil {
			return 5
		}
		self, _ := os.Executable()
		o, err := Prepare(context.Background(), NativeSpec{Executable: self, Args: []string{testChildArg, "marker", filepath.Join(args[1], "blocked.marker")}, Cwd: args[1]})
		if err == nil {
			o.Close()
			return 6
		}
		return 0
	}
	return 2
}

func TestWindowsCreationTimeJobAndRestrictedInheritance(t *testing.T) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	var inheritable windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), event, windows.CurrentProcess(), &inheritable, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(inheritable)
	var out bytes.Buffer
	s := selfSpec(t, "platform", "event", strconv.FormatUint(uint64(inheritable), 10))
	s.Stdout = &out
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	n := o.native.(*windowsOwner)
	active, err := n.active()
	if err != nil || active != 1 {
		t.Fatalf("root was not in its owned job before first instruction: %d %v", active, err)
	}
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	root := waitRoot(t, o)
	if !root.Known || root.ExitCode != 0 {
		t.Fatal(root)
	}
	cleanupOwned(t, o)
	if out.String() != "false" {
		t.Fatal("unrelated inheritable owner handle reached the worker")
	}
	status, err := windows.WaitForSingleObject(event, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("worker signalled an owner-only handle")
	}
}

func TestWindowsIncompatibleOuterJobFailsWithoutWorker(t *testing.T) {
	dir := t.TempDir()
	self, _ := os.Executable()
	cmd := exec.Command(self, testChildArg, "platform", "restricted_outer", dir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatalf("restricted outer job fixture: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blocked.marker")); !os.IsNotExist(err) {
		t.Fatal("worker executed despite incompatible outer job")
	}
}

func TestWindowsAccountingFailureRemainsUnresolved(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "linger"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	n := o.native.(*windowsOwner)
	n.queryJob = func(windows.Handle) (uint32, error) { return 0, errors.New("injected accounting failure") }
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if result := o.Cleanup(ctx); result.Verified {
		t.Fatal("failed accounting became an empty-job receipt")
	}
}
