package store

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// The producer and artifact facts below are controlled SQLite metadata, not OS
// or model qualification. Actual native/selected-subject handoff is tested by Engine.
func graphReviewAdmissionFacts(t *testing.T, s *Store, d domain.GraphDispatchView, author domain.WorkflowStepFacts) domain.WorkflowStepAdmissionFacts {
	t.Helper()
	ctx := context.Background()
	view, e := s.Workflow(ctx, d.Dispatch.TaskID, domain.WorkflowSelector{})
	if e != nil || view.Workflow == nil {
		t.Fatal(view, e)
	}
	w := *view.Workflow
	task, e := s.Task(ctx, d.Dispatch.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	graph, e := s.Graph(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	input, e := s.GraphInput(ctx, d.Dispatch.GraphID, d.Dispatch.InputID)
	if e != nil || input.InputBinding == nil {
		t.Fatal(input, e)
	}
	key, invalid := domain.WorkflowStageKey("review", 0, "", *w.SelectedCandidateID)
	if invalid != nil {
		t.Fatal(invalid)
	}
	manifest, e := s.Artifact(ctx, author.CandidateBinding.ManifestArtifactID)
	if e != nil {
		t.Fatal(e)
	}
	index, e := s.Artifact(ctx, author.CandidateBinding.IndexArtifactID)
	if e != nil {
		t.Fatal(e)
	}
	ref := func(a domain.ArtifactRecord) domain.GraphArtifactRef {
		return domain.GraphArtifactRef{ArtifactID: a.ID, ProducerAttemptID: a.AttemptID, SHA256: *a.SHA256, ByteLength: a.ByteLength}
	}
	node := graph.Nodes[0]
	for _, n := range graph.Nodes {
		if n.ID == d.Dispatch.NodeID {
			node = n
		}
	}
	control := domain.GraphDispatchBinding{GraphID: d.Dispatch.GraphID, GraphVersion: d.Dispatch.GraphVersion, RootTaskID: graph.Graph.RootTaskID, OperationAttemptID: d.Dispatch.AttemptID, OwnerInstanceID: d.Dispatch.OwnerInstanceID, Generation: graph.DispatchAuthority.Generation, GraphControlRevision: graph.Graph.ControlRevision, BudgetAuthorityEpoch: graph.Graph.BudgetAuthorityEpoch, WindowNumber: graph.Budget.Budget.CurrentWindow.Number, NodeID: &node.ID, TaskID: &node.TaskID, TaskVersion: &node.TaskVersion, NodeControlRevision: &node.ControlRevision, InputID: &d.Dispatch.InputID, ContributorVector: input.InputBinding.ContributorVector}
	return domain.WorkflowStepAdmissionFacts{ExpectedTaskRevision: task.Task.Revision, ExpectedWorkflowRevision: w.Revision, Routing: wrapperRoutingFacts("fake"), Plan: domain.WorkflowStagePlan{Role: "review", ExecutorKind: "native", Cycle: 0, LogicalKey: key, InputCandidateID: w.SelectedCandidateID, PredecessorStepID: w.CurrentStepID, ReasonCodes: []string{"fresh_graph_review"}}, Graph: &domain.GraphStageAdmissionFacts{Trigger: domain.GraphFreshTrigger{Kind: "completion", ID: d.Dispatch.AttemptID, OwnerGeneration: graph.DispatchAuthority.Generation}, Control: control, NodeAdmissionID: d.Dispatch.NodeAdmissionID, InputBinding: *input.InputBinding, SourceBasis: domain.GraphSourceBasis{BasisKind: "node_subject", InputID: d.Dispatch.InputID, InputSHA256: d.Dispatch.InputSHA256, CandidateBinding: author.CandidateBinding, ExpectedManifest: ref(manifest), ExpectedIndex: ref(index), ExpectedWorkingEntriesSHA256: strings.Repeat("d", 64), ExpectedIndexEntriesSHA256: strings.Repeat("e", 64)}, VerifiedArtifacts: author.VerifiedArtifacts}}
}
func TestGraphStageAdmissionUsesOneBudgetAndSiblingSafeProof(t *testing.T) {
	s, d, author, owner := graphOutputSettlementFixture(t)
	ctx := context.Background()
	if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, author); e != nil {
		t.Fatal(e)
	}
	facts := graphReviewAdmissionFacts(t, s, d, author)
	graph, e := s.Graph(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	root, e := s.GraphRoot(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	rootProofs := []domain.VerifiedResponse{}
	for _, ref := range []domain.GraphArtifactRef{root.Manifest, root.Index, root.StagedBlobs} {
		a, e := s.Artifact(ctx, ref.ArtifactID)
		if e != nil {
			t.Fatal(e)
		}
		rootProofs = append(rootProofs, domain.VerifiedResponse{ArtifactID: a.ID, SHA256: *a.SHA256, ByteLength: a.ByteLength, PhysicalIdentity: a.PhysicalIdentity})
	}
	sibling := graph.Nodes[1]
	other, trigger := graphReadyNodeFacts(t, s, domain.GraphAdmission{Graph: graph.Graph, Nodes: graph.Nodes, PreparationAttemptID: root.OperationAttemptID}, root, rootProofs, owner, sibling)
	second, e := s.AdmitGraphNode(ctx, owner, d.Dispatch.GraphID, trigger, other)
	if e != nil || second.Dispatch.ID == "" {
		t.Fatal(second, e)
	}
	before, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if e != nil {
		t.Fatal(e)
	}
	predecessor, e := s.WorkflowStep(ctx, d.Dispatch.StepID)
	if e != nil || predecessor.AdvanceProof == nil || predecessor.AdvanceProof.BudgetRevision == before.Budget.Revision {
		t.Fatal("fixture did not create a distinct sibling debit", predecessor, before, e)
	}
	next, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, facts)
	if e != nil || next == nil {
		t.Fatal("sibling debit stranded unchanged graph workflow authority", next, e)
	}
	registered, e := s.GraphDispatchForAttempt(ctx, next.Attempt.ID)
	if e != nil || registered.Public.Dispatch.NodeAdmissionID != d.Dispatch.ID || registered.Public.Dispatch.InputID != d.Dispatch.InputID || registered.Public.Dispatch.SourceBasis.BasisKind != "node_subject" || registered.Public.Dispatch.SourceBasis.StepID != next.Step.ID || registered.Public.Dispatch.SourceBasis.CandidateBinding == nil || registered.Public.Dispatch.SourceBasis.CandidateBinding.CandidateID != author.CandidateBinding.CandidateID || next.GraphBudgetRef == nil || next.GraphBudgetRef.BudgetOwnerTaskID != graph.Graph.RootTaskID {
		t.Fatal("supporting execution lost exact selected source or root budget", registered, next, e)
	}
	after, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
	if e != nil || after.Budget.ConsumedAdmissions != "3" || after.Budget.RemainingAdmissions != "1" || !reflect.DeepEqual(before.Budget.CurrentWindow, after.Budget.CurrentWindow) || after.NodeConsumption[0].ConsumedAdmissions != "2" || after.NodeConsumption[1].ConsumedAdmissions != "1" {
		t.Fatal("stage admission refilled/reset/misowned graph budget", after, e)
	}
	old, e := s.WorkflowStep(ctx, d.Dispatch.StepID)
	if e != nil || !reflect.DeepEqual(old, predecessor) {
		t.Fatal("continuation rewrote predecessor proof", old, e)
	}
	if duplicate, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, facts); e != nil || duplicate != nil {
		t.Fatal("same stage proof consumed twice", duplicate, e)
	}
}
func TestGraphStageAdmissionRejectsMissingStaleOrUnverifiedScope(t *testing.T) {
	for _, kind := range []string{"missing_graph", "wrong_epoch", "wrong_input", "wrong_anchor", "wrong_selected_subject", "missing_fresh_proofs"} {
		t.Run(kind, func(t *testing.T) {
			s, d, author, owner := graphOutputSettlementFixture(t)
			ctx := context.Background()
			if _, e := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, author); e != nil {
				t.Fatal(e)
			}
			facts := graphReviewAdmissionFacts(t, s, d, author)
			switch kind {
			case "missing_graph":
				facts.Graph = nil
			case "wrong_epoch":
				facts.Graph.Control.BudgetAuthorityEpoch = "2"
			case "wrong_input":
				facts.Graph.Control.InputID = ptr(domain.NewID())
			case "wrong_anchor":
				facts.Graph.NodeAdmissionID = domain.NewID()
			case "wrong_selected_subject":
				copy := *facts.Graph.SourceBasis.CandidateBinding
				copy.CandidateID = domain.NewID()
				facts.Graph.SourceBasis.CandidateBinding = &copy
			case "missing_fresh_proofs":
				facts.Graph.VerifiedArtifacts = nil
			}
			before, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
			if e != nil {
				t.Fatal(e)
			}
			if next, e := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, facts); e == nil && next != nil {
				t.Fatal("unsafe graph stage admitted", next)
			}
			after, e := s.GraphBudget(ctx, d.Dispatch.GraphID)
			if e != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("invalid stage scope spent shared budget", after, e)
			}
		})
	}
}
