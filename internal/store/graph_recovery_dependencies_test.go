package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Actual accepted A -> ready B input, plus independent ready C. This proves
// the transaction guard without claiming native process or filesystem work.
func TestGraphRecoveryPausedAncestorBlocksInitialAdmissionButNotIndependentSibling(t *testing.T) {
	f := graphInitialAncestorStopFixture(t)
	s := f.store
	ctx := context.Background()
	a := f.view.Nodes[0]
	task, err := s.Task(ctx, a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	paused := s.PauseTask(ctx, f.owner, a.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision})
	if paused.Envelope.Error != nil || paused.Envelope.Receipt == nil {
		t.Fatal("accepted ancestor pause failed", paused)
	}
	before, err := s.Graph(ctx, f.view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	b := graphInitialAncestorCurrentFact(t, f.b, before)
	c := graphInitialAncestorCurrentFact(t, f.c, before)
	rows := graphInitialAncestorRows(t, s)
	refused := s.AdvanceGraph(ctx, f.owner, before.Graph.ID, graphAdvanceInput(before), domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{c, b}})
	if refused.Envelope.Error == nil || refused.Envelope.Error.Code != "scope_paused" || refused.Envelope.Receipt != nil {
		t.Fatal("paused ancestor allowed dependent admission", refused)
	}
	graphInitialAncestorAssertRows(t, rows, graphInitialAncestorRows(t, s), "paused ancestor refusal did not roll back sibling admission")
	allowed := s.AdvanceGraph(ctx, f.owner, before.Graph.ID, graphAdvanceInput(before), domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{c}})
	result, err := decode[domain.GraphAdvanceResult](encode(allowed.Envelope.Data))
	if allowed.Envelope.Error != nil || err != nil || len(result.Dispatches) != 1 || result.Dispatches[0].Dispatch.NodeID != c.NodeID {
		t.Fatal("paused prerequisite blocked unrelated sibling", allowed, result, err)
	}
}
