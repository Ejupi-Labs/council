package engine

import (
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphOwnerAdvancePreparesActualGuidedInputAndReplaysBeforeChangedProject(t *testing.T) {
	e, projectID, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	group, _ := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{{
		Key: "receiver", OriginalText: "Preserve this complete original 東京\r\nPrepare only after explicit updated intent.",
		FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"},
	}}})
	before, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil || len(before.Nodes) != 1 || before.Nodes[0].InputID == nil {
		t.Fatal("Actual initial input prerequisite failed", before, err)
	}
	oldNode, oldInput := before.Nodes[0], *before.Nodes[0].InputID
	guidance := domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: oldNode.TaskRevision, GuidanceText: "Complete fresh guidance café 🙂\r\nUse a fresh captured receiving input."},
		ExpectedGraphControlRevision: before.Graph.ControlRevision, NodeID: oldNode.ID, ExpectedNodeControlRevision: oldNode.ControlRevision, TaskVersion: oldNode.TaskVersion}
	if result := e.db.UpdateGraphNodeGuidance(ctx, e.config.OwnerInstanceID, before.Graph.ID, oldNode.Key, guidance, domain.GraphAdvanceFacts{}); result.Status != 200 {
		t.Fatal("Actual guidance prerequisite failed", result)
	}
	guided, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || guided.Nodes[0].InputID != nil || guided.Nodes[0].TaskVersion != "2" {
		t.Fatal("Guidance did not invalidate its original input", guided, err)
	}
	node := guided.Nodes[0]
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: guided.Graph.GraphVersion, ExpectedGraphControlRevision: guided.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: guided.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: guided.Budget.Budget.CurrentWindow.Number,
		NodeAuthorities: []domain.GraphNodeAuthority{{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision}}}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", before.Graph.ID, body)
	if result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The exact public Advance could not prepare the current guided version", result)
	}
	var accepted domain.GraphAdvanceResult
	encoded, _ := json.Marshal(result.Envelope.Data)
	if err = json.Unmarshal(encoded, &accepted, json.RejectUnknownMembers(true)); err != nil || len(accepted.PreparationAttemptIDs) != 1 || len(accepted.Dispatches) != 0 || len(accepted.ReboundNodes) != 0 {
		t.Fatal("Advance did not acknowledge exactly one new local preparation", accepted, err)
	}
	if !reflect.DeepEqual(guided.Budget, accepted.Budget) || accepted.DispatchAuthority == nil || accepted.DispatchAuthority.Generation == guided.DispatchAuthority.Generation {
		t.Fatal("Advance refilled allowance or failed to register fresh intent", accepted)
	}
	claimed, err := e.db.ClaimAttempt(ctx, accepted.PreparationAttemptIDs[0], e.config.OwnerInstanceID)
	if err != nil || claimed.ID != accepted.PreparationAttemptIDs[0] || claimed.ExecutorKind != "local_graph" {
		t.Fatal("The acknowledged preparation was not the actual claimed local operation", claimed, err)
	}
	e.run(ctx, claimed)
	prepared, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || prepared.Nodes[0].InputID == nil || *prepared.Nodes[0].InputID == oldInput || prepared.Nodes[0].TaskVersion != "2" || prepared.Nodes[0].NodeAdmissionID != nil {
		t.Fatal("Actual preparation did not retain a distinct V2 input without a native admission", prepared, err)
	}
	input, err := e.db.GraphInputRecord(ctx, before.Graph.ID, *prepared.Nodes[0].InputID)
	if err != nil || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil || input.Public.InputBinding.ReceivingTaskVersion != "2" {
		t.Fatal("Actual guided input is not ready and bound to V2", input, err)
	}
	artifacts := graphGuardArtifactNames(t, e)
	moved := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-advance-replay")
	if err := os.Rename(repo, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Rename(moved, repo); err != nil {
			t.Error(err)
		}
	})
	replay := e.MutateGraph(ctx, "graphs.advance", before.Graph.ID, body)
	var originalWire any
	first, _ := json.Marshal(result.Envelope)
	var replayWire any
	second, _ := json.Marshal(replay.Envelope)
	firstErr := json.Unmarshal(first, &originalWire)
	secondErr := json.Unmarshal(second, &replayWire)
	if firstErr != nil || secondErr != nil || replay.Status != 200 || !reflect.DeepEqual(originalWire, replayWire) {
		t.Fatal("Exact Advance replay followed changed project/current input prerequisites", replay, result, firstErr, secondErr)
	}
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 || !reflect.DeepEqual(artifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Receipt replay created another local/native operation or artifact", pending, err)
	}
}

func TestGraphOwnerAdvanceResumesActualReviewAfterGrantWithoutReplayingAuthor(t *testing.T) {
	f := graphOutputAdmittedNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Create the controlled file result and independently review its complete original conditions.",
		FakeScenario: "graph-branch-a", Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}})
	e, author := f.engine, f.attempt
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	e.run(ctx, author)
	terminal, err := e.db.Attempt(ctx, author.ID)
	if err != nil || terminal.Outcome == nil || *terminal.Outcome != "succeeded" || terminal.CandidateID == nil {
		t.Fatal("Actual author prerequisite failed", terminal, err)
	}
	before, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	grant := e.db.AmendGraphBudget(ctx, e.config.OwnerInstanceID, before.Graph.ID, domain.GraphBudgetAmendmentInput{RequestID: domain.NewID(),
		ExpectedGraphControlRevision: before.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: before.Graph.BudgetAuthorityEpoch,
		ExpectedWindowNumber: before.Budget.Budget.CurrentWindow.Number, AdditionalAdmissions: "1"})
	if grant.Status != 200 {
		t.Fatal("Actual grant prerequisite failed", grant)
	}
	// A grant is allowance only. Even this genuine old completion cannot use the
	// superseded execution registration to start another role.
	e.continueWorkflow(author.ID)
	if pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100); err != nil || len(pending) != 0 {
		t.Fatal("A grant rearmed the old completion callback", pending, err)
	}
	current, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	node := current.Nodes[0]
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: current.Graph.GraphVersion, ExpectedGraphControlRevision: current.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: current.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: current.Budget.Budget.CurrentWindow.Number,
		NodeAuthorities: []domain.GraphNodeAuthority{{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision}}}
	body, _ := json.Marshal(in)
	result := e.MutateGraph(ctx, "graphs.advance", current.Graph.ID, body)
	var accepted domain.GraphAdvanceResult
	encoded, _ := json.Marshal(result.Envelope.Data)
	if err = json.Unmarshal(encoded, &accepted, json.RejectUnknownMembers(true)); result.Status != 200 || err != nil || len(accepted.Dispatches) != 1 || len(accepted.PreparationAttemptIDs) != 0 {
		t.Fatal("Fresh public Advance did not resume exactly the retained next review", result, accepted, err)
	}
	d := accepted.Dispatches[0].Dispatch
	if d.Role != "review" || d.WorkflowID != author.WorkflowID || d.TaskVersion != author.TaskVersion || d.NodeAdmissionID != f.dispatch.Dispatch.NodeAdmissionID || d.InputID != f.inputBinding.ID || d.SourceBasis.BasisKind != "node_subject" || d.SourceBasis.CandidateBinding == nil || d.SourceBasis.CandidateBinding.CandidateID != *terminal.CandidateID || d.FreshTriggerID != in.RequestID {
		t.Fatal("Explicit resume replayed or replaced prior authority", d)
	}
	reviewer, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, reviewer)
	reviewed, err := e.db.Attempt(ctx, reviewer.ID)
	if err != nil || reviewed.Phase != "settled" || reviewed.Outcome == nil || *reviewed.Outcome != "succeeded" || reviewed.StartedAt == nil || reviewed.Cleanup.State != "verified" {
		t.Fatal("The actual explicitly resumed reviewer did not complete", reviewed, err)
	}
	retained, err := e.db.Attempt(ctx, author.ID)
	// These two fields are read-time projections from observe(), even for a
	// settled attempt. Every persisted terminal/control/identity field must match.
	retained.ObservedAt, retained.RemainingSeconds = terminal.ObservedAt, terminal.RemainingSeconds
	if err != nil || !reflect.DeepEqual(terminal, retained) {
		t.Fatal("Explicit review resume rewrote the actual prior terminal", retained, err)
	}
	after, err := e.db.GraphBudget(ctx, current.Graph.ID)
	if err != nil || after.Budget.ConsumedAdmissions != "2" || after.Budget.AutomaticAdmissions != current.Budget.Budget.AutomaticAdmissions || after.Budget.GrantedAdmissions != current.Budget.Budget.GrantedAdmissions {
		t.Fatal("Explicit resumed review was not one additional manual admission", after, err)
	}
	replay := e.MutateGraph(ctx, "graphs.advance", current.Graph.ID, body)
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if replay.Status != 200 || pendingErr != nil || len(pending) != 0 {
		t.Fatal("Exact resume receipt replay admitted another worker", replay, pending, pendingErr)
	}
}

func TestGraphOwnerAdvanceAdmitsCurrentReadyNodesAndRunsTheirActualWorkers(t *testing.T) {
	e, before, _ := graphGuidanceOwnerFixture(t)
	ctx := context.Background()
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: before.Graph.GraphVersion, ExpectedGraphControlRevision: before.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: before.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: before.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range before.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	body, _ := json.Marshal(in)
	result := e.MutateGraph(ctx, "graphs.advance", before.Graph.ID, body)
	var accepted domain.GraphAdvanceResult
	encoded, _ := json.Marshal(result.Envelope.Data)
	if err := json.Unmarshal(encoded, &accepted, json.RejectUnknownMembers(true)); result.Status != 200 || err != nil || len(accepted.Dispatches) != 2 || len(accepted.PreparationAttemptIDs) != 0 || len(accepted.ReboundNodes) != 0 {
		t.Fatal("Public Advance did not atomically admit the two actual ready inputs", result, accepted, err)
	}
	if accepted.Budget.Budget.ConsumedAdmissions != "2" || accepted.Budget.Budget.AutomaticAdmissions != "0" || accepted.Budget.Budget.CurrentWindow.FirstAdmissionAt == nil || accepted.Budget.Budget.CurrentWindow.DeadlineAt == nil {
		t.Fatal("Explicit initial admissions did not share one finite root window and two manual debits", accepted.Budget)
	}
	contexts := map[string]bool{}
	for _, dispatch := range accepted.Dispatches {
		d := dispatch.Dispatch
		if d.Role != "author" || d.ExecutorKind != "native" || d.NodeAdmissionID != d.ID || d.FreshTriggerID != in.RequestID || d.SourceBasis.BasisKind != "graph_input" {
			t.Fatal("A ready node was replaced with a derived or historical role", d)
		}
		a, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
		if err != nil || a.ResolvedTarget == nil || a.ResolvedTarget.ContextID == "" || contexts[a.ResolvedTarget.ContextID] {
			t.Fatal("Initial ready nodes did not reserve distinct actual fake contexts", a, err)
		}
		contexts[a.ResolvedTarget.ContextID] = true
		e.run(ctx, a)
		terminal, err := e.db.Attempt(ctx, a.ID)
		if err != nil || terminal.Phase != "settled" || terminal.Outcome == nil || *terminal.Outcome != "succeeded" || terminal.StartedAt == nil || terminal.Cleanup.State != "verified" {
			t.Fatal("The actually admitted current-input worker did not complete", terminal, err)
		}
		observed, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
		if err != nil || observed.Public.Materialization == nil || observed.Public.Materialization.State != "complete" || observed.Public.OutputObservation == nil || observed.Public.OutputObservation.Response == nil || observed.Public.OutputObservation.Response.State != "complete" {
			t.Fatal("Actual worker lost its own receiving and terminal output associations", observed, err)
		}
	}
	after, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replay := e.MutateGraph(ctx, "graphs.advance", before.Graph.ID, body); replay.Status != 200 {
		t.Fatal("Exact initial-admission receipt could not replay after its workers settled", replay)
	}
	retained, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || !reflect.DeepEqual(after, retained) {
		t.Fatal("Receipt replay refreshed execution authority, allowance or current node outcomes", retained, err)
	}
	if pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100); err != nil || len(pending) != 0 {
		t.Fatal("Receipt replay admitted more work", pending, err)
	}
}
