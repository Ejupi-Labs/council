package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphAdvanceSkipsPausedMemberAndAdmitsRealSibling(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	graph, _ := graphReleaseReadyGroup(t, e, projectID)
	view, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || len(view.Nodes) != 2 || view.Nodes[0].InputID == nil || view.Nodes[1].InputID == nil {
		t.Fatal("the real input fixture is incomplete", view, err)
	}
	paused := view.Nodes[0]
	result := e.db.PauseTask(ctx, e.config.OwnerInstanceID, paused.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: paused.TaskVersion, ExpectedTaskRevision: paused.TaskRevision})
	if result.Envelope.Error != nil {
		t.Fatal("member pause failed", result)
	}
	view, err = e.db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number}
	for _, node := range view.Nodes {
		input.NodeAuthorities = append(input.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	advanced := e.advanceGraph(ctx, graph.Graph.ID, input)
	if advanced.Envelope.Error != nil || advanced.Envelope.Receipt == nil {
		t.Fatal("independent sibling did not advance", advanced)
	}
	after, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || after.Nodes[0].NodeAdmissionID != nil || after.Nodes[1].NodeAdmissionID == nil || after.Nodes[1].LatestAttemptID == nil || after.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("advance did not isolate the paused member", after, err)
	}
	sibling := *after.Nodes[1].LatestAttemptID
	if replay := e.advanceGraph(ctx, graph.Graph.ID, input); replay.Envelope.Error != nil || replay.Envelope.Receipt == nil {
		t.Fatal("the exact Advance receipt could not be replayed", replay)
	}
	again, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || again.Nodes[0].NodeAdmissionID != nil || again.Nodes[1].LatestAttemptID == nil || *again.Nodes[1].LatestAttemptID != sibling || again.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("receipt replay changed or duplicated work", again, err)
	}
}
