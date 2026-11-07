package engine

import (
	"context"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRestartInterruptsRetainedQueueWithoutAutomaticDispatch(t *testing.T) {
	ctx := context.Background()
	old, project, _ := testEngine(t)
	stopFixtureScheduler(t, old)
	admission := decodeAdmission(t, old.db.CreateTask(ctx, old.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "An old queued task is not retry permission.", ExecutionMode: "fake"}))
	if err := old.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err := old.evidence.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(old.db, old.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	a, err := next.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || a.Outcome == nil || *a.Outcome != "interrupted" || a.Cleanup.State != "verified" || a.Cleanup.Scope != "not_launched" || a.LaunchIntent {
		t.Fatal(a, err)
	}
	revision := a.Revision
	// This fixture intentionally abandons the old engine without a clean
	// epoch close. The queued attempt is known not to have launched, but that
	// alone does not establish that the lost owner had no other effects.
	gate, err := next.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || gate.ActiveAttemptID != nil {
		t.Fatal("lost owner did not retain its durable uncertainty fence", gate, err)
	}
	incident, err := next.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.Code != "owner_epoch_uncertain" {
		t.Fatal("lost owner epoch was not visible", incident, err)
	}
	next.signal()
	fresh := next.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Explicit work must respect the unknown owner.", ExecutionMode: "fake", FakeScenario: "response"})
	if fresh.Envelope.Error == nil || fresh.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal("new admission bypassed the uncertain owner", fresh)
	}
	a, err = next.db.Attempt(ctx, a.ID)
	if err != nil || a.Revision != revision || a.StartedAt != nil || a.LaunchIntent || a.Outcome == nil || *a.Outcome != "interrupted" {
		t.Fatal("refused work replayed the old queue", a, err)
	}
	history, err := next.db.Attempts(ctx, a.TaskID, 100, "")
	if err != nil || len(history.Items) != 1 || history.Items[0].ID != a.ID {
		t.Fatal("restart or refused admission added old-task work", history, err)
	}
	events, err := next.db.Events(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events.Items {
		if event.Kind == "launch_intent" || event.Kind == "worker_started" {
			t.Fatal("Old queue dispatched after restart")
		}
	}
}
func TestRestartLaunchIntentRetainsEvidenceAndFencesWithoutReplay(t *testing.T) {
	ctx := context.Background()
	old, project, _ := testEngine(t)
	stopFixtureScheduler(t, old)
	admission := decodeAdmission(t, old.db.CreateTask(ctx, old.config.OwnerInstanceID, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Launch intent can represent executed ownership.", ExecutionMode: "fake"}))
	if _, err := old.db.ClaimAttempt(ctx, admission.Attempt.ID, old.config.OwnerInstanceID); err != nil {
		t.Fatal(err)
	}
	manifest, err := old.evidence.Put(ctx, admission.Attempt.ID, "source_manifest", []byte("retained manifest fixture"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := old.evidence.Put(ctx, admission.Attempt.ID, "index_snapshot", []byte("retained index fixture"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: admission.Attempt.ID, ManifestArtifactID: manifest.ID, IndexArtifactID: index.ID, ManifestSHA256: *manifest.SHA256, CreatedAt: domain.Now()}
	if err = old.db.PutSourceSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	before, err := old.db.RecordLaunchIntent(ctx, admission.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, err := old.db.Events(ctx, before.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = old.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err = old.evidence.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(old.db, old.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	after, err := next.db.Attempt(ctx, before.ID)
	if err != nil || after.Outcome == nil || *after.Outcome != "interrupted" || after.Cleanup.State != "unknown" || after.Cleanup.Scope != before.Cleanup.Scope || after.SourceSnapshotID == nil || *after.SourceSnapshotID != snapshot.ID || !after.LaunchIntent {
		t.Fatal(after, err)
	}
	gate, err := next.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != after.ID {
		t.Fatal(gate, err)
	}
	if _, err = next.evidence.VerifyClosed(ctx, manifest.ID); err != nil {
		t.Fatal("Restart lost retained bytes", err)
	}
	blocked := next.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "No new work while ownership is unknown.", ExecutionMode: "fake"})
	if blocked.Envelope.Error == nil || blocked.Envelope.Error.Code != "dispatch_fenced" {
		t.Fatal(blocked)
	}
	next.signal()
	time.Sleep(150 * time.Millisecond)
	eventsAfter, err := next.db.Events(ctx, after.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	for _, event := range eventsAfter.Items {
		if event.Kind == "launch_intent" {
			launches++
		}
		if event.Kind == "worker_started" {
			t.Fatal("Uncertain launch intent was automatically replayed")
		}
	}
	if launches != 1 || len(eventsAfter.Items) <= len(eventsBefore.Items) {
		t.Fatal("Recovery lineage did not retain exactly one launch intent", eventsAfter)
	}
	attempts, err := next.db.Attempts(ctx, after.TaskID, 100, "")
	if err != nil || len(attempts.Items) != 1 {
		t.Fatal(attempts, err)
	}
}
