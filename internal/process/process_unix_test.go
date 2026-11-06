//go:build darwin || linux

package process

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func platformTestChild(args []string) int {
	if len(args) == 2 && args[0] == "ignore_term" {
		signal.Ignore(syscall.SIGTERM)
		if os.WriteFile(args[1], []byte("ready"), 0600) != nil {
			return 3
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if len(args) == 1 && args[0] == "control_probe" {
		for _, fd := range []int{3, 4} {
			var st unix.Stat_t
			if unix.Fstat(fd, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFIFO {
				fmt.Fprint(os.Stdout, "true")
				return 0
			}
		}
		fmt.Fprint(os.Stdout, "false")
		return 0
	}
	if len(args) == 1 && args[0] == "crash_guardian" {
		if unix.Kill(os.Getppid(), unix.SIGKILL) != nil {
			return 3
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if len(args) == 2 && args[0] == "escape_child" {
		deadline := time.Now().Add(4 * time.Second)
		for n := 0; time.Now().Before(deadline); n++ {
			if os.WriteFile(args[1], []byte(strconv.Itoa(n)), 0600) != nil {
				return 3
			}
			time.Sleep(20 * time.Millisecond)
		}
		return 0
	}
	if len(args) == 2 && args[0] == "escape_parent" {
		self, _ := os.Executable()
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return 3
		}
		defer null.Close()
		child := exec.Command(self, testChildArg, "platform", "escape_child", args[1])
		child.Stdin = null
		child.Stdout = null
		child.Stderr = null
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if child.Start() != nil {
			return 4
		}
		go child.Wait()
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, err := os.Stat(args[1]); err == nil {
				break
			}
			if time.Now().After(deadline) {
				return 5
			}
			time.Sleep(5 * time.Millisecond)
		}
		return 0
	}
	return 2
}

func TestUnixPrivateControlDoesNotReachWorker(t *testing.T) {
	var out bytes.Buffer
	s := selfSpec(t, "platform", "control_probe")
	s.Stdout = &out
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	waitRoot(t, o)
	cleanupOwned(t, o)
	if out.String() != "false" {
		t.Fatal("guardian private fd3/fd4 pipe endpoints reached the worker")
	}
}

func TestUnixSignalsPrecedeReapAndReuseOnlyBlocks(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	waitRoot(t, o)
	n := o.native.(*unixOwner)
	var signalMu sync.Mutex
	var signals []unix.Signal
	postReapSignal := false
	n.signalGroup = func(pgid int, sig unix.Signal) error {
		signalMu.Lock()
		defer signalMu.Unlock()
		signals = append(signals, sig)
		if sig != 0 && n.reaped {
			postReapSignal = true
		}
		if sig == 0 {
			return nil
		}
		return unix.Kill(pgid, sig)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	r := o.Cleanup(ctx)
	if r.Verified {
		t.Fatal("synthetic post-reap group reuse became verified cleanup")
	}
	signalMu.Lock()
	defer signalMu.Unlock()
	if postReapSignal {
		t.Fatal("a cached group identity was signalled after guardian reap")
	}
	if len(signals) < 3 || signals[0] != unix.SIGTERM || signals[1] != unix.SIGKILL {
		t.Fatalf("invalid final signal order: %v", signals)
	}
}

func TestUnixGuardianCrashDoesNotInventRootSuccess(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "platform", "crash_guardian"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	root := waitRoot(t, o)
	if root.Known || root.ExitCode == 0 {
		t.Fatalf("guardian exit became vendor success: %+v", root)
	}
	cleanupOwned(t, o)
}

func TestUnixGuardianCrashRetainsAlreadyObservedRoot(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	before := waitRoot(t, o)
	if !before.Known || before.ExitCode != 0 {
		t.Fatal(before)
	}
	n := o.native.(*unixOwner)
	if err := n.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cleanupOwned(t, o)
	after, _ := o.RootOutcome()
	if !after.Known || after.ExitCode != before.ExitCode {
		t.Fatal("guardian crash erased an observed root result")
	}
}

func TestUnixScopedReceiptDoesNotClaimDetachedCoverage(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "detached.marker")
	s := selfSpec(t, "platform", "escape_parent", marker)
	s.Stdout = io.Discard
	s.Stderr = io.Discard
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	waitRoot(t, o)
	r := cleanupOwned(t, o)
	if r.Scope != "managed_process_group_empty" {
		t.Fatal("receipt made an unsupported descendant claim")
	}
	first, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	second, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("detached fixture did not demonstrate the declared process-group limitation")
	}
	// This deliberately detached fixture expires itself; no cached PID is killed.
	time.Sleep(2 * time.Second)
}

func TestUnixIgnoredTermRequiresFinalKill(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ready.marker")
	o, err := Prepare(context.Background(), selfSpec(t, "platform", "ignore_term", marker))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("TERM-ignore fixture was not ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
	started := time.Now()
	cleanupOwned(t, o)
	if time.Since(started) < GracePeriod {
		t.Fatal("TERM-ignore fixture skipped the fixed graceful interval")
	}
}
