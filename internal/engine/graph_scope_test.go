package engine

import (
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphOrdinaryControlsReportScopeBeforeMissingProjectOrLegacyAuthority(t *testing.T) {
	e, projectID, repo := testEngine(t)
	ctx := context.Background()
	facts, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	e.control.Lock()
	e.fenced = true
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{{Key: "a", OriginalText: "Keep graph scope"}}}, facts)
	e.control.Unlock()
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	graph := created.Envelope.Data.(domain.GraphAdmission)
	before, err := e.db.GraphBudget(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-scope-moved")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Rename(moved, repo); err != nil {
			t.Error(err)
		}
	})
	marshal := func(in any) []byte {
		body, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	for _, taskID := range []string{graph.Graph.RootTaskID, graph.Nodes[0].TaskID} {
		for _, name := range []string{"run", "guidance", "grant", "adopt", "advance", "check"} {
			t.Run(name+"/"+taskID, func(t *testing.T) {
				requestID := domain.NewID()
				var result domain.Result
				switch name {
				case "run":
					result = e.StartAttempt(ctx, taskID, domain.StartAttemptInput{RequestID: requestID, ExpectedRevision: "1", TaskVersion: "1", ExecutionMode: "fake"})
				case "guidance":
					result = e.Guidance(ctx, taskID, domain.GuidanceInput{RequestID: requestID, ExpectedRevision: "1", GuidanceText: "This must use the graph controls."})
				case "grant":
					result = e.MutateRouting(ctx, "tasks.budget.amend", taskID, marshal(domain.BudgetAmendmentInput{RequestID: requestID, Action: "grant", ExpectedRevision: "1", ExpectedBudgetRevision: "1", AdditionalAdmissions: "1"}))
				case "adopt":
					result = e.MutateWorkflow(ctx, "tasks.workflow.adopt", taskID, marshal(domain.WorkflowAdoptInput{RequestID: requestID, ExpectedRevision: "1", TaskVersion: "1", Workflow: &domain.WorkflowInput{Mode: "auto"}}))
				case "advance":
					result = e.MutateWorkflow(ctx, "tasks.workflow.advance", taskID, marshal(domain.WorkflowAdvanceInput{RequestID: requestID, ExpectedRevision: "1", WorkflowRevision: "1", TaskVersion: "1"}))
				case "check":
					result = e.MutateWorkflow(ctx, "tasks.workflow.check", taskID, marshal(domain.WorkflowCheckInput{RequestID: requestID, ExpectedRevision: "1", WorkflowRevision: "1", TaskVersion: "1", CheckID: domain.NewID()}))
				}
				code := "graph_scope_required"
				if name == "grant" {
					code = "graph_budget_required"
				}
				if result.Status != 409 || result.Envelope.Error == nil || result.Envelope.Error.Code != code || result.Envelope.Error.GraphRef == nil || result.Envelope.Error.GraphRef.GraphID != graph.Graph.ID || result.Envelope.Error.GraphBudgetRef == nil || result.Envelope.Error.GraphBudgetRef.BudgetOwnerTaskID != graph.Graph.RootTaskID {
					t.Fatal("Graph membership was hidden by ordinary prerequisites", result)
				}
			})
		}
	}
	after, err := e.db.GraphBudget(ctx, graph.Graph.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Ordinary controls changed shared graph authority", before, after, err)
	}
}
