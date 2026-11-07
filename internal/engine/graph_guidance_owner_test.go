package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphGuidanceOwnerFixture(t *testing.T) (*Engine, domain.GraphDetail, domain.GraphGuidanceInput) {
	t.Helper()
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	e.ctx, e.cancel = ownerCtx, cancel
	checks, zero := []domain.CheckDeclaration{}, 0
	group, _ := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "a", OriginalText: "Complete unchanged receiver original 東京\r\nPreserve every byte and the exact captured choices.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"},
			Routing: &domain.RoutingOptions{Suitability: "general"}, Workflow: &domain.WorkflowInput{Review: "auto", Checks: &checks, MaxCorrectionCycles: &zero}},
		{Key: "b", OriginalText: "Keep this independent captured sibling unchanged.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
	}})
	view, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	var node domain.GraphNodeSummary
	for _, n := range view.Nodes {
		if n.Key == "a" {
			node = n
		}
	}
	in := domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: node.TaskRevision, GuidanceText: "\ufeffComplete additional receiving guidance café 🙂\r\nUse the newly captured review choice.",
		Workflow: &domain.WorkflowInput{Review: "required"}, Routing: &domain.RoutingOptions{Suitability: "routine"}},
		ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: node.ID, ExpectedNodeControlRevision: node.ControlRevision, TaskVersion: node.TaskVersion}
	return e, view, in
}

func TestGraphOwnerGuidanceUsesExactPublicRouteWithoutRearmingOrReplacingSibling(t *testing.T) {
	e, before, in := graphGuidanceOwnerFixture(t)
	ctx := context.Background()
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := graphGuardArtifactNames(t, e)
	result := e.MutateGraph(ctx, "graphs.nodes.guidance", before.Graph.ID+"/a", body)
	if result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The exact public graph guidance route did not append its captured version", result)
	}
	after, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || !reflect.DeepEqual(before.Budget, after.Budget) || before.DispatchAuthority.Generation != after.DispatchAuthority.Generation {
		t.Fatal("Guidance refilled allowance or registered fresh execution", after, err)
	}
	for i, node := range after.Nodes {
		if node.Key == "a" {
			if node.TaskVersion != "2" || node.InputID != nil || node.LatestAttemptID != nil {
				t.Fatal("Guidance retained obsolete input or started an agent", node)
			}
			detail, err := e.db.Task(ctx, node.TaskID)
			if err != nil || detail.Task.OriginalText != "Complete unchanged receiver original 東京\r\nPreserve every byte and the exact captured choices." || detail.Version.GuidanceText != in.GuidanceText {
				t.Fatal("Guidance changed the original or shortened complete added text", detail, err)
			}
		} else if !reflect.DeepEqual(before.Nodes[i], node) {
			t.Fatal("Guidance replaced the independent sibling", before.Nodes[i], node)
		}
	}
	wire := func(value domain.Result) any {
		t.Helper()
		encoded, err := json.Marshal(value.Envelope)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err = json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	if replay := e.MutateGraph(ctx, "graphs.nodes.guidance", before.Graph.ID+"/a", body); replay.Status != 200 || !reflect.DeepEqual(wire(replay), wire(result)) {
		t.Fatal("Exact guidance replay followed now-stale version proofs", replay, result)
	}
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 || !reflect.DeepEqual(artifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Guidance or its receipt replay created preparation, model work or artifacts", pending, err)
	}
}

func TestGraphOwnerRunReportsActuallyStaleGuidedVersionBeforeOrdinaryPrerequisites(t *testing.T) {
	e, before, in := graphGuidanceOwnerFixture(t)
	ctx := context.Background()
	updated := e.db.UpdateGraphNodeGuidance(ctx, e.config.OwnerInstanceID, before.Graph.ID, "a", in, domain.GraphAdvanceFacts{})
	if updated.Status != 200 {
		t.Fatal("Real graph guidance prerequisite failed", updated)
	}
	result := e.StartAttempt(ctx, before.Nodes[0].TaskID, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: in.ExpectedRevision, TaskVersion: "1", ExecutionMode: "fake"})
	if result.Status != 409 || result.Envelope.Error == nil || result.Envelope.Error.Code != "graph_input_stale" || result.Envelope.Error.GraphRef == nil || result.Envelope.Error.GraphRef.GraphID != before.Graph.ID {
		t.Fatal("The owner hid the stale graph version behind ordinary run scope", result)
	}
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatal("A stale ordinary Run created graph work", pending, err)
	}
}

func TestGraphOwnerCapturesGuidedWorkflowAndRoutingChoicesFromActualNewInput(t *testing.T) {
	e, before, in := graphGuidanceOwnerFixture(t)
	ctx := context.Background()
	if result := e.db.UpdateGraphNodeGuidance(ctx, e.config.OwnerInstanceID, before.Graph.ID, "a", in, domain.GraphAdvanceFacts{}); result.Status != 200 {
		t.Fatal(result)
	}
	view, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	advance := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		advance.NodeAuthorities = append(advance.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	if result := e.db.AdvanceGraph(ctx, e.config.OwnerInstanceID, view.Graph.ID, advance, domain.GraphAdvanceFacts{}); result.Status != 200 {
		t.Fatal(result)
	}
	root, err := e.db.GraphRoot(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Register this test's actual V2 input directly from fresh explicit Store
	// authority. Public Advance/controller preparation is a separate seam:
	// guidance intentionally leaves the node blocked until that explicit action.
	registered, err := e.db.Graph(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	inputDefinition, err := e.db.GraphDefinitionPrivate(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, inputDefinition.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := e.verifyGraphRootClosure(ctx, root, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	var receiving domain.GraphNodeSummary
	for _, node := range registered.Nodes {
		if node.Key == "a" {
			receiving = node
		}
	}
	vector := []domain.GraphContributorBinding{}
	preparation := domain.GraphInputPreparationFacts{Trigger: domain.GraphFreshTrigger{Kind: "explicit", ID: advance.RequestID, OwnerGeneration: registered.DispatchAuthority.Generation},
		RootBinding: root, Contributions: []domain.AcceptedContribution{}, ContributorVector: vector, VerifiedArtifacts: proofs,
		Control: domain.GraphDispatchBinding{GraphID: registered.Graph.ID, GraphVersion: registered.Graph.GraphVersion, RootTaskID: registered.Graph.RootTaskID, OperationAttemptID: root.OperationAttemptID,
			OwnerInstanceID: e.config.OwnerInstanceID, Generation: registered.DispatchAuthority.Generation, GraphControlRevision: registered.Graph.ControlRevision, BudgetAuthorityEpoch: registered.Graph.BudgetAuthorityEpoch,
			WindowNumber: registered.Budget.Budget.CurrentWindow.Number, NodeID: &receiving.ID, TaskID: &receiving.TaskID, TaskVersion: &receiving.TaskVersion, NodeControlRevision: &receiving.ControlRevision, ContributorVector: vector}}
	if _, err = e.db.BeginGraphInput(ctx, e.config.OwnerInstanceID, registered.Graph.ID, receiving.ID, preparation); err != nil {
		t.Fatal("Actual captured V2 input registration failed", err)
	}
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 1 || pending[0].ExecutorKind != "local_graph" {
		t.Fatal("The new captured version did not register its real local input", pending, err)
	}
	local, err := e.db.ClaimAttempt(ctx, pending[0].ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, local)
	settled, err := e.db.Attempt(ctx, local.ID)
	if err != nil || settled.Phase != "settled" || settled.Outcome == nil || *settled.Outcome != "succeeded" {
		t.Fatal("Actual V2 local input prerequisite did not complete", settled, err)
	}
	view, err = e.db.Graph(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Nodes {
		if node.Key != "a" {
			continue
		}
		if node.TaskVersion != "2" || node.InputID == nil || node.State != "ready" {
			t.Fatal("Actual V2 ready-input prerequisite was not retained", node)
		}
		control := domain.GraphDispatchBinding{GraphID: view.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID, OwnerInstanceID: e.config.OwnerInstanceID,
			Generation: view.DispatchAuthority.Generation, GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
			NodeID: &node.ID, TaskID: &node.TaskID, TaskVersion: &node.TaskVersion, NodeControlRevision: &node.ControlRevision}
		facts, err := e.graphInitialAdmissionFacts(ctx, definition, node, control)
		if err != nil || facts.Capture == nil || facts.Capture.Authority.Review != "required" || facts.Capture.Authority.OptionSnapshot.Checks == nil || len(*facts.Capture.Authority.OptionSnapshot.Checks) != 0 || facts.Capture.Authority.OptionSnapshot.MaxCorrectionCycles == nil || *facts.Capture.Authority.OptionSnapshot.MaxCorrectionCycles != 0 {
			t.Fatal("The owner replaced captured guided workflow choices with the initial declaration", facts.Capture, err)
		}
		if facts.Routing.Assessment.Classification != "routine" || facts.Routing.Assessment.Source != "user_declared" {
			t.Fatal("The owner ignored the newly captured routing suitability hint", facts.Routing.Assessment)
		}
	}
}
