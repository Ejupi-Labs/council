package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphReleaseReadyGroup(t *testing.T, e *Engine, projectID string, definitions ...domain.GraphStartInput) (domain.GraphAdmission, string) {
	t.Helper()
	ctx := context.Background()
	facts, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	definition := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake",
		Nodes: []domain.GraphNodeInput{{Key: "a", OriginalText: "Produce the first independent controlled result.", FakeScenario: "graph-branch-a"}, {Key: "b", OriginalText: "Produce the second independent controlled result.", FakeScenario: "graph-branch-b"}}}
	if len(definitions) > 1 {
		t.Fatal("Select one complete immutable graph declaration")
	}
	if len(definitions) == 1 {
		definition = definitions[0]
		if definition.RequestID == "" {
			definition.RequestID = domain.NewID()
		}
		if definition.ProjectID == "" {
			definition.ProjectID = projectID
		}
		if definition.ExecutionMode == "" {
			definition.ExecutionMode = "fake"
		}
	}
	expectedInputs := len(definition.Nodes)
	if definition.Dependencies != nil {
		receivers := map[string]bool{}
		for _, edge := range *definition.Dependencies {
			receivers[edge.To] = true
		}
		expectedInputs -= len(receivers)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, definition, facts)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	group := created.Envelope.Data.(domain.GraphAdmission)
	t.Cleanup(func() {
		view, err := e.db.Graph(ctx, group.Graph.ID)
		if err != nil {
			t.Error(err)
			return
		}
		for _, node := range view.Nodes {
			if node.LatestAttemptID == nil {
				continue
			}
			a, err := e.db.Attempt(ctx, *node.LatestAttemptID)
			if err != nil || a.Phase == "settled" {
				continue
			}
			_, err = e.db.SettleWorkflowStep(ctx, a.ID, graphReleaseNoProcessFacts(e, a))
			if err != nil {
				t.Error(err)
			}
		}
	})
	root, err := e.db.ClaimAttempt(ctx, group.PreparationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, root)
	e.continueGraphInputs(root.ID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil {
		t.Fatal(err)
	}
	source, count := "", 0
	for _, queued := range pending {
		if queued.ExecutorKind != "local_graph" || queued.TaskID != group.Graph.RootTaskID || queued.Phase != "queued" {
			continue
		}
		a, err := e.db.ClaimAttempt(ctx, queued.ID, e.config.OwnerInstanceID)
		if err != nil {
			t.Fatal(err)
		}
		e.run(ctx, a)
		terminal, err := e.db.Attempt(ctx, a.ID)
		if err != nil || terminal.Outcome == nil || *terminal.Outcome != "succeeded" {
			t.Fatal("Actual local release fixture failed to retain a ready input", terminal, err)
		}
		source, count = a.ID, count+1
	}
	if count != expectedInputs {
		t.Fatal("The actual group did not retain every independent input", count, expectedInputs)
	}
	return group, source
}

func graphReleaseNoProcessFacts(e *Engine, a domain.Attempt) domain.WorkflowStepFacts {
	return domain.WorkflowStepFacts{StepID: a.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
		Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"},
			Failure: primary("fixture_no_process", "The exact reserved offline attempt created no native process.")}}
}

func TestGraphActualClaimReleaseWakesOtherLiveGroupWithoutRearmingOldCallback(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	first, _ := graphReleaseReadyGroup(t, e, projectID)
	second, source := graphReleaseReadyGroup(t, e, projectID)
	// This completion occurred after both explicit registrations. The live fan-out
	// must consider each exact registered group, rather than only its source group.
	e.continueGraphAdmissions(source)
	views := []domain.GraphDetail{}
	for _, id := range []string{first.Graph.ID, second.Graph.ID} {
		view, err := e.db.Graph(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		views = append(views, view)
	}
	var waiting *domain.GraphDetail
	for i := range views {
		if views[i].Budget.Budget.ConsumedAdmissions == "0" {
			waiting = &views[i]
		}
	}
	if waiting == nil || waiting.DispatchAuthority == nil || waiting.DispatchAuthority.State != "capacity_wait" || len(waiting.Blockers) == 0 || waiting.Blockers[0].Code != "capacity_wait" {
		t.Fatal("The real completion did not observe the other live registration's capacity wait", views)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || len(gate.ActiveClaims) != 2 {
		t.Fatal("Initial live fan-out did not reserve exactly two native slots", gate, err)
	}
	released, err := e.db.ClaimAttempt(ctx, gate.ActiveClaims[0].AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.SettleWorkflowStep(ctx, released.ID, graphReleaseNoProcessFacts(e, released)); err != nil {
		t.Fatal(err)
	}
	// Merely rereading the new state and repeating the old callback cannot turn
	// the old wait observation into new native work after capacity has changed.
	e.continueGraphAdmissions(source)
	unchanged, err := e.db.Graph(ctx, waiting.Graph.ID)
	if err != nil || unchanged.Budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal("An old callback rearmed an already consumed wait observation", unchanged, err)
	}
	e.continueGraphAdmissions(released.ID)
	woken, err := e.db.Graph(ctx, waiting.Graph.ID)
	if err != nil || woken.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("A real unrelated current-owner claim release failed to wake the live group", woken, err)
	}
	admitted := 0
	for _, node := range woken.Nodes {
		if node.LatestAttemptID == nil {
			continue
		}
		d, err := e.db.GraphDispatchForAttempt(ctx, *node.LatestAttemptID)
		if err != nil || d.Public.Dispatch.FreshTriggerID != released.ID {
			t.Fatal("The wake used a historical or invented release identity", d, err)
		}
		admitted++
	}
	if admitted != 1 {
		t.Fatal("One actual slot release admitted more than one waiting receiver", admitted)
	}
}
