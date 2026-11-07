package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphDependentCompletionFixture(t *testing.T, change string, configure ...func(*domain.GraphStartInput)) (*Store, domain.GraphInputOperation, domain.GraphInputCompletionFacts, string) {
	t.Helper()
	s, view, begin, owner := graphDependentInputFixture(t, configure...)
	ctx := context.Background()
	op, e := s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, begin)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ClaimAttempt(ctx, op.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	node := view.Nodes[1]
	task, e := s.Task(ctx, node.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	sharedText := ""
	if op.Definition.Original.SharedInstructions != nil {
		sharedText = *op.Definition.Original.SharedInstructions
	}
	shared, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(sharedText))
	original, _ := graphPayload(t, s, op.Attempt.ID, "graph_original", []byte(task.Task.OriginalText))
	acceptance, _ := graphPayload(t, s, op.Attempt.ID, "graph_acceptance", []byte(encode(task.Version.Acceptance)))
	upstream, e := s.GraphInputRecord(ctx, view.Graph.ID, begin.ContributorVector[0].InputID)
	if e != nil || upstream.ConditionsBinding == nil {
		t.Fatal(upstream, e)
	}
	var source domain.GraphConditionSource
	for _, item := range upstream.ConditionsBinding.Sources {
		if item.AuthorityScope == "receiver" {
			source = item
		}
	}
	source.Ref, source.AuthorityScope = "upstream-node", "upstream_local"
	sources := []domain.GraphConditionSource{{Ref: "graph-instructions", AuthorityScope: "graph_shared", OriginalRef: shared}, {Ref: "node-instructions", AuthorityScope: "receiver", NodeID: &node.ID, TaskVersion: &node.TaskVersion, OriginalRef: original, AcceptanceRef: &acceptance}, source}
	if change == "missing_upstream_source" {
		sources = sources[:2]
	}
	if change == "wrong_upstream_original" {
		sources[2].OriginalRef = shared // An actual closed original, from the wrong scoped source.
	}
	conditions := domain.GraphConditionsBinding{GraphID: view.Graph.ID, GraphVersion: "1", ReceiverTaskID: node.TaskID, ReceiverTaskVersion: node.TaskVersion, Sources: sources}
	conditions.ConditionsSHA256, e = domain.GraphConditionsDigest(conditions)
	if e != nil {
		t.Fatal(e)
	}
	conditionRef, conditionProof := graphPayload(t, s, op.Attempt.ID, "graph_conditions", []byte(encode(conditions)))
	manifestBody := []byte(`{"controlled":"assembled input/source metadata"}`)
	manifest, mp := graphPayload(t, s, op.Attempt.ID, "graph_input_manifest", manifestBody)
	index, ip := graphPayload(t, s, op.Attempt.ID, "index_snapshot", []byte("assembled raw index metadata"))
	blobs, bp := graphPayload(t, s, op.Attempt.ID, "graph_index_blobs", []byte("assembled staged closure metadata"))
	provenance, pp := graphPayload(t, s, op.Attempt.ID, "graph_input_provenance", []byte("complete causal metadata"))
	input := domain.GraphInputBinding{ID: op.ID, GraphID: op.GraphID, GraphVersion: "1", RootBasisSHA256: begin.RootBinding.BasisSHA256, OperationAttemptID: op.Attempt.ID, ReceivingNodeID: node.ID, ReceivingTaskID: node.TaskID, ReceivingTaskVersion: node.TaskVersion, Contributions: op.Contributions, ContributorVector: op.ContributorVector, Manifest: manifest, Index: index, StagedBlobs: blobs, Provenance: provenance}
	input.InputSHA256, e = domain.GraphInputDigest(input)
	if e != nil {
		t.Fatal(e)
	}
	packet := domain.GraphContextPacket{ID: domain.NewID(), GraphID: op.GraphID, InputID: op.ID, InputSHA256: input.InputSHA256, ReceivingTaskVersion: node.TaskVersion, GraphInstructionsRef: shared, NodeInstructionsRef: original, AcceptanceRef: &acceptance, ConstraintRefs: []domain.GraphScopedReference{{Ref: "applicable-conditions", ArtifactRef: conditionRef, AuthorityScope: "receiver", NodeID: &node.ID, TaskVersion: &node.TaskVersion, Paths: []string{}}}, ContributedEvidence: []domain.GraphContributedEvidence{}, ProvidedAccess: []domain.GraphProvidedAccess{{Ref: "graph-instructions", ArtifactRef: shared, Scope: "graph_shared", Paths: []string{}, Mode: "inline"}, {Ref: "node-instructions", ArtifactRef: original, Scope: "receiver", Paths: []string{}, Mode: "inline"}, {Ref: "receiver-acceptance", ArtifactRef: acceptance, Scope: "receiver", Paths: []string{}, Mode: "inline"}, {Ref: "applicable-conditions", ArtifactRef: conditionRef, Scope: "receiver", Paths: []string{}, Mode: "inline"}}}
	for _, c := range op.Contributions {
		packet.ConstraintRefs = append(packet.ConstraintRefs, c.ConstraintRefs...)
		packet.ContributedEvidence = append(packet.ContributedEvidence, c.EvidenceRefs...)
		for _, item := range c.ConstraintRefs {
			if change == "missing_original_access" && item.ArtifactRef == source.OriginalRef {
				continue
			}
			packet.ProvidedAccess = append(packet.ProvidedAccess, domain.GraphProvidedAccess{Ref: item.Ref, ArtifactRef: item.ArtifactRef, Scope: item.AuthorityScope, Paths: item.Paths, Mode: "inline"})
		}
		for _, item := range c.EvidenceRefs {
			packet.ProvidedAccess = append(packet.ProvidedAccess, domain.GraphProvidedAccess{Ref: item.Ref, ArtifactRef: item.ArtifactRef, Scope: item.Scope, Paths: item.Paths, Mode: "inline"})
		}
	}
	packet.PacketSHA256, e = domain.GraphContextDigest(packet)
	if e != nil {
		t.Fatal(e)
	}
	packetRef, cp := graphPayload(t, s, op.Attempt.ID, "graph_context", []byte(encode(packet)))
	input.ContextPacket = packetRef
	sourceManifest, sp := graphPayload(t, s, op.Attempt.ID, "source_manifest", manifestBody)
	snapshot := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: op.Attempt.ID, ManifestArtifactID: sourceManifest.ArtifactID, IndexArtifactID: index.ArtifactID, ManifestSHA256: sourceManifest.SHA256, ProjectIdentity: begin.RootBinding.ProjectIdentity, BaseCommit: begin.RootBinding.BaseCommit, Branch: begin.RootBinding.Branch, SelectedRelativePath: begin.RootBinding.SelectedRelativePath, CreatedAt: domain.Now()}
	if change == "wrong_source_scope" {
		snapshot.SelectedRelativePath = "different-scope"
	}
	return s, op, domain.GraphInputCompletionFacts{InputBinding: &input, SourceSnapshot: &snapshot, ConditionsRef: &conditionRef, ConditionsBinding: &conditions, ContextPacket: &packet, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "ready", VerifiedArtifacts: []domain.VerifiedResponse{mp, ip, bp, pp, cp, sp, conditionProof}}, owner
}

func TestGraphDependentCompletionRetainsActualAssemblySnapshotAndScopedSources(t *testing.T) {
	s, op, f, owner := graphDependentCompletionFixture(t, "")
	ctx := context.Background()
	if e := s.CompleteGraphInput(ctx, owner, op.Attempt.ID, f); e != nil {
		t.Fatal(e)
	}
	a, e := s.Attempt(ctx, op.Attempt.ID)
	if e != nil || a.SourceSnapshotID == nil || *a.SourceSnapshotID != f.SourceSnapshot.ID || a.Outcome == nil || *a.Outcome != "succeeded" || a.ExecutorKind != "local_graph" || a.Role != "graph_input" {
		t.Fatal("actual root-owned assembly snapshot was not committed with input completion", a, e)
	}
	var count int
	if e = s.db.QueryRow("SELECT count(*) FROM source_snapshots WHERE id=? AND attempt_id=?", f.SourceSnapshot.ID, op.Attempt.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal("assembly observation missing", count, e)
	}
}

func TestGraphDependentCompletionRejectsMissingOrWrongScopedSourceAndAccess(t *testing.T) {
	for _, change := range []string{"missing_upstream_source", "wrong_upstream_original", "missing_original_access", "wrong_source_scope"} {
		t.Run(change, func(t *testing.T) {
			s, op, f, owner := graphDependentCompletionFixture(t, change)
			if e := s.CompleteGraphInput(context.Background(), owner, op.Attempt.ID, f); e == nil {
				t.Fatal("dependent completion accepted incomplete or misattributed input", change)
			}
			a, e := s.Attempt(context.Background(), op.Attempt.ID)
			if e != nil || a.Phase == "settled" || a.SourceSnapshotID != nil {
				t.Fatal("refused completion partly committed", a, e)
			}
		})
	}
}
