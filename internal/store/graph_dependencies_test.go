package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are real SQLite workflow/acknowledgement transitions with registered
// metadata proofs. Actual working/index byte composition is tested in workspace.
func graphDependentInputFixture(t *testing.T, configure ...func(*domain.GraphStartInput)) (*Store, domain.GraphDetail, domain.GraphInputPreparationFacts, string) {
	t.Helper()
	setup := []func(*domain.GraphStartInput){func(in *domain.GraphStartInput) {
		edges := []domain.GraphDependencyInput{{From: in.Nodes[0].Key, To: in.Nodes[1].Key, Kind: "context"}}
		in.Dependencies = &edges
	}}
	setup = append(setup, configure...)
	s, d, output, owner := graphOutputConfiguredFixture(t, true, setup...)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, output); e != nil {
		t.Fatal(e)
	}
	task, e := s.Task(ctx, d.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	binding := output.ResponseBinding
	ack := s.AcceptResponseWithWorkflow(ctx, task.Task.ID, domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: task.Task.Revision, TaskVersion: task.Version.Version, AttemptID: binding.ProducerAttemptID, ResponseArtifactID: binding.ResponseArtifactID, ResponseSHA256: binding.ResponseSHA256, Confirmation: "reviewed_response_and_conditions"}, output.VerifiedArtifacts)
	if ack.Status != 200 {
		t.Fatal("actual predecessor acknowledgement failed", ack)
	}
	view, e := s.Graph(ctx, d.Dispatch.GraphID)
	if e != nil || view.Nodes[0].AcceptedResult == nil {
		t.Fatal("predecessor has no actual accepted projection", view, e)
	}
	return s, view, graphResponseDependencyFacts(t, s, view, output, owner), owner
}

func graphResponseDependencyFacts(t *testing.T, s *Store, view domain.GraphDetail, output domain.WorkflowStepFacts, owner string) domain.GraphInputPreparationFacts {
	t.Helper()
	ctx := context.Background()
	binding := output.ResponseBinding
	root, e := s.GraphRoot(ctx, view.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	proofs := append([]domain.VerifiedResponse{}, output.VerifiedArtifacts...)
	for _, ref := range []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs} {
		a, e := s.Artifact(ctx, ref.ArtifactID)
		if e != nil || a.SHA256 == nil {
			t.Fatal(a, e)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: a.ID, SHA256: *a.SHA256, ByteLength: a.ByteLength, PhysicalIdentity: a.PhysicalIdentity})
	}
	source, receiver := view.Nodes[0], view.Nodes[1]
	edge, e := domain.GraphEdgeID(view.Graph.ID, view.Graph.GraphVersion, source.ID, receiver.ID, "context")
	if e != nil {
		t.Fatal(e)
	}
	accepted := source.AcceptedResult
	closure := *output.GraphOutput.Observation.Response.ClosureRef
	contribution := domain.AcceptedContribution{EdgeID: edge, EdgeKind: "context", FromNodeID: source.ID, TaskID: source.TaskID, TaskVersion: source.TaskVersion, AcceptanceKind: accepted.AcceptanceKind, AcceptanceID: accepted.AcceptanceID, ProducerAttemptID: accepted.ProducerAttemptID, InputID: accepted.InputID, ResponseBinding: binding, OutputClosureRef: closure, ConstraintRefs: []domain.GraphScopedReference{}, EvidenceRefs: []domain.GraphContributedEvidence{}}
	sourceInput, e := s.GraphInputRecord(ctx, view.Graph.ID, accepted.InputID)
	if e != nil || sourceInput.ConditionsBinding == nil || sourceInput.Public.ConditionsRef == nil {
		t.Fatal(sourceInput, e)
	}
	addConstraint := func(name string, ref domain.GraphArtifactRef) {
		contribution.ConstraintRefs = append(contribution.ConstraintRefs, domain.GraphScopedReference{Ref: name, ArtifactRef: ref, AuthorityScope: "upstream_local", NodeID: &source.ID, TaskVersion: &source.TaskVersion, Paths: []string{}})
		a, e := s.Artifact(ctx, ref.ArtifactID)
		if e != nil || a.SHA256 == nil {
			t.Fatal(a, e)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: a.ID, SHA256: *a.SHA256, ByteLength: a.ByteLength, PhysicalIdentity: a.PhysicalIdentity})
	}
	for _, condition := range sourceInput.ConditionsBinding.Sources {
		addConstraint("up-"+condition.Ref+"-original", condition.OriginalRef)
		if condition.GuidanceRef != nil {
			addConstraint("up-"+condition.Ref+"-guidance", *condition.GuidanceRef)
		}
		if condition.AcceptanceRef != nil {
			addConstraint("up-"+condition.Ref+"-acceptance", *condition.AcceptanceRef)
		}
	}
	addConstraint("up-conditions", *sourceInput.Public.ConditionsRef)
	for _, item := range []struct {
		name string
		ref  domain.GraphArtifactRef
	}{{"result", *output.GraphOutput.ResponseClosure.ResponseArtifact}, {"closure", closure}} {
		contribution.EvidenceRefs = append(contribution.EvidenceRefs, domain.GraphContributedEvidence{Ref: "edge-" + edge + "-" + item.name, EdgeID: edge, SourceNodeID: source.ID, AcceptanceKind: accepted.AcceptanceKind, AcceptanceID: accepted.AcceptanceID, ProducerAttemptID: accepted.ProducerAttemptID, ResultKind: "response", ResultID: binding.ResponseArtifactID, ArtifactRef: item.ref, Scope: "whole_response", Paths: []string{}})
	}
	vector := []domain.GraphContributorBinding{{NodeID: source.ID, TaskID: source.TaskID, TaskVersion: source.TaskVersion, AcceptanceKind: accepted.AcceptanceKind, AcceptanceID: accepted.AcceptanceID, ProducerAttemptID: accepted.ProducerAttemptID, InputID: accepted.InputID, ResponseBinding: binding, OutputClosureRef: closure}}
	facts := graphBeginFacts(domain.GraphAdmission{Graph: view.Graph, Nodes: view.Nodes, PreparationAttemptID: root.OperationAttemptID}, receiver, root, proofs, owner)
	facts.Control.GraphControlRevision, facts.Control.BudgetAuthorityEpoch, facts.Control.WindowNumber = view.Graph.ControlRevision, view.Graph.BudgetAuthorityEpoch, view.Budget.Budget.CurrentWindow.Number
	facts.Control.Generation = view.DispatchAuthority.Generation
	facts.Trigger.OwnerGeneration = view.DispatchAuthority.Generation
	facts.Trigger.Kind, facts.Trigger.ID = "response_ack", accepted.AcceptanceID
	facts.Control.OperationAttemptID = binding.ProducerAttemptID
	facts.Contributions = []domain.AcceptedContribution{contribution}
	facts.ContributorVector = vector
	facts.Control.ContributorVector = vector
	return facts
}

func TestGraphDependentInputBindsActualAcknowledgementAndExactCausalVector(t *testing.T) {
	s, view, facts, owner := graphDependentInputFixture(t)
	ctx := context.Background()
	before, e := s.GraphBudget(ctx, view.Graph.ID)
	if e != nil {
		t.Fatal(e)
	}
	op, e := s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, facts)
	if e != nil || op.Attempt.ExecutorKind != "local_graph" || op.Attempt.TaskID != view.Graph.RootTaskID || !reflect.DeepEqual(op.Contributions, facts.Contributions) || !reflect.DeepEqual(op.ContributorVector, facts.ContributorVector) {
		t.Fatal("accepted context edge did not register its exact retained composition", op, e)
	}
	if _, e = s.ClaimAttempt(ctx, op.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, op.Attempt.ID); e != nil {
		t.Fatal("exact dependent operation cannot perform guarded retention", e)
	}
	after, e := s.GraphBudget(ctx, view.Graph.ID)
	if e != nil || !reflect.DeepEqual(before.Budget, after.Budget) {
		t.Fatal("dependency composition spent or renewed the model allowance", before, after, e)
	}
	if _, e = s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, facts); e == nil {
		t.Fatal("same accepted callback registered duplicate composition")
	}
}

func TestGraphDependentInputRejectsIncompleteOrChangedAcceptedSelection(t *testing.T) {
	for _, kind := range []string{"missing_edge", "wrong_edge", "wrong_acceptance", "changed_response", "wrong_closure", "missing_vector", "missing_closure_proof"} {
		t.Run(kind, func(t *testing.T) {
			s, view, facts, owner := graphDependentInputFixture(t)
			switch kind {
			case "missing_edge":
				facts.Contributions = nil
			case "wrong_edge":
				facts.Contributions[0].EdgeID = domain.NewID()
			case "wrong_acceptance":
				facts.Contributions[0].AcceptanceID = domain.NewID()
			case "changed_response":
				changed := *facts.Contributions[0].ResponseBinding
				changed.ResponseSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				facts.Contributions[0].ResponseBinding = &changed
			case "wrong_closure":
				facts.Contributions[0].OutputClosureRef = facts.RootBinding.Manifest
			case "missing_vector":
				facts.ContributorVector = nil
				facts.Control.ContributorVector = nil
			case "missing_closure_proof":
				id := facts.Contributions[0].OutputClosureRef.ArtifactID
				proofs := []domain.VerifiedResponse{}
				for _, proof := range facts.VerifiedArtifacts {
					if proof.ArtifactID != id {
						proofs = append(proofs, proof)
					}
				}
				facts.VerifiedArtifacts = proofs
			}
			var before int
			if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&before); e != nil {
				t.Fatal(e)
			}
			if _, e := s.BeginGraphInput(context.Background(), owner, view.Graph.ID, view.Nodes[1].ID, facts); e == nil {
				t.Fatal("changed or incomplete accepted selection registered new input", kind)
			}
			var after int
			if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&after); e != nil || after != before {
				t.Fatal("refused dependent input partly registered work", before, after, e)
			}
		})
	}
}
