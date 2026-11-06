package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestLegacyOptionLikeSelectorReceiptReplaysBeforeNewLaunchGuard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	project := openFixture(t, s, "legacy-selector")
	legacy := taskInput(project.ID)
	legacy.Pins = &domain.ExecutionPins{HarnessID: "codex", Model: "--legacy-option-like-selector"}
	// Accepted002 structural validation allowed this value. Retain its original
	// acknowledged envelope without authorizing a new native invocation under003.
	owner := domain.NewID()
	task := domain.Task{ID: domain.NewID(), ProjectID: project.ID, OriginalText: legacy.OriginalText, CurrentVersion: "1", Revision: "1", StateRevision: "1"}
	version := resolvedVersion(task.ID, domain.Policy{Mode: "autonomous", Revision: "1"}, legacy)
	attempt := newAttempt(task, version, legacy.RequestID, owner, "fake", "")
	receipt := &domain.Receipt{RequestID: legacy.RequestID, Operation: "tasks.start", CommittedAt: domain.Now()}
	envelope := domain.Success(201, legacy.RequestID, domain.Admission{Task: taskSummary(task, version, &attempt, false), Version: version, Attempt: attempt}, receipt).Envelope
	raw := encode(envelope)
	if _, err := s.db.Exec("INSERT INTO receipts VALUES(?,?,?,?,?)", legacy.RequestID, "tasks.start", TaskStartFingerprint(legacy), 201, raw); err != nil {
		t.Fatal(err)
	}
	if r := s.CreateTask(ctx, owner, legacy); r.Status != 201 || r.Envelope.Error != nil || r.Envelope.Receipt == nil || *r.Envelope.Receipt != *receipt {
		t.Fatal("legacy receipt rejected by new guard", r)
	}
	var retained string
	if err := s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", legacy.RequestID).Scan(&retained); err != nil || retained != raw {
		t.Fatal("legacy envelope changed", err)
	}
	legacy.RequestID = domain.NewID()
	if r := s.CreateTask(ctx, owner, legacy); r.Envelope.Error == nil || r.Envelope.Error.Code != "pin_unavailable" {
		t.Fatal("new unsafe launch was not blocked", r)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM tasks").Scan(&count); err != nil || count != 0 {
		t.Fatal("receipt replay or blocked start admitted new work", count, err)
	}
}
