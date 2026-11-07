package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/workspace"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphStepOutputStandaloneCreatesNoGraphClosure(t *testing.T) {
	e, admission := watchFixture(t)
	ctx := context.Background()
	beforeArtifacts, err := e.db.Artifacts(ctx, admission.Attempt.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	beforeBudget, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := e.captureGraphStepOutput(ctx, graphStepOutputRequest{Attempt: admission.Attempt})
	if err != nil || facts != nil {
		t.Fatal("Standalone output acquired graph authority or closure", facts, err)
	}
	afterArtifacts, err := e.db.Artifacts(ctx, admission.Attempt.ID, 100, "")
	if err != nil || !reflect.DeepEqual(beforeArtifacts, afterArtifacts) {
		t.Fatal("Standalone graph-output helper created artifacts", err)
	}
	afterBudget, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil || !reflect.DeepEqual(beforeBudget, afterBudget) {
		t.Fatal("Standalone graph-output helper changed budget", err)
	}
}

func TestGraphStepOutputRegisteredRootOperationCannotMasqueradeAsNodeProducer(t *testing.T) {
	e, projectID, _ := graphNativeContextFixture(t)
	ctx := context.Background()
	creation, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake",
		Nodes: []domain.GraphNodeInput{{Key: "node", OriginalText: "Registered root operation is not a node result."}}}, creation)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission := created.Envelope.Data.(domain.GraphAdmission)
	attempt, err := e.db.ClaimAttempt(ctx, admission.PreparationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	// This is a real registered local_graph producer with a legitimate root task,
	// not an invented node dispatch or qualification of native node admission.
	task, err := e.db.Task(ctx, attempt.TaskID)
	if err != nil || task.Task.GraphRef == nil || attempt.ExecutorKind != "local_graph" {
		t.Fatal("Registered graph-operation precondition", task, attempt, err)
	}
	before, err := e.db.Artifacts(ctx, attempt.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	facts, err := e.captureGraphStepOutput(ctx, graphStepOutputRequest{Attempt: attempt})
	if err == nil {
		t.Fatal("A real root-operation producer without node dispatch fell through as a standalone output", facts)
	}
	if facts != nil && (facts.Observation.Candidate != nil || facts.Observation.Response != nil || len(facts.VerifiedArtifacts) != 0) {
		t.Fatal("Ineligible producer created qualifying graph output facts", facts)
	}
	after, err := e.db.Artifacts(ctx, attempt.ID, 100, "")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Ineligible registered producer created output artifacts", err)
	}
}

type graphOutputProducerCase struct {
	engine        *Engine
	attempt       domain.Attempt
	dispatch      domain.GraphDispatchView
	materialized  workspace.GraphMaterialization
	budget        domain.GraphBudgetView
	repo          string
	originalIndex []byte
	rootBinding   domain.GraphRootBinding
	rootSource    *workspace.Snapshot
	inputBinding  domain.GraphInputBinding
}

// This fixture registers a real claimed graph node before materialization.
// It invokes no native process and proves neither native success, model
// inspection, task acceptance nor a provider/account observation.
func graphOutputAdmittedAttemptFixture(t *testing.T, originals ...string) graphOutputProducerCase {
	t.Helper()
	return graphOutputAdmittedAttemptForScenarioFixture(t, "graph-branch-a", originals...)
}

// The fixed scenario is captured by actual graph creation, never retargeted on
// a returned attempt or substituted after its routing/admission receipt.
func graphOutputAdmittedAttemptForScenarioFixture(t *testing.T, scenario string, originals ...string) graphOutputProducerCase {
	t.Helper()
	original := "Retain an exact response without modifying the captured files."
	if len(originals) > 1 {
		t.Fatal("Select at most one complete controlled original")
	}
	if len(originals) == 1 {
		original = originals[0]
	}
	return graphOutputAdmittedNodeFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: original, FakeScenario: scenario, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}})
}

func graphOutputAdmittedNodeFixture(t *testing.T, declaration domain.GraphNodeInput, siblings ...domain.GraphNodeInput) graphOutputProducerCase {
	t.Helper()
	return graphOutputConfiguredNodeFixture(t, declaration, nil, siblings...)
}

func graphOutputConfiguredNodeFixture(t *testing.T, declaration domain.GraphNodeInput, configure func(*domain.GraphStartInput), siblings ...domain.GraphNodeInput) graphOutputProducerCase {
	t.Helper()
	ctx := context.Background()
	e, projectID, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("\ufeffcontrolled staged source 東京\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, repo, "add", "source.txt")
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("controlled working source🙂\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	originalIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	creation, err := e.captureGraphCreateFacts(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	declarations := append([]domain.GraphNodeInput{declaration}, siblings...)
	input := domain.GraphStartInput{RequestID: domain.NewID(), ProjectID: projectID, ExecutionMode: "fake", Nodes: declarations}
	if configure != nil {
		configure(&input)
	}
	created := e.db.CreateGraph(ctx, e.config.OwnerInstanceID, input, creation)
	if created.Envelope.Error != nil {
		t.Fatal(created)
	}
	admission := created.Envelope.Data.(domain.GraphAdmission)
	rootAttempt, err := e.db.ClaimAttempt(ctx, admission.PreparationAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, rootAttempt)
	root, err := e.db.GraphInputRecord(ctx, admission.Graph.ID, *admission.Graph.RootInputID)
	if err != nil || root.PrivateRoot == nil || root.Public.Summary.State != "ready" {
		t.Fatal("Actual graph root setup failed", root, err)
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, admission.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, *root.PrivateRoot, definition.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.db.Graph(ctx, admission.Graph.ID)
	if err != nil || view.DispatchAuthority == nil || len(view.Nodes) < 1 {
		t.Fatal(view, err)
	}
	var node domain.GraphNodeSummary
	for _, current := range view.Nodes {
		if current.Key == declaration.Key {
			node = current
			break
		}
	}
	if node.ID == "" {
		t.Fatal("The first declared producer is absent from the actual graph", view)
	}
	proofs, err := e.verifyGraphRootClosure(ctx, *root.PrivateRoot, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, taskID, version, revision := node.ID, node.TaskID, node.TaskVersion, node.ControlRevision
	inputFacts := domain.GraphInputPreparationFacts{
		Trigger: domain.GraphFreshTrigger{Kind: "completion", ID: rootAttempt.ID, OwnerGeneration: view.DispatchAuthority.Generation},
		Control: domain.GraphDispatchBinding{GraphID: admission.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: admission.Graph.RootTaskID, OperationAttemptID: rootAttempt.ID,
			OwnerInstanceID: e.config.OwnerInstanceID, Generation: view.DispatchAuthority.Generation, GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch,
			WindowNumber: view.Budget.Budget.CurrentWindow.Number, NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision, ContributorVector: []domain.GraphContributorBinding{}},
		RootBinding: *root.PrivateRoot, Contributions: []domain.AcceptedContribution{}, ContributorVector: []domain.GraphContributorBinding{}, VerifiedArtifacts: proofs}
	operation, err := e.db.BeginGraphInput(ctx, e.config.OwnerInstanceID, admission.Graph.ID, node.ID, inputFacts)
	if err != nil {
		t.Fatal("Actual receiving input registration failed", err)
	}
	inputAttempt, err := e.db.ClaimAttempt(ctx, operation.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, inputAttempt)
	view, err = e.db.Graph(ctx, admission.Graph.ID)
	if err != nil || len(view.Nodes) < 1 {
		t.Fatal("Actual receiving input did not become ready", view, err)
	}
	for _, current := range view.Nodes {
		if current.ID == node.ID {
			node = current
			break
		}
	}
	if node.InputID == nil {
		t.Fatal("The first declared producer's actual receiving input did not become ready", view)
	}
	admitFacts, err := e.graphInitialAdmissionFacts(ctx, definition, node, operation.Control)
	if err != nil {
		t.Fatal("Actual sealed node-admission facts failed", err)
	}
	dispatch, err := e.db.AdmitGraphNode(ctx, e.config.OwnerInstanceID, admission.Graph.ID,
		domain.GraphFreshTrigger{Kind: "completion", ID: inputAttempt.ID, OwnerGeneration: operation.Control.Generation}, admitFacts)
	if err != nil {
		t.Fatal("Actual node admission setup failed", err)
	}
	attempt, err := e.db.ClaimAttempt(ctx, dispatch.Dispatch.AttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		current, readErr := e.db.Attempt(context.Background(), attempt.ID)
		if readErr != nil {
			t.Error(readErr)
			return
		}
		if current.Phase == "settled" {
			return
		}
		_, settleErr := e.db.SettleWorkflowStep(context.Background(), attempt.ID, domain.WorkflowStepFacts{StepID: attempt.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
			Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"},
				Failure: primary("fixture_no_process", "The controlled physical-output fixture created no native process and does not assert native success.")}})
		if settleErr != nil {
			t.Error("Could not settle the exact controlled unlaunched claim", settleErr)
		}
	})
	budget, err := e.db.GraphBudget(ctx, admission.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	return graphOutputProducerCase{engine: e, attempt: attempt, dispatch: dispatch, budget: budget, repo: repo, originalIndex: originalIndex,
		rootBinding: *root.PrivateRoot, rootSource: rootSource, inputBinding: admitFacts.InputBinding}
}

func graphOutputAdmittedProducerFixture(t *testing.T, originals ...string) graphOutputProducerCase {
	t.Helper()
	f := graphOutputAdmittedAttemptFixture(t, originals...)
	ctx := context.Background()
	e, attempt, dispatch := f.engine, f.attempt, f.dispatch
	materialized, err := e.workspace.MaterializeGraphInput(ctx, workspace.GraphMaterializeRequest{GraphID: dispatch.Dispatch.GraphID, ReceivingAttemptID: attempt.ID, Role: "author",
		RootBinding: f.rootBinding, RootSource: f.rootSource, InputBinding: f.inputBinding, SourceBasis: dispatch.Dispatch.SourceBasis})
	if err != nil || materialized.Snapshot == nil || materialized.Receipt.ObservedManifest == nil {
		t.Fatal("Actual receiving materialization setup failed", err)
	}
	materializationProof, err := e.workspace.RecheckGraphMaterialization(ctx, materialized.Receipt, materialized.Snapshot, materialized.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	anchors := append(append([]domain.GraphArtifactRef{}, materializationProof.Artifacts...), materialized.ReceiptArtifact)
	proofs, err := e.verifyGraphClosureAnchors(ctx, anchors, anchors)
	if err != nil {
		t.Fatal(err)
	}
	source := materialized.Snapshot
	retainedSource := &domain.SourceSnapshot{ID: source.ID, AttemptID: attempt.ID, ManifestArtifactID: source.ManifestArtifactID, IndexArtifactID: source.IndexArtifactID,
		ManifestSHA256: materialized.Receipt.ObservedManifest.SHA256, BaseCommit: source.BaseCommit, Branch: f.rootBinding.Branch, ProjectIdentity: f.rootBinding.ProjectIdentity,
		SelectedRelativePath: source.SelectedSubfolder, CreatedAt: domain.Now()}
	if err = e.db.RecordGraphMaterialization(ctx, e.config.OwnerInstanceID, attempt.ID, domain.GraphMaterializationFacts{Receipt: materialized.Receipt, ReceiptArtifact: materialized.ReceiptArtifact, SourceSnapshot: retainedSource,
		WorkspaceIdentity: materialized.WorkspaceIdentity, GitControlSHA256: source.GitControlSHA256, VerifiedArtifacts: proofs}); err != nil {
		t.Fatal("Actual materialization registrar failed", err)
	}
	f.materialized = materialized
	return f
}

func TestGraphStepOutputCapturesActualAdmittedCandidateAndEligibleResponse(t *testing.T) {
	f := graphOutputAdmittedProducerFixture(t)
	e, a, snapshot := f.engine, f.attempt, f.materialized.Snapshot
	ctx := context.Background()
	candidate, err := e.workspace.Candidate(ctx, snapshot)
	if err != nil || candidate == nil || len(candidate.Changes) != 0 {
		t.Fatal("Actual no-delta producer capture failed", err)
	}
	index, err := e.workspace.RetainIndex(ctx, a.ID, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(e.lock.Root(), snapshot.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.db.PutCandidate(ctx, domain.Candidate{ID: candidate.ID, AttemptID: a.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: candidate.ManifestArtifactID,
		DeltaArtifactID: candidate.DeltaArtifactID, IndexArtifactID: index.ID, WorkspaceRef: filepath.ToSlash(relative), HasDelta: false, HasIndexDelta: false, Completeness: "complete",
		Verification: "pending", Integration: "not_performed", CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	subject, err := e.candidateWorkflowSubject(ctx, *view.Workflow, candidate.ID)
	if err != nil || subject.Binding == nil {
		t.Fatal(err)
	}
	response, err := e.evidence.Put(ctx, a.ID, "final_response", []byte("\ufeffControlled retained response 東京🙂\r\n"))
	if err != nil || response.SHA256 == nil || index.SHA256 == nil {
		t.Fatal(response, err)
	}
	if _, err = e.db.RecordAttempt(ctx, a.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &response.ID}); err != nil {
		t.Fatal(err)
	}
	responseBinding := domain.ResponseBinding{ProducerAttemptID: a.ID, ResponseArtifactID: response.ID, ResponseSHA256: *response.SHA256, ResponseByteLength: response.ByteLength,
		SourceSnapshotID: snapshot.ID, ManifestSHA256: subject.Source.ManifestSHA256, IndexArtifactID: index.ID, IndexSHA256: *index.SHA256, ConditionsSHA256: view.Workflow.Authority.ConditionsSHA256}
	facts, err := e.captureGraphStepOutput(ctx, graphStepOutputRequest{Attempt: a, Snapshot: snapshot, CandidateBinding: subject.Binding, EligibleResponseBinding: &responseBinding})
	if err != nil || facts == nil {
		t.Fatal("Actual registered output capture did not retain its dual closure", facts, err)
	}
	observation := facts.Observation
	if observation.ProducerAttemptID != a.ID || observation.Candidate == nil || observation.Response == nil || observation.Candidate.State != "complete" || observation.Response.State != "complete" ||
		observation.Candidate.ClosureRef == nil || observation.Response.ClosureRef == nil || observation.Candidate.CandidateID != candidate.ID || observation.Response.ResponseArtifactID != response.ID ||
		len(facts.VerifiedArtifacts) == 0 || len(facts.VerifiedArtifacts) > 256 {
		t.Fatal("Actual dual output lost exact complete producer-bound closure facts", facts)
	}
	for kind, ref := range map[string]domain.GraphArtifactRef{"candidate": *observation.Candidate.ClosureRef, "response": *observation.Response.ClosureRef} {
		body, readErr := e.readGraphArtifact(ctx, ref, "graph_output_closure", 64<<20)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var closure domain.GraphOutputClosure
		if err = json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)); err != nil || closure.Kind != kind || closure.ProducerAttemptID != a.ID || closure.NodeID != f.dispatch.Dispatch.NodeID || closure.ReceivedInputID != f.dispatch.Dispatch.InputID {
			t.Fatal("Closed graph output is not the exact admitted node result", closure, err)
		}
		if kind == "response" && closure.Index.ArtifactID != responseBinding.IndexArtifactID {
			t.Fatal("Response output rewrote its acknowledged distinct after-index artifact")
		}
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || dispatch.Public.OutputObservation != nil {
		t.Fatal("Capture helper mutated terminal dispatch association", dispatch, err)
	}
	afterBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || !reflect.DeepEqual(f.budget, afterBudget) {
		t.Fatal("Physical closure capture spent additional native admissions", err)
	}
	actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("Physical output capture changed original user staging", err)
	}
}
