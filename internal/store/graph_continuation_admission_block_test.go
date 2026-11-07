package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These registered DAO fixtures qualify transactional projections and immutable
// metadata. The actual owner/worker reproduction is separate Engine evidence.
func graphContinuationBlockFixture(t *testing.T, reason string) (*Store, domain.GraphDispatchView, domain.WorkflowStepAdmissionFacts, string) {
	t.Helper()
	s, dispatch, author, owner := graphOutputConfiguredFixture(t, false, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.Nodes[0].Workflow = &domain.WorkflowInput{Review: "required"}
		if reason == "budget_exhausted" {
			in.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "1"}}
		}
	})
	ctx := context.Background()
	if _, err := s.SettleWorkflowStep(ctx, dispatch.Dispatch.AttemptID, author); err != nil {
		t.Fatal(err)
	}
	facts := graphReviewAdmissionFacts(t, s, dispatch, author)
	step, err := s.WorkflowStep(ctx, dispatch.Dispatch.StepID)
	if err != nil || step.AdvanceProof == nil || step.AdvanceProof.NextLogicalKey != facts.Plan.LogicalKey {
		t.Fatal("fixture must have the exact canonical automatic review proof", step, err)
	}
	switch reason {
	case "budget_expired":
		// Only this DAO case adjusts a metadata deadline. The Engine common-window
		// tests independently wait for a real captured expiry.
		if err := s.executionWrite(ctx, func(tx *sql.Tx) error {
			budget, err := s.taskBudget(ctx, tx, dispatch.Dispatch.TaskID)
			if err != nil {
				return err
			}
			past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
			budget.CurrentWindow.DeadlineAt = &past
			return saveBudget(ctx, tx, *budget)
		}); err != nil {
			t.Fatal(err)
		}
	case "routing_unavailable":
		enabled := false
		for _, id := range []string{"codex-native", "claude-native"} {
			result := s.UpdateRoutingContext(ctx, id, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &enabled})
			if result.Status != 200 {
				t.Fatal("fixture must disable the actual routing declarations", result)
			}
		}
	}
	return s, dispatch, facts, owner
}

type graphContinuationBlockSnapshot struct {
	Budget   domain.GraphBudgetView
	Ledger   domain.Page[domain.BudgetLedgerEntry]
	Step     domain.WorkflowStep
	Terminal string
	Claims   []domain.GraphClaimView
	Counts   map[string]int
}

func graphContinuationBlockState(t *testing.T, s *Store, dispatch domain.GraphDispatchView) graphContinuationBlockSnapshot {
	t.Helper()
	ctx := context.Background()
	var state graphContinuationBlockSnapshot
	var err error
	state.Budget, err = s.GraphBudget(ctx, dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	state.Ledger, err = s.GraphBudgetLedger(ctx, dispatch.Dispatch.GraphID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	state.Step, err = s.WorkflowStep(ctx, dispatch.Dispatch.StepID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", dispatch.Dispatch.AttemptID).Scan(&state.Terminal); err != nil {
		t.Fatal(err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("fixture must retain its exact open ownership gate", gate, err)
	}
	state.Claims = gate.ActiveClaims
	state.Counts = map[string]int{}
	for _, table := range []string{"attempts", "attempt_terminals", "workflow_steps", "graph_dispatches", "task_versions", "budget_ledger", "capacity_claims"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		state.Counts[table] = count
	}
	return state
}

func TestGraphKnownContinuationAdmissionBlockProjectsCurrentWorkflowWithoutRearm(t *testing.T) {
	for _, reason := range []string{"budget_exhausted", "budget_expired", "routing_unavailable"} {
		t.Run(reason, func(t *testing.T) {
			s, dispatch, facts, owner := graphContinuationBlockFixture(t, reason)
			ctx := context.Background()
			before := graphContinuationBlockState(t, s, dispatch)
			if len(before.Claims) != 0 {
				t.Fatal("single settled node must have no remaining claims", before.Claims)
			}
			graphBefore, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
			if err != nil || len(graphBefore.Nodes) != 1 || graphBefore.Nodes[0].NextOwner != "council" {
				t.Fatal("fixture must reach a real pending automatic review", graphBefore, err)
			}
			next, err := s.ContinueWorkflow(ctx, owner, dispatch.Dispatch.StepID, facts)
			var fault *domain.Error
			if next != nil || !errors.As(err, &fault) || fault.Code != reason {
				t.Fatal("wrong actual continuation refusal", next, err)
			}
			workflow, err := s.Workflow(ctx, dispatch.Dispatch.TaskID, domain.WorkflowSelector{})
			if err != nil || workflow.Workflow == nil || workflow.Workflow.State != "blocked" || workflow.Workflow.NextAction != reason || workflow.Workflow.NextOwner != "user" || workflow.Workflow.Revision == facts.ExpectedWorkflowRevision {
				t.Fatal("known refusal did not consume automatic workflow freshness", workflow, err)
			}
			after := graphContinuationBlockState(t, s, dispatch)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("passive block changed predecessor terminal/proof, budget/ledger, claim or work counts", before, after)
			}
			graph, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
			if err != nil || graph.Nodes[0].State != "blocked" || graph.Nodes[0].NextAction != reason || graph.Nodes[0].NextOwner != "user" || graph.Nodes[0].AcceptedResult != nil || graph.Graph.State != "blocked" || graph.Graph.NextOwner != "user" || graph.Graph.Counts.Blocked != "1" {
				t.Fatal("graph still promises Council work after the current workflow became user-blocked", graph, err)
			}
			if graph.DispatchAuthority == nil || graph.DispatchAuthority.State != "suspended" || graph.Graph.ControlRevision != graphBefore.Graph.ControlRevision || graph.Graph.BudgetAuthorityEpoch != graphBefore.Graph.BudgetAuthorityEpoch || graph.Nodes[0].TaskVersion != graphBefore.Nodes[0].TaskVersion || graph.Nodes[0].ControlRevision != graphBefore.Nodes[0].ControlRevision {
				t.Fatal("block projection changed controls/version or retained dispatch permission", graph)
			}
			if repeated, err := s.ContinueWorkflow(ctx, owner, dispatch.Dispatch.StepID, facts); err != nil || repeated != nil {
				t.Fatal("old automatic proof rearmed a known blocked continuation", repeated, err)
			}
			replayed, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
			if err != nil || !reflect.DeepEqual(graph, replayed) || !reflect.DeepEqual(after, graphContinuationBlockState(t, s, dispatch)) {
				t.Fatal("observing/replaying the old proof changed graph authority or accounting", replayed, err)
			}
			if reason == "budget_exhausted" {
				grant := graphGrantInput(graph.Graph)
				grant.AdditionalAdmissions = "1"
				result := s.AmendGraphBudget(ctx, owner, graph.Graph.ID, grant)
				if result.Status != 200 {
					t.Fatal(result)
				}
				granted := graphContinuationBlockState(t, s, dispatch)
				if repeated, err := s.ContinueWorkflow(ctx, owner, dispatch.Dispatch.StepID, facts); err != nil || repeated != nil {
					t.Fatal("a grant rearmed the old automatic review proof", repeated, err)
				}
				if !reflect.DeepEqual(granted, graphContinuationBlockState(t, s, dispatch)) {
					t.Fatal("old proof spent or reopened granted authority")
				}
			}
		})
	}
}

func TestGraphContinuationAdmissionBlockPreservesChangedCurrentScope(t *testing.T) {
	for _, change := range []string{"accepted_stop", "current_version", "recovered_owner"} {
		t.Run(change, func(t *testing.T) {
			s, dispatch, facts, owner := graphContinuationBlockFixture(t, "budget_exhausted")
			ctx := context.Background()
			graph, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "accepted_stop":
				result := s.StopGraph(ctx, owner, graph.Graph.ID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: graph.Graph.ControlRevision})
				if result.Status != 202 {
					t.Fatal(result)
				}
			case "current_version":
				task, err := s.Task(ctx, dispatch.Dispatch.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				result := s.UpdateGraphNodeGuidance(ctx, owner, graph.Graph.ID, graph.Nodes[0].Key, graphGuidanceInput(graph, task), domain.GraphAdvanceFacts{})
				if result.Status != 200 {
					t.Fatal(result)
				}
			case "recovered_owner":
				if err := s.RecoverOwner(ctx, domain.NewID()); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.Graph(ctx, graph.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			unchanged := graphContinuationBlockState(t, s, dispatch)
			var workflowBefore string
			if err := s.db.QueryRow("SELECT data FROM workflows WHERE id=?", dispatch.Dispatch.WorkflowID).Scan(&workflowBefore); err != nil {
				t.Fatal(err)
			}
			if change == "accepted_stop" {
				// The actual continuation rejects changed graph controls before its
				// known-admission-block projection can be reached.
				if next, _ := s.ContinueWorkflow(ctx, owner, dispatch.Dispatch.StepID, facts); next != nil {
					t.Fatal("an accepted Stop admitted its old pending review", next)
				}
			} else if err := s.projectWorkflowAdmissionBlock(ctx, owner, dispatch.Dispatch.StepID, facts, domain.Fail("budget_exhausted", "Controlled late passive block observation.")); err != nil {
				t.Fatal(err)
			}
			after, err := s.Graph(ctx, graph.Graph.ID)
			var workflowAfter string
			if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(unchanged, graphContinuationBlockState(t, s, dispatch)) {
				t.Fatal("a stale block changed Stop, new version/owner, or retained accounting", change, before, after, err)
			}
			if err := s.db.QueryRow("SELECT data FROM workflows WHERE id=?", dispatch.Dispatch.WorkflowID).Scan(&workflowAfter); err != nil || workflowAfter != workflowBefore {
				t.Fatal("a stale block rewrote retained workflow history", change, err)
			}
		})
	}
}

func TestGraphContinuationAdmissionBlockPreservesIndependentAdmittedSibling(t *testing.T) {
	s, dispatch, author, owner := graphOutputSettlementFixture(t)
	ctx := context.Background()
	if _, err := s.SettleWorkflowStep(ctx, dispatch.Dispatch.AttemptID, author); err != nil {
		t.Fatal(err)
	}
	facts := graphReviewAdmissionFacts(t, s, dispatch, author)
	graph, err := s.Graph(ctx, dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := s.GraphRoot(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	proofs := []domain.VerifiedResponse{}
	for _, ref := range []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs} {
		artifact, err := s.Artifact(ctx, ref.ArtifactID)
		if err != nil {
			t.Fatal(err)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: artifact.ID, SHA256: *artifact.SHA256, ByteLength: artifact.ByteLength, PhysicalIdentity: artifact.PhysicalIdentity})
	}
	sibling := graph.Nodes[1]
	siblingFacts, trigger := graphReadyNodeFacts(t, s, domain.GraphAdmission{Graph: graph.Graph, Nodes: graph.Nodes, PreparationAttemptID: root.OperationAttemptID}, root, proofs, owner, sibling)
	admitted, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, siblingFacts)
	if err != nil || admitted.Dispatch.ID == "" {
		t.Fatal("independent sibling must have a real pending admission", admitted, err)
	}
	disabled := false
	for _, id := range []string{"codex-native", "claude-native"} {
		result := s.UpdateRoutingContext(ctx, id, domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &disabled})
		if result.Status != 200 {
			t.Fatal(result)
		}
	}
	before, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || len(before.ActiveClaims) != 1 || before.ActiveClaims[0].AttemptID != admitted.Dispatch.AttemptID {
		t.Fatal("sibling must retain its actual reserved claim", before, err)
	}
	accounting := graphContinuationBlockState(t, s, dispatch)
	next, err := s.ContinueWorkflow(ctx, owner, dispatch.Dispatch.StepID, facts)
	var fault *domain.Error
	if next != nil || !errors.As(err, &fault) || fault.Code != "routing_unavailable" {
		t.Fatal(next, err)
	}
	after, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || after.Nodes[0].State != "blocked" || after.Nodes[0].NextOwner != "user" || after.Nodes[0].NextAction != "routing_unavailable" ||
		!reflect.DeepEqual(before.Nodes[1], after.Nodes[1]) || after.Graph.State != "queued" || after.Graph.NextOwner != "council" || after.DispatchAuthority == nil || after.DispatchAuthority.State == "suspended" {
		t.Fatal("one known blocked review relabelled/suspended independent admitted work", before, after, err)
	}
	if !reflect.DeepEqual(accounting, graphContinuationBlockState(t, s, dispatch)) {
		t.Fatal("blocked review spent an admission or released the sibling claim")
	}
}
