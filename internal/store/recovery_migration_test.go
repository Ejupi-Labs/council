package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

//go:embed testdata/schema6-graph.sql
var frozenRecoveryV6GraphDDL string

// Compare accepted006 graph and capacity DDL against a literal fixture, not
// the current graphSchema constants used by the migration under test.
func assertFrozenRecoveryV6DDL(t *testing.T, db *sql.DB) {
	t.Helper()
	ddl := strings.SplitN(frozenRecoveryV6GraphDDL, "\n", 2)
	if len(ddl) != 2 {
		t.Fatal("missing frozen006 DDL")
	}
	count := 0
	for _, statement := range strings.Split(ddl[1], ";") {
		statement = strings.TrimSpace(statement)
		fields := strings.Fields(statement)
		if len(fields) < 3 {
			continue
		}
		name := strings.Split(fields[2], "(")[0]
		var actual string
		if err := db.QueryRow("SELECT sql FROM sqlite_schema WHERE name=?", name).Scan(&actual); err != nil {
			t.Fatal(name, err)
		}
		if compactDDL(actual) != compactDDL(statement) {
			t.Fatal("accepted006 DDL changed", name)
		}
		count++
	}
	if count != 13 {
		t.Fatal("incomplete frozen006 DDL", count)
	}
}

// recoveryV6Fixture starts from the frozen populated accepted005 records, runs
// only the accepted 006 migration, and retains a live claim and an incident.
func recoveryV6Fixture(t *testing.T, path string) (*sql.DB, graphSchema5IDs) {
	t.Helper()
	db, ids := graphSchema5Database(t, path, true)
	s := &Store{db: db}
	if err := s.migrateGraphs(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFrozenRecoveryV6DDL(t, db)
	incident := strings.Repeat("ab", 16)
	if _, err := db.Exec("INSERT INTO execution_incidents(id,attempt_id,data) VALUES(?,?,?)", incident, ids.ActiveAttemptID, `{"id":"`+incident+`","code":"fixture","message":"retained uncertain owner","createdAt":"2026-11-07T00:00:00Z"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE execution_control SET state='fenced',incident_id=? WHERE singleton=1", incident); err != nil {
		t.Fatal(err)
	}
	return db, ids
}

// Snapshot complete raw row values, including all accepted006 graph tables.
func recoveryV6Rows(t *testing.T, db *sql.DB) map[string][][]any {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT GLOB 'sqlite_*' AND name NOT IN ('recovery_owner_epoch','recovery_owner_epoch_events','recovery_checkpoints','recovery_graph_scopes','recovery_observations','recovery_decisions') ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	out := make(map[string][][]any)
	for _, name := range names {
		r, err := db.Query(`SELECT * FROM "` + name + `" ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := r.Columns()
		if err != nil {
			t.Fatal(err)
		}
		out[name] = [][]any{}
		for r.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, v := range values {
				if b, ok := v.([]byte); ok {
					values[i] = string(b)
				}
			}
			out[name] = append(out[name], values)
		}
		if err := r.Err(); err != nil {
			t.Fatal(err)
		}
		r.Close()
	}
	return out
}

func TestRecoveryMigrationPreservesV6RowsAndFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accepted6.sqlite3")
	db, _ := recoveryV6Fixture(t, path)
	before := recoveryV6Rows(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		t.Fatal(version, err)
	}
	if after := recoveryV6Rows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("v7 migration altered accepted v6 data or held claims")
	}
	var state string
	var incident sql.NullString
	if err := s.db.QueryRow("SELECT state,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &incident); err != nil || state != "fenced" || !incident.Valid {
		t.Fatal("uncertain owner lost fence", state, incident, err)
	}
	for _, table := range []string{"recovery_checkpoints", "recovery_graph_scopes", "recovery_observations", "recovery_decisions"} {
		var n int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("migration invented recovery authority", table, n, err)
		}
	}
}

func TestRecoveryMigrationFaultBoundaries(t *testing.T) {
	for _, boundary := range []string{"before_commit", "after_commit", "cancelled"} {
		t.Run(boundary, func(t *testing.T) {
			db, _ := recoveryV6Fixture(t, filepath.Join(t.TempDir(), "old.sqlite3"))
			defer db.Close()
			before := recoveryV6Rows(t, db)
			s := &Store{db: db}
			ctx := context.Background()
			fail := func() error { return errors.New("injected migration failure") }
			if boundary == "before_commit" {
				s.BeforeCommit = fail
			}
			if boundary == "after_commit" {
				s.AfterCommit = fail
			}
			if boundary == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := s.migrateRecovery(ctx)
			if err == nil {
				t.Fatal("missing migration failure")
			}
			want := 6
			if boundary == "after_commit" {
				want = 7
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "write_uncertain" {
					t.Fatal("lost uncertain commit", err)
				}
			}
			var got int
			if e := db.QueryRow("PRAGMA user_version").Scan(&got); e != nil || got != want {
				t.Fatal(got, want, e)
			}
			if after := recoveryV6Rows(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("failure rewrote historical rows")
			}
			var count int
			if e := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='recovery_checkpoints'").Scan(&count); e != nil || (want == 6 && count != 0) || (want == 7 && count != 1) {
				t.Fatal("partial schema", want, count, e)
			}
		})
	}
}

func TestRecoveryMigrationRejectsUnknownSchemas(t *testing.T) {
	for _, statement := range []string{"CREATE TABLE unexpected_table(x TEXT)", "CREATE INDEX unexpected_index ON tasks(project_id)", "CREATE TABLE recovery_checkpoints(id TEXT)", "PRAGMA user_version=7"} {
		t.Run(statement, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unknown.sqlite3")
			db, _ := recoveryV6Fixture(t, path)
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			before := recoveryV6Rows(t, db)
			db.Close()
			if s, err := Open(path); err == nil {
				s.Close()
				t.Fatal("unknown schema accepted")
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if after := recoveryV6Rows(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected schema modified data")
			}
		})
	}
}

func TestRecoveryDomainInputValidation(t *testing.T) {
	id := strings.Repeat("1", 32)
	good := domain.PauseTaskInput{RequestID: id, TaskVersion: "1", ExpectedTaskRevision: "2"}
	if err := domain.ValidatePauseTaskInput(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []domain.PauseTaskInput{{RequestID: id, TaskVersion: "01", ExpectedTaskRevision: "2"}, {RequestID: "x", TaskVersion: "1", ExpectedTaskRevision: "2"}, {RequestID: id, TaskVersion: "1", ExpectedTaskRevision: "9223372036854775808"}} {
		if domain.ValidatePauseTaskInput(bad) == nil {
			t.Fatal("accepted invalid pause", bad)
		}
	}
	scope := domain.RecoveryScope{Kind: "task", TaskID: id, TaskVersion: "1", ExpectedTaskRevision: "2"}
	if err := domain.ValidateRecoveryScope(scope); err != nil {
		t.Fatal(err)
	}
	scope.GraphID = id
	if domain.ValidateRecoveryScope(scope) == nil {
		t.Fatal("accepted mixed task/graph scope")
	}
}

func TestRecoverySchemaFreshAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.sqlite3")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		t.Fatal(version, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal("fresh schema did not reopen", err)
	}
	if _, err := s.db.Exec("CREATE INDEX unexpected_recovery_index ON recovery_checkpoints(state)"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err = Open(path); err == nil {
		s.Close()
		t.Fatal("tampered v7 schema was accepted")
	}
}
