package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"reflect"
	"strings"
	"testing"
)

//go:embed testdata/schema5.sql
var acceptedGraphSchema5 string

//go:embed testdata/schema5.json
var acceptedGraphSchema5JSON []byte

type graphSchema5Golden struct {
	DeliveryPreference jsontext.Value `json:"deliveryPreference"`
	Authority          jsontext.Value `json:"authority"`
	Workflow           jsontext.Value `json:"workflow"`
	WorkflowReceipt    jsontext.Value `json:"workflowReceipt"`
	RoutingReceipt     jsontext.Value `json:"routingReceipt"`
}

type graphSchema5IDs struct {
	routingSchema3IDs
	WorkflowID        string
	WorkflowRequestID string
	RoutingRequestID  string
}

// graphSchema5Database uses frozen accepted004 population and frozen accepted005
// DDL, never current Open/initializeSchema/migration/default behavior. All records
// are explicitly local metadata fixtures, not provider or filesystem proof.
func graphSchema5Database(t *testing.T, path string, populated bool) (*sql.DB, graphSchema5IDs) {
	t.Helper()
	db, legacy := workflowSchema4Database(t, path, "", populated)
	parts := strings.Split(acceptedGraphSchema5, "-- Accepted005 workflow schema addition\n")
	if len(parts) != 2 {
		t.Fatal("frozen accepted005 schema appendix missing")
	}
	if _, err := db.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := db.Exec(statement, args...); err != nil {
			t.Fatal(statement, err)
		}
	}
	var golden graphSchema5Golden
	if err := json.Unmarshal(acceptedGraphSchema5JSON, &golden, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal("frozen005 JSON fixture invalid", err)
	}
	exec("INSERT INTO delivery_preferences VALUES(1,1,?)", string(golden.DeliveryPreference))
	ids := graphSchema5IDs{routingSchema3IDs: legacy, WorkflowID: strings.Repeat("0b", 16), WorkflowRequestID: strings.Repeat("0c", 16), RoutingRequestID: strings.Repeat("0d", 16)}
	if populated {
		// These exact accepted005 bytes were captured before006 DTO changes.
		// Current marshaling/defaults must never reconstruct historical fixtures.
		exec("INSERT INTO workflows VALUES(?,?,1,1,1,'blocked',?,?)", ids.WorkflowID, ids.TaskID, string(golden.Authority), string(golden.Workflow))
		exec("INSERT INTO receipts VALUES(?,?,?,200,?)", ids.WorkflowRequestID, "tasks.workflow.adopt", "frozen005-workflow-input", string(golden.WorkflowReceipt))
		exec("INSERT INTO receipts VALUES(?,?,?,200,?)", ids.RoutingRequestID, "routing.policy.update", "frozen005-routing-input", string(golden.RoutingReceipt))
	}
	if err := validateForeignKeys(context.Background(), db); err != nil {
		t.Fatal("historical fixture has invalid foreign keys", err)
	}
	return db, ids
}

// Excludes only the deliberately replaced singleton claim table and new graph
// tables. Every accepted foundation/routing/workflow row is compared byte-exact.
func graphSchema5Rows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	rows, err := db.Query("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT GLOB 'sqlite_*' AND name!='capacity_claims' AND name NOT IN ('task_graphs','graph_versions','graph_nodes','graph_inputs','graph_dispatches','graph_admission_observations','graph_delivery_bindings','recovery_owner_epoch','recovery_owner_epoch_events','recovery_checkpoints','recovery_graph_scopes','recovery_observations','recovery_decisions','history_snapshots','history_previews','history_export_operations','history_effect_journal','history_incident_bindings') ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, name := range names {
		r, err := db.Query(`SELECT * FROM "` + name + `" ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := r.Columns()
		if err != nil {
			r.Close()
			t.Fatal(err)
		}
		out[name] = []string{}
		for r.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(values))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := r.Scan(pointers...); err != nil {
				r.Close()
				t.Fatal(err)
			}
			encoded, err := json.Marshal(values)
			if err != nil {
				r.Close()
				t.Fatal(err)
			}
			out[name] = append(out[name], string(encoded))
		}
		err = r.Err()
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestGraphSchema5FrozenFixtureRetainsFiniteHistoryAndClaim(t *testing.T) {
	db, ids := graphSchema5Database(t, t.TempDir()+"/accepted5.sqlite3", true)
	var version, foreignKeys int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 5 {
		t.Fatal("fixture no longer represents accepted005", version, err)
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatal("fixture foreign keys disabled", foreignKeys, err)
	}
	var claim, owner, rawPolicy string
	if err := db.QueryRow("SELECT attempt_id FROM capacity_claims WHERE resource_id='managed-native'").Scan(&claim); err != nil || claim != ids.ActiveAttemptID {
		t.Fatal("fixture does not retain actual singleton claim", claim, err)
	}
	if err := db.QueryRow("SELECT owner_instance_id FROM execution_control").Scan(&owner); err != nil || owner != strings.Repeat("6", 32) {
		t.Fatal("fixture owner epoch changed", owner, err)
	}
	if err := db.QueryRow("SELECT data FROM routing_policy").Scan(&rawPolicy); err != nil || !strings.Contains(rawPolicy, `"admissionLimit":"2"`) {
		t.Fatal("accepted saved finite default was replaced", rawPolicy, err)
	}
	before := graphSchema5Rows(t, db)
	for _, table := range []string{"tasks", "task_versions", "receipts", "workflows"} {
		for _, raw := range before[table] {
			for _, forbidden := range []string{"\\\"graphRef\\\"", "\\\"graphBudgetRef\\\"", "\\\"graphConditionsRef\\\""} {
				if strings.Contains(raw, forbidden) {
					t.Fatal("historical fixture acquired006 projection fields", table, forbidden)
				}
			}
		}
	}
	if len(before["workflows"]) != 1 || len(before["receipts"]) != 3 || len(before["budget_ledger"]) != 1 {
		t.Fatal("fixture lacks populated005 workflow/routing/admission history")
	}
	if !reflect.DeepEqual(before, graphSchema5Rows(t, db)) {
		t.Fatal("read-only fixture observation changed historical bytes")
	}
}
