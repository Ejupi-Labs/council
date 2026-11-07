package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphOrdinaryRunNamesStaleRequestedVersionWithoutNewAuthority(t *testing.T) {
	s, graph, _, _, owner := graphReadyBasis(t)
	ctx := context.Background()
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, view.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	if guided := s.UpdateGraphNodeGuidance(ctx, owner, graph.Graph.ID, view.Nodes[0].Key, graphGuidanceInput(view, task), domain.GraphAdvanceFacts{}); guided.Status != 200 {
		t.Fatal(guided)
	}
	task, e = s.Task(ctx, task.Task.ID)
	if e != nil || task.Version.Version != "2" {
		t.Fatal(task, e)
	}
	var before int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&before); e != nil {
		t.Fatal(e)
	}
	for _, route := range []string{"early", "routing", "workflow"} {
		t.Run(route, func(t *testing.T) {
			var failure *domain.Error
			in := domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: "1"}
			switch route {
			case "early":
				failure = domain.SafeError(s.CheckStandaloneAuthority(ctx, task.Task.ID, false, "1"))
			case "routing":
				failure = s.StartAttemptWithRouting(ctx, owner, task.Task.ID, in, wrapperRoutingFacts("fake")).Envelope.Error
			case "workflow":
				failure = s.StartAttemptWithWorkflow(ctx, owner, task.Task.ID, in, domain.WorkflowAdmissionFacts{Routing: wrapperRoutingFacts("fake")}).Envelope.Error
			}
			if failure == nil || failure.Code != "graph_input_stale" || failure.GraphRef == nil || failure.GraphRef.GraphID != graph.Graph.ID || failure.GraphRef.NodeID == nil || *failure.GraphRef.NodeID != view.Nodes[0].ID {
				t.Fatal("stale requested graph version lost its precise actionable scope", route, failure)
			}
		})
	}
	if failure := domain.SafeError(s.CheckStandaloneAuthority(ctx, task.Task.ID, false, "2")); failure.Code != "graph_scope_required" {
		t.Fatal("current graph version became an ordinary standalone run", failure)
	}
	if failure := domain.SafeError(s.CheckStandaloneAuthority(ctx, task.Task.ID, true)); failure.Code != "graph_budget_required" {
		t.Fatal("budget route changed its distinct scope error", failure)
	}
	var after int
	if e = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&after); e != nil || after != before {
		t.Fatal("refused stale Run created new work", before, after, e)
	}
}
