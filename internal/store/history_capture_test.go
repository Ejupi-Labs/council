package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryCaptureScopesImmutableSnapshotAndReplay(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	first := openFixture(t, s, "history-first")
	second := openFixture(t, s, "history-second")
	owner := domain.NewID()
	create := func(project string) domain.Admission {
		r := s.CreateTask(ctx, owner, taskInput(project))
		if r.Status != 201 {
			t.Fatalf("create task: %+v", r)
		}
		return r.Envelope.Data.(domain.Admission)
	}
	a, b, foreign := create(first.ID), create(first.ID), create(second.ID)
	// Invalid opaque records prove the projection does not decode the original
	// record JSON. This is not instrumentation of physical SQLite page reads.
	const sentinel = "excluded_sentinel_not_json"
	if _, err := s.db.ExecContext(ctx, "UPDATE tasks SET data=? WHERE id=?", sentinel, a.Task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE attempts SET data=? WHERE id=?", sentinel, a.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE tasks SET revision=? WHERE id=?", int64(9007199254740993), a.Task.ID); err != nil {
		t.Fatal(err)
	}
	in := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: first.ID}
	r := s.CaptureHistory(ctx, in)
	if r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatalf("capture: %+v", r)
	}
	var id string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", first.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.HistorySnapshot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Summary.Completeness != domain.HistoryCompleteness || snapshot.Summary.Notice != domain.HistoryNotice {
		t.Fatal("snapshot is missing its incomplete-history notice")
	}
	if len(snapshot.Tasks) != 2 || len(snapshot.Attempts) != 2 || snapshot.Tasks[0].TaskID != a.Task.ID || snapshot.Tasks[1].TaskID != b.Task.ID || snapshot.Tasks[0].Revision != "9007199254740993" || snapshot.Attempts[0].TaskID != a.Task.ID {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || strings.Contains(string(encoded), sentinel) || strings.Contains(string(encoded), foreign.Task.ID) || strings.Contains(string(encoded), foreign.Attempt.ID) {
		t.Fatalf("snapshot leaked excluded data: %v", err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE tasks SET revision=revision+1 WHERE id=?", a.Task.ID); err != nil {
		t.Fatal(err)
	}
	old, err := s.HistorySnapshot(ctx, id)
	if err != nil || old.Tasks[0].Revision != "9007199254740993" {
		t.Fatalf("snapshot changed after source update: %+v %v", old, err)
	}
	replay := s.CaptureHistory(ctx, in)
	if replay.Status != r.Status || replay.Envelope.Receipt == nil || replay.Envelope.Receipt.CommittedAt != r.Envelope.Receipt.CommittedAt {
		t.Fatalf("replay: %+v", replay)
	}
	changed := in
	changed.ProjectID = second.ID
	conflict := s.CaptureHistory(ctx, changed)
	if conflict.Envelope.Error == nil || conflict.Envelope.Error.Code != "request_conflict" {
		t.Fatalf("changed replay: %+v", conflict)
	}
	var count int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM history_snapshots").Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot count %d: %v", count, err)
	}
}

func TestHistoryCaptureEmptyLostReceiptAndInvalidRelation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "history-empty")
	in := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}
	s.AfterCommit = func() error { return errors.New("lost acknowledgement") }
	r := s.CaptureHistory(ctx, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" {
		t.Fatalf("expected uncertain receipt: %+v", r)
	}
	s.AfterCommit = nil
	r = s.CaptureHistory(ctx, in)
	if r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatalf("replay: %+v", r)
	}
	var id string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", project.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.HistorySnapshot(ctx, id)
	if err != nil || len(snapshot.Tasks) != 0 || len(snapshot.Attempts) != 0 || snapshot.Summary.TaskCount != "0" {
		t.Fatalf("empty: %+v %v", snapshot, err)
	}
	missing := s.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: domain.NewID()})
	if missing.Envelope.Error == nil || missing.Envelope.Error.Code != "not_found" {
		t.Fatalf("unknown project: %+v", missing)
	}
	a := s.CreateTask(ctx, domain.NewID(), taskInput(project.ID))
	if a.Status != 201 {
		t.Fatal(a)
	}
	task := a.Envelope.Data.(domain.Admission).Task
	if _, err = s.db.ExecContext(ctx, "UPDATE tasks SET current_version=2 WHERE id=?", task.ID); err != nil {
		t.Fatal(err)
	}
	r = s.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID})
	if r.Envelope.Error == nil {
		t.Fatal("missing task version accepted")
	}
}

func TestHistorySnapshotPagingAuthenticatesCursor(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "history-pages")
	owner := domain.NewID()
	for i := 0; i < 3; i++ {
		if r := s.CreateTask(ctx, owner, taskInput(project.ID)); r.Status != 201 {
			t.Fatal(r)
		}
	}
	if r := s.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}); r.Status != 201 {
		t.Fatal(r)
	}
	var id string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", project.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	page, err := s.HistoryRecords(ctx, id, domain.HistoryRecordsQuery{Kind: "tasks", Limit: "1"})
	if err != nil || page.NextCursor == nil || len(page.Items.([]domain.HistoryTaskRow)) != 1 {
		t.Fatalf("first page: %+v %v", page, err)
	}
	if page.Completeness != domain.HistoryCompleteness || page.Notice != domain.HistoryNotice {
		t.Fatal("records page is missing its incomplete-history notice")
	}
	cursor := *page.NextCursor
	next, err := s.HistoryRecords(ctx, id, domain.HistoryRecordsQuery{Kind: "tasks", Limit: "1", Cursor: cursor})
	if err != nil || next.NextCursor == nil || next.Items.([]domain.HistoryTaskRow)[0].Ordinal != "2" {
		t.Fatalf("next page: %+v %v", next, err)
	}
	if next.Completeness != domain.HistoryCompleteness || next.Notice != domain.HistoryNotice {
		t.Fatal("next records page is missing its incomplete-history notice")
	}
	if _, err = s.HistoryRecords(ctx, id, domain.HistoryRecordsQuery{Kind: "attempts", Limit: "1", Cursor: cursor}); err == nil {
		t.Fatal("cross-section cursor accepted")
	}
	if r := s.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}); r.Status != 201 {
		t.Fatal(r)
	}
	var anotherID string
	if err = s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=? AND id<>?", project.ID, id).Scan(&anotherID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.HistoryRecords(ctx, anotherID, domain.HistoryRecordsQuery{Kind: "tasks", Limit: "1", Cursor: cursor}); err == nil {
		t.Fatal("cross-snapshot cursor accepted")
	}
	last := "A"
	if strings.HasSuffix(cursor, "A") {
		last = "B"
	}
	tampered := cursor[:len(cursor)-1] + last
	if _, err = s.HistoryRecords(ctx, id, domain.HistoryRecordsQuery{Kind: "tasks", Limit: "1", Cursor: tampered}); err == nil {
		t.Fatal("tampered cursor accepted")
	}
}

func TestHistoryCaptureRejectsOverflowWithoutReceipt(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "history-bounded")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= domain.HistoryMaxTasks+1; i++ {
		id := fmt.Sprintf("%032x", i)
		if _, err = tx.ExecContext(ctx, "INSERT INTO tasks(sequence,id,project_id,revision,state_revision,current_version,data) VALUES(?,?,?,1,1,1,'opaque')", i, id, project.ID); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO task_versions(task_id,version,data) VALUES(?,1,'opaque')", id); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	in := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}
	r := s.CaptureHistory(ctx, in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "history_too_large" {
		t.Fatalf("overflow not rejected: %+v", r)
	}
	var count int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM history_snapshots WHERE project_id=?", project.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("overflow snapshot count %d: %v", count, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM receipts WHERE request_id=?", in.RequestID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("overflow receipt count %d: %v", count, err)
	}
}

func TestHistorySnapshotPagingSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	project := openFixture(t, s, "history-reopen")
	owner := domain.NewID()
	for i := 0; i < 2; i++ {
		if r := s.CreateTask(ctx, owner, taskInput(project.ID)); r.Status != 201 {
			s.Close()
			t.Fatal(r)
		}
	}
	if r := s.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}); r.Status != 201 {
		s.Close()
		t.Fatal(r)
	}
	var id string
	if err = s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", project.ID).Scan(&id); err != nil {
		s.Close()
		t.Fatal(err)
	}
	page, err := s.HistoryRecords(ctx, id, domain.HistoryRecordsQuery{Kind: "tasks", Limit: "1"})
	if err != nil || page.NextCursor == nil {
		s.Close()
		t.Fatalf("page: %+v %v", page, err)
	}
	cursor := *page.NextCursor
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err = s.HistoryRecords(ctx, id, domain.HistoryRecordsQuery{Kind: "tasks", Limit: "1", Cursor: cursor})
	if err != nil || page.NextCursor != nil || page.Items.([]domain.HistoryTaskRow)[0].Ordinal != "2" {
		t.Fatalf("reopened page: %+v %v", page, err)
	}
}

func TestHistoryCaptureSerializesConcurrentStoreMutation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "history-concurrent")
	owner := domain.NewID()
	if r := s.CreateTask(ctx, owner, taskInput(project.ID)); r.Status != 201 {
		t.Fatal(r)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	s.BeforeCommit = func() error {
		s.BeforeCommit = nil
		close(entered)
		<-release
		return nil
	}
	captured := make(chan domain.Result, 1)
	go func() {
		captured <- s.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID})
	}()
	<-entered
	changed := make(chan domain.Result, 1)
	go func() { changed <- s.CreateTask(ctx, owner, taskInput(project.ID)) }()
	close(release)
	if r := <-captured; r.Status != 201 {
		t.Fatal(r)
	}
	if r := <-changed; r.Status != 201 {
		t.Fatal(r)
	}
	var id string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM history_snapshots WHERE project_id=?", project.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.HistorySnapshot(ctx, id)
	if err != nil || len(snapshot.Tasks) != 1 || len(snapshot.Attempts) != 1 {
		t.Fatalf("mixed snapshot: %+v %v", snapshot, err)
	}
}
