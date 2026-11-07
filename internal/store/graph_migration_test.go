package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphMigrationPreservesSchema5HistoryAndHeldClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accepted5.sqlite3")
	db, ids := graphSchema5Database(t, path, true)
	before := graphSchema5Rows(t, db)
	artifactPath := filepath.Join(filepath.Dir(path), "artifacts", strings.Repeat("0a", 16)+".bin")
	artifactBefore, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 6 {
		t.Fatalf("known schema5 was not upgraded transactionally: version=%d err=%v", version, err)
	}
	if after := graphSchema5Rows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("migration rewrote accepted005 JSON/receipts/budgets/workflow/defaults")
	}
	var slot int64
	var attempt, owner, contextID string
	var barrier int
	if err = s.db.QueryRow("SELECT slot,attempt_id,owner_instance_id,context_id,legacy_barrier FROM capacity_claims").Scan(&slot, &attempt, &owner, &contextID, &barrier); err != nil || slot != 1 || attempt != ids.ActiveAttemptID || owner != strings.Repeat("6", 32) || contextID != "codex-native" || barrier != 1 {
		t.Fatal("held singleton ownership/barrier lost", slot, attempt, owner, contextID, barrier, err)
	}
	for _, table := range []string{"task_graphs", "graph_versions", "graph_nodes", "graph_inputs", "graph_dispatches", "graph_admission_observations", "graph_delivery_bindings"} {
		var n int
		if err = s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("migration invented graph authority/work", table, n, err)
		}
	}
	artifactAfter, err := os.ReadFile(artifactPath)
	if err != nil || !reflect.DeepEqual(artifactBefore, artifactAfter) {
		t.Fatal("migration changed retained artifact bytes", err)
	}
	if replay := s.CreateTask(context.Background(), owner, ids.Input); replay.Status != 201 || replay.Envelope.Receipt == nil || replay.Envelope.Receipt.RequestID != ids.RequestID {
		t.Fatal("old exact admission replay became new work", replay)
	}
	if !reflect.DeepEqual(before, graphSchema5Rows(t, s.db)) {
		t.Fatal("receipt replay rewrote historical state")
	}
	assertDurableJournal(t, s)
}

func TestGraphMigrationFaultBoundariesPreserveHistory(t *testing.T) {
	for _, boundary := range []string{"before_commit", "after_commit", "cancelled"} {
		t.Run(boundary, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "accepted5.sqlite3")
			db, _ := graphSchema5Database(t, path, true)
			s := &Store{db: db}
			before := graphSchema5Rows(t, db)
			ctx := context.Background()
			fault := func() error { return errors.New("fixed graph migration persistence fault") }
			if boundary == "before_commit" {
				s.BeforeCommit = fault
			}
			if boundary == "after_commit" {
				s.AfterCommit = fault
			}
			if boundary == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := s.migrateGraphs(ctx)
			if err == nil {
				t.Fatal("migration acknowledgement/cancellation fault disappeared")
			}
			want := 5
			if boundary == "after_commit" {
				want = 6
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "write_uncertain" {
					t.Fatal("actual commit acknowledgement lost uncertainty", err)
				}
			}
			if boundary == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("known no-commit cancellation became storage uncertainty", err)
			}
			var version int
			if e := db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != want {
				t.Fatal("migration marker escaped transaction boundary", version, want, e)
			}
			if !reflect.DeepEqual(before, graphSchema5Rows(t, db)) {
				t.Fatal("migration boundary changed accepted rows")
			}
		})
	}
}

func TestGraphMigrationRefusesUnknownDDLWithoutRepair(t *testing.T) {
	for _, change := range []string{"CREATE TABLE task_graphs(sentinel TEXT)", "CREATE INDEX foreign_graph_index ON tasks(project_id)", "PRAGMA user_version=6"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unknown.sqlite3")
			db, _ := graphSchema5Database(t, path, true)
			if _, err := db.Exec(change); err != nil {
				t.Fatal(err)
			}
			before := graphSchema5Rows(t, db)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			bytesBefore, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := Open(path); err == nil {
				_ = s.Close()
				t.Fatal("unknown/colliding/incomplete graph schema accepted")
			}
			bytesAfter, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(bytesBefore, bytesAfter) {
				t.Fatal("refused database bytes changed", err)
			}
			dsn, err := sqliteDSN(path)
			if err != nil {
				t.Fatal(err)
			}
			check, err := sql.Open("sqlite", dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = check.Close() })
			if !reflect.DeepEqual(before, graphSchema5Rows(t, check)) {
				t.Fatal("unknown schema repair rewrote history")
			}
		})
	}
}
