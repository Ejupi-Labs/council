package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func graphActorDiagnostics(e *Engine, graphID string) string {
	ctx := context.Background()
	view, graphErr := e.db.Graph(ctx, graphID)
	gate, gateErr := e.db.Gate(ctx)
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	text := fmt.Sprintf("graph=%+v nodes=%+v blockers=%+v graphErr=%v gate=%+v gateErr=%v pending=%+v pendingErr=%v", view.Graph, view.Nodes, view.Blockers, graphErr, gate, gateErr, pending, pendingErr)
	for _, node := range view.Nodes {
		workflow, err := e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{})
		steps, stepErr := e.db.WorkflowSteps(ctx, node.TaskID, domain.WorkflowSelector{}, 20, "")
		text += fmt.Sprintf("\nnode=%s workflow=%+v workflowErr=%v steps=%+v stepErr=%v", node.Key, workflow, err, steps, stepErr)
	}
	return text
}

// This runs the public creation path and the actual live actor only. It never
// calls ClaimAttempt, run, or any continuation helper. All workers are the
// repository's explicitly offline fixed author/reviewer processes, and the
// single configured slot makes no claim about native process overlap.
func TestGraphActorAutomaticallyCompletesReviewedFileDependencyWithOneSharedBudget(t *testing.T) {
	e, projectID, repo := testEngine(t)
	ctx := context.Background()
	staged := []byte("\ufeffControlled user staged bytes 東京\r\n")
	working := []byte("\ufeffControlled user working bytes 🙂\r\n")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), staged, 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, repo, "add", "source.txt")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), working, 0600); err != nil {
		t.Fatal(err)
	}
	unrelated := []byte("Unrelated user's untracked edit café\r\n")
	if err := os.WriteFile(filepath.Join(repo, "user-note.txt"), unrelated, 0600); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	edges := []domain.GraphDependencyInput{{From: "a", To: "b", Kind: "files"}}
	created := e.createGraph(ctx, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", MaxParallelism: "1",
		Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "4"}}, Dependencies: &edges,
		Nodes: []domain.GraphNodeInput{
			{Key: "a", OriginalText: "Author the controlled branch A file and independently review its entire candidate.", FakeScenario: "graph-branch-a",
				Pins: &domain.ExecutionPins{HarnessID: "codex"}, Workflow: &domain.WorkflowInput{Review: "required"},
				Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete authored file and preserve all user source and staging."}},
			{Key: "b", OriginalText: "Receive the accepted branch A file, author controlled branch B, and independently review the complete resulting candidate.", FakeScenario: "graph-branch-b",
				Pins: &domain.ExecutionPins{HarnessID: "codex"}, Workflow: &domain.WorkflowInput{Review: "required"},
				Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the inherited file and the new file while preserving the complete original source and staging."}},
		}})
	if created.Status != 201 || created.Envelope.Error != nil {
		t.Fatal("The actual public graph declaration did not register", created)
	}
	admission := created.Envelope.Data.(domain.GraphAdmission)
	watch := newGraphActorWatchdog(t, e, admission.Graph.ID)
	var view domain.GraphDetail
	for {
		var gate domain.ExecutionGate
		view, gate = watch.observe()
		if view.Graph.State == "completed" {
			break
		}
		if gate.State != "open" {
			t.Fatalf("The live graph actor fenced before its declared journey completed: %s", graphActorDiagnostics(e, admission.Graph.ID))
		}
		switch view.Graph.State {
		case "blocked", "failed", "cancelled", "cancelling", "interrupted":
			t.Fatalf("The live graph actor paused or failed instead of completing its declared dependency: %s", graphActorDiagnostics(e, admission.Graph.ID))
		}
		// Polling never renews a fixed declared transition's observation bound.
		time.Sleep(50 * time.Millisecond)
	}
	if len(view.Nodes) != 2 || view.Graph.Counts.Required != "2" || view.Graph.Counts.Completed != "2" || view.Graph.FinalNodeID != nil {
		t.Fatal("The separate reviewable results did not complete every required node", view)
	}
	nodes := map[string]domain.GraphNodeSummary{}
	authors := map[string]domain.Attempt{}
	nativeIDs := map[string]bool{}
	for _, node := range view.Nodes {
		nodes[node.Key] = node
		result := node.AcceptedResult
		if node.State != "completed" || result == nil || result.AcceptanceKind != "workflow" || result.CandidateID == nil || result.ResponseArtifactID != nil || node.InputID == nil || result.InputID != *node.InputID {
			t.Fatal("The actual actor did not retain an exact accepted file producer", node)
		}
		workflow, err := e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{})
		if err != nil || workflow.Workflow == nil || workflow.Workflow.State != "completed" || workflow.Acceptance == nil || workflow.Acceptance.ID != result.AcceptanceID || workflow.Acceptance.CandidateBinding.CandidateID != *result.CandidateID {
			t.Fatal("The node projection is not backed by an actual workflow acceptance", workflow, err)
		}
		steps, err := e.db.WorkflowSteps(ctx, node.TaskID, domain.WorkflowSelector{}, 20, "")
		if err != nil || steps.NextCursor != nil || len(steps.Items) != 2 {
			t.Fatal("The actor did not run exactly one author and one independent review", steps, err)
		}
		var review domain.Attempt
		for _, step := range steps.Items {
			if step.ExecutorKind != "native" || step.Status != "succeeded" || step.AttemptID == nil {
				t.Fatal("An actual native workflow stage is incomplete", step)
			}
			a, err := e.db.Attempt(ctx, *step.AttemptID)
			if err != nil || a.Phase != "settled" || a.Outcome == nil || *a.Outcome != "succeeded" || a.Cleanup.State != "verified" || a.EvidenceStatus != "complete" || a.StartedAt == nil || a.ExecutionMode != "fake" {
				t.Fatal("A fixed native worker was inferred instead of actually owned and settled", a, err)
			}
			if nativeIDs[a.ID] {
				t.Fatal("The actor reused a native process occurrence", a.ID)
			}
			nativeIDs[a.ID] = true
			switch step.Role {
			case "author":
				if a.ID != result.ProducerAttemptID || a.CandidateID == nil || *a.CandidateID != *result.CandidateID {
					t.Fatal("The selected producer is not the actual authored candidate", a, result)
				}
				authors[node.Key] = a
			case "review":
				review = a
				if step.ReviewID == nil {
					t.Fatal("The actual reviewer retained no structured review", step)
				}
				report, err := e.db.WorkflowReview(ctx, *step.ReviewID)
				if err != nil || report.Report.Outcome != "approved" || report.Report.Subject.Kind != "candidate" || report.Report.Subject.ID != *result.CandidateID || len(report.Report.InspectedEvidence) == 0 {
					t.Fatal("The actual fresh reviewer did not inspect the exact selected author", report, err)
				}
			default:
				t.Fatal("The controlled actor journey admitted an unexpected stage", step)
			}
		}
		if authors[node.Key].ID == "" || review.ID == "" || node.LatestAttemptID == nil || *node.LatestAttemptID != review.ID || review.ID == result.ProducerAttemptID {
			t.Fatal("Supporting review output replaced the accepted author", node, review)
		}
		subject, err := e.candidateWorkflowSubject(ctx, *workflow.Workflow, *result.CandidateID)
		if err != nil || subject.Binding == nil {
			t.Fatal("The selected node candidate has no actual full retained closure", err)
		}
		for path, expected := range map[string][]byte{"source.txt": working, "user-note.txt": unrelated, fixture.GraphFileA: []byte(fixture.GraphContentA)} {
			member := closureTestEntry(t, subject.Manifest, path)
			meta, err := e.evidence.VerifyClosed(ctx, member.ArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			body, err := e.readAllResponse(meta)
			if err != nil || !bytes.Equal(body, expected) {
				t.Fatal("The actual selected candidate lost cumulative or unrelated bytes", node.Key, path, err)
			}
		}
		if node.Key == "b" {
			member := closureTestEntry(t, subject.Manifest, fixture.GraphFileB)
			body, err := e.readGraphArtifact(ctx, domain.GraphArtifactRef{ArtifactID: member.ArtifactID, ProducerAttemptID: authors[node.Key].ID, SHA256: member.SHA256, ByteLength: fmt.Sprint(member.Size)}, "source_file", 64<<20)
			if err != nil || string(body) != fixture.GraphContentB {
				t.Fatal("The second actual selected candidate omitted its own authored file", err)
			}
		}
	}
	if len(nativeIDs) != 4 {
		t.Fatal("The controlled journey did not execute four distinct fixed native stages", nativeIDs)
	}
	input, err := e.db.GraphInput(ctx, admission.Graph.ID, *nodes["b"].InputID)
	if err != nil || !input.Summary.Current || input.Summary.State != "ready" || input.InputBinding == nil || input.ContextPacket == nil || len(input.InputBinding.Contributions) != 1 || len(input.InputBinding.ContributorVector) != 1 {
		t.Fatal("The second author did not receive an actual complete accepted predecessor input", input, err)
	}
	contribution := input.InputBinding.Contributions[0]
	if contribution.EdgeKind != "files" || contribution.FromNodeID != nodes["a"].ID || contribution.ProducerAttemptID != authors["a"].ID || contribution.AcceptanceID != nodes["a"].AcceptedResult.AcceptanceID || contribution.CandidateBinding == nil || contribution.CandidateBinding.CandidateID != *nodes["a"].AcceptedResult.CandidateID {
		t.Fatal("The files edge selected prose, a reviewer or another accepted result", contribution)
	}
	verified, err := e.workspace.VerifyGraphInput(ctx, *input.InputBinding)
	if err != nil {
		t.Fatal(err)
	}
	var provenance workspace.GraphInputProvenance
	if err = json.Unmarshal(verified.Provenance, &provenance); err != nil {
		t.Fatal(err)
	}
	foundOrigin := false
	for _, slot := range provenance.Slots {
		if slot.Dimension == "working" && slot.Path == fixture.GraphFileA {
			for _, origin := range slot.Origins {
				foundOrigin = foundOrigin || origin.Node != nil && origin.Node.NodeID == nodes["a"].ID && origin.Node.ProducerAttemptID == authors["a"].ID && origin.Node.AcceptanceID == contribution.AcceptanceID
			}
		}
	}
	if !foundOrigin {
		t.Fatal("The actual receiving file lost its accepted causal origin", provenance)
	}
	foundStaged := false
	for _, entry := range verified.Manifest.Index {
		if entry.Path != "source.txt" {
			continue
		}
		for _, blob := range verified.StagedBlobs.Entries {
			if blob.ObjectID != entry.Object {
				continue
			}
			meta, err := e.evidence.VerifyClosed(ctx, blob.ArtifactRef.ArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			body, err := e.readAllResponse(meta)
			if err != nil || !bytes.Equal(body, staged) || bytes.Equal(body, working) {
				t.Fatal("Actual staged input was replaced by working bytes", err)
			}
			foundStaged = true
		}
	}
	if !foundStaged {
		t.Fatal("The actual receiving input has no retained staged object")
	}
	budget, err := e.db.GraphBudget(ctx, admission.Graph.ID)
	if err != nil || budget.Budget.GrantedAdmissions != "4" || budget.Budget.ConsumedAdmissions != "4" || budget.Budget.CurrentWindow.DeadlineAt == nil || len(budget.NodeConsumption) != 2 {
		t.Fatal("The actor did not use one explicit four-admission common window", budget, err)
	}
	for _, node := range budget.NodeConsumption {
		if node.ConsumedAdmissions != "2" {
			t.Fatal("A node hid or duplicated its author/review consumption", node)
		}
	}
	ledger, err := e.db.GraphBudgetLedger(ctx, admission.Graph.ID, 100, "")
	if err != nil || ledger.NextCursor != nil {
		t.Fatal(ledger, err)
	}
	debits := map[string]bool{}
	for _, entry := range ledger.Items {
		if entry.ConsumedDelta == "0" {
			continue
		}
		if entry.TaskID != admission.Graph.RootTaskID || entry.ConsumedDelta != "1" || entry.AttemptID == nil || !nativeIDs[*entry.AttemptID] || debits[*entry.AttemptID] {
			t.Fatal("Canonical graph ledger did not bind each actual native admission once", entry)
		}
		debits[*entry.AttemptID] = true
	}
	if len(debits) != 4 {
		t.Fatal("Local graph preparation was counted as a model or native debit was omitted", debits)
	}
	afterWorking, workingErr := os.ReadFile(filepath.Join(repo, "source.txt"))
	afterUnrelated, unrelatedErr := os.ReadFile(filepath.Join(repo, "user-note.txt"))
	afterIndex, indexErr := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if workingErr != nil || unrelatedErr != nil || indexErr != nil || !bytes.Equal(afterWorking, working) || !bytes.Equal(afterUnrelated, unrelated) || !bytes.Equal(afterIndex, index) {
		t.Fatal("The review-only graph changed the user's original working or staged state", workingErr, unrelatedErr, indexErr)
	}
	for _, path := range []string{fixture.GraphFileA, fixture.GraphFileB} {
		if _, err := os.Stat(filepath.Join(repo, path)); !os.IsNotExist(err) {
			t.Fatal("A separate reviewable graph result was silently applied to the user project", path, err)
		}
	}
}
