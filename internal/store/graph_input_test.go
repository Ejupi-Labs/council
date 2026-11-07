package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphReadyBasis(t *testing.T, configure ...func(*domain.GraphStartInput)) (*Store, domain.GraphAdmission, domain.GraphRootBinding, []domain.VerifiedResponse, string) {
	t.Helper()
	s, graph, f, owner := graphLocalFixture(t, configure...)
	ctx := context.Background()
	attempt := graph.PreparationAttemptID
	if _, e := s.ClaimAttempt(ctx, attempt, owner); e != nil {
		t.Fatal(e)
	}
	manifest := graphLocalMetadata(t, s, attempt, "source_manifest")
	index := graphLocalMetadata(t, s, attempt, "index_snapshot")
	blobs := graphLocalMetadata(t, s, attempt, "graph_index_blobs")
	ref := func(a domain.ArtifactRecord) domain.GraphArtifactRef {
		return domain.GraphArtifactRef{ArtifactID: a.ID, ProducerAttemptID: attempt, SHA256: *a.SHA256, ByteLength: a.ByteLength}
	}
	source := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: attempt, ManifestArtifactID: manifest.ID, IndexArtifactID: index.ID, ManifestSHA256: *manifest.SHA256, BaseCommit: f.BaseCommit, Branch: f.Branch, ProjectIdentity: f.ProjectIdentity, SelectedRelativePath: f.SelectedRelativePath, CreatedAt: domain.Now()}
	root := domain.GraphRootBinding{GraphID: graph.Graph.ID, RootTaskID: graph.Graph.RootTaskID, OperationAttemptID: attempt, SourceSnapshotID: source.ID, Manifest: ref(manifest), Index: ref(index), StagedBlobs: ref(blobs), ProjectID: graph.Graph.ProjectID, ProjectIdentity: f.ProjectIdentity, RepositoryIdentity: f.RepositoryIdentity, SelectedRelativePath: f.SelectedRelativePath, BaseCommit: f.BaseCommit, Branch: f.Branch}
	var e error
	root.BasisSHA256, e = domain.GraphRootDigest(root)
	if e != nil {
		t.Fatal(e)
	}
	proofs := []domain.VerifiedResponse{}
	for _, r := range []domain.ArtifactRecord{manifest, index, blobs} {
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: r.ID, SHA256: *r.SHA256, PhysicalIdentity: r.PhysicalIdentity, ByteLength: r.ByteLength})
	}
	if e = s.CompleteGraphInput(ctx, owner, attempt, domain.GraphInputCompletionFacts{RootBinding: &root, SourceSnapshot: &source, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "ready", VerifiedArtifacts: proofs}); e != nil {
		t.Fatal(e)
	}
	return s, graph, root, proofs, owner
}
func graphBeginFacts(graph domain.GraphAdmission, node domain.GraphNodeSummary, root domain.GraphRootBinding, proofs []domain.VerifiedResponse, owner string) domain.GraphInputPreparationFacts {
	return domain.GraphInputPreparationFacts{Trigger: domain.GraphFreshTrigger{Kind: "completion", ID: graph.PreparationAttemptID, OwnerGeneration: "1"}, Control: domain.GraphDispatchBinding{GraphID: graph.Graph.ID, GraphVersion: "1", RootTaskID: graph.Graph.RootTaskID, OperationAttemptID: graph.PreparationAttemptID, OwnerInstanceID: owner, Generation: "1", GraphControlRevision: "1", BudgetAuthorityEpoch: "1", WindowNumber: "1", NodeID: &node.ID, TaskID: &node.TaskID, TaskVersion: &node.TaskVersion, NodeControlRevision: &node.ControlRevision, ContributorVector: []domain.GraphContributorBinding{}}, RootBinding: root, Contributions: []domain.AcceptedContribution{}, ContributorVector: []domain.GraphContributorBinding{}, VerifiedArtifacts: proofs}
}
func TestGraphBeginInputConsumesFreshTriggerPerNodeWithoutNativeDebit(t *testing.T) {
	s, graph, root, proofs, owner := graphReadyBasis(t)
	ctx := context.Background()
	firstFacts := graphBeginFacts(graph, graph.Nodes[0], root, proofs, owner)
	first, e := s.BeginGraphInput(ctx, owner, graph.Graph.ID, graph.Nodes[0].ID, firstFacts)
	if e != nil || first.Kind != "node_input" || first.ID == *graph.Graph.RootInputID || first.Attempt.ExecutorKind != "local_graph" || first.Attempt.Role != "graph_input" || first.Attempt.TaskID != graph.Graph.RootTaskID || first.RootBinding == nil || !reflect.DeepEqual(*first.RootBinding, root) || first.Control.OperationAttemptID != first.Attempt.ID || first.Control.InputID == nil || *first.Control.InputID != first.ID || first.Control.TaskID == nil || *first.Control.TaskID != graph.Nodes[0].TaskID {
		t.Fatal("exact independent receiver operation was not registered", first, e)
	}
	if _, e = s.BeginGraphInput(ctx, owner, graph.Graph.ID, graph.Nodes[0].ID, firstFacts); e == nil {
		t.Fatal("same fresh callback prepared one node twice")
	}
	second, e := s.BeginGraphInput(ctx, owner, graph.Graph.ID, graph.Nodes[1].ID, graphBeginFacts(graph, graph.Nodes[1], root, proofs, owner))
	if e != nil || second.ID == first.ID || second.Attempt.ID == first.Attempt.ID {
		t.Fatal("global trigger consumption stranded a second ready node", second, e)
	}
	budget, e := s.GraphBudget(ctx, graph.Graph.ID)
	if e != nil || budget.Budget.ConsumedAdmissions != "0" || budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("input preparation invented native debit/window", budget, e)
	}
	for _, op := range []domain.GraphInputOperation{first, second} {
		if _, e = s.ClaimAttempt(ctx, op.Attempt.ID, owner); e != nil {
			t.Fatal(e)
		}
		if e = s.CheckGraphLocalOperation(ctx, owner, op.Attempt.ID); e != nil {
			t.Fatal("registered independent preparation cannot perform guarded I/O", e)
		}
	}
}

func TestGraphBeginInputRejectsStaleOrUnverifiedFactsBeforeNewOperation(t *testing.T) {
	for _, kind := range []string{"wrong_owner", "wrong_epoch", "wrong_node", "missing_root_proof", "fabricated_trigger"} {
		t.Run(kind, func(t *testing.T) {
			s, graph, root, proofs, owner := graphReadyBasis(t)
			facts := graphBeginFacts(graph, graph.Nodes[0], root, proofs, owner)
			switch kind {
			case "wrong_owner":
				facts.Control.OwnerInstanceID = domain.NewID()
			case "wrong_epoch":
				facts.Control.BudgetAuthorityEpoch = "2"
			case "wrong_node":
				facts.Control.NodeID = &graph.Nodes[1].ID
			case "missing_root_proof":
				facts.VerifiedArtifacts = nil
			case "fabricated_trigger":
				facts.Trigger.ID = domain.NewID()
				facts.Control.OperationAttemptID = facts.Trigger.ID
			}
			if _, e := s.BeginGraphInput(context.Background(), owner, graph.Graph.ID, graph.Nodes[0].ID, facts); e == nil {
				t.Fatal("unsafe input preparation accepted", kind)
			}
			var count int
			if e := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count); e != nil || count != 1 {
				t.Fatal("rejected facts registered new work", count, e)
			}
		})
	}
}
