package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var ErrOwnershipUncertain = errors.New("Native preparation cleanup could not be verified; preserve the execution fence.")

const GracePeriod = 2 * time.Second
const CleanupLimit = 15 * time.Second

type Owned struct {
	mu                     sync.Mutex
	native                 nativeOwner
	started                bool
	closing                bool
	root                   RootOutcome
	rootSet                bool
	rootDone               chan struct{}
	faults                 chan StreamFault
	pipes                  pipeSet
	input                  []byte
	stdinDone              chan struct{}
	stdinState             string
	stdinBytesWritten      int64
	stdinError             bool
	stdoutDone, stderrDone chan struct{}
	stdoutErr, stderrErr   bool
	cleanupOnce            sync.Once
	cleanupDone            chan struct{}
	cleanupResult          CleanupResult
}

type pipeSet struct{ stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW *os.File }

func (p pipeSet) closeChild()  { p.stdinR.Close(); p.stdoutW.Close(); p.stderrW.Close() }
func (p pipeSet) closeParent() { p.stdinW.Close(); p.stdoutR.Close(); p.stderrR.Close() }
func (p pipeSet) closeAll()    { p.closeChild(); p.closeParent() }
func makePipes() (p pipeSet, err error) {
	p.stdinR, p.stdinW, err = os.Pipe()
	if err != nil {
		return p, err
	}
	p.stdoutR, p.stdoutW, err = os.Pipe()
	if err != nil {
		p.stdinR.Close()
		p.stdinW.Close()
		return p, err
	}
	p.stderrR, p.stderrW, err = os.Pipe()
	if err != nil {
		p.stdinR.Close()
		p.stdinW.Close()
		p.stdoutR.Close()
		p.stdoutW.Close()
		return p, err
	}
	return p, nil
}

func validString(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func validateSpec(s NativeSpec) error {
	if !filepath.IsAbs(s.Executable) || !filepath.IsAbs(s.Cwd) || !validString(s.Executable) || !validString(s.Cwd) {
		return errors.New("The native executable and working directory must be absolute lossless paths.")
	}
	if len(s.Stdin) > 256*1024 {
		return errors.New("The native input exceeds the 256 KiB launch limit.")
	}
	if len(s.Args) > 4096 || len(s.Env) > 4096 {
		return errors.New("The native launch metadata exceeds its bound.")
	}
	totalBytes := len(s.Executable) + len(s.Cwd)
	for _, value := range append(append([]string{}, s.Args...), s.Env...) {
		if totalBytes > 1024*1024 || len(value) > 1024*1024-totalBytes {
			return errors.New("The native launch metadata exceeds its byte bound.")
		}
		totalBytes += len(value)
		if !validString(value) {
			return errors.New("The native argument or environment encoding is invalid.")
		}
	}
	i, err := os.Stat(s.Executable)
	if err != nil || !i.Mode().IsRegular() {
		return errors.New("The native executable is unavailable.")
	}
	i, err = os.Stat(s.Cwd)
	if err != nil || !i.IsDir() {
		return errors.New("The native working directory is unavailable.")
	}
	return nil
}

// Prepare establishes native ownership before any worker instruction. The caller must
// serialize its durable final launch guard with Start and accepted Stop.
func Prepare(ctx context.Context, s NativeSpec) (*Owned, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.New("Native preparation was interrupted.")
	}
	if err := validateSpec(s); err != nil {
		return nil, err
	}
	pipes, err := makePipes()
	if err != nil {
		return nil, errors.New("Cannot prepare native standard streams.")
	}
	o := &Owned{pipes: pipes, input: bytes.Clone(s.Stdin), rootDone: make(chan struct{}), faults: make(chan StreamFault, 3), stdinDone: make(chan struct{}), stdinState: StdinNotStarted, stdoutDone: make(chan struct{}), stderrDone: make(chan struct{}), cleanupDone: make(chan struct{})}
	native, err := prepareNative(ctx, s, pipes, o)
	pipes.closeChild()
	if err != nil {
		pipes.closeParent()
		return nil, err
	}
	o.native = native
	if s.Stdout == nil {
		s.Stdout = io.Discard
	}
	if s.Stderr == nil {
		s.Stderr = io.Discard
	}
	go o.drain("stdout", pipes.stdoutR, s.Stdout, o.stdoutDone)
	go o.drain("stderr", pipes.stderrR, s.Stderr, o.stderrDone)
	return o, nil
}

func (o *Owned) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.started {
		return errors.New("The owned process has already passed its launch gate.")
	}
	if o.closing {
		return errors.New("The owned process is already closing.")
	}
	if err := o.native.start(); err != nil {
		return err
	}
	o.started = true
	o.stdinState = StdinIncomplete // A released gate is not complete input-delivery proof.
	go o.deliverInput()
	return nil
}
func (o *Owned) RootDone() <-chan struct{} { return o.rootDone }
func (o *Owned) RootOutcome() (RootOutcome, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.root, o.rootSet
}
func (o *Owned) Faults() <-chan StreamFault { return o.faults }
func (o *Owned) setRoot(result RootOutcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.rootSet {
		return
	}
	o.root = result
	o.rootSet = true
	close(o.rootDone)
}
func rootResult(code int, known bool, err error) RootOutcome {
	return RootOutcome{Known: known, ExitCode: code, Err: err, ObservedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
}

func (o *Owned) drain(name string, r *os.File, w io.Writer, done chan struct{}) {
	defer close(done)
	defer r.Close()
	_, err := io.Copy(w, r)
	if err != nil {
		o.mu.Lock()
		if name == "stdout" {
			o.stdoutErr = true
		} else {
			o.stderrErr = true
		}
		o.mu.Unlock()
		o.reportFault(StreamFault{Stream: name, Message: "The owned stream could not be retained completely."})
	}
}

func (o *Owned) cleanupSnapshot() CleanupResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	r := o.cleanupResult
	r.StdinState = o.stdinState
	r.StdinBytesWritten = o.stdinBytesWritten
	r.ExpectedInputBytes = int64(len(o.input))
	r.StdinError = o.stdinError
	if o.rootSet {
		copy := o.root
		r.Root = &copy
	}
	select {
	case <-o.stdoutDone:
		r.StdoutDrained = !o.stdoutErr
	default:
	}
	select {
	case <-o.stderrDone:
		r.StderrDrained = !o.stderrErr
	default:
	}
	r.StdoutError = o.stdoutErr
	r.StderrError = o.stderrErr
	return r
}

func (o *Owned) Cleanup(ctx context.Context) CleanupResult {
	o.cleanupOnce.Do(func() {
		o.mu.Lock()
		o.closing = true
		if !o.started {
			close(o.stdinDone)
		}
		o.mu.Unlock()
		go func() {
			bounded, cancel := context.WithTimeout(ctx, CleanupLimit)
			defer cancel()
			verified, scope, detail := o.native.cleanup(bounded)
			o.pipes.stdinW.Close()
			for _, done := range []chan struct{}{o.rootDone, o.stdinDone, o.stdoutDone, o.stderrDone} {
				select {
				case <-done:
				case <-bounded.Done():
				}
			}
			if bounded.Err() != nil {
				o.pipes.stdoutR.Close()
				o.pipes.stderrR.Close()
			}
			o.mu.Lock()
			o.cleanupResult = CleanupResult{Verified: verified, Scope: scope, Detail: detail}
			o.mu.Unlock()
			close(o.cleanupDone)
		}()
	})
	select {
	case <-o.cleanupDone:
		return o.cleanupSnapshot()
	case <-ctx.Done():
		r := o.cleanupSnapshot()
		r.Verified = false
		r.Detail = "Owned cleanup could not be verified before the observation deadline."
		return r
	}
}
func (o *Owned) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), CleanupLimit)
	defer cancel()
	r := o.Cleanup(ctx)
	err := o.native.close()
	o.pipes.closeParent()
	if !r.Verified {
		return errors.New("Owned process cleanup remains unresolved.")
	}
	return err
}

func (o *Owned) reportFault(fault StreamFault) {
	select {
	case o.faults <- fault:
	default:
	}
}

func (o *Owned) deliverInput() {
	written, err := io.Copy(o.pipes.stdinW, bytes.NewReader(o.input))
	closeErr := o.pipes.stdinW.Close()
	if err == nil && closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
		err = closeErr
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.stdinBytesWritten = written
	o.stdinError = err != nil || written != int64(len(o.input))
	if o.stdinError {
		o.stdinState = StdinIncomplete
		o.reportFault(StreamFault{Stream: "stdin", Message: "The complete original input was not accepted by the owned standard-input pipe."})
	} else {
		o.stdinState = StdinComplete
	}
	close(o.stdinDone)
}
