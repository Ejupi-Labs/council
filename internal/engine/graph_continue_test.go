package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphCurrentOwnerAdmitsReadyIndependentNodesOnceFromRealCompletion(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	// The actor stays stopped: only the named real completion callback below
	// may admit work. No native worker or successful process is fabricated.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	creation, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{
		RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake",
		Nodes: []domain.GraphNodeInput{
			{Key: "a", OriginalText: "Create the first independent controlled result.", FakeScenario: "graph-branch-a"},
			{Key: "b", OriginalText: "Create the second independent controlled result.", FakeScenario: "graph-branch-b"},
		},
	}, creation)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission := created.Envelope.Data.(domain.GraphAdmission)
	t.Cleanup(func() {
		view, readErr := e.db.Graph(context.Background(), admission.Graph.ID)
		if readErr != nil {
			t.Error(readErr)
			return
		}
		for _, node := range view.Nodes {
			if node.LatestAttemptID == nil {
				continue
			}
			a, readErr := e.db.Attempt(context.Background(), *node.LatestAttemptID)
			if readErr != nil || a.Phase == "settled" {
				continue
			}
			_, settleErr := e.db.SettleWorkflowStep(context.Background(), a.ID, domain.WorkflowStepFacts{StepID: a.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
				Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"},
					Failure: primary("fixture_no_process", "This admission-only check created no native process.")}})
			if settleErr != nil {
				t.Error(settleErr)
			}
		}
	})
	basis, err := e.db.ClaimAttempt(ctx, admission.PreparationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, basis)
	e.continueGraphInputs(basis.ID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 2 {
		t.Fatal("Real root completion did not register two local inputs", pending, err)
	}
	source := ""
	for _, a := range pending {
		if a.ExecutorKind != "local_graph" {
			t.Fatal("Input setup unexpectedly launched native work", a)
		}
		claimed, claimErr := e.db.ClaimAttempt(ctx, a.ID, e.config.OwnerInstanceID)
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		e.run(ctx, claimed)
		source = a.ID
	}
	before, err := e.db.Graph(ctx, admission.Graph.ID)
	if err != nil || before.Budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal(before, err)
	}
	for _, node := range before.Nodes {
		if node.InputID == nil || node.State != "ready" {
			t.Fatal("The actual input operation did not produce a ready receiver", node)
		}
	}
	e.continueGraphAdmissions(source)
	after, err := e.db.Graph(ctx, admission.Graph.ID)
	if err != nil || after.Budget.Budget.ConsumedAdmissions != "2" || len(after.ActiveClaims) != 2 {
		t.Fatal("Actual current-owner completion did not atomically admit both ready receivers", after, err)
	}
	contexts := map[string]bool{}
	for i, node := range after.Nodes {
		if node.InputID == nil || *node.InputID != *before.Nodes[i].InputID || node.NodeAdmissionID == nil || node.LatestAttemptID == nil || node.State != "queued" {
			t.Fatal("Admission changed exact input or omitted its node dispatch", node)
		}
		record, readErr := e.db.GraphDispatchForAttempt(ctx, *node.LatestAttemptID)
		if readErr != nil || record.Public.Dispatch.InputID != *node.InputID || record.Public.Dispatch.FreshTriggerID != source {
			t.Fatal(record, readErr)
		}
	}
	for _, claim := range after.ActiveClaims {
		if claim.ExecutorKind != "native" || claim.ContextID == nil || contexts[*claim.ContextID] {
			t.Fatal("Two admissions reused a native context reservation", after.ActiveClaims)
		}
		contexts[*claim.ContextID] = true
	}
	e.continueGraphAdmissions(source)
	replayed, err := e.db.Graph(ctx, admission.Graph.ID)
	if err != nil || replayed.Budget.Budget.ConsumedAdmissions != "2" || len(replayed.ActiveClaims) != 2 {
		t.Fatal("Re-observed completion duplicated native work or allowance", replayed, err)
	}
	for i, node := range replayed.Nodes {
		if node.LatestAttemptID == nil || *node.LatestAttemptID != *after.Nodes[i].LatestAttemptID {
			t.Fatal("Re-observed completion replaced an admitted receiver", node)
		}
	}
}
