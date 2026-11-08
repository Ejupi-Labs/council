package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryCaptureRejectsCrossProjectVersionAndIgnoresOpaqueClaims(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	local := openFixture(t, s, "history-local-boundary")
	foreign := openFixture(t, s, "history-foreign-boundary")
	owner := domain.NewID()
	localResult := s.CreateTask(ctx, owner, taskInput(local.ID))
	foreignResult := s.CreateTask(ctx, owner, taskInput(foreign.ID))
	if localResult.Status != 201 || foreignResult.Status != 201 {
		t.Fatal(localResult, foreignResult)
	}
	localTask := localResult.Envelope.Data.(domain.Admission)
	foreignTask := foreignResult.Envelope.Data.(domain.Admission)
	// Opaque JSON falsely claims a foreign task. Capture must use the dedicated
	// relation, never mine this source text to infer a different relationship.
	spoof := fmt.Sprintf(`{"taskId":"%s","projectId":"%s","marker":"foreign_sentinel"}`, foreignTask.Task.ID, foreign.ID)
	if _, err := s.db.ExecContext(ctx, "UPDATE attempts SET data=? WHERE id=?", spoof, localTask.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	good := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: local.ID}
	if r := s.CaptureHistory(ctx, good); r.Status != 201 {
		t.Fatal(r)
	}
	var snapshotID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", local.ID).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.HistorySnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].TaskID != localTask.Task.ID || len(snapshot.Attempts) != 1 || snapshot.Attempts[0].TaskID != localTask.Task.ID || snapshot.Attempts[0].AttemptID != localTask.Attempt.ID {
		t.Fatal(snapshot)
	}
	var raw string
	if err = s.db.QueryRowContext(ctx, "SELECT data FROM history_snapshots WHERE id=?", snapshotID).Scan(&raw); err != nil || strings.Contains(raw, "foreign_sentinel") || strings.Contains(raw, foreignTask.Task.ID) || strings.Contains(raw, foreign.ID) {
		t.Fatalf("opaque or foreign fields entered snapshot: %v", err)
	}
	// Simulate a corrupt legacy relation in this disposable fixture: only the
	// foreign task owns version 2. Its existence must not satisfy the local
	// attempt's composite (task ID, version) authority.
	if _, err = s.db.ExecContext(ctx, "INSERT INTO task_versions(task_id,version,data) VALUES(?,2,'opaque')", foreignTask.Task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE attempts SET task_version=2 WHERE id=?", localTask.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	bad := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: local.ID}
	r := s.CaptureHistory(ctx, bad)
	if r.Envelope.Error == nil {
		t.Fatal("cross-project version satisfied local relation", r)
	}
	var receipts int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM receipts WHERE request_id=?", bad.RequestID).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("malformed capture receipt=%d: %v", receipts, err)
	}
	foreignCapture := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: foreign.ID}
	if r = s.CaptureHistory(ctx, foreignCapture); r.Status != 201 {
		t.Fatal("unrelated project was not independently inspectable", r)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT data FROM history_snapshots WHERE project_id=?", foreign.ID).Scan(&raw); err != nil || strings.Contains(raw, localTask.Task.ID) || strings.Contains(raw, localTask.Attempt.ID) {
		t.Fatalf("foreign snapshot leaked local task: %v", err)
	}
}

func TestHistoryCaptureAttemptLimitIsExact(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "history-attempt-bound")
	r := s.CreateTask(ctx, domain.NewID(), taskInput(project.ID))
	if r.Status != 201 {
		t.Fatal(r)
	}
	admission := r.Envelope.Data.(domain.Admission)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO attempts(sequence,id,task_id,task_version,revision,phase,data) VALUES(?,?,?,1,1,'settled','opaque')")
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	for i := 2; i <= domain.HistoryMaxAttempts+1; i++ {
		if _, err = stmt.ExecContext(ctx, i, fmt.Sprintf("%032x", i), admission.Task.ID); err != nil {
			stmt.Close()
			tx.Rollback()
			t.Fatal(err)
		}
	}
	stmt.Close()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	over := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}
	r = s.CaptureHistory(ctx, over)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "history_too_large" {
		t.Fatalf("attempt overflow: %+v", r)
	}
	var count int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM receipts WHERE request_id=?", over.RequestID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("overflow receipt=%d: %v", count, err)
	}
	if _, err = s.db.ExecContext(ctx, "DELETE FROM attempts WHERE sequence=?", domain.HistoryMaxAttempts+1); err != nil {
		t.Fatal(err)
	}
	ok := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}
	r = s.CaptureHistory(ctx, ok)
	if r.Status != 201 {
		t.Fatalf("exact attempt limit rejected: %+v", r)
	}
	var id string
	if err = s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", project.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.HistorySnapshot(ctx, id)
	if err != nil || len(snapshot.Attempts) != domain.HistoryMaxAttempts || snapshot.Summary.AttemptCount != fmt.Sprint(domain.HistoryMaxAttempts) {
		t.Fatalf("bounded snapshot: count=%d err=%v", len(snapshot.Attempts), err)
	}
}
