//go:build darwin || linux

package process

import (
	"bufio"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// HandleInternal must run before public CLI parsing in this same executable.
func HandleInternal(args []string) (bool, int) {
	if len(args) == 0 || args[0] != guardianArg {
		return false, 0
	}
	if len(args) != 1 {
		return true, 2
	}
	return true, runGuardian()
}
func runGuardian() int {
	control := os.NewFile(3, "guardian-control")
	events := os.NewFile(4, "guardian-events")
	if control == nil || events == nil {
		return 2
	}
	defer control.Close()
	defer events.Close()
	unix.CloseOnExec(3)
	unix.CloseOnExec(4)
	reader := bufio.NewReaderSize(control, 1024*1024+1)
	init, err := readRecord(reader)
	if err != nil || init.Kind != "prepare" || len(init.Token) != 32 || init.Spec == nil {
		return 2
	}
	wire := init.Spec
	if validateSpec(NativeSpec{Executable: wire.Executable, Args: wire.Args, Cwd: wire.Cwd, Env: wire.Env}) != nil {
		return 2
	}
	// A caught TERM keeps the guardian alive as the group identity anchor. exec resets
	// this handler for the worker; the worker does not inherit a permanent TERM ignore.
	terms := make(chan os.Signal, 1)
	signal.Notify(terms, syscall.SIGTERM)
	defer signal.Stop(terms)
	go func() {
		for range terms {
		}
	}()
	if writeRecord(events, controlRecord{Kind: "ready", Token: init.Token}) != nil {
		return 2
	}
	gate, err := readRecord(reader)
	if err != nil {
		return 0
	}
	if gate.Kind != "go" || gate.Token != init.Token {
		return 2
	}
	// The owner keepalive never enters the provider. EOF always uses this living
	// guardian's own current group, never a recovered PID/PGID from disk.
	go func() {
		for {
			_, err := readRecord(reader)
			if err != nil {
				_ = unix.Kill(-unix.Getpgrp(), unix.SIGKILL)
				return
			}
			_ = unix.Kill(-unix.Getpgrp(), unix.SIGKILL)
			return
		}
	}()
	cmd := exec.Command(wire.Executable, wire.Args...)
	cmd.Dir = wire.Cwd
	cmd.Env = wire.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = writeRecord(events, controlRecord{Kind: "root", Token: init.Token, Known: false, ExitCode: -1})
	} else {
		go func() {
			err := cmd.Wait()
			code := cmd.ProcessState.ExitCode()
			if err == nil {
				code = 0
			}
			_ = writeRecord(events, controlRecord{Kind: "root", Token: init.Token, Known: true, ExitCode: code})
		}()
	}
	// Only the worker and its descendants retain these stream endpoints. Keeping
	// guardian stdout/stderr open would hide a worker that closes streams but lives.
	os.Stdin.Close()
	os.Stdout.Close()
	os.Stderr.Close()
	for {
		time.Sleep(time.Hour)
	}
}
