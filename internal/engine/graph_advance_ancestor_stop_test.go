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

// The stopped scheduler exposes the actual ready-but-unadmitted boundary. All
// source operations, author/reviewer processes and candidate acceptance below
// are real; the fixture does not manufacture a result or change stored controls.
func TestGraphAdvanceDoesNotAdmitReadyDescendantOfStoppedAncestor(t *testing.T) {
	edges := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: "files"}}
	f := graphOutputConfiguredNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Author the controlled file contribution and independently review its complete conditions.",
		FakeScenario: "graph-branch-a", Pins: &domain.ExecutionPins{HarnessID: "codex"}, Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}},
		func(in *domain.GraphStartInput) {
			in.Dependencies = &edges
			in.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "4"}}
		},
		domain.GraphNodeInput{Key: "receiver", OriginalText: "Receive the exact accepted file contribution before returning the complete response.", FakeScenario: "graph-response", Pins: &domain.ExecutionPins{HarnessID: "claude-code"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		domain.GraphNodeInput{Key: "independent", OriginalText: "Return an independent response without changing user files.", FakeScenario: "graph-response", Pins: &domain.ExecutionPins{HarnessID: "codex"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}})
	e, ctx := f.engine, context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	e.ctx, e.cancel = ownerCtx, cancel
	graphID := f.dispatch.Dispatch.GraphID
	t.Cleanup(func() {
		// This case admits but never executes the independent next worker. Any
		// unexpectedly reserved descendant is likewise known not to have run.
		view, err := e.db.Graph(ctx, graphID)
		if err != nil {
			t.Error(err)
			return
		}
		for _, node := range view.Nodes {
			if node.LatestAttemptID == nil {
				continue
			}
			attempt, err := e.db.Attempt(ctx, *node.LatestAttemptID)
			if err != nil {
				t.Error(err)
				continue
			}
			if attempt.Phase == "settled" {
				continue
			}
			if _, err := e.db.SettleWorkflowStep(ctx, attempt.ID, graphReleaseNoProcessFacts(e, attempt)); err != nil {
				t.Error(err)
			}
		}
	})
	e.run(ctx, f.attempt)
	author, err := e.db.Attempt(ctx, f.attempt.ID)
	if err != nil || author.Phase != "settled" || author.Outcome == nil || *author.Outcome != "succeeded" || author.CandidateID == nil || author.Cleanup.State != "verified" {
		t.Fatal("The actual controlled author did not succeed", author, err)
	}
	workflow, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || workflow.Workflow == nil {
		t.Fatal(workflow, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, author.TaskID, *workflow.Workflow)
	if err != nil || progress.decision.Role != "review" {
		t.Fatal("The actual candidate does not require its captured independent review", progress.decision, err)
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, author.StepID)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := e.db.ContinueWorkflow(ctx, e.config.OwnerInstanceID, author.StepID, facts)
	if err != nil || admitted == nil {
		t.Fatal("The actual review was not admitted", admitted, err)
	}
	review, err := e.db.ClaimAttempt(ctx, admitted.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, review)
	workflow, err = e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || workflow.Workflow == nil {
		t.Fatal(workflow, err)
	}
	progress, err = e.inspectWorkflowProgress(ctx, author.TaskID, *workflow.Workflow)
	if err != nil || progress.decision.Action != "accept" || progress.subject.Binding == nil {
		t.Fatal("The actual reviewer did not qualify the original authored candidate", progress.decision, err)
	}
	accepted := e.acceptCurrentWorkflow(ctx, progress, domain.NewID(), review.StepID)
	if accepted.Envelope.Error != nil {
		t.Fatal("The exact reviewed candidate was not accepted", accepted)
	}
	e.continueGraphInputs(review.ID)
	prepared, err := e.db.Graph(ctx, graphID)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"receiver", "independent"} {
		node := graphAncestorNode(t, prepared, key)
		if node.InputID == nil || node.NodeAdmissionID != nil {
			t.Fatal("The exact receiver was not allocated without native admission", node)
		}
		record, err := e.db.GraphInputRecord(ctx, graphID, *node.InputID)
		if err != nil {
			t.Fatal(err)
		}
		local, err := e.db.ClaimAttempt(ctx, record.Public.Summary.OperationAttemptID, e.config.OwnerInstanceID)
		if err != nil {
			t.Fatal(err)
		}
		e.run(ctx, local)
		input, err := e.db.GraphInput(ctx, graphID, *node.InputID)
		if err != nil || input.Summary.State != "ready" || !input.Summary.Current || input.InputBinding == nil {
			t.Fatal("The exact current input did not finish before ancestor Stop", input, err)
		}
	}
	beforeStop, err := e.db.Graph(ctx, graphID)
	if err != nil || beforeStop.Budget.Budget.ConsumedAdmissions != "2" || len(beforeStop.ActiveClaims) != 0 {
		t.Fatal("Preparation spent native allowance or retained an execution", beforeStop, err)
	}
	producer := graphAncestorNode(t, beforeStop, "producer")
	receiver := graphAncestorNode(t, beforeStop, "receiver")
	independent := graphAncestorNode(t, beforeStop, "independent")
	if producer.AcceptedResult == nil || producer.AcceptedResult.ProducerAttemptID != author.ID || producer.AcceptedResult.CandidateID == nil || *producer.AcceptedResult.CandidateID != *author.CandidateID || receiver.State != "ready" || receiver.NodeAdmissionID != nil || independent.State != "ready" {
		t.Fatal("The actual accepted-ancestor/ready-descendant boundary is missing", beforeStop)
	}
	inputBefore, err := e.db.GraphInput(ctx, graphID, *receiver.InputID)
	if err != nil {
		t.Fatal(err)
	}
	historyBefore, err := e.db.Attempts(ctx, producer.TaskID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	stopped := graphBlockedInitialMutate(t, e, "graphs.nodes.stop", graphID+"/"+producer.Key, domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: beforeStop.Graph.ControlRevision, NodeID: producer.ID, ExpectedNodeControlRevision: producer.ControlRevision})
	var stopResult domain.GraphStopResult
	stopWire, encodeErr := json.Marshal(stopped.Envelope.Data)
	decodeErr := json.Unmarshal(stopWire, &stopResult, json.RejectUnknownMembers(true))
	if stopped.Status != 202 || stopped.Envelope.Error != nil || encodeErr != nil || decodeErr != nil || !stopResult.StopAccepted || stopResult.Reason != "accepted" {
		t.Fatal("The ancestor Stop was not actually accepted for its unlaunched descendant", stopped, stopResult, encodeErr, decodeErr)
	}
	current, err := e.db.Graph(ctx, graphID)
	if err != nil {
		t.Fatal(err)
	}
	stoppedProducer := graphAncestorNode(t, current, "producer")
	blocked := graphAncestorNode(t, current, "receiver")
	if stoppedProducer.Stop == nil || !reflect.DeepEqual(producer.AcceptedResult, stoppedProducer.AcceptedResult) || blocked.State != "blocked" || blocked.NextAction != "dependency_stopped" || blocked.Stop != nil || blocked.NodeAdmissionID != nil || blocked.ControlRevision == receiver.ControlRevision || !reflect.DeepEqual(blocked.InputID, receiver.InputID) {
		t.Fatal("Stop did not preserve the exact accepted result and block its unadmitted descendant", current)
	}
	inputAfterStop, err := e.db.GraphInput(ctx, graphID, *blocked.InputID)
	if err != nil || !reflect.DeepEqual(inputBefore, inputAfterStop) {
		t.Fatal("The regression requires a complete retained input after Stop", inputAfterStop, err)
	}
	in := graphBlockedInitialAdvanceInput(current)
	advanced := graphBlockedInitialAdvanceResult(t, graphBlockedInitialMutate(t, e, "graphs.advance", graphID, in))
	if len(advanced.Dispatches) != 1 || advanced.Dispatches[0].Dispatch.NodeID != independent.ID || len(advanced.PreparationAttemptIDs) != 0 || len(advanced.ReboundNodes) != 0 || advanced.Budget.Budget.ConsumedAdmissions != "3" || advanced.Budget.Budget.RemainingAdmissions != "1" {
		t.Fatal("Fresh Advance spent authority on the stopped descendant or blocked the independent sibling", advanced)
	}
	after, err := e.db.Graph(ctx, graphID)
	if err != nil || !reflect.DeepEqual(blocked, graphAncestorNode(t, after, "receiver")) || !reflect.DeepEqual(stoppedProducer, graphAncestorNode(t, after, "producer")) || len(after.ActiveClaims) != 1 || after.ActiveClaims[0].AttemptID != advanced.Dispatches[0].Dispatch.AttemptID {
		t.Fatal("Explicit Advance changed stopped scope or retained an unauthorized claim", after, err)
	}
	receiverHistory, err := e.db.Attempts(ctx, receiver.TaskID, 100, "")
	if err != nil || len(receiverHistory.Items) != 0 {
		t.Fatal("The stopped descendant gained a native attempt", receiverHistory, err)
	}
	replay := graphBlockedInitialAdvanceResult(t, graphBlockedInitialMutate(t, e, "graphs.advance", graphID, in))
	if !reflect.DeepEqual(advanced, replay) {
		t.Fatal("Replay changed the exact receipt or rearmed stopped scope", advanced, replay)
	}
	historyAfter, err := e.db.Attempts(ctx, producer.TaskID, 100, "")
	for i := range historyBefore.Items {
		historyBefore.Items[i].ObservedAt, historyBefore.Items[i].RemainingSeconds = "", 0
	}
	for i := range historyAfter.Items {
		historyAfter.Items[i].ObservedAt, historyAfter.Items[i].RemainingSeconds = "", 0
	}
	if err != nil || !reflect.DeepEqual(historyBefore, historyAfter) {
		t.Fatal("Advance or ancestor Stop rewrote actual accepted producer history", historyAfter, err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(body, f.originalIndex) {
		t.Fatal("The actual Stop/Advance journey changed user staging", err)
	}
	if body, err := os.ReadFile(filepath.Join(f.repo, "source.txt")); err != nil || !bytes.Equal(body, []byte("controlled working source🙂\r\n")) {
		t.Fatal("The actual Stop/Advance journey changed user working bytes", err)
	}
}
