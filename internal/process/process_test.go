package process

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

const testChildArg = "__council_process_test_child_v1"

func TestMain(m *testing.M) {
	if handled, code := HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := fixture.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if len(os.Args) > 1 && os.Args[1] == testChildArg {
		os.Exit(runTestChild(os.Args[2:]))
	}
	os.Exit(m.Run())
}
func runTestChild(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "marker":
		if len(args) != 2 {
			return 2
		}
		if os.WriteFile(args[1], []byte("first instruction"), 0600) != nil {
			return 3
		}
		return 0
	case "tail":
		os.Stdout.Write([]byte(strings.Repeat("o", 2*1024*1024) + "stdout tail π\n"))
		os.Stderr.Write([]byte(strings.Repeat("e", 512*1024) + "stderr tail 東京\n"))
		return 0
	case "environment":
		if len(args) != 1 {
			return 2
		}
		// Inspect only a test-owned sentinel; never print the inherited environment.
		value, present := os.LookupEnv("COUNCIL_PROCESS_TEST_ENV_V1")
		if !present {
			fmt.Fprint(os.Stdout, "absent")
		} else if value == "controlled-presence" {
			fmt.Fprint(os.Stdout, "present")
		} else {
			fmt.Fprint(os.Stdout, "unexpected")
		}
		return 0
	case "argv":
		data, _ := json.Marshal(args[1:])
		os.Stdout.Write(data)
		return 0
	case "early_terminal":
		fmt.Fprintln(os.Stdout, `{"type":"turn.completed","council_fixture":true,"fixture_contract":"002-v1"}`)
		for {
			time.Sleep(time.Second)
		}
	case "closed_streams":
		os.Stdout.Close()
		os.Stderr.Close()
		for {
			time.Sleep(time.Second)
		}
	case "linger":
		for {
			time.Sleep(time.Second)
		}
	case "platform":
		return platformTestChild(args[1:])
	case "crash_owner_before_go":
		if len(args) != 2 {
			return 2
		}
		self, _ := os.Executable()
		_, err := Prepare(context.Background(), NativeSpec{Executable: self, Args: []string{testChildArg, "marker", filepath.Join(args[1], "never.marker")}, Cwd: args[1]})
		if err != nil {
			return 4
		}
		return 0
	case "crash_owner":
		if len(args) != 2 {
			return 2
		}
		response := filepath.Join(args[1], "response.txt")
		fakeArgs, err := fixture.Args("owned_descendant", response)
		if err != nil {
			return 3
		}
		self, _ := os.Executable()
		o, err := Prepare(context.Background(), NativeSpec{Executable: self, Args: fakeArgs, Cwd: args[1], Stdout: io.Discard, Stderr: io.Discard})
		if err != nil {
			return 4
		}
		if o.Start() != nil {
			return 5
		}
		select {
		case <-o.RootDone():
		case <-time.After(5 * time.Second):
			return 6
		}
		return 0
	}
	return 2
}
func selfSpec(t *testing.T, args ...string) NativeSpec {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return NativeSpec{Executable: self, Args: append([]string{testChildArg}, args...), Cwd: t.TempDir()}
}
func waitRoot(t *testing.T, o *Owned) RootOutcome {
	t.Helper()
	select {
	case <-o.RootDone():
	case <-time.After(7 * time.Second):
		t.Fatal("root outcome was not observed")
	}
	r, ok := o.RootOutcome()
	if !ok {
		t.Fatal("root outcome was lost")
	}
	return r
}
func cleanupOwned(t *testing.T, o *Owned) CleanupResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), CleanupLimit)
	defer cancel()
	r := o.Cleanup(ctx)
	if !r.Verified || !r.StdoutDrained || !r.StderrDrained {
		t.Fatalf("cleanup was not completely verified: %+v", r)
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPrepareGatesFirstInstruction(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "first.marker")
	s := selfSpec(t, "marker", marker)
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("worker executed before Start")
	}
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	r := waitRoot(t, o)
	if !r.Known || r.ExitCode != 0 {
		t.Fatalf("wrong root outcome %+v", r)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("worker did not pass launch gate")
	}
	cleanupOwned(t, o)
	if err := o.Start(); err == nil {
		t.Fatal("launch gate replayed")
	}
}
func TestConcurrentStreamsRetainExactTails(t *testing.T) {
	var out, diagnostics bytes.Buffer
	s := selfSpec(t, "tail")
	s.Stdout = &out
	s.Stderr = &diagnostics
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	r := waitRoot(t, o)
	if !r.Known || r.ExitCode != 0 {
		t.Fatal(r)
	}
	cleanupOwned(t, o)
	if out.String() != strings.Repeat("o", 2*1024*1024)+"stdout tail π\n" || diagnostics.String() != strings.Repeat("e", 512*1024)+"stderr tail 東京\n" {
		t.Fatal("stdout/stderr bytes were clipped or changed")
	}
}
func TestExplicitArgumentVector(t *testing.T) {
	want := []string{"", "spaces and 東京", `quote"inside`, `slash\tail\`, `$(not-a-shell)`}
	var out bytes.Buffer
	s := selfSpec(t, append([]string{"argv"}, want...)...)
	s.Stdout = &out
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	waitRoot(t, o)
	cleanupOwned(t, o)
	var got []string
	if json.Unmarshal(out.Bytes(), &got) != nil {
		t.Fatal("argument output invalid")
	}
	expected, _ := json.Marshal(want)
	actual, _ := json.Marshal(got)
	if !bytes.Equal(expected, actual) {
		t.Fatalf("arguments changed: %s", actual)
	}
}
func TestRootOutcomePrecedesDescendantCleanup(t *testing.T) {
	dir := t.TempDir()
	response := filepath.Join(dir, "response.txt")
	args, err := fixture.Args("owned_descendant", response)
	if err != nil {
		t.Fatal(err)
	}
	self, _ := os.Executable()
	o, err := Prepare(context.Background(), NativeSpec{Executable: self, Args: args, Cwd: dir, Stdin: []byte("exact fixture input"), Stdout: io.Discard, Stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	r := waitRoot(t, o)
	if !r.Known || r.ExitCode != 0 {
		t.Fatal(r)
	}
	marker := filepath.Join(dir, "fixture-descendant.marker")
	before, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	// Root exit does not settle inherited descendant streams or imply cleanup.
	select {
	case <-o.stdoutDone:
		t.Fatal("descendant did not retain its inherited output scope")
	default:
	}
	cleanupOwned(t, o)
	after, _ := os.ReadFile(marker)
	time.Sleep(80 * time.Millisecond)
	stable, _ := os.ReadFile(marker)
	if !bytes.Equal(after, stable) {
		t.Fatal("owned descendant survived verified cleanup")
	}
	if len(before) == 0 {
		t.Fatal("descendant first instruction was missing")
	}
}
func TestCleanupBeforeStartNeverRunsWorker(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "never.marker")
	o, err := Prepare(context.Background(), selfSpec(t, "marker", marker))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	cleanupOwned(t, o)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("cleanup released a prepared worker")
	}
	if o.Start() == nil {
		t.Fatal("closed launch gate restarted")
	}
}

type rejectingWriter struct{}

func (rejectingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestStreamFailureIsSeparateFromRootAndCleanup(t *testing.T) {
	s := selfSpec(t, "tail")
	s.Stdout = rejectingWriter{}
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	select {
	case fault := <-o.Faults():
		if fault.Stream != "stdout" {
			t.Fatal(fault)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream fault was not observable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), CleanupLimit)
	defer cancel()
	r := o.Cleanup(ctx)
	if !r.Verified || !r.StdoutError || r.StdoutDrained {
		t.Fatalf("stream fault was erased: %+v", r)
	}
}
func TestOwnerCrashStopsOwnedDescendant(t *testing.T) {
	dir := t.TempDir()
	self, _ := os.Executable()
	cmd := exec.Command(self, testChildArg, "crash_owner", dir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "fixture-descendant.marker")
	deadline := time.Now().Add(5 * time.Second)
	var stable []byte
	observedStable := false
	for time.Now().Before(deadline) {
		first, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(80 * time.Millisecond)
		second, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(first, second) {
			stable = second
			observedStable = true
			break
		}
	}
	if !observedStable {
		t.Fatal("descendant remained active after owner crash")
	}
	time.Sleep(100 * time.Millisecond)
	after, _ := os.ReadFile(marker)
	if !bytes.Equal(stable, after) {
		t.Fatal("descendant resumed after owner crash cleanup")
	}
}
func TestNativeSpecValidation(t *testing.T) {
	for _, s := range []NativeSpec{{Executable: "relative", Cwd: t.TempDir()}, {Executable: os.Args[0], Cwd: "relative"}, {Executable: os.Args[0], Cwd: t.TempDir(), Args: []string{"\x00"}}, {Executable: os.Args[0], Cwd: t.TempDir(), Stdin: make([]byte, 256*1024+1)}, {Executable: os.Args[0], Cwd: t.TempDir(), Args: []string{strings.Repeat("x", 1024*1024+1)}}} {
		if o, err := Prepare(context.Background(), s); err == nil {
			o.Close()
			t.Fatal("invalid process input accepted")
		}
	}
}
func TestCleanupConcurrentObservers(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "linger"))
	if err != nil {
		t.Fatal(err)
	}
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), CleanupLimit)
			defer cancel()
			if !o.Cleanup(ctx).Verified {
				t.Error("shared cleanup lost its proof")
			}
		}()
	}
	wg.Wait()
	o.Close()
}

func TestOwnerCrashBeforeGateDoesNotStartWorker(t *testing.T) {
	dir := t.TempDir()
	self, _ := os.Executable()
	cmd := exec.Command(self, testChildArg, "crash_owner_before_go", dir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "never.marker")); !os.IsNotExist(err) {
		t.Fatal("owner loss released a prepared worker")
	}
}

type blockedWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockedWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(data), nil
}
func TestBlockedDrainReturnsIncompleteWithinDeadline(t *testing.T) {
	writer := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	s := selfSpec(t, "tail")
	s.Stdout = writer
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	select {
	case <-writer.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not enter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := o.Cleanup(ctx)
	close(writer.release)
	if time.Since(started) > time.Second || result.StdoutDrained {
		t.Fatal("blocked drain was reported complete or exceeded its bounded observation")
	}
}

type notifyingWriter struct {
	ready chan struct{}
	once  sync.Once
}

func (w *notifyingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.ready) })
	return len(data), nil
}
func TestTerminalBytesDoNotBecomeARootExit(t *testing.T) {
	output := &notifyingWriter{ready: make(chan struct{})}
	s := selfSpec(t, "early_terminal")
	s.Stdout = output
	o, err := Prepare(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	select {
	case <-output.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("early terminal bytes missing")
	}
	select {
	case <-o.RootDone():
		t.Fatal("terminal bytes became a fabricated root outcome")
	default:
	}
	cleanupOwned(t, o)
}
func TestStreamEOFDoesNotBecomeARootExit(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "closed_streams"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Start() != nil {
		t.Fatal("start failed")
	}
	for _, done := range []chan struct{}{o.stdoutDone, o.stderrDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("fixture streams did not close")
		}
	}
	select {
	case <-o.RootDone():
		t.Fatal("closed output streams became a fabricated root outcome")
	default:
	}
	cleanupOwned(t, o)
}
