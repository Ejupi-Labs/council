package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type observationWait struct {
	Attempt  domain.Attempt `json:"attempt"`
	Changed  bool           `json:"changed"`
	TimedOut bool           `json:"timedOut"`
}

func observationData[T any](t *testing.T, result domain.Result) T {
	t.Helper()
	if result.Status != 200 || result.Envelope.Error != nil || result.Envelope.Receipt != nil {
		t.Fatalf("Read failed or created a mutation receipt: %+v", result)
	}
	raw, err := json.Marshal(result.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var value T
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func stoppedObservationFixture(t *testing.T) (*Engine, domain.Admission, string) {
	t.Helper()
	e, project, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	admission := decodeAdmission(t, e.db.CreateTask(context.Background(), e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Inspect this exact local fixture without executing it.", ExecutionMode: "fake"}))
	return e, admission, repo
}

func TestWaitRevisionZeroAndFutureUseStrictGreaterWithoutExecution(t *testing.T) {
	e, admission, _ := stoppedObservationFixture(t)
	ctx := context.Background()
	id := admission.Attempt.ID
	before, err := e.db.Task(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, after, timeout string
		changed, timedOut    bool
	}{{"zero", "0", "0", true, false}, {"equal", admission.Attempt.Revision, "0", false, true}, {"future_exact_int64", "9223372036854775807", "0", false, true}, {"bounded_timeout", admission.Attempt.Revision, "20", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			got := observationData[observationWait](t, e.Read(readCtx, "attempts.wait", id, map[string]string{"afterRevision": tc.after, "timeoutMs": tc.timeout}))
			if got.Attempt.ID != id || got.Attempt.Phase != "queued" || got.Changed != tc.changed || got.TimedOut != tc.timedOut || got.Attempt.LaunchIntent || got.Attempt.Stop != nil {
				t.Fatalf("Incorrect wait observation: %+v", got)
			}
			if got.Attempt.SourceSnapshot != nil || got.Attempt.Candidate != nil || got.Attempt.Incident != nil {
				t.Fatal("Missing optional observations were fabricated")
			}
		})
	}
	for _, after := range []string{"00", "-1", "9223372036854775808"} {
		r := e.Read(ctx, "attempts.wait", id, map[string]string{"afterRevision": after, "timeoutMs": "0"})
		if r.Status != 400 || r.Envelope.Error == nil {
			t.Fatal("Invalid exact afterRevision accepted", after, r)
		}
	}
	after, err := e.db.Task(ctx, admission.Task.ID)
	if err != nil || after.Task.Revision != before.Task.Revision || after.Task.StateRevision != before.Task.StateRevision || after.LatestAttempt.Revision != before.LatestAttempt.Revision || after.LatestAttempt.Phase != "queued" {
		t.Fatal("Observation changed task state", after, err)
	}
	events, err := e.db.Events(ctx, id, 100, "")
	if err != nil || len(events.Items) != 1 || events.Items[0].Kind != "admitted" {
		t.Fatal("Wait launched, stopped or otherwise mutated the attempt", events, err)
	}
}

func TestOptionalInspectionIsEnrichedAcrossAttemptTaskAndWait(t *testing.T) {
	e, admission, _ := stoppedObservationFixture(t)
	ctx := context.Background()
	id := admission.Attempt.ID
	queued := observationData[domain.Attempt](t, e.Read(ctx, "attempts.get", id, nil))
	if queued.SourceSnapshot != nil || queued.Candidate != nil || queued.Incident != nil {
		t.Fatal("Queued inspection fabricated unavailable facts")
	}
	if _, err := e.db.ClaimAttempt(ctx, id, e.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	// Controlled closed records test inspection joins, not source/worktree execution.
	manifest, err := e.evidence.Put(ctx, id, "source_manifest", []byte(`{"fixture":"source"}`))
	if err != nil {
		t.Fatal(err)
	}
	index, err := e.evidence.Put(ctx, id, "index_snapshot", []byte("fixture index"))
	if err != nil {
		t.Fatal(err)
	}
	source := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: id, ManifestArtifactID: manifest.ID, IndexArtifactID: index.ID, ManifestSHA256: *manifest.SHA256, BaseCommit: strings.Repeat("1", 40), ProjectIdentity: "controlled-observation-fixture", SelectedRelativePath: ".", CreatedAt: domain.Now()}
	if err = e.db.PutSourceSnapshot(ctx, source); err != nil {
		t.Fatal(err)
	}
	after, err := e.evidence.Put(ctx, id, "candidate_manifest", []byte(`{"fixture":"candidate"}`))
	if err != nil {
		t.Fatal(err)
	}
	delta, err := e.evidence.Put(ctx, id, "candidate_delta", []byte("fixture delta"))
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.Candidate{ID: domain.NewID(), AttemptID: id, SourceSnapshotID: source.ID, ManifestArtifactID: after.ID, DeltaArtifactID: delta.ID, WorkspaceRef: "workspaces/" + id, HasDelta: true, HasIndexDelta: true, Verification: "pending", Integration: "not_performed", CreatedAt: domain.Now()}
	if err = e.db.PutCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: &id, Code: "observation_fixture", Message: "Controlled unresolved observation fixture.", CreatedAt: domain.Now()}
	if err = e.db.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.RecordAttempt(ctx, id, domain.AttemptUpdate{Kind: "cleanup_unknown", Cleanup: &domain.Cleanup{State: "unknown", Scope: "not_launched", IncidentID: &incident.ID}}); err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, a domain.Attempt) {
		t.Helper()
		if a.ID != id || a.SourceSnapshot == nil || a.SourceSnapshot.ID != source.ID || a.Candidate == nil || a.Candidate.ID != candidate.ID || a.Candidate.SourceSnapshotID != source.ID || a.Candidate.Verification != "pending" || a.Candidate.Integration != "not_performed" || a.Incident == nil || a.Incident.ID != incident.ID || a.Cleanup.IncidentID == nil || *a.Cleanup.IncidentID != incident.ID {
			t.Fatalf("Incomplete exact inspection: %+v", a)
		}
	}
	check(t, observationData[domain.Attempt](t, e.Read(ctx, "attempts.get", id, nil)))
	detail := observationData[domain.TaskDetail](t, e.Read(ctx, "tasks.get", admission.Task.ID, nil))
	if detail.LatestAttempt == nil {
		t.Fatal("Task detail lost its latest attempt")
	}
	check(t, *detail.LatestAttempt)
	waited := observationData[observationWait](t, e.Read(ctx, "attempts.wait", id, map[string]string{"afterRevision": "9223372036854775807", "timeoutMs": "0"}))
	check(t, waited.Attempt)
	if waited.Changed || !waited.TimedOut {
		t.Fatal("Inspection enrichment changed future-revision wait semantics", waited)
	}
	retained, err := e.db.Attempt(ctx, id)
	if err != nil || retained.SourceSnapshot != nil || retained.Candidate != nil || retained.Incident != nil {
		t.Fatal("Read enrichment rewrote persisted attempt metadata", retained, err)
	}
}

func TestSpecificSourceBlockerAndExpiredPreparationRemainDistinct(t *testing.T) {
	e, admission, repo := stoppedObservationFixture(t)
	ctx := context.Background()
	file := filepath.Join(repo, "intent-to-add.txt")
	if err := os.WriteFile(file, []byte("ordinary uncommitted fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git := exec.Command("git", "add", "--intent-to-add", "--", "intent-to-add.txt")
	git.Dir = repo
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("Cannot prepare intent-to-add fixture: %v %s", err, output)
	}
	indexPath := filepath.Join(repo, ".git", "index")
	beforeIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.db.ClaimAttempt(ctx, admission.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, a)
	blocked, err := e.db.Attempt(ctx, a.ID)
	if err != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" || blocked.PrimaryFailure == nil || blocked.PrimaryFailure.Code != "source_capture_blocked" || !strings.Contains(blocked.PrimaryFailure.Message, "Intent-to-add") || strings.Contains(blocked.PrimaryFailure.Message, repo) || len(blocked.PrimaryFailure.Message) > 512 || !utf8.ValidString(blocked.PrimaryFailure.Message) || blocked.LaunchIntent || blocked.StartedAt != nil {
		t.Fatalf("Specific safe blocker was lost: %+v %v", blocked, err)
	}
	afterIndex, err := os.ReadFile(indexPath)
	if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatal("Blocked capture changed the original index", err)
	}
	one := 1
	r := e.db.CreateTask(ctx, e.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: admission.Task.ProjectID, OriginalText: "The captured preparation deadline is finite.", ExecutionMode: "fake", TimeoutSeconds: &one})
	next := decodeAdmission(t, r)
	a, err = e.db.ClaimAttempt(ctx, next.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, a.DeadlineAt)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(max(time.Until(deadline)+10*time.Millisecond, 0))
	e.run(ctx, a)
	expired, err := e.db.Attempt(ctx, a.ID)
	if err != nil || expired.Outcome == nil || *expired.Outcome != "failed" || expired.PrimaryFailure == nil || expired.PrimaryFailure.Code != "deadline_exceeded" || expired.LaunchIntent || expired.StartedAt != nil || expired.Cleanup.State != "verified" || expired.Cleanup.Scope != "not_launched" {
		t.Fatalf("Expired preparation was presented as an ordinary source blocker: %+v %v", expired, err)
	}
}
