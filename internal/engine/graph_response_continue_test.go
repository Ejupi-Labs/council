package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

func TestGraphFreshResponseAcknowledgementPreparesDependentInputOnceWithoutReplay(t *testing.T) {
	e, projectID, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	ownerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e.ctx, e.cancel = ownerCtx, cancel
	edges := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: "context"}}
	group, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "producer", OriginalText: "Return the exact controlled response for explicit human acknowledgement.\r\n", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		{Key: "receiver", OriginalText: "Keep the complete acknowledged predecessor response as attributed context.\r\n", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
	}, Dependencies: &edges})
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
	if producer.LatestAttemptID == nil || receiver.InputID != nil {
		t.Fatal("The real declared context edge admitted an unacknowledged receiver", view)
	}
	author, err := e.db.ClaimAttempt(ctx, *producer.LatestAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, author)
	in := exactResponseAcceptanceInput(t, e, author.TaskID, author.ID)
	before, err := e.db.GraphInputs(ctx, group.Graph.ID, 100, "")
	if err != nil || len(before.Items) != 2 || before.NextCursor != nil {
		t.Fatal("The controlled root and independent producer input prerequisite failed", before, err)
	}
	read := func(owner *Engine) {
		t.Helper()
		for _, operation := range []string{"graphs.get", "graphs.definition.get", "graphs.budget.get", "graphs.inputs.list", "graphs.dispatches.list"} {
			if result := owner.readGraph(ctx, operation, group.Graph.ID, nil); result.Status != 200 || result.Envelope.Error != nil {
				t.Fatal("An actual graph observation failed", operation, result)
			}
		}
	}
	read(e)
	unchanged, err := e.db.GraphInputs(ctx, group.Graph.ID, 100, "")
	if err != nil || !reflect.DeepEqual(before, unchanged) {
		t.Fatal("Observation prepared a dependent input before human acknowledgement", unchanged, err)
	}

	// Hold the preparation gate after the actual acknowledgement commit. Exact
	// concurrent receipt retries must return while the first response may still
	// be awaiting its fresh callback; they cannot manufacture another callback.
	e.prepareGate.Lock()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(e.prepareGate.Unlock) }
	defer release()
	committed := make(chan struct{})
	var committedOnce sync.Once
	previousAfterCommit := e.db.AfterCommit
	e.db.AfterCommit = func() error {
		if previousAfterCommit != nil {
			if err := previousAfterCommit(); err != nil {
				return err
			}
		}
		committedOnce.Do(func() { close(committed) })
		return nil
	}
	first := make(chan domain.Result, 1)
	firstCollected := false
	var firstResult *domain.Result
	defer func() {
		release()
		if !firstCollected {
			select {
			case <-first:
			case <-time.After(5 * time.Second):
				t.Error("The controlled acknowledgement did not finish after releasing its preparation gate")
			}
		}
		e.db.AfterCommit = previousAfterCommit
	}()
	go func() { first <- e.AcceptResponse(ctx, author.TaskID, in) }()
	select {
	case <-committed:
	case result := <-first:
		firstCollected = true
		select {
		case <-committed:
			firstResult = &result
		default:
			t.Fatal("The actual acknowledgement did not reach its durable commit", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("The actual acknowledgement did not commit within its bounded fixture window")
	}
	retries := make(chan domain.Result, 4)
	for range 4 {
		go func() { retries <- e.AcceptResponse(ctx, author.TaskID, in) }()
	}
	var ack domain.ResponseAcceptance
	for range 4 {
		select {
		case result := <-retries:
			actual := assertOriginalResponseAcceptanceWire(t, result, in)
			if ack.ID != "" && !reflect.DeepEqual(actual, ack) {
				t.Fatal("Concurrent receipt retries selected another acknowledgement", actual, ack)
			}
			ack = actual
		case <-time.After(5 * time.Second):
			t.Fatal("An exact concurrent receipt retry waited for new graph preparation")
		}
	}
	release()
	if firstResult == nil {
		select {
		case result := <-first:
			firstCollected = true
			firstResult = &result
		case <-time.After(5 * time.Second):
			t.Fatal("The fresh acknowledgement callback did not finish within its bounded fixture window")
		}
	}
	if actual := assertOriginalResponseAcceptanceWire(t, *firstResult, in); !reflect.DeepEqual(actual, ack) {
		t.Fatal("The first response and concurrent receipts disagree", actual, ack)
	}
	e.db.AfterCommit = previousAfterCommit
	pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || len(pending) != 1 || pending[0].ExecutorKind != "local_graph" || pending[0].TaskID != group.Graph.RootTaskID || pending[0].Phase != "queued" {
		t.Fatal("Fresh human acknowledgement did not prepare exactly one dependent local input", pending, err)
	}
	queued := pending[0]
	local, err := e.db.ClaimAttempt(ctx, queued.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := e.db.GraphInputOperationForAttempt(ctx, e.config.OwnerInstanceID, local.ID)
	if err != nil || op.Kind != "node_input" || op.Control.OperationAttemptID != local.ID || op.Control.NodeID == nil || *op.Control.NodeID != receiver.ID ||
		op.InputRecord.Trigger.Kind != "response_ack" || op.InputRecord.Trigger.ID != ack.ID || len(op.Contributions) != 1 || len(op.ContributorVector) != 1 ||
		op.Contributions[0].AcceptanceID != ack.ID || op.Contributions[0].ResponseBinding == nil || op.Contributions[0].ResponseBinding.ProducerAttemptID != author.ID {
		t.Fatal("The callback inferred another producer, stale completion or file authority", op, err)
	}
	e.run(ctx, local)
	input, err := e.db.GraphInput(ctx, group.Graph.ID, op.ID)
	if err != nil || input.Summary.State != "ready" || input.InputBinding == nil || input.ContextPacket == nil || len(input.InputBinding.Contributions) != 1 ||
		input.InputBinding.Contributions[0].AcceptanceID != ack.ID || input.InputBinding.Contributions[0].CandidateBinding != nil {
		t.Fatal("The fresh callback's actual local runner did not retain complete response input", input, err)
	}
	foundResponse := false
	for _, ref := range input.ContextPacket.ContributedEvidence {
		if ref.ArtifactRef.ArtifactID != in.ResponseArtifactID {
			continue
		}
		body, err := e.readGraphArtifact(ctx, ref.ArtifactRef, "final_response", 64<<20)
		if err != nil || string(body) != fixture.GraphResponse || ref.AcceptanceID != ack.ID || ref.ProducerAttemptID != author.ID {
			t.Fatal("The prepared receiver lost full exact acknowledged response access", ref, err)
		}
		foundResponse = true
	}
	if !foundResponse {
		t.Fatal("The callback retained only a summary or hash instead of the complete response")
	}
	inputs, err := e.db.GraphInputs(ctx, group.Graph.ID, 100, "")
	if err != nil || len(inputs.Items) != 3 || inputs.NextCursor != nil {
		t.Fatal("Concurrent acknowledgement prepared multiple dependent inputs", inputs, err)
	}
	artifacts := graphGuardArtifactNames(t, e)
	assertNoNewPreparation := func(owner *Engine) {
		t.Helper()
		read(owner)
		after, err := owner.db.GraphInputs(ctx, group.Graph.ID, 100, "")
		if err != nil || !reflect.DeepEqual(after, inputs) || !reflect.DeepEqual(artifacts, graphGuardArtifactNames(t, owner)) {
			t.Fatal("Observation or exact acknowledgement replay created new preparation evidence", after, err)
		}
		pending, err := owner.db.PendingAttempts(ctx, owner.config.OwnerInstanceID, 100)
		if err != nil || len(pending) != 0 {
			t.Fatal("Observation, restart or receipt replay dispatched work", pending, err)
		}
		budget, err := owner.db.GraphBudget(ctx, group.Graph.ID)
		if err != nil || budget.Budget.ConsumedAdmissions != "1" {
			t.Fatal("Human acknowledgement or local input preparation spent a model admission", budget, err)
		}
		events, err := owner.db.Events(ctx, author.ID, 100, "")
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
			t.Fatal("An acknowledgement receipt replay manufactured another fresh event", count)
		}
	}
	assertNoNewPreparation(e)
	if replay := e.AcceptResponse(ctx, author.TaskID, in); assertOriginalResponseAcceptanceWire(t, replay, in).ID != ack.ID {
		t.Fatal("Exact original acknowledgement replay changed its identity", replay)
	}
	assertNoNewPreparation(e)
	if err = e.Close(); err != nil {
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
	assertNoNewPreparation(next)
	if replay := next.AcceptResponse(ctx, author.TaskID, in); assertOriginalResponseAcceptanceWire(t, replay, in).ID != ack.ID {
		t.Fatal("Restart observation changed exact acknowledgement receipt replay", replay)
	}
	assertNoNewPreparation(next)
	afterWorking, err := os.ReadFile(filepath.Join(repo, "source.txt"))
	if err != nil || !bytes.Equal(working, afterWorking) {
		t.Fatal("Acknowledgement-driven local preparation changed the user's working file", err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(index, afterIndex) {
		t.Fatal("Acknowledgement-driven local preparation changed the user's staging", err)
	}
}
