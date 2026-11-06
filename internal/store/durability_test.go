package store

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func assertDurableJournal(t *testing.T, s *Store) {
	t.Helper()
	var mode string
	var synchronous int
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatal("WAL was not established", mode, err)
	}
	if err := s.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil || synchronous != 2 {
		t.Fatal("FULL durability was not established", synchronous, err)
	}
}

func TestWALRefusesAStoreThatCannotEstablishWAL(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err = configureDurableJournal(context.Background(), db); err == nil {
		t.Fatal("An unsupported journal backend silently fell back")
	}
	var mode string
	if err = db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "memory" {
		t.Fatal(mode, err)
	}
}
func TestWALFullEstablishedOnFreshAndReopenedStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertDurableJournal(t, s)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertDurableJournal(t, s)
}
func TestWALConfigurationFollowsSchemaValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsupported.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE sentinel(value TEXT);INSERT INTO sentinel VALUES('keep');PRAGMA application_id=1129202226;PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open(path); err == nil {
		t.Fatal("Unsupported schema was opened")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("Unsupported database was changed before rejection", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err = os.Lstat(path + suffix); !os.IsNotExist(err) {
			t.Fatal("Unsupported schema acquired WAL companions", suffix, err)
		}
	}
}
func TestWALMigrationPreservesFoundationReceiptBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foundation.db")
	db := foundationDatabase(t, path)
	id := domain.NewID()
	receipt := &domain.Receipt{RequestID: id, Operation: "policy.set", CommittedAt: domain.Now()}
	original := domain.Success(200, id, domain.Policy{Mode: "supervised", SelectionSource: "user", Revision: "9007199254740993", UpdatedAt: domain.Now()}, receipt)
	raw := encode(original.Envelope)
	if _, err := db.Exec("INSERT INTO receipts VALUES(?,?,?,?,?)", id, "policy.set", "foundation-intent", 200, raw); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertDurableJournal(t, s)
	var retained string
	if err = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", id).Scan(&retained); err != nil || retained != raw {
		t.Fatal("Foundation receipt changed during migration/WAL activation", err)
	}
	replayed, err := s.Replay(context.Background(), id, "policy.set", "foundation-intent")
	if err != nil || replayed == nil || replayed.Status != 200 || replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *receipt {
		t.Fatal(replayed, err)
	}
	if replayed.Envelope.Data.(map[string]any)["revision"] != "9007199254740993" {
		t.Fatal("Large exact foundation revision changed")
	}
}

func TestWALCrashChild(t *testing.T) {
	if os.Getenv("COUNCIL_WAL_CRASH_CHILD") != "1" {
		t.Skip("private subprocess fixture")
	}
	root := os.Getenv("COUNCIL_WAL_CRASH_ROOT")
	request := os.Getenv("COUNCIL_WAL_CRASH_REQUEST")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(lock.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	assertDurableJournal(t, s)
	r := s.SetPolicy(context.Background(), request, "committed-wal-intent", "1", "supervised")
	if r.Status != 200 || r.Envelope.Receipt == nil {
		t.Fatal(r)
	}
	fmt.Println(encode(r.Envelope))
	os.Exit(0)
}
func TestCommittedWALReceiptSurvivesAbruptProcessExit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	request := domain.NewID()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestWALCrashChild$")
	cmd.Env = append(os.Environ(), "COUNCIL_WAL_CRASH_CHILD=1", "COUNCIL_WAL_CRASH_ROOT="+root, "COUNCIL_WAL_CRASH_REQUEST="+request)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Private WAL child failed: %v %s", err, output)
	}
	original := strings.TrimSpace(string(output))
	walPath := filepath.Join(root, "workspace.sqlite3-wal")
	beforeGuard, err := os.ReadFile(walPath)
	if err != nil || len(beforeGuard) == 0 {
		t.Fatal("Child did not leave an uncheckpointed WAL", err)
	}
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	afterGuard, err := os.ReadFile(walPath)
	if err != nil || !bytes.Equal(beforeGuard, afterGuard) {
		t.Fatal("Owner privacy preparation changed the retained WAL", err)
	}
	s, err := Open(lock.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertDurableJournal(t, s)
	policy, err := s.Policy(context.Background())
	if err != nil || policy.Mode != "supervised" || policy.Revision != "2" {
		t.Fatal("Committed policy was lost", policy, err)
	}
	var retained string
	if err = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", request).Scan(&retained); err != nil || retained != original {
		t.Fatal("Committed receipt bytes were not recovered exactly", err)
	}
	r := s.SetPolicy(context.Background(), request, "committed-wal-intent", "1", "supervised")
	if r.Status != 200 || r.Envelope.Receipt == nil || r.Envelope.RequestID != request {
		t.Fatal("Lost acknowledgement replay became a stale overwrite", r)
	}
	var integrity string
	if err = s.db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal(integrity, err)
	}
}
