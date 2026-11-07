package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func graphNativeRunFixture(t *testing.T) graphOutputProducerCase {
	t.Helper()
	f := graphOutputAdmittedAttemptFixture(t, strings.Repeat("Complete receiving instruction 東京\r\n", 700))
	if f.attempt.ExecutorKind != "native" || f.attempt.Role != "author" || f.attempt.FakeScenario != "graph-branch-a" || f.attempt.SourceSnapshotID != nil {
		t.Fatal("Actual fresh graph author setup is not an unmaterialized native dispatch", f.attempt)
	}
	return f
}

func graphNativeRunUserState(t *testing.T, f graphOutputProducerCase) ([]byte, []byte) {
	t.Helper()
	working, err := os.ReadFile(filepath.Join(f.repo, "source.txt"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	return working, index
}

func graphNativeRunAssertUserState(t *testing.T, f graphOutputProducerCase, working, index []byte) {
	t.Helper()
	afterWorking, afterIndex := graphNativeRunUserState(t, f)
	if !bytes.Equal(working, afterWorking) || !bytes.Equal(index, afterIndex) {
		t.Fatal("Graph execution changed the user's working bytes or exact raw index")
	}
	if _, err := os.Stat(filepath.Join(f.repo, fixture.GraphFileA)); !os.IsNotExist(err) {
		t.Fatal("The owned fake graph worker wrote into the user's checkout", err)
	}
}

func TestGraphNativeRunOwnedAuthorUsesClosedInputContextAndTerminalOutput(t *testing.T) {
	f := graphNativeRunFixture(t)
	e, a := f.engine, f.attempt
	ctx := context.Background()
	// Change both original working and staged states after graph capture. The
	// receiver must use its sealed input, and preserve this newer user intent.
	if err := os.WriteFile(filepath.Join(f.repo, "source.txt"), []byte("later staged user intent 東京\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, f.repo, "add", "source.txt")
	if err := os.WriteFile(filepath.Join(f.repo, "source.txt"), []byte("later unstaged user intent 🙂\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	working, index := graphNativeRunUserState(t, f)
	prepares := 0
	var spec process.NativeSpec
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		prepares++
		spec = actual
		return process.Prepare(ctx, actual)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || prepares != 1 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" || !current.LaunchIntent || current.StartedAt == nil {
		t.Fatal("Actual owned graph author did not complete its guarded worker", current, prepares, err)
	}
	if current.SourceSnapshotID == nil || current.CandidateID == nil || current.ResponseArtifactID == nil {
		t.Fatal("Actual owned graph author lost source, candidate or response evidence", current)
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || dispatch.Public.Materialization == nil || dispatch.Public.Materialization.State != "complete" || dispatch.MaterializationArtifact == nil || dispatch.Public.OutputObservation == nil || dispatch.Public.OutputObservation.Candidate == nil || dispatch.Public.OutputObservation.Candidate.State != "complete" || dispatch.Public.OutputObservation.Candidate.ClosureRef == nil {
		t.Fatal("Terminal graph worker has no atomic receiving receipt and complete actual candidate closure", dispatch, err)
	}
	if dispatch.Public.OutputObservation.Candidate.CandidateID != *current.CandidateID {
		t.Fatal("Graph output is not the actual owned candidate")
	}
	// A file-producing author response is retained, but is not fabricated into
	// an eligible response-only graph result.
	if dispatch.Public.OutputObservation.Response == nil || dispatch.Public.OutputObservation.Response.ResponseArtifactID != *current.ResponseArtifactID || dispatch.Public.OutputObservation.Response.State == "complete" || dispatch.Public.OutputObservation.Response.ClosureRef != nil {
		t.Fatal("An ineligible raw response was lost or promoted to graph deliverable authority", dispatch.Public.OutputObservation)
	}
	if !strings.Contains(strings.Join(spec.Args, "\n"), "--context-directory") || !bytes.Contains(spec.Stdin, []byte(`"graphContext"`)) {
		t.Fatal("Owned fake graph worker received no actual prepared context transport")
	}
	if actual, err := os.ReadFile(filepath.Join(spec.Cwd, "source.txt")); err != nil || string(actual) != "controlled working source🙂\r\n" {
		t.Fatal("The receiving author recaptured newer live source instead of its sealed input", err)
	}
	if actual, err := os.ReadFile(filepath.Join(spec.Cwd, fixture.GraphFileA)); err != nil || string(actual) != fixture.GraphContentA {
		t.Fatal("The actual owned fixed worker did not author its expected isolated result", err)
	}
	artifacts, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	input, err := e.db.GraphInputRecord(ctx, f.dispatch.Dispatch.GraphID, f.inputBinding.ID)
	if err != nil || input.Public.ContextPacket == nil {
		t.Fatal("Actual closed receiving context is unavailable", err)
	}
	inspected := false
	for _, artifact := range artifacts.Items {
		if artifact.Kind != "stdout" {
			continue
		}
		meta, verifyErr := e.evidence.VerifyClosed(ctx, artifact.ID)
		if verifyErr != nil {
			t.Fatal(verifyErr)
		}
		body, readErr := e.readAllResponse(meta)
		if readErr != nil {
			t.Fatal(readErr)
		}
		inspected = bytes.Contains(body, []byte(`"fixture_context_reads"`)) && bytes.Contains(body, []byte(input.Public.ContextPacket.NodeInstructionsRef.ArtifactID)) && bytes.Contains(body, []byte(input.Public.ContextPacket.NodeInstructionsRef.SHA256))
		if !bytes.Contains(body, []byte(`"readVia":"file"`)) || !bytes.Contains(body, []byte(`"artifactId"`)) {
			t.Fatal("The owned worker did not retain actual source-read tuples")
		}
	}
	if !inspected {
		t.Fatal("No real owned-worker context inspection receipt was retained")
	}
	afterBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || afterBudget.Budget.ConsumedAdmissions != f.budget.Budget.ConsumedAdmissions {
		t.Fatal("One admitted graph author consumed fresh child allowance", afterBudget, err)
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

func TestGraphNativeRunOwnedResponseRetainsActualCandidateAndEligibleResponseClosures(t *testing.T) {
	f := graphOutputAdmittedAttemptForScenarioFixture(t, "graph-response", strings.Repeat("Complete receiving instruction 東京\r\n", 700))
	e, a := f.engine, f.attempt
	ctx := context.Background()
	if a.FakeScenario != "graph-response" || a.SourceSnapshotID != nil {
		t.Fatal("Actual response-only fixture was not admitted prospectively", a)
	}
	working, index := graphNativeRunUserState(t, f)
	prepares := 0
	var cwd string
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		prepares++
		cwd = spec.Cwd
		return process.Prepare(ctx, spec)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || prepares != 1 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" || current.CandidateID == nil || current.ResponseArtifactID == nil || current.SourceSnapshotID == nil {
		t.Fatal("The actual owned response-only worker did not complete with full observed evidence", current, prepares, err)
	}
	candidate, err := e.db.Candidate(ctx, *current.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta || candidate.Completeness != "complete" {
		t.Fatal("Response-only fixture did not preserve the actual receiving files and index", candidate, err)
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil || view.Workflow.SelectedCandidateID != nil || view.Workflow.SelectedResponseBinding == nil || view.Workflow.SelectedResponseBinding.ResponseArtifactID != *current.ResponseArtifactID {
		t.Fatal("Complete candidate history was promoted over the eligible selected response", view, err)
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || record.Public.OutputObservation == nil {
		t.Fatal("The actual response worker has no terminal graph observation", record, err)
	}
	observed := record.Public.OutputObservation
	if observed.Candidate == nil || observed.Candidate.CandidateID != *current.CandidateID || observed.Candidate.State != "complete" || observed.Candidate.ClosureRef == nil || observed.Response == nil || observed.Response.ResponseArtifactID != *current.ResponseArtifactID || observed.Response.State != "complete" || observed.Response.ClosureRef == nil {
		t.Fatal("An eligible response discarded the actual own zero-delta candidate closure history", observed)
	}
	source, err := e.db.SourceSnapshot(ctx, *current.SourceSnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	selected := view.Workflow.SelectedResponseBinding
	if selected.IndexArtifactID == source.IndexArtifactID {
		t.Fatal("Actual response setup did not retain its distinct after-index artifact")
	}
	for kind, ref := range map[string]domain.GraphArtifactRef{"candidate": *observed.Candidate.ClosureRef, "response": *observed.Response.ClosureRef} {
		body, err := e.readGraphArtifact(ctx, ref, "graph_output_closure", 64<<20)
		if err != nil {
			t.Fatal(err)
		}
		var closure domain.GraphOutputClosure
		if err := json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)); err != nil || closure.Kind != kind || closure.ProducerAttemptID != a.ID || closure.ReceivedInputID != f.inputBinding.ID {
			t.Fatal("Actual output history lost its exact producer/input closure", closure, err)
		}
		if kind == "candidate" && (closure.CandidateID == nil || *closure.CandidateID != *current.CandidateID) {
			t.Fatal("Candidate history closure identifies another candidate")
		}
		if kind == "response" && (closure.Index.ArtifactID != selected.IndexArtifactID || closure.ResponseArtifact == nil || closure.ResponseArtifact.ArtifactID != *current.ResponseArtifactID) {
			t.Fatal("Response closure rewrote its acknowledged result/index binding")
		}
	}
	if _, err := os.Stat(filepath.Join(cwd, fixture.GraphFileA)); !os.IsNotExist(err) {
		t.Fatal("Response-only fixed worker unexpectedly authored a file", err)
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

func TestGraphNativeRunRechecksContextSourceAndStopBeforeGO(t *testing.T) {
	for _, change := range []string{"private-context", "receiving-source", "accepted-group-stop"} {
		t.Run(change, func(t *testing.T) {
			f := graphNativeRunFixture(t)
			e, a := f.engine, f.attempt
			ctx := context.Background()
			working, index := graphNativeRunUserState(t, f)
			prepares := 0
			var cwd string
			e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
				prepares++
				cwd = spec.Cwd
				owned, err := process.Prepare(ctx, spec)
				if err != nil {
					return owned, err
				}
				switch change {
				case "private-context":
					directory := ""
					for i, arg := range spec.Args {
						if arg == "--context-directory" && i+1 < len(spec.Args) {
							directory = spec.Args[i+1]
						}
					}
					if directory == "" {
						t.Error("Actual pre-GO worker has no private graph context directory")
						break
					}
					entries, readErr := os.ReadDir(directory)
					if readErr != nil || len(entries) == 0 {
						t.Error("Actual private context fixture is empty", readErr)
						break
					}
					path := filepath.Join(directory, entries[len(entries)-1].Name())
					file, openErr := os.OpenFile(path, os.O_WRONLY, 0)
					if openErr != nil {
						t.Error(openErr)
						break
					}
					_, writeErr := file.WriteAt([]byte("!"), 0)
					if closeErr := file.Close(); writeErr != nil || closeErr != nil {
						t.Error(writeErr, closeErr)
					}
				case "receiving-source":
					if err := os.WriteFile(filepath.Join(spec.Cwd, "source.txt"), []byte("changed after real native preparation\r\n"), 0600); err != nil {
						t.Error(err)
					}
				case "accepted-group-stop":
					view, readErr := e.db.Graph(context.Background(), f.dispatch.Dispatch.GraphID)
					if readErr != nil {
						t.Error(readErr)
						break
					}
					body, marshalErr := json.Marshal(domain.GraphStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: view.Graph.ControlRevision})
					if marshalErr != nil {
						t.Error(marshalErr)
						break
					}
					result := e.MutateGraph(context.Background(), "graphs.stop", view.Graph.ID, body)
					if result.Envelope.Error != nil || result.Status != 202 {
						t.Error("Actual graph Stop precondition failed", result)
					}
				}
				return owned, nil
			}
			e.run(ctx, a)
			current, err := e.db.Attempt(ctx, a.ID)
			if err != nil || prepares != 1 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome == "succeeded" || current.StartedAt != nil || current.Cleanup.State != "verified" {
				t.Fatal("Pre-GO graph change escaped the final physical/current-control guard", current, prepares, err)
			}
			if change == "accepted-group-stop" && current.Stop == nil {
				t.Fatal("Accepted group Stop was not retained on the actual worker")
			}
			if _, err := os.Stat(filepath.Join(cwd, fixture.GraphFileA)); !os.IsNotExist(err) {
				t.Fatal("The fixed graph instructions ran after their pre-GO authority changed", err)
			}
			graphNativeRunAssertUserState(t, f, working, index)
		})
	}
}

type graphNativeRunLostContextEffectsSink struct {
	artifactSink
	root string
	lost bool
}

func (s *graphNativeRunLostContextEffectsSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	if err := s.artifactSink.SaveArtifact(ctx, m); err != nil {
		return err
	}
	if m.Kind == "graph_operation_effects" && m.Closed && m.Completeness == "complete" {
		body, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(m.RelativePath)))
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte(`"contract":"006-context-preparation-v1"`)) {
			s.lost = true
			return domain.Fail("write_uncertain", "Controlled loss of the real context-effects metadata acknowledgement.")
		}
	}
	return nil
}

func TestGraphNativeRunUnknownContextEffectsRetainClaimAfterKnownNoProcessCleanup(t *testing.T) {
	f := graphNativeRunFixture(t)
	e, a := f.engine, f.attempt
	ctx := context.Background()
	working, index := graphNativeRunUserState(t, f)
	sink := &graphNativeRunLostContextEffectsSink{artifactSink: artifactSink{db: e.db}, root: e.lock.Root()}
	artifacts, err := evidence.New(e.lock, sink)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := workspace.New(e.lock, artifacts)
	if err != nil {
		artifacts.Close()
		t.Fatal(err)
	}
	priorEvidence, priorWorkspace := e.evidence, e.workspace
	e.evidence, e.workspace = artifacts, managed
	t.Cleanup(func() {
		e.evidence, e.workspace = priorEvidence, priorWorkspace
		if err := managed.Close(); err != nil {
			t.Error(err)
		}
		if err := artifacts.Close(); err != nil {
			t.Error(err)
		}
	})
	prepares := 0
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		prepares++
		return process.Prepare(ctx, spec)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || !sink.lost || prepares != 0 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome == "succeeded" || current.StartedAt != nil || current.LaunchIntent || current.Cleanup.State != "verified" || current.EvidenceStatus != "uncertain" {
		t.Fatal("Actual context acknowledgement uncertainty was hidden or native instructions began", current, sink.lost, prepares, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.OwnerInstanceID != e.config.OwnerInstanceID || gate.State == "open" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != a.ID || gate.ActiveClaims[0].GraphID == nil || *gate.ActiveClaims[0].GraphID != f.dispatch.Dispatch.GraphID || gate.ActiveClaims[0].NodeID == nil || *gate.ActiveClaims[0].NodeID != f.dispatch.Dispatch.NodeID {
		t.Fatal("Verified no-process cleanup released or replaced the exact uncertain graph preparation claim", gate, err)
	}
	e.control.Lock()
	fenced := e.fenced
	e.control.Unlock()
	if !fenced {
		t.Fatal("Actual uncertain context preparation failed to fence dispatch")
	}
	// Its real effects bytes/metadata committed despite the controlled lost ack.
	// They remain inspectable; that observation does not retroactively acknowledge
	// the original call or permit recreating this receiving attempt's context.
	page, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	retained := false
	for _, artifact := range page.Items {
		if artifact.Kind != "graph_operation_effects" {
			continue
		}
		meta, err := e.evidence.VerifyClosed(ctx, artifact.ID)
		if err != nil {
			continue
		}
		body, err := e.readAllResponse(meta)
		if err != nil {
			t.Fatal(err)
		}
		retained = retained || bytes.Contains(body, []byte(`"contract":"006-context-preparation-v1"`))
	}
	if !retained {
		t.Fatal("Unacknowledged actual context effects were discarded")
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

func graphNativeReviewAttemptFixture(t *testing.T, declarations ...domain.GraphNodeInput) (graphOutputProducerCase, domain.Attempt) {
	t.Helper()
	declaration := domain.GraphNodeInput{Key: "producer", OriginalText: strings.Repeat("Complete receiving instruction 東京\r\n", 700),
		Pins: &domain.ExecutionPins{HarnessID: "codex"}, FakeScenario: "graph-branch-a", Acceptance: &domain.AcceptanceInput{Deliverable: "candidate", ConditionsText: "Independently inspect the complete authored file change and its source conditions."}}
	if len(declarations) > 1 {
		t.Fatal("Select at most one complete declared review fixture node")
	}
	if len(declarations) == 1 {
		declaration = declarations[0]
	}
	f := graphOutputAdmittedNodeFixture(t, declaration)
	e, author := f.engine, f.attempt
	ctx := context.Background()
	e.run(ctx, author)
	current, err := e.db.Attempt(ctx, author.ID)
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.CandidateID == nil {
		t.Fatal("Actual author prerequisite failed", current, err)
	}
	view, err := e.db.Workflow(ctx, author.TaskID, domain.WorkflowSelector{WorkflowID: author.WorkflowID})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, author.TaskID, *view.Workflow)
	if err != nil || progress.decision.Role != "review" {
		t.Fatal("Actual authored subject did not require a review", progress.decision, err)
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, author.StepID)
	if err != nil || facts.Graph == nil {
		t.Fatal("Actual review facts are unavailable", facts, err)
	}
	next, err := e.db.ContinueWorkflow(ctx, e.config.OwnerInstanceID, author.StepID, facts)
	if err != nil || next == nil {
		t.Fatal("Actual shared-budget review admission failed", next, err)
	}
	review, err := e.db.ClaimAttempt(ctx, next.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil || review.Role != "review" || review.ConnectionID != "codex-native" || review.SourceSnapshotID != nil {
		t.Fatal("Actual review execution has no fresh exact native claim", review, err)
	}
	t.Cleanup(func() {
		current, err := e.db.Attempt(context.Background(), review.ID)
		if err != nil {
			t.Error(err)
			return
		}
		if current.Phase == "settled" {
			return
		}
		// A failed fixture cannot assert that no partial Handoff effects occurred.
		_, err = e.db.SettleWorkflowStep(context.Background(), review.ID, domain.WorkflowStepFacts{StepID: review.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
			Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: false, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete",
				Failure: primary("fixture_no_process", "The controlled review fixture did not launch a native process; unresolved preparation effects remain retained.")}})
		if err != nil {
			t.Error(err)
		}
	})
	return f, review
}

// Real accepted candidate setup for later contribution/receiver fixtures. It
// captures the declaration, runs both owned workers and consumes the exact
// existing owner acceptance path; no acceptance/projection row is invented.
func graphNativeAcceptedCandidateFixture(t *testing.T) (graphOutputProducerCase, domain.Attempt) {
	t.Helper()
	f, supporting := graphNativeReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	e.run(ctx, supporting)
	view, err := e.db.Workflow(ctx, supporting.TaskID, domain.WorkflowSelector{WorkflowID: supporting.WorkflowID})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, supporting.TaskID, *view.Workflow)
	if err != nil || progress.decision.Action != "accept" || progress.subject.Binding == nil {
		t.Fatal("Actual accepted-candidate fixture is not qualified", progress.decision, err)
	}
	result := e.acceptCurrentWorkflow(ctx, progress, domain.NewID(), supporting.StepID)
	if result.Envelope.Error != nil {
		t.Fatal("Actual accepted-candidate mutation failed", result)
	}
	graph, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || len(graph.Nodes) != 1 || graph.Nodes[0].AcceptedResult == nil || graph.Nodes[0].AcceptedResult.AcceptanceKind != "workflow" || graph.Nodes[0].AcceptedResult.ProducerAttemptID != f.attempt.ID || graph.Nodes[0].AcceptedResult.CandidateID == nil || *graph.Nodes[0].AcceptedResult.CandidateID != progress.subject.Binding.CandidateID {
		t.Fatal("Actual acceptance fixture has no exact own producer projection", graph, err)
	}
	return f, supporting
}

func TestGraphNativeRunOwnedReviewPreservesSelectedSubjectWithoutDeliverableOutput(t *testing.T) {
	f, a := graphNativeReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	working, index := graphNativeRunUserState(t, f)
	author, err := e.db.Attempt(ctx, f.attempt.ID)
	if err != nil || author.CandidateID == nil {
		t.Fatal("Actual selected producer is unavailable", author, err)
	}
	beforeBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	prepares := 0
	var spec process.NativeSpec
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		prepares++
		spec = actual
		return process.Prepare(ctx, actual)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || prepares != 1 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" || current.StartedAt == nil || current.SourceSnapshotID == nil || current.CandidateID == nil {
		t.Fatal("The actual owned graph review did not qualify its captured subject", current, prepares, err)
	}
	step, err := e.db.WorkflowStep(ctx, a.StepID)
	if err != nil || step.ReviewID == nil {
		t.Fatal("The owned reviewer retained no strict review record", step, err)
	}
	review, err := e.db.WorkflowReview(ctx, *step.ReviewID)
	if err != nil || review.Report.Outcome != "approved" || review.Report.Subject.Kind != "candidate" || review.Report.Subject.ID != *author.CandidateID || len(review.Report.InspectedEvidence) == 0 {
		t.Fatal("Review evidence does not identify the exact selected authored result", review, err)
	}
	candidate, err := e.db.Candidate(ctx, *current.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta || candidate.Completeness != "complete" {
		t.Fatal("The supporting reviewer changed its receiving files/index", candidate, err)
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil || view.Workflow.SelectedCandidateID == nil || *view.Workflow.SelectedCandidateID != *author.CandidateID || *view.Workflow.SelectedCandidateID == *current.CandidateID || view.Workflow.SelectedResponseBinding != nil {
		t.Fatal("Supporting review output replaced the actual deliverable producer", view, err)
	}
	stored, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || stored.Public.Materialization == nil || stored.Public.Materialization.State != "complete" || stored.Public.Materialization.SourceBasis.BasisKind != "node_subject" || stored.Public.Materialization.SourceBasis.CandidateBinding == nil || stored.Public.Materialization.SourceBasis.CandidateBinding.CandidateID != *author.CandidateID {
		t.Fatal("Review preparation reseeded the graph input or lost its selected-subject receipt", stored, err)
	}
	if observed := stored.Public.OutputObservation; observed != nil {
		if observed.Candidate != nil && (observed.Candidate.State == "complete" || observed.Candidate.ClosureRef != nil) || observed.Response != nil && (observed.Response.State == "complete" || observed.Response.ClosureRef != nil) {
			t.Fatal("A supporting reviewer acquired deliverable output authority", observed)
		}
	}
	if body, err := os.ReadFile(filepath.Join(spec.Cwd, fixture.GraphFileA)); err != nil || string(body) != fixture.GraphContentA {
		t.Fatal("The actual reviewer did not receive the authored candidate file", err)
	}
	if !bytes.Contains(spec.Stdin, []byte(`"graphContext"`)) || !strings.Contains(strings.Join(spec.Args, "\n"), "graph-review") {
		t.Fatal("The graph reviewer did not receive its exact fixed context-reading protocol")
	}
	page, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	readContext := false
	for _, item := range page.Items {
		if item.Kind != "stdout" {
			continue
		}
		meta, err := e.evidence.VerifyClosed(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		body, err := e.readAllResponse(meta)
		if err != nil {
			t.Fatal(err)
		}
		readContext = bytes.Contains(body, []byte(`"fixture_context_reads"`)) && bytes.Contains(body, []byte(`"readVia":"file"`))
	}
	if !readContext {
		t.Fatal("The actual supporting reviewer retained no complete-source read tuples")
	}
	afterBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || afterBudget.Budget.ConsumedAdmissions != beforeBudget.Budget.ConsumedAdmissions || afterBudget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("One author plus one supporting review did not use the one shared finite allowance", afterBudget, err)
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

func TestGraphNativeRunOwnedResponseReviewPreservesEligibleOriginalProducer(t *testing.T) {
	f, a := graphNativeReviewAttemptFixture(t, domain.GraphNodeInput{Key: "producer", OriginalText: strings.Repeat("Complete receiving response instruction 東京\r\n", 700),
		Pins: &domain.ExecutionPins{HarnessID: "codex"}, FakeScenario: "graph-response", Workflow: &domain.WorkflowInput{Review: "required"}, Acceptance: &domain.AcceptanceInput{Deliverable: "response_only", ConditionsText: "Independently inspect the complete controlled response and its captured conditions."}})
	e, ctx := f.engine, context.Background()
	working, index := graphNativeRunUserState(t, f)
	before, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || before.Workflow == nil || before.Workflow.SelectedResponseBinding == nil || before.Workflow.SelectedCandidateID != nil {
		t.Fatal("The actual response author did not select its eligible response", before, err)
	}
	selected := *before.Workflow.SelectedResponseBinding
	prepares := 0
	var spec process.NativeSpec
	e.config.Prepare = func(ctx context.Context, actual process.NativeSpec) (*process.Owned, error) {
		prepares++
		spec = actual
		return process.Prepare(ctx, actual)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || prepares != 1 || current.Phase != "settled" || current.Outcome == nil || *current.Outcome != "succeeded" || current.Cleanup.State != "verified" || current.EvidenceStatus != "complete" || current.CandidateID == nil || current.ResponseArtifactID == nil {
		t.Fatal("Actual owned response review did not qualify", current, prepares, err)
	}
	step, err := e.db.WorkflowStep(ctx, a.StepID)
	if err != nil || step.ReviewID == nil {
		t.Fatal("Actual response review has no strict review record", step, err)
	}
	report, err := e.db.WorkflowReview(ctx, *step.ReviewID)
	if err != nil || report.Report.Outcome != "approved" || report.Report.Subject.Kind != "response" || report.Report.Subject.ID != selected.ResponseArtifactID || report.Report.Subject.SHA256 != selected.ResponseSHA256 || report.ResponseBinding == nil || !reflect.DeepEqual(*report.ResponseBinding, selected) {
		t.Fatal("The supporting report changed the selected response producer or binding", report, err)
	}
	after, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || after.Workflow == nil || after.Workflow.SelectedCandidateID != nil || after.Workflow.SelectedResponseBinding == nil || !reflect.DeepEqual(*after.Workflow.SelectedResponseBinding, selected) || after.Workflow.SelectedResponseBinding.ProducerAttemptID == a.ID {
		t.Fatal("Supporting reviewer output replaced the original eligible response", after, err)
	}
	record, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || record.Public.Materialization == nil || record.Public.Materialization.State != "complete" || record.Public.Materialization.SourceBasis.BasisKind != "node_subject" || record.Public.Materialization.SourceBasis.ResponseBinding == nil || !reflect.DeepEqual(*record.Public.Materialization.SourceBasis.ResponseBinding, selected) || record.Public.Materialization.SourceBasis.ExpectedIndex.ArtifactID != selected.IndexArtifactID {
		t.Fatal("Response review reset its acknowledged source/index basis", record, err)
	}
	if output := record.Public.OutputObservation; output != nil {
		if output.Candidate != nil && (output.Candidate.State == "complete" || output.Candidate.ClosureRef != nil) || output.Response != nil && (output.Response.State == "complete" || output.Response.ClosureRef != nil) {
			t.Fatal("A response reviewer acquired graph deliverable authority", output)
		}
	}
	candidate, err := e.db.Candidate(ctx, *current.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta {
		t.Fatal("A supporting response review changed the actual source/index", candidate, err)
	}
	metadata, err := e.evidence.VerifyClosed(ctx, *current.ResponseArtifactID)
	if err != nil || metadata.Kind != "review_result" || metadata.ID == selected.ResponseArtifactID {
		t.Fatal("Supporting structured review was confused with the deliverable response", metadata, err)
	}
	if _, err := os.Stat(filepath.Join(spec.Cwd, fixture.GraphFileA)); !os.IsNotExist(err) {
		t.Fatal("Response-only review fabricated a file result", err)
	}
	budget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("Actual response author/review used an incorrect shared allowance", budget, err)
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

func TestGraphNativeRunActualCandidateAcceptanceProjectsExactGraphResult(t *testing.T) {
	f, a := graphNativeReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	working, index := graphNativeRunUserState(t, f)
	e.run(ctx, a)
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, a.TaskID, *view.Workflow)
	if err != nil || progress.decision.Action != "accept" || progress.subject.Binding == nil || progress.subject.Candidate == nil || progress.subject.Candidate.AttemptID != f.attempt.ID {
		t.Fatal("Actual author and approved review did not qualify ReviewOnly acceptance", progress.decision, err)
	}
	beforeBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	requestID := domain.NewID()
	accepted := e.acceptCurrentWorkflow(ctx, progress, requestID, a.StepID)
	if accepted.Envelope.Error != nil || accepted.Status != 200 {
		t.Fatal("Actual captured owner acceptance failed", accepted)
	}
	completed, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || completed.Workflow == nil || completed.Workflow.State != "completed" || completed.Acceptance == nil || completed.Acceptance.CandidateBinding.CandidateID != progress.subject.Binding.CandidateID || completed.Acceptance.DeliveryResult != "review_only" {
		t.Fatal("Actual acceptance did not retain its verified candidate and review-only scope", completed, err)
	}
	graph, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || len(graph.Nodes) != 1 {
		t.Fatal(graph, err)
	}
	node := graph.Nodes[0]
	result := node.AcceptedResult
	if node.State != "completed" || result == nil || result.AcceptanceKind != "workflow" || result.AcceptanceID != completed.Acceptance.ID || result.TaskVersion != a.TaskVersion || result.ProducerAttemptID != f.attempt.ID || result.ProducerAttemptID == a.ID || result.CandidateID == nil || *result.CandidateID != progress.subject.Binding.CandidateID || result.ResponseArtifactID != nil || result.InputID != f.inputBinding.ID || node.LatestAttemptID == nil || *node.LatestAttemptID != a.ID {
		t.Fatal("Actual workflow acceptance did not project the exact deliverable producer independently of the supporting reviewer", node)
	}
	if graph.Graph.State != "completed" || graph.Graph.Counts.Required != "1" || graph.Graph.Counts.Completed != "1" || len(graph.ActiveClaims) != 0 {
		t.Fatal("All required ReviewOnly results were not reflected truthfully in the aggregate", graph)
	}
	producer, err := e.db.GraphDispatchForAttempt(ctx, result.ProducerAttemptID)
	if err != nil || producer.Public.OutputObservation == nil || producer.Public.OutputObservation.Candidate == nil || producer.Public.OutputObservation.Candidate.CandidateID != *result.CandidateID || producer.Public.OutputObservation.Candidate.State != "complete" || producer.Public.OutputObservation.Candidate.ClosureRef == nil {
		t.Fatal("Accepted graph result has no exact complete own producer closure", producer, err)
	}
	if _, err := e.readGraphArtifact(ctx, *producer.Public.OutputObservation.Candidate.ClosureRef, "graph_output_closure", 64<<20); err != nil {
		t.Fatal("Accepted producer closure is not actual closed evidence", err)
	}
	replay := e.acceptCurrentWorkflow(ctx, progress, requestID, a.StepID)
	if replay.Envelope.Error != nil {
		t.Fatal("Exact owner acceptance replay failed", replay)
	}
	afterBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || afterBudget.Budget.ConsumedAdmissions != beforeBudget.Budget.ConsumedAdmissions || afterBudget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("Acceptance or its replay created fresh native allowance consumption", afterBudget, err)
	}
	retained, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || retained.Acceptance == nil || retained.Acceptance.ID != completed.Acceptance.ID {
		t.Fatal("Exact replay replaced the original accepted result", retained, err)
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

func TestGraphNativeRunDerivedPromptFailureRetainsActualHandoffPrefix(t *testing.T) {
	f, a := graphNativeReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	working, index := graphNativeRunUserState(t, f)
	detail, err := e.db.TaskVersion(ctx, a.TaskID, a.TaskVersion)
	if err != nil {
		t.Fatal(err)
	}
	resolverErr := errors.New("Controlled resolver failure after actual selected-subject Handoff")
	resolved := false
	_, stage, err := e.prepareWorkflowNative(ctx, a, detail, filepath.Join(t.TempDir(), "unlaunched-review.json"),
		func(context.Context, domain.ExecutionPins) (string, error) { resolved = true; return "", resolverErr }, true)
	if !resolved || !errors.Is(err, resolverErr) || stage == nil || !stage.handoffAttempted || !stage.effectsKnown || stage.snapshot == nil {
		t.Fatal("The actual derived fixture did not fail after its completed Handoff", stage, err)
	}
	if body, err := os.ReadFile(filepath.Join(stage.snapshot.WorkspacePath, fixture.GraphFileA)); err != nil || string(body) != fixture.GraphContentA {
		t.Fatal("The review Handoff reset to original input instead of the selected authored subject", err)
	}
	beforeBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	receiving, known, prefixErr := e.retainGraphHandoffPrefix(ctx, a, stage)
	if prefixErr != nil || !known || receiving == nil || receiving.Materialized.Snapshot == nil || receiving.Materialized.Receipt.State != "complete" || receiving.Materialized.Receipt.SourceBasis.BasisKind != "node_subject" {
		t.Fatal("A prompt failure lost the actual completed Handoff prefix", receiving, known, prefixErr)
	}
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || current.SourceSnapshotID == nil || *current.SourceSnapshotID != stage.snapshot.ID || current.LaunchIntent || current.StartedAt != nil {
		t.Fatal("Handoff prefix association invented a launch or lost exact source ownership", current, err)
	}
	source, err := e.db.SourceSnapshot(ctx, *current.SourceSnapshotID)
	if err != nil || source.CreatedAt != receiving.Materialized.Receipt.CompletedAt {
		t.Fatal("Atomic source association lost its stable receipt timestamp", source, err)
	}
	e.settleUnlaunchedEffects(a, primary("fixture_resolver_failure", "The controlled native resolver failed after selected-subject Handoff."), known, nil, true, resolverErr)
	current, err = e.db.Attempt(ctx, a.ID)
	if err != nil || current.Phase != "settled" || current.Outcome == nil || *current.Outcome == "succeeded" || current.Cleanup.State != "verified" || current.LaunchIntent {
		t.Fatal("Known Handoff prefix failure did not settle as unlaunched", current, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("A known acknowledged Handoff prefix retained or invented unknown process ownership", gate, err)
	}
	afterBudget, err := e.db.GraphBudget(ctx, f.dispatch.Dispatch.GraphID)
	if err != nil || afterBudget.Budget.ConsumedAdmissions != beforeBudget.Budget.ConsumedAdmissions {
		t.Fatal("Handoff prefix settlement spent fresh model allowance", afterBudget, err)
	}
	graphNativeRunAssertUserState(t, f, working, index)
}

type graphNativeReviewStopAfterSchemaSink struct {
	artifactSink
	attemptID string
	stop      func() error
	stopped   bool
}

func (s *graphNativeReviewStopAfterSchemaSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	if err := s.artifactSink.SaveArtifact(ctx, m); err != nil {
		return err
	}
	if !s.stopped && m.AttemptID == s.attemptID && m.Kind == "review_schema" && m.Closed && m.Completeness == "complete" {
		s.stopped = true
		return s.stop()
	}
	return nil
}

func TestGraphNativeRunDerivedStopAfterHandoffRetainsUnassociatedClaim(t *testing.T) {
	f, a := graphNativeReviewAttemptFixture(t)
	e, ctx := f.engine, context.Background()
	working, index := graphNativeRunUserState(t, f)
	sink := &graphNativeReviewStopAfterSchemaSink{artifactSink: artifactSink{db: e.db}, attemptID: a.ID}
	sink.stop = func() error {
		view, err := e.db.Graph(ctx, f.dispatch.Dispatch.GraphID)
		if err != nil {
			return err
		}
		node := view.Nodes[0]
		body, err := json.Marshal(domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: node.ID, ExpectedNodeControlRevision: node.ControlRevision})
		if err != nil {
			return err
		}
		result := e.MutateGraph(ctx, "graphs.nodes.stop", view.Graph.ID+"/"+node.Key, body)
		if result.Envelope.Error != nil {
			return result.Envelope.Error
		}
		if result.Status != 202 {
			return errors.New("The actual review Node Stop was not newly accepted")
		}
		return nil
	}
	artifacts, err := evidence.New(e.lock, sink)
	if err != nil {
		t.Fatal(err)
	}
	managed, err := workspace.New(e.lock, artifacts)
	if err != nil {
		artifacts.Close()
		t.Fatal(err)
	}
	priorEvidence, priorWorkspace := e.evidence, e.workspace
	e.evidence, e.workspace = artifacts, managed
	t.Cleanup(func() {
		e.evidence, e.workspace = priorEvidence, priorWorkspace
		if err := managed.Close(); err != nil {
			t.Error(err)
		}
		if err := artifacts.Close(); err != nil {
			t.Error(err)
		}
	})
	prepares := 0
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		prepares++
		return process.Prepare(ctx, spec)
	}
	e.run(ctx, a)
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || !sink.stopped || prepares != 0 || current.Phase != "settled" || current.Stop == nil || current.StartedAt != nil || current.LaunchIntent || current.SourceSnapshotID != nil || current.Cleanup.State != "verified" {
		t.Fatal("The actual Stop barrier did not occur after Handoff and before receiving association/native creation", current, sink.stopped, prepares, err)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State == "open" || len(gate.ActiveClaims) != 1 || gate.ActiveClaims[0].AttemptID != a.ID {
		t.Fatal("A completed but unassociated Handoff was treated as known-zero I/O after Stop", gate, err)
	}
	page, err := e.db.Artifacts(ctx, a.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	hasHandoff := false
	for _, item := range page.Items {
		hasHandoff = hasHandoff || item.Kind == "source_manifest"
	}
	if !hasHandoff {
		t.Fatal("The Stop counterexample did not retain an actual Handoff prefix")
	}
	graphNativeRunAssertUserState(t, f, working, index)
}
