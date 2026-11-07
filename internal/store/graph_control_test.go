package store

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphStopNeedsNoBudgetAndReplaysExactScope(t *testing.T) {
	s, graph, _, owner := graphLocalFixture(t)
	ctx := context.Background()
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		b, e := s.taskBudget(ctx, tx, graph.Graph.RootTaskID)
		if e != nil {
			return e
		}
		b.ConsumedAdmissions = b.GrantedAdmissions
		b.RemainingAdmissions = "0"
		b.CurrentWindow.FirstAdmissionAt = ptr("2000-01-01T00:00:00.000Z")
		b.CurrentWindow.DeadlineAt = ptr("2000-01-01T00:30:00.000Z")
		return saveBudget(ctx, tx, *b)
	}); e != nil {
		t.Fatal(e)
	}
	input := domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1"}
	first := s.StopGraph(ctx, owner, graph.Graph.ID, input)
	if first.Status != 202 {
		t.Fatal("Stop incorrectly depended on balance/window or stayed unavailable", first)
	}
	stopped := first.Envelope.Data.(domain.GraphStopResult)
	if stopped.Reason != "accepted" || !stopped.StopAccepted || stopped.Graph.ControlRevision != "2" || stopped.Graph.Stop == nil || stopped.Graph.Stop.RequestID != input.RequestID || len(stopped.TargetAttemptIDs) != 1 || stopped.TargetAttemptIDs[0] != graph.PreparationAttemptID {
		t.Fatal("Stop did not capture its exact owned scope", stopped)
	}
	a, e := s.Attempt(ctx, graph.PreparationAttemptID)
	if e != nil || a.Outcome == nil || *a.Outcome != "cancelled" || a.LaunchIntent {
		t.Fatal("known unstarted basis did not settle cancelled", a, e)
	}
	gate, e := s.Gate(ctx)
	if e != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("unstarted Stop invented uncertainty", gate, e)
	}
	replay := s.StopGraph(ctx, domain.NewID(), graph.Graph.ID, input)
	decoded, e := decode[domain.GraphStopResult](encode(replay.Envelope.Data))
	if replay.Status != 202 || e != nil || !reflect.DeepEqual(stopped, decoded) || !reflect.DeepEqual(first.Envelope.Receipt, replay.Envelope.Receipt) {
		t.Fatal("Stop replay consulted newer owner/controls", replay, e)
	}
	fresh := s.StopGraph(ctx, owner, graph.Graph.ID, domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "2"})
	if fresh.Status != 200 || fresh.Envelope.Data.(domain.GraphStopResult).StopAccepted || fresh.Envelope.Data.(domain.GraphStopResult).Reason != "already_stopped" {
		t.Fatal("repeat Stop created another control transition", fresh)
	}
}

func TestGraphNodeStopPreservesIndependentIntentAndSharedBasis(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	in, f, owner := graphTestStart(t, s)
	in.Nodes = append(in.Nodes, domain.GraphNodeInput{Key: "c", OriginalText: "Dependent C"})
	edges := []domain.GraphDependencyInput{{From: "a", To: "c"}}
	in.Dependencies = &edges
	result := s.CreateGraph(ctx, owner, in, f)
	if result.Status != 201 {
		t.Fatal(result)
	}
	graph := result.Envelope.Data.(domain.GraphAdmission)
	a, b, c := graph.Nodes[0], graph.Nodes[1], graph.Nodes[2]
	wrong := s.StopGraphNode(ctx, owner, graph.Graph.ID, "a", domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1", NodeID: b.ID, ExpectedNodeControlRevision: "1"})
	if wrong.Envelope.Error == nil {
		t.Fatal("route/body node mismatch retargeted Stop", wrong)
	}
	stopped := s.StopGraphNode(ctx, owner, graph.Graph.ID, "a", domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: "1", NodeID: a.ID, ExpectedNodeControlRevision: "1"})
	if stopped.Status != 202 || stopped.Envelope.Data.(domain.GraphStopResult).Reason != "accepted" {
		t.Fatal(stopped)
	}
	view, e := s.Graph(ctx, graph.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	nodes := map[string]domain.GraphNodeSummary{}
	for _, n := range view.Nodes {
		nodes[n.Key] = n
	}
	if view.Graph.ControlRevision != "1" || nodes["a"].ControlRevision != "2" || nodes["a"].Stop == nil || nodes["c"].ID != c.ID || nodes["c"].State != "blocked" || nodes["b"].ID != b.ID || nodes["b"].ControlRevision != "1" || nodes["b"].State != "waiting" || nodes["b"].Stop != nil {
		t.Fatal("node Stop cancelled an independent sibling or lost descendants", view)
	}
	root, e := s.Attempt(ctx, graph.PreparationAttemptID)
	if e != nil || root.Stop != nil || root.Phase != "queued" {
		t.Fatal("node Stop cancelled the shared basis needed by its sibling", root, e)
	}
}

func TestGraphReadPagesBindHighwaterAndFilters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	input, f, owner := graphTestStart(t, s)
	create := func() domain.GraphAdmission {
		t.Helper()
		input.RequestID = domain.NewID()
		r := s.CreateGraph(ctx, owner, input, f)
		if r.Status != 201 {
			t.Fatal(r)
		}
		return r.Envelope.Data.(domain.GraphAdmission)
	}
	first, second := create(), create()
	page, e := s.Graphs(ctx, input.ProjectID, 1, "")
	if e != nil || len(page.Items) != 1 || page.Items[0].ID != first.Graph.ID || page.NextCursor == nil {
		t.Fatal("graph identity page unavailable", page, e)
	}
	third := create()
	rest, e := s.Graphs(ctx, input.ProjectID, 100, *page.NextCursor)
	if e != nil || len(rest.Items) != 1 || rest.Items[0].ID != second.Graph.ID || rest.Items[0].ID == third.Graph.ID {
		t.Fatal("graph page included post-highwater identity", rest, e)
	}
	if _, e = s.Graphs(ctx, domain.NewID(), 100, *page.NextCursor); e == nil {
		t.Fatal("graph cursor changed project scope")
	}
	inputs, e := s.GraphInputs(ctx, first.Graph.ID, 100, "")
	if e != nil || len(inputs.Items) != 1 || inputs.Items[0].ID != *first.Graph.RootInputID || inputs.Items[0].Kind != "root_basis" {
		t.Fatal("real preparing input missing", inputs, e)
	}
	dispatches, e := s.GraphDispatches(ctx, first.Graph.ID, "", "", 100, "")
	if e != nil || dispatches.Items == nil || len(dispatches.Items) != 0 {
		t.Fatal("empty dispatch list invented native work", dispatches, e)
	}
}
