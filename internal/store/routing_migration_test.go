package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoutingMigrationPreservesAcceptedSchema3WithoutGrantingAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema3.db")
	db, ids := routingSchema3Database(t, path, "", true)
	before := unaffectedRows(t, db)
	var claimBefore string
	if e := db.QueryRow("SELECT attempt_id FROM capacity_claims").Scan(&claimBefore); e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if after := unaffectedRows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("migration rewrote accepted003 rows/JSON/receipt bytes")
	}
	var version, budgets, authorities int
	var claim string
	s.db.QueryRow("PRAGMA user_version").Scan(&version)
	s.db.QueryRow("SELECT count(*) FROM task_budgets").Scan(&budgets)
	s.db.QueryRow("SELECT count(*) FROM task_routing_versions").Scan(&authorities)
	s.db.QueryRow("SELECT attempt_id FROM capacity_claims").Scan(&claim)
	if version != 5 || budgets != 0 || authorities != 0 || claim != claimBefore || claim != ids.ActiveAttemptID {
		t.Fatal("migration granted work or released ownership", version, budgets, authorities, claim)
	}
	replayed := s.CreateTask(context.Background(), "66666666666666666666666666666666", ids.Input)
	if replayed.Status != 201 || replayed.Envelope.Receipt == nil {
		t.Fatal("new config/gate prevented old exact replay", replayed)
	}
	assertDurableJournal(t, s)
}
func TestRoutingMigrationRollbackAndCommittedAcknowledgement(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[after], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema3.db")
			db, _ := routingSchema3Database(t, path, "", true)
			before := unaffectedRows(t, db)
			s := &Store{db: db}
			fault := func() error { return errors.New("routing migration boundary fault") }
			if after {
				s.AfterCommit = fault
			} else {
				s.BeforeCommit = fault
			}
			err := s.migrateRouting(context.Background())
			if err == nil {
				t.Fatal("fault disappeared")
			}
			var version int
			db.QueryRow("PRAGMA user_version").Scan(&version)
			want := 3
			if after {
				want = 4
			}
			if version != want {
				t.Fatal(version, want, err)
			}
			if rows := unaffectedRows(t, db); !reflect.DeepEqual(before, rows) {
				t.Fatal("migration fault rewrote old rows")
			}
			db.Close()
			reopened, e := Open(path)
			if e != nil {
				t.Fatal("safe reopen could not recognize/retry whole migration", e)
			}
			reopened.Close()
		})
	}
}
func TestRoutingMigrationRefusesCollidingObjectsWithoutChangingSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collision.db")
	db, _ := routingSchema3Database(t, path, "", false)
	if _, e := db.Exec("CREATE TABLE routing_policy(sentinel TEXT);INSERT INTO routing_policy VALUES('keep')"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if s, e := Open(path); e == nil {
		s.Close()
		t.Fatal("unknown colliding schema accepted")
	}
	after, e := os.ReadFile(path)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("unknown source bytes changed", e)
	}
	db, e = sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var version int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 3 {
		t.Fatal(version)
	}
}

func TestRoutingMigrationCanceledBeforeTransactionPreservesSchema3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancelled.db")
	db, _ := routingSchema3Database(t, path, "", true)
	before := unaffectedRows(t, db)
	s := &Store{db: db}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := s.migrateRouting(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal("known no-commit cancellation became durability uncertainty", e)
	}
	var version, objects int
	db.QueryRow("PRAGMA user_version").Scan(&version)
	db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name=\x27routing_policy\x27").Scan(&objects)
	if version != 3 || objects != 0 || !reflect.DeepEqual(before, unaffectedRows(t, db)) {
		t.Fatal("canceled migration changed retained source", version, objects)
	}
}
func TestRoutingSchema4UnknownObjectsAndMissingSeedsRefuseWithoutReset(t *testing.T) {
	for _, change := range []string{"CREATE INDEX unknown_budget_index ON budget_ledger(task_id)", "DELETE FROM routing_profiles WHERE id=\x27claude-haiku-economy\x27"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema4.db")
			fixture, _ := workflowSchema4Database(t, path, "", false)
			if e := fixture.Close(); e != nil {
				t.Fatal(e)
			}
			dsn, e := sqliteDSN(path)
			if e != nil {
				t.Fatal(e)
			}
			db, e := sql.Open("sqlite", dsn)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { db.Close() })
			if _, e = db.Exec(change); e != nil {
				t.Fatal(e)
			}
			db.Close()
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if reopened, e := Open(path); e == nil {
				reopened.Close()
				t.Fatal("unknown or incomplete schema4 accepted")
			}
			after, e := os.ReadFile(path)
			if e != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("refused schema4 was reset or rewritten", e)
			}
		})
	}
}
