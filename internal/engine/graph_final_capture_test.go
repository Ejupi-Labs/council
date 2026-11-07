package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphFinalActualNoOwnEditRetainsSeparateCumulativeVerificationBasis(t *testing.T) {
	e, projectID, _ := testEngine(t)
	ctx := context.Background()
	nodes := []domain.GraphNodeInput{}
	for i, key := range []string{"a", "b", "final"} {
		scenario := []string{"graph-branch-a", "graph-branch-b", "graph-join-no-edit"}[i]
		harness := "codex"
		if key == "b" {
			harness = "claude-code"
		}
		nodes = append(nodes, domain.GraphNodeInput{Key: key, OriginalText: "Inspect the complete controlled cumulative file result for node " + key + "; preserve source bytes and staging.",
			FakeScenario: scenario, Pins: &domain.ExecutionPins{HarnessID: harness}, Workflow: &domain.WorkflowInput{Review: "required"},
			Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete original graph root to final file result, retaining all source conditions."}})
	}
	edges := []domain.GraphDependencyInput{{From: "a", To: "final", Kind: "files"}, {From: "b", To: "final", Kind: "files"}}
	created := e.createGraph(ctx, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: nodes, Dependencies: &edges, FinalNodeKey: "final",
		Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "6"}}})
	if created.Status != 201 || created.Envelope.Error != nil {
		t.Fatal(created)
	}
	group := created.Envelope.Data.(domain.GraphAdmission)
	deadline := time.Now().Add(25 * time.Second)
	var selected domain.GraphDispatchView
	for {
		page, err := e.db.GraphDispatches(ctx, group.Graph.ID, "", "final", 100, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, dispatch := range page.Items {
			if dispatch.Dispatch.Role == "author" && dispatch.OutputObservation != nil && dispatch.OutputObservation.Candidate != nil {
				producer, err := e.db.Attempt(ctx, dispatch.Dispatch.AttemptID)
				if err != nil {
					t.Fatal(err)
				}
				if producer.Phase == "settled" && producer.Outcome != nil && *producer.Outcome == "succeeded" {
					selected = dispatch
				}
			}
		}
		if selected.Dispatch.ID != "" {
			break
		}
		gate, err := e.db.Gate(ctx)
		view, graphErr := e.db.Graph(ctx, group.Graph.ID)
		if err != nil || graphErr != nil || gate.State != "open" || view.Graph.State == "failed" || view.Graph.State == "interrupted" || time.Now().After(deadline) {
			t.Fatalf("The actual no-own-edit final producer did not settle: %s", graphActorDiagnostics(e, group.Graph.ID))
		}
		time.Sleep(15 * time.Millisecond)
	}
	observation := selected.OutputObservation.Candidate
	if observation.State != "complete" || observation.ClosureRef == nil || observation.FinalVerificationRef == nil {
		t.Fatal("The actual complete final candidate lacks its separate same-terminal cumulative verification basis", observation)
	}
	producer, err := e.db.Attempt(ctx, selected.Dispatch.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := e.db.Candidate(ctx, observation.CandidateID)
	if err != nil || candidate.AttemptID != producer.ID || candidate.HasDelta || candidate.HasIndexDelta || candidate.Completeness != "complete" {
		t.Fatal("Final eligibility manufactured node authorship or lost the actual empty-delta candidate", candidate, err)
	}
	body, err := e.readGraphArtifact(ctx, *observation.FinalVerificationRef, "graph_final_verification", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateKey := range []string{`"projectIdentity"`, `"repositoryIdentity"`} {
		if bytes.Contains(body, []byte(privateKey)) {
			t.Fatal("A public final verification artifact exposed a private native root identity", privateKey)
		}
	}
	var final domain.GraphFinalVerificationBinding
	if err = json.Unmarshal(body, &final, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	digest, err := domain.GraphFinalVerificationDigest(final)
	if err != nil || digest != final.BindingSHA256 || final.GraphID != group.Graph.ID || final.GraphVersion != selected.Dispatch.GraphVersion || final.FinalNodeID != selected.Dispatch.NodeID || final.FinalTaskID != producer.TaskID || final.FinalTaskVersion != producer.TaskVersion || final.FinalWorkflowID != producer.WorkflowID || final.FinalInputID != selected.Dispatch.InputID || final.OutputClosureRef != *observation.ClosureRef || !reflect.DeepEqual(final.ContributorVector, selected.Dispatch.ContributorVector) || len(final.ContributorVector) != 2 || !final.HasCumulativeWorkingDelta || final.HasCumulativeIndexDelta {
		t.Fatal("The final basis is not the exact cumulative working-only graph result", final, err)
	}
	root, err := e.db.GraphRoot(ctx, group.Graph.ID)
	if err != nil || !reflect.DeepEqual(final.RootBinding, domain.GraphRootPublic(root)) || candidate.RootSourceSnapshotID == root.SourceSnapshotID || final.CumulativeDeltaRef.ProducerAttemptID != producer.ID || len(final.OriginalAccess) == 0 {
		t.Fatal("The final basis relabelled its node source or omitted complete scoped source access", final, candidate, err)
	}
	w, err := e.db.Workflow(ctx, producer.TaskID, domain.WorkflowSelector{WorkflowID: producer.WorkflowID})
	if err != nil || w.Workflow == nil {
		t.Fatal(err)
	}
	subject, err := e.candidateWorkflowSubject(ctx, *w.Workflow, candidate.ID)
	if err != nil || subject.Binding == nil || !reflect.DeepEqual(final.FinalCandidateBinding, *subject.Binding) || final.GraphConditionsSHA256 == "" || final.GraphConditionsRef.ArtifactID == "" {
		t.Fatal("The cumulative basis replaced the actual selected candidate or captured conditions", final, err)
	}
	deltaBody, err := e.readGraphArtifact(ctx, final.CumulativeDeltaRef, "graph_cumulative_delta", 64<<20)
	if err != nil {
		t.Fatal("The complete root-to-selected cumulative delta was not retained", err)
	}
	for _, privateKey := range []string{`"projectIdentity"`, `"repositoryIdentity"`} {
		if bytes.Contains(deltaBody, []byte(privateKey)) {
			t.Fatal("A public cumulative delta artifact exposed a private native root identity", privateKey)
		}
	}
}
