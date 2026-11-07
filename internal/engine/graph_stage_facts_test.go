package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphWorkflowStageFactsRetainActualSelectedSubjectAndAdmissionAnchor(t *testing.T) {
	f := graphOutputAdmittedNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: "Create the first controlled file result and obtain independent review.", FakeScenario: "graph-branch-a", Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete authored file change and its source conditions."}})
	e, a := f.engine, f.attempt
	ctx := context.Background()
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || current.Outcome == nil || *current.Outcome != "succeeded" || current.CandidateID == nil {
		t.Fatal("The actual owned author did not produce the stage prerequisite", current, err)
	}
	selected, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || selected.Workflow == nil {
		t.Fatal(selected, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, a.TaskID, *selected.Workflow)
	if err != nil || progress.decision.Role != "review" || progress.subject.Binding == nil {
		t.Fatal("The actual authored result did not require its independent next stage", progress.decision, err)
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, a.StepID)
	if err != nil || facts.Graph == nil {
		t.Fatal("The next workflow stage omitted exact graph authority", facts, err)
	}
	g := facts.Graph
	if g.NodeAdmissionID != f.dispatch.Dispatch.NodeAdmissionID || g.Control.OperationAttemptID != a.ID || g.InputBinding.ID != f.inputBinding.ID || g.InputBinding.InputSHA256 != f.inputBinding.InputSHA256 ||
		g.SourceBasis.BasisKind != "node_subject" || g.SourceBasis.CandidateBinding == nil || *g.SourceBasis.CandidateBinding != *progress.subject.Binding || g.SourceBasis.ResponseBinding != nil ||
		g.SourceBasis.ExpectedManifest.ArtifactID != progress.subject.Binding.ManifestArtifactID || g.SourceBasis.ExpectedIndex.ArtifactID != progress.subject.Binding.IndexArtifactID ||
		g.SourceBasis.WorkflowID != a.WorkflowID || g.SourceBasis.StepID != "" {
		t.Fatal("The next stage reset its input or invented a new admission identity", g)
	}
	anchors := map[string]bool{}
	for _, proof := range g.VerifiedArtifacts {
		anchors[proof.ArtifactID] = true
	}
	for _, id := range []string{f.inputBinding.Manifest.ArtifactID, f.inputBinding.Index.ArtifactID, f.inputBinding.StagedBlobs.ArtifactID, f.inputBinding.ContextPacket.ArtifactID,
		progress.subject.Source.ManifestArtifactID, progress.subject.Source.IndexArtifactID, progress.subject.RootSource.ManifestArtifactID, progress.subject.RootSource.IndexArtifactID,
		progress.subject.Binding.ManifestArtifactID, progress.subject.Binding.IndexArtifactID} {
		if !anchors[id] {
			t.Fatal("The next stage omitted a required exact source anchor", id)
		}
	}
	after, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || after.Budget.ConsumedAdmissions != f.budget.Budget.ConsumedAdmissions {
		t.Fatal("Reading actual stage facts consumed an admission", after, err)
	}
	beforeArtifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	stopped := e.MutateGraph(ctx, "graphs.stop", view.Graph.ID, []byte(`{"requestId":"`+domain.NewID()+`","expectedGraphControlRevision":"`+view.Graph.ControlRevision+`"}`))
	if stopped.Envelope.Error != nil {
		t.Fatal(stopped)
	}
	if g, err := e.graphWorkflowStageFacts(ctx, progress, a.StepID); err == nil || g != nil {
		t.Fatal("Accepted graph Stop still produced authority for a later role", g, err)
	}
	retained, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil || len(retained.Items) != len(beforeArtifacts.Items) {
		t.Fatal("Stopped stage inspection created artifact effects", retained, err)
	}
}
