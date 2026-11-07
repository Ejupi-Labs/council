package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// All producers, reviews, accepted predecessors and final output associations
// below come from actual owned offline workers. No accepted tuple is invented.
func graphFinalVerificationFixture(t *testing.T) (*Engine, workflowSubject, domain.GraphDispatchView, string, []byte, []byte) {
	t.Helper()
	return graphFinalVerificationWithWorkflowFixture(t, nil)
}

type graphFinalVerificationOptions struct {
	BeforeGraph                  func(t *testing.T, repo string)
	FinalOriginal                string
	FinalWorkflow                *domain.WorkflowInput
	Delivery                     *domain.WorkflowDeliveryInput
	ContextOnlyARecreatedByFinal bool
}

func graphFinalVerificationWithWorkflowFixture(t *testing.T, finalWorkflow *domain.WorkflowInput) (*Engine, workflowSubject, domain.GraphDispatchView, string, []byte, []byte) {
	t.Helper()
	return graphFinalVerificationWithOptionsFixture(t, graphFinalVerificationOptions{FinalWorkflow: finalWorkflow})
}

func graphFinalVerificationWithOptionsFixture(t *testing.T, options graphFinalVerificationOptions) (*Engine, workflowSubject, domain.GraphDispatchView, string, []byte, []byte) {
	t.Helper()
	// Observe only fixed boundaries already crossed by this fixture. Polls and
	// revisions never renew its original prerequisite deadline.
	timingStarted := time.Now()
	timing := map[string]any{}
	markTiming := func(name string) {
		now := time.Now()
		timing[name] = map[string]any{"at": now.UTC().Format(time.RFC3339Nano), "elapsedNanoseconds": now.Sub(timingStarted).Nanoseconds()}
	}
	markTiming("fixtureEntered")
	defer func() {
		markTiming("fixtureExited")
		timing["failed"] = t.Failed()
		body, err := json.Marshal(timing)
		t.Logf("Actual cumulative fixture boundary timing: %s; marshalErr=%v", body, err)
	}()
	e, projectID, repo := testEngine(t)
	ctx := context.Background()
	if options.BeforeGraph != nil {
		options.BeforeGraph(t, repo)
	}
	staged, working := []byte("Exact staged final-basis source 東京\r\n"), []byte("Exact working final-basis source 🙂\r\n")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), staged, 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, repo, "add", "source.txt")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), working, 0600); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	nodes := []domain.GraphNodeInput{}
	for i, key := range []string{"a", "b", "final"} {
		harness := "codex"
		if key == "b" {
			harness = "claude-code"
		}
		nodes = append(nodes, domain.GraphNodeInput{Key: key, OriginalText: "Inspect the complete controlled cumulative file result for node " + key + "; preserve source bytes and staging.",
			FakeScenario: []string{"graph-branch-a", "graph-branch-b", "graph-join-no-edit"}[i], Pins: &domain.ExecutionPins{HarnessID: harness},
			Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete original graph root to final file result, retaining all source conditions."}})
	}
	if options.FinalOriginal != "" {
		nodes[len(nodes)-1].OriginalText = options.FinalOriginal
	}
	if options.FinalWorkflow != nil {
		captured := *options.FinalWorkflow
		nodes[len(nodes)-1].Workflow = &captured
	}
	edges := []domain.GraphDependencyInput{{From: "a", To: "final", Kind: "files"}, {From: "b", To: "final", Kind: "files"}}
	if options.ContextOnlyARecreatedByFinal {
		// This bounded counterexample recreates A through a real final edit;
		// matching bytes cannot turn its captured context edge into file ancestry.
		edges[0].Kind = "context"
		nodes[len(nodes)-1].FakeScenario = "graph-branch-a"
	}
	created := e.createGraph(ctx, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: nodes, Dependencies: &edges, FinalNodeKey: "final", Delivery: options.Delivery,
		Routing: &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "5"}}})
	markTiming("graphCreationReturned")
	if created.Status != 201 || created.Envelope.Error != nil {
		t.Fatal(created)
	}
	group := created.Envelope.Data.(domain.GraphAdmission)
	deadline := time.Now().Add(25 * time.Second)
	timing["graphId"] = group.Graph.ID
	timing["prerequisiteDeadlineAt"] = deadline.UTC().Format(time.RFC3339Nano)
	markTiming("prerequisiteObserverEntered")
	var selected domain.GraphDispatchView
	for {
		page, err := e.db.GraphDispatches(ctx, group.Graph.ID, "", "final", 100, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, dispatch := range page.Items {
			if dispatch.Dispatch.Role != "author" || dispatch.OutputObservation == nil || dispatch.OutputObservation.Candidate == nil {
				continue
			}
			producer, err := e.db.Attempt(ctx, dispatch.Dispatch.AttemptID)
			if err != nil {
				t.Fatal(err)
			}
			if producer.Phase == "settled" && producer.Outcome != nil && *producer.Outcome == "succeeded" {
				selected = dispatch
			}
		}
		if selected.Dispatch.ID != "" {
			break
		}
		gate, err := e.db.Gate(ctx)
		if err != nil || gate.State != "open" || time.Now().After(deadline) {
			t.Fatalf("Actual cumulative final prerequisite did not settle: %s", graphActorDiagnostics(e, group.Graph.ID))
		}
		time.Sleep(15 * time.Millisecond)
	}
	markTiming("finalAuthorObservedSettled")
	timing["finalAuthorAttemptId"] = selected.Dispatch.AttemptID
	// SQL terminal visibility precedes the scheduler worker's continuation
	// callbacks. Join their actual commit acknowledgements before cancelling the
	// fixture owner, using the same prerequisite deadline rather than a sleep.
	acknowledged := make(chan struct{})
	go func() {
		e.workers.Wait()
		close(acknowledged)
	}()
	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("Final continuation acknowledgement exceeded its original bound: %s", graphActorDiagnostics(e, group.Graph.ID))
	}
	wait := time.NewTimer(remaining)
	defer wait.Stop()
	select {
	case <-acknowledged:
		markTiming("continuationAcknowledgementsJoined")
	case <-wait.C:
		t.Fatalf("Final continuation acknowledgement did not complete: %s", graphActorDiagnostics(e, group.Graph.ID))
	}
	e.control.Lock()
	gate, gateErr := e.db.Gate(ctx)
	pending, pendingErr := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
	budget, budgetErr := e.db.GraphBudget(ctx, group.Graph.ID)
	fenced, owned := e.fenced, len(e.activeWorkers)
	e.control.Unlock()
	if gateErr != nil || pendingErr != nil || budgetErr != nil || gate.State != "open" || fenced || owned != 0 || len(pending) != 0 ||
		budget.Budget.ConsumedAdmissions != "5" || budget.Budget.RemainingAdmissions != "0" {
		t.Fatalf("The fixture has no acknowledged idle budget boundary before shutdown: gate=%+v fenced=%t owned=%d pending=%+v budget=%+v errors=%v/%v/%v graph=%s", gate, fenced, owned, pending, budget, gateErr, pendingErr, budgetErr, graphActorDiagnostics(e, group.Graph.ID))
	}
	// Verification owns no actor or new work. Stop only this fixture's scheduler
	// after the acknowledged producer and budget-wait continuation have joined.
	stopFixtureScheduler(t, e)
	markTiming("schedulerStopped")
	observation := selected.OutputObservation.Candidate
	if observation.State != "complete" || observation.ClosureRef == nil || observation.FinalVerificationRef == nil {
		t.Fatal("Actual final producer has no complete same-terminal basis", observation)
	}
	w, err := e.db.Workflow(ctx, selected.Dispatch.TaskID, domain.WorkflowSelector{WorkflowID: selected.Dispatch.WorkflowID})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	subject, err := e.selectedWorkflowSubject(ctx, *w.Workflow)
	if err != nil || subject.Binding == nil || subject.Candidate == nil || subject.Candidate.ID != observation.CandidateID || subject.Candidate.HasDelta != options.ContextOnlyARecreatedByFinal || subject.Candidate.HasIndexDelta {
		t.Fatal("Actual final candidate lost its declared creation-delta facts or was relabelled", subject, options.ContextOnlyARecreatedByFinal, err)
	}
	markTiming("fixtureReturning")
	return e, subject, selected, repo, working, index
}

func TestGraphFinalVerificationUsesActualCumulativeBasisAndRejectsCorruptRootMember(t *testing.T) {
	e, subject, selected, repo, working, index := graphFinalVerificationFixture(t)
	ctx := context.Background()
	beforeBinding, beforeCandidate := *subject.Binding, *subject.Candidate
	verified, err := e.loadGraphFinalVerification(ctx, subject)
	if err != nil || verified == nil {
		t.Fatal("Actual same-terminal final basis was unavailable for cumulative qualification", err)
	}
	if verified.BindingRef != *selected.OutputObservation.Candidate.FinalVerificationRef || verified.Binding.FinalCandidateBinding != beforeBinding || !reflect.DeepEqual(verified.Binding.RootBinding, domain.GraphRootPublic(verified.Root)) || verified.Root.SourceSnapshotID == beforeBinding.RootSourceSnapshotID || len(verified.Changes) != 2 || !verified.Binding.HasCumulativeWorkingDelta || verified.Binding.HasCumulativeIndexDelta || len(verified.ArtifactRefs) == 0 || len(verified.VerifiedArtifacts) == 0 || len(verified.VerifiedArtifacts) > 256 {
		t.Fatal("Loaded context did not distinguish actual node source from complete graph-root cumulative files", verified)
	}
	detail, err := e.db.TaskVersion(ctx, subject.Workflow.TaskID, subject.Workflow.TaskVersion)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := e.graphFinalPlanFacts(ctx, verified, detail.OriginalText, detail.Version)
	if err != nil || facts == nil || facts.CumulativeDirectProven || facts.CurrentSubject != beforeBinding || !facts.HasCumulativeWorkingDelta || facts.HasCumulativeIndexDelta {
		t.Fatal("Opaque final task manufactured direct proof or lost cumulative scope", facts, err)
	}
	decision := workflowpolicy.Plan(workflowpolicy.PlanInput{Authority: subject.Workflow.Authority, HasSubject: true, HasDelta: false, HasIndexDelta: false, GraphFinal: facts})
	if decision.Role != "review" || decision.Action != "review" || !reflect.DeepEqual(*subject.Binding, beforeBinding) || !reflect.DeepEqual(*subject.Candidate, beforeCandidate) {
		t.Fatal("Cumulative eligibility bypassed fresh review or manufactured node edits", decision)
	}
	progress, err := e.inspectWorkflowProgress(ctx, subject.Workflow.TaskID, subject.Workflow)
	authorStep, stepErr := e.db.WorkflowStep(ctx, selected.Dispatch.StepID)
	expectedNext, invalid := domain.WorkflowStageKey("review", 0, "", beforeBinding.CandidateID)
	if err != nil || stepErr != nil || invalid != nil || progress.decision.Role != "review" || progress.decision.Action != "review" || progress.hasDelta || progress.hasIndexDelta || !reflect.DeepEqual(progress.subject.Binding, subject.Binding) || facts.VerificationBindingRef != verified.BindingRef || authorStep.AdvanceProof == nil || authorStep.AdvanceProof.NextLogicalKey != expectedNext || authorStep.AdvanceProof.CandidateID == nil || *authorStep.AdvanceProof.CandidateID != beforeBinding.CandidateID {
		t.Fatal("Actual final progress/terminal proof did not require fresh cumulative review", progress.decision, authorStep.AdvanceProof, err, stepErr, invalid)
	}
	keys := []string{}
	for _, condition := range subject.Workflow.Authority.Conditions {
		keys = append(keys, condition.Key)
	}
	scope := domain.CheckScope{Kind: "whole_candidate", Paths: []string{}}
	nodeOnly, err := e.workflowApplicability(ctx, subject, scope, keys, workflowHash(adapters.ReviewSchema()))
	if err != nil {
		t.Fatal(err)
	}
	cumulative, err := verified.applicability(scope, keys, workflowHash(adapters.ReviewSchema()))
	if err != nil || workflowpolicy.Applicable(nodeOnly, cumulative) || cumulative.DefinitionSHA256 != nodeOnly.DefinitionSHA256 || cumulative.ScopeKind != nodeOnly.ScopeKind || cumulative.BasisSHA256 == "" {
		t.Fatal("Node-only evidence acquired cumulative applicability", nodeOnly, cumulative, err)
	}
	// The caller's actual selected binding is required in full, not only an ID or
	// a matching manifest hash. This is not a fabricated producer transition.
	wrong := subject
	wrongBinding := beforeBinding
	wrongBinding.SourceSnapshotID = domain.NewID()
	wrong.Binding = &wrongBinding
	if bad, err := e.loadGraphFinalVerification(ctx, wrong); err == nil || bad != nil {
		t.Fatal("A mismatched selected binding obtained a usable cumulative context", bad, err)
	}
	var memberID string
	for _, entry := range verified.RootProof.Manifest.Entries {
		if entry.Path == "source.txt" && entry.Type == "file" {
			memberID = entry.ArtifactID
		}
	}
	if memberID == "" || memberID == verified.Root.Manifest.ArtifactID || memberID == verified.Root.Index.ArtifactID || memberID == verified.Root.StagedBlobs.ArtifactID {
		t.Fatal("Fixture lacks an unchanged non-anchor root payload")
	}
	meta, err := e.db.Artifact(ctx, memberID)
	if err != nil || meta.Kind != "source_file" {
		t.Fatal(meta, err)
	}
	path := filepath.Join(e.lock.Root(), filepath.FromSlash(meta.PrivateRef))
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		t.Fatal(err)
	}
	body[0] ^= 1
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = e.evidence.VerifyClosed(ctx, verified.Root.Manifest.ArtifactID); err != nil {
		t.Fatal("The corruption changed an anchor instead of its retained member", err)
	}
	if bad, err := e.loadGraphFinalVerification(ctx, subject); err == nil || bad != nil || !errors.Is(err, evidence.ErrCorrupt) {
		t.Fatal("A corrupt unchanged root member still supplied cumulative qualification", bad, err)
	}
	actualWorking, err := os.ReadFile(filepath.Join(repo, "source.txt"))
	if err != nil || !bytes.Equal(actualWorking, working) {
		t.Fatal("Verification or private fault changed user working bytes", err)
	}
	actualIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil || !bytes.Equal(actualIndex, index) {
		t.Fatal("Verification changed the exact user index", err)
	}
}
