package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
)

func (s *Store) migrateWorkflow(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var version, appID int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return storageError()
		}
		if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil || version != 4 || appID != applicationID {
			return storageError()
		}
		return initializeWorkflowTx(ctx, tx)
	})
}
func initializeWorkflowTx(ctx context.Context, tx *sql.Tx) error {
	if err := validateKnownRoutingDDL(ctx, tx, true); err != nil {
		return err
	}
	if err := validateRoutingSeedCounts(ctx, tx); err != nil {
		return err
	}
	if err := validateForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, workflowSchema); err != nil {
		return storageError()
	}
	pref := domain.DeliveryPreference{LocalMode: "review_only", SelectionSource: "default", Revision: "1", UpdatedAt: domain.Now()}
	if _, err := tx.ExecContext(ctx, "INSERT INTO delivery_preferences VALUES(1,1,?)", encode(pref)); err != nil {
		return storageError()
	}
	if err := validateKnownWorkflowDDL(ctx, tx); err != nil {
		return err
	}
	if err := validateForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version=5"); err != nil {
		return storageError()
	}
	return nil
}
func expectedWorkflowDDL() map[string]string {
	out := expectedRoutingDDL(true)
	for _, statement := range strings.Split(workflowSchema, ";") {
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
		out[name] = compactDDL(statement)
	}
	return out
}
func validateKnownWorkflowDDL(ctx context.Context, q schemaQueryer) error {
	expected := expectedWorkflowDDL()
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
		if !ok || (got != want && got != quoted) {
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
func validateRoutingSeedCounts(ctx context.Context, q queryer) error {
	for _, entry := range []struct {
		table string
		want  int
	}{{"routing_contexts", 2}, {"routing_profiles", 6}, {"routing_policy", 1}} {
		var count int
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM "+entry.table).Scan(&count); err != nil || count != entry.want {
			return storageError()
		}
	}
	return nil
}
func (s *Store) validateWorkflowSchema(ctx context.Context) error {
	if err := validateKnownWorkflowDDL(ctx, s.db); err != nil {
		return err
	}
	if err := validateRoutingSeedCounts(ctx, s.db); err != nil {
		return err
	}
	if _, err := s.deliveryPreference(ctx, s.db); err != nil {
		return err
	}
	return validateForeignKeys(ctx, s.db)
}
