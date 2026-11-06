package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/owner"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

const cliTestArg = "__council_cli_test_v1"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == maintenanceFixtureArg {
		os.Exit(runMaintenanceFixture(os.Args[2:]))
	}
	if handled, code := process.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if handled, code := fixture.HandleInternal(os.Args[1:]); handled {
		os.Exit(code)
	}
	if len(os.Args) > 1 && os.Args[1] == owner.InternalArg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		handled, code := HandleInternal(ctx, os.Args[1:], io.Discard)
		cancel()
		if handled {
			os.Exit(code)
		}
	}
	if len(os.Args) > 1 && os.Args[1] == cliTestArg {
		os.Exit(Run(context.Background(), os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

type cliFixture struct {
	t                      *testing.T
	executable, home, repo string
}

func newCLIFixture(t *testing.T) *cliFixture {
	t.Helper()
	f := &cliFixture{t: t, home: t.TempDir(), repo: t.TempDir()}
	f.executable, _ = os.Executable()
	t.Cleanup(f.cleanup)
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		c := exec.Command("git", append([]string{"-C", f.repo}, args...)...)
		c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if output, err := c.CombinedOutput(); err != nil {
			t.Fatalf("Git fixture setup failed: %v %s", err, output)
		}
	}
	return f
}
func (f *cliFixture) command(ctx context.Context, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, f.executable, append([]string{cliTestArg}, args...)...)
	c.Dir = f.repo
	c.Env = append(os.Environ(), "HOME="+f.home, "USERPROFILE="+f.home)
	return c
}
func (f *cliFixture) run(args ...string) domain.Envelope {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	c := f.command(ctx, append(args, "--json")...)
	var out, diagnostics bytes.Buffer
	c.Stdout = &out
	c.Stderr = &diagnostics
	err := c.Run()
	var envelope domain.Envelope
	if json.Unmarshal(out.Bytes(), &envelope, json.RejectUnknownMembers(true)) != nil {
		f.t.Fatalf("CLI did not return one envelope: %v stdout=%q stderr=%q", err, out.String(), diagnostics.String())
	}
	if err != nil || envelope.Error != nil {
		f.t.Fatalf("CLI command failed: %v %+v stderr=%q", err, envelope.Error, diagnostics.String())
	}
	return envelope
}
func decodeCLIData(t *testing.T, envelope domain.Envelope, out any) {
	t.Helper()
	data, err := json.Marshal(envelope.Data)
	if err != nil || json.Unmarshal(data, out) != nil {
		t.Fatal("CLI result data does not satisfy its contract")
	}
}
func (f *cliFixture) settled(id string) domain.Attempt {
	f.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var attempt domain.Attempt
	for time.Now().Before(deadline) {
		decodeCLIData(f.t, f.run("attempts", "get", id), &attempt)
		if attempt.Phase == "settled" {
			return attempt
		}
		time.Sleep(50 * time.Millisecond)
	}
	f.t.Fatalf("Attempt did not settle within fixture bound: phase=%s failure=%+v cleanup=%+v incident=%+v snapshot=%v candidate=%v response=%v", attempt.Phase, attempt.PrimaryFailure, attempt.Cleanup, attempt.Incident, attempt.SourceSnapshotID, attempt.CandidateID, attempt.ResponseArtifactID)
	return domain.Attempt{}
}

func TestBackgroundCLIExecutionReplayObservationAndExactBytes(t *testing.T) {
	f := newCLIFixture(t)
	var project struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &project)
	startID := domain.NewID()
	startArgs := []string{"tasks", "start", "--project", project.Project.ID, "--task", "\ufeffExact independent CLI task\r\n東京\n", "--deliverable", "response_only", "--fake", "--timeout-seconds", "30", "--request-id", startID}
	first := f.run(startArgs...)
	var admitted domain.Admission
	decodeCLIData(t, first, &admitted)
	if admitted.Attempt.ExecutionMode != "fake" || admitted.Attempt.ID == "" || first.Receipt == nil {
		t.Fatal("fake admission did not retain its identity")
	}
	replayed := f.run(startArgs...)
	if !reflect.DeepEqual(first, replayed) {
		a, _ := json.Marshal(first, json.Deterministic(true))
		b, _ := json.Marshal(replayed, json.Deterministic(true))
		t.Fatalf("lost-ack retry changed the admitted attempt or original receipt: first=%s replay=%s", a, b)
	}
	finished := f.settled(admitted.Attempt.ID)
	if finished.Outcome == nil || *finished.Outcome != "succeeded" || finished.Cleanup.State != "verified" || finished.ResponseArtifactID == nil {
		t.Fatalf("fake authoring did not retain successful scoped settlement: %+v", finished)
	}
	var task domain.TaskDetail
	decodeCLIData(t, f.run("tasks", "get", admitted.Task.ID), &task)
	if task.Task.OriginalText != "\ufeffExact independent CLI task\r\n東京\n" {
		t.Fatal("CLI owner changed original text")
	}
	var result []byte
	offset := "0"
	through := ""
	for range 256 {
		args := []string{"artifacts", "read", *finished.ResponseArtifactID, "--offset", offset, "--limit", "7"}
		if through != "" {
			args = append(args, "--through", through)
		}
		var chunk struct {
			Offset      string `json:"offset"`
			NextOffset  string `json:"nextOffset"`
			Through     string `json:"through"`
			BytesBase64 string `json:"bytesBase64"`
			EOF         bool   `json:"eof"`
		}
		decodeCLIData(t, f.run(args...), &chunk)
		if through != "" && chunk.Through != through {
			t.Fatal("byte snapshot changed")
		}
		through = chunk.Through
		raw, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, raw...)
		offset = chunk.NextOffset
		if chunk.EOF {
			break
		}
	}
	if string(result) != fixture.ResponseText {
		t.Fatal("chunk boundaries changed exact response bytes")
	}
	// A separately killed observer has no lifecycle authority over this slow attempt.
	var slow domain.Admission
	decodeCLIData(t, f.run("tasks", "start", "--project", project.Project.ID, "--task", "slow observer fixture", "--fake", "--fake-scenario", "slow", "--timeout-seconds", "30"), &slow)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var current domain.Attempt
		decodeCLIData(t, f.run("attempts", "get", slow.Attempt.ID), &current)
		if current.Phase == "running" {
			break
		}
		if current.Phase == "settled" {
			t.Fatalf("slow fixture settled before observation: %+v", current)
		}
		time.Sleep(50 * time.Millisecond)
	}
	observerCtx, cancelObserver := context.WithTimeout(context.Background(), 200*time.Millisecond)
	observer := f.command(observerCtx, "attempts", "wait", slow.Attempt.ID, "--after-revision", "9223372036854775807", "--timeout-ms", "30000", "--json")
	observer.Stdout = io.Discard
	observer.Stderr = io.Discard
	_ = observer.Run()
	cancelObserver()
	var stillRunning domain.Attempt
	decodeCLIData(t, f.run("attempts", "get", slow.Attempt.ID), &stillRunning)
	if stillRunning.Stop != nil || stillRunning.Phase == "settled" {
		t.Fatal("observer loss cancelled or settled owned work")
	}
	f.run("attempts", "stop", slow.Attempt.ID)
	stopped := f.settled(slow.Attempt.ID)
	if stopped.Outcome == nil || *stopped.Outcome != "cancelled" || stopped.Cleanup.State != "verified" {
		t.Fatalf("explicit Stop did not retain cancelled settlement: %+v", stopped)
	}
}

func (f *cliFixture) cleanup() {
	root := filepath.Join(f.home, ".council", "v2")
	descriptor, err := localstate.ReadDescriptor(root)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"requestId": domain.NewID(), "ownerInstanceId": descriptor.OwnerInstanceID})
	request, err := http.NewRequestWithContext(ctx, "POST", descriptor.Origin+"/api/v1/service/stop", bytes.NewReader(body))
	if err == nil {
		request.Header.Set("Authorization", "Bearer "+descriptor.Token)
		request.Header.Set("Origin", descriptor.Origin)
		request.Header.Set("Content-Type", "application/json")
		response, err := (&http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 20 * time.Second}).Do(request)
		if err == nil {
			response.Body.Close()
		}
	}
	for ctx.Err() == nil {
		lock, err := localstate.Acquire(root)
		if err == nil {
			lock.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Error("the explicitly owned CLI fixture did not settle its native owner lock")
}
