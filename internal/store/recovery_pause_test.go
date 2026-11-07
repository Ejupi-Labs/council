package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These tests exercise real SQLite authority and receipts with fake controller
// facts. Native process and filesystem proof is qualified separately.
func recoveryPauseResult(t *testing.T, r domain.Result) domain.RecoveryPauseResult {
	t.Helper()
	if r.Envelope.Error != nil || r.Envelope.Receipt == nil || (r.Status != 200 && r.Status != 202) {
		t.Fatalf("pause did not return a durable accepted receipt: %+v", r)
	}
	value, err := decode[domain.RecoveryPauseResult](encode(r.Envelope.Data))
	if err != nil || value.Checkpoint.ID == "" {
		t.Fatalf("pause receipt did not retain a checkpoint: %+v, %v", r, err)
	}
	return value
}

func recoveryContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// The shared workflow fixture predates owner registration and starts its fake
// worker directly. Register that exact fake owner without restarting it.
func recoveryRegisterFixtureOwner(t *testing.T, s *Store, owner string) {
	t.Helper()
	if err := s.executionWrite(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE execution_control SET owner_instance_id=? WHERE singleton=1", owner)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryPauseLostAcknowledgementAndDrainedSettlement(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	task, err := s.Task(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision}
	s.AfterCommit = func() error { return errors.New("simulated lost pause acknowledgement") }
	lost := s.PauseTask(ctx, owner, task.Task.ID, input)
	s.AfterCommit = nil
	if lost.Envelope.Error == nil || lost.Envelope.Error.Code != "write_uncertain" || lost.Envelope.Receipt != nil {
		t.Fatal("committed pause did not report lost acknowledgement", lost)
	}

	first := s.PauseTask(ctx, owner, task.Task.ID, input)
	paused := recoveryPauseResult(t, first).Checkpoint
	if paused.Scope.Kind != "task" || paused.Scope.TaskID != task.Task.ID || paused.State != "pausing" || !recoveryContains(paused.AdmittedAttemptIDs, admission.Attempt.ID) {
		t.Fatal("pause omitted active admitted work or misrepresented drain", paused)
	}
	before, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil || before.Phase == "settled" || before.Stop != nil {
		t.Fatal("pause cancelled or rewrote admitted work", before, err)
	}
	replayed := s.PauseTask(ctx, domain.NewID(), task.Task.ID, input)
	replay := recoveryPauseResult(t, replayed).Checkpoint
	if !reflect.DeepEqual(paused, replay) || !reflect.DeepEqual(first.Envelope.Receipt, replayed.Envelope.Receipt) {
		t.Fatal("exact retry re-evaluated owner or created another checkpoint", first, replayed)
	}
	changed := input
	changed.TaskVersion = "2"
	if conflict := s.PauseTask(ctx, owner, task.Task.ID, changed); conflict.Envelope.Error == nil || conflict.Envelope.Error.Code != "request_conflict" {
		t.Fatal("request identity was reused for a different pause", conflict)
	}

	workflow, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement)
	if err != nil {
		t.Fatal("previously admitted work could not drain", err)
	}
	checkpoint, err := s.Checkpoint(ctx, paused.ID)
	if err != nil || checkpoint.State != "paused" || !recoveryContains(checkpoint.CompletedAttemptIDs, admission.Attempt.ID) {
		t.Fatal("drained outcome did not update the durable checkpoint", checkpoint, err)
	}
	step, err := s.WorkflowStep(ctx, admission.Step.ID)
	if err != nil || step.AdvanceProof == nil {
		t.Fatal("pause lost completed work or its retained continuation proof", step, err)
	}
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: step.AdvanceProof.NextLogicalKey, Cycle: 0, DefinitionID: &workflow.Authority.Checks[0].ID, InputCandidateID: workflow.SelectedCandidateID, PredecessorStepID: &step.ID}
	next, err := s.ContinueWorkflow(ctx, owner, step.ID, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: task.Task.Revision, ExpectedWorkflowRevision: workflow.Revision})
	if err == nil && next != nil {
		t.Fatal("paused task admitted its next workflow stage", next)
	}
	if _, attempts, steps, _ := workflowCounts(t, s); attempts != 1 || steps != 1 {
		t.Fatal("pause or observation created additional work", attempts, steps)
	}
}

func TestRecoveryPausePreCommitFailureAndStaleAuthority(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, _ := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: admission.Task.CurrentVersion, ExpectedTaskRevision: admission.Task.Revision}
	stale := input
	stale.RequestID = domain.NewID()
	stale.ExpectedTaskRevision = "999"
	if result := s.PauseTask(ctx, owner, admission.Task.ID, stale); result.Envelope.Error == nil {
		t.Fatal("stale task control accepted a pause", result)
	}
	s.BeforeCommit = func() error { return errors.New("simulated pause commit failure") }
	failed := s.PauseTask(ctx, owner, admission.Task.ID, input)
	s.BeforeCommit = nil
	if failed.Envelope.Error == nil || failed.Envelope.Error.Code != "storage_failed" {
		t.Fatal("pre-commit failure was acknowledged as a pause", failed)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM receipts WHERE request_id IN (?, ?)", input.RequestID, stale.RequestID).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected or rolled-back request left an accepted receipt", count, err)
	}
	accepted := recoveryPauseResult(t, s.PauseTask(ctx, owner, admission.Task.ID, input)).Checkpoint
	if accepted.Scope.TaskID != admission.Task.ID {
		t.Fatal("fresh retry did not pause its exact original task", accepted)
	}
}

func TestRecoveryPauseRejectsOversizedHistoryWithoutTruncation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "bounded-checkpoint-history")
	owner := domain.NewID()
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	created := s.CreateTask(ctx, owner, taskInput(project.ID))
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission := created.Envelope.Data.(domain.Admission)
	if err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		for sequence := 2; sequence <= maxRecoveryCheckpointAttempts+1; sequence++ {
			a := admission.Attempt
			a.ID = domain.NewID()
			if _, err := tx.ExecContext(ctx, "INSERT INTO attempts(sequence,id,task_id,task_version,revision,phase,data) VALUES(?,?,?,?,?,?,?)", sequence, a.ID, a.TaskID, intRevision(a.TaskVersion), intRevision(a.Revision), a.Phase, encode(a)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: admission.Task.CurrentVersion, ExpectedTaskRevision: admission.Task.Revision}
	result := s.PauseTask(ctx, owner, admission.Task.ID, input)
	if result.Envelope.Error == nil || result.Envelope.Error.Code != "checkpoint_too_large" || result.Envelope.Receipt != nil {
		t.Fatal("oversized history was silently truncated or acknowledged", result)
	}
	var attempts, checkpoints, receipts int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM attempts WHERE task_id=?", admission.Task.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM recovery_checkpoints WHERE task_id=?", admission.Task.ID).Scan(&checkpoints); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM receipts WHERE request_id=?", input.RequestID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if attempts != maxRecoveryCheckpointAttempts+1 || checkpoints != 0 || receipts != 0 {
		t.Fatal("rejected pause changed history or left a partial control", attempts, checkpoints, receipts)
	}
}

func TestRecoveryPauseRejectsCorruptExcessActiveScopes(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "bounded-checkpoint-scopes")
	owner := domain.NewID()
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	created := s.CreateTask(ctx, owner, taskInput(project.ID))
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission := created.Envelope.Data.(domain.Admission)
	if err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		for i := 0; i <= maxRecoveryActiveCandidates; i++ {
			cp := domain.RecoveryCheckpoint{ID: domain.NewID(), Scope: domain.RecoveryScope{Kind: "task", TaskID: admission.Task.ID, TaskVersion: admission.Task.CurrentVersion, ExpectedTaskRevision: admission.Task.Revision}, Revision: "1", State: "pausing", OwnerInstanceID: owner, AdmittedAttemptIDs: []string{admission.Attempt.ID}}
			if _, err := tx.ExecContext(ctx, "INSERT INTO recovery_checkpoints(id,scope_kind,task_id,revision,state,owner_instance_id,data) VALUES(?,'task',?,1,'pausing',?,?)", cp.ID, admission.Task.ID, owner, encode(cp)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActiveRecoveryCheckpoints(ctx, admission.Task.ID, ""); err == nil {
		t.Fatal("active scope listing silently truncated corrupt records")
	}
	_, err := s.SettleAttempt(ctx, admission.Attempt.ID, domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}})
	var fault *domain.Error
	if !errors.As(err, &fault) || fault.Code != "checkpoint_scope_overflow" {
		t.Fatal("settlement did not fail closed on excess active scopes", err)
	}
	after, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil || after.Phase != "queued" {
		t.Fatal("overflowing checkpoint refresh committed a partial settlement", after, err)
	}
}

func TestRecoveryCheckpointRetainsUncertainSettlement(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: admission.Task.CurrentVersion, ExpectedTaskRevision: admission.Task.Revision}
	checkpoint := recoveryPauseResult(t, s.PauseTask(ctx, owner, admission.Task.ID, input)).Checkpoint
	settlement.Settlement.Outcome = "failed"
	settlement.Settlement.Cleanup.State = "unknown"
	settlement.Settlement.EvidenceStatus = "uncertain"
	settlement.Settlement.Failure = &domain.PrimaryFailure{Code: "fake_failure", Message: "The controlled worker failed before cleanup could be verified."}
	if _, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal("failed work could not retain its terminal evidence", err)
	}
	checkpoint, err := s.Checkpoint(ctx, checkpoint.ID)
	if err != nil || checkpoint.State != "pausing" || !recoveryContains(checkpoint.CompletedAttemptIDs, admission.Attempt.ID) || !recoveryContains(checkpoint.Blockers, "cleanup_unverified:"+admission.Attempt.ID) || !recoveryContains(checkpoint.Blockers, "evidence_incomplete:"+admission.Attempt.ID) || !recoveryContains(checkpoint.Blockers, "claim_retained:"+admission.Attempt.ID) {
		t.Fatal("uncertain settlement was represented as a safe pause", checkpoint, err)
	}
	attempt, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil || attempt.PrimaryFailure == nil || attempt.PrimaryFailure.Code != "fake_failure" || attempt.Outcome == nil || *attempt.Outcome != "failed" || attempt.Cleanup.IncidentID == nil {
		t.Fatal("checkpoint erased the primary failure or incident", attempt, err)
	}
}

func TestRecoveryCheckpointPreservesSafelyFailedIncompleteHistory(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	settlement.Settlement.Outcome = "failed"
	settlement.Settlement.EvidenceStatus = "incomplete"
	settlement.Settlement.EffectsKnown = true
	settlement.Settlement.Failure = &domain.PrimaryFailure{Code: "known_failure", Message: "The controlled worker failed with a known partial result."}
	if _, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal(err)
	}
	task, err := s.Task(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision}
	cp := recoveryPauseResult(t, s.PauseTask(ctx, owner, task.Task.ID, input)).Checkpoint
	if cp.State != "paused" || len(cp.Blockers) != 0 || !recoveryContains(cp.CompletedAttemptIDs, admission.Attempt.ID) {
		t.Fatal("known settled failure was incorrectly represented as uncertain", cp)
	}
	a, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "known_failure" || a.EvidenceStatus != "incomplete" || a.Outcome == nil || *a.Outcome != "failed" {
		t.Fatal("pause erased the original failed result", a, err)
	}
}

func TestRecoveryCheckpointDoesNotTreatUnknownFailedEffectsAsSettled(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	settlement.Settlement.Outcome = "failed"
	settlement.Settlement.EvidenceStatus = "incomplete"
	settlement.Settlement.EffectsKnown = false
	settlement.Settlement.Failure = &domain.PrimaryFailure{Code: "unknown_effects", Message: "The controlled failure left effects unverified."}
	if _, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal(err)
	}
	task, err := s.Task(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: task.Task.CurrentVersion, ExpectedTaskRevision: task.Task.Revision}
	cp := recoveryPauseResult(t, s.PauseTask(ctx, owner, task.Task.ID, input)).Checkpoint
	if cp.State != "pausing" || !recoveryContains(cp.Blockers, "evidence_incomplete:"+admission.Attempt.ID) {
		t.Fatal("unknown effects were represented as a safe drained checkpoint", cp)
	}
}

func TestRecoverySettlementRetainsClaimUntilIncidentReconciled(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, owner, settlement := workflowAuthorSettlementFixture(t, s)
	recoveryRegisterFixtureOwner(t, s, owner)
	incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: &admission.Attempt.ID, Code: "cleanup_watchdog", Message: "Controlled cleanup exceeded its observation window.", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SettleWorkflowStep(ctx, admission.Attempt.ID, settlement); err != nil {
		t.Fatal("verified cleanup must preserve the incident and terminal outcome", err)
	}
	var claims int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM capacity_claims WHERE attempt_id=?", admission.Attempt.ID).Scan(&claims); err != nil || claims != 1 {
		t.Fatal("settlement released a claim before incident reconciliation", claims, err)
	}
	attempt, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil || attempt.Cleanup.State != "verified" || attempt.Outcome == nil || *attempt.Outcome != "succeeded" {
		t.Fatal("claim retention rewrote the verified terminal result", attempt, err)
	}
}

func TestRecoveryMemberAndGroupPausesKeepIndependentScopes(t *testing.T) {
	ctx := context.Background()
	s, graph, root, proofs, owner := graphReadyBasis(t)
	member := graph.Nodes[0]
	sibling := graph.Nodes[1]
	staleMemberFacts := graphBeginFacts(graph, member, root, proofs, owner)

	memberInput := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: member.TaskVersion, ExpectedTaskRevision: member.TaskRevision}
	memberCheckpoint := recoveryPauseResult(t, s.PauseTask(ctx, owner, member.TaskID, memberInput)).Checkpoint
	if memberCheckpoint.Scope.Kind != "member" || memberCheckpoint.Scope.GraphID != graph.Graph.ID || memberCheckpoint.Scope.NodeID != member.ID || memberCheckpoint.Scope.TaskID != member.TaskID {
		t.Fatal("member pause lost actual graph ownership", memberCheckpoint)
	}
	if op, err := s.BeginGraphInput(ctx, owner, graph.Graph.ID, member.ID, staleMemberFacts); err == nil {
		t.Fatal("pre-pause captured facts admitted input for the paused member", op)
	}

	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || view.DispatchAuthority == nil {
		t.Fatal("sibling's current graph authority unavailable", view, err)
	}
	var currentSibling domain.GraphNodeSummary
	for _, node := range view.Nodes {
		if node.ID == sibling.ID {
			currentSibling = node
		}
	}
	if currentSibling.ID == "" {
		t.Fatal("unrelated sibling disappeared", view)
	}
	siblingFacts := graphBeginFacts(graph, currentSibling, root, proofs, owner)
	siblingFacts.Control.Generation = view.DispatchAuthority.Generation
	siblingFacts.Control.GraphControlRevision = view.Graph.ControlRevision
	siblingFacts.Control.BudgetAuthorityEpoch = view.Graph.BudgetAuthorityEpoch
	siblingFacts.Control.WindowNumber = view.Budget.Budget.CurrentWindow.Number
	siblingFacts.Trigger.OwnerGeneration = view.DispatchAuthority.Generation
	siblingOp, err := s.BeginGraphInput(ctx, owner, graph.Graph.ID, sibling.ID, siblingFacts)
	if err != nil || siblingOp.Attempt.ID == "" {
		t.Fatal("member pause prevented independently eligible sibling work", siblingOp, err)
	}

	view, err = s.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorities := make([]domain.GraphNodeAuthority, 0, len(view.Nodes))
	for _, node := range view.Nodes {
		authorities = append(authorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	groupInput := domain.PauseGraphInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: authorities}
	groupCheckpoint := recoveryPauseResult(t, s.PauseGraph(ctx, owner, graph.Graph.ID, groupInput)).Checkpoint
	if groupCheckpoint.ID == memberCheckpoint.ID || groupCheckpoint.Scope.Kind != "graph" || groupCheckpoint.Scope.GraphID != graph.Graph.ID || groupCheckpoint.State != "pausing" {
		t.Fatal("overlapping group pause lost its independent active checkpoint", groupCheckpoint, memberCheckpoint)
	}
	retainedMember, err := s.Checkpoint(ctx, memberCheckpoint.ID)
	if err != nil || retainedMember.Scope.NodeID != member.ID || retainedMember.State == "resumed" {
		t.Fatal("group pause cleared the earlier member pause", retainedMember, err)
	}
	if _, err = s.ClaimAttempt(ctx, siblingOp.Attempt.ID, owner); err != nil {
		t.Fatal("work admitted before group pause could not drain", err)
	}
	budget, err := s.GraphBudget(ctx, graph.Graph.ID)
	if err != nil || budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal("pause or local preparation invented native model admission", budget, err)
	}
}
