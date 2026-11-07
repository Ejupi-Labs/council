package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

func TestGraphAcknowledgedResponseFeedsActualScopedDependentInput(t *testing.T) {
	e, projectID, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx, e.cancel = ctx, cancel
	dependencies := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: "context"}}
	group, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "producer", OriginalText: "Produce the exact controlled response for explicit user review.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		{Key: "receiver", OriginalText: "Use only the exact acknowledged predecessor response as attributed context.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
	}, Dependencies: &dependencies})
	beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	beforeWorking, err := os.ReadFile(filepath.Join(repo, "source.txt"))
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
		t.Fatal("The real declared dependency admitted an unqualified receiver", view)
	}
	a, err := e.db.ClaimAttempt(ctx, *producer.LatestAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, a)
	in := exactResponseAcceptanceInput(t, e, a.TaskID, a.ID)
	acknowledged := e.AcceptResponse(ctx, a.TaskID, in)
	ack := assertOriginalResponseAcceptanceWire(t, acknowledged, in)
	view, err = e.db.Graph(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Nodes {
		if node.Key == "producer" {
			producer = node
		} else if node.Key == "receiver" {
			receiver = node
		}
	}
	if producer.AcceptedResult == nil || producer.AcceptedResult.AcceptanceID != ack.ID || producer.AcceptedResult.ProducerAttemptID != a.ID {
		t.Fatal("The real acknowledgement did not qualify its original producer", producer)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := e.db.GraphRoot(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeArtifacts := graphGuardArtifactNames(t, e)
	beforeBudget, err := e.db.GraphBudget(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := e.selectGraphContributions(ctx, definition, root, receiver)
	if err != nil || len(selected.Sources) != 1 || len(selected.ContributorVector) != 1 {
		t.Fatal("The real acknowledged predecessor did not provide an exact physical contribution", selected, err)
	}
	c := selected.Sources[0].Binding
	if c.EdgeKind != "context" || c.AcceptanceKind != "response" || c.AcceptanceID != ack.ID || c.FromNodeID != producer.ID || c.ProducerAttemptID != a.ID || c.ResponseBinding == nil || c.ResponseBinding.ResponseArtifactID != in.ResponseArtifactID || c.CandidateBinding != nil || selected.ContributorVector[0].AcceptanceID != ack.ID {
		t.Fatal("Selection promoted another response or inferred file authority", c)
	}
	if len(selected.ConstraintRefs) < 4 || len(selected.EvidenceRefs) != 2 || len(selected.VerifiedArtifacts) == 0 {
		t.Fatal("The accepted result omitted complete historical source/acceptance/result evidence", selected)
	}
	for _, constraint := range selected.ConstraintRefs {
		if constraint.AuthorityScope != "upstream_local" || constraint.NodeID == nil || *constraint.NodeID != producer.ID || constraint.TaskVersion == nil || *constraint.TaskVersion != producer.TaskVersion {
			t.Fatal("A historical peer source was promoted to receiving authority", constraint)
		}
	}
	readResponse := false
	for _, evidence := range selected.EvidenceRefs {
		if evidence.ArtifactRef.ArtifactID != in.ResponseArtifactID {
			continue
		}
		body, err := e.readGraphArtifact(ctx, evidence.ArtifactRef, "final_response", 64<<20)
		if err != nil || string(body) != fixture.GraphResponse {
			t.Fatal("The actual complete accepted response was not available", err)
		}
		readResponse = true
	}
	if !readResponse {
		t.Fatal("Selection replaced full response access with a digest or summary")
	}
	afterBudget, err := e.db.GraphBudget(ctx, group.Graph.ID)
	if err != nil || beforeBudget.Budget.ConsumedAdmissions != "1" || !reflect.DeepEqual(beforeBudget, afterBudget) || !reflect.DeepEqual(beforeArtifacts, graphGuardArtifactNames(t, e)) {
		t.Fatal("Accepted source observation consumed work or wrote artifacts", afterBudget, err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatal("Accepted source observation changed the user's staging", err)
	}
	afterWorking, err := os.ReadFile(filepath.Join(repo, "source.txt"))
	if err != nil || !bytes.Equal(beforeWorking, afterWorking) {
		t.Fatal("Accepted source observation changed the user's working file", err)
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	rootProofs, err := e.verifyGraphRootClosure(ctx, root, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := mergeGraphAcceptanceProofs(rootProofs, selected.VerifiedArtifacts)
	if err != nil {
		t.Fatal(err)
	}
	if view.DispatchAuthority == nil {
		t.Fatal("The pending declared receiver lost its explicit graph registration")
	}
	nodeID, taskID, version, revision := receiver.ID, receiver.TaskID, receiver.TaskVersion, receiver.ControlRevision
	contributions := make([]domain.AcceptedContribution, 0, len(selected.Sources))
	for _, source := range selected.Sources {
		contributions = append(contributions, source.Binding)
	}
	preparation := domain.GraphInputPreparationFacts{Trigger: domain.GraphFreshTrigger{Kind: "response_ack", ID: ack.ID, OwnerGeneration: view.DispatchAuthority.Generation}, RootBinding: root,
		Control: domain.GraphDispatchBinding{GraphID: view.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID, OperationAttemptID: a.ID,
			OwnerInstanceID: e.config.OwnerInstanceID, Generation: view.DispatchAuthority.Generation, GraphControlRevision: view.Graph.ControlRevision,
			BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
			NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision, ContributorVector: selected.ContributorVector},
		Contributions: contributions, ContributorVector: selected.ContributorVector, VerifiedArtifacts: proofs}
	if receiver.InputID == nil {
		t.Fatal("The actual fresh acknowledgement did not register the dependent input")
	}
	if repeated, err := e.db.BeginGraphInput(ctx, e.config.OwnerInstanceID, view.Graph.ID, nodeID, preparation); err == nil {
		t.Fatal("The already consumed acknowledgement registered a duplicate receiver", repeated)
	}
	record, err := e.db.GraphInputRecord(ctx, view.Graph.ID, *receiver.InputID)
	if err != nil || record.Trigger.Kind != "response_ack" || record.Trigger.ID != ack.ID {
		t.Fatal("The callback did not consume the actual fresh acknowledgement", record, err)
	}
	local, err := e.db.ClaimAttempt(ctx, record.Public.Summary.OperationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, local)
	retained, err := e.db.GraphInput(ctx, view.Graph.ID, *receiver.InputID)
	if err != nil || retained.Summary.State != "ready" || retained.InputBinding == nil || retained.ContextPacket == nil || len(retained.InputBinding.Contributions) != 1 || len(retained.InputBinding.ContributorVector) != 1 {
		t.Fatal("The actual root-local runner did not retain complete acknowledged dependent input", retained, err)
	}
	if retained.InputBinding.Contributions[0].AcceptanceID != ack.ID || retained.InputBinding.Contributions[0].ResponseBinding == nil || retained.InputBinding.Contributions[0].ResponseBinding.ProducerAttemptID != a.ID {
		t.Fatal("The receiver lost its exact acknowledged response provenance", retained.InputBinding)
	}
	upstream := 0
	for _, constraint := range retained.ContextPacket.ConstraintRefs {
		if constraint.AuthorityScope == "upstream_local" {
			upstream++
		}
	}
	if upstream < len(selected.ConstraintRefs) {
		t.Fatal("The actual receiver omitted selected upstream condition/original access", retained.ContextPacket)
	}
	finalBudget, err := e.db.GraphBudget(ctx, group.Graph.ID)
	if err != nil || finalBudget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("Local dependent composition invoked or reserved a model", finalBudget, err)
	}
}
