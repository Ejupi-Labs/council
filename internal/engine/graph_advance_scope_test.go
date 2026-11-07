package engine

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These cases use an actual author terminal and captured graph controls. The
// resumed review is only admitted here; no reviewer success is manufactured.
func graphAdvanceScopeFixture(t *testing.T) (graphOutputProducerCase, domain.GraphDetail, domain.GraphAdvanceInput) {
	t.Helper()
	f := graphOutputAdmittedNodeFixture(t, domain.GraphNodeInput{Key: "a", OriginalText: "Create the controlled file result and independently review the complete original conditions.",
		FakeScenario: "graph-branch-a", Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}},
		domain.GraphNodeInput{Key: "b", OriginalText: "Remain an independent unstarted response task.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}})
	e := f.engine
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	e.run(ctx, f.attempt)
	author, err := e.db.Attempt(ctx, f.attempt.ID)
	if err != nil || author.Phase != "settled" || author.Outcome == nil || *author.Outcome != "succeeded" || author.CandidateID == nil {
		t.Fatal("Actual author prerequisite failed", author, err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || len(view.Nodes) != 2 {
		t.Fatal("Actual two-node graph prerequisite failed", view, err)
	}
	in := domain.GraphAdvanceInput{RequestID: domain.NewID(), GraphVersion: view.Graph.GraphVersion, ExpectedGraphControlRevision: view.Graph.ControlRevision,
		ExpectedBudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, ExpectedWindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeAuthorities: []domain.GraphNodeAuthority{}}
	for _, node := range view.Nodes {
		in.NodeAuthorities = append(in.NodeAuthorities, domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision})
		if node.Key == "b" && node.NodeAdmissionID != nil {
			t.Fatal("The independent sibling was already admitted", node)
		}
	}
	return f, view, in
}

func TestGraphAdvanceIgnoresStoppedUnrelatedNodeControlsForExactPendingReview(t *testing.T) {
	f, captured, in := graphAdvanceScopeFixture(t)
	e, ctx := f.engine, context.Background()
	var sibling, producer domain.GraphNodeSummary
	for _, node := range captured.Nodes {
		if node.Key == "b" {
			sibling = node
		} else if node.Key == "a" {
			producer = node
		}
	}
	if sibling.ID == "" {
		t.Fatal("The actual independent sibling is missing")
	}
	stop := domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: captured.Graph.ControlRevision,
		NodeID: sibling.ID, ExpectedNodeControlRevision: sibling.ControlRevision}
	body, err := json.Marshal(stop)
	if err != nil {
		t.Fatal(err)
	}
	stopped := e.MutateGraph(ctx, "graphs.nodes.stop", captured.Graph.ID+"/"+sibling.Key, body)
	if stopped.Envelope.Error != nil || stopped.Status != 200 && stopped.Status != 202 {
		t.Fatal("Actual unrelated node Stop prerequisite failed", stopped)
	}
	current, err := e.db.Graph(ctx, captured.Graph.ID)
	if err != nil || current.Graph.ControlRevision != captured.Graph.ControlRevision {
		t.Fatal("Node Stop changed the group control rather than only its relevant node", current, err)
	}
	for _, node := range current.Nodes {
		if node.Key == "b" && (node.Stop == nil || node.ControlRevision == sibling.ControlRevision || node.NodeAdmissionID != nil) {
			t.Fatal("The unrelated sibling is not actually stopped and unadmitted", node)
		}
		if node.Key == "a" && (node.ID != f.dispatch.Dispatch.NodeID || node.TaskVersion != f.attempt.TaskVersion || node.ControlRevision != producer.ControlRevision || node.Stop != nil) {
			t.Fatal("The selected producer's captured controls changed", node)
		}
	}
	body, err = json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", captured.Graph.ID, body)
	var accepted domain.GraphAdvanceResult
	wire, _ := json.Marshal(result.Envelope.Data)
	decodeErr := json.Unmarshal(wire, &accepted, json.RejectUnknownMembers(true))
	if result.Status != 200 || result.Envelope.Error != nil || decodeErr != nil || len(accepted.Dispatches) != 1 || len(accepted.PreparationAttemptIDs) != 0 || len(accepted.ReboundNodes) != 0 {
		t.Fatal("An unrelated stopped node blocked the exact pending review", result, accepted, decodeErr)
	}
	d := accepted.Dispatches[0].Dispatch
	if d.NodeID != f.dispatch.Dispatch.NodeID || d.TaskID != f.attempt.TaskID || d.TaskVersion != f.attempt.TaskVersion || d.Role != "review" || d.ExecutorKind != "native" ||
		d.WorkflowID != f.attempt.WorkflowID || d.NodeAdmissionID != f.dispatch.Dispatch.NodeAdmissionID || d.InputID != f.inputBinding.ID || d.SourceBasis.CandidateBinding == nil ||
		d.SourceBasis.CandidateBinding.CandidateID == "" || d.FreshTriggerID != in.RequestID {
		t.Fatal("Advance substituted authority instead of resuming only the retained producer review", d)
	}
	after, err := e.db.Graph(ctx, captured.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	author, err := e.db.Attempt(ctx, f.attempt.ID)
	if err != nil || author.CandidateID == nil || d.SourceBasis.CandidateBinding.CandidateID != *author.CandidateID {
		t.Fatal("The review did not retain its actual selected author candidate", d, author, err)
	}
	for _, node := range after.Nodes {
		if node.Key == "b" && (node.Stop == nil || node.NodeAdmissionID != nil) {
			t.Fatal("Advance rearmed the unrelated stopped node", node)
		}
	}
	if accepted.Budget.Budget.ConsumedAdmissions != "2" || accepted.Budget.Budget.GrantedAdmissions != captured.Budget.Budget.GrantedAdmissions || accepted.Budget.Budget.AutomaticAdmissions != captured.Budget.Budget.AutomaticAdmissions || accepted.Budget.Budget.CurrentWindow.Number != in.ExpectedWindowNumber {
		t.Fatal("Exact review resume refilled or reset the shared budget", accepted.Budget)
	}
	t.Cleanup(func() {
		// This test establishes admission only. Settle the genuinely queued
		// reviewer as known unlaunched, without claiming a report or success.
		current, err := e.db.Attempt(context.Background(), d.AttemptID)
		if err != nil || current.Phase == "settled" {
			if err != nil {
				t.Error(err)
			}
			return
		}
		_, err = e.db.SettleWorkflowStep(context.Background(), d.AttemptID, domain.WorkflowStepFacts{StepID: d.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
			Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete",
				Failure: primary("fixture_no_process", "The scope fixture admits but does not execute its exact review.")}})
		if err != nil {
			t.Error(err)
		}
	})
}

func TestGraphAdvanceRejectsChangedSelectedNodeBeforeAnyPreparation(t *testing.T) {
	f, captured, in := graphAdvanceScopeFixture(t)
	e, ctx := f.engine, context.Background()
	var selected domain.GraphNodeSummary
	for _, node := range captured.Nodes {
		if node.ID == f.dispatch.Dispatch.NodeID {
			selected = node
		}
	}
	guidance := domain.GraphGuidanceInput{GuidanceInput: domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: selected.TaskRevision,
		GuidanceText: "Fresh selected-node guidance invalidates the previously captured Advance authority."}, ExpectedGraphControlRevision: captured.Graph.ControlRevision,
		NodeID: selected.ID, ExpectedNodeControlRevision: selected.ControlRevision, TaskVersion: selected.TaskVersion}
	body, err := json.Marshal(guidance)
	if err != nil {
		t.Fatal(err)
	}
	guided := e.MutateGraph(ctx, "graphs.nodes.guidance", captured.Graph.ID+"/"+selected.Key, body)
	if guided.Status != 200 || guided.Envelope.Error != nil {
		t.Fatal("Actual selected-node guidance prerequisite failed", guided)
	}
	before, err := e.db.Graph(ctx, captured.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingBefore, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := graphGuardArtifactNames(t, e)
	body, err = json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result := e.MutateGraph(ctx, "graphs.advance", captured.Graph.ID, body)
	if result.Status != 409 || result.Envelope.Error == nil || result.Envelope.Error.Code != "graph_control_conflict" {
		t.Fatal("Advance accepted changed authority for the selected node", result)
	}
	after, err := e.db.Graph(ctx, captured.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	pendingAfter, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || !reflect.DeepEqual(pendingBefore, pendingAfter) || !reflect.DeepEqual(before.Budget, after.Budget) || !reflect.DeepEqual(artifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Relevant stale authority performed effects or admissions before conflict", result, pendingBefore, pendingAfter, err)
	}
}
