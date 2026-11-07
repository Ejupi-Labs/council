package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Actual closed physical payloads qualify the copy protocol here. These opaque
// graph/workflow tuples claim no Store acceptance or native model execution.
func graphSupplementContextFixture(t *testing.T) (graphMaterializeCase, GraphMaterialization, GraphContextPrepareRequest) {
	t.Helper()
	f, author, request := graphPreparedContextFixture(t)
	ctx := context.Background()
	candidate, err := f.m.Candidate(ctx, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	index, err := f.m.RetainIndex(ctx, request.ReceivingAttemptID, author.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifest := graphArtifactRef(t, f.m, candidate.ManifestArtifactID)
	indexRef := graphIndexRef(index)
	verified, err := f.m.VerifyGraphInput(ctx, request.InputBinding)
	if err != nil {
		t.Fatal(err)
	}
	conditions := domain.GraphConditionsBinding{GraphID: request.InputBinding.GraphID, GraphVersion: request.InputBinding.GraphVersion, ReceiverTaskID: request.InputBinding.ReceivingTaskID, ReceiverTaskVersion: request.InputBinding.ReceivingTaskVersion,
		Sources: []domain.GraphConditionSource{{Ref: "graph-instructions", AuthorityScope: "graph_shared", OriginalRef: verified.ContextPacket.GraphInstructionsRef}, {Ref: "node-instructions", AuthorityScope: "receiver", NodeID: &request.InputBinding.ReceivingNodeID, TaskVersion: &request.InputBinding.ReceivingTaskVersion, OriginalRef: verified.ContextPacket.NodeInstructionsRef}}}
	conditions.ConditionsSHA256, err = domain.GraphConditionsDigest(conditions)
	if err != nil {
		t.Fatal(err)
	}
	put := func(kind string, value any) domain.GraphArtifactRef {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		meta, err := f.m.artifacts.Put(ctx, request.ReceivingAttemptID, kind, body)
		if err != nil {
			t.Fatal(err)
		}
		return graphIndexRef(meta)
	}
	conditionRef := put("graph_conditions", conditions)
	binding := domain.CandidateBinding{RootSourceSnapshotID: author.Snapshot.ID, SourceSnapshotID: author.Snapshot.ID, CandidateID: candidate.ID, ManifestArtifactID: manifest.ArtifactID, ManifestSHA256: manifest.SHA256, IndexArtifactID: indexRef.ArtifactID, IndexSHA256: indexRef.SHA256, ConditionsSHA256: conditions.ConditionsSHA256}
	output, err := f.m.CaptureGraphOutput(ctx, GraphOutputCaptureRequest{GraphID: request.InputBinding.GraphID, NodeID: request.InputBinding.ReceivingNodeID, ReceivedInputID: request.InputBinding.ID, ProducerAttemptID: request.ReceivingAttemptID,
		Kind: "candidate", Source: author.Snapshot, CandidateBinding: &binding, Manifest: manifest, Index: indexRef})
	if err != nil {
		t.Fatal(err)
	}
	delta := put("graph_cumulative_delta", struct{ Contract string }{"controlled-physical-cumulative-context"})
	final := domain.GraphFinalVerificationBinding{GraphID: request.InputBinding.GraphID, GraphVersion: request.InputBinding.GraphVersion, FinalNodeID: request.InputBinding.ReceivingNodeID, FinalTaskID: request.InputBinding.ReceivingTaskID, FinalTaskVersion: request.InputBinding.ReceivingTaskVersion,
		FinalWorkflowID: request.ReceivingAttemptID, RootBinding: domain.GraphRootPublic(f.root.Binding), FinalCandidateBinding: binding, OutputClosureRef: output.ClosureArtifact, FinalInputID: request.InputBinding.ID, ContributorVector: request.InputBinding.ContributorVector,
		GraphConditionsRef: conditionRef, GraphConditionsSHA256: conditions.ConditionsSHA256, OriginalAccess: verified.ContextPacket.ProvidedAccess, CumulativeDeltaRef: delta}
	final.BindingSHA256, err = domain.GraphFinalVerificationDigest(final)
	if err != nil {
		t.Fatal(err)
	}
	ref := put("graph_final_verification", final)
	var binary domain.GraphArtifactRef
	for _, entry := range f.root.Snapshot.Manifest.Entries {
		if entry.Path == "file.bin" {
			binary = graphArtifactRef(t, f.m, entry.ArtifactID)
		}
	}
	if binary.ArtifactID == "" {
		t.Fatal("Fixture lacks a real retained binary before payload")
	}
	request.FinalVerificationRef = &ref
	request.Supplemental = []GraphContextSupplement{{Ref: "final-verification", ArtifactRef: ref, Scope: "cumulative_final", Paths: []string{}}, {Ref: "final-cumulative-delta", ArtifactRef: delta, Scope: "cumulative_final", Paths: []string{}}, {Ref: "final-before-0", ArtifactRef: binary, Scope: "cumulative_final", Paths: []string{"file.bin"}}}
	return f, author, request
}

func TestGraphPreparedFinalContextCopiesOneDirectoryAndRechecksDescriptor(t *testing.T) {
	f, author, request := graphSupplementContextFixture(t)
	ctx := context.Background()
	input, err := f.m.VerifyGraphInput(ctx, request.InputBinding)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := f.m.PrepareGraphContext(ctx, request)
	defer prepared.Close()
	if err != nil || prepared.State != "complete" || !prepared.EffectsKnown || prepared.EvidenceStatus != "complete" || prepared.EffectsArtifact == nil || prepared.root == nil || prepared.FinalVerificationRef == nil || *prepared.FinalVerificationRef != *request.FinalVerificationRef || prepared.DescriptorPath == "" || prepared.DescriptorIdentity == "" || prepared.DescriptorSHA256 == "" || len(prepared.Supplemental) != len(request.Supplemental) {
		t.Fatal("Final context has no actual complete copied descriptor/holder", prepared, err)
	}
	if !reflect.DeepEqual(prepared.Packet, input.ContextPacket) || len(prepared.Entries) != len(input.ContextPacket.ProvidedAccess) || filepath.Dir(prepared.DescriptorPath) != prepared.Directory || filepath.Dir(prepared.Directory) != filepath.Dir(author.Snapshot.WorkspacePath) {
		t.Fatal("Supplemental context replaced original packet or used another directory")
	}
	body, err := os.ReadFile(prepared.DescriptorPath)
	if err != nil || hashBytes(body) != prepared.DescriptorSHA256 {
		t.Fatal("Private descriptor bytes do not match retained identity/hash", err)
	}
	var descriptor GraphFinalContextDescriptor
	if json.Unmarshal(body, &descriptor, json.RejectUnknownMembers(true)) != nil || descriptor.Contract != "006-final-context-v1" || descriptor.ReceivingAttemptID != request.ReceivingAttemptID || descriptor.InputID != request.InputBinding.ID || descriptor.InputSHA256 != request.InputBinding.InputSHA256 || descriptor.PacketSHA256 != prepared.Packet.PacketSHA256 || descriptor.FinalVerificationRef != *request.FinalVerificationRef || len(descriptor.Access) != len(prepared.Entries)+len(prepared.Supplemental) {
		t.Fatal("Descriptor does not bind complete ordinary and final source access", descriptor)
	}
	for i, access := range descriptor.Access {
		if filepath.Dir(access.Path) != prepared.Directory {
			t.Fatal("Descriptor grants a raw evidence or foreign path", access)
		}
		copied, err := os.ReadFile(access.Path)
		if err != nil || hashBytes(copied) != access.ArtifactRef.SHA256 {
			t.Fatal("Descriptor ordinal path is not the actual complete source bytes", access, err)
		}
		if i >= len(prepared.Entries) && (access.Scope != "cumulative_final" || !reflect.DeepEqual(access.Paths, request.Supplemental[i-len(prepared.Entries)].Paths)) {
			t.Fatal("Final evidence was promoted to original instruction authority", access)
		}
	}
	if _, err = f.m.RecheckGraphContext(ctx, prepared); err != nil {
		t.Fatal("Actual complete final context could not be rechecked", err)
	}
	effects, err := f.m.graphIndexLoad(ctx, *prepared.EffectsArtifact, "graph_operation_effects")
	if err != nil || bytes.Contains(effects, []byte(`"path"`)) || bytes.Contains(effects, []byte(`"directoryIdentity"`)) || bytes.Contains(effects, []byte(`"descriptorPath"`)) {
		t.Fatal("Public operation effects exposed private transport paths/identity", err)
	}
	body[0] ^= 1
	if err = os.WriteFile(prepared.DescriptorPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.m.RecheckGraphContext(ctx, prepared); err == nil {
		t.Fatal("A changed descriptor survived the final pre-launch proof")
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("Supplemental transport changed the user's exact index", err)
	}
}

func TestGraphPreparedFinalContextRejectsCombinedReferenceOverflowBeforeEffects(t *testing.T) {
	f, _, request := graphSupplementContextFixture(t)
	request.OtherReferenceCount = 64
	prepared, err := f.m.PrepareGraphContext(context.Background(), request)
	defer prepared.Close()
	if err == nil || domain.SafeError(err).Code != "context_capacity" || prepared.Directory != "" || prepared.EffectsArtifact != nil {
		t.Fatal("Combined real role and graph references did not block before copying", prepared, err)
	}
	path := filepath.Join(f.m.lock.Root(), "workspaces", "graph-context-"+request.ReceivingAttemptID)
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Overflow created a context directory before its explicit capacity refusal", err)
	}
}
