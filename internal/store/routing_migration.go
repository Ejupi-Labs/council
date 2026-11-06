package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

// migrateRouting adds only schema4 state; retained legacy task/receipt JSON and
// claimed ownership are deliberately not read-modify-written.
func (s *Store) migrateRouting(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var version, appID int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return storageError()
		}
		if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil || version != 3 || appID != applicationID {
			return storageError()
		}
		return initializeRoutingTx(ctx, tx)
	})
}

func initializeRoutingTx(ctx context.Context, tx *sql.Tx) error {
	if err := validateKnownRoutingDDL(ctx, tx, false); err != nil {
		return err
	}
	if err := validateCapacityTable(ctx, tx, 3); err != nil {
		return err
	}
	if err := validateForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, routingSchema); err != nil {
		return storageError()
	}
	now := domain.Now()
	policy := routing.DefaultPolicy()
	policy.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, "INSERT INTO routing_policy VALUES(1,1,?)", encode(policy)); err != nil {
		return storageError()
	}
	for _, native := range routing.Contexts() {
		native.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, "INSERT INTO routing_contexts VALUES(?,?,1,1,?)", native.ID, native.HarnessID, encode(native)); err != nil {
			return storageError()
		}
		link := domain.AccountLink{ContextID: native.ID, Revision: "1", UpdatedAt: now, Source: "user_declaration"}
		if _, err := tx.ExecContext(ctx, "INSERT INTO routing_account_links VALUES(?,NULL,1,?)", native.ID, encode(link)); err != nil {
			return storageError()
		}
		state := domain.CapacityState{ScopeKey: "context:" + native.ID, Subject: domain.CapacitySubject{Kind: "context", ID: native.ID}, Revision: "1", RetainedNegatives: []domain.RetainedCapacityReport{}}
		if _, err := tx.ExecContext(ctx, "INSERT INTO routing_capacity_state VALUES(?,?,NULL,1,?)", state.ScopeKey, native.ID, encode(state)); err != nil {
			return storageError()
		}
	}
	for _, profile := range routing.Catalog() {
		profile.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, "INSERT INTO routing_profiles VALUES(?,?,1,1,?)", profile.ID, profile.ContextID, encode(profile)); err != nil {
			return storageError()
		}
	}
	if err := validateKnownRoutingDDL(ctx, tx, true); err != nil {
		return err
	}
	if err := validateForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
		return storageError()
	}
	return nil
}

// expectedDDL uses only authored constant SQL, never caller text. Existing
// SQLite-generated autoindexes are excluded from user-object comparison.
func expectedRoutingDDL(includeRouting bool) map[string]string {
	out := map[string]string{}
	all := schema + "\n" + executionSchema
	if includeRouting {
		all += "\n" + routingSchema
	}
	for _, statement := range strings.Split(all, ";") {
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
func validateKnownRoutingDDL(ctx context.Context, q schemaQueryer, includeRouting bool) error {
	expected := expectedRoutingDDL(includeRouting)
	rows, err := q.QueryContext(ctx, "SELECT name,sql FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*' AND type IN ('table','index','trigger','view')")
	if err != nil {
		return storageError()
	}
	seen := map[string]bool{}
	for rows.Next() {
		var name, statement string
		if err := rows.Scan(&name, &statement); err != nil {
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
func (s *Store) validateRoutingSchema(ctx context.Context) error {
	if err := validateKnownRoutingDDL(ctx, s.db, true); err != nil {
		return err
	}
	var contexts, profiles, policy int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM routing_contexts").Scan(&contexts); err != nil || contexts != 2 {
		return storageError()
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM routing_profiles").Scan(&profiles); err != nil || profiles != 6 {
		return storageError()
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM routing_policy WHERE singleton=1").Scan(&policy); err != nil || policy != 1 {
		return storageError()
	}
	return validateForeignKeys(ctx, s.db)
}
