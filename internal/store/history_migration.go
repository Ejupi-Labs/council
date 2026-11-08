package store

import (
	"context"
	"database/sql"
	"strings"
)

// historySchema stores only the controller's bounded typed snapshots and
// operation records. Existing v7 records and receipt bytes are not rewritten.
const historySchema = `
CREATE TABLE history_snapshots(id TEXT PRIMARY KEY,project_id TEXT NOT NULL REFERENCES projects(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision=1),cursor_key BLOB NOT NULL CHECK(typeof(cursor_key)='blob' AND length(cursor_key)=32),data TEXT NOT NULL);
CREATE INDEX history_snapshots_project ON history_snapshots(project_id,id);
CREATE TABLE history_previews(id TEXT PRIMARY KEY,snapshot_id TEXT NOT NULL REFERENCES history_snapshots(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision=1),data TEXT NOT NULL,UNIQUE(id,snapshot_id));
CREATE TABLE history_export_operations(id TEXT PRIMARY KEY,request_id TEXT NOT NULL UNIQUE,preview_id TEXT NOT NULL,snapshot_id TEXT NOT NULL REFERENCES history_snapshots(id),owner_instance_id TEXT NOT NULL,owner_generation INTEGER NOT NULL CHECK(typeof(owner_generation)='integer' AND owner_generation>=1),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),state TEXT NOT NULL CHECK(state IN ('prepared','effect_started','published','failed','uncertain','no_publication')),frozen INTEGER NOT NULL CHECK(typeof(frozen)='integer' AND frozen IN (0,1)),journal_revision INTEGER NOT NULL CHECK(typeof(journal_revision)='integer' AND journal_revision>=0),data TEXT NOT NULL,FOREIGN KEY(preview_id,snapshot_id) REFERENCES history_previews(id,snapshot_id));
CREATE INDEX history_export_operations_snapshot ON history_export_operations(snapshot_id,id);
CREATE TABLE history_effect_journal(operation_id TEXT NOT NULL REFERENCES history_export_operations(id),sequence INTEGER NOT NULL CHECK(typeof(sequence)='integer' AND sequence>=1),id TEXT NOT NULL UNIQUE,kind TEXT NOT NULL CHECK(kind IN ('permit','observation','decision')),data TEXT NOT NULL,PRIMARY KEY(operation_id,sequence));
CREATE UNIQUE INDEX history_effect_journal_one_permit ON history_effect_journal(operation_id) WHERE kind='permit';
CREATE TABLE history_incident_bindings(incident_id TEXT PRIMARY KEY REFERENCES execution_incidents(id),kind TEXT NOT NULL CHECK(kind IN ('operation','capture','preview','create')),operation_id TEXT UNIQUE REFERENCES history_export_operations(id),request_id TEXT NOT NULL,fingerprint TEXT NOT NULL,owner_instance_id TEXT NOT NULL,owner_generation INTEGER NOT NULL CHECK(typeof(owner_generation)='integer' AND owner_generation>=1),CHECK((kind='operation' AND operation_id IS NOT NULL) OR (kind IN ('capture','preview','create') AND operation_id IS NULL)),UNIQUE(kind,request_id));
`

const historyJournalNoUpdate = `CREATE TRIGGER history_effect_journal_no_update BEFORE UPDATE ON history_effect_journal BEGIN SELECT RAISE(ABORT,'history journal is append-only'); END`
const historyJournalNoDelete = `CREATE TRIGGER history_effect_journal_no_delete BEFORE DELETE ON history_effect_journal BEGIN SELECT RAISE(ABORT,'history journal is append-only'); END`

// SQLite REPLACE can bypass DELETE triggers unless recursive triggers are
// enabled. Reject every journal uniqueness collision before conflict handling.
const historyJournalNoReplace = `CREATE TRIGGER history_effect_journal_no_replace BEFORE INSERT ON history_effect_journal WHEN EXISTS(SELECT 1 FROM history_effect_journal e WHERE e.id=NEW.id OR (e.operation_id=NEW.operation_id AND (e.sequence=NEW.sequence OR (e.kind='permit' AND NEW.kind='permit')))) BEGIN SELECT RAISE(ABORT,'history journal is append-only'); END`
const historySnapshotNoUpdate = `CREATE TRIGGER history_snapshots_no_update BEFORE UPDATE ON history_snapshots BEGIN SELECT RAISE(ABORT,'history snapshot is immutable'); END`
const historySnapshotNoDelete = `CREATE TRIGGER history_snapshots_no_delete BEFORE DELETE ON history_snapshots BEGIN SELECT RAISE(ABORT,'history snapshot is immutable'); END`
const historySnapshotNoReplace = `CREATE TRIGGER history_snapshots_no_replace BEFORE INSERT ON history_snapshots WHEN EXISTS(SELECT 1 FROM history_snapshots s WHERE s.id=NEW.id) BEGIN SELECT RAISE(ABORT,'history snapshot is immutable'); END`
const historyPreviewNoUpdate = `CREATE TRIGGER history_previews_no_update BEFORE UPDATE ON history_previews BEGIN SELECT RAISE(ABORT,'history preview is immutable'); END`
const historyPreviewNoDelete = `CREATE TRIGGER history_previews_no_delete BEFORE DELETE ON history_previews BEGIN SELECT RAISE(ABORT,'history preview is immutable'); END`
const historyPreviewNoReplace = `CREATE TRIGGER history_previews_no_replace BEFORE INSERT ON history_previews WHEN EXISTS(SELECT 1 FROM history_previews p WHERE p.id=NEW.id) BEGIN SELECT RAISE(ABORT,'history preview is immutable'); END`
const historyBindingNoUpdate = `CREATE TRIGGER history_incident_bindings_no_update BEFORE UPDATE ON history_incident_bindings BEGIN SELECT RAISE(ABORT,'history incident binding is immutable'); END`
const historyBindingNoDelete = `CREATE TRIGGER history_incident_bindings_no_delete BEFORE DELETE ON history_incident_bindings BEGIN SELECT RAISE(ABORT,'history incident binding is immutable'); END`
const historyBindingNoReplace = `CREATE TRIGGER history_incident_bindings_no_replace BEFORE INSERT ON history_incident_bindings WHEN EXISTS(SELECT 1 FROM history_incident_bindings b WHERE b.incident_id=NEW.incident_id OR (NEW.operation_id IS NOT NULL AND b.operation_id=NEW.operation_id) OR (b.kind=NEW.kind AND b.request_id=NEW.request_id)) BEGIN SELECT RAISE(ABORT,'history incident binding is immutable'); END`

var historyTriggers = map[string]string{
	"history_effect_journal_no_update":     historyJournalNoUpdate,
	"history_effect_journal_no_delete":     historyJournalNoDelete,
	"history_effect_journal_no_replace":    historyJournalNoReplace,
	"history_snapshots_no_update":          historySnapshotNoUpdate,
	"history_snapshots_no_delete":          historySnapshotNoDelete,
	"history_snapshots_no_replace":         historySnapshotNoReplace,
	"history_previews_no_update":           historyPreviewNoUpdate,
	"history_previews_no_delete":           historyPreviewNoDelete,
	"history_previews_no_replace":          historyPreviewNoReplace,
	"history_incident_bindings_no_update":  historyBindingNoUpdate,
	"history_incident_bindings_no_delete":  historyBindingNoDelete,
	"history_incident_bindings_no_replace": historyBindingNoReplace,
}

func expectedHistoryDDL() map[string]string {
	expected := expectedRecoveryDDL()
	for _, statement := range strings.Split(historySchema, ";") {
		statement = strings.TrimSpace(statement)
		fields := strings.Fields(statement)
		if len(fields) < 3 || !strings.EqualFold(fields[0], "CREATE") {
			continue
		}
		index := 2
		if strings.EqualFold(fields[1], "UNIQUE") {
			index = 3
		}
		if len(fields) <= index {
			continue
		}
		name := strings.Split(fields[index], "(")[0]
		expected[name] = compactDDL(statement)
	}
	for name, statement := range historyTriggers {
		expected[name] = compactDDL(statement)
	}
	return expected
}

func validateKnownHistoryDDL(ctx context.Context, q schemaQueryer) error {
	expected := expectedHistoryDDL()
	rows, err := q.QueryContext(ctx, "SELECT name,sql FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*' AND type IN ('table','index','trigger','view')")
	if err != nil {
		return storageError()
	}
	seen := map[string]bool{}
	for rows.Next() {
		var name, statement string
		if err = rows.Scan(&name, &statement); err != nil {
			rows.Close()
			return storageError()
		}
		want, ok := expected[name]
		got := compactDDL(statement)
		quoted := strings.Replace(want, "createtable"+name+"(", "createtable\""+name+"\"(", 1)
		if !ok || got != want && got != quoted {
			rows.Close()
			return storageError()
		}
		seen[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(seen) != len(expected) {
		return storageError()
	}
	return nil
}

func (s *Store) validateHistorySchemaWith(ctx context.Context, q queryer) error {
	if err := validateKnownHistoryDDL(ctx, q); err != nil {
		return err
	}
	return s.validateRecoveryDataWith(ctx, q)
}

func (s *Store) migrateHistory(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var version, appID int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return storageError()
		}
		if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil || version != 7 || appID != applicationID {
			return storageError()
		}
		if err := s.validateRecoverySchemaWith(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, historySchema); err != nil {
			return storageError()
		}
		for _, statement := range historyTriggers {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return storageError()
			}
		}
		if err := s.validateHistorySchemaWith(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=8"); err != nil {
			return storageError()
		}
		return nil
	})
}
