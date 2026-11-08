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

//go:embed testdata/schema7-recovery.sql
var frozenHistoryV7RecoveryDDL string

// The fixture starts from frozen populated schema5 and separately verifies the
// accepted schema6/7 definitions before the migration under test. All rows are
// controlled synthetic metadata, including the retained claim and incident.
func historyV7Fixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, ids := recoveryV6Fixture(t, path)
	s := &Store{db: db}
	if err := s.migrateRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, statement := range strings.Split(frozenHistoryV7RecoveryDDL, ";") {
		statement = strings.TrimSpace(statement)
		fields := strings.Fields(statement)
		if len(fields) < 3 {
			continue
		}
		name := strings.Split(fields[2], "(")[0]
		var actual string
		if err := db.QueryRow("SELECT sql FROM sqlite_schema WHERE name=?", name).Scan(&actual); err != nil || compactDDL(actual) != compactDDL(statement) {
			t.Fatal("accepted v7 DDL changed", name, err)
		}
		count++
	}
	if count != 11 {
		t.Fatal("incomplete frozen v7 DDL", count)
	}
	checkpointID, observationID, decisionID, requestID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	incidentID := strings.Repeat("ab", 16)
	if _, err := db.Exec("INSERT INTO recovery_checkpoints VALUES(?,'task',?,NULL,NULL,1,'paused',?,?)", checkpointID, ids.TaskID, strings.Repeat("6", 32), `{"fixture":"retained checkpoint"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO recovery_observations VALUES(?,?,?)", observationID, incidentID, `{"fixture":"retained observation"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO recovery_decisions VALUES(?,?,?,?)", decisionID, incidentID, requestID, `{"outcome":"unresolved","fixture":"retained decision"}`); err != nil {
		t.Fatal(err)
	}
	return db
}

func historyLegacyRows(t *testing.T, db *sql.DB) map[string][][]any {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT GLOB 'sqlite_*' AND name NOT IN ('history_snapshots','history_previews','history_export_operations','history_effect_journal','history_incident_bindings') ORDER BY name")
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
	out := map[string][][]any{}
	for _, name := range names {
		r, err := db.Query(`SELECT * FROM "` + name + `" ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := r.Columns()
		if err != nil {
			t.Fatal(err)
		}
		out[name] = [][]any{}
		for r.Next() {
			values, pointers := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := r.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				if b, ok := value.([]byte); ok {
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

func TestHistoryMigrationPreservesV7RowsAndFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old7.sqlite3")
	db := historyV7Fixture(t, path)
	before := historyLegacyRows(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 8 {
		t.Fatal(version, err)
	}
	if after := historyLegacyRows(t, s.db); !reflect.DeepEqual(before, after) {
		t.Fatal("history migration rewrote v7 records, receipts, epoch or claim")
	}
	var state string
	var incident sql.NullString
	if err := s.db.QueryRow("SELECT state,incident_id FROM execution_control WHERE singleton=1").Scan(&state, &incident); err != nil || state != "fenced" || !incident.Valid {
		t.Fatal("migration changed retained global fence", state, incident, err)
	}
	for _, table := range []string{"history_snapshots", "history_previews", "history_export_operations", "history_effect_journal", "history_incident_bindings"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("migration invented history authority", table, count, err)
		}
	}
}

func TestHistoryMigrationFaultBoundaries(t *testing.T) {
	for _, boundary := range []string{"before_commit", "after_commit", "cancelled"} {
		t.Run(boundary, func(t *testing.T) {
			db := historyV7Fixture(t, filepath.Join(t.TempDir(), "old7.sqlite3"))
			defer db.Close()
			before := historyLegacyRows(t, db)
			s := &Store{db: db}
			ctx := context.Background()
			fault := func() error { return errors.New("controlled migration fault") }
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
			err := s.migrateHistory(ctx)
			if err == nil {
				t.Fatal("migration fault disappeared")
			}
			want := 7
			if boundary == "after_commit" {
				want = 8
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "write_uncertain" {
					t.Fatal("lost uncertain commit", err)
				}
			}
			var got, tables int
			if e := db.QueryRow("PRAGMA user_version").Scan(&got); e != nil || got != want {
				t.Fatal(got, want, e)
			}
			if e := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='history_snapshots'").Scan(&tables); e != nil || (want == 7 && tables != 0) || (want == 8 && tables != 1) {
				t.Fatal("partial history migration", want, tables, e)
			}
			if after := historyLegacyRows(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("failure changed v7 records")
			}
		})
	}
}

func TestHistoryMigrationRejectsUnknownV7DDL(t *testing.T) {
	for _, statement := range []string{"CREATE TABLE unexpected_history_fixture(x TEXT)", "CREATE INDEX unexpected_history_index ON recovery_checkpoints(state)", "CREATE TABLE history_snapshots(id TEXT)", "PRAGMA user_version=8"} {
		t.Run(statement, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unknown.sqlite3")
			db := historyV7Fixture(t, path)
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			before := historyLegacyRows(t, db)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(path); err == nil {
				s.Close()
				t.Fatal("unknown schema accepted")
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if after := historyLegacyRows(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected schema changed prior data")
			}
		})
	}
}

func TestHistorySchemaFreshReopenRejectsChangedTriggers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.sqlite3")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := readRecoveryOwnerEpoch(context.Background(), s.db)
	if err != nil || epoch.State != "clean" || epoch.Generation != 0 {
		t.Fatal("fresh provenance changed", epoch, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal("current history schema did not reopen", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER history_effect_journal_no_delete"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(path); err == nil {
		reopened.Close()
		t.Fatal("altered history schema accepted")
	}
}

func TestHistorySchemaImmutabilityScopeAndSinglePermit(t *testing.T) {
	s := testStore(t)
	db := s.db
	projectID, snapshotID, otherSnapshot, previewID, operationID := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
	if _, err := db.Exec("INSERT INTO projects(sequence,id,physical_identity,resolved_path,data) VALUES(1,?,'controlled-project','controlled-path','{}')", projectID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{snapshotID, otherSnapshot} {
		if _, err := db.Exec("INSERT INTO history_snapshots VALUES(?,?,1,?,'{}')", id, projectID, make([]byte, 32)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO history_previews VALUES(?,?,1,'{}')", previewID, snapshotID); err != nil {
		t.Fatal(err)
	}
	insertOperation := "INSERT INTO history_export_operations VALUES(?,?,?,?,?,1,1,'prepared',0,0,'{}')"
	if _, err := db.Exec(insertOperation, operationID, domain.NewID(), previewID, otherSnapshot, domain.NewID()); err == nil {
		t.Fatal("operation accepted a preview from another snapshot")
	}
	if _, err := db.Exec(insertOperation, operationID, domain.NewID(), previewID, snapshotID, domain.NewID()); err != nil {
		t.Fatal(err)
	}
	eventID := domain.NewID()
	if _, err := db.Exec("INSERT INTO history_effect_journal VALUES(?,1,?,'permit','{}')", operationID, eventID); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"UPDATE history_effect_journal SET data='changed' WHERE id=?", []any{eventID}},
		{"DELETE FROM history_effect_journal WHERE id=?", []any{eventID}},
		{"INSERT OR REPLACE INTO history_effect_journal VALUES(?,1,?,'observation','{}')", []any{operationID, domain.NewID()}},
		{"INSERT OR REPLACE INTO history_effect_journal VALUES(?,2,?,'permit','{}')", []any{operationID, domain.NewID()}},
		{"UPDATE history_snapshots SET data='changed' WHERE id=?", []any{snapshotID}},
		{"DELETE FROM history_snapshots WHERE id=?", []any{snapshotID}},
		{"INSERT OR REPLACE INTO history_snapshots VALUES(?,?,1,?,'changed')", []any{snapshotID, projectID, make([]byte, 32)}},
		{"UPDATE history_previews SET data='changed' WHERE id=?", []any{previewID}},
		{"INSERT OR REPLACE INTO history_previews VALUES(?,?,1,'changed')", []any{previewID, snapshotID}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err == nil {
			t.Fatal("immutable history statement succeeded", statement.query)
		}
	}
	var data string
	if err := db.QueryRow("SELECT data FROM history_effect_journal WHERE id=?", eventID).Scan(&data); err != nil || data != "{}" {
		t.Fatal("original permit changed", data, err)
	}
}
