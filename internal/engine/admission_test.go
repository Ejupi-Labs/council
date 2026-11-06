package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func evidenceMetadata(record domain.ArtifactRecord) evidence.Metadata {
	return evidence.Metadata{ID: record.ID, AttemptID: record.AttemptID, Kind: record.Kind, ByteLength: record.ByteLength, Closed: record.Closed, Completeness: record.Completeness, SHA256: record.SHA256, CreatedAt: record.CreatedAt, RelativePath: record.PrivateRef, Identity: record.PhysicalIdentity}
}
func TestStorageValidOversizedPromptIsRetainedAndBlockedBeforeWorker(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	text := "\ufeff" + strings.Repeat("x", domain.MaxTextBytes-3)
	if err := domain.ValidateText(text); err != nil {
		t.Fatal(err)
	}
	r := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: text, ExecutionMode: "fake", FakeScenario: "candidate"})
	if r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatal("Storage-valid input was rejected instead of retained", r)
	}
	admission := decodeAdmission(t, r)
	a := settled(t, e, admission.Attempt.ID)
	if a.Outcome == nil || *a.Outcome != "blocked" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "input_too_large" || a.LaunchIntent || a.StartedAt != nil || a.SourceSnapshotID != nil || a.Cleanup.State != "verified" || a.Cleanup.Scope != "not_launched" {
		t.Fatalf("Oversized prompt was dispatched or falsely completed: %+v", a)
	}
	detail, err := e.db.Task(ctx, a.TaskID)
	if err != nil || detail.Task.OriginalText != text || detail.Task.CurrentVersion != "1" {
		t.Fatal("Full original was not retained", err)
	}
	artifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil || len(artifacts.Items) != 0 {
		t.Fatal("Unexpected worker/output evidence", artifacts, err)
	}
	events, err := e.db.Events(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events.Items {
		if event.Kind == "launch_intent" || event.Kind == "worker_started" {
			t.Fatal("Oversized prompt invoked a worker")
		}
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.ActiveAttemptID != nil || gate.State != "open" {
		t.Fatal("Known blocked input retained unsafe ownership", gate, err)
	}
}
func TestExactDraftAdmissionReplayRetainsPolicyAfterGlobalAndDraftChanges(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	created := e.db.CreateDraft(ctx, domain.NewID(), "immutable source draft", project, "\ufeff Original draft\r\n exact text ")
	if created.Envelope.Error != nil {
		t.Fatal(created.Envelope.Error)
	}
	draft := created.Envelope.Data.(domain.Draft)
	policy, err := e.db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed := e.db.SetPolicy(ctx, domain.NewID(), "narrower-global", policy.Revision, "supervised")
	if changed.Envelope.Error != nil {
		t.Fatal(changed.Envelope.Error)
	}
	in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: draft.OriginalText, SourceDraftRef: &domain.DraftRef{DraftID: draft.ID, Revision: draft.Revision}, ExecutionMode: "fake"}
	first := e.StartTask(ctx, in)
	if first.Status != 201 {
		t.Fatal(first)
	}
	admission := decodeAdmission(t, first)
	if admission.Version.RequestedPolicy != draft.RequestedPolicy {
		t.Fatal("Start adopted a changed global preference", admission.Version, draft.RequestedPolicy)
	}
	policy, err = e.db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed = e.db.SetPolicy(ctx, domain.NewID(), "broader-global", policy.Revision, "autonomous")
	if changed.Envelope.Error != nil {
		t.Fatal(changed.Envelope.Error)
	}
	updated := e.db.UpdateDraft(ctx, domain.NewID(), "later draft version", draft.ID, draft.Revision, "Later draft text", true)
	if updated.Envelope.Error != nil {
		t.Fatal(updated.Envelope.Error)
	}
	replayed := e.StartTask(ctx, in)
	if replayed.Status != first.Status || replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *first.Envelope.Receipt {
		t.Fatal("Admission replay changed receipt", replayed)
	}
	encoded, err := json.Marshal(replayed.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	var original domain.Admission
	if err = json.Unmarshal(encoded, &original); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, admission) {
		t.Fatal("Lost acknowledgement replay refreshed input/policy/observation")
	}
	attempts, err := e.db.Attempts(ctx, admission.Task.ID, 100, "")
	if err != nil || len(attempts.Items) != 1 {
		t.Fatal("Duplicate execution admitted", attempts, err)
	}
	detail, err := e.db.Task(ctx, admission.Task.ID)
	if err != nil || detail.Task.OriginalText != draft.OriginalText || detail.Task.SourceDraftRef == nil || detail.Task.SourceDraftRef.Revision != "1" || detail.Version.RequestedPolicy != draft.RequestedPolicy {
		t.Fatal(detail, err)
	}
}
