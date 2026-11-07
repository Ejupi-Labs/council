package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphInitialRetainRequest(t *testing.T, f graphMaterializeCase) GraphInputRetainRequest {
	t.Helper()
	operation := strings.Repeat("c", 32)
	put := func(kind string, body []byte) domain.GraphArtifactRef {
		meta, err := f.m.artifacts.Put(context.Background(), operation, kind, body)
		if err != nil {
			t.Fatal(err)
		}
		return graphIndexRef(meta)
	}
	shared := put("graph_original", []byte("\xef\xbb\xbfOnly shared instructions 東京\r\n"))
	node := put("graph_original", []byte("Exact receiver original🙂\r\n"))
	inputID := strings.Repeat("d", 32)
	return GraphInputRetainRequest{GraphID: f.root.Binding.GraphID, GraphVersion: "1", RootTaskID: f.root.Binding.RootTaskID, OperationAttemptID: operation, InputID: inputID,
		ReceivingNodeID: strings.Repeat("b", 32), ReceivingTaskID: strings.Repeat("6", 32), ReceivingTaskVersion: "1", RootBinding: f.root.Binding, RootSource: f.root.Snapshot,
		Context: GraphContextRequest{GraphID: f.root.Binding.GraphID, InputID: inputID, ReceivingNodeID: strings.Repeat("b", 32), ReceivingTaskVersion: "1", GraphInstructionsRef: shared, NodeInstructionsRef: node,
			ConstraintRefs:      []domain.GraphScopedReference{{Ref: "shared-original", ArtifactRef: shared, AuthorityScope: "graph_shared", Paths: []string{}}},
			ContributedEvidence: []domain.GraphContributedEvidence{}, SupportedAccess: []domain.GraphProvidedAccess{{Ref: "shared-original", ArtifactRef: shared, Scope: "whole_original", Paths: []string{}, Mode: "file"}, {Ref: "node-original", ArtifactRef: node, Scope: "whole_original", Paths: []string{}, Mode: "file"}}}}
}

func TestGraphInitialInputRetainsClosedRootAndFullRootProvenance(t *testing.T) {
	f := graphMaterializeFixture(t)
	request := graphInitialRetainRequest(t, f)
	put(t, f.repo, "file.bin", []byte("later user bytes\r\n"))
	put(t, f.root.Snapshot.WorkspacePath, "file.bin", []byte("later root workspace bytes\r\n"))
	binding, err := f.m.RetainGraphInput(context.Background(), request)
	if err != nil {
		t.Fatal("actual independent initial-input retention unavailable", err)
	}
	if binding.ID != request.InputID || binding.OperationAttemptID != request.OperationAttemptID || binding.RootBasisSHA256 != f.root.Binding.BasisSHA256 || binding.ReceivingNodeID != request.ReceivingNodeID || binding.ReceivingTaskID != request.ReceivingTaskID || binding.ReceivingTaskVersion != "1" || len(binding.Contributions) != 0 || len(binding.ContributorVector) != 0 {
		t.Fatalf("input invented/dropped root/receiver/contribution facts: %+v", binding)
	}
	for _, ref := range []domain.GraphArtifactRef{binding.Manifest, binding.Index, binding.StagedBlobs, binding.Provenance, binding.ContextPacket} {
		if ref.ProducerAttemptID != request.OperationAttemptID {
			t.Fatal("input top-level artifact relabelled an earlier producer")
		}
	}
	verified, err := f.m.VerifyGraphInput(context.Background(), binding)
	if err != nil {
		t.Fatal("actual retained input closure unavailable", err)
	}
	if !sameManifest(verified.Manifest, f.root.Snapshot.Manifest) || !bytes.Equal(verified.RawIndex, f.originalIndex) {
		t.Fatal("initial input followed mutable source or changed exact root raw index")
	}
	var provenance GraphInputProvenance
	if err = json.Unmarshal(verified.Provenance, &provenance, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	if provenance.Contract != "006-input-provenance-v1" || provenance.RootBasisSHA256 != f.root.Binding.BasisSHA256 || len(provenance.Slots) != len(verified.Manifest.Entries)+len(verified.Manifest.Index) {
		t.Fatal("root provenance omitted complete working/index states")
	}
	previous := ""
	for _, slot := range provenance.Slots {
		key := slot.Dimension + ":" + slot.Path
		if key <= previous || slot.Absent || !domain.ValidSHA256(slot.StateSHA256) || len(slot.Origins) != 1 || slot.Origins[0].Kind != "root" || slot.Origins[0].RootBasisSHA256 != f.root.Binding.BasisSHA256 || slot.Origins[0].Node != nil || !domain.ValidSHA256(slot.Origins[0].OriginSHA256) {
			t.Fatalf("invalid root-origin state %+v", slot)
		}
		if slot.Dimension == "working" && (slot.Working == nil || slot.Index != nil) || slot.Dimension == "index" && (slot.Index == nil || slot.Working != nil) {
			t.Fatal("provenance dimension lost complete state")
		}
		previous = key
	}
	if verified.ContextPacket.GraphInstructionsRef != request.Context.GraphInstructionsRef || verified.ContextPacket.NodeInstructionsRef != request.Context.NodeInstructionsRef || verified.ContextPacket.InputSHA256 != binding.InputSHA256 || len(verified.ContextPacket.ConstraintRefs) != 1 {
		t.Fatal("initial context lost exact shared/receiver source scopes")
	}
	for _, ref := range []domain.GraphArtifactRef{request.Context.GraphInstructionsRef, request.Context.NodeInstructionsRef} {
		var actual bytes.Buffer
		if _, err = f.m.artifacts.CopyTo(context.Background(), ref.ArtifactID, &actual, MaxFileBytes); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(actual.Bytes(), []byte("\r\n")) {
			t.Fatal("original context bytes were normalized")
		}
	}
	if actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index")); err != nil || !bytes.Equal(actual, f.originalIndex) {
		t.Fatal("input retention changed user index", err)
	}
}

func TestGraphInitialInputRejectsMissingRequiredOriginal(t *testing.T) {
	f := graphMaterializeFixture(t)
	request := graphInitialRetainRequest(t, f)
	request.Context.NodeInstructionsRef.ArtifactID = strings.Repeat("f", 32)
	binding, err := f.m.RetainGraphInput(context.Background(), request)
	if err == nil || binding.InputSHA256 != "" {
		t.Fatal("missing required full original produced usable input", err)
	}
}

func TestGraphInitialInputRejectsOverLimitRequiredAccess(t *testing.T) {
	f := graphMaterializeFixture(t)
	request := graphInitialRetainRequest(t, f)
	request.Context.SupportedAccess = []domain.GraphProvidedAccess{}
	for n := 0; n < 65; n++ {
		request.Context.SupportedAccess = append(request.Context.SupportedAccess, domain.GraphProvidedAccess{Ref: "required-" + strconv.Itoa(n), ArtifactRef: request.Context.NodeInstructionsRef, Scope: "whole_original", Paths: []string{}, Mode: "file"})
	}
	binding, err := f.m.RetainGraphInput(context.Background(), request)
	if err == nil || binding.InputSHA256 != "" {
		t.Fatal("required context access was silently clipped", err)
	}
}
