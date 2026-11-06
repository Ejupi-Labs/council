//go:build darwin || linux

package process

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const guardianArg = "__council_guardian_v1"

type wireSpec struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	Cwd        string   `json:"cwd"`
	Env        []string `json:"env"`
}
type controlRecord struct {
	Kind     string    `json:"kind"`
	Token    string    `json:"token"`
	Spec     *wireSpec `json:"spec,omitempty"`
	Known    bool      `json:"known,omitempty"`
	ExitCode int       `json:"exitCode,omitempty"`
}

func writeRecord(f *os.File, r controlRecord) error {
	data, err := json.Marshal(r)
	if err != nil || len(data) > 1024*1024 {
		return errors.New("The guardian control record is invalid.")
	}
	_, err = f.Write(append(data, '\n'))
	return err
}
func readRecord(reader *bufio.Reader) (controlRecord, error) {
	var r controlRecord
	data, err := reader.ReadSlice('\n')
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r, json.RejectUnknownMembers(true)); err != nil {
		return r, errors.New("The guardian control record is invalid.")
	}
	return r, nil
}

type unixOwner struct {
	mu                sync.Mutex
	command           *exec.Cmd
	control, events   *os.File
	token             string
	pgid              int
	finalSent, reaped bool
	waitDone          chan struct{}
	closeOnce         sync.Once
	waitOnce          sync.Once
	signalGroup       func(int, unix.Signal) error
}

func prepareNative(ctx context.Context, s NativeSpec, p pipeSet, o *Owned) (nativeOwner, error) {
	controlR, controlW, err := os.Pipe()
	if err != nil {
		return nil, errors.New("Cannot prepare the guardian control pipe.")
	}
	eventR, eventW, err := os.Pipe()
	if err != nil {
		controlR.Close()
		controlW.Close()
		return nil, errors.New("Cannot prepare the guardian observation pipe.")
	}
	closeAll := func() { controlR.Close(); controlW.Close(); eventR.Close(); eventW.Close() }
	self, err := os.Executable()
	if err != nil {
		closeAll()
		return nil, errors.New("Cannot identify the exact guardian executable.")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		closeAll()
		return nil, errors.New("Cannot establish guardian identity.")
	}
	token := hex.EncodeToString(nonce[:])
	cmd := exec.Command(self, guardianArg)
	cmd.Dir = s.Cwd
	cmd.Stdin = p.stdinR
	cmd.Stdout = p.stdoutW
	cmd.Stderr = p.stderrW
	cmd.ExtraFiles = []*os.File{controlR, eventW}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	if err := cmd.Start(); err != nil {
		closeAll()
		return nil, errors.New("Cannot start the owned process-group guardian.")
	}
	controlR.Close()
	eventW.Close()
	n := &unixOwner{command: cmd, control: controlW, events: eventR, token: token, pgid: cmd.Process.Pid, waitDone: make(chan struct{})}
	abort := func() error {
		n.mu.Lock()
		_ = n.signalOwned(unix.SIGKILL)
		n.finalSent = true
		n.mu.Unlock()
		n.beginWait()
		timer := time.NewTimer(CleanupLimit)
		defer timer.Stop()
		select {
		case <-n.waitDone:
			probe := n.signalOwned(0)
			n.close()
			// Delivery can fail for a zombie-only group on Darwin. Verified
			// absence after final-attempt-before-reap is the cleanup proof.
			if errors.Is(probe, unix.ESRCH) {
				return nil
			}
		case <-timer.C:
			n.close()
		}
		return ErrOwnershipUncertain
	}
	prepared, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	written := make(chan error, 1)
	go func() {
		written <- writeRecord(controlW, controlRecord{Kind: "prepare", Token: token, Spec: &wireSpec{Executable: s.Executable, Args: s.Args, Cwd: s.Cwd, Env: s.Env}})
	}()
	select {
	case err := <-written:
		if err != nil {
			if cleanupErr := abort(); cleanupErr != nil {
				return nil, cleanupErr
			}
			return nil, errors.New("Cannot initialize the owned guardian.")
		}
	case <-prepared.Done():
		if cleanupErr := abort(); cleanupErr != nil {
			return nil, cleanupErr
		}
		return nil, errors.New("Guardian preparation was interrupted before its launch gate.")
	}
	reader := bufio.NewReaderSize(eventR, 1024*1024+1)
	type readiness struct {
		record controlRecord
		err    error
	}
	ready := make(chan readiness, 1)
	go func() { r, e := readRecord(reader); ready <- readiness{r, e} }()
	select {
	case <-prepared.Done():
		if cleanupErr := abort(); cleanupErr != nil {
			return nil, cleanupErr
		}
		return nil, errors.New("Guardian preparation was interrupted before its launch gate.")
	case result := <-ready:
		if result.err != nil || result.record.Kind != "ready" || result.record.Token != token {
			if cleanupErr := abort(); cleanupErr != nil {
				return nil, cleanupErr
			}
			return nil, errors.New("The owned guardian did not establish its launch gate.")
		}
	}
	go func() {
		for {
			r, err := readRecord(reader)
			if err != nil {
				o.setRoot(rootResult(-1, false, errors.New("The guardian lost the owned root observation.")))
				return
			}
			if r.Token != token {
				o.setRoot(rootResult(-1, false, errors.New("The guardian identity could not be verified.")))
				return
			}
			if r.Kind != "root" {
				o.setRoot(rootResult(-1, false, errors.New("The guardian returned an unsupported observation.")))
				return
			}
			var rootErr error
			if !r.Known || r.ExitCode != 0 {
				rootErr = errors.New("The owned root did not report a successful exit.")
			}
			o.setRoot(rootResult(r.ExitCode, r.Known, rootErr))
		}
	}()
	return n, nil
}
func (n *unixOwner) start() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.reaped || n.finalSent {
		return errors.New("The guardian launch gate is already closing.")
	}
	if err := writeRecord(n.control, controlRecord{Kind: "go", Token: n.token}); err != nil {
		return errors.New("The guardian launch gate could not be released.")
	}
	return nil
}
func (n *unixOwner) cleanup(ctx context.Context) (bool, string, string) {
	const scope = "managed_process_group_empty"
	n.mu.Lock()
	if n.reaped || n.finalSent {
		n.mu.Unlock()
		return false, scope, "The process-group anchor was already reaped; no further signals were sent."
	}
	termErr := n.signalOwned(unix.SIGTERM)
	n.mu.Unlock()
	// Darwin's killpg1 excludes zombies and can return EPERM for an existing
	// zombie-only group. A delivery error proves neither liveness nor absence.
	// Always attempt the final signal while the guardian remains unreaped, then
	// require guardian reap and an ESRCH group probe before verifying cleanup.
	timer := time.NewTimer(GracePeriod)
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	timer.Stop()
	// No Wait can run until the final signal has been issued to this unreaped anchor.
	n.mu.Lock()
	if n.reaped || n.finalSent {
		n.mu.Unlock()
		return false, scope, "The process-group anchor was lost before final signalling."
	}
	killErr := n.signalOwned(unix.SIGKILL)
	n.finalSent = true
	n.mu.Unlock()
	delivery := " Signal attempts: TERM=" + signalStatus(termErr) + ", KILL=" + signalStatus(killErr) + "."
	n.beginWait()
	select {
	case <-n.waitDone:
	case <-ctx.Done():
		return false, scope, "The guardian exit could not be observed before the cleanup deadline." + delivery
	}
	// A cached PGID is only probed after reaping. Reuse can conservatively block, never kill.
	// Darwin may still report EPERM for orphan zombies awaiting their parent's
	// reap. Retry permission-denied observations under this original deadline;
	// neither a permission error nor an accepted zero signal proves absence.
	lastProbe := "not_observed"
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return false, scope, "Managed group absence was not proved before the cleanup deadline. Last group probe: " + lastProbe + "." + delivery
		}
		err := n.signalOwned(0)
		if errors.Is(err, unix.ESRCH) {
			return true, scope, "The managed process group was observed empty after the final signal attempt and guardian reap." + delivery
		}
		if err != nil && !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.EACCES) {
			return false, scope, "The managed process group could not be probed (" + signalStatus(err) + ")." + delivery
		}
		lastProbe = signalStatus(err)
		if err == nil {
			lastProbe = "present"
		}
		select {
		case <-ctx.Done():
			return false, scope, "Managed group absence was not proved before the cleanup deadline. Last group probe: " + lastProbe + "." + delivery
		case <-ticker.C:
		}
	}
}

// signalStatus retains bounded native error categories without exposing paths or
// arbitrary error text in persisted cleanup observations.
func signalStatus(err error) string {
	switch {
	case err == nil:
		return "accepted"
	case errors.Is(err, unix.ESRCH):
		return "ESRCH"
	case errors.Is(err, unix.EPERM):
		return "EPERM"
	case errors.Is(err, unix.EACCES):
		return "EACCES"
	case errors.Is(err, unix.EINVAL):
		return "EINVAL"
	case errors.Is(err, unix.EINTR):
		return "EINTR"
	default:
		return "other_error"
	}
}

func (n *unixOwner) close() error {
	n.closeOnce.Do(func() {
		n.mu.Lock()
		n.finalSent = true // Closing control forbids any subsequent parent PGID signal.
		n.control.Close()
		n.mu.Unlock()
		n.beginWait()
		n.events.Close()
	})
	return nil
}
func (n *unixOwner) signalOwned(signal unix.Signal) error {
	if n.signalGroup != nil {
		return n.signalGroup(-n.pgid, signal)
	}
	return unix.Kill(-n.pgid, signal)
}
func (n *unixOwner) beginWait() {
	n.waitOnce.Do(func() {
		go func() {
			_ = n.command.Wait()
			n.mu.Lock()
			n.reaped = true
			n.mu.Unlock()
			close(n.waitDone)
		}()
	})
}
