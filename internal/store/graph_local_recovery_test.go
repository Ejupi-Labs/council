package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Actual Store creation, root completion, receiver registration, claim and
// recovery transactions. Existing controlled DAO metadata proofs do not claim
// filesystem composition, native execution or verification of user bytes.
func graphLocalRecoveryFixture(t *testing.T, kind string) (*Store, domain.GraphAdmission, domain.Attempt, string) {
	t.Helper()
	ctx := context.Background()
	if kind == "root_basis" {
		s, graph, _, owner := graphLocalFixture(t)
		a, err := s.Attempt(ctx, graph.PreparationAttemptID)
		if err != nil {
			t.Fatal(err)
		}
		return s, graph, a, owner
	}
	s, graph, root, proofs, owner := graphReadyBasis(t)
	op, err := s.BeginGraphInput(ctx, owner, graph.Graph.ID, graph.Nodes[0].ID, graphBeginFacts(graph, graph.Nodes[0], root, proofs, owner))
	if err != nil || op.Kind != "node_input" {
		t.Fatal("The actual independent node-input operation was not registered", op, err)
	}
	return s, graph, op.Attempt, owner
}

func graphLocalRecoveryPreserved(t *testing.T, before, after map[string][]string) {
	t.Helper()
	// Recovery may settle attempts and change the mutation owner. It cannot edit
	// captured instructions, roots, source/artifact identities, receipts, grants,
	// delivery evidence or already registered native authority.
	for _, table := range []string{"projects", "task_versions", "graph_versions", "artifacts", "source_snapshots", "candidates", "response_acceptances", "workflow_acceptances", "task_budgets", "budget_ledger", "graph_dispatches", "graph_admission_observations", "graph_delivery_bindings", "receipts"} {
		if !reflect.DeepEqual(before[table], after[table]) {
			t.Error("Recovery changed retained authority, source history or a saved receipt", table)
		}
	}
	if len(before["attempts"]) != len(after["attempts"]) || len(before["graph_inputs"]) != len(after["graph_inputs"]) {
		t.Error("Recovery dispatched retained work or allocated another input")
	}
}

func TestGraphLocalRecoveryDistinguishesNeverClaimedFromPreparingEffects(t *testing.T) {
	for _, kind := range []string{"root_basis", "node_input"} {
		for _, claimed := range []bool{false, true} {
			state := "never_claimed"
			if claimed {
				state = "claimed_preparing"
			}
			t.Run(kind+"/"+state, func(t *testing.T) {
				s, graph, a, owner := graphLocalRecoveryFixture(t, kind)
				ctx := context.Background()
				if a.Phase != "queued" || a.ExecutorKind != "local_graph" || a.TaskID != graph.Graph.RootTaskID || a.LaunchIntent || a.SourceSnapshotID != nil {
					t.Fatal("The fixture does not contain a genuine unclaimed root-owned local operation", a)
				}
				member, err := s.graphMembership(ctx, s.db, a.TaskID)
				if err != nil || member == nil || member.Kind != "root" || member.GraphID != graph.Graph.ID {
					t.Fatal("The local operation is not an actual graph-root member", member, err)
				}
				if err := s.CheckGraphLocalOperation(ctx, owner, a.ID); err == nil {
					t.Fatal("The queued operation could already perform local I/O")
				}
				if claimed {
					a, err = s.ClaimAttempt(ctx, a.ID, owner)
					if err != nil || a.Phase != "preparing" || a.LaunchIntent || a.SourceSnapshotID != nil {
						t.Fatal("The uncertain-effects control did not enter actual preparation", a, err)
					}
					if err := s.CheckGraphLocalOperation(ctx, owner, a.ID); err != nil {
						t.Fatal("The claimed control was not actually authorized to begin local effects", err)
					}
				}
				claimsBefore, err := s.GraphClaims(ctx)
				if err != nil || len(claimsBefore) != boolCount(claimed) {
					t.Fatal("The queued/claimed boundary has the wrong actual ownership", claimsBefore, err)
				}
				var preparationEvents int
				if err := s.db.QueryRow("SELECT count(*) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='preparation_started'", a.ID).Scan(&preparationEvents); err != nil || preparationEvents != boolCount(claimed) {
					t.Fatal("The durable preparation history does not match the real claim boundary", preparationEvents, err)
				}
				budgetBefore, err := s.GraphBudget(ctx, graph.Graph.ID)
				if err != nil || budgetBefore.Budget.ConsumedAdmissions != "0" || budgetBefore.Budget.CurrentWindow.DeadlineAt != nil {
					t.Fatal("The local-only fixture consumed native allowance", budgetBefore, err)
				}
				beforeRows := graphInitialAncestorRows(t, s)
				oldTerminals := map[string]string{}
				rows, err := s.db.Query("SELECT attempt_id,data FROM attempt_terminals")
				if err != nil {
					t.Fatal(err)
				}
				for rows.Next() {
					var id, raw string
					if err := rows.Scan(&id, &raw); err != nil {
						rows.Close()
						t.Fatal(err)
					}
					oldTerminals[id] = raw
				}
				if err := rows.Err(); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				rows.Close()
				newOwner := domain.NewID()
				if err := s.RecoverOwner(ctx, newOwner); err != nil {
					t.Fatal("Actual owner recovery failed", err)
				}
				settled, err := s.Attempt(ctx, a.ID)
				if err != nil || settled.Phase != "settled" || settled.Outcome == nil || *settled.Outcome != "interrupted" || settled.Cleanup.State != "verified" || settled.Cleanup.Scope != "not_launched" || settled.EvidenceStatus != "incomplete" || settled.LaunchIntent || settled.SourceSnapshotID != nil || settled.OwnerInstanceID != owner {
					t.Fatal("Recovery rewrote the old owner or invented native execution/success", settled, err)
				}
				gate, err := s.Gate(ctx)
				if err != nil || gate.OwnerInstanceID != newOwner {
					t.Fatal("Recovery did not install the exact new mutation owner", gate, err)
				}
				claims, err := s.GraphClaims(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if claimed {
					if gate.State != "fenced" || gate.IncidentID == nil || settled.Cleanup.IncidentID == nil || !reflect.DeepEqual(claimsBefore, claims) {
						t.Fatal("Claimed local effects lost their uncertainty fence or exact prior ownership", settled, gate, claims, err)
					}
					if err := s.checkAttemptClaim(ctx, s.db, owner, a.ID); err != nil {
						t.Fatal("Recovery released an actual potentially effectful local claim", err)
					}
				} else if gate.State != "open" || gate.IncidentID != nil || settled.Cleanup.IncidentID != nil || len(claims) != 0 {
					t.Fatal("Never-claimed queued local work invented unknown effects or a global recovery fence", settled, gate, claims)
				}
				if err := s.CheckGraphLocalOperation(ctx, owner, a.ID); err == nil {
					t.Fatal("The old owner retained permission to execute recovered work")
				}
				var terminal string
				if err := s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", a.ID).Scan(&terminal); err != nil {
					t.Fatal(err)
				}
				recorded, err := decode[struct {
					Attempt    domain.Attempt    `json:"attempt"`
					Settlement domain.Settlement `json:"settlement"`
				}](terminal)
				if err != nil || recorded.Settlement.EffectsKnown != !claimed {
					t.Error("Recovery did not retain the actual known-zero versus potential-effects distinction", recorded, err)
				}
				for id, raw := range oldTerminals {
					var retained string
					if err := s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", id).Scan(&retained); err != nil || raw != retained {
						t.Error("Recovery rewrote an existing original terminal", id, err)
					}
				}
				graphLocalRecoveryPreserved(t, beforeRows, graphInitialAncestorRows(t, s))
				budgetAfter, err := s.GraphBudget(ctx, graph.Graph.ID)
				if err != nil || !reflect.DeepEqual(budgetBefore, budgetAfter) {
					t.Fatal("Recovery refilled, opened or debited the common native allowance", budgetAfter, err)
				}
				if err := s.RecoverOwner(ctx, domain.NewID()); err != nil {
					t.Fatal(err)
				}
				var repeated string
				if err := s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", a.ID).Scan(&repeated); err != nil || terminal != repeated {
					t.Fatal("A second owner recovery replayed or rewrote the interrupted operation", err)
				}
				graphLocalRecoveryPreserved(t, beforeRows, graphInitialAncestorRows(t, s))
			})
		}
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
