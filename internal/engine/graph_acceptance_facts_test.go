package engine

import (
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

func graphAcceptanceSubjectFixture(t *testing.T) (graphOutputProducerCase, domain.Attempt, workflowSubject) {
	t.Helper()
	f, supporting := graphNativeReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	e.run(ctx, supporting)
	view, err := e.db.Workflow(ctx, supporting.TaskID, domain.WorkflowSelector{WorkflowID: supporting.WorkflowID})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, supporting.TaskID, *view.Workflow)
	if err != nil || progress.decision.Action != "accept" || progress.subject.Binding == nil || progress.subject.Candidate == nil || progress.subject.Candidate.AttemptID != f.attempt.ID {
		t.Fatal("Actual author/review did not produce an exact acceptance subject", progress.decision, err)
	}
	return f, supporting, progress.subject
}

func graphAcceptanceExpectedAnchors(t *testing.T, f graphOutputProducerCase, subject workflowSubject) []string {
	t.Helper()
	e, ctx := f.engine, context.Background()
	record, err := e.db.GraphDispatchForAttempt(ctx, subject.Candidate.AttemptID)
	if err != nil || record.Public.OutputObservation == nil || record.Public.OutputObservation.Candidate == nil || record.Public.OutputObservation.Candidate.ClosureRef == nil {
		t.Fatal("Actual selected producer closure is unavailable", record, err)
	}
	ref := *record.Public.OutputObservation.Candidate.ClosureRef
	body, err := e.readGraphArtifact(ctx, ref, "graph_output_closure", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	var closure domain.GraphOutputClosure
	if err := json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	input, err := e.db.GraphInputRecord(ctx, record.Public.Dispatch.GraphID, record.Public.Dispatch.InputID)
	if err != nil || input.Public.InputBinding == nil || input.Public.ConditionsRef == nil {
		t.Fatal(input, err)
	}
	binding := input.Public.InputBinding
	return []string{ref.ArtifactID, binding.Manifest.ArtifactID, binding.Index.ArtifactID, binding.StagedBlobs.ArtifactID, binding.Provenance.ArtifactID, binding.ContextPacket.ArtifactID, input.Public.ConditionsRef.ArtifactID,
		closure.Manifest.ArtifactID, closure.Index.ArtifactID, closure.StagedBlobs.ArtifactID}
}

func TestGraphAcceptanceProofsStandaloneCreateNoGraphEvidence(t *testing.T) {
	e, admitted := watchFixture(t)
	ctx := context.Background()
	before, err := e.db.Artifacts(ctx, admitted.Attempt.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := e.graphAcceptanceArtifactProofs(ctx, workflowSubject{Workflow: domain.Workflow{TaskID: admitted.Task.ID}})
	if err != nil || len(proofs) != 0 {
		t.Fatal("Standalone acknowledgement acquired graph proofs or authority", proofs, err)
	}
	after, err := e.db.Artifacts(ctx, admitted.Attempt.ID, 100, "")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Standalone graph-proof inspection created evidence", err)
	}
}

func TestGraphAcceptanceProofsUseSelectedProducerAndSurviveOwnerRestart(t *testing.T) {
	f, supporting, subject := graphAcceptanceSubjectFixture(t)
	e, ctx := f.engine, context.Background()
	expected := graphAcceptanceExpectedAnchors(t, f, subject)
	beforeBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := e.graphAcceptanceArtifactProofs(ctx, subject)
	if err != nil {
		t.Fatal("Actual retained selected graph acknowledgement proofs unavailable", err)
	}
	seen := map[string]bool{}
	for _, proof := range proofs {
		if seen[proof.ArtifactID] {
			t.Fatal("Graph acceptance returned duplicate physical proof IDs")
		}
		seen[proof.ArtifactID] = true
		meta, err := e.evidence.VerifyClosed(ctx, proof.ArtifactID)
		if err != nil || meta.SHA256 == nil || *meta.SHA256 != proof.SHA256 || meta.ByteLength != proof.ByteLength || meta.Identity != proof.PhysicalIdentity {
			t.Fatal("Acknowledgement proof is not actual fresh closed evidence", meta, err)
		}
	}
	for _, id := range expected {
		if !seen[id] {
			t.Fatal("Graph acceptance omitted a selected producer/input/conditions closure anchor", id)
		}
	}
	if len(proofs) > 256 {
		t.Fatal("Graph acceptance retained an unbounded payload inventory in SQL facts")
	}
	// Reopen through the real owner recovery API after every worker settled.
	// No old native generation is refreshed and no retained result is replayed.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := next.Close(); err != nil {
			t.Error(err)
		}
	})
	stopFixtureScheduler(t, next)
	if err := next.db.CheckGraphDispatchPreparation(ctx, next.config.OwnerInstanceID, f.attempt.ID); err == nil {
		t.Fatal("Restart fixture unexpectedly retained old native execution authority")
	}
	fresh, err := next.graphAcceptanceArtifactProofs(ctx, subject)
	if err != nil || len(fresh) != len(proofs) {
		t.Fatal("Nonexecuting acknowledgement incorrectly required old live/native owner authority", fresh, err)
	}
	for _, proof := range fresh {
		if !seen[proof.ArtifactID] {
			t.Fatal("Owner recovery changed the selected acknowledgement input", proof.ArtifactID)
		}
	}
	afterBudget, err := next.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || afterBudget.Budget.ConsumedAdmissions != beforeBudget.Budget.ConsumedAdmissions {
		t.Fatal("Physical acknowledgement proof collection consumed new allowance", afterBudget, err)
	}
	current, err := next.db.Attempt(ctx, supporting.ID)
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" {
		t.Fatal("Proof inspection replayed or rewrote the supporting execution", current, err)
	}
}

func TestGraphAcceptanceProofsRejectChangedSelectedClosureAndUnstagedPayload(t *testing.T) {
	for _, changed := range []string{"selected-closure", "unstaged-working-payload"} {
		t.Run(changed, func(t *testing.T) {
			f, _, subject := graphAcceptanceSubjectFixture(t)
			e, ctx := f.engine, context.Background()
			id := ""
			if changed == "selected-closure" {
				record, err := e.db.GraphDispatchForAttempt(ctx, f.attempt.ID)
				if err != nil || record.Public.OutputObservation == nil || record.Public.OutputObservation.Candidate == nil || record.Public.OutputObservation.Candidate.ClosureRef == nil {
					t.Fatal(record, err)
				}
				id = record.Public.OutputObservation.Candidate.ClosureRef.ArtifactID
			} else {
				for _, entry := range subject.Manifest.Entries {
					if entry.Path == fixture.GraphFileA {
						id = entry.ArtifactID
					}
				}
				for _, entry := range subject.Manifest.Index {
					if entry.Path == fixture.GraphFileA {
						t.Fatal("Fixture payload is staged and would not test the working-only closure")
					}
				}
			}
			if id == "" {
				t.Fatal("Actual selected artifact to corrupt is unavailable")
			}
			meta, err := e.db.Artifact(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(filepath.Join(e.lock.Root(), filepath.FromSlash(meta.PrivateRef)), os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.WriteAt([]byte("!"), 0)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatal(writeErr, closeErr)
			}
			if _, err := e.graphAcceptanceArtifactProofs(ctx, subject); err == nil {
				t.Fatal("Changed actual retained graph output was accepted from stale previously-read subject data")
			}
		})
	}
}

func TestGraphAutoResponseHumanAcknowledgementAfterRestartAndAttemptExpiry(t *testing.T) {
	timeout := 15
	f := graphOutputAdmittedNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Return the complete controlled response for human review. Preserve files and staged intent.\r\n",
		FakeScenario: "graph-response", TimeoutSeconds: &timeout, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}})
	old, a := f.engine, f.attempt
	ctx := context.Background()
	working, originalIndex := graphNativeRunUserState(t, f)
	old.run(ctx, a)
	produced, err := old.db.Attempt(ctx, a.ID)
	if err != nil || produced.Outcome == nil || *produced.Outcome != "succeeded" || produced.ResponseArtifactID == nil || produced.CandidateID == nil {
		t.Fatal("Actual Auto response producer did not finish before its captured ceiling", produced, err)
	}
	view, err := old.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil || view.Workflow.SelectedResponseBinding == nil {
		t.Fatal(view, err)
	}
	progress, err := old.inspectWorkflowProgress(ctx, a.TaskID, *view.Workflow)
	if err != nil || progress.decision.Action != "response_acceptance_pending" || progress.decision.Role != "" {
		t.Fatal("Auto response incorrectly demanded an extra model review", progress.decision, err)
	}
	selected := *view.Workflow.SelectedResponseBinding
	if selected.ProducerAttemptID != a.ID || selected.ResponseArtifactID != *produced.ResponseArtifactID {
		t.Fatal("Auto response did not select the actual original producer")
	}
	budget, err := old.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("One Auto response spent additional model allowance", budget, err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(old.db, old.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := next.Close(); err != nil {
			t.Error(err)
		}
	})
	stopFixtureScheduler(t, next)
	// This is the actual captured attempt ceiling, not a backdated row or a
	// shared-window grant. Nonexecuting human acknowledgement remains meaningful.
	deadline, err := time.Parse(time.RFC3339Nano, produced.DeadlineAt)
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(deadline) + 5*time.Millisecond
	if remaining > 16*time.Second {
		t.Fatal("Fixture did not capture the explicit finite attempt ceiling")
	}
	if remaining > 0 {
		time.Sleep(remaining)
	}
	if time.Now().Before(deadline) {
		t.Fatal("Attempt-expiry precondition did not occur")
	}
	detail, err := next.db.Task(ctx, a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	input := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: a.TaskVersion, AttemptID: a.ID,
		ResponseArtifactID: selected.ResponseArtifactID, ResponseSHA256: selected.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}
	accepted := next.AcceptResponse(ctx, a.TaskID, input)
	if accepted.Envelope.Error != nil || accepted.Status != 200 {
		t.Fatal("Nonexecuting original response acknowledgement was blocked after restart/attempt expiry", accepted)
	}
	detail, err = next.db.Task(ctx, a.TaskID)
	if err != nil || detail.Acceptance == nil || detail.Acceptance.AttemptID != a.ID || detail.Acceptance.ResponseArtifactID != selected.ResponseArtifactID || detail.Acceptance.ResponseSHA256 != selected.ResponseSHA256 {
		t.Fatal("Human acknowledgement changed the original selected response", detail, err)
	}
	ack := *detail.Acceptance
	graph, err := next.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || len(graph.Nodes) != 1 || graph.Nodes[0].AcceptedResult == nil {
		t.Fatal("Actual human acknowledgement did not project its graph result", graph, err)
	}
	result := graph.Nodes[0].AcceptedResult
	if result.AcceptanceKind != "response" || result.AcceptanceID != ack.ID || result.ProducerAttemptID != a.ID || result.CandidateID != nil || result.ResponseArtifactID == nil || *result.ResponseArtifactID != selected.ResponseArtifactID || result.InputID != f.inputBinding.ID || graph.Graph.State != "completed" {
		t.Fatal("Human response acknowledgement fabricated file delivery or another producer", result, graph.Graph)
	}
	stored, err := next.db.GraphResponseAcceptance(ctx, graph.Graph.ID, graph.Nodes[0].ID, ack.ID)
	if err != nil || !reflect.DeepEqual(stored, ack) {
		t.Fatal("Exact current graph acknowledgement read does not identify the actual user mutation", stored, err)
	}
	producer, err := next.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || producer.Public.OutputObservation == nil || producer.Public.OutputObservation.Response == nil || producer.Public.OutputObservation.Response.State != "complete" || producer.Public.OutputObservation.Response.ClosureRef == nil {
		t.Fatal("Accepted response has no actual complete original source closure", producer, err)
	}
	if _, err := next.readGraphArtifact(ctx, *producer.Public.OutputObservation.Response.ClosureRef, "graph_output_closure", 64<<20); err != nil {
		t.Fatal(err)
	}
	checkAckEvent := func() {
		t.Helper()
		page, err := next.db.Events(ctx, a.ID, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, event := range page.Items {
			if event.Kind != "graph_response_acknowledged" {
				continue
			}
			count++
			var meta struct {
				AcceptanceID    string `json:"acceptanceId"`
				RequestID       string `json:"requestId"`
				GraphID         string `json:"graphId"`
				NodeID          string `json:"nodeId"`
				TaskVersion     string `json:"taskVersion"`
				OwnerInstanceID string `json:"ownerInstanceId"`
			}
			if err := json.Unmarshal(event.Metadata, &meta, json.RejectUnknownMembers(true)); err != nil || meta.AcceptanceID != ack.ID || meta.RequestID != input.RequestID || meta.GraphID != graph.Graph.ID || meta.NodeID != graph.Nodes[0].ID || meta.TaskVersion != a.TaskVersion || meta.OwnerInstanceID != next.config.OwnerInstanceID {
				t.Fatal("Fresh acknowledgement trigger is not the actual user acceptance/current owner", meta, err)
			}
		}
		if count != 1 {
			t.Fatal("Original acknowledgement/replay must produce exactly one fresh response-ack event", count)
		}
	}
	checkAckEvent()
	replayed := next.AcceptResponse(ctx, a.TaskID, input)
	if replayed.Envelope.Error != nil || replayed.Envelope.Receipt == nil || accepted.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *accepted.Envelope.Receipt {
		t.Fatal("Exact response acknowledgement replay did not retain its original receipt", replayed)
	}
	checkAckEvent()
	pending, err := next.db.PendingAttempts(ctx, next.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 0 {
		t.Fatal("Acknowledgement or its replay dispatched new work after recovery", pending, err)
	}
	afterBudget, err := next.db.GraphBudget(ctx, graph.Graph.ID)
	if err != nil || afterBudget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("Human acknowledgement/replay spent a new model admission", afterBudget, err)
	}
	retained, err := next.db.Attempt(ctx, a.ID)
	if err != nil || retained.SettledAt == nil || produced.SettledAt == nil || *retained.SettledAt != *produced.SettledAt || retained.OwnerInstanceID != produced.OwnerInstanceID || retained.DeadlineAt != produced.DeadlineAt || !reflect.DeepEqual(retained.Outcome, produced.Outcome) {
		t.Fatal("Metadata acknowledgement rewrote the original execution terminal", retained, err)
	}
	graphNativeRunAssertUserState(t, f, working, originalIndex)
}

func TestGraphHumanResponseAcknowledgementAfterCommonWindowExpiryDoesNotRearm(t *testing.T) {
	e, projectID, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.ctx, e.cancel = ownerCtx, cancel
	seconds := 15
	edges := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: "context"}}
	group, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{
		Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{WindowSeconds: &seconds}},
		Nodes: []domain.GraphNodeInput{
			{Key: "producer", OriginalText: "Return the complete controlled response for explicit human acknowledgement.\r\n", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
			{Key: "receiver", OriginalText: "Keep the exact acknowledged response as context, and wait for actual remaining execution authority.\r\n", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		}, Dependencies: &edges,
	})
	working, err := os.ReadFile(filepath.Join(repo, "source.txt"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	e.continueGraphAdmissions(source)
	view, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	var producer, receiver domain.GraphNodeSummary
	for _, node := range view.Nodes {
		if node.Key == "producer" {
			producer = node
		} else if node.Key == "receiver" {
			receiver = node
		}
	}
	if producer.LatestAttemptID == nil || receiver.InputID != nil || receiver.LatestAttemptID != nil {
		t.Fatal("The controlled declared context edge requires exactly one independent native admission", view)
	}
	author, err := e.db.ClaimAttempt(ctx, *producer.LatestAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, author)
	terminal, err := e.db.Attempt(ctx, author.ID)
	if err != nil || terminal.Outcome == nil || *terminal.Outcome != "succeeded" || terminal.ResponseArtifactID == nil {
		t.Fatal("The actual response did not qualify before its configured common window expired", terminal, err)
	}
	in := exactResponseAcceptanceInput(t, e, author.TaskID, author.ID)
	beforeBudget, err := e.db.GraphBudget(ctx, group.Graph.ID)
	if err != nil || beforeBudget.Budget.ConsumedAdmissions != "1" || beforeBudget.Budget.CurrentWindow.WindowSeconds != seconds ||
		beforeBudget.Budget.CurrentWindow.FirstAdmissionAt == nil || beforeBudget.Budget.CurrentWindow.DeadlineAt == nil {
		t.Fatal("The actual first native admission did not start the explicitly configured shared window", beforeBudget, err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, *beforeBudget.Budget.CurrentWindow.DeadlineAt)
	if err != nil {
		t.Fatal(err)
	}
	firstAdmission, err := time.Parse(time.RFC3339Nano, *beforeBudget.Budget.CurrentWindow.FirstAdmissionAt)
	if err != nil || deadline.Sub(firstAdmission) != time.Duration(seconds)*time.Second {
		t.Fatal("The persisted common window is not the actual configured first-admission window", beforeBudget.Budget.CurrentWindow, err)
	}
	beforeInputs, err := e.db.GraphInputs(ctx, group.Graph.ID, 100, "")
	if err != nil || len(beforeInputs.Items) != 2 || beforeInputs.NextCursor != nil {
		t.Fatal("An unacknowledged response unexpectedly prepared its dependent input", beforeInputs, err)
	}
	beforeArtifacts := graphGuardArtifactNames(t, e)
	// The real shared deadline expires without modifying a persisted timestamp,
	// creating a replacement window or spending another model admission.
	remaining := time.Until(deadline) + 5*time.Millisecond
	if remaining > 16*time.Second {
		t.Fatal("The explicit common-window fixture exceeded its finite wait bound")
	}
	if remaining > 0 {
		time.Sleep(remaining)
	}
	if time.Now().Before(deadline) {
		t.Fatal("The actual shared budget window has not expired")
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || gate.OwnerInstanceID != e.config.OwnerInstanceID || len(gate.ActiveClaims) != 0 {
		t.Fatal("The nonexecuting acknowledgement precondition has unrelated ownership uncertainty", gate, err)
	}
	beforeAck, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil || beforeAck.DispatchAuthority == nil || beforeAck.DispatchAuthority.OwnerInstanceID != e.config.OwnerInstanceID || beforeAck.DispatchAuthority.State == "suspended" {
		t.Fatal("This expiry case lost current-owner registration before the human action", beforeAck, err)
	}
	accepted := e.AcceptResponse(ctx, author.TaskID, in)
	ack := assertOriginalResponseAcceptanceWire(t, accepted, in)
	if ack.AttemptID != author.ID || ack.ResponseArtifactID != in.ResponseArtifactID || ack.ResponseSHA256 != in.ResponseSHA256 {
		t.Fatal("Window-expired acknowledgement replaced the original response producer", ack)
	}
	assertExpiredObservation := func() {
		t.Helper()
		if read := e.readGraph(ctx, "graphs.get", group.Graph.ID, nil); read.Status != 200 || read.Envelope.Error != nil {
			t.Fatal("The completed response and expired shared allowance are not observable", read)
		}
		after, err := e.db.Graph(ctx, group.Graph.ID)
		if err != nil || after.Graph.State == "completed" || after.Graph.Counts.Completed != "1" || after.Graph.Counts.Required != "2" {
			t.Fatal("An unfinished declared receiver became a successful graph after response acknowledgement", after, err)
		}
		for _, node := range after.Nodes {
			if node.ID == producer.ID && (node.AcceptedResult == nil || node.AcceptedResult.AcceptanceKind != "response" || node.AcceptedResult.AcceptanceID != ack.ID || node.AcceptedResult.ProducerAttemptID != author.ID) {
				t.Fatal("The nonexecuting acknowledgement did not preserve its qualified result", node)
			}
			if node.ID == receiver.ID && (node.InputID != nil || node.LatestAttemptID != nil || node.AcceptedResult != nil) {
				t.Fatal("Human acknowledgement rearmed dependent preparation after shared-window expiry", node)
			}
		}
		stored, err := e.db.GraphResponseAcceptance(ctx, group.Graph.ID, producer.ID, ack.ID)
		if err != nil || !reflect.DeepEqual(stored, ack) {
			t.Fatal("The actual acknowledged response is not retained independently of expired execution authority", stored, err)
		}
		afterBudget, err := e.db.GraphBudget(ctx, group.Graph.ID)
		if err != nil || !reflect.DeepEqual(beforeBudget, afterBudget) {
			t.Fatal("Acknowledgement refreshed the shared window, epoch, allowance or model debit", afterBudget, err)
		}
		afterInputs, err := e.db.GraphInputs(ctx, group.Graph.ID, 100, "")
		if err != nil || !reflect.DeepEqual(beforeInputs, afterInputs) || !reflect.DeepEqual(beforeArtifacts, graphGuardArtifactNames(t, e)) {
			t.Fatal("Expiry acknowledgement or observation wrote new preparation artifacts", afterInputs, err)
		}
		pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
		if err != nil || len(pending) != 0 {
			t.Fatal("Expiry acknowledgement or observation created new local/native work", pending, err)
		}
		events, err := e.db.Events(ctx, author.ID, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, event := range events.Items {
			if event.Kind == "graph_response_acknowledged" {
				count++
			}
		}
		if count != 1 {
			t.Fatal("Expired-window receipt replay manufactured another fresh acknowledgement", count)
		}
	}
	assertExpiredObservation()
	if replay := e.AcceptResponse(ctx, author.TaskID, in); assertOriginalResponseAcceptanceWire(t, replay, in).ID != ack.ID {
		t.Fatal("Expired-window acknowledgement receipt replay changed its original identity", replay)
	}
	assertExpiredObservation()
	afterWorking, err := os.ReadFile(filepath.Join(repo, "source.txt"))
	if err != nil || !reflect.DeepEqual(working, afterWorking) {
		t.Fatal("Shared-window acknowledgement changed the user's working file", err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !reflect.DeepEqual(index, afterIndex) {
		t.Fatal("Shared-window acknowledgement changed the user's exact staged index", err)
	}
	retained, err := e.db.Attempt(ctx, author.ID)
	if err != nil || retained.OwnerInstanceID != terminal.OwnerInstanceID || retained.DeadlineAt != terminal.DeadlineAt || !reflect.DeepEqual(retained.SettledAt, terminal.SettledAt) || !reflect.DeepEqual(retained.Outcome, terminal.Outcome) {
		t.Fatal("Human acknowledgement rewrote its original native terminal after window expiry", retained, err)
	}
}
