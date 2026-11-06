package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type concurrentCLIResult struct {
	index          int
	pid            int
	stdout, stderr []byte
	err            error
}

func concurrentCLICommand(ctx context.Context, f *cliFixture, index int, args ...string) concurrentCLIResult {
	command := f.command(ctx, append(args, "--json")...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	result := concurrentCLIResult{index: index, stdout: bytes.Clone(stdout.Bytes()), stderr: bytes.Clone(stderr.Bytes()), err: err}
	if command.Process != nil {
		result.pid = command.Process.Pid
	}
	return result
}

func concurrentCLIEnvelope(t *testing.T, result concurrentCLIResult) domain.Envelope {
	t.Helper()
	var envelope domain.Envelope
	if err := json.Unmarshal(result.stdout, &envelope, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("independent CLI process returned invalid JSON: process=%d run=%v decode=%v stdout=%q stderr=%q", result.pid, result.err, err, result.stdout, result.stderr)
	}
	if result.err != nil || envelope.Error != nil {
		t.Fatalf("independent CLI process failed: process=%d run=%v error=%+v stderr=%q", result.pid, result.err, envelope.Error, result.stderr)
	}
	return envelope
}

func TestConcurrentIndependentCLIStartsSerializeNativeWorkers(t *testing.T) {
	f := newCLIFixture(t) // Registers authenticated owner cleanup before admission.
	var project struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &project)
	// Project preparation uses transient ownership. Both starts below independently
	// discover/bootstrap the continuing owner against the same private home.
	if descriptor, err := localstate.ReadDescriptor(filepath.Join(f.home, ".council", "v2")); err == nil {
		t.Fatalf("fixture unexpectedly has a continuing owner before the concurrent starts: %s", descriptor.OwnerInstanceID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	requestIDs := [2]string{domain.NewID(), domain.NewID()}
	barrier := make(chan struct{})
	results := make(chan concurrentCLIResult, 2)
	for index := range requestIDs {
		harness := []string{"codex", "claude-code"}[index]
		args := []string{"tasks", "start", "--project", project.Project.ID, "--harness", harness,
			"--task", fmt.Sprintf("independent concurrent fake task %d", index), "--deliverable", "response_only",
			"--fake", "--fake-scenario", "slow", "--timeout-seconds", "60", "--request-id", requestIDs[index]}
		go func(index int, args []string) {
			<-barrier
			results <- concurrentCLICommand(ctx, f, index, args...)
		}(index, args)
	}
	close(barrier)
	var admissions [2]domain.Admission
	var processIDs [2]int
	for range admissions {
		select {
		case result := <-results:
			envelope := concurrentCLIEnvelope(t, result)
			decodeCLIData(t, envelope, &admissions[result.index])
			processIDs[result.index] = result.pid
			admitted := admissions[result.index]
			if admitted.Version.Pins.HarnessID != []string{"codex", "claude-code"}[result.index] || admitted.Attempt.ConnectionID != []string{"codex-native", "claude-native"}[result.index] {
				t.Fatalf("independent clients lost their captured different targets: %+v", admitted)
			}
			if envelope.RequestID != requestIDs[result.index] || envelope.Receipt == nil || envelope.Receipt.RequestID != requestIDs[result.index] || envelope.Receipt.Operation != "tasks.start" || admitted.Attempt.StartRequestID != requestIDs[result.index] {
				t.Fatalf("concurrent admission did not retain its distinct request identity: %+v", admitted)
			}
			if admitted.Task.ID == "" || admitted.Attempt.ID == "" || admitted.Attempt.ExecutionMode != "fake" || admitted.Attempt.FakeScenario != "slow" {
				t.Fatalf("concurrent admission lost its task/attempt/fake identity: %+v", admitted)
			}
		case <-ctx.Done():
			t.Fatal("independent concurrent CLI starts exceeded their bounded context")
		}
	}
	if processIDs[0] <= 0 || processIDs[0] == processIDs[1] || admissions[0].Task.ID == admissions[1].Task.ID || admissions[0].Attempt.ID == admissions[1].Attempt.ID {
		t.Fatal("concurrent starts did not come from distinct native clients with distinct tasks and attempts")
	}
	ownerID := admissions[0].Attempt.OwnerInstanceID
	descriptor, err := localstate.ReadDescriptor(filepath.Join(f.home, ".council", "v2"))
	if err != nil || ownerID == "" || ownerID != admissions[1].Attempt.OwnerInstanceID || ownerID != descriptor.OwnerInstanceID {
		t.Fatalf("concurrent bootstrap did not converge on one continuing owner: first=%s second=%s descriptor=%s err=%v", ownerID, admissions[1].Attempt.OwnerInstanceID, descriptor.OwnerInstanceID, err)
	}
	read := func(args ...string) domain.Envelope {
		t.Helper()
		return concurrentCLIEnvelope(t, concurrentCLICommand(ctx, f, 0, args...))
	}
	get := func(id string) domain.Attempt {
		t.Helper()
		var attempt domain.Attempt
		decodeCLIData(t, read("attempts", "get", id), &attempt)
		if attempt.ID != id || attempt.OwnerInstanceID != ownerID {
			t.Fatalf("observation changed the admitted attempt/owner identity: %+v", attempt)
		}
		return attempt
	}
	pause := func() {
		t.Helper()
		select {
		case <-time.After(25 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("concurrent worker observation exceeded its bounded context")
		}
	}
	active, queued := -1, -1
	var observed [2]domain.Attempt
	for active < 0 {
		for index := range admissions {
			observed[index] = get(admissions[index].Attempt.ID)
			if observed[index].Phase == "settled" {
				t.Fatalf("slow fixture settled before the serialization observation: %+v", observed[index])
			}
		}
		if observed[0].LaunchIntent && observed[1].LaunchIntent {
			t.Fatalf("two native launch intents escaped the single-worker queue: %+v", observed)
		}
		for index := range observed {
			other := 1 - index
			if observed[index].Phase == "running" && observed[other].Phase == "queued" {
				active, queued = index, other
			}
		}
		if active < 0 {
			pause()
		}
	}
	first, second := observed[active], observed[queued]
	if !first.LaunchIntent || first.StartedAt == nil || first.SourceSnapshotID == nil || second.LaunchIntent || second.StartedAt != nil || second.SourceSnapshotID != nil || second.SourceSnapshot != nil {
		t.Fatalf("queued attempt acquired source or native launch evidence while capacity was occupied: active=%+v queued=%+v", first, second)
	}
	events := func(id string) []domain.AttemptEvent {
		t.Helper()
		var all []domain.AttemptEvent
		cursor := ""
		for range 10 {
			args := []string{"attempts", "events", id, "--limit", "100"}
			if cursor != "" {
				args = append(args, "--cursor", cursor)
			}
			var page domain.Page[domain.AttemptEvent]
			decodeCLIData(t, read(args...), &page)
			all = append(all, page.Items...)
			if page.NextCursor == nil {
				return all
			}
			cursor = *page.NextCursor
		}
		t.Fatal("fixture event history exceeded its bounded page count")
		return nil
	}
	for _, event := range events(second.ID) {
		if event.Kind == "preparation_started" || event.Kind == "launch_intent" || event.Kind == "worker_started" {
			t.Fatalf("queued attempt escaped capacity before the first Stop: %+v", event)
		}
	}
	read("attempts", "stop", first.ID)
	for {
		first = get(first.ID)
		if first.Phase == "settled" {
			break
		}
		pause()
	}
	if first.Outcome == nil || *first.Outcome != "cancelled" || first.Cleanup.State != "verified" || first.Cleanup.Scope == "not_launched" {
		t.Fatalf("first Stop did not settle verified native ownership: %+v", first)
	}
	for {
		second = get(second.ID)
		if second.Phase == "running" {
			break
		}
		if second.Phase == "settled" {
			t.Fatalf("queued attempt did not obtain the released worker capacity: %+v", second)
		}
		pause()
	}
	if !second.LaunchIntent || second.StartedAt == nil || second.SourceSnapshotID == nil {
		t.Fatalf("released capacity did not launch the queued attempt: %+v", second)
	}
	read("attempts", "stop", second.ID)
	for {
		second = get(second.ID)
		if second.Phase == "settled" {
			break
		}
		pause()
	}
	if second.Outcome == nil || *second.Outcome != "cancelled" || second.Cleanup.State != "verified" || second.Cleanup.Scope == "not_launched" {
		t.Fatalf("second Stop did not settle verified native ownership: %+v", second)
	}
	// Event sequences are global persisted order. Exactly one launch/start/settled
	// event per attempt, and a second preparation claim after first settlement,
	// prove that these two independently admitted clients never overlap workers.
	lifecycle := func(attemptID string) map[string]int64 {
		t.Helper()
		ordered := map[string]int64{}
		for _, event := range events(attemptID) {
			if event.AttemptID != attemptID {
				t.Fatal("event history crossed attempt identities")
			}
			switch event.Kind {
			case "preparation_started", "launch_intent", "worker_started", "settled":
				if ordered[event.Kind] != 0 {
					t.Fatalf("attempt %s repeated lifecycle event %s", attemptID, event.Kind)
				}
				sequence, err := domain.ParseRevision(event.Sequence)
				if err != nil {
					t.Fatalf("invalid retained event sequence: %+v", event)
				}
				ordered[event.Kind] = sequence
			}
		}
		if !(0 < ordered["preparation_started"] && ordered["preparation_started"] < ordered["launch_intent"] && ordered["launch_intent"] < ordered["worker_started"] && ordered["worker_started"] < ordered["settled"]) {
			t.Fatalf("attempt does not retain exactly one ordered worker lifecycle: %s %+v", attemptID, ordered)
		}
		return ordered
	}
	firstLifecycle, secondLifecycle := lifecycle(first.ID), lifecycle(second.ID)
	if firstLifecycle["settled"] >= secondLifecycle["preparation_started"] {
		t.Fatalf("second worker capacity was claimed before verified first settlement: first=%+v second=%+v", firstLifecycle, secondLifecycle)
	}
	if current, err := localstate.ReadDescriptor(filepath.Join(f.home, ".council", "v2")); err != nil || current.OwnerInstanceID != ownerID {
		t.Fatalf("the two independent task starts did not retain one owner through cleanup: %+v %v", current, err)
	}
}
