package store

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

//go:embed testdata/schema4.sql
var acceptedWorkflowSchema4 string

// Frozen accepted004 DDL is populated directly, never through current Open or
// current task/default mutation behavior. Old metadata is explicitly unqualified.
func workflowSchema4Database(t *testing.T, databasePath, selected string, full bool) (*sql.DB, routingSchema3IDs) {
	t.Helper()
	db, ids := routingLegacyDatabase(t, databasePath, selected, full, acceptedWorkflowSchema4)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e != nil {
			t.Fatal(q, e)
		}
	}
	now := "2026-11-06T00:00:00.000Z"
	// The factory-looking two is deliberately indistinguishable from a user
	// selection. A future current DefaultPolicy must not change this fixture.
	policy := domain.RoutingPolicy{Revision: "1", EconomyOrder: []string{"codex-native-default", "codex-luna-economy", "codex-sol-general", "claude-native-default", "claude-haiku-economy", "claude-sonnet-general"}, UnknownAvailability: "allow", DefaultBudget: domain.RoutingBudgetDefaults{AdmissionLimit: "2", WindowSeconds: 1800}, Continuation: domain.ContinuationPolicy{MaxAutomaticAdmissions: "0", Categories: []string{}}, UpdatedAt: now}
	exec("INSERT INTO routing_policy VALUES(1,1,?)", encode(policy))
	// These fixed accepted004 context/catalog declarations carry no app version,
	// live account, quota or vendor-observed fact. No current policy is consulted.
	for _, native := range routing.Contexts() {
		native.UpdatedAt = now
		exec("INSERT INTO routing_contexts VALUES(?,?,1,1,?)", native.ID, native.HarnessID, encode(native))
		link := domain.AccountLink{ContextID: native.ID, Revision: "1", UpdatedAt: now, Source: "user_declaration"}
		exec("INSERT INTO routing_account_links VALUES(?,NULL,1,?)", native.ID, encode(link))
		state := domain.CapacityState{ScopeKey: "context:" + native.ID, Subject: domain.CapacitySubject{Kind: "context", ID: native.ID}, Revision: "1", RetainedNegatives: []domain.RetainedCapacityReport{}}
		exec("INSERT INTO routing_capacity_state VALUES(?,?,NULL,1,?)", state.ScopeKey, native.ID, encode(state))
	}
	for _, profile := range routing.Catalog() {
		profile.UpdatedAt = now
		exec("INSERT INTO routing_profiles VALUES(?,?,1,1,?)", profile.ID, profile.ContextID, encode(profile))
	}
	if full {
		deadline := "2026-11-06T00:30:00.000Z"
		budget := domain.TaskBudget{TaskID: ids.TaskID, Revision: "9007199254740993", GrantedAdmissions: "2", ConsumedAdmissions: "1", RemainingAdmissions: "1", AutomaticAdmissions: "0", HistoricalAdmissions: "1", CurrentWindow: domain.BudgetWindow{Number: "1", WindowSeconds: 1800, FirstAdmissionAt: &now, DeadlineAt: &deadline}, UpdatedAt: now}
		exec("INSERT INTO task_budgets VALUES(?,?,2,1,0,1,1,?,?,?)", ids.TaskID, int64(9007199254740993), now, deadline, encode(budget))
		entry := domain.BudgetLedgerEntry{ID: strings.Repeat("0", 32), Sequence: "1", TaskID: ids.TaskID, BudgetRevision: budget.Revision, Kind: "legacy_baseline", RequestID: ids.RequestID, GrantedDelta: "2", ConsumedDelta: "1", Window: budget.CurrentWindow, CreatedAt: now}
		exec("INSERT INTO budget_ledger VALUES(1,?,?,?,'legacy_baseline',NULL,?)", entry.ID, ids.TaskID, int64(9007199254740993), encode(entry))
		// Actual retained bytes are compared around migration. Their labelled
		// metadata is never represented as a live qualified provider result.
		artifactID := strings.Repeat("0a", 16)
		body := []byte("\ufeff Retained accepted004 café \ufffd\r\n東京\n")
		artifactPath := filepath.Join(filepath.Dir(databasePath), "artifacts", artifactID+".bin")
		if e := os.MkdirAll(filepath.Dir(artifactPath), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(artifactPath, body, 0600); e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])
		metadata := domain.ArtifactMetadata{ID: artifactID, AttemptID: ids.AttemptID, Kind: "final_response", ByteLength: domain.Decimal(int64(len(body))), Closed: true, Completeness: "complete", SHA256: &hash, CreatedAt: now}
		exec("INSERT INTO artifacts VALUES(6,?,?,?,?,?,?)", artifactID, ids.AttemptID, "artifacts/"+artifactID+".bin", "labelled-accepted004-fixture", len(body), encode(metadata))
	}
	return db, ids
}

// Explicit disposable-home seed for independent browser/CLI migration journeys.
func TestWorkflowSchema4SeedChild(t *testing.T) {
	databasePath := os.Getenv("COUNCIL_WORKFLOW_SCHEMA4_SEED_PATH")
	if databasePath == "" {
		t.Skip("explicit disposable schema4 seed helper")
	}
	selected := os.Getenv("COUNCIL_WORKFLOW_SCHEMA4_PROJECT")
	output := os.Getenv("COUNCIL_WORKFLOW_SCHEMA4_SEED_OUTPUT")
	if !filepath.IsAbs(databasePath) || filepath.Base(databasePath) != "workspace.sqlite3" || !filepath.IsAbs(selected) || !filepath.IsAbs(output) {
		t.Fatal("absolute disposable database/project/output paths required")
	}
	if info, e := os.Stat(databasePath); e == nil && info.Size() != 0 {
		t.Fatal("refusing existing nonempty database")
	}
	lock, e := localstate.Acquire(filepath.Dir(databasePath))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = lock.Close() })
	db, ids := workflowSchema4Database(t, databasePath, selected, false)
	cleanSyntheticBrowserEpoch(t, databasePath, db, ids, 4)
	b, e := json.Marshal(ids)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(output, b, 0600); e != nil {
		t.Fatal(e)
	}
}
