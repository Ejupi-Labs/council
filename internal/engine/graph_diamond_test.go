package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphDiamondNode(t *testing.T, e *Engine, graphID, key string) domain.GraphNodeSummary {
	t.Helper()
	view, err := e.db.Graph(context.Background(), graphID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Nodes {
		if node.Key == key {
			return node
		}
	}
	t.Fatal("The actual immutable graph lost its declared node", key)
	return domain.GraphNodeSummary{}
}

func graphDiamondAcknowledge(t *testing.T, e *Engine, graphID, key string) domain.ResponseAcceptance {
	t.Helper()
	node := graphDiamondNode(t, e, graphID, key)
	if node.LatestAttemptID == nil {
		t.Fatal("The controlled response node was not actually admitted", node)
	}
	a, err := e.db.ClaimAttempt(context.Background(), *node.LatestAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(context.Background(), a)
	in := exactResponseAcceptanceInput(t, e, a.TaskID, a.ID)
	return assertOriginalResponseAcceptanceWire(t, e.AcceptResponse(context.Background(), a.TaskID, in), in)
}

func TestGraphActualAcknowledgedDiamondDeduplicatesSharedAncestorAndCompletesAllNodes(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.ctx, e.cancel = ownerCtx, cancel
	edges := []domain.GraphDependencyInput{{From: "a", To: "b", Kind: "context"}, {From: "a", To: "c", Kind: "context"}, {From: "b", To: "d", Kind: "context"}, {From: "c", To: "d", Kind: "context"}}
	nodes := []domain.GraphNodeInput{}
	for _, key := range []string{"a", "b", "c", "d"} {
		nodes = append(nodes, domain.GraphNodeInput{Key: key, OriginalText: "Return the exact controlled response while retaining all attributed predecessor context for node " + key + ".", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}})
	}
	group, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: nodes, Dependencies: &edges, Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "4"}}})
	e.continueGraphAdmissions(source)
	acks := map[string]domain.ResponseAcceptance{"a": graphDiamondAcknowledge(t, e, group.Graph.ID, "a")}
	last := ""
	for _, key := range []string{"b", "c"} {
		node := graphDiamondNode(t, e, group.Graph.ID, key)
		if node.InputID == nil {
			t.Fatal("The actual acknowledged ancestor did not prepare both registered branches", node)
		}
		record, err := e.db.GraphInputRecord(ctx, group.Graph.ID, *node.InputID)
		if err != nil {
			t.Fatal(err)
		}
		a, err := e.db.ClaimAttempt(ctx, record.Public.Summary.OperationAttemptID, e.config.OwnerInstanceID)
		if err != nil {
			t.Fatal(err)
		}
		e.run(ctx, a)
		input, err := e.db.GraphInput(ctx, group.Graph.ID, *node.InputID)
		if err != nil || input.Summary.State != "ready" || input.InputBinding == nil || len(input.InputBinding.ContributorVector) != 1 {
			t.Fatal("A real diamond branch lost its exact accepted ancestor", input, err)
		}
		last = a.ID
	}
	e.continueGraphAdmissions(last)
	acks["b"] = graphDiamondAcknowledge(t, e, group.Graph.ID, "b")
	if node := graphDiamondNode(t, e, group.Graph.ID, "d"); node.InputID != nil {
		t.Fatal("A partial fan-in qualified before the other required branch was accepted", node)
	}
	acks["c"] = graphDiamondAcknowledge(t, e, group.Graph.ID, "c")
	receiver := graphDiamondNode(t, e, group.Graph.ID, "d")
	if receiver.InputID == nil {
		t.Fatal("The actual complete acknowledged diamond did not prepare its receiver", receiver)
	}
	record, err := e.db.GraphInputRecord(ctx, group.Graph.ID, *receiver.InputID)
	if err != nil || len(record.ExpectedContributions) != 2 || len(record.ExpectedAuthority.ContributorVector) != 3 {
		t.Fatal("The shared ancestor was omitted or duplicated across the actual two paths", record, err)
	}
	seen := map[string]bool{}
	for _, contributor := range record.ExpectedAuthority.ContributorVector {
		if seen[contributor.NodeID] || contributor.AcceptanceKind != "response" || contributor.ResponseBinding == nil || contributor.CandidateBinding != nil {
			t.Fatal("The actual diamond vector lost exact unique response provenance", contributor)
		}
		seen[contributor.NodeID] = true
		matched := false
		for _, ack := range acks {
			matched = matched || contributor.AcceptanceID == ack.ID && contributor.ProducerAttemptID == ack.AttemptID && contributor.ResponseBinding.ResponseArtifactID == ack.ResponseArtifactID
		}
		if !matched {
			t.Fatal("The diamond imported a response which was never actually acknowledged", contributor)
		}
	}
	local, err := e.db.ClaimAttempt(ctx, record.Public.Summary.OperationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, local)
	input, err := e.db.GraphInput(ctx, group.Graph.ID, *receiver.InputID)
	if err != nil || input.Summary.State != "ready" || input.InputBinding == nil || len(input.InputBinding.ContributorVector) != 3 || input.ContextPacket == nil {
		t.Fatal("Actual diamond composition failed to preserve all three accepted ancestors", input, err)
	}
	if _, err = e.workspace.VerifyGraphInput(ctx, *input.InputBinding); err != nil {
		t.Fatal("The complete diamond input failed fresh physical verification", err)
	}
	e.continueGraphAdmissions(local.ID)
	acks["d"] = graphDiamondAcknowledge(t, e, group.Graph.ID, "d")
	view, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil || view.Graph.State != "completed" || view.Graph.Counts.Completed != "4" || view.Budget.Budget.ConsumedAdmissions != "4" || view.DispatchAuthority == nil || view.DispatchAuthority.State != "suspended" {
		t.Fatal("The economical actual diamond did not complete exactly four accepted response authors", view, err)
	}
	for _, node := range view.Nodes {
		if node.AcceptedResult == nil || node.AcceptedResult.AcceptanceID != acks[node.Key].ID {
			t.Fatal("The all-required aggregate omitted an actual acknowledgement", node)
		}
	}
}
