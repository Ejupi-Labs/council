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

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func workflowPreservedRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	newTables := map[string]bool{"delivery_preferences": true, "workflows": true, "workflow_steps": true, "workflow_reviews": true, "workflow_checks": true, "workflow_resolutions": true, "workflow_acceptances": true, "delivery_operations": true, "delivery_effects": true}
	rows, e := db.Query("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT GLOB 'sqlite_*' ORDER BY name")
	if e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			rows.Close()
			t.Fatal(e)
		}
		if !newTables[name] {
			names = append(names, name)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	out := map[string][]string{}
	for _, name := range names {
		r, e := db.Query(`SELECT * FROM "` + name + `" ORDER BY rowid`)
		if e != nil {
			t.Fatal(e)
		}
		columns, e := r.Columns()
		if e != nil {
			r.Close()
			t.Fatal(e)
		}
		out[name] = []string{}
		for r.Next() {
			values := make([]any, len(columns))
			ptrs := make([]any, len(columns))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if e = r.Scan(ptrs...); e != nil {
				r.Close()
				t.Fatal(e)
			}
			raw, e := json.Marshal(values)
			if e != nil {
				r.Close()
				t.Fatal(e)
			}
			out[name] = append(out[name], string(raw))
		}
		e = r.Err()
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
	}
	return out
}

func TestWorkflowMigrationPreservesAcceptedSchema4WithoutGrantingAuthority(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "schema4.db")
	db, ids := workflowSchema4Database(t, databasePath, "", true)
	before := workflowPreservedRows(t, db)
	artifactPath := filepath.Join(filepath.Dir(databasePath), "artifacts", strings.Repeat("0a", 16)+".bin")
	bytesBefore, e := os.ReadFile(artifactPath)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	s, e := Open(databasePath)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Close() })
	if after := workflowPreservedRows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("schema5 migration rewrote accepted004 row JSON/receipts/defaults/claim")
	}
	var version, workflows int
	if e = s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != 5 {
		t.Fatal("accepted004 was not upgraded", version, e)
	}
	if e = s.db.QueryRow("SELECT count(*) FROM workflows").Scan(&workflows); e != nil || workflows != 0 {
		t.Fatal("migration seeded workflow authority", workflows, e)
	}
	pref, e := s.DeliveryPreference(context.Background())
	if e != nil || pref.LocalMode != "review_only" || pref.SelectionSource != "default" {
		t.Fatal("migration inferred local delivery consent", pref, e)
	}
	policy, e := s.RoutingPolicy(context.Background())
	if e != nil || policy.DefaultBudget.AdmissionLimit != "2" {
		t.Fatal("migration refilled existing saved default", policy, e)
	}
	bytesAfter, e := os.ReadFile(artifactPath)
	if e != nil || !reflect.DeepEqual(bytesBefore, bytesAfter) {
		t.Fatal("migration changed retained exact artifact bytes", e)
	}
	replayed := s.CreateTask(context.Background(), strings.Repeat("6", 32), ids.Input)
	if replayed.Status != 201 || replayed.Envelope.Receipt == nil {
		t.Fatal("migration/new defaults prevented original exact receipt replay", replayed)
	}
	assertDurableJournal(t, s)
}

func TestWorkflowMigrationFreshFourExistingExplicitDefaultsUnchanged(t *testing.T) {
	for _, limit := range []string{"", "2", "7"} {
		t.Run("saved_"+limit, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "store.db")
			if limit != "" {
				db, _ := workflowSchema4Database(t, databasePath, "", false)
				var raw string
				if e := db.QueryRow("SELECT data FROM routing_policy").Scan(&raw); e != nil {
					t.Fatal(e)
				}
				p, e := decode[domain.RoutingPolicy](raw)
				if e != nil {
					t.Fatal(e)
				}
				p.DefaultBudget.AdmissionLimit = limit
				if _, e = db.Exec("UPDATE routing_policy SET data=?", encode(p)); e != nil {
					t.Fatal(e)
				}
				db.Close()
			}
			s, e := Open(databasePath)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = s.Close() })
			p, e := s.RoutingPolicy(context.Background())
			want := limit
			if want == "" {
				want = "4"
			}
			if e != nil || p.DefaultBudget.AdmissionLimit != want {
				t.Fatal("fresh/default provenance was inferred from numeric value", p.DefaultBudget.AdmissionLimit, want, e)
			}
		})
	}
}

func TestWorkflowMigrationRollbackUncertainAcknowledgementAndCancellation(t *testing.T) {
	for _, boundary := range []string{"before_commit", "after_commit", "cancelled"} {
		t.Run(boundary, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "schema4.db")
			db, _ := workflowSchema4Database(t, databasePath, "", true)
			s := &Store{db: db}
			before := workflowPreservedRows(t, db)
			ctx := context.Background()
			fault := func() error { return errors.New("workflow migration durability fixture") }
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
			err := s.migrateWorkflow(ctx)
			if err == nil {
				t.Fatal("migration boundary fault disappeared")
			}
			wantVersion := 4
			if boundary == "after_commit" {
				wantVersion = 5
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "write_uncertain" {
					t.Fatal("commit acknowledgement lost uncertainty", err)
				}
			}
			if boundary == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("known precommit cancellation became durability fault", err)
			}
			var version int
			if e := db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != wantVersion {
				t.Fatal("wrong transactional migration version", version, wantVersion, e)
			}
			if !reflect.DeepEqual(before, workflowPreservedRows(t, db)) {
				t.Fatal("migration failure changed accepted historical records")
			}
		})
	}
}

func TestWorkflowMigrationUnknownSchemaRefusesWithoutRepair(t *testing.T) {
	for _, change := range []string{"CREATE TABLE workflows(sentinel TEXT)", "CREATE INDEX foreign_budget_index ON budget_ledger(task_id)", "PRAGMA user_version=5"} {
		t.Run(change, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "refuse.db")
			db, _ := workflowSchema4Database(t, databasePath, "", false)
			if _, e := db.Exec(change); e != nil {
				t.Fatal(e)
			}
			before := workflowPreservedRows(t, db)
			db.Close()
			if s, e := Open(databasePath); e == nil {
				s.Close()
				t.Fatal("unknown/colliding/incomplete schema accepted")
			}
			dsn, e := sqliteDSN(databasePath)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := sql.Open("sqlite", dsn)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = raw.Close() })
			if !reflect.DeepEqual(before, workflowPreservedRows(t, raw)) {
				t.Fatal("refused database was repaired/reset")
			}
		})
	}
}
