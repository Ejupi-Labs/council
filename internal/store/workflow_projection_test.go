package store

import (
	"context"
	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
	"testing"
)

// This goes through real DAO admission/settlement/acceptance with explicitly
// controlled metadata proofs. It qualifies read projection/replay, not OS work.
func TestWorkflowCurrentTaskSummaryCompletionHistoricalAndReceiptIsolation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, owner, authorFacts := workflowAuthorSettlementFixture(t, s)
	var receiptBefore string
	if e := s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", a.Attempt.StartRequestID).Scan(&receiptBefore); e != nil {
		t.Fatal(e)
	}
	w, e := s.SettleWorkflowStep(ctx, a.Attempt.ID, authorFacts)
	if e != nil {
		t.Fatal(e)
	}
	predecessor, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil || predecessor.AdvanceProof == nil {
		t.Fatal(predecessor, e)
	}
	definition := w.Authority.Checks[0]
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: predecessor.AdvanceProof.NextLogicalKey, DefinitionID: &definition.ID, InputCandidateID: w.SelectedCandidateID, PredecessorStepID: &predecessor.ID}
	admitted, e := s.ContinueWorkflow(ctx, owner, predecessor.ID, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: "1", ExpectedWorkflowRevision: w.Revision})
	if e != nil || admitted == nil {
		t.Fatal(admitted, e)
	}
	if _, e = s.ClaimAttempt(ctx, admitted.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	app := domain.Applicability{ScopeKind: "whole_candidate", Paths: []string{}, ConditionKeys: definition.CoveredConditionKeys, DefinitionSHA256: definition.DefinitionSHA256, BasisSHA256: strings.Repeat("b", 64)}
	cleanup := domain.Cleanup{State: "verified", Scope: "not_launched"}
	check := domain.CheckReceipt{ID: domain.NewID(), WorkflowID: w.ID, StepID: admitted.Step.ID, AttemptID: admitted.Attempt.ID, DefinitionID: definition.ID, DefinitionSHA256: definition.DefinitionSHA256, InputCandidateBinding: authorFacts.CandidateBinding, Applicability: app, Outcome: "passed", BuiltinCompleted: true, Cleanup: cleanup, CreatedAt: domain.Now()}
	detail, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	facts := domain.WorkflowStepFacts{StepID: admitted.Step.ID, OwnerInstanceID: owner, TaskRevision: detail.Task.Revision, WorkflowRevision: admitted.Workflow.Revision, CandidateBinding: authorFacts.CandidateBinding, VerifiedArtifacts: authorFacts.VerifiedArtifacts, DirectProven: true, HasTaskDelta: true, Applicability: &app, Check: &check, Settlement: domain.Settlement{Outcome: "succeeded", Cleanup: cleanup, EvidenceStatus: "complete", BuiltinCompleted: true}}
	w, e = s.SettleWorkflowStep(ctx, admitted.Attempt.ID, facts)
	if e != nil {
		t.Fatal(e)
	}
	detail, e = s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	digest, invalid := domain.WorkflowAuthoritySHA256(w.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	requestID := domain.NewID()
	record := domain.WorkflowAcceptance{ID: domain.NewID(), WorkflowID: w.ID, TaskID: a.Task.ID, TaskVersion: w.TaskVersion, CandidateBinding: *authorFacts.CandidateBinding, AuthoritySHA256: digest, ReviewUses: []domain.EvidenceUse{}, CheckUses: w.EvidenceUses, FindingResolutionIDs: []string{}, ExternalResultRefs: []string{}, DeliveryResult: "review_only", AcceptedByRequestID: requestID}
	accepted := s.AcceptWorkflow(ctx, a.Task.ID, domain.WorkflowAcceptanceFacts{AutomaticPredecessorStepID: admitted.Step.ID, RequestID: requestID, ExpectedTaskRevision: detail.Task.Revision, ExpectedWorkflowRevision: w.Revision, OwnerInstanceID: owner, VerifiedArtifacts: authorFacts.VerifiedArtifacts, Acceptance: record})
	if accepted.Status != 200 {
		t.Fatal(accepted)
	}
	page, e := s.Tasks(ctx, a.Task.ProjectID, 100, "")
	if e != nil || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
	summary := page.Items[0]
	if summary.Status != "completed" || summary.LatestAttemptID == nil || *summary.LatestAttemptID != admitted.Attempt.ID || *summary.LatestAttemptID == a.Attempt.ID || !strings.Contains(summary.NextAction, "review") {
		t.Fatal("current read contradicts exact workflow acceptance or changes Stop target", summary)
	}
	historical, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{WorkflowID: w.ID})
	if e != nil || historical.Acceptance == nil {
		t.Fatal(historical, e)
	}
	historicalBytes, e := json.Marshal(historical)
	if e != nil {
		t.Fatal(e)
	}
	detail, e = s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	capture := domain.WorkflowAdmissionFacts{Routing: wrapperRoutingFacts("fake"), Capture: &domain.WorkflowCapture{Authority: w.Authority}}
	guidance := domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, GuidanceText: "Preserve the revised future authority."}
	workflowRecaptureGuidance(t, detail.Version, detail.Task.OriginalText, guidance.GuidanceText, &capture)
	if changed := s.UpdateGuidanceWithWorkflow(ctx, a.Task.ID, guidance, capture); changed.Status != 200 {
		t.Fatal(changed)
	}
	page, e = s.Tasks(ctx, a.Task.ProjectID, 100, "")
	if e != nil || page.Items[0].Status == "completed" {
		t.Fatal("historical acceptance completed a new version", page, e)
	}
	historicalAgain, e := s.Workflow(ctx, a.Task.ID, domain.WorkflowSelector{WorkflowID: w.ID})
	if e != nil {
		t.Fatal(e)
	}
	after, e := json.Marshal(historicalAgain)
	if e != nil || string(after) != string(historicalBytes) {
		t.Fatal("current projection rewrote history", historicalAgain, e)
	}
	var receiptAfter string
	if e = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", a.Attempt.StartRequestID).Scan(&receiptAfter); e != nil || receiptBefore != receiptAfter {
		t.Fatal("current projection changed exact original receipt", e)
	}
}
