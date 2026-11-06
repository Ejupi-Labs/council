//go:build windows

package process

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const procThreadAttributeJobList = uintptr(0x0002000d)

type jobAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}
type windowsOwner struct {
	mu          sync.Mutex
	job, thread windows.Handle
	owner       *Owned
	closed      bool
	queryJob    func(windows.Handle) (uint32, error)
}

func duplicateChildHandle(f *os.File) (windows.Handle, error) {
	var result windows.Handle
	err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(f.Fd()), windows.CurrentProcess(), &result, 0, true, windows.DUPLICATE_SAME_ACCESS)
	return result, err
}
func environmentBlock(env []string) []uint16 {
	copy := append([]string{}, env...)
	sort.SliceStable(copy, func(i, j int) bool { return strings.ToUpper(copy[i]) < strings.ToUpper(copy[j]) })
	var block []uint16
	for _, entry := range copy {
		block = append(block, utf16.Encode([]rune(entry))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	if len(copy) == 0 {
		block = append(block, 0)
	}
	return block
}

func prepareNative(ctx context.Context, s NativeSpec, p pipeSet, o *Owned) (nativeOwner, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.New("Native preparation was interrupted.")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.New("Cannot create the owned Windows job.")
	}
	keepJob := false
	defer func() {
		if !keepJob {
			windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, errors.New("Cannot configure the no-breakaway Windows job.")
	}
	var handles []windows.Handle
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	for _, f := range []*os.File{p.stdinR, p.stdoutW, p.stderrW} {
		h, err := duplicateChildHandle(f)
		if err != nil {
			return nil, errors.New("Cannot prepare restricted child standard handles.")
		}
		handles = append(handles, h)
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, errors.New("The Windows process attribute API is unavailable.")
	}
	defer attributes.Delete()
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, errors.New("Cannot restrict child handle inheritance.")
	}
	jobs := []windows.Handle{job}
	if err := attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])); err != nil {
		return nil, errors.New("Creation-time Windows job assignment is unsupported.")
	}
	app, err := windows.UTF16PtrFromString(s.Executable)
	if err != nil {
		return nil, errors.New("The native executable path is invalid.")
	}
	command := windows.ComposeCommandLine(append([]string{s.Executable}, s.Args...))
	commandUTF16, err := windows.UTF16FromString(command)
	if err != nil || len(commandUTF16) > 32767 {
		return nil, errors.New("The native argument vector exceeds Windows limits.")
	}
	cwd, err := windows.UTF16PtrFromString(s.Cwd)
	if err != nil {
		return nil, errors.New("The native working directory is invalid.")
	}
	var env []uint16
	var envPtr *uint16
	if s.Env != nil {
		env = environmentBlock(s.Env)
		if len(env) > 1024*1024 {
			return nil, errors.New("The native environment exceeds its launch bound.")
		}
		envPtr = &env[0]
	}
	si := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES, StdInput: handles[0], StdOutput: handles[1], StdErr: handles[2]}, ProcThreadAttributeList: attributes.List()}
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	err = windows.CreateProcess(app, &commandUTF16[0], nil, nil, true, flags, envPtr, cwd, &si.StartupInfo, &pi)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(env)
	runtime.KeepAlive(commandUTF16)
	if err != nil {
		return nil, errors.New("The native process could not be created in its owned Windows job.")
	}
	n := &windowsOwner{job: job, thread: pi.Thread, owner: o}
	keepJob = true
	go func() {
		event, err := windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		result := rootResult(-1, false, errors.New("The owned root exit could not be observed."))
		if err == nil && event == windows.WAIT_OBJECT_0 {
			var code uint32
			if windows.GetExitCodeProcess(pi.Process, &code) == nil {
				var outcomeErr error
				if code != 0 {
					outcomeErr = errors.New("The owned root exited unsuccessfully.")
				}
				result = rootResult(int(code), true, outcomeErr)
			}
		}
		windows.CloseHandle(pi.Process)
		o.setRoot(result)
	}()
	return n, nil
}

func (n *windowsOwner) start() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.thread == 0 {
		return errors.New("The Windows launch gate is unavailable.")
	}
	previous, err := windows.ResumeThread(n.thread)
	if err != nil || previous != 1 {
		return errors.New("The Windows launch gate could not be released safely.")
	}
	windows.CloseHandle(n.thread)
	n.thread = 0
	return nil
}
func (n *windowsOwner) active() (uint32, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.job == 0 {
		return 0, errors.New("The owned job handle is unavailable.")
	}
	if n.queryJob != nil {
		return n.queryJob(n.job)
	}
	var accounting jobAccounting
	err := windows.QueryInformationJobObject(n.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
	return accounting.ActiveProcesses, err
}
func (n *windowsOwner) terminate() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.job == 0 {
		return errors.New("The owned job handle is unavailable.")
	}
	return windows.TerminateJobObject(n.job, 1)
}
func (n *windowsOwner) cleanup(ctx context.Context) (bool, string, string) {
	const scope = "managed_windows_job_empty"
	if active, err := n.active(); err == nil && active == 0 {
		return true, scope, "The owned job reports zero active processes."
	}
	grace := time.NewTimer(GracePeriod)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer grace.Stop()
	defer ticker.Stop()
	waitGrace := true
	for waitGrace {
		select {
		case <-ctx.Done():
			waitGrace = false
		case <-grace.C:
			waitGrace = false
		case <-ticker.C:
			if active, err := n.active(); err == nil && active == 0 {
				return true, scope, "The owned job reports zero active processes."
			}
		}
	}
	if err := n.terminate(); err != nil {
		return false, scope, "Owned job termination could not be requested."
	}
	for {
		active, err := n.active()
		if err != nil {
			return false, scope, "Owned job accounting could not be verified."
		}
		if active == 0 {
			return true, scope, "The owned job reports zero active processes."
		}
		select {
		case <-ctx.Done():
			return false, scope, "Owned job processes remain or their exit could not be confirmed before the cleanup deadline."
		case <-ticker.C:
		}
	}
}
func (n *windowsOwner) close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	if n.thread != 0 {
		windows.CloseHandle(n.thread)
		n.thread = 0
	}
	if n.job != 0 {
		err := windows.CloseHandle(n.job)
		n.job = 0
		return err
	}
	return nil
}
func HandleInternal([]string) (bool, int) { return false, 0 }
