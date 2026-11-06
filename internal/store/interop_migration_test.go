package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Frozen schema-2 claim DDL from accepted 002 (f5edd96). The execution records
// below are populated with accepted APIs before this fixture is downgraded.
const frozenClaimV2 = "CREATE TABLE capacity_claims(connection_id TEXT PRIMARY KEY CHECK(connection_id='codex-native'),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id))"

func makeSchema2(t *testing.T, path string, populated ...bool) (*sql.DB, string, string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	project := openFixture(t, s, "migration")
	projectID := project.ID
	owner := domain.NewID()
	if err = s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if len(populated) > 0 && populated[0] {
		populateMigrationHistory(t, s, projectID, owner)
	}
	request := domain.NewID()
	r := s.CreateTask(ctx, owner, domain.StartTaskInput{RequestID: request, ProjectID: projectID, OriginalText: "Preserve original\r\n\ufeff text", ExecutionMode: "fake"})
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission).Attempt
	if _, err = s.ClaimAttempt(ctx, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: &a.ID, Code: "retained_incident", Message: "Keep unknown cleanup", CreatedAt: domain.Now()}
	if err = s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE tasks SET revision=9007199254740993,state_revision=9007199254740994"); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	// This synthetic fixture owns this disposable current database. Strip only
	// authored005/004 tables before restoring frozen accepted002 claim DDL.
	// Frozen accepted003/004 fixtures independently qualify real upgrade shapes.
	for _, name := range []string{"delivery_effects", "workflow_resolutions", "workflow_acceptances", "delivery_operations", "workflow_reviews", "workflow_checks", "workflow_steps", "workflows", "delivery_preferences", "budget_ledger", "budget_amendments", "routing_continuation_proofs", "routing_decisions", "task_budgets", "task_routing_versions", "routing_report_bindings", "routing_capacity_reports", "routing_capacity_state", "routing_account_links", "routing_accounts", "routing_profiles", "routing_contexts", "routing_policy"} {
		if _, err = db.Exec("DROP TABLE " + name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec("PRAGMA foreign_keys=ON; DROP TABLE capacity_claims; " + frozenClaimV2 + "; INSERT INTO capacity_claims VALUES('codex-native','" + a.ID + "'); PRAGMA user_version=2"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db, a.ID, request
}

func unaffectedRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, table := range []string{"policy", "projects", "drafts", "draft_versions", "receipts", "tasks", "task_versions", "attempts", "attempt_terminals", "attempt_events", "artifacts", "source_snapshots", "candidates", "response_acceptances", "execution_incidents", "execution_control"} {
		rows, err := db.Query("SELECT quote(rowid),* FROM " + table + " ORDER BY rowid")
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			pointers := make([]any, len(cols))
			for i := range pointers {
				pointers[i] = &values[i]
			}
			if err = rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			out[table] = append(out[table], encode(values))
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return out
}

func TestInteropMigrationPreservesRowsAndHeldClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema2.db")
	db, attemptID, request := makeSchema2(t, path, true)
	before := unaffectedRows(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if after := unaffectedRows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("unaffected rows changed", before, after)
	}
	var version int
	var resource, claim string
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 5 {
		t.Fatal(version, err)
	}
	if err = s.db.QueryRow("SELECT resource_id,attempt_id FROM capacity_claims").Scan(&resource, &claim); err != nil || resource != "managed-native" || claim != attemptID {
		t.Fatal(resource, claim, err)
	}
	g, err := s.Gate(context.Background())
	if err != nil || g.State != "fenced" || g.ActiveAttemptID == nil || *g.ActiveAttemptID != attemptID || g.IncidentID == nil {
		t.Fatal(g, err)
	}
	var receipt string
	if err = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", request).Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	if receipt == "" {
		t.Fatal("receipt lost")
	}
	assertDurableJournal(t, s)
}

func TestInteropMigrationRollbackAndLostAcknowledgement(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[afterCommit], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema2.db")
			db, _, _ := makeSchema2(t, path)
			defer db.Close()
			before := unaffectedRows(t, db)
			s := &Store{db: db}
			fault := func() error { return errors.New("injected migration acknowledgement fault") }
			if afterCommit {
				s.AfterCommit = fault
			} else {
				s.BeforeCommit = fault
			}
			err := s.migrateSharedCapacity(context.Background())
			var domainErr *domain.Error
			if !errors.As(err, &domainErr) {
				t.Fatal("missing typed migration failure", err)
			}
			wantVersion, wantCode := 2, "storage_failed"
			if afterCommit {
				wantVersion, wantCode = 4, "write_uncertain"
			}
			var version int
			if e := db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != wantVersion || domainErr.Code != wantCode {
				t.Fatal(version, domainErr, e)
			}
			if after := unaffectedRows(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("migration fault changed preserved rows")
			}
		})
	}
}

func TestInteropMigrationRejectsUnknownClaimDDLWithoutChangingBytes(t *testing.T) {
	for _, extra := range []string{"missing_check", "extra_index", "extra_trigger", "unknown_schema3"} {
		t.Run(extra, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unknown.db")
			db, attempt, _ := makeSchema2(t, path)
			switch extra {
			case "missing_check":
				_, _ = db.Exec("DROP TABLE capacity_claims;CREATE TABLE capacity_claims(connection_id TEXT PRIMARY KEY,attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id));INSERT INTO capacity_claims VALUES('codex-native','" + attempt + "')")
			case "extra_index":
				_, _ = db.Exec("CREATE INDEX foreign_claim_index ON capacity_claims(attempt_id)")
			case "extra_trigger":
				_, _ = db.Exec("CREATE TRIGGER foreign_claim_trigger AFTER DELETE ON capacity_claims BEGIN UPDATE execution_control SET state='open'; END")
			case "unknown_schema3":
				_, _ = db.Exec("PRAGMA user_version=3")
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := Open(path); err == nil {
				s.Close()
				t.Fatal("unknown claim DDL accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("unknown database bytes changed", err)
			}
		})
	}
}
