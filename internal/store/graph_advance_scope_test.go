package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphAdvanceIgnoresUnrelatedStoppedNodeWithoutRefreshingItsAuthority(t *testing.T) {
	s, prior, _, captured, stage, owner := graphReviewResumeFixture(t, false)
	ctx := context.Background()
	in := graphAdvanceInput(captured)
	b := captured.Nodes[1]
	stop := s.StopGraphNode(ctx, owner, captured.Graph.ID, b.Key, domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: captured.Graph.ControlRevision, NodeID: b.ID, ExpectedNodeControlRevision: b.ControlRevision})
	if stop.Status != 202 {
		t.Fatal(stop)
	}
	result := s.AdvanceGraph(ctx, owner, captured.Graph.ID, in, domain.GraphAdvanceFacts{Stages: []domain.WorkflowStepAdmissionFacts{stage}})
	if result.Status != 200 {
		t.Fatal("unrelated Stop retroactively invalidated the selected review scope", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || len(data.Dispatches) != 1 || data.Dispatches[0].Dispatch.NodeID != prior.Dispatch.NodeID || data.Dispatches[0].Dispatch.Role != "review" {
		t.Fatal("selected review scope did not produce exactly its own execution", data, err)
	}
	view, err := s.Graph(ctx, captured.Graph.ID)
	if err != nil || view.Nodes[1].Stop == nil || view.Nodes[1].ControlRevision == b.ControlRevision || view.Nodes[1].NodeAdmissionID != nil || view.Budget.Budget.ConsumedAdmissions != "2" || view.Budget.Budget.GrantedAdmissions != captured.Budget.Budget.GrantedAdmissions {
		t.Fatal("selected Advance resumed stopped work or changed the wrong shared allowance", view, err)
	}
	found := false
	for _, authority := range view.DispatchAuthority.NodeAuthorities {
		if authority.NodeID == b.ID {
			found = true
			if authority.NodeControlRevision != b.ControlRevision || authority.TaskVersion != b.TaskVersion {
				t.Fatal("Advance silently substituted newer unrelated authority", authority, b)
			}
		}
	}
	if !found {
		t.Fatal("captured full vector was silently rewritten", view.DispatchAuthority)
	}
	actual, err := s.graphNodes(ctx, s.db, captured.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range actual {
		if node.Summary.ID == b.ID && graphAuthorizedNode(mustGraphHeader(t, s, captured.Graph.ID), node.Summary) {
			t.Fatal("stale unrelated tuple became current dispatch authority")
		}
	}
}

func TestGraphAdvanceStillRejectsChangedSelectedNode(t *testing.T) {
	s, _, _, captured, stage, owner := graphReviewResumeFixture(t, false)
	ctx := context.Background()
	in := graphAdvanceInput(captured)
	task, err := s.Task(ctx, captured.Nodes[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	guided := s.UpdateGraphNodeGuidance(ctx, owner, captured.Graph.ID, captured.Nodes[0].Key, graphGuidanceInput(captured, task), domain.GraphAdvanceFacts{})
	if guided.Status != 200 {
		t.Fatal(guided)
	}
	var attempts int
	if err = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	result := s.AdvanceGraph(ctx, owner, captured.Graph.ID, in, domain.GraphAdvanceFacts{Stages: []domain.WorkflowStepAdmissionFacts{stage}})
	if result.Envelope.Error == nil || result.Envelope.Error.Code != "graph_control_conflict" || result.Envelope.Receipt != nil {
		t.Fatal("selected current version/control mismatch was ignored", result)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count); err != nil || count != attempts {
		t.Fatal("changed selected authority allocated work", attempts, count, err)
	}
}

// Typed boundary facts only; the actual Stop/known-first-GO path has independent
// Store and owned Engine coverage.
func TestGraphAdvanceStopOnlyDriftRequiresExactlyPreviousControl(t *testing.T) {
	for _, prior := range []string{"1", "0", "2", "3"} {
		node := domain.GraphNodeSummary{TaskVersion: "1", ControlRevision: "2", Stop: &domain.GraphStopRecord{NodeControlRevision: ptr("2")}}
		captured := domain.GraphNodeAuthority{TaskVersion: "1", NodeControlRevision: prior}
		if allowed := graphAdvanceStopOnlyDrift(node, captured); allowed != (prior == "1") {
			t.Fatal("Stop-only drift admitted an arbitrary old/future control", prior, allowed)
		}
	}
	node := domain.GraphNodeSummary{TaskVersion: "2", ControlRevision: "2", Stop: &domain.GraphStopRecord{NodeControlRevision: ptr("2")}}
	if graphAdvanceStopOnlyDrift(node, domain.GraphNodeAuthority{TaskVersion: "1", NodeControlRevision: "1"}) {
		t.Fatal("Stop-only exception hid changed task guidance")
	}
	node.TaskVersion = "1"
	node.Stop.NodeControlRevision = ptr("1")
	if graphAdvanceStopOnlyDrift(node, domain.GraphNodeAuthority{TaskVersion: "1", NodeControlRevision: "1"}) {
		t.Fatal("historical Stop marker did not identify the current control")
	}
}
