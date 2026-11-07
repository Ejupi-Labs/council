package store

import (
	"context"
	"database/sql"
	"strings"
)

// recoverySchema adds durable checkpoint and append-only reconciliation records.
// Existing incident and graph records remain unchanged during the v6 migration.
const recoverySchema = `
CREATE TABLE recovery_owner_epoch(singleton INTEGER PRIMARY KEY CHECK(singleton=1),owner_instance_id TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN ('untracked','clean','dirty')),generation INTEGER NOT NULL CHECK(typeof(generation)='integer' AND generation>=0));
INSERT INTO recovery_owner_epoch VALUES(1,'','untracked',0);
CREATE TABLE recovery_owner_epoch_events(generation INTEGER NOT NULL CHECK(typeof(generation)='integer' AND generation>=1),owner_instance_id TEXT NOT NULL,kind TEXT NOT NULL CHECK(kind IN ('begin','clean')),created_at TEXT NOT NULL,PRIMARY KEY(generation,kind),UNIQUE(owner_instance_id,kind));
CREATE TABLE recovery_checkpoints(id TEXT PRIMARY KEY,scope_kind TEXT NOT NULL CHECK(scope_kind IN ('task','member','graph')),task_id TEXT REFERENCES tasks(id),graph_id TEXT REFERENCES task_graphs(id),node_id TEXT REFERENCES graph_nodes(id),revision INTEGER NOT NULL CHECK(typeof(revision)='integer' AND revision>=1),state TEXT NOT NULL CHECK(state IN ('pausing','paused','resumed','cancelled','failed')),owner_instance_id TEXT NOT NULL,data TEXT NOT NULL,CHECK((scope_kind='task' AND task_id IS NOT NULL AND graph_id IS NULL AND node_id IS NULL) OR (scope_kind='member' AND task_id IS NOT NULL AND graph_id IS NOT NULL AND node_id IS NOT NULL) OR (scope_kind='graph' AND task_id IS NULL AND graph_id IS NOT NULL AND node_id IS NULL)));
CREATE INDEX recovery_checkpoints_task ON recovery_checkpoints(task_id,state);
CREATE INDEX recovery_checkpoints_graph ON recovery_checkpoints(graph_id,state);
CREATE TABLE recovery_graph_scopes(checkpoint_id TEXT PRIMARY KEY REFERENCES recovery_checkpoints(id),request_id TEXT UNIQUE NOT NULL,graph_id TEXT NOT NULL REFERENCES task_graphs(id),node_id TEXT NOT NULL REFERENCES graph_nodes(id),task_id TEXT NOT NULL REFERENCES tasks(id),owner_instance_id TEXT NOT NULL,graph_version INTEGER NOT NULL CHECK(typeof(graph_version)='integer' AND graph_version>=1),graph_control_revision INTEGER NOT NULL CHECK(typeof(graph_control_revision)='integer' AND graph_control_revision>=1),node_control_revision INTEGER NOT NULL CHECK(typeof(node_control_revision)='integer' AND node_control_revision>=1),generation INTEGER NOT NULL CHECK(typeof(generation)='integer' AND generation>=1),state TEXT NOT NULL CHECK(state IN ('active','superseded')),data TEXT NOT NULL);
CREATE INDEX recovery_graph_scopes_node ON recovery_graph_scopes(graph_id,node_id,state);
CREATE TABLE recovery_observations(id TEXT PRIMARY KEY,incident_id TEXT NOT NULL REFERENCES execution_incidents(id),data TEXT NOT NULL);
CREATE INDEX recovery_observations_incident ON recovery_observations(incident_id);
CREATE TABLE recovery_decisions(id TEXT PRIMARY KEY,incident_id TEXT NOT NULL REFERENCES execution_incidents(id),request_id TEXT UNIQUE NOT NULL,data TEXT NOT NULL);
CREATE INDEX recovery_decisions_incident ON recovery_decisions(incident_id);
`

func expectedRecoveryDDL() map[string]string {
	expected := expectedGraphDDL()
	for _, statement := range strings.Split(recoverySchema, ";") {
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
	return expected
}

func validateKnownRecoveryDDL(ctx context.Context, q schemaQueryer) error {
	expected := expectedRecoveryDDL()
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

func (s *Store) validateRecoverySchemaWith(ctx context.Context, q queryer) error {
	if err := validateKnownRecoveryDDL(ctx, q); err != nil {
		return err
	}
	if _, err := readRecoveryOwnerEpoch(ctx, q); err != nil {
		return err
	}
	if err := validateRoutingSeedCounts(ctx, q); err != nil {
		return err
	}
	if _, err := s.deliveryPreference(ctx, q); err != nil {
		return err
	}
	if err := validateForeignKeys(ctx, q); err != nil {
		return err
	}
	_, err := s.graphClaimRecords(ctx, q)
	return err
}

func (s *Store) migrateRecovery(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var version, appID int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return storageError()
		}
		if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil || version != 6 || appID != applicationID {
			return storageError()
		}
		if err := s.validateGraphSchemaWith(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, recoverySchema); err != nil {
			return storageError()
		}
		if err := s.validateRecoverySchemaWith(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=7"); err != nil {
			return storageError()
		}
		return nil
	})
}
