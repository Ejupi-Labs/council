package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

// Corrupt actual retained payload bytes while keeping every header, seal, inode
// and size intact. A later usable-input path must still hash this member.
func graphProgressCorruptRootMember(t *testing.T, e *Engine, graphID string) {
	t.Helper()
	ctx := context.Background()
	view, err := e.db.Graph(ctx, graphID)
	if err != nil || view.Graph.RootInputID == nil {
		t.Fatal("The real root preparation did not produce a retained root input", view, err)
	}
	record, err := e.db.GraphInputRecord(ctx, graphID, *view.Graph.RootInputID)
	if err != nil || record.PrivateRoot == nil {
		t.Fatal(record, err)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, graphID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := e.graphRootSourceSnapshot(ctx, *record.PrivateRoot, definition.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := e.workspace.VerifyGraphRoot(ctx, *record.PrivateRoot, source)
	if err != nil {
		t.Fatal(err)
	}
	memberID := closureTestEntry(t, proof.Manifest, "source.txt").ArtifactID
	if memberID == record.PrivateRoot.Manifest.ArtifactID || memberID == record.PrivateRoot.Index.ArtifactID || memberID == record.PrivateRoot.StagedBlobs.ArtifactID {
		t.Fatal("The selected payload is a root anchor instead of an ordinary member")
	}
	member, err := e.evidence.VerifyClosed(ctx, memberID)
	if err != nil {
		t.Fatal(err)
	}
	if err := graphDeliveryCorruptExistingMember(e.lock.Root(), member); err != nil {
		t.Fatal(err)
	}
	if _, err := e.evidence.VerifyClosed(ctx, memberID); !errors.Is(err, evidence.ErrCorrupt) {
		t.Fatal("The controlled existing member was not actually corrupted", err)
	}
}

func TestGraphCompletionWithoutInputTargetsDoesNotReopenPhysicalRoot(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	group, source := graphReleaseReadyGroup(t, e, projectID)
	before, err := e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range before.Nodes {
		if node.InputID == nil || node.State != "ready" {
			t.Fatal("The no-target fixture still has an eligible input receiver", node)
		}
	}
	graphProgressCorruptRootMember(t, e, group.Graph.ID)
	artifacts := graphGuardArtifactNames(t, e)
	e.continueGraphInputs(source) // A real current-owner local input terminal.
	after, err := e.db.Graph(ctx, group.Graph.ID)
	gate, gateErr := e.db.Gate(ctx)
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || gateErr != nil || pendingErr != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 || len(pending) != 0 {
		t.Fatal("A completion with no receiving target reopened root bytes or created work", gate, pending, err, gateErr, pendingErr)
	}
	if !reflect.DeepEqual(before.Nodes, after.Nodes) || !reflect.DeepEqual(before.Graph, after.Graph) || before.Budget.Budget.ConsumedAdmissions != after.Budget.Budget.ConsumedAdmissions || !reflect.DeepEqual(artifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("A no-target completion changed graph history, allowance or evidence", before, after)
	}
}

func TestGraphCompletionWithInputTargetStillVerifiesWholePhysicalRoot(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.ctx, e.cancel = ctx, cancel
	creation, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: []domain.GraphNodeInput{
		{Key: "a", OriginalText: "Receive the complete freshly verified root before authoring.", FakeScenario: "graph-response"},
		{Key: "b", OriginalText: "Independently receive the same exact complete root before authoring.", FakeScenario: "graph-response"},
	}}, creation)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	group := created.Envelope.Data.(domain.GraphAdmission)
	root, err := e.db.ClaimAttempt(ctx, group.PreparationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, root)
	terminal, err := e.db.Attempt(ctx, root.ID)
	if err != nil || terminal.Phase != "settled" || terminal.Outcome == nil || *terminal.Outcome != "succeeded" {
		t.Fatal("The actual root operation prerequisite failed", terminal, err)
	}
	graphProgressCorruptRootMember(t, e, group.Graph.ID)
	e.continueGraphInputs(root.ID)
	view, err := e.db.Graph(ctx, group.Graph.ID)
	gate, gateErr := e.db.Gate(ctx)
	inputs, inputErr := e.db.GraphInputs(ctx, group.Graph.ID, 100, "")
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	if err != nil || gateErr != nil || inputErr != nil || pendingErr != nil || gate.State == "open" || gate.IncidentID == nil || len(inputs.Items) != 1 || inputs.NextCursor != nil || len(pending) != 0 || view.Budget.Budget.ConsumedAdmissions != "0" {
		t.Fatal("An eligible receiver bypassed full physical root integrity", view, gate, inputs, pending, err, gateErr, inputErr, pendingErr)
	}
	for _, node := range view.Nodes {
		if node.InputID != nil || node.NodeAdmissionID != nil || node.LatestAttemptID != nil || node.AcceptedResult != nil {
			t.Fatal("A corrupted root supplied usable input or native admission", node)
		}
	}
}
