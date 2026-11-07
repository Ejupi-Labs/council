package store

import (
	"context"
	"database/sql"
	"reflect"
	"slices"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are actual Store creation/admission/claim/recovery transactions with
// registered metadata proofs. They qualify projection and retained ownership,
// not filesystem preparation, OS cleanup or native model execution.
func graphRecoveryProjectionNode(t *testing.T, graph domain.GraphDetail, id string) domain.GraphNodeSummary {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatal("The exact graph node is absent", id)
	return domain.GraphNodeSummary{}
}

func graphRecoveryProjectionPreserved(t *testing.T, s *Store, before domain.GraphDetail, rows map[string][]string, changedNodeID string) {
	t.Helper()
	afterRows := graphInitialAncestorRows(t, s)
	graphLocalRecoveryPreserved(t, rows, afterRows)
	for _, terminal := range rows["attempt_terminals"] {
		if !slices.Contains(afterRows["attempt_terminals"], terminal) {
			t.Error("Terminal projection rewrote an existing exact terminal")
		}
	}
	after, err := s.Graph(context.Background(), before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Graph.ID != before.Graph.ID || after.Graph.GraphVersion != before.Graph.GraphVersion || after.Graph.ControlRevision != before.Graph.ControlRevision ||
		after.Graph.BudgetAuthorityEpoch != before.Graph.BudgetAuthorityEpoch || after.Graph.MaxParallelism != before.Graph.MaxParallelism ||
		!reflect.DeepEqual(after.Graph.RootInputID, before.Graph.RootInputID) || !reflect.DeepEqual(after.Graph.FinalNodeID, before.Graph.FinalNodeID) ||
		!reflect.DeepEqual(after.Graph.Stop, before.Graph.Stop) || !reflect.DeepEqual(after.Budget, before.Budget) {
		t.Error("Terminal observation changed graph control, root, final selection or shared allowance", before.Graph, after.Graph)
	}
	for _, old := range before.Nodes {
		current := graphRecoveryProjectionNode(t, after, old.ID)
		if current.ID != changedNodeID {
			if !reflect.DeepEqual(old, current) {
				t.Error("Terminal projection rewrote an independent or accepted sibling", old, current)
			}
			continue
		}
		if current.TaskID != old.TaskID || current.TaskVersion != old.TaskVersion || current.ControlRevision != old.ControlRevision ||
			!reflect.DeepEqual(current.InputID, old.InputID) || !reflect.DeepEqual(current.NodeAdmissionID, old.NodeAdmissionID) ||
			!reflect.DeepEqual(current.AcceptedResult, old.AcceptedResult) || !reflect.DeepEqual(current.Stop, old.Stop) {
			t.Error("Terminal projection replaced captured node authority or accepted history", old, current)
		}
	}
}

func TestGraphRecoveryProjectsRootAndInputTerminalState(t *testing.T) {
	for _, kind := range []string{"root_basis", "node_input"} {
		for _, claimed := range []bool{false, true} {
			name := "never_claimed"
			if claimed {
				name = "claimed_preparing"
			}
			t.Run(kind+"/"+name, func(t *testing.T) {
				s, admission, a, owner := graphLocalRecoveryFixture(t, kind)
				ctx := context.Background()
				op, err := s.GraphInputOperationForAttempt(ctx, owner, a.ID)
				if err != nil || a.Phase != "queued" || op.InputRecord.Public.Summary.State != "preparing" || op.Attempt.WorkflowID != "" {
					t.Fatal("The real root-owned local operation is not pending preparation", op, err)
				}
				if claimed {
					a, err = s.ClaimAttempt(ctx, a.ID, owner)
					if err != nil || a.Phase != "preparing" {
						t.Fatal("The local uncertainty control never acquired its real claim", a, err)
					}
					if err := s.CheckGraphLocalOperation(ctx, owner, a.ID); err != nil {
						t.Fatal("The claimed control could not enter actual local I/O", err)
					}
				}
				before, err := s.Graph(ctx, admission.Graph.ID)
				if err != nil || before.Graph.State != "preparing" {
					t.Fatal("The local fixture lacks a live preparing graph projection", before, err)
				}
				rows := graphInitialAncestorRows(t, s)
				claimsBefore, err := s.GraphClaims(ctx)
				if err != nil || len(claimsBefore) != boolCount(claimed) {
					t.Fatal(claimsBefore, err)
				}
				newOwner := domain.NewID()
				if err := s.RecoverOwner(ctx, newOwner); err != nil {
					t.Fatal(err)
				}
				settled, err := s.Attempt(ctx, a.ID)
				if err != nil || settled.Phase != "settled" || settled.Outcome == nil || *settled.Outcome != "interrupted" || settled.OwnerInstanceID != owner {
					t.Fatal("Recovery did not record the actual interrupted old-owner operation", settled, err)
				}
				input, err := s.GraphInput(ctx, admission.Graph.ID, op.ID)
				if err != nil || input.Summary.State != "interrupted" || input.Summary.Current || input.InputBinding != nil || input.RootBinding != nil {
					t.Error("Interrupted local preparation still advertises a current preparing/usable input", input, err)
				}
				after, err := s.Graph(ctx, admission.Graph.ID)
				if err != nil || after.Graph.State != "interrupted" || after.Graph.NextOwner != "user" {
					t.Error("Recovered graph still advertises active Council-owned preparation", after, err)
				}
				changedNode := ""
				if op.Control.NodeID != nil {
					changedNode = *op.Control.NodeID
					node := graphRecoveryProjectionNode(t, after, changedNode)
					if node.State != "interrupted" || node.NextOwner != "user" || after.Graph.Counts.Interrupted != "1" || after.Graph.Counts.Preparing != "0" {
						t.Error("Recovered receiving node/counts still promise active preparation", node, after.Graph.Counts)
					}
				}
				gate, err := s.Gate(ctx)
				claimsAfter, claimErr := s.GraphClaims(ctx)
				if err != nil || claimErr != nil || gate.OwnerInstanceID != newOwner {
					t.Fatal(gate, err, claimErr)
				}
				if claimed {
					if gate.State != "fenced" || gate.IncidentID == nil || !reflect.DeepEqual(claimsBefore, claimsAfter) {
						t.Error("Projection released or hid actual potentially effectful ownership", gate, claimsAfter)
					}
				} else if gate.State != "open" || gate.IncidentID != nil || len(claimsAfter) != 0 {
					t.Error("Known-zero recovery invented uncertainty or retained capacity", gate, claimsAfter)
				}
				graphRecoveryProjectionPreserved(t, s, before, rows, changedNode)
				firstRows := graphInitialAncestorRows(t, s)
				if err := s.RecoverOwner(ctx, domain.NewID()); err != nil {
					t.Fatal(err)
				}
				secondRows := graphInitialAncestorRows(t, s)
				for _, table := range []string{"task_graphs", "graph_nodes", "graph_inputs", "attempt_terminals", "attempts", "capacity_claims", "budget_ledger"} {
					if !reflect.DeepEqual(firstRows[table], secondRows[table]) {
						t.Error("A second recovery repainted or replayed settled graph history", table)
					}
				}
			})
		}
	}
}

func graphRecoveryProjectionNativeFixture(t *testing.T) (*Store, domain.GraphDispatchView, string) {
	t.Helper()
	s, op, completion, owner := graphDependentCompletionFixture(t, "")
	ctx := context.Background()
	if err := s.CompleteGraphInput(ctx, owner, op.Attempt.ID, completion); err != nil {
		t.Fatal(err)
	}
	facts, trigger := graphNodeFactsForInput(t, s, op, completion)
	d, err := s.AdmitGraphNode(ctx, owner, op.GraphID, trigger, facts)
	if err != nil || d.Dispatch.ID == "" {
		t.Fatal("The actual dependent native dispatch was not admitted", d, err)
	}
	view, err := s.Graph(ctx, op.GraphID)
	if err != nil || view.Nodes[0].AcceptedResult == nil || view.Graph.Counts.Completed != "1" || view.Budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("The fixture lacks actual accepted A and admitted B under the shared allowance", view, err)
	}
	return s, d, owner
}

func TestGraphRecoveryProjectsCurrentNativeWithoutRewritingAcceptedSibling(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		name := "queued_reserved"
		if claimed {
			name = "claimed_preparing"
		}
		t.Run(name, func(t *testing.T) {
			s, d, owner := graphRecoveryProjectionNativeFixture(t)
			ctx := context.Background()
			a, err := s.Attempt(ctx, d.Dispatch.AttemptID)
			if err != nil || a.Phase != "queued" || a.LaunchIntent || a.SourceSnapshotID != nil {
				t.Fatal("The native fixture crossed the actual queued boundary", a, err)
			}
			if claimed {
				a, err = s.ClaimAttempt(ctx, a.ID, owner)
				if err != nil || a.Phase != "preparing" {
					t.Fatal(a, err)
				}
				if err := s.CheckGraphDispatchPreparation(ctx, owner, a.ID); err != nil {
					t.Fatal("The claimed native control could not enter preparation", err)
				}
			}
			before, err := s.Graph(ctx, d.Dispatch.GraphID)
			if err != nil || before.ActiveClaims == nil || len(before.ActiveClaims) != 1 {
				t.Fatal("The actual native reservation is absent", before, err)
			}
			rows := graphInitialAncestorRows(t, s)
			if err := s.RecoverOwner(ctx, domain.NewID()); err != nil {
				t.Fatal(err)
			}
			settled, err := s.Attempt(ctx, a.ID)
			workflow, workflowErr := s.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
			if err != nil || workflowErr != nil || settled.Outcome == nil || *settled.Outcome != "interrupted" || workflow.Workflow == nil || workflow.Workflow.State != "interrupted" {
				t.Fatal("Actual recovery did not settle the exact current workflow", settled, workflow, err, workflowErr)
			}
			after, err := s.Graph(ctx, d.Dispatch.GraphID)
			if err != nil {
				t.Fatal(err)
			}
			node := graphRecoveryProjectionNode(t, after, d.Dispatch.NodeID)
			if node.State != "interrupted" || node.NextOwner != "user" || after.Graph.State != "interrupted" || after.Graph.NextOwner != "user" ||
				after.Graph.Counts.Interrupted != "1" || after.Graph.Counts.Completed != "1" || after.Graph.Counts.Queued != "0" || after.Graph.Counts.Preparing != "0" {
				t.Error("Interrupted native workflow left an active graph/node/count projection", node, after.Graph)
			}
			gate, err := s.Gate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if claimed {
				if gate.State != "fenced" || !reflect.DeepEqual(before.ActiveClaims, after.ActiveClaims) {
					t.Error("Native preparation uncertainty lost its exact claim", gate, after.ActiveClaims)
				}
			} else if gate.State != "open" || len(after.ActiveClaims) != 0 {
				t.Error("Never-prepared native recovery invented uncertain execution", gate, after.ActiveClaims)
			}
			graphRecoveryProjectionPreserved(t, s, before, rows, d.Dispatch.NodeID)
		})
	}
}

func TestGraphQueuedExpiryProjectsTerminalWithoutNewAuthority(t *testing.T) {
	for _, kind := range []string{"root_basis", "node_input", "native"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			var s *Store
			var a domain.Attempt
			var graphID, inputID, nodeID, owner string
			if kind == "native" {
				var d domain.GraphDispatchView
				s, d, owner = graphRecoveryProjectionNativeFixture(t)
				graphID, inputID, nodeID = d.Dispatch.GraphID, d.Dispatch.InputID, d.Dispatch.NodeID
				var err error
				a, err = s.Attempt(ctx, d.Dispatch.AttemptID)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var graph domain.GraphAdmission
				s, graph, a, owner = graphLocalRecoveryFixture(t, kind)
				graphID = graph.Graph.ID
				op, err := s.GraphInputOperationForAttempt(ctx, owner, a.ID)
				if err != nil {
					t.Fatal(err)
				}
				inputID = op.ID
				if op.Control.NodeID != nil {
					nodeID = *op.Control.NodeID
				}
			}
			if a.Phase != "queued" || a.LaunchIntent || a.SourceSnapshotID != nil {
				t.Fatal("The expiry fixture is not actual unprepared queued work", a)
			}
			// This DAO-only case moves the metadata clock, as existing expiry DAO
			// tests do. It does not claim a real elapsed Engine/model deadline.
			if err := s.executionWrite(ctx, func(tx *sql.Tx) error {
				current, err := s.attempt(ctx, tx, a.ID)
				if err != nil {
					return err
				}
				current.DeadlineAt = "2000-01-01T00:00:00.000Z"
				return saveAttempt(ctx, tx, current)
			}); err != nil {
				t.Fatal(err)
			}
			before, err := s.Graph(ctx, graphID)
			if err != nil {
				t.Fatal(err)
			}
			rows := graphInitialAncestorRows(t, s)
			// Native admission already reserves capacity, so ClaimAttempt's expiry
			// boundary applies there. Local queued operations are swept unclaimed.
			if kind == "native" {
				if _, err := s.ClaimAttempt(ctx, a.ID, owner); err != nil {
					t.Fatal(err)
				}
			} else if count, err := s.SweepQueued(ctx, owner); err != nil || count != 1 {
				t.Fatal("The real queued expiry transaction did not settle its sole operation", count, err)
			}
			settled, err := s.Attempt(ctx, a.ID)
			if err != nil || settled.Phase != "settled" || settled.Outcome == nil || *settled.Outcome != "failed" || settled.PrimaryFailure == nil || settled.PrimaryFailure.Code != "deadline_exceeded" {
				t.Fatal("Actual queued expiry lacks its retained terminal failure", settled, err)
			}
			after, err := s.Graph(ctx, graphID)
			if err != nil || after.Graph.State != "failed" || after.Graph.NextOwner != "user" {
				t.Error("Expired graph work still advertises Council-owned progress", after, err)
			}
			if nodeID != "" {
				node := graphRecoveryProjectionNode(t, after, nodeID)
				if node.State != "failed" || node.NextOwner != "user" || after.Graph.Counts.Failed != "1" {
					t.Error("Expired receiving work did not project its actual failure", node, after.Graph.Counts)
				}
			}
			if kind != "native" {
				input, err := s.GraphInput(ctx, graphID, inputID)
				// Input states have no 'failed' member: incomplete is the existing
				// nonqualifying result of a failed local preparation operation.
				if err != nil || input.Summary.State != "incomplete" || input.Summary.Current || input.InputBinding != nil || input.RootBinding != nil {
					t.Error("Expired local input remained current/preparing or invented a binding", input, err)
				}
			}
			gate, err := s.Gate(ctx)
			if err != nil || gate.State != "open" || gate.IncidentID != nil || len(after.ActiveClaims) != 0 {
				t.Error("Known-zero expiry invented unresolved ownership", gate, err)
			}
			graphRecoveryProjectionPreserved(t, s, before, rows, nodeID)
		})
	}
}
