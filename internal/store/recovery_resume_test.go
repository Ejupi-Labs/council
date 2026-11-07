package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type recoveryResumeResult struct {
	Checkpoint         domain.RecoveryCheckpoint `json:"checkpoint"`
	AdmittedAttemptIDs []string                  `json:"admittedAttemptIds"`
	Blockers           []string                  `json:"blockers"`
	NextAction         string                    `json:"nextAction"`
}

func TestRecoveryResumeAtomicallyAdmitsPreparedStageOnce(t *testing.T) {
	ctx := context.Background()
	s, admission, owner, workflow, cp := recoverySettledPausedTask(t)
	step, err := s.WorkflowStep(ctx, admission.Step.ID)
	if err != nil || step.AdvanceProof == nil {
		t.Fatal("fixture lacks a current predecessor", step, err)
	}
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: step.AdvanceProof.NextLogicalKey, Cycle: 0, DefinitionID: &workflow.Authority.Checks[0].ID, InputCandidateID: workflow.SelectedCandidateID, PredecessorStepID: &step.ID}
	facts := domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: cp.Scope.ExpectedTaskRevision, ExpectedWorkflowRevision: workflow.Revision}
	input := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: cp.Revision, Scope: cp.Scope}
	s.AfterCommit = func() error { return errors.New("resume acknowledgement lost after atomic admission") }
	lost := s.ResumeCheckpoint(ctx, owner, cp.ID, input, RecoveryResumeFacts{Workflow: &facts})
	s.AfterCommit = nil
	if lost.Envelope.Error == nil || lost.Envelope.Error.Code != "write_uncertain" {
		t.Fatal("lost atomic acknowledgement was not reported", lost)
	}
	receipt := recoveryResumeReceipt(t, s.ResumeCheckpoint(ctx, domain.NewID(), cp.ID, input))
	if len(receipt.AdmittedAttemptIDs) != 1 || receipt.AdmittedAttemptIDs[0] == admission.Attempt.ID || !reflect.DeepEqual(receipt.Checkpoint.CompletedAttemptIDs, cp.CompletedAttemptIDs) {
		t.Fatal("atomic receipt lost its distinct unfinished work", receipt)
	}
	if _, attempts, steps, _ := workflowCounts(t, s); attempts != 2 || steps != 2 {
		t.Fatal("same resume admitted a duplicate or lost its new stage", attempts, steps)
	}
	if r := s.ResumeCheckpoint(ctx, owner, cp.ID, input, RecoveryResumeFacts{Workflow: &facts}); r.Envelope.Error != nil {
		t.Fatal("identical resume receipt did not replay", r)
	}
	if _, attempts, steps, _ := workflowCounts(t, s); attempts != 2 || steps != 2 {
		t.Fatal("receipt replay duplicated work", attempts, steps)
	}
}

func recoveryResumeReceipt(t *testing.T, result domain.Result) recoveryResumeResult {
	t.Helper()
	if result.Status != 200 || result.Envelope.Error != nil || result.Envelope.Receipt == nil {
		t.Fatalf("resume did not return its durable receipt: %+v", result)
	}
	value, err := decode[recoveryResumeResult](encode(result.Envelope.Data))
	if err != nil || value.Checkpoint.State != "resumed" {
		t.Fatalf("resume did not retain its resumed checkpoint: %+v, %v", result, err)
	}
	return value
}

func recoverySettledPausedTask(t *testing.T) (*Store, domain.Admission, string, domain.Workflow, domain.RecoveryCheckpoint) {
	t.Helper()
	ctx := context.Background()
	s := testStore(t)
	admission, owner, facts := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	workflow, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, facts)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Task(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	pause := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision}
	cp := recoveryPauseResult(t, s.PauseTask(ctx, owner, task.Task.ID, pause)).Checkpoint
	if cp.State != "paused" || !recoveryContains(cp.CompletedAttemptIDs, admission.Attempt.ID) {
		t.Fatal("settled task did not yield a durable paused checkpoint", cp)
	}
	return s, admission, owner, workflow, cp
}

func TestRecoveryResumeLostAcknowledgementDoesNotReplayCompletedWork(t *testing.T) {
	ctx := context.Background()
	s, admission, owner, workflow, cp := recoverySettledPausedTask(t)
	step, err := s.WorkflowStep(ctx, admission.Step.ID)
	if err != nil || step.AdvanceProof == nil {
		t.Fatal("fixture lacks a captured pre-pause continuation", step, err)
	}
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: step.AdvanceProof.NextLogicalKey, Cycle: 0, DefinitionID: &workflow.Authority.Checks[0].ID, InputCandidateID: workflow.SelectedCandidateID, PredecessorStepID: &step.ID}
	facts := domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: step.AdvanceProof.TaskRevision, ExpectedWorkflowRevision: workflow.Revision}
	input := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: cp.Revision, Scope: cp.Scope}
	s.AfterCommit = func() error { return errors.New("simulated lost resume acknowledgement") }
	lost := s.ResumeCheckpoint(ctx, owner, cp.ID, input)
	s.AfterCommit = nil
	if lost.Envelope.Error == nil || lost.Envelope.Error.Code != "write_uncertain" || lost.Envelope.Receipt != nil {
		t.Fatal("resume did not retain uncertain acknowledgement", lost)
	}
	first := s.ResumeCheckpoint(ctx, owner, cp.ID, input)
	resumed := recoveryResumeReceipt(t, first)
	if !reflect.DeepEqual(resumed.Checkpoint.CompletedAttemptIDs, cp.CompletedAttemptIDs) || resumed.NextAction == "" {
		t.Fatal("resume changed completed identities or omitted the next action", resumed)
	}
	replay := s.ResumeCheckpoint(ctx, domain.NewID(), cp.ID, input)
	if repeated := recoveryResumeReceipt(t, replay); !reflect.DeepEqual(resumed, repeated) || !reflect.DeepEqual(first.Envelope.Receipt, replay.Envelope.Receipt) {
		t.Fatal("resume retry rechecked newer ownership or repeated effects", first, replay)
	}
	changed := input
	changed.ExpectedCheckpointRevision = resumed.Checkpoint.Revision
	if conflict := s.ResumeCheckpoint(ctx, owner, cp.ID, changed); conflict.Envelope.Error == nil || conflict.Envelope.Error.Code != "request_conflict" {
		t.Fatal("reused resume identity changed its input", conflict)
	}
	if next, err := s.ContinueWorkflow(ctx, owner, step.ID, facts); err == nil && next != nil {
		t.Fatal("resume revived a pre-pause continuation", next)
	}
	if _, attempts, steps, _ := workflowCounts(t, s); attempts != 1 || steps != 1 {
		t.Fatal("resume or a stale callback created work", attempts, steps)
	}
}

func TestRecoveryResumeRejectsStaleCheckpointAndScope(t *testing.T) {
	ctx := context.Background()
	s, _, owner, _, cp := recoverySettledPausedTask(t)
	stale := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: "999", Scope: cp.Scope}
	if r := s.ResumeCheckpoint(ctx, owner, cp.ID, stale); r.Envelope.Error == nil {
		t.Fatal("stale checkpoint revision was accepted", r)
	}
	stale.RequestID = domain.NewID()
	stale.ExpectedCheckpointRevision = cp.Revision
	stale.Scope.ExpectedTaskRevision = "999"
	if r := s.ResumeCheckpoint(ctx, owner, cp.ID, stale); r.Envelope.Error == nil {
		t.Fatal("stale task authority was accepted", r)
	}
	current, err := s.Checkpoint(ctx, cp.ID)
	if err != nil || current.State != "paused" || current.Revision != cp.Revision {
		t.Fatal("rejected resume changed the checkpoint", current, err)
	}
}

func TestRecoveryMemberResumePreservesOverlappingGroupPause(t *testing.T) {
	ctx := context.Background()
	s, graph, _, _, owner := graphReadyBasis(t)
	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorities := make([]domain.GraphNodeAuthority, 0, len(view.Nodes))
	for _, node := range view.Nodes {
		authorities = append(authorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	groupInput := domain.PauseGraphInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: authorities}
	group := recoveryPauseResult(t, s.PauseGraph(ctx, owner, graph.Graph.ID, groupInput)).Checkpoint
	member := view.Nodes[0]
	memberInput := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: member.TaskVersion, ExpectedTaskRevision: member.TaskRevision}
	pausedMember := recoveryPauseResult(t, s.PauseTask(ctx, owner, member.TaskID, memberInput)).Checkpoint
	if group.State != "paused" || pausedMember.State != "paused" {
		t.Fatal("settled scopes were not paused", group, pausedMember)
	}
	resume := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: pausedMember.Revision, Scope: pausedMember.Scope}
	result := recoveryResumeReceipt(t, s.ResumeCheckpoint(ctx, owner, pausedMember.ID, resume))
	if !recoveryContains(result.Blockers, group.ID) {
		t.Fatal("member resume concealed the active group pause", result)
	}
	groupAfter, err := s.Checkpoint(ctx, group.ID)
	if err != nil || groupAfter.State != "paused" || groupAfter.Revision != group.Revision {
		t.Fatal("member resume changed the group checkpoint", groupAfter, err)
	}
	if err = s.requireRecoveryAdmission(ctx, s.db, "", graph.Graph.ID, ""); err == nil {
		t.Fatal("member resume opened admission through the group fence")
	}
}

// The prerequisite is actually accepted, so readiness alone would otherwise
// allow B. C is independent and must not be rolled back with blocked B.
func TestRecoveryGroupResumeSkipsPausedAncestorDependent(t *testing.T) {
	f := graphInitialAncestorStopFixture(t)
	s, ctx := f.store, context.Background()
	before, err := s.Graph(ctx, f.view.Graph.ID)
	if err != nil || before.Nodes[0].AcceptedResult == nil {
		t.Fatal("accepted prerequisite is required", before, err)
	}
	ancestor := before.Nodes[0]
	paused := s.PauseTask(ctx, f.owner, ancestor.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: ancestor.TaskVersion, ExpectedTaskRevision: ancestor.TaskRevision})
	ancestorCheckpoint := recoveryPauseResult(t, paused).Checkpoint
	before, err = s.Graph(ctx, f.view.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	groupPause := domain.PauseGraphInput{RequestID: domain.NewID(), GraphVersion: before.Graph.GraphVersion, ExpectedGraphControlRevision: before.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: before.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: before.Budget.Budget.CurrentWindow.Number, NodeAuthorities: graphAdvanceInput(before).NodeAuthorities}
	group := recoveryPauseResult(t, s.PauseGraph(ctx, f.owner, before.Graph.ID, groupPause)).Checkpoint
	if group.State != "paused" {
		t.Fatal("settled group did not pause", group)
	}
	resume := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: group.Revision, Scope: group.Scope}
	advance := graphAdvanceInput(before)
	advance.RequestID = resume.RequestID
	b := graphInitialAncestorCurrentFact(t, f.b, before)
	c := graphInitialAncestorCurrentFact(t, f.c, before)
	facts := domain.GraphAdvanceFacts{Admissions: []domain.GraphNodeAdmissionFacts{b, c}}
	result := recoveryResumeReceipt(t, s.ResumeCheckpoint(ctx, f.owner, group.ID, resume, RecoveryResumeFacts{GraphInput: &advance, Graph: &facts}))
	if len(result.AdmittedAttemptIDs) != 1 || !recoveryContains(result.Blockers, ancestorCheckpoint.ID) {
		t.Fatal("group resume did not admit only the eligible sibling", result)
	}
	after, err := s.Graph(ctx, before.Graph.ID)
	if err != nil || after.Nodes[1].NodeAdmissionID != nil || after.Nodes[2].NodeAdmissionID == nil || after.Nodes[2].LatestAttemptID == nil || *after.Nodes[2].LatestAttemptID != result.AdmittedAttemptIDs[0] || !reflect.DeepEqual(after.Nodes[0].AcceptedResult, before.Nodes[0].AcceptedResult) {
		t.Fatal("group resume admitted a blocked dependent or lost accepted work", before, after, err)
	}
	if after.Budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("resume debited anything other than the independent sibling", after.Budget)
	}
}
