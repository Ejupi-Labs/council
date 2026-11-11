package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) migrateGraphs(ctx context.Context) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		var version, appID int
		if e := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
			return storageError()
		}
		if e := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); e != nil || version != 5 || appID != applicationID {
			return storageError()
		}
		if e := validateKnownWorkflowDDL(ctx, tx); e != nil {
			return e
		}
		if e := validateRoutingSeedCounts(ctx, tx); e != nil {
			return e
		}
		if e := validateForeignKeys(ctx, tx); e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, "SELECT attempt_id FROM capacity_claims ORDER BY resource_id")
		if e != nil {
			return storageError()
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return storageError()
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil || len(ids) > 1 {
			return storageError()
		}
		legacy := []graphClaimRecord{}
		for _, id := range ids {
			a, e := s.attempt(ctx, tx, id)
			if e != nil {
				return e
			}
			kind := attemptExecutor(a)
			view := domain.GraphClaimView{Slot: "1", AttemptID: id, ExecutorKind: kind, LegacyBarrier: true}
			if kind == "native" {
				if domain.ValidContextID(a.ConnectionID) {
					view.ContextID = ptr(a.ConnectionID)
				}
				if view.ContextID != nil && a.ResolvedTarget != nil && a.ResolvedTarget.DeclaredAccountAssociation != nil {
					account := a.ResolvedTarget.DeclaredAccountAssociation
					view.CapturedDeclaredAccountID, view.CapturedLinkRevision = ptr(account.AccountID), ptr(account.LinkRevision)
				}
			}
			if domain.ValidateGraphClaims([]domain.GraphClaimView{view}) != nil || a.OwnerInstanceID == "" {
				return storageError()
			}
			legacy = append(legacy, graphClaimRecord{View: view, OwnerInstanceID: a.OwnerInstanceID, CapturedMaxParallelism: "1", CreatedAt: domain.Now()})
		}
		if _, e = tx.ExecContext(ctx, graphSchema); e != nil {
			return storageError()
		}
		if _, e = tx.ExecContext(ctx, "DROP TABLE capacity_claims"); e != nil {
			return storageError()
		}
		if _, e = tx.ExecContext(ctx, graphCapacitySchema); e != nil {
			return storageError()
		}
		for _, r := range legacy {
			var link any
			if r.View.CapturedLinkRevision != nil {
				n, err := domain.ParseRevision(*r.View.CapturedLinkRevision)
				if err != nil {
					return storageError()
				}
				link = n
			}
			if _, e = tx.ExecContext(ctx, "INSERT INTO capacity_claims(slot,attempt_id,owner_instance_id,context_id,captured_account_id,captured_link_revision,graph_id,legacy_barrier,data) VALUES(1,?,?,?,?,?,NULL,1,?)", r.View.AttemptID, r.OwnerInstanceID, r.View.ContextID, r.View.CapturedDeclaredAccountID, link, encode(r)); e != nil {
				return storageError()
			}
		}
		if e := s.validateGraphSchemaWith(ctx, tx); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "PRAGMA user_version=6"); e != nil {
			return storageError()
		}
		return nil
	})
}

func expectedGraphDDL() map[string]string {
	expected := expectedWorkflowDDL()
	delete(expected, "capacity_claims")
	for _, statement := range strings.Split(graphSchema+";"+graphCapacitySchema, ";") {
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

func validateKnownGraphDDL(ctx context.Context, q schemaQueryer) error {
	expected := expectedGraphDDL()
	rows, e := q.QueryContext(ctx, "SELECT name,sql FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*' AND type IN ('table','index','trigger','view')")
	if e != nil {
		return storageError()
	}
	seen := map[string]bool{}
	for rows.Next() {
		var name, statement string
		if e = rows.Scan(&name, &statement); e != nil {
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
	e = rows.Err()
	rows.Close()
	if e != nil || len(seen) != len(expected) {
		return storageError()
	}
	return nil
}

func (s *Store) validateGraphSchemaWith(ctx context.Context, q queryer) error {
	if e := validateKnownGraphDDL(ctx, q); e != nil {
		return e
	}
	if e := validateRoutingSeedCounts(ctx, q); e != nil {
		return e
	}
	if _, e := s.deliveryPreference(ctx, q); e != nil {
		return e
	}
	if e := validateForeignKeys(ctx, q); e != nil {
		return e
	}
	_, e := s.graphClaimRecords(ctx, q)
	return e
}
