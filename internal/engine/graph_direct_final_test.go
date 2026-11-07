package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

const graphDirectFinalOriginal = `Replace "original" with "updated" in source.txt only`

type graphDirectFinalCase struct {
	engine   *Engine
	graph    domain.GraphDetail
	subject  workflowSubject
	selected domain.GraphDispatchView
	verified *graphFinalVerification
	repo     string
	index    []byte
}

// This group has actual root capture, accepted predecessor, physical files-edge
// composition and two owned native authors. Only the fixed predecessor worker
// edits source.txt; the declared file-final is the unchanged graph-response worker.
func graphDirectFinalFixture(t *testing.T, finalOriginal, finalConditions string, complete bool) graphDirectFinalCase {
	t.Helper()
	e, projectID, repo := testEngine(t)
	ctx := context.Background()
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	nodes := []domain.GraphNodeInput{
		{Key: "edit", OriginalText: graphDirectFinalOriginal, FakeScenario: "graph-document", Pins: &domain.ExecutionPins{HarnessID: "codex"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate"}},
		{Key: "final", OriginalText: finalOriginal, FakeScenario: "graph-response", Pins: &domain.ExecutionPins{HarnessID: "claude-code"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: finalConditions}},
	}
	edges := []domain.GraphDependencyInput{{From: "edit", To: "final", Kind: "files"}}
	windowSeconds := 900
	created := e.createGraph(ctx, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: nodes, Dependencies: &edges, FinalNodeKey: "final",
		Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "2", WindowSeconds: &windowSeconds}}})
	if created.Status != 201 || created.Envelope.Error != nil {
		t.Fatal("The actual fixed document graph was not admitted", created)
	}
	admission := created.Envelope.Data.(domain.GraphAdmission)
	deadline := time.Now().Add(30 * time.Second)
	var selected domain.GraphDispatchView
	for {
		page, err := e.db.GraphDispatches(ctx, admission.Graph.ID, "", "final", 100, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, dispatch := range page.Items {
			if dispatch.Dispatch.Role != "author" || dispatch.OutputObservation == nil || dispatch.OutputObservation.Candidate == nil {
				continue
			}
			attempt, err := e.db.Attempt(ctx, dispatch.Dispatch.AttemptID)
			if err != nil {
				t.Fatal(err)
			}
			if attempt.Phase == "settled" && attempt.Outcome != nil && *attempt.Outcome == "succeeded" {
				selected = dispatch
			}
		}
		if selected.Dispatch.ID != "" {
			workflow, err := e.db.Workflow(ctx, selected.Dispatch.TaskID, domain.WorkflowSelector{WorkflowID: selected.Dispatch.WorkflowID})
			if err != nil {
				t.Fatal(err)
			}
			if !complete || workflow.Workflow != nil && workflow.Workflow.State == "completed" {
				break
			}
		}
		gate, err := e.db.Gate(ctx)
		if err != nil || gate.State != "open" || time.Now().After(deadline) {
			t.Fatalf("The exact document graph did not reach its actual final boundary: %s", graphActorDiagnostics(e, admission.Graph.ID))
		}
		time.Sleep(15 * time.Millisecond)
	}
	// Join the actual completion callbacks, rather than cancelling the owner at
	// SQL terminal visibility before its continuation acknowledgement settles.
	joined := make(chan struct{})
	go func() { e.workers.Wait(); close(joined) }()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-joined:
	case <-timer.C:
		t.Fatalf("The document graph completion acknowledgement did not settle within its original bound: %s", graphActorDiagnostics(e, admission.Graph.ID))
	}
	e.control.Lock()
	graph, graphErr := e.db.Graph(ctx, admission.Graph.ID)
	gate, gateErr := e.db.Gate(ctx)
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	owned, fenced := len(e.activeWorkers), e.fenced
	e.control.Unlock()
	if graphErr != nil || gateErr != nil || pendingErr != nil || gate.State != "open" || fenced || owned != 0 || len(pending) != 0 ||
		graph.Budget.Budget.ConsumedAdmissions != "2" || graph.Budget.Budget.RemainingAdmissions != "0" || len(graph.ActiveClaims) != 0 {
		t.Fatalf("The document graph has no acknowledged two-admission idle boundary: graph=%+v gate=%+v owned=%d fenced=%t pending=%+v errors=%v/%v/%v", graph, gate, owned, fenced, pending, graphErr, gateErr, pendingErr)
	}
	stopFixtureScheduler(t, e)
	workflow, err := e.db.Workflow(ctx, selected.Dispatch.TaskID, domain.WorkflowSelector{WorkflowID: selected.Dispatch.WorkflowID})
	if err != nil || workflow.Workflow == nil {
		t.Fatal(workflow, err)
	}
	subject, err := e.selectedWorkflowSubject(ctx, *workflow.Workflow)
	if err != nil || subject.Binding == nil || subject.Candidate == nil || subject.Candidate.HasDelta || subject.Candidate.HasIndexDelta ||
		subject.Workflow.HasTaskDelta || subject.Workflow.HasTaskIndexDelta {
		t.Fatal("The unchanged final acquired predecessor authorship or lost its actual own candidate", subject, err)
	}
	verified, err := e.loadGraphFinalVerification(ctx, subject)
	if err != nil || verified == nil || !verified.Binding.HasCumulativeWorkingDelta || verified.Binding.HasCumulativeIndexDelta || len(verified.Changes) != 1 {
		t.Fatal("The actual graph-root to unchanged-final document basis was unavailable", verified, err)
	}
	return graphDirectFinalCase{engine: e, graph: graph, subject: subject, selected: selected, verified: verified, repo: repo, index: index}
}

func graphDirectFinalDocumentBytes(t *testing.T, e *Engine, manifest workspace.Manifest) []byte {
	t.Helper()
	for _, entry := range manifest.Entries {
		if entry.Path != "source.txt" {
			continue
		}
		var body bytes.Buffer
		meta, err := e.evidence.CopyTo(context.Background(), entry.ArtifactID, &body, 1024)
		if err != nil || meta.SHA256 == nil || *meta.SHA256 != entry.SHA256 || meta.ByteLength != domain.Decimal(entry.Size) {
			t.Fatal("The actual retained document payload could not be freshly read", meta, err)
		}
		return body.Bytes()
	}
	t.Fatal("The actual retained manifest omitted source.txt")
	return nil
}

func graphDirectFinalUserUnchanged(t *testing.T, f graphDirectFinalCase) {
	t.Helper()
	working, err := os.ReadFile(filepath.Join(f.repo, "source.txt"))
	if err != nil || string(working) != "original\n" {
		t.Fatal("Review-only graph work changed the user's original document", string(working), err)
	}
	index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || !bytes.Equal(index, f.index) {
		t.Fatal("Review-only graph work changed the exact user index", err)
	}
}

func TestGraphDirectFinalActualDocumentUsesFreshCumulativeBuiltinWithZeroOwnDelta(t *testing.T) {
	f := graphDirectFinalFixture(t, graphDirectFinalOriginal, "", true)
	e, ctx := f.engine, context.Background()
	if string(graphDirectFinalDocumentBytes(t, e, f.verified.RootProof.Manifest)) != "original\n" || string(graphDirectFinalDocumentBytes(t, e, f.verified.OutputProof.Manifest)) != "updated\n" {
		t.Fatal("The direct proof did not use actual graph-root before and cumulative final after bytes")
	}
	version, err := e.db.TaskVersion(ctx, f.subject.Workflow.TaskID, f.subject.Workflow.TaskVersion)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := e.graphFinalPlanFacts(ctx, f.verified, version.OriginalText, version.Version)
	if err != nil || facts == nil || !facts.CumulativeDirectProven || facts.CurrentSubject != *f.subject.Binding {
		t.Fatal("The real recognized cumulative document result did not reproduce its cheap proof", facts, err)
	}
	plan := workflowpolicy.Plan(workflowpolicy.PlanInput{Authority: f.subject.Workflow.Authority, HasSubject: true, HasDelta: false, HasIndexDelta: false, GraphFinal: facts})
	if plan.Action != "check" || plan.ExecutorKind != "local_check" {
		t.Fatal("A recognized cumulative final invented review work or skipped its fresh builtin", plan)
	}
	if f.graph.Graph.State != "completed" || f.graph.Graph.Counts.Completed != "2" || f.graph.Graph.Counts.Required != "2" {
		t.Fatal("The actual two-node review-only direct group did not complete", f.graph)
	}
	var ancestorReceipt *domain.CheckReceipt
	for _, node := range f.graph.Nodes {
		workflow, err := e.db.Workflow(ctx, node.TaskID, domain.WorkflowSelector{})
		if err != nil || workflow.Workflow == nil || workflow.Acceptance == nil || workflow.Acceptance.DeliveryResult != "review_only" || workflow.Acceptance.DeliveryOperationID != nil || node.AcceptedResult == nil {
			t.Fatal("A direct node omitted real acceptance or invented local delivery", node, workflow, err)
		}
		steps, err := e.db.WorkflowSteps(ctx, node.TaskID, domain.WorkflowSelector{WorkflowID: workflow.Workflow.ID}, 100, "")
		if err != nil || len(steps.Items) != 2 || steps.Items[0].Role != "author" || steps.Items[1].Role != "check" || steps.Items[1].ExecutorKind != "local_check" {
			t.Fatal("Cheap document qualification added native review work or reused a predecessor receipt", steps, err)
		}
		checks, err := e.db.WorkflowChecks(ctx, node.TaskID, domain.WorkflowSelector{WorkflowID: workflow.Workflow.ID}, 100, "")
		if err != nil || len(checks.Items) != 1 || checks.Items[0].Outcome != "passed" || !checks.Items[0].BuiltinCompleted || checks.Items[0].ExitCode != nil || checks.Items[0].RootTerminal ||
			checks.Items[0].InputCandidateBinding == nil || checks.Items[0].AttemptID == "" || steps.Items[1].AttemptID == nil || checks.Items[0].AttemptID != *steps.Items[1].AttemptID {
			t.Fatal("Fresh builtin receipt is missing or fabricates a command process", checks, err)
		}
		if node.Key == "edit" {
			copy := checks.Items[0]
			ancestorReceipt = &copy
			continue
		}
		check := checks.Items[0]
		if *check.InputCandidateBinding != *f.subject.Binding || check.WorkflowID != f.subject.Workflow.ID {
			t.Fatal("The final reused the predecessor's checked candidate", check)
		}
		var definition *domain.CheckDefinition
		for i := range f.subject.Workflow.Authority.Checks {
			if f.subject.Workflow.Authority.Checks[i].ID == check.DefinitionID {
				definition = &f.subject.Workflow.Authority.Checks[i]
			}
		}
		if definition == nil || definition.Kind != "builtin" || definition.BuiltinID != "literal_document_replace/1" {
			t.Fatal("The actual final receipt is not its captured builtin definition", check)
		}
		expected, err := f.verified.applicability(definition.Scope, definition.CoveredConditionKeys, definition.DefinitionSHA256)
		if err != nil || !reflect.DeepEqual(check.Applicability, expected) {
			t.Fatal("The final builtin receipt did not bind its exact cumulative final basis", check.Applicability, expected, err)
		}
		if ancestorReceipt == nil || ancestorReceipt.ID == check.ID || ancestorReceipt.AttemptID == check.AttemptID || workflowpolicy.Applicable(ancestorReceipt.Applicability, check.Applicability) {
			t.Fatal("A predecessor receipt was promoted into fresh cumulative evidence", ancestorReceipt, check)
		}
	}
	graphDirectFinalUserUnchanged(t, f)
}

func TestGraphDirectFinalOpaqueOrAdditionalReceiverConstraintRequiresReview(t *testing.T) {
	for _, tc := range []struct{ name, original, conditions string }{
		{"opaque_original", "Inspect the complete document result and decide whether it is suitable.", ""},
		{"receiver_constraint", graphDirectFinalOriginal, "Also satisfy this additional opaque receiver condition without dropping its scope."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := graphDirectFinalFixture(t, tc.original, tc.conditions, false)
			e, ctx := f.engine, context.Background()
			version, err := e.db.TaskVersion(ctx, f.subject.Workflow.TaskID, f.subject.Workflow.TaskVersion)
			if err != nil {
				t.Fatal(err)
			}
			facts, err := e.graphFinalPlanFacts(ctx, f.verified, version.OriginalText, version.Version)
			progress, progressErr := e.inspectWorkflowProgress(ctx, f.subject.Workflow.TaskID, f.subject.Workflow)
			if err != nil || progressErr != nil || facts == nil || facts.CumulativeDirectProven || progress.decision.Role != "review" || progress.final == nil || progress.final.BindingRef != f.verified.BindingRef {
				t.Fatal("Opaque or scoped extra obligations were cheap-qualified by the literal file delta", facts, progress.decision, err, progressErr)
			}
			checks, err := e.db.WorkflowChecks(ctx, f.subject.Workflow.TaskID, domain.WorkflowSelector{WorkflowID: f.subject.Workflow.ID}, 100, "")
			workflow, workflowErr := e.db.Workflow(ctx, f.subject.Workflow.TaskID, domain.WorkflowSelector{WorkflowID: f.subject.Workflow.ID})
			steps, stepsErr := e.db.WorkflowSteps(ctx, f.subject.Workflow.TaskID, domain.WorkflowSelector{WorkflowID: f.subject.Workflow.ID}, 100, "")
			if err != nil || workflowErr != nil || stepsErr != nil || len(checks.Items) != 0 || workflow.Acceptance != nil || len(steps.Items) != 1 || steps.Items[0].Role != "author" {
				t.Fatal("The exhausted two-unit group invented a check, review launch or final acceptance", checks, workflow, steps, err, workflowErr, stepsErr)
			}
			for _, node := range f.graph.Nodes {
				if node.Key == "final" && node.AcceptedResult != nil {
					t.Fatal("The extra receiver condition was bypassed by aggregate projection", node)
				}
			}
			graphDirectFinalUserUnchanged(t, f)
		})
	}
}
