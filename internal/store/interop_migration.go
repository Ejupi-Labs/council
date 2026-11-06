package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const claimTableV2 = "CREATE TABLE capacity_claims(connection_id TEXT PRIMARY KEY CHECK(connection_id='codex-native'),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id))"
const claimTableV3 = "CREATE TABLE capacity_claims(resource_id TEXT PRIMARY KEY CHECK(resource_id='managed-native'),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id))"

type schemaQueryer interface {
	queryer
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Store) initializeSchema(ctx context.Context, foundation bool) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		if foundation {
			if _, err := tx.ExecContext(ctx, schema); err != nil {
				return storageError()
			}
			policy := domain.Policy{Mode: "autonomous", SelectionSource: "default", Revision: "1", UpdatedAt: domain.Now()}
			if _, err := tx.ExecContext(ctx, "INSERT INTO policy(singleton,revision,data) VALUES(1,1,?)", encode(policy)); err != nil {
				return storageError()
			}
		}
		if _, err := tx.ExecContext(ctx, executionSchema); err != nil {
			return storageError()
		}
		if err := validateCapacityTable(ctx, tx, 3); err != nil {
			return err
		}
		if err := validateForeignKeys(ctx, tx); err != nil {
			return err
		}
		return initializeRoutingTx(ctx, tx)
	})
}

// migrateSharedCapacity changes only the claim resource key. It does not recover
// attempts, normalize JSON, release claims, or infer process cleanup.
func (s *Store) migrateSharedCapacity(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var version, appID int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return storageError()
		}
		if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil || version != 2 || appID != applicationID {
			return storageError()
		}
		if err := validateExecutionTables(ctx, tx); err != nil {
			return err
		}
		if err := validateCapacityTable(ctx, tx, 2); err != nil {
			return err
		}
		if err := validateForeignKeys(ctx, tx); err != nil {
			return err
		}
		var oldCount int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&oldCount); err != nil {
			return storageError()
		}
		for _, statement := range []string{
			"CREATE TABLE capacity_claims_v3(resource_id TEXT PRIMARY KEY CHECK(resource_id='managed-native'),attempt_id TEXT UNIQUE NOT NULL REFERENCES attempts(id))",
			"INSERT INTO capacity_claims_v3(resource_id,attempt_id) SELECT 'managed-native',attempt_id FROM capacity_claims",
			"DROP TABLE capacity_claims",
			"ALTER TABLE capacity_claims_v3 RENAME TO capacity_claims",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return storageError()
			}
		}
		var newCount int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&newCount); err != nil || newCount != oldCount {
			return storageError()
		}
		if err := validateCapacityTable(ctx, tx, 3); err != nil {
			return err
		}
		if err := validateForeignKeys(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=3"); err != nil {
			return storageError()
		}
		return initializeRoutingTx(ctx, tx)
	})
}

func validateExecutionTables(ctx context.Context, q schemaQueryer) error {
	for _, statement := range []string{
		"SELECT sequence,id,project_id,revision,state_revision,current_version,data FROM tasks LIMIT 0",
		"SELECT task_id,version,data FROM task_versions LIMIT 0",
		"SELECT sequence,id,task_id,task_version,revision,phase,data FROM attempts LIMIT 0",
		"SELECT attempt_id,data FROM attempt_terminals LIMIT 0",
		"SELECT sequence,attempt_id,data FROM attempt_events LIMIT 0",
		"SELECT sequence,id,attempt_id,private_ref,physical_identity,byte_length,data FROM artifacts LIMIT 0",
		"SELECT id,attempt_id,manifest_artifact_id,index_artifact_id,data FROM source_snapshots LIMIT 0",
		"SELECT id,attempt_id,source_snapshot_id,manifest_artifact_id,delta_artifact_id,workspace_ref,data FROM candidates LIMIT 0",
		"SELECT id,task_id,task_version,attempt_id,response_artifact_id,data FROM response_acceptances LIMIT 0",
		"SELECT id,attempt_id,data FROM execution_incidents LIMIT 0",
		"SELECT singleton,state,owner_instance_id,incident_id FROM execution_control LIMIT 0",
	} {
		rows, err := q.QueryContext(ctx, statement)
		if err != nil {
			return storageError()
		}
		if err = rows.Close(); err != nil {
			return storageError()
		}
	}
	return nil
}

// compactDDL ignores only formatting outside string literals. Removing whitespace
// inside a CHECK literal could make an incompatible constraint look equivalent.
func compactDDL(value string) string {
	var out strings.Builder
	quoted := false
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b == '\'' {
			out.WriteByte(b)
			if quoted && i+1 < len(value) && value[i+1] == '\'' {
				i++
				out.WriteByte(value[i])
				continue
			}
			quoted = !quoted
			continue
		}
		if !quoted && (b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == ';') {
			continue
		}
		if !quoted && b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		out.WriteByte(b)
	}
	return out.String()
}

func validateCapacityTable(ctx context.Context, q schemaQueryer, version int) error {
	want, column, key := claimTableV3, "resource_id", "managed-native"
	if version == 2 {
		want, column, key = claimTableV2, "connection_id", "codex-native"
	} else if version != 3 {
		return storageError()
	}
	var actual string
	if err := q.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE type='table' AND name='capacity_claims'").Scan(&actual); err != nil {
		return storageError()
	}
	got, expected := compactDDL(actual), compactDDL(want)
	// SQLite quotes the table name when ALTER TABLE RENAME rewrites stored SQL.
	quoted := strings.Replace(expected, "createtablecapacity_claims(", "createtable\"capacity_claims\"(", 1)
	if got != expected && got != quoted {
		return storageError()
	}
	var extras, count, invalid int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE tbl_name='capacity_claims' AND type IN ('index','trigger') AND sql IS NOT NULL").Scan(&extras); err != nil || extras != 0 {
		return storageError()
	}
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims").Scan(&count); err != nil || count > 1 {
		return storageError()
	}
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims c LEFT JOIN attempts a ON a.id=c.attempt_id WHERE c."+column+" IS NOT ? OR a.id IS NULL", key).Scan(&invalid); err != nil || invalid != 0 {
		return storageError()
	}
	return nil
}

func validateForeignKeys(ctx context.Context, q schemaQueryer) error {
	rows, err := q.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return storageError()
	}
	defer rows.Close()
	if rows.Next() || rows.Err() != nil {
		return storageError()
	}
	return nil
}
