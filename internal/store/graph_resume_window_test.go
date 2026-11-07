package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphAdvanceReplacementWindowCanBeginWithRequiredNativeReview(t *testing.T) {
	s, prior, _, before, stage, owner := graphReviewResumeFixture(t, false, 120)
	ctx := context.Background()
	if before.Budget.Budget.CurrentWindow.Number != "2" || before.Budget.Budget.CurrentWindow.FirstAdmissionAt != nil || before.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("fixture did not grant an unopened replacement window", before.Budget)
	}
	in := graphAdvanceInput(before)
	result := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Stages: []domain.WorkflowStepAdmissionFacts{stage}})
	if result.Status != 200 {
		t.Fatal("replacement window required an unnecessary author before its actual next review", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || len(data.Dispatches) != 1 || data.Dispatches[0].Dispatch.Role != "review" || data.Dispatches[0].Dispatch.WorkflowID != prior.Dispatch.WorkflowID || data.Dispatches[0].Dispatch.WindowNumber != "2" || data.Budget.Budget.ConsumedAdmissions != "2" || data.Budget.Budget.CurrentWindow.FirstAdmissionAt == nil || data.Budget.Budget.CurrentWindow.DeadlineAt == nil {
		t.Fatal("next actual native admission did not open exactly its replacement common window", data, err)
	}
	if replay := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{}); replay.Status != 200 {
		t.Fatal(replay)
	}
	current, err := s.GraphBudget(ctx, before.Graph.ID)
	if err != nil || current.Budget.ConsumedAdmissions != "2" || *current.Budget.CurrentWindow.FirstAdmissionAt != *data.Budget.Budget.CurrentWindow.FirstAdmissionAt || *current.Budget.CurrentWindow.DeadlineAt != *data.Budget.Budget.CurrentWindow.DeadlineAt {
		t.Fatal("replacement-window replay reopened or spent again", current, err)
	}
}

func TestGraphAdvanceUnopenedWindowLocalCheckUsesFixedPreparationDeadline(t *testing.T) {
	s, prior, author, originalStage, owner := graphLocalCheckStageFixture(t)
	ctx := context.Background()
	before, err := s.Graph(ctx, prior.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	seconds := 120
	grant := graphGrantInput(before.Graph)
	grant.NewWindowSeconds = &seconds
	if result := s.AmendGraphBudget(ctx, owner, before.Graph.ID, grant); result.Status != 200 {
		t.Fatal(result)
	}
	before, err = s.Graph(ctx, before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixed := mustGraphHeader(t, s, before.Graph.ID).PreparationDeadlineAt
	stage := graphReviewAdmissionFacts(t, s, prior, author)
	stage.Plan = originalStage.Plan
	in := graphAdvanceInput(before)
	result := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Stages: []domain.WorkflowStepAdmissionFacts{stage}})
	if result.Status != 200 {
		t.Fatal("cheap explicit local check required a new model merely to open the clock", result)
	}
	data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if err != nil || len(data.Dispatches) != 1 || data.Dispatches[0].Dispatch.ExecutorKind != "local_check" || data.Dispatches[0].Dispatch.NativeConsumptionKey != nil || data.Budget.Budget.ConsumedAdmissions != "1" || data.Budget.Budget.CurrentWindow.FirstAdmissionAt != nil || data.Budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("local check spent a model unit or opened the native clock", data, err)
	}
	d := data.Dispatches[0]
	a, err := s.Attempt(ctx, d.Dispatch.AttemptID)
	if err != nil || a.DeadlineAt != fixed {
		t.Fatal("local stage renewed the fixed preparation deadline", a, fixed, err)
	}
	root, err := s.GraphRoot(ctx, before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	materialization := graphMaterializationForDispatch(t, s, d, domain.GraphNodeAdmissionFacts{InputBinding: stage.Graph.InputBinding, VerifiedArtifacts: stage.Graph.VerifiedArtifacts}, root, owner)
	if err = s.CheckGraphDispatchPreparation(ctx, owner, a.ID); err != nil {
		t.Fatal("owned local preparation could not use its fixed unopened-window allowance", err)
	}
	if err = s.RecordGraphMaterialization(ctx, owner, a.ID, materialization); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordLaunchIntent(ctx, a.ID); err != nil {
		t.Fatal("direct local command could not start inside its explicit fixed deadline", err)
	}
	current, err := s.GraphBudget(ctx, before.Graph.ID)
	if err != nil || current.Budget.ConsumedAdmissions != "1" || current.Budget.CurrentWindow.FirstAdmissionAt != nil || current.Budget.CurrentWindow.DeadlineAt != nil || mustGraphHeader(t, s, before.Graph.ID).PreparationDeadlineAt != fixed {
		t.Fatal("local preparation/launch opened or renewed model time", current, err)
	}
}
