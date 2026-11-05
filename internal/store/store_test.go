package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("SQLite %s", s.SQLiteVersion)
	t.Cleanup(func() { s.Close() })
	return s
}
func TestPolicyReplayConflictAndRollback(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	id := domain.NewID()
	first := s.SetPolicy(ctx, id, "fingerprint", "1", "supervised")
	if first.Status != 200 {
		t.Fatal(first)
	}
	replay := s.SetPolicy(ctx, id, "fingerprint", "1", "supervised")
	if replay.Envelope.Receipt.CommittedAt != first.Envelope.Receipt.CommittedAt {
		t.Fatal("replay changed receipt")
	}
	if r := s.SetPolicy(ctx, id, "changed", "2", "autonomous"); r.Envelope.Error.Code != "request_conflict" {
		t.Fatal(r)
	}
	if r := s.SetPolicy(ctx, domain.NewID(), "stale", "1", "autonomous"); r.Envelope.Error.Code != "revision_conflict" {
		t.Fatal(r)
	}
	s.BeforeCommit = func() error { return errors.New("deterministic failure") }
	if r := s.SetPolicy(ctx, domain.NewID(), "rollback", "2", "autonomous"); r.Envelope.Error.Code != "storage_failed" {
		t.Fatal(r)
	}
	s.BeforeCommit = nil
	p, e := s.Policy(ctx)
	if e != nil || p.Revision != "2" || p.Mode != "supervised" {
		t.Fatal(p, e)
	}
}
func TestUncertainCommitReconcilesByIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	id := domain.NewID()
	s.AfterCommit = func() error { return errors.New("lost acknowledgement") }
	r := s.SetPolicy(ctx, id, "input", "1", "supervised")
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
	s.AfterCommit = nil
	r = s.SetPolicy(ctx, id, "input", "1", "supervised")
	if r.Status != 200 || r.Envelope.Receipt == nil {
		t.Fatal(r)
	}
}

func TestSchemaMismatchAndMalformedDatabaseAreNotReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("CREATE TABLE sentinel(value TEXT); INSERT INTO sentinel VALUES('retained'); PRAGMA user_version=99"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	if _, e = Open(path); e == nil {
		t.Fatal("opened unknown schema")
	}
	db, e = sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var value string
	if e = db.QueryRow("SELECT value FROM sentinel").Scan(&value); e != nil || value != "retained" {
		t.Fatal("schema was replaced", value, e)
	}
	malformed := filepath.Join(t.TempDir(), "malformed.db")
	original := []byte("not a database: preserve this file")
	if e = os.WriteFile(malformed, original, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(malformed); e == nil {
		t.Fatal("opened malformed database")
	}
	preserved, e := os.ReadFile(malformed)
	if e != nil || !bytes.Equal(preserved, original) {
		t.Fatal("malformed file changed", e)
	}
}
func TestRestartRetainsDraftAndOriginalReceipt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	project := openFixture(t, s, "restart")
	id := domain.NewID()
	first := s.CreateDraft(ctx, id, "restart-input", project.ID, "exact\r\n\ufeff\ufffd text")
	if first.Status != 201 {
		t.Fatal(first)
	}
	original := first.Envelope.Data.(domain.Draft)
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	loaded, e := s.Draft(ctx, original.ID, "")
	if e != nil || loaded.OriginalText != original.OriginalText || loaded.RequestedPolicy != original.RequestedPolicy {
		t.Fatal(loaded, e)
	}
	repeated := s.CreateDraft(ctx, id, "restart-input", project.ID, original.OriginalText)
	if repeated.Status != first.Status || repeated.Envelope.Receipt.CommittedAt != first.Envelope.Receipt.CommittedAt {
		t.Fatal(repeated)
	}
	var count int
	if e = s.db.QueryRow("SELECT count(*) FROM drafts").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
func TestDraftRollbackDoesNotAdvancePointerOrCreateReceipt(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "rollback")
	r := s.CreateDraft(ctx, domain.NewID(), "create", project.ID, "original")
	if r.Status != 201 {
		t.Fatal(r)
	}
	original := r.Envelope.Data.(domain.Draft)
	id := domain.NewID()
	s.BeforeCommit = func() error { return errors.New("simulated storage refusal") }
	r = s.UpdateDraft(ctx, id, "update", original.ID, "1", "new", true)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "storage_failed" {
		t.Fatal(r)
	}
	s.BeforeCommit = nil
	loaded, e := s.Draft(ctx, original.ID, "")
	if e != nil || loaded.Revision != "1" || loaded.OriginalText != "original" {
		t.Fatal(loaded, e)
	}
	if _, e = s.Draft(ctx, original.ID, "2"); e == nil {
		t.Fatal("rolled-back version persisted")
	}
	previous, e := s.Replay(ctx, id, "drafts.update", "update")
	if e != nil || previous != nil {
		t.Fatal("rolled-back receipt persisted", previous, e)
	}
	r = s.UpdateDraft(ctx, id, "update", original.ID, "1", "new", true)
	if r.Status != 200 {
		t.Fatal(r)
	}
}
