package engine

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// The real terminal effects acknowledgement is committed before this hook
// corrupts an existing, unchanged source_file. The manifest, inode and length
// remain intact; neither a synthetic proof nor a failed metadata write is used.
type graphInputCorruptAfterTerminalSink struct {
	artifactSink
	privateRoot string
	attemptID   string
	member      evidence.Metadata
	terminal    *graphInputEffects
	corrupted   bool
	fault       error
}

func (s *graphInputCorruptAfterTerminalSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	if err := s.artifactSink.SaveArtifact(ctx, m); err != nil {
		return err
	}
	if s.terminal != nil || m.AttemptID != s.attemptID || m.Kind != "graph_operation_effects" || !m.Closed || m.Completeness != "complete" {
		return nil
	}
	body, err := os.ReadFile(filepath.Join(s.privateRoot, filepath.FromSlash(m.RelativePath)))
	if err != nil {
		return err
	}
	var operation graphInputEffects
	if json.Unmarshal(body, &operation) != nil || operation.Contract != "006-input-retention-effects-v1" || operation.Input == nil {
		return nil // Ignore the real intent and the workspace composition records.
	}
	s.terminal = &operation
	manifestMeta, err := s.artifactSink.LoadArtifact(ctx, operation.Input.Manifest.ArtifactID)
	if err == nil {
		body, err = os.ReadFile(filepath.Join(s.privateRoot, filepath.FromSlash(manifestMeta.RelativePath)))
	}
	var manifest workspace.Manifest
	if err == nil {
		err = json.Unmarshal(body, &manifest)
	}
	found := false
	for _, member := range manifest.Entries {
		found = found || member.Path == "source.txt" && member.Type == "file" && member.ArtifactID == s.member.ID && member.SHA256 == *s.member.SHA256
	}
	if err == nil && !found {
		err = fmt.Errorf("the actual terminal input does not retain the unchanged source_file")
	}
	if err == nil {
		err = s.corruptMember()
	}
	s.fault = err
	return err
}

func (s *graphInputCorruptAfterTerminalSink) corruptMember() error {
	if !filepath.IsLocal(filepath.FromSlash(s.member.RelativePath)) || s.member.Kind != "source_file" || s.member.SHA256 == nil {
		return fmt.Errorf("the controlled member is not an owned closed source_file")
	}
	path := filepath.Join(s.privateRoot, filepath.FromSlash(s.member.RelativePath))
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return errors.Join(err, fmt.Errorf("the controlled source_file must contain actual bytes"))
	}
	body[0] ^= 0x20
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	n, writeErr := file.WriteAt(body, 0)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil || n != len(body) {
		return errors.Join(err, fmt.Errorf("the controlled in-place corruption did not complete"))
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() {
		return errors.Join(err, fmt.Errorf("the controlled corruption replaced or resized its member"))
	}
	s.corrupted = true
	return nil
}

// This registers a real dependent operation after an actual owned response
// author and explicit acknowledgement. It deliberately calls the store with
// the controller's fresh physical proofs so an automatic acknowledgement
// callback cannot preempt the controlled local operation under test.
func graphInputFaultAcknowledgedOperation(t *testing.T) (*Engine, domain.GraphInputOperation) {
	t.Helper()
	ctx := context.Background()
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	runCtx, cancel := context.WithCancel(ctx)
	e.ctx, e.cancel = runCtx, cancel
	t.Cleanup(cancel)
	edges := []domain.GraphDependencyInput{{From: "producer", To: "receiver", Kind: "context"}}
	group, source := graphReleaseReadyGroup(t, e, projectID, domain.GraphStartInput{Nodes: []domain.GraphNodeInput{
		{Key: "producer", OriginalText: "Produce the controlled response for explicit acknowledgement.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
		{Key: "receiver", OriginalText: "Receive the exact acknowledged response without changing the original source.", FakeScenario: "graph-response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}},
	}, Dependencies: &edges})
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
		t.Fatal("actual dependency fixture admitted an unqualified receiver", view)
	}
	claimed, err := e.db.ClaimAttempt(ctx, *producer.LatestAttemptID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	e.run(ctx, claimed)
	in := exactResponseAcceptanceInput(t, e, claimed.TaskID, claimed.ID)
	wf, err := e.db.Workflow(ctx, claimed.TaskID, domain.WorkflowSelector{})
	if err != nil || wf.Workflow == nil {
		t.Fatal(wf, err)
	}
	progress, err := e.inspectWorkflowProgress(ctx, claimed.TaskID, *wf.Workflow)
	if err != nil || progress.subject.Response == nil || progress.decision.Action != "response_acceptance_pending" {
		t.Fatal("actual response did not reach explicit acknowledgement", progress.decision, err)
	}
	ordinary, err := e.verifyWorkflowArtifacts(ctx, append(progress.subject.artifactIDs(), progress.artifactIDs...)...)
	if err != nil {
		t.Fatal(err)
	}
	graphProofs, err := e.graphAcceptanceArtifactProofs(ctx, progress.subject)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := mergeGraphAcceptanceProofs(ordinary, graphProofs)
	if err != nil {
		t.Fatal(err)
	}
	acknowledged := e.db.AcceptResponseWithWorkflow(ctx, claimed.TaskID, in, proofs)
	ack := assertOriginalResponseAcceptanceWire(t, acknowledged, in)
	view, err = e.db.Graph(ctx, group.Graph.ID)
	if err != nil || view.DispatchAuthority == nil {
		t.Fatal(view, err)
	}
	for _, node := range view.Nodes {
		if node.ID == receiver.ID {
			receiver = node
		}
	}
	definition, err := e.db.GraphDefinitionPrivate(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := e.db.GraphRoot(ctx, group.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := e.selectGraphContributions(ctx, definition, root, receiver)
	if err != nil || len(selected.Sources) != 1 || selected.Sources[0].Binding.AcceptanceID != ack.ID {
		t.Fatal("actual acknowledged contribution was not selected", selected, err)
	}
	rootSource, err := e.graphRootSourceSnapshot(ctx, root, definition.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	rootProofs, err := e.verifyGraphRootClosure(ctx, root, rootSource)
	if err != nil {
		t.Fatal(err)
	}
	proofs, err = mergeGraphAcceptanceProofs(rootProofs, selected.VerifiedArtifacts)
	if err != nil {
		t.Fatal(err)
	}
	nodeID, taskID, version, revision := receiver.ID, receiver.TaskID, receiver.TaskVersion, receiver.ControlRevision
	input, err := e.db.BeginGraphInput(ctx, e.config.OwnerInstanceID, group.Graph.ID, nodeID, domain.GraphInputPreparationFacts{
		Trigger: domain.GraphFreshTrigger{Kind: "response_ack", ID: ack.ID, OwnerGeneration: view.DispatchAuthority.Generation}, RootBinding: root,
		Control: domain.GraphDispatchBinding{GraphID: group.Graph.ID, GraphVersion: view.Graph.GraphVersion, RootTaskID: view.Graph.RootTaskID,
			OperationAttemptID: claimed.ID, OwnerInstanceID: e.config.OwnerInstanceID, Generation: view.DispatchAuthority.Generation,
			GraphControlRevision: view.Graph.ControlRevision, BudgetAuthorityEpoch: view.Graph.BudgetAuthorityEpoch, WindowNumber: view.Budget.Budget.CurrentWindow.Number,
			NodeID: &nodeID, TaskID: &taskID, TaskVersion: &version, NodeControlRevision: &revision, ContributorVector: selected.ContributorVector},
		Contributions: []domain.AcceptedContribution{selected.Sources[0].Binding}, ContributorVector: selected.ContributorVector, VerifiedArtifacts: proofs})
	if err != nil {
		t.Fatal("actual fresh acknowledgement could not register the dependent input", err)
	}
	local, err := e.db.ClaimAttempt(ctx, input.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := e.db.GraphInputOperationForAttempt(ctx, e.config.OwnerInstanceID, local.ID)
	if err != nil || len(op.Contributions) != 1 || len(op.ContributorVector) != 1 {
		t.Fatal("actual claimed dependent operation has no exact selection", op, err)
	}
	return e, op
}

func TestGraphInputTerminalAcknowledgementRechecksNonAnchorRetainedMembers(t *testing.T) {
	for _, kind := range []string{"composed", "independent"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			var e *Engine
			var op domain.GraphInputOperation
			if kind == "composed" {
				e, op = graphInputFaultAcknowledgedOperation(t)
			} else {
				e, op, _ = graphUpstreamIndependentOperationFixture(t)
			}
			if op.RootBinding == nil {
				t.Fatal("actual claimed input has no private root")
			}
			manifest, _, err := e.workflowManifest(ctx, op.RootBinding.Manifest.ArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			entry := closureTestEntry(t, manifest, "source.txt")
			member, err := e.evidence.VerifyClosed(ctx, entry.ArtifactID)
			if err != nil || member.Kind != "source_file" || member.SHA256 == nil || *member.SHA256 != entry.SHA256 {
				t.Fatal("unchanged non-anchor source_file precondition", member, err)
			}
			project, _, err := e.db.Project(ctx, op.Definition.Original.ProjectID)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(filepath.Join(project.ResolvedPath, "source.txt"))
			if err != nil {
				t.Fatal(err)
			}
			index, err := os.ReadFile(filepath.Join(project.ResolvedPath, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			budget, err := e.db.GraphBudget(ctx, op.GraphID)
			if err != nil {
				t.Fatal(err)
			}
			sink := &graphInputCorruptAfterTerminalSink{artifactSink: artifactSink{e.db}, privateRoot: e.lock.Root(), attemptID: op.Attempt.ID, member: member}
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
			e.run(ctx, op.Attempt)
			if sink.fault != nil || !sink.corrupted || sink.terminal == nil || sink.terminal.Input == nil {
				t.Fatal("actual post-verification terminal acknowledgement fault was not reached", sink.corrupted, sink.fault)
			}
			if _, err = e.evidence.VerifyClosed(ctx, member.ID); !errors.Is(err, evidence.ErrCorrupt) {
				t.Fatal("controlled actual member corruption was not detected by direct verification", err)
			}
			b := *sink.terminal.Input
			for _, ref := range []domain.GraphArtifactRef{op.RootBinding.Manifest, b.Manifest, b.Index, b.StagedBlobs, b.Provenance, b.ContextPacket} {
				if _, err = e.evidence.VerifyClosed(ctx, ref.ArtifactID); err != nil {
					t.Fatal("the fault changed an anchor instead of its unchanged retained member", ref.ArtifactID, err)
				}
			}
			input, err := e.db.GraphInput(ctx, op.GraphID, op.ID)
			if err != nil || input.Summary.State == "ready" || input.Summary.Current || input.InputBinding != nil {
				t.Fatal("a member corrupted during terminal acknowledgement became a ready/current usable input", input, err)
			}
			terminal, err := e.db.Attempt(ctx, op.Attempt.ID)
			if err != nil || terminal.Phase != "settled" || terminal.Outcome == nil || *terminal.Outcome == "succeeded" || terminal.SourceSnapshotID != nil || terminal.EvidenceStatus == "complete" || terminal.LaunchIntent || terminal.Cleanup.State != "verified" || terminal.Cleanup.Scope != "not_launched" {
				t.Fatal("corrupt retained evidence became complete source or successful work", terminal, err)
			}
			if input.OperationEffects == nil || input.Summary.Failure == nil {
				t.Fatal("nonqualifying input lost actual effects or failure evidence", input)
			}
			if _, err = e.verifyGraphArtifact(ctx, *input.OperationEffects, "graph_operation_effects"); err != nil {
				t.Fatal("actual acknowledged operation effects were lost", err)
			}
			if terminal.EvidenceStatus == "uncertain" {
				gate, err := e.db.Gate(ctx)
				if err != nil || gate.State != "fenced" {
					t.Fatal("uncertain retained input did not fence dispatch", gate, err)
				}
			}
			afterBudget, err := e.db.GraphBudget(ctx, op.GraphID)
			if err != nil || !reflect.DeepEqual(budget.Budget, afterBudget.Budget) {
				t.Fatal("local input integrity failure spent or replenished model authority", afterBudget, err)
			}
			after, err := os.ReadFile(filepath.Join(project.ResolvedPath, "source.txt"))
			afterIndex, indexErr := os.ReadFile(filepath.Join(project.ResolvedPath, ".git", "index"))
			if err != nil || indexErr != nil || !bytes.Equal(original, after) || !bytes.Equal(index, afterIndex) {
				t.Fatal("private retained-member corruption changed original source or staging", err, indexErr)
			}
		})
	}
}
