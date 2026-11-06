package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"testing"
)

func taskInput(p string) domain.StartTaskInput {
	return domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: p, OriginalText: "\ufeff original\r\n exact", ExecutionMode: "fake"}
}
func TestTaskAdmissionSnapshotReplayAndGuidance(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "task")
	in := taskInput(p.ID)
	owner := domain.NewID()
	r := s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	s.SetPolicy(ctx, domain.NewID(), "newpolicy", "1", "supervised")
	r = s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	detail, e := s.Task(ctx, a.Task.ID)
	if e != nil || detail.Task.OriginalText != in.OriginalText || detail.Version.RequestedPolicy.Mode != "autonomous" {
		t.Fatal(detail, e)
	}
	g := domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", GuidanceText: "additional"}
	r = s.UpdateGuidance(ctx, a.Task.ID, g)
	if r.Status != 200 {
		t.Fatal(r)
	}
	old, e := s.TaskVersion(ctx, a.Task.ID, "1")
	if e != nil || old.Version.GuidanceText != "" {
		t.Fatal(old, e)
	}
	attempt, e := s.ClaimAttempt(ctx, a.Attempt.ID, owner)
	if e != nil || attempt.Phase != "settled" || attempt.Outcome == nil || *attempt.Outcome != "blocked" || attempt.PrimaryFailure.Code != "guidance_changed" {
		t.Fatal(attempt, e)
	}
}
func TestExactDraftPolicyAndConflictNoAdmission(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "draft-task")
	d := s.CreateDraft(ctx, domain.NewID(), "draft", p.ID, "original").Envelope.Data.(domain.Draft)
	s.SetPolicy(ctx, domain.NewID(), "supervised", "1", "supervised")
	in := taskInput(p.ID)
	in.OriginalText = d.OriginalText
	in.SourceDraftRef = &domain.DraftRef{DraftID: d.ID, Revision: "1"}
	r := s.CreateTask(ctx, domain.NewID(), in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	if r.Envelope.Data.(domain.Admission).Version.RequestedPolicy.Mode != "autonomous" {
		t.Fatal(r)
	}
	in.RequestID = domain.NewID()
	in.OriginalText = "changed"
	r = s.CreateTask(ctx, domain.NewID(), in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "source_draft_conflict" {
		t.Fatal(r)
	}
}
