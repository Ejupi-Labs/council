package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphMembershipRejectsStandaloneAuthorityMutations(t *testing.T) {
	cases := []struct {
		name, code string
		run        func(*Store, string, string) domain.Result
	}{
		{"grant", "graph_budget_required", func(s *Store, owner, task string) domain.Result {
			return s.AmendBudget(context.Background(), task, domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: "1", ExpectedBudgetRevision: "1", AdditionalAdmissions: "1"})
		}},
		{"guidance", "graph_scope_required", func(s *Store, owner, task string) domain.Result {
			return s.UpdateGuidance(context.Background(), task, domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", GuidanceText: "New standalone guidance must not bypass graph controls."})
		}},
		{"run_routing", "graph_scope_required", func(s *Store, owner, task string) domain.Result {
			return s.StartAttempt(context.Background(), owner, task, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: "1", ExecutionMode: "fake"})
		}},
		{"run_workflow", "graph_scope_required", func(s *Store, owner, task string) domain.Result {
			return s.StartAttemptWithWorkflow(context.Background(), owner, task, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: "1", ExecutionMode: "fake"}, domain.WorkflowAdmissionFacts{})
		}},
		{"adopt_workflow", "graph_scope_required", func(s *Store, owner, task string) domain.Result {
			return s.AdoptWorkflow(context.Background(), owner, task, domain.WorkflowAdoptInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: "1", Workflow: &domain.WorkflowInput{Mode: "auto"}}, domain.WorkflowAdmissionFacts{})
		}},
		{"advance_workflow", "graph_scope_required", func(s *Store, owner, task string) domain.Result {
			return s.AdvanceWorkflow(context.Background(), owner, task, domain.WorkflowAdvanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", WorkflowRevision: "1", TaskVersion: "1"}, domain.WorkflowStepAdmissionFacts{})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, graph, _, owner := graphLocalFixture(t)
			ctx := context.Background()
			node := graph.Nodes[0]
			before, e := s.GraphBudget(ctx, graph.Graph.ID)
			if e != nil {
				t.Fatal(e)
			}
			result := c.run(s, owner, node.TaskID)
			if result.Status != 409 || result.Envelope.Error == nil || result.Envelope.Error.Code != c.code || result.Envelope.Error.GraphRef == nil || result.Envelope.Error.GraphRef.GraphID != graph.Graph.ID || result.Envelope.Error.GraphBudgetRef == nil || result.Envelope.Error.GraphBudgetRef.BudgetOwnerTaskID != graph.Graph.RootTaskID {
				t.Fatal("ordinary route bypassed or hid graph authority", result)
			}
			after, e := s.GraphBudget(ctx, graph.Graph.ID)
			if e != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rejected standalone mutation changed shared authority", before, after, e)
			}
			task, e := s.Task(ctx, node.TaskID)
			if e != nil || task.Task.CurrentVersion != "1" || task.Task.Revision != "1" || task.Task.LatestAttemptID != nil {
				t.Fatal("rejected scope created work or a derived version", task, e)
			}
		})
	}
}

func TestGraphMembershipLedgerNamesCanonicalBudget(t *testing.T) {
	s, graph, _, _ := graphLocalFixture(t)
	ctx := context.Background()
	_, e := s.BudgetLedger(ctx, graph.Nodes[0].TaskID, 10, "")
	de, ok := e.(*domain.Error)
	if !ok || de.Code != "graph_budget_required" || de.GraphBudgetRef == nil || de.GraphBudgetRef.GraphID != graph.Graph.ID {
		t.Fatal("child ledger did not return exact budget owner error", e)
	}
	ledger, e := s.GraphBudgetLedger(ctx, graph.Graph.ID, 10, "")
	if e != nil || len(ledger.Items) != 1 || ledger.Items[0].TaskID != graph.Graph.RootTaskID || ledger.Items[0].Kind != "grant" {
		t.Fatal("graph ledger is not its one canonical history", ledger, e)
	}
}
