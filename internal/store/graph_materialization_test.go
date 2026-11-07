package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Registered metadata/proof fixtures test the transaction authority and exact
// association. Workspace tests independently qualify actual filesystem effects.
func graphMaterializationFixture(t *testing.T, configure ...func(*domain.GraphStartInput)) (*Store, domain.GraphDispatchView, domain.GraphMaterializationFacts, string) {
	t.Helper()
	s, graph, root, proofs, owner := graphReadyBasis(t, configure...)
	ctx := context.Background()
	input, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	dispatch, e := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, input)
	if e != nil || dispatch.Dispatch.ID == "" {
		t.Fatal(dispatch, e)
	}
	return s, dispatch, graphMaterializationForDispatch(t, s, dispatch, input, root, owner), owner
}

func graphMaterializationForDispatch(t *testing.T, s *Store, dispatch domain.GraphDispatchView, input domain.GraphNodeAdmissionFacts, root domain.GraphRootBinding, owner string) domain.GraphMaterializationFacts {
	t.Helper()
	ctx := context.Background()
	a, e := s.ClaimAttempt(ctx, dispatch.Dispatch.AttemptID, owner)
	if e != nil {
		t.Fatal(e)
	}
	manifest, mp := graphPayload(t, s, a.ID, "source_manifest", []byte(`{"controlled":"receiving-owned manifest"}`))
	index, ip := graphPayload(t, s, a.ID, "index_snapshot", []byte("receiving-owned reconstructed raw index"))
	effects, ep := graphPayload(t, s, a.ID, "graph_operation_effects", []byte(`{"controlled":"observed private effects"}`))
	source := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: a.ID, ManifestArtifactID: manifest.ArtifactID, IndexArtifactID: index.ArtifactID, ManifestSHA256: manifest.SHA256, ProjectIdentity: root.ProjectIdentity, BaseCommit: root.BaseCommit, Branch: root.Branch, SelectedRelativePath: root.SelectedRelativePath, CreatedAt: domain.Now()}
	working, semanticIndex := dispatch.Dispatch.SourceBasis.ExpectedWorkingEntriesSHA256, dispatch.Dispatch.SourceBasis.ExpectedIndexEntriesSHA256
	staged := input.InputBinding.StagedBlobs.SHA256
	selectedProofs := []domain.VerifiedResponse{}
	if dispatch.Dispatch.SourceBasis.BasisKind == "node_subject" {
		// Handoff observes a new receiving-owned staged envelope, separately
		// from the immutable first-input envelope used by an initial author.
		ref, proof := graphPayload(t, s, a.ID, "graph_index_blobs", []byte("controlled receiving selected staged closure"))
		staged = ref.SHA256
		selectedProofs = append(selectedProofs, proof)
	}
	receipt := domain.GraphMaterializationReceipt{ID: domain.NewID(), GraphID: dispatch.Dispatch.GraphID, InputID: input.InputBinding.ID, InputSHA256: input.InputBinding.InputSHA256, OperationAttemptID: a.ID, ReceivingAttemptID: a.ID, SourceBasis: dispatch.Dispatch.SourceBasis, SourceSnapshotID: &source.ID, ObservedManifest: &manifest, ObservedIndex: &index, ObservedWorkingEntriesSHA256: &working, ObservedIndexEntriesSHA256: &semanticIndex, StagedBlobsSHA256: &staged, OperationEffects: &effects, EffectCount: "1", EffectsKnown: true, EvidenceStatus: "complete", State: "complete", CompletedAt: domain.Now()}
	ref, rp := graphPayload(t, s, a.ID, "graph_materialization", []byte(encode(receipt)))
	return domain.GraphMaterializationFacts{Receipt: receipt, ReceiptArtifact: ref, SourceSnapshot: &source, WorkspaceIdentity: "typed-private-workspace-identity", GitControlSHA256: working, VerifiedArtifacts: append(append(input.VerifiedArtifacts, mp, ip, ep, rp), selectedProofs...)}
}
func TestGraphMaterializationBindsExactReceivingReceiptAndPrivateIdentity(t *testing.T) {
	s, dispatch, f, owner := graphMaterializationFixture(t)
	ctx := context.Background()
	if e := s.RecordGraphMaterialization(ctx, owner, dispatch.Dispatch.AttemptID, f); e != nil {
		t.Fatal(e)
	}
	actual, e := s.GraphDispatchForAttempt(ctx, dispatch.Dispatch.AttemptID)
	if e != nil || actual.MaterializationArtifact == nil || *actual.MaterializationArtifact != f.ReceiptArtifact || actual.Public.Materialization == nil || !reflect.DeepEqual(*actual.Public.Materialization, f.Receipt) || actual.PrivateMaterialization == nil || actual.PrivateMaterialization.WorkspaceIdentity != f.WorkspaceIdentity || actual.PrivateMaterialization.GitControlSHA256 != f.GitControlSHA256 {
		t.Fatal("receiving receipt/private proof association was lost", actual, e)
	}
	a, e := s.Attempt(ctx, dispatch.Dispatch.AttemptID)
	if e != nil || a.SourceSnapshotID == nil || *a.SourceSnapshotID != f.SourceSnapshot.ID || a.LaunchIntent {
		t.Fatal("materialization invented launch or lost source", a, e)
	}
	if e = s.RecordGraphMaterialization(ctx, owner, a.ID, f); e != nil {
		t.Fatal("exact observation reconciliation failed", e)
	}
	changed := f
	changed.WorkspaceIdentity = "different-private-workspace"
	if e = s.RecordGraphMaterialization(ctx, owner, a.ID, changed); e == nil {
		t.Fatal("materialization observation could be rebound")
	}
}
func TestGraphMaterializationRejectsForeignMissingOrChangedProofBeforeSourceCommit(t *testing.T) {
	for _, kind := range []string{"wrong_receiver", "wrong_subject", "wrong_semantic_index", "missing_receipt_proof", "missing_source_proof", "missing_private_identity"} {
		t.Run(kind, func(t *testing.T) {
			s, dispatch, f, owner := graphMaterializationFixture(t)
			switch kind {
			case "wrong_receiver":
				f.Receipt.ReceivingAttemptID = domain.NewID()
			case "wrong_subject":
				f.Receipt.SourceBasis.StepID = domain.NewID()
			case "wrong_semantic_index":
				bad := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
				f.Receipt.ObservedIndexEntriesSHA256 = &bad
			case "missing_receipt_proof":
				f.VerifiedArtifacts = f.VerifiedArtifacts[:len(f.VerifiedArtifacts)-1]
			case "missing_source_proof":
				for i, p := range f.VerifiedArtifacts {
					if p.ArtifactID == f.SourceSnapshot.ManifestArtifactID {
						f.VerifiedArtifacts = append(f.VerifiedArtifacts[:i], f.VerifiedArtifacts[i+1:]...)
						break
					}
				}
			case "missing_private_identity":
				f.WorkspaceIdentity = ""
			}
			if kind == "wrong_receiver" || kind == "wrong_subject" || kind == "wrong_semantic_index" {
				// Keep the tampered receipt itself genuinely closed and freshly proven so
				// the negative reaches exact receiver/subject/semantic binding guards.
				ref, proof := graphPayload(t, s, dispatch.Dispatch.AttemptID, "graph_materialization", []byte(encode(f.Receipt)))
				f.ReceiptArtifact = ref
				f.VerifiedArtifacts = append(f.VerifiedArtifacts, proof)
			}
			if e := s.RecordGraphMaterialization(context.Background(), owner, dispatch.Dispatch.AttemptID, f); e == nil {
				t.Fatal("unsafe materialization accepted")
			}
			a, e := s.Attempt(context.Background(), dispatch.Dispatch.AttemptID)
			if e != nil || a.SourceSnapshotID != nil || a.LaunchIntent {
				t.Fatal("failed proof partly installed source/launch", a, e)
			}
		})
	}
}
