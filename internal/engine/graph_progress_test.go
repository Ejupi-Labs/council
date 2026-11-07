package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphCurrentOwnerPreparesIndependentInputsFromRealBasisCompletion(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.ctx, e.cancel = ownerCtx, cancel
	// Stop the actor to inspect the real local-only boundary before its new
	// native-admission callback. The helper runs the actual basis and inputs.
	admission, _ := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{
		Nodes: []domain.GraphNodeInput{{Key: "a", OriginalText: "First complete independent instruction"}, {Key: "b", OriginalText: "Second complete independent instruction"}},
	})
	view, err := e.db.Graph(ctx, admission.Graph.ID)
	if err != nil || len(view.Nodes) != 2 {
		t.Fatal(view, err)
	}
	for _, node := range view.Nodes {
		if node.InputID == nil || node.State != "ready" || node.NodeAdmissionID != nil || node.LatestAttemptID != nil {
			t.Fatal("Real local preparation skipped its inspectable ready-input boundary", node)
		}
	}
	if view.Budget.Budget.ConsumedAdmissions != "0" || view.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("Local progression spent native budget or opened its window", view.Budget)
	}
	for _, node := range view.Nodes {
		input, err := e.db.GraphInput(ctx, admission.Graph.ID, *node.InputID)
		if err != nil || input.Summary.State != "ready" || input.ContextPacket == nil {
			t.Fatal("Progression omitted a receiver's retained complete context", input, err)
		}
	}
	// Re-observing an old event is not a fresh dispatch. The per-target consumption
	// must leave the exact prepared inputs and zero native allowance unchanged.
	e.continueGraphInputs(admission.PreparationAttemptID)
	inputs, err := e.db.GraphInputs(ctx, admission.Graph.ID, 30, "")
	if err != nil || len(inputs.Items) != 3 {
		t.Fatal("Independent input progression duplicated operations", inputs, err)
	}
	stopped := e.MutateGraph(ctx, "graphs.stop", admission.Graph.ID, []byte(`{"requestId":"`+domain.NewID()+`","expectedGraphControlRevision":"`+view.Graph.ControlRevision+`"}`))
	if stopped.Envelope.Error != nil {
		t.Fatal(stopped)
	}
}
