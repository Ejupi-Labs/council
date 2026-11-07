package engine

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRecoveryGroupResumeSkipsAcceptedPausedAncestorDependent(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	edges := []domain.GraphDependencyInput{{From: "a", To: "c", Kind: "context"}}
	graph, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "a", OriginalText: "Produce the response used by C.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		{Key: "b", OriginalText: "Continue independently.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		{Key: "c", OriginalText: "Use A's accepted response.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
	}, Dependencies: &edges})
	view, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || view.DispatchAuthority == nil {
		t.Fatal(view, err)
	}
	a := view.Nodes[0]
	definition, err := e.db.GraphDefinitionPrivate(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	control := domain.GraphDispatchBinding{GraphID: graph.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID, OwnerInstanceID: e.config.OwnerInstanceID, Generation: view.DispatchAuthority.Generation, GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeID: &a.ID, TaskID: &a.TaskID, TaskVersion: &a.TaskVersion, NodeControlRevision: &a.ControlRevision}
	facts, err := e.graphInitialAdmissionFacts(ctx, definition, a, control)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := e.db.AdmitGraphNode(ctx, e.config.OwnerInstanceID, graph.Graph.ID, domain.GraphFreshTrigger{Kind: "claim_release", ID: source, OwnerGeneration: view.DispatchAuthority.Generation}, facts)
	if err != nil || dispatch.Dispatch.AttemptID == "" {
		t.Fatal("could not admit exact prerequisite", dispatch, err)
	}
	author, err := e.db.ClaimAttempt(ctx, dispatch.Dispatch.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, author)
	accept := exactResponseAcceptanceInput(t, e, author.TaskID, author.ID)
	// Keep the acknowledgement callback behind the gate until the accepted
	// ancestor and the whole group have both actually been paused.
	e.prepareGate.Lock()
	held := true
	defer func() {
		if held {
			e.prepareGate.Unlock()
		}
	}()
	accepted := make(chan domain.Result, 1)
	go func() { accepted <- e.AcceptResponse(ctx, author.TaskID, accept) }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		view, err = e.db.Graph(ctx, graph.Graph.ID)
		if err == nil && view.Nodes[0].AcceptedResult != nil {
			break
		}
		select {
		case result := <-accepted:
			t.Fatal("acceptance returned before its blocked callback", result)
		case <-deadline.C:
			t.Fatal("accepted prerequisite was not committed", view, err)
		case <-tick.C:
		}
	}
	ancestor := view.Nodes[0]
	memberPause := e.db.PauseTask(ctx, e.config.OwnerInstanceID, ancestor.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: ancestor.TaskVersion, ExpectedTaskRevision: ancestor.TaskRevision})
	if memberPause.Envelope.Error != nil {
		t.Fatal(memberPause)
	}
	memberCheckpoint := memberPause.Envelope.Data.(domain.RecoveryPauseResult).Checkpoint
	view, err = e.db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorities := make([]domain.GraphNodeAuthority, 0, len(view.Nodes))
	for _, node := range view.Nodes {
		authorities = append(authorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	groupPause := e.db.PauseGraph(ctx, e.config.OwnerInstanceID, graph.Graph.ID, domain.PauseGraphInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: authorities})
	if groupPause.Envelope.Error != nil {
		t.Fatal(groupPause)
	}
	groupCheckpoint := groupPause.Envelope.Data.(domain.RecoveryPauseResult).Checkpoint
	e.prepareGate.Unlock()
	held = false
	if ack := <-accepted; ack.Envelope.Error != nil {
		t.Fatal(ack)
	}
	resume := e.ResumeCheckpoint(ctx, groupCheckpoint.ID, domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: groupCheckpoint.Revision, Scope: groupCheckpoint.Scope})
	if resume.Envelope.Error != nil {
		t.Fatal("group resume failed instead of skipping the paused ancestor's dependent", resume)
	}
	after, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || after.Nodes[1].NodeAdmissionID == nil || after.Nodes[2].NodeAdmissionID != nil || after.Nodes[2].InputID != nil || !reflect.DeepEqual(after.Nodes[0].AcceptedResult, view.Nodes[0].AcceptedResult) {
		t.Fatal("group resume did not isolate the independent sibling", view, after, err)
	}
	stillPaused, err := e.db.Checkpoint(ctx, memberCheckpoint.ID)
	if err != nil || stillPaused.State != "paused" {
		t.Fatal("group resume cleared the member pause", stillPaused, err)
	}
}

// This exercises the actual fake graph input worker and its completion callback;
// it does not fabricate a native process or use a live provider.
func TestRecoveryMemberInputCompletionAdmitsOnlySelectedNode(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	creation, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{
		{Key: "a", OriginalText: "Create the first independent controlled result.", FakeScenario: "graph-branch-a"},
		{Key: "b", OriginalText: "Create the second independent controlled result.", FakeScenario: "graph-branch-b"},
	}}, creation)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	graph := created.Envelope.Data.(domain.GraphAdmission)
	basis, err := e.db.ClaimAttempt(ctx, graph.PreparationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, basis)
	view, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	member, sibling := view.Nodes[0], view.Nodes[1]
	pause := domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: member.TaskVersion, ExpectedTaskRevision: member.TaskRevision}
	paused := e.db.PauseTask(ctx, e.config.OwnerInstanceID, member.TaskID, pause)
	if paused.Envelope.Error != nil {
		t.Fatal(paused)
	}
	cp := paused.Envelope.Data.(domain.RecoveryPauseResult).Checkpoint
	if cp.State != "paused" {
		t.Fatal("unstarted member did not pause", cp)
	}
	e.continueGraphInputs(basis.ID)
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 1 || pending[0].ExecutorKind != "local_graph" {
		t.Fatal("paused member or sibling preparation was incorrect", pending, err)
	}
	siblingInput, err := e.db.ClaimAttempt(ctx, pending[0].ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, siblingInput)
	e.continueGraphAdmissions(siblingInput.ID)
	before, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || before.Nodes[1].ID != sibling.ID || before.Nodes[1].LatestAttemptID == nil || before.Nodes[1].NodeAdmissionID == nil || before.Nodes[0].NodeAdmissionID != nil {
		t.Fatal("independent sibling did not continue while member was paused", before, err)
	}
	shared := *before.DispatchAuthority
	siblingBefore := before.Nodes[1]
	resume := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: cp.Revision, Scope: cp.Scope}
	result := e.ResumeCheckpoint(ctx, cp.ID, resume)
	if result.Envelope.Error != nil {
		t.Fatal("member resume failed", result)
	}
	pending, err = e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var memberInput domain.Attempt
	for _, item := range pending {
		if item.ExecutorKind == "local_graph" {
			memberInput = item
		}
	}
	if memberInput.ID == "" {
		t.Fatal("resume did not admit a distinct member preparation", pending)
	}
	claimed, err := e.db.ClaimAttempt(ctx, memberInput.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, claimed)
	e.continueGraphAdmissions(memberInput.ID)
	after, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || after.Nodes[0].NodeAdmissionID == nil || after.Nodes[0].LatestAttemptID == nil || after.Nodes[0].State != "queued" {
		t.Fatal("real member-input completion did not admit its native stage", after, err)
	}
	if !reflect.DeepEqual(*after.DispatchAuthority, shared) || !reflect.DeepEqual(after.Nodes[1], siblingBefore) {
		t.Fatal("member continuation changed sibling registration or work", before, after)
	}
	e.continueGraphAdmissions(memberInput.ID)
	again, err := e.db.Graph(ctx, graph.Graph.ID)
	if err != nil || *again.Nodes[0].NodeAdmissionID != *after.Nodes[0].NodeAdmissionID || *again.Nodes[1].LatestAttemptID != *siblingBefore.LatestAttemptID {
		t.Fatal("replayed completion duplicated member or sibling work", again, err)
	}
	// Release only fixture admissions without launching native model calls.
	for _, node := range again.Nodes {
		if node.LatestAttemptID == nil {
			continue
		}
		a, readErr := e.db.Attempt(ctx, *node.LatestAttemptID)
		if readErr != nil || a.Phase == "settled" {
			continue
		}
		if _, settleErr := e.db.SettleWorkflowStep(ctx, a.ID, domain.WorkflowStepFacts{StepID: a.StepID, OwnerInstanceID: e.config.OwnerInstanceID, Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, Failure: primary("fixture_no_process", "The test retained a queued fake admission.")}}); settleErr != nil {
			t.Fatal(settleErr)
		}
	}
}

func TestRecoveryMemberAfterOwnerRestartContinuesToReview(t *testing.T) {
	old, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, old)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old.ctx, old.cancel = ctx, cancel
	creation, err := old.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	created := old.db.CreateGraph(ctx, old.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{
		{Key: "a", OriginalText: "Author and review the controlled first result.", FakeScenario: "graph-branch-a", Workflow: &domain.WorkflowInput{Review: "required"}},
		{Key: "b", OriginalText: "Preserve this independent sibling.", FakeScenario: "graph-branch-b"},
	}}, creation)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	graph := created.Envelope.Data.(domain.GraphAdmission)
	basis, err := old.db.ClaimAttempt(ctx, graph.PreparationAttemptID, old.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	old.run(ctx, basis)
	view, err := old.db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	member := view.Nodes[0]
	paused := old.db.PauseTask(ctx, old.config.OwnerInstanceID, member.TaskID, domain.PauseTaskInput{RequestID: domain.NewID(), TaskVersion: member.TaskVersion, ExpectedTaskRevision: member.TaskRevision})
	if paused.Envelope.Error != nil {
		t.Fatal(paused)
	}
	cp := paused.Envelope.Data.(domain.RecoveryPauseResult).Checkpoint
	oldOwner := old.config.OwnerInstanceID
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	// Closing the old owner cancels its context; the new owner receives its
	// own live context, as it would after a real restart.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	current, err := New(old.db, old.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal("new owner did not recover retained state", err)
	}
	t.Cleanup(func() { _ = current.Close() })
	stopFixtureScheduler(t, current)
	current.ctx, current.cancel = context.WithCancel(ctx)
	if current.config.OwnerInstanceID == oldOwner {
		t.Fatal("fixture did not change owner")
	}
	resume := domain.ResumeCheckpointInput{RequestID: domain.NewID(), ExpectedCheckpointRevision: cp.Revision, Scope: cp.Scope}
	result := current.ResumeCheckpoint(ctx, cp.ID, resume)
	if result.Envelope.Error != nil {
		t.Fatal("new owner could not resume the exact member", result)
	}
	pending, err := current.db.PendingAttempts(ctx, current.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 1 || pending[0].ExecutorKind != "local_graph" {
		t.Fatal("resume did not admit exact member input", pending, err)
	}
	local, err := current.db.ClaimAttempt(ctx, pending[0].ID, current.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	current.run(ctx, local)
	current.continueGraphAdmissions(local.ID)
	view, err = current.db.Graph(ctx, graph.Graph.ID)
	if err != nil || view.Nodes[0].LatestAttemptID == nil || view.Nodes[1].LatestAttemptID != nil {
		t.Fatal("scoped completion did not isolate the resumed member", view, err)
	}
	author, err := current.db.ClaimAttempt(ctx, *view.Nodes[0].LatestAttemptID, current.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	current.run(ctx, author)
	completed, err := current.db.Attempt(ctx, author.ID)
	if err != nil || completed.Outcome == nil || *completed.Outcome != "succeeded" {
		t.Fatal("the fake author did not actually settle", completed, err)
	}
	current.continueWorkflow(author.ID)
	workflow, err := current.db.Workflow(ctx, member.TaskID, domain.WorkflowSelector{})
	if err != nil || workflow.Workflow == nil || workflow.Workflow.CurrentStepID == nil || *workflow.Workflow.CurrentStepID == author.StepID {
		t.Fatal("scoped owner did not continue to the required review", workflow, err)
	}
	review, err := current.db.WorkflowStep(ctx, *workflow.Workflow.CurrentStepID)
	if err != nil || review.Role != "review" || review.AttemptID == nil || *review.AttemptID == author.ID {
		t.Fatal("review did not receive a new execution identity", review, err)
	}
}
