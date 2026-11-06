package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
	"testing"
)

func TestWorkflowUnavailableCheckRetainsNonqualifyingReceipt(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, owner, author := workflowAuthorSettlementFixture(t, s)
	w, e := s.SettleWorkflowStep(ctx, a.Attempt.ID, author)
	if e != nil {
		t.Fatal(e)
	}
	previous, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil {
		t.Fatal(e)
	}
	definition := w.Authority.Checks[0]
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: previous.AdvanceProof.NextLogicalKey, DefinitionID: &definition.ID, InputCandidateID: w.SelectedCandidateID, PredecessorStepID: &previous.ID}
	next, e := s.ContinueWorkflow(ctx, owner, previous.ID, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: "1", ExpectedWorkflowRevision: w.Revision})
	if e != nil || next == nil {
		t.Fatal(next, e)
	}
	if _, e = s.ClaimAttempt(ctx, next.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	detail, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	app := domain.Applicability{ScopeKind: "whole_candidate", Paths: []string{}, ConditionKeys: definition.CoveredConditionKeys, DefinitionSHA256: definition.DefinitionSHA256, BasisSHA256: strings.Repeat("b", 64)}
	cleanup := domain.Cleanup{State: "verified", Scope: "not_launched"}
	receipt := domain.CheckReceipt{ID: domain.NewID(), WorkflowID: w.ID, StepID: next.Step.ID, AttemptID: next.Attempt.ID, DefinitionID: definition.ID, DefinitionSHA256: definition.DefinitionSHA256, InputCandidateBinding: author.CandidateBinding, Applicability: app, Outcome: "unavailable", Cleanup: cleanup, CreatedAt: domain.Now()}
	facts := domain.WorkflowStepFacts{StepID: next.Step.ID, OwnerInstanceID: owner, TaskRevision: detail.Task.Revision, WorkflowRevision: next.Workflow.Revision, CandidateBinding: author.CandidateBinding, Applicability: &app, Check: &receipt, VerifiedArtifacts: author.VerifiedArtifacts, Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, Cleanup: cleanup, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "check_unavailable", Message: "The actual no-child proof did not complete."}}}
	observed, e := s.SettleWorkflowStep(ctx, next.Attempt.ID, facts)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := s.WorkflowCheck(ctx, receipt.ID)
	if e != nil || saved.Outcome != "unavailable" || saved.BuiltinCompleted || saved.ExitCode != nil || saved.Finding != nil {
		t.Fatal("unavailable receipt vanished or became observed proof/finding", saved, e)
	}
	step, e := s.WorkflowStep(ctx, next.Step.ID)
	if e != nil || step.CheckID == nil || *step.CheckID != receipt.ID || step.AdvanceProof != nil || observed.State == "completed" || observed.LatestAcceptanceID != nil {
		t.Fatal("nonqualifying receipt created execution/acceptance authority", step, observed, e)
	}
}
