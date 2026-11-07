package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphAdvanceSkipsNewlyPausedMemberAndReplaysSiblingOnce(t *testing.T) {
	ctx := context.Background()
	s, graph, root, proofs, owner := graphReadyBasis(t)
	first, _ := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	second, _ := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[1])
	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	advance := graphAdvanceInput(view)
	// These controller facts were captured before the pause committed. The
	// Store must recheck under the actual admission transaction.
	paused := s.PauseTask(ctx, owner, view.Nodes[0].TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: view.Nodes[0].TaskVersion, ExpectedTaskRevision: view.Nodes[0].TaskRevision})
	cp := recoveryPauseResult(t, paused).Checkpoint
	if cp.State != "paused" {
		t.Fatal("the member did not reach a durable pause", cp)
	}
	rebind := graphAdvanceInput(view)
	rebindIDs := []string{view.Nodes[0].ID}
	rebind.RebindNodeIDs = &rebindIDs
	if result := s.AdvanceGraph(ctx, owner, graph.Graph.ID, rebind, domain.GraphAdvanceFacts{}); result.Envelope.Error == nil || result.Envelope.Error.Code != "scope_paused" {
		t.Fatal("an explicit rebind silently skipped its paused target", result)
	}

	s.AfterCommit = func() error { return errors.New("controlled Advance acknowledgement loss") }
	lost := s.AdvanceGraph(ctx, owner, graph.Graph.ID, advance, domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{first, second}})
	s.AfterCommit = nil
	if lost.Envelope.Error == nil || lost.Envelope.Error.Code != "write_uncertain" {
		t.Fatal("lost acknowledgement did not retain uncertainty", lost)
	}
	replay := s.AdvanceGraph(ctx, domain.NewID(), graph.Graph.ID, advance, domain.GraphAdvanceFacts{})
	result, err := decode[domain.GraphAdvanceResult](encode(replay.Envelope.Data))
	if replay.Status != 200 || replay.Envelope.Receipt == nil || err != nil || len(result.Dispatches) != 1 || result.Dispatches[0].Dispatch.NodeID != view.Nodes[1].ID {
		t.Fatal("the exact receipt did not retain only the independent sibling", replay, result, err)
	}
	after, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || after.Nodes[0].NodeAdmissionID != nil || after.Nodes[1].NodeAdmissionID == nil || after.Nodes[1].LatestAttemptID == nil || *after.Nodes[1].LatestAttemptID != result.Dispatches[0].Dispatch.AttemptID || after.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("paused member or sibling admission changed incorrectly", after, err)
	}

	resume := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: cp.Revision, Scope: cp.Scope}
	resumed := s.ResumeCheckpoint(ctx, owner, cp.ID, resume)
	if resumed.Envelope.Error != nil {
		t.Fatal("member resume failed", resumed)
	}
	stale := advance
	stale.RequestID = domain.NewID()
	if result := s.AdvanceGraph(ctx, owner, graph.Graph.ID, stale, domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{first}}); result.Envelope.Error == nil || result.Envelope.Receipt != nil {
		t.Fatal("pre-pause member authority silently became current after resume", result)
	}
	final, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || final.Nodes[0].NodeAdmissionID != nil || final.Nodes[1].LatestAttemptID == nil || !reflect.DeepEqual(final.Nodes[1].LatestAttemptID, after.Nodes[1].LatestAttemptID) || final.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("stale request replayed or changed sibling work", after, final, err)
	}
}
