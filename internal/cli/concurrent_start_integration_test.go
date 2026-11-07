package cli

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"path/filepath"
	"reflect"
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

func TestConcurrentIndependentCLIStartsShareTwoSlotsAndSerializeCapturedContext(t *testing.T) {
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
			if admitted.Version.Pins.HarnessID != []string{"codex", "claude-code"}[result.index] || admitted.Attempt.ConnectionID != []string{"codex-native", "claude-native"}[result.index] ||
				admitted.Attempt.ResolvedTarget == nil || admitted.Attempt.ResolvedTarget.ContextID != []string{"codex-native", "claude-native"}[result.index] ||
				admitted.Attempt.ResolvedTarget.ConnectionID != admitted.Attempt.ConnectionID || admitted.Attempt.ResolvedTarget.HarnessID != admitted.Version.Pins.HarnessID {
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
	captured := map[string]domain.Attempt{}
	for _, admitted := range admissions {
		captured[admitted.Attempt.ID] = admitted.Attempt
	}
	read := func(args ...string) domain.Envelope {
		t.Helper()
		return concurrentCLIEnvelope(t, concurrentCLICommand(ctx, f, 0, args...))
	}
	get := func(id string) domain.Attempt {
		t.Helper()
		var attempt domain.Attempt
		decodeCLIData(t, read("attempts", "get", id), &attempt)
		expected, exists := captured[id]
		if !exists || attempt.ID != id || attempt.OwnerInstanceID != ownerID || attempt.TaskID != expected.TaskID ||
			attempt.StartRequestID != expected.StartRequestID || attempt.ConnectionID != expected.ConnectionID ||
			!reflect.DeepEqual(attempt.ResolvedTarget, expected.ResolvedTarget) ||
			attempt.ExecutionMode != "fake" || attempt.FakeScenario != "slow" {
			t.Fatalf("observation changed the admitted attempt/owner/target identity: %+v", attempt)
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

	// Schema6's global pool permits two independent captured contexts. Neither
	// this fresh home nor these starts installs a legacy or shared-account barrier.
	var observed [2]domain.Attempt
	for {
		for index := range admissions {
			observed[index] = get(admissions[index].Attempt.ID)
			if observed[index].Phase == "settled" {
				t.Fatalf("slow fixture settled before the two-worker overlap observation: %+v", observed[index])
			}
		}
		if observed[0].Phase == "running" && observed[1].Phase == "running" {
			break
		}
		pause()
	}
	codex, claude := observed[0], observed[1]
	for _, attempt := range observed {
		if !attempt.LaunchIntent || attempt.StartedAt == nil || attempt.SourceSnapshotID == nil {
			t.Fatalf("two-context capacity did not start an actual owned worker: %+v", attempt)
		}
	}
	if *codex.SourceSnapshotID == *claude.SourceSnapshotID {
		t.Fatal("distinct native workers reused one retained source identity")
	}

	thirdRequest := domain.NewID()
	thirdEnvelope := read("tasks", "start", "--project", project.Project.ID, "--harness", "codex",
		"--task", "third controlled Codex waits for its captured context", "--deliverable", "response_only",
		"--fake", "--fake-scenario", "slow", "--timeout-seconds", "60", "--request-id", thirdRequest)
	var thirdAdmission domain.Admission
	decodeCLIData(t, thirdEnvelope, &thirdAdmission)
	third := thirdAdmission.Attempt
	if thirdEnvelope.RequestID != thirdRequest || thirdEnvelope.Receipt == nil || thirdEnvelope.Receipt.RequestID != thirdRequest ||
		thirdEnvelope.Receipt.Operation != "tasks.start" || third.StartRequestID != thirdRequest ||
		thirdAdmission.Version.Pins.HarnessID != "codex" || third.ConnectionID != "codex-native" ||
		third.ResolvedTarget == nil || third.ResolvedTarget.ContextID != "codex-native" || third.ResolvedTarget.ConnectionID != "codex-native" || third.ResolvedTarget.HarnessID != "codex" ||
		third.OwnerInstanceID != ownerID || third.ExecutionMode != "fake" || third.FakeScenario != "slow" ||
		third.ID == "" || third.TaskID == "" || third.ID == codex.ID || third.ID == claude.ID ||
		third.TaskID == codex.TaskID || third.TaskID == claude.TaskID {
		t.Fatalf("third independent client lost its exact request/owner/captured context: %+v", thirdAdmission)
	}
	captured[third.ID] = third
	assertQueued := func(stage string) {
		t.Helper()
		waiting := get(third.ID)
		if waiting.Phase != "queued" || waiting.LaunchIntent || waiting.StartedAt != nil ||
			waiting.SourceSnapshotID != nil || waiting.SourceSnapshot != nil {
			t.Fatalf("%s: waiting captured context acquired preparation or native effects: %+v", stage, waiting)
		}
		for _, event := range events(third.ID) {
			if event.Kind == "preparation_started" || event.Kind == "launch_intent" || event.Kind == "worker_started" {
				t.Fatalf("%s: queued attempt escaped its occupied capacity/context: %+v", stage, event)
			}
		}
	}
	assertQueued("both global slots occupied")
	if get(codex.ID).Phase != "running" || get(claude.ID).Phase != "running" {
		t.Fatal("both actual workers must remain active while the third waits")
	}
	stopAndSettle := func(attemptID string) domain.Attempt {
		t.Helper()
		read("attempts", "stop", attemptID)
		for {
			current := get(attemptID)
			if current.Phase == "settled" {
				if current.Outcome == nil || *current.Outcome != "cancelled" || current.Cleanup.State != "verified" || current.Cleanup.Scope == "not_launched" {
					t.Fatalf("Stop did not retain verified actual native settlement: %+v", current)
				}
				return current
			}
			pause()
		}
	}

	// A free global slot alone cannot bypass the still occupied Codex context.
	// Final event ordering below checks the entire held interval, not just a poll.
	claude = stopAndSettle(claude.ID)
	if current := get(codex.ID); current.Phase != "running" {
		t.Fatalf("releasing Claude changed the independently running Codex worker: %+v", current)
	}
	assertQueued("Claude released but the captured Codex context remains occupied")
	codex = stopAndSettle(codex.ID)
	for {
		third = get(third.ID)
		if third.Phase == "running" {
			break
		}
		if third.Phase == "settled" {
			t.Fatalf("waiting Codex did not obtain its actually released context: %+v", third)
		}
		pause()
	}
	if !third.LaunchIntent || third.StartedAt == nil || third.SourceSnapshotID == nil {
		t.Fatalf("the released exact context did not launch the queued worker: %+v", third)
	}
	third = stopAndSettle(third.ID)

	// Global persisted event sequences prove actual first-pair overlap, then
	// third-worker preparation only after its captured context's known settlement.
	// Each attempt must still retain exactly one complete native lifecycle.
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
	codexLifecycle, claudeLifecycle, thirdLifecycle := lifecycle(codex.ID), lifecycle(claude.ID), lifecycle(third.ID)
	if codexLifecycle["worker_started"] >= claudeLifecycle["settled"] || claudeLifecycle["worker_started"] >= codexLifecycle["settled"] {
		t.Fatalf("the two actual distinct-context workers did not overlap: codex=%+v claude=%+v", codexLifecycle, claudeLifecycle)
	}
	if claudeLifecycle["settled"] >= codexLifecycle["settled"] || codexLifecycle["settled"] >= thirdLifecycle["preparation_started"] {
		t.Fatalf("third preparation bypassed the full pool or occupied captured context: codex=%+v claude=%+v third=%+v", codexLifecycle, claudeLifecycle, thirdLifecycle)
	}
	if current, err := localstate.ReadDescriptor(filepath.Join(f.home, ".council", "v2")); err != nil || current.OwnerInstanceID != ownerID {
		t.Fatalf("the independent task starts did not retain one owner through cleanup: owner=%s err=%v", current.OwnerInstanceID, err)
	}
}
