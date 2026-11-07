package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type graphMaterializeCase struct {
	m                              *Manager
	repo                           string
	root                           GraphRootCapture
	input                          domain.GraphInputBinding
	request                        GraphMaterializeRequest
	working, staged, originalIndex []byte
}

func graphMaterializeFixture(t *testing.T) graphMaterializeCase {
	t.Helper()
	m, repo, _ := fixture(t)
	staged := []byte("\xef\xbb\xbfgraph input staged 東京\r\n\x00\xfe")
	working := []byte("graph input working🙂\r\n\x00\xff")
	put(t, repo, "file.bin", staged)
	runGit(t, repo, nil, "add", "file.bin")
	put(t, repo, "file.bin", working)
	put(t, repo, "unchanged.txt", []byte("closed unchanged member\r\n"))
	rawIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := m.CaptureGraphRoot(context.Background(), graphRootRequest(t, repo, repo))
	if err != nil {
		t.Fatal("actual materialization fixture root", err)
	}
	putArtifact := func(kind string, body []byte) domain.GraphArtifactRef {
		meta, err := m.artifacts.Put(context.Background(), root.Binding.OperationAttemptID, kind, body)
		if err != nil {
			t.Fatal(err)
		}
		return graphIndexRef(meta)
	}
	var manifestBody bytes.Buffer
	if _, err = m.artifacts.CopyTo(context.Background(), root.Binding.Manifest.ArtifactID, &manifestBody, MaxFileBytes); err != nil {
		t.Fatal(err)
	}
	// This is a root-node input: composition copies the actual root state, not an
	// authored candidate. Production Store authorization is outside this fixture.
	input := domain.GraphInputBinding{ID: strings.Repeat("5", 32), GraphID: root.Binding.GraphID, GraphVersion: "1", RootBasisSHA256: root.Binding.BasisSHA256,
		OperationAttemptID: root.Binding.OperationAttemptID, ReceivingNodeID: strings.Repeat("b", 32), ReceivingTaskID: strings.Repeat("6", 32), ReceivingTaskVersion: "1",
		Contributions: []domain.AcceptedContribution{}, ContributorVector: []domain.GraphContributorBinding{},
		Manifest: putArtifact("graph_input_manifest", manifestBody.Bytes()), Index: root.Binding.Index, StagedBlobs: root.Binding.StagedBlobs,
		Provenance: putArtifact("graph_input_provenance", []byte(`{"contract":"006-root-input-provenance-v1","rootBasisSha256":"`+root.Binding.BasisSHA256+`"}`))}
	input.InputSHA256, err = domain.GraphInputDigest(input)
	if err != nil {
		t.Fatal(err)
	}
	instructions := putArtifact("graph_original", []byte("Controlled shared graph instruction.\r\n"))
	packet := domain.GraphContextPacket{ID: strings.Repeat("7", 32), GraphID: input.GraphID, InputID: input.ID, InputSHA256: input.InputSHA256, ReceivingTaskVersion: "1",
		GraphInstructionsRef: instructions, NodeInstructionsRef: instructions, ConstraintRefs: []domain.GraphScopedReference{}, ContributedEvidence: []domain.GraphContributedEvidence{},
		ProvidedAccess: []domain.GraphProvidedAccess{{Ref: "graph-instructions", ArtifactRef: instructions, Scope: "whole_original", Paths: []string{}, Mode: "file"}}}
	packet.PacketSHA256, err = domain.GraphContextDigest(packet)
	if err != nil {
		t.Fatal(err)
	}
	packetBody, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	input.ContextPacket = putArtifact("graph_context", packetBody)
	workingSHA, indexSHA := graphMaterializeExpectedDigests(t, root.Snapshot.Manifest)
	basis := domain.GraphSourceBasis{BasisKind: "graph_input", WorkflowID: strings.Repeat("8", 32), StepID: strings.Repeat("9", 32), InputID: input.ID, InputSHA256: input.InputSHA256,
		ExpectedManifest: input.Manifest, ExpectedIndex: input.Index, ExpectedWorkingEntriesSHA256: workingSHA, ExpectedIndexEntriesSHA256: indexSHA}
	request := GraphMaterializeRequest{GraphID: input.GraphID, ReceivingAttemptID: strings.Repeat("a", 32), Role: "author", RootBinding: root.Binding, RootSource: root.Snapshot, InputBinding: input, SourceBasis: basis}
	return graphMaterializeCase{m: m, repo: repo, root: root, input: input, request: request, working: working, staged: staged, originalIndex: rawIndex}
}

// Fixed wire tuples make the filesystem test's expected semantic basis independent
// of the reducer under test; artifact IDs/producer ownership are intentionally absent.
func graphMaterializeExpectedDigests(t *testing.T, manifest Manifest) (string, string) {
	t.Helper()
	type member struct {
		Path   string `json:"path"`
		Type   string `json:"type"`
		Mode   uint32 `json:"mode"`
		Size   int64  `json:"size"`
		SHA256 string `json:"contentSha256"`
		Target string `json:"target"`
	}
	entries := make([]member, 0, len(manifest.Entries))
	for _, e := range manifest.Entries {
		entries = append(entries, member{e.Path, e.Type, e.Mode, e.Size, e.SHA256, e.Target})
	}
	body, err := json.Marshal(struct {
		Entries    []member    `json:"entries"`
		Exclusions []Exclusion `json:"exclusions"`
	}{entries, manifest.Exclusions})
	if err != nil {
		t.Fatal(err)
	}
	index, err := graphIndexDigest(manifest.Index)
	if err != nil {
		t.Fatal(err)
	}
	return hashBytes(body), index
}

func TestGraphMaterializationUsesClosedInputAndDistinctStaging(t *testing.T) {
	f := graphMaterializeFixture(t)
	// Both mutable locations now disagree with the retained input. Neither may be
	// reread as source proof for the receiver.
	put(t, f.repo, "file.bin", []byte("later user working change\r\n"))
	put(t, f.root.Snapshot.WorkspacePath, "file.bin", []byte("later old worker change\r\n"))
	result, err := f.m.MaterializeGraphInput(context.Background(), f.request)
	if err != nil {
		t.Fatal("actual graph input materialization unavailable", err)
	}
	if result.Snapshot == nil || result.Snapshot.AttemptID != f.request.ReceivingAttemptID || result.Snapshot.ID == f.root.Snapshot.ID || result.Receipt.State != "complete" || !result.Receipt.EffectsKnown || result.Receipt.EvidenceStatus != "complete" || result.Receipt.Failure != nil || result.Receipt.ObservedManifest == nil || result.Receipt.ObservedIndex == nil || result.Receipt.OperationEffects == nil || result.Receipt.SourceSnapshotID == nil || *result.Receipt.SourceSnapshotID != result.Snapshot.ID || result.ReceiptArtifact.ProducerAttemptID != f.request.ReceivingAttemptID {
		t.Fatalf("receiving materialization lost actual ownership/closed proof: %+v", result)
	}
	if actual, err := os.ReadFile(filepath.Join(result.Snapshot.WorkspacePath, "file.bin")); err != nil || !bytes.Equal(actual, f.working) {
		t.Fatal("receiver substituted mutable source/old workspace", err)
	}
	if actual := runGit(t, result.Snapshot.WorkspacePath, nil, "show", ":file.bin"); !bytes.Equal(actual, f.staged) {
		t.Fatal("receiver substituted working bytes for staged payload")
	}
	if result.Receipt.ObservedWorkingEntriesSHA256 == nil || *result.Receipt.ObservedWorkingEntriesSHA256 != f.request.SourceBasis.ExpectedWorkingEntriesSHA256 || result.Receipt.ObservedIndexEntriesSHA256 == nil || *result.Receipt.ObservedIndexEntriesSHA256 != f.request.SourceBasis.ExpectedIndexEntriesSHA256 {
		t.Fatal("receiving semantic basis differs")
	}
	if _, err = f.m.RecheckGraphMaterialization(context.Background(), result.Receipt, result.Snapshot, result.WorkspaceIdentity); err != nil {
		t.Fatal("fresh actual receiving proof rejected", err)
	}
	put(t, result.Snapshot.WorkspacePath, "unchanged.txt", []byte("changed before GO\r\n"))
	if _, err = f.m.RecheckGraphMaterialization(context.Background(), result.Receipt, result.Snapshot, result.WorkspaceIdentity); err == nil {
		t.Fatal("actual receiving working-tree mutation was ignored")
	}
	if index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(index, f.originalIndex) {
		t.Fatal("graph materialization changed user raw index", err)
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, "file.bin")); err != nil || string(actual) != "later user working change\r\n" {
		t.Fatal("graph materialization changed later user edit", err)
	}
}

func TestGraphMaterializationRejectsOriginalInputResetForLaterRole(t *testing.T) {
	f := graphMaterializeFixture(t)
	f.request.Role = "correction"
	result, err := f.m.MaterializeGraphInput(context.Background(), f.request)
	if err == nil || result.Snapshot != nil {
		t.Fatal("correction was reset to original graph input", err)
	}
}

func TestGraphMaterializationRejectsCorruptUnchangedClosedMember(t *testing.T) {
	f := graphMaterializeFixture(t)
	var artifact string
	for _, entry := range f.root.Snapshot.Manifest.Entries {
		if entry.Path == "unchanged.txt" {
			artifact = entry.ArtifactID
		}
	}
	meta, err := f.m.artifacts.VerifyClosed(context.Background(), artifact)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(f.m.lock.Root(), filepath.FromSlash(meta.RelativePath)), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteAt([]byte("X"), 0)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	result, err := f.m.MaterializeGraphInput(context.Background(), f.request)
	if err == nil || result.Snapshot != nil {
		t.Fatal("corrupt unchanged member was accepted from valid top-level manifests", err)
	}
}
