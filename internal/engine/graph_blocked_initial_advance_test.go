package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphBlockedInitialCase struct {
	producer             graphOutputProducerCase
	preparationAttemptID string
	blocked              domain.GraphDetail
	old                  domain.Attempt
	version              domain.TaskVersionDetail
	step                 domain.WorkflowStep
}

func graphBlockedInitialAdvanceInput(view domain.GraphDetail) domain.GraphAdvanceInput {
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion,
		ExpectedGraphControlRevision: view.Graph.ControlRevision, ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch,
		ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
	}
	return in
}

func graphBlockedInitialMutate(t *testing.T, e *Engine, operation, target string, body any) domain.Result {
	t.Helper()
	wire, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return e.MutateGraph(context.Background(), operation, target, wire)
}

func graphBlockedInitialAdvanceResult(t *testing.T, result domain.Result) domain.GraphAdvanceResult {
	t.Helper()
	wire, err := json.Marshal(result.Envelope.Data)
	var advanced domain.GraphAdvanceResult
	if err != nil || json.Unmarshal(wire, &advanced, json.RejectUnknownMembers(true)) != nil || result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The exact public Advance was not acknowledged", result, err)
	}
	return advanced
}

// The qualified stopped-actor fixture lets this test cross the real owned
// completion callbacks in a fixed order. Every admission, source, native result
// and local preparation is actual; no row or successful outcome is fabricated.
func graphBlockedInitialFixture(t *testing.T) graphBlockedInitialCase {
	t.Helper()
	f := graphOutputConfiguredNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Return the complete controlled response while preserving source and staging.",
		FakeScenario: "graph-response", Pins: &domain.ExecutionPins{HarnessID: "codex"}, Workflow: &domain.WorkflowInput{Review: "auto"},
		Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}}, func(in *domain.GraphStartInput) {
		in.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "1"}}
	})
	e, ctx := f.engine, context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	e.ctx, e.cancel = ownerCtx, cancel
	e.run(ctx, f.attempt)
	old, err := e.db.Attempt(ctx, f.attempt.ID)
	if err != nil || old.Phase != "settled" || old.Outcome == nil || *old.Outcome != "succeeded" || old.EvidenceStatus != "complete" || old.ResponseArtifactID == nil || old.Cleanup.State != "verified" {
		t.Fatal("Actual original response prerequisite failed", old, err)
	}
	version, err := e.db.TaskVersion(ctx, old.TaskID, old.TaskVersion)
	if err != nil {
		t.Fatal(err)
	}
	step, err := e.db.WorkflowStep(ctx, old.StepID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || len(view.Nodes) != 1 || len(view.ActiveClaims) != 0 || view.Budget.Budget.ConsumedAdmissions != "1" || view.Budget.Budget.RemainingAdmissions != "0" {
		t.Fatal("Actual author did not exhaust its captured one-unit allowance", view, err)
	}
	node := view.Nodes[0]
	guidance := domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: node.TaskRevision,
		GuidanceText: "Complete version-two guidance 東京🙂; retain the original response and source history."},
		ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: node.ID, ExpectedNodeControlRevision: node.ControlRevision, TaskVersion: node.TaskVersion}
	guided := graphBlockedInitialMutate(t, e, "graphs.nodes.guidance", view.Graph.ID+"/"+node.Key, guidance)
	if guided.Status != 200 || guided.Envelope.Error != nil {
		t.Fatal("Actual guidance did not append version two", guided)
	}
	view, err = e.db.Graph(ctx, view.Graph.ID)
	if err != nil || view.Nodes[0].TaskVersion != "2" || view.Nodes[0].InputID != nil || view.Nodes[0].NodeAdmissionID != nil {
		t.Fatal("Guidance reused the old version's authority", view, err)
	}
	prepared := graphBlockedInitialAdvanceResult(t, graphBlockedInitialMutate(t, e, "graphs.advance", view.Graph.ID, graphBlockedInitialAdvanceInput(view)))
	if len(prepared.PreparationAttemptIDs) != 1 || len(prepared.Dispatches) != 0 || prepared.Budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("Zero-balance Advance must prepare only the exact new input", prepared)
	}
	local, err := e.db.ClaimAttempt(ctx, prepared.PreparationAttemptIDs[0], e.config.OwnerInstanceID)
	if err != nil || local.Phase != "preparing" || local.ExecutorKind != "local_graph" {
		t.Fatal("Actual version-two preparation could not be claimed", local, err)
	}
	e.run(ctx, local)
	completed, err := e.db.Attempt(ctx, local.ID)
	if err != nil || completed.Phase != "settled" || completed.Outcome == nil || *completed.Outcome != "succeeded" || completed.EvidenceStatus != "complete" {
		t.Fatal("Actual version-two input preparation did not complete", completed, err)
	}
	// This is the scheduler's real fresh-release callback, after the complete
	// local input. It must observe the exhausted allowance before any grant.
	e.continueGraphAdmissions(local.ID)
	blocked, err := e.db.Graph(ctx, view.Graph.ID)
	if err != nil || blocked.Nodes[0].TaskVersion != "2" || blocked.Nodes[0].InputID == nil || blocked.Nodes[0].NodeAdmissionID != nil || blocked.Nodes[0].State != "blocked" ||
		blocked.Nodes[0].NextOwner != "user" || len(blocked.ActiveClaims) != 0 || blocked.Budget.Budget.RemainingAdmissions != "0" {
		t.Fatal("The current unadmitted receiver was not explicitly budget-blocked", blocked, err)
	}
	found := false
	for _, blocker := range blocked.Blockers {
		if blocker.Code == "budget_exhausted" && blocker.NodeID != nil && *blocker.NodeID == blocked.Nodes[0].ID && blocker.InputID != nil && *blocker.InputID == *blocked.Nodes[0].InputID {
			found = true
		}
	}
	input, err := e.db.GraphInputRecord(ctx, blocked.Graph.ID, *blocked.Nodes[0].InputID)
	if err != nil || !found || input.Public.Summary.State != "ready" || !input.Public.Summary.Current || input.Public.InputBinding == nil || input.Public.InputBinding.ReceivingTaskVersion != "2" {
		t.Fatal("Blocked does not identify a complete current version-two input", input, blocked.Blockers, err)
	}
	return graphBlockedInitialCase{f, local.ID, blocked, old, version, step}
}

func graphBlockedInitialGrant(t *testing.T, f graphBlockedInitialCase) domain.GraphDetail {
	t.Helper()
	e, before, ctx := f.producer.engine, f.blocked, context.Background()
	grant := domain.GraphBudgetAmendmentInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: before.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: before.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: before.Budget.Budget.CurrentWindow.Number, AdditionalAdmissions: "1"}
	result := graphBlockedInitialMutate(t, e, "graphs.budget.amend", before.Graph.ID, grant)
	if result.Status != 200 || result.Envelope.Error != nil {
		t.Fatal("The captured shared grant was not acknowledged", result)
	}
	// A repeated actual release callback cannot turn this grant into fresh
	// registration or dispatch authority.
	e.continueGraphAdmissions(f.preparationAttemptID)
	after, err := e.db.Graph(ctx, before.Graph.ID)
	if err != nil || after.Budget.Budget.GrantedAdmissions != "2" || after.Budget.Budget.ConsumedAdmissions != "1" || after.Budget.Budget.RemainingAdmissions != "1" ||
		!reflect.DeepEqual(before.Budget.Budget.CurrentWindow, after.Budget.Budget.CurrentWindow) || len(after.ActiveClaims) != 0 || after.DispatchAuthority == nil || after.DispatchAuthority.State != "suspended" ||
		before.DispatchAuthority.Generation != after.DispatchAuthority.Generation || !reflect.DeepEqual(before.Nodes, after.Nodes) {
		t.Fatal("Grant alone refilled spent units, changed input or rearmed execution", before, after, err)
	}
	history, err := e.db.Attempts(ctx, f.old.TaskID, 100, "")
	if err != nil || len(history.Items) != 1 || history.Items[0].ID != f.old.ID {
		t.Fatal("Grant alone created a new attempt", history, err)
	}
	return after
}

func graphBlockedInitialPreserved(t *testing.T, f graphBlockedInitialCase) {
	t.Helper()
	e, ctx := f.producer.engine, context.Background()
	old, err := e.db.Attempt(ctx, f.old.ID)
	before := f.old
	before.ObservedAt, old.ObservedAt = "", ""
	before.RemainingSeconds, old.RemainingSeconds = 0, 0
	if err != nil || !reflect.DeepEqual(before, old) {
		t.Fatal("Explicit continuation rewrote the historical terminal", old, err)
	}
	version, err := e.db.TaskVersion(ctx, f.old.TaskID, f.old.TaskVersion)
	if err != nil || !reflect.DeepEqual(f.version, version) {
		t.Fatal("Explicit continuation rewrote the original captured version", version, err)
	}
	step, err := e.db.WorkflowStep(ctx, f.old.StepID)
	if err != nil || !reflect.DeepEqual(f.step, step) {
		t.Fatal("Explicit continuation rewrote the historical workflow step", step, err)
	}
	root, err := e.db.GraphRoot(ctx, f.blocked.Graph.ID)
	if err != nil || !reflect.DeepEqual(root, f.producer.rootBinding) {
		t.Fatal("Explicit continuation replaced the actual graph root", root, err)
	}
	if body, err := os.ReadFile(filepath.Join(f.producer.repo, ".git", "index")); err != nil || !bytes.Equal(body, f.producer.originalIndex) {
		t.Fatal("Explicit continuation changed user staging", err)
	}
	if body, err := os.ReadFile(filepath.Join(f.producer.repo, "source.txt")); err != nil || !bytes.Equal(body, []byte("controlled working source🙂\r\n")) {
		t.Fatal("Explicit continuation changed user working bytes", err)
	}
}

func TestGraphBlockedInitialAdvanceAdmitsOnlyFreshCurrentVersionAfterGrant(t *testing.T) {
	f := graphBlockedInitialFixture(t)
	e, ctx := f.producer.engine, context.Background()
	granted := graphBlockedInitialGrant(t, f)
	in := graphBlockedInitialAdvanceInput(granted)
	result := graphBlockedInitialMutate(t, e, "graphs.advance", granted.Graph.ID, in)
	advanced := graphBlockedInitialAdvanceResult(t, result)
	if len(advanced.Dispatches) != 1 || len(advanced.PreparationAttemptIDs) != 0 || len(advanced.ReboundNodes) != 0 {
		t.Fatal("Fresh explicit Advance skipped the current budget-blocked initial admission", advanced)
	}
	d := advanced.Dispatches[0].Dispatch
	node := granted.Nodes[0]
	if d.NodeID != node.ID || d.TaskID != node.TaskID || d.TaskVersion != "2" || d.Role != "author" || d.ExecutorKind != "native" || d.InputID != *node.InputID || d.FreshTriggerID != in.RequestID ||
		d.GraphControlRevision != in.ExpectedGraphControlRevision || d.NodeControlRevision != node.ControlRevision || d.BudgetAuthorityEpoch != in.ExpectedBudgetAuthorityEpoch || d.WindowNumber != in.ExpectedWindowNumber ||
		advanced.Budget.Budget.ConsumedAdmissions != "2" || advanced.Budget.Budget.RemainingAdmissions != "0" {
		t.Fatal("Fresh explicit Advance substituted scope, proof or shared accounting", d, advanced.Budget)
	}
	claimed, err := e.db.ClaimAttempt(ctx, d.AttemptID, e.config.OwnerInstanceID)
	if err != nil || claimed.Phase != "preparing" {
		t.Fatal("The exact new author could not be claimed", claimed, err)
	}
	e.run(ctx, claimed)
	current, err := e.db.Attempt(ctx, claimed.ID)
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.TaskVersion != "2" || current.ResponseArtifactID == nil || current.Cleanup.State != "verified" {
		t.Fatal("The exact new version did not execute successfully", current, err)
	}
	replayed := graphBlockedInitialAdvanceResult(t, graphBlockedInitialMutate(t, e, "graphs.advance", granted.Graph.ID, in))
	if !reflect.DeepEqual(advanced, replayed) {
		t.Fatal("Exact replay rebuilt current authority instead of returning its original receipt", advanced, replayed)
	}
	history, err := e.db.Attempts(ctx, node.TaskID, 100, "")
	view, graphErr := e.db.Graph(ctx, granted.Graph.ID)
	if err != nil || graphErr != nil || len(history.Items) != 2 || len(view.ActiveClaims) != 0 || view.Budget.Budget.ConsumedAdmissions != "2" || view.Budget.Budget.CurrentWindow.Number != granted.Budget.Budget.CurrentWindow.Number ||
		!reflect.DeepEqual(view.Budget.Budget.CurrentWindow.DeadlineAt, granted.Budget.Budget.CurrentWindow.DeadlineAt) {
		t.Fatal("Execution or replay duplicated work or renewed the common window", history, view, err, graphErr)
	}
	graphBlockedInitialPreserved(t, f)
}

func TestGraphBlockedInitialAdvanceRejectsStaleVersionWithoutReopening(t *testing.T) {
	f := graphBlockedInitialFixture(t)
	e, ctx := f.producer.engine, context.Background()
	granted := graphBlockedInitialGrant(t, f)
	in := graphBlockedInitialAdvanceInput(granted)
	in.NodeAuthorities[0].TaskVersion = "1"
	result := graphBlockedInitialMutate(t, e, "graphs.advance", granted.Graph.ID, in)
	if result.Status != 409 || result.Envelope.Error == nil || result.Envelope.Error.Code != "graph_control_conflict" {
		t.Fatal("A blocked node's stale version acquired current explicit authority", result)
	}
	after, err := e.db.Graph(ctx, granted.Graph.ID)
	history, historyErr := e.db.Attempts(ctx, f.old.TaskID, 100, "")
	if err != nil || historyErr != nil || !reflect.DeepEqual(granted, after) || len(history.Items) != 1 || history.Items[0].ID != f.old.ID {
		t.Fatal("Refused stale intent changed the blocked node or shared allowance", after, history, err, historyErr)
	}
	graphBlockedInitialPreserved(t, f)
}
