package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These fixtures use actual SQLite acceptance, guidance and native-admission
// transactions with explicitly controlled registered metadata proofs. They do
// not qualify filesystem materialization or a real model invocation.
func graphRebindFixture(t *testing.T) (*Store, domain.GraphDetail, domain.GraphInputPreparationFacts, string) {
	t.Helper()
	empty, zero, timeout := []domain.CheckDeclaration{}, 0, 47
	s, op, completion, owner := graphDependentCompletionFixture(t, "", func(in *domain.GraphStartInput) {
		in.Nodes[1].OriginalText = "\ufeff Exact B original\r\nUnicode: \u03bb"
		in.Nodes[1].TimeoutSeconds = &timeout
		in.Nodes[1].Workflow = &domain.WorkflowInput{Mode: "auto", Review: "auto", Checks: &empty, MaxCorrectionCycles: &zero}
		in.Nodes[1].Routing = &domain.RoutingOptions{ExpectedPolicyRevision: "1"}
		in.Nodes[1].FakeScenario = "graph-response"
	})
	ctx := context.Background()
	if e := s.CompleteGraphInput(ctx, owner, op.Attempt.ID, completion); e != nil {
		t.Fatal(e)
	}
	bf, bt := graphNodeFactsForInput(t, s, op, completion)
	bd, e := s.AdmitGraphNode(ctx, owner, op.GraphID, bt, bf)
	if e != nil || bd.Dispatch.ID == "" {
		t.Fatal("initial receiver workflow was not actually admitted", bd, e)
	}
	if _, e = s.SettleWorkflowStep(ctx, bd.Dispatch.AttemptID, domain.WorkflowStepFacts{StepID: bd.Dispatch.StepID, OwnerInstanceID: owner, Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "controlled_prelaunch_block", Message: "The fixed receiver never launched."}}}); e != nil {
		t.Fatal(e)
	}
	view, e := s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.Task(ctx, view.Nodes[0].TaskID)
	if e != nil {
		t.Fatal(e)
	}
	if guided := s.UpdateGraphNodeGuidance(ctx, owner, view.Graph.ID, view.Nodes[0].Key, graphGuidanceInput(view, a), domain.GraphAdvanceFacts{}); guided.Status != 200 {
		t.Fatal(guided)
	}
	view, e = s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	explicit := graphAdvanceInput(view)
	if result := s.AdvanceGraph(ctx, owner, view.Graph.ID, explicit, domain.GraphAdvanceFacts{}); result.Status != 200 {
		t.Fatal(result)
	}
	view, e = s.Graph(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	root, e := s.GraphRoot(ctx, op.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	proofs := []domain.VerifiedResponse{}
	for _, ref := range []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs} {
		artifact, e := s.Artifact(ctx, ref.ArtifactID)
		if e != nil || artifact.SHA256 == nil {
			t.Fatal(artifact, e)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: artifact.ID, SHA256: *artifact.SHA256, ByteLength: artifact.ByteLength, PhysicalIdentity: artifact.PhysicalIdentity})
	}
	begin := graphBeginFacts(domain.GraphAdmission{Graph: view.Graph}, view.Nodes[0], root, proofs, owner)
	begin.Control.Generation = view.DispatchAuthority.Generation
	begin.Control.GraphControlRevision, begin.Control.BudgetAuthorityEpoch, begin.Control.WindowNumber = view.Graph.ControlRevision, view.Graph.BudgetAuthorityEpoch, view.Budget.Budget.CurrentWindow.Number
	begin.Trigger = domain.GraphFreshTrigger{Kind: "explicit", ID: explicit.RequestID, OwnerGeneration: view.DispatchAuthority.Generation}
	af, at := graphReadyNodeFacts(t, s, domain.GraphAdmission{Graph: view.Graph}, root, proofs, owner, view.Nodes[0], begin)
	ad, e := s.AdmitGraphNode(ctx, owner, view.Graph.ID, at, af)
	if e != nil || ad.Dispatch.ID == "" {
		t.Fatal("new predecessor version was not actually admitted", ad, e)
	}
	materialization := graphMaterializationForDispatch(t, s, ad, af, root, owner)
	output := graphOutputForDispatch(t, s, ad, materialization, owner, true)
	if _, e = s.SettleWorkflowStep(ctx, ad.Dispatch.AttemptID, output); e != nil {
		t.Fatal(e)
	}
	a, e = s.Task(ctx, ad.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	binding := output.ResponseBinding
	ack := s.AcceptResponseWithWorkflow(ctx, a.Task.ID, domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: a.Task.Revision, TaskVersion: a.Version.Version, AttemptID: binding.ProducerAttemptID, ResponseArtifactID: binding.ResponseArtifactID, ResponseSHA256: binding.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}, output.VerifiedArtifacts)
	if ack.Status != 200 {
		t.Fatal("new predecessor has no actual acknowledgement", ack)
	}
	view, e = s.Graph(ctx, op.GraphID)
	if e != nil || view.Nodes[0].TaskVersion != "2" || view.Nodes[0].AcceptedResult == nil || view.Nodes[1].TaskVersion != "1" || view.Nodes[1].InputID == nil {
		t.Fatal("fixture lacks a changed accepted predecessor and retained old receiver workflow", view, e)
	}
	return s, view, graphResponseDependencyFacts(t, s, view, output, owner), owner
}

func TestGraphAdvanceRebindDerivesExactVersionAndRetainsPreviousAuthority(t *testing.T) {
	s, before, preparation, owner := graphRebindFixture(t)
	ctx := context.Background()
	node := before.Nodes[1]
	oldTask, e := s.Task(ctx, node.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	oldOptions, e := s.GraphNodeOptions(ctx, before.Graph.ID, node.ID, node.TaskVersion)
	if e != nil {
		t.Fatal(e)
	}
	basis, e := s.graphVersionRoutingBasis(ctx, s.db, mustGraphHeader(t, s, before.Graph.ID), oldOptions)
	if e != nil {
		t.Fatal(e)
	}
	oldWorkflow, e := s.Workflow(ctx, node.TaskID, domain.WorkflowSelector{})
	if e != nil || oldWorkflow.Workflow == nil {
		t.Fatal(oldWorkflow, e)
	}
	var oldVersion, oldTerminal string
	if e = s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=1", node.TaskID).Scan(&oldVersion); e != nil {
		t.Fatal(e)
	}
	if e = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", *oldTask.Task.LatestAttemptID).Scan(&oldTerminal); e != nil {
		t.Fatal(e)
	}
	in := graphAdvanceInput(before)
	ids := []string{node.ID}
	in.RebindNodeIDs = &ids
	result := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Preparations: []domain.GraphInputPreparationFacts{preparation}})
	if result.Status != 200 {
		t.Fatal("explicit fresh dependency rebind unavailable", result)
	}
	data, e := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
	if e != nil || len(data.ReboundNodes) != 1 || len(data.PreparationAttemptIDs) != 1 || len(data.Dispatches) != 0 {
		t.Fatal("rebind did not report its actual derived version/local preparation", data, e)
	}
	rebound := data.ReboundNodes[0]
	if rebound.NodeID != node.ID || rebound.TaskID != node.TaskID || rebound.FromTaskVersion != "1" || rebound.ToTaskVersion != "2" || rebound.Provenance.Kind != "dependency_rebind" || rebound.Provenance.RequestID != in.RequestID || rebound.Provenance.PreviousInputID != *node.InputID || len(rebound.Provenance.ContributorVectorSHA256) != 64 {
		t.Fatal("rebind provenance lost exact old/new scope", rebound)
	}
	expectedVectorDigest, e := domain.GraphContributorVectorDigest(preparation.ContributorVector)
	if e != nil || rebound.Provenance.ContributorVectorSHA256 != expectedVectorDigest {
		t.Fatal("rebind provenance did not seal the exact selected vector", rebound, expectedVectorDigest, e)
	}
	var privateRaw string
	if e = s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=2", node.TaskID).Scan(&privateRaw); e != nil {
		t.Fatal(e)
	}
	private, e := decode[graphTaskVersionRecord](privateRaw)
	if e != nil || private.RebindProvenance == nil || !reflect.DeepEqual(*private.RebindProvenance, rebound.Provenance) {
		t.Fatal("derived version lost durable rebind provenance", private, e)
	}
	current, e := s.Task(ctx, node.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	expectedVersion := oldTask.Version
	expectedVersion.Version, expectedVersion.CreatedAt, expectedVersion.CreatedBy = current.Version.Version, current.Version.CreatedAt, current.Version.CreatedBy
	if current.Version.Version != "2" || current.Version.CreatedBy != "dependency_rebind" || !reflect.DeepEqual(current.Version, expectedVersion) || current.Task.OriginalText != oldTask.Task.OriginalText || !reflect.DeepEqual(current.Task.LatestAttemptID, oldTask.Task.LatestAttemptID) {
		t.Fatal("derived version replaced user originals, choices, policy or supporting attempt", oldTask, current)
	}
	options, e := s.GraphNodeOptions(ctx, before.Graph.ID, node.ID, "2")
	oldOptions.TaskVersion = "2"
	if e != nil || options.RoutingBasis == nil {
		t.Fatal(options, e)
	}
	// V1 derives its basis at first admission; the rebind must retain that exact
	// effective authority rather than read the current global policy again.
	oldOptions.RoutingBasis = &basis
	if !reflect.DeepEqual(options, oldOptions) {
		t.Fatal("rebind changed immutable execution/workflow/routing choices", oldOptions, options)
	}
	operation, e := s.GraphInputOperationForAttempt(ctx, owner, data.PreparationAttemptIDs[0])
	if e != nil || operation.NodeAuthority == nil || operation.NodeAuthority.TaskVersion != "2" || operation.Control.TaskVersion == nil || *operation.Control.TaskVersion != "2" || !reflect.DeepEqual(operation.Contributions, preparation.Contributions) || !reflect.DeepEqual(operation.ContributorVector, preparation.ContributorVector) || operation.Attempt.ExecutorKind != "local_graph" || operation.Attempt.LaunchIntent {
		t.Fatal("new retained-input operation did not bind the exact current accepted vector", operation, e)
	}
	if uncaptured, e := s.Workflow(ctx, node.TaskID, domain.WorkflowSelector{TaskVersion: "2"}); e != nil || uncaptured.Workflow != nil {
		t.Fatal("rebind finalized workflow before retaining its input")
	}
	after, e := s.Graph(ctx, before.Graph.ID)
	if e != nil || !reflect.DeepEqual(before.Budget, after.Budget) || !reflect.DeepEqual(before.Nodes[0], after.Nodes[0]) || after.Nodes[1].NodeAdmissionID != nil || after.Nodes[1].InputID == nil || *after.Nodes[1].InputID == *node.InputID {
		t.Fatal("rebind spent/refilled allowance, replayed accepted predecessor or kept old authority", before, after, e)
	}
	old, e := s.Workflow(ctx, node.TaskID, domain.WorkflowSelector{WorkflowID: oldWorkflow.Workflow.ID})
	if e != nil || !reflect.DeepEqual(old, oldWorkflow) {
		t.Fatal("rebind rewrote old workflow authority/steps", old, e)
	}
	var retainedVersion, retainedTerminal string
	if e = s.db.QueryRow("SELECT data FROM task_versions WHERE task_id=? AND version=1", node.TaskID).Scan(&retainedVersion); e != nil || retainedVersion != oldVersion {
		t.Fatal("rebind rewrote historical version bytes", e)
	}
	if e = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", *oldTask.Task.LatestAttemptID).Scan(&retainedTerminal); e != nil || retainedTerminal != oldTerminal {
		t.Fatal("rebind rewrote historical producer terminal", e)
	}
	var frozenEnvelope string
	if e = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", in.RequestID).Scan(&frozenEnvelope); e != nil {
		t.Fatal(e)
	}
	// First-call data is a typed DTO; persisted replay data is decoded through
	// any. Compare the complete common wire shape, independently of map order.
	expectedEnvelope, e := decode[domain.Envelope](encode(result.Envelope))
	if e != nil {
		t.Fatal(e)
	}
	replay := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{})
	if replay.Status != 200 || !reflect.DeepEqual(replay.Envelope, expectedEnvelope) {
		t.Fatal("rebind replay did not retain the exact committed result", replay)
	}
	var replayEnvelope string
	if e = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", in.RequestID).Scan(&replayEnvelope); e != nil || replayEnvelope != frozenEnvelope {
		t.Fatal("rebind replay rewrote frozen receipt bytes", e)
	}
}

func mustGraphHeader(t *testing.T, s *Store, graphID string) graphHeader {
	t.Helper()
	h, e := s.graphHeader(context.Background(), s.db, graphID)
	if e != nil {
		t.Fatal(e)
	}
	return h
}

func TestGraphAdvanceRebindRejectsUnprovenOrUnrequestedScopeAtomically(t *testing.T) {
	for _, kind := range []string{"missing_preparation", "not_named", "missing_authority", "wrong_version", "missing_vector", "wrong_acceptance", "missing_fresh_closure", "accepted_stop"} {
		t.Run(kind, func(t *testing.T) {
			s, before, preparation, owner := graphRebindFixture(t)
			in := graphAdvanceInput(before)
			ids := []string{before.Nodes[1].ID}
			in.RebindNodeIDs = &ids
			facts := domain.GraphAdvanceFacts{Preparations: []domain.GraphInputPreparationFacts{preparation}}
			switch kind {
			case "missing_preparation":
				facts.Preparations = nil
			case "not_named":
				in.RebindNodeIDs = nil
			case "missing_authority":
				in.NodeAuthorities = in.NodeAuthorities[:1]
			case "wrong_version":
				wrong := "2"
				facts.Preparations[0].Control.TaskVersion = &wrong
			case "missing_vector":
				facts.Preparations[0].ContributorVector = nil
				facts.Preparations[0].Control.ContributorVector = nil
			case "wrong_acceptance":
				facts.Preparations[0].Contributions[0].AcceptanceID = domain.NewID()
			case "missing_fresh_closure":
				proofs := []domain.VerifiedResponse{}
				for _, proof := range facts.Preparations[0].VerifiedArtifacts {
					if proof.ArtifactID != preparation.Contributions[0].OutputClosureRef.ArtifactID {
						proofs = append(proofs, proof)
					}
				}
				facts.Preparations[0].VerifiedArtifacts = proofs
			case "accepted_stop":
				stopped := s.StopGraphNode(context.Background(), owner, before.Graph.ID, before.Nodes[1].Key, domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: before.Graph.ControlRevision, NodeID: before.Nodes[1].ID, ExpectedNodeControlRevision: before.Nodes[1].ControlRevision})
				if stopped.Status != 202 {
					t.Fatal(stopped)
				}
			}
			var versions, attempts int
			if e := s.db.QueryRow("SELECT count(*) FROM task_versions").Scan(&versions); e != nil {
				t.Fatal(e)
			}
			if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts); e != nil {
				t.Fatal(e)
			}
			if result := s.AdvanceGraph(context.Background(), owner, before.Graph.ID, in, facts); result.Envelope.Error == nil || result.Envelope.Receipt != nil {
				t.Fatal("unsafe rebind committed", kind, result)
			}
			var afterVersions, afterAttempts int
			if e := s.db.QueryRow("SELECT count(*) FROM task_versions").Scan(&afterVersions); e != nil || versions != afterVersions {
				t.Fatal("refused rebind appended version", versions, afterVersions, e)
			}
			if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&afterAttempts); e != nil || attempts != afterAttempts {
				t.Fatal("refused rebind appended operation", attempts, afterAttempts, e)
			}
		})
	}
}

func TestGraphAdvanceRebindCommitFaultRetainsOneDerivedVersionAndOperation(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit"}[after], func(t *testing.T) {
			s, before, preparation, owner := graphRebindFixture(t)
			in := graphAdvanceInput(before)
			ids := []string{before.Nodes[1].ID}
			in.RebindNodeIDs = &ids
			fault := func() error { return errors.New("fixed dependency-rebind acknowledgement fault") }
			if after {
				s.AfterCommit = fault
			} else {
				s.BeforeCommit = fault
			}
			result := s.AdvanceGraph(context.Background(), owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Preparations: []domain.GraphInputPreparationFacts{preparation}})
			s.BeforeCommit, s.AfterCommit = nil, nil
			expected := "storage_failed"
			version := "1"
			if after {
				expected, version = "write_uncertain", "2"
			}
			current, e := s.Task(context.Background(), before.Nodes[1].TaskID)
			if e != nil || result.Envelope.Error == nil || result.Envelope.Error.Code != expected || current.Version.Version != version {
				t.Fatal("rebind fault lost atomic outcome", result, current, e)
			}
			if after {
				replay := s.AdvanceGraph(context.Background(), owner, before.Graph.ID, in, domain.GraphAdvanceFacts{})
				if replay.Status != 200 {
					t.Fatal("committed rebind could not replay", replay)
				}
				var count int
				if e = s.db.QueryRow("SELECT count(*) FROM task_versions WHERE task_id=? AND version=2", before.Nodes[1].TaskID).Scan(&count); e != nil || count != 1 {
					t.Fatal("lost acknowledgement duplicated derived version", count, e)
				}
			}
		})
	}
}

func TestGraphAdvanceRebindCannotCloneUnchangedCompletedInput(t *testing.T) {
	s, view, dependency, owner := graphDependentInputFixture(t)
	ctx := context.Background()
	completed := view.Nodes[0]
	in := graphAdvanceInput(view)
	ids := []string{completed.ID}
	in.RebindNodeIDs = &ids
	preparation := graphBeginFacts(domain.GraphAdmission{Graph: view.Graph}, completed, dependency.RootBinding, dependency.VerifiedArtifacts, owner)
	preparation.Control.Generation = view.DispatchAuthority.Generation
	preparation.Control.GraphControlRevision, preparation.Control.BudgetAuthorityEpoch, preparation.Control.WindowNumber = view.Graph.ControlRevision, view.Graph.BudgetAuthorityEpoch, view.Budget.Budget.CurrentWindow.Number
	result := s.AdvanceGraph(ctx, owner, view.Graph.ID, in, domain.GraphAdvanceFacts{Preparations: []domain.GraphInputPreparationFacts{preparation}})
	if result.Envelope.Error == nil || result.Envelope.Error.Code != "graph_input_current" || result.Envelope.Receipt != nil {
		t.Fatal("explicit name alone cloned an unchanged completed logical execution", result)
	}
	after, err := s.Graph(ctx, view.Graph.ID)
	if err != nil || !reflect.DeepEqual(after, view) {
		t.Fatal("refused unchanged rebind mutated current graph state", view, after, err)
	}
}
