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

func graphMemberSlotsFixture(t *testing.T) (graphMaterializeCase, GraphRootProof, VerifiedGraphInput) {
	t.Helper()
	f := graphMaterializeFixture(t)
	proof, err := f.m.VerifyGraphRoot(context.Background(), f.root.Binding, f.root.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := f.m.RetainGraphInput(context.Background(), graphInitialRetainRequest(t, f))
	if err != nil {
		t.Fatal(err)
	}
	input, err := f.m.VerifyGraphInput(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	return f, proof, input
}

func TestGraphMemberSlotsReadsActualRootAndIndependentInputWithoutChangingBytes(t *testing.T) {
	f, root, input := graphMemberSlotsFixture(t)
	before := append([]byte{}, input.Provenance...)
	for _, basis := range []string{"root", "receiving_input"} {
		t.Run(basis, func(t *testing.T) {
			var selected *VerifiedGraphInput
			if basis == "receiving_input" {
				selected = &input
			}
			slots, err := GraphMemberSlots(f.root.Binding, root, selected)
			if err != nil || len(slots) != len(root.Manifest.Entries)+len(root.Manifest.Index) {
				t.Fatal("The real closed root/input could not provide complete member slots", slots, err)
			}
			working, staged := false, false
			for _, slot := range slots {
				if len(slot.Origins) != 1 || slot.Origins[0].Kind != "root" || slot.Origins[0].Node != nil || slot.Origins[0].RootBasisSHA256 != f.root.Binding.BasisSHA256 || !domain.ValidSHA256(slot.Origins[0].OriginSHA256) {
					t.Fatal("An independent retained member acquired fictional node ancestry", slot)
				}
				if slot.Path != "file.bin" {
					continue
				}
				if slot.Dimension == "working" && slot.Working != nil {
					working = slot.Working.SHA256 == hashBytes(f.working)
				}
				if slot.Dimension == "index" && slot.Index != nil {
					staged = slot.Index.BlobRef.SHA256 == hashBytes(f.staged) && slot.Index.BlobRef.SHA256 != hashBytes(f.working)
				}
			}
			if !working || !staged {
				t.Fatal("Member projection confused actual working bytes and staged object bytes", slots)
			}
		})
	}
	if !bytes.Equal(input.Provenance, before) {
		t.Fatal("Read-only member validation mutated its retained provenance")
	}
	working, err := os.ReadFile(filepath.Join(f.repo, "file.bin"))
	index, indexErr := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || indexErr != nil || !bytes.Equal(working, f.working) || !bytes.Equal(index, f.originalIndex) {
		t.Fatal("Member validation changed actual original files or staging", err, indexErr)
	}
}

// These malformed provenance artifacts are genuinely closed and re-sealed into
// real input/context artifacts. VerifyGraphInput checks their physical bytes;
// member projection must additionally reject their incorrect slot semantics.
func TestGraphMemberSlotsRejectsPhysicallyClosedMalformedProvenance(t *testing.T) {
	f, root, valid := graphMemberSlotsFixture(t)
	for _, kind := range []string{"missing_row", "state_digest", "independent_node_origin"} {
		t.Run(kind, func(t *testing.T) {
			var provenance GraphInputProvenance
			if err := json.Unmarshal(valid.Provenance, &provenance); err != nil || len(provenance.Slots) == 0 {
				t.Fatal("The actual retained provenance fixture is empty", err)
			}
			switch kind {
			case "missing_row":
				provenance.Slots = provenance.Slots[1:]
			case "state_digest":
				provenance.Slots[0].StateSHA256 = strings.Repeat("f", 64)
			case "independent_node_origin":
				provenance.Slots[0].Origins[0].Kind = "node"
				provenance.Slots[0].Origins[0].Node = &domain.GraphContributorBinding{NodeID: composeID(30)}
			}
			binding := valid.Binding
			ref, err := f.m.graphRetainJSON(context.Background(), binding.OperationAttemptID, "graph_input_provenance", provenance)
			if err != nil {
				t.Fatal(err)
			}
			binding.Provenance = ref
			binding.InputSHA256, err = domain.GraphInputDigest(binding)
			if err != nil {
				t.Fatal(err)
			}
			packet := valid.ContextPacket
			packet.InputSHA256 = binding.InputSHA256
			packet.PacketSHA256, err = domain.GraphContextDigest(packet)
			if err != nil {
				t.Fatal(err)
			}
			binding.ContextPacket, err = f.m.graphRetainJSON(context.Background(), binding.OperationAttemptID, "graph_context", packet)
			if err != nil {
				t.Fatal(err)
			}
			physical, err := f.m.VerifyGraphInput(context.Background(), binding)
			if err != nil {
				t.Fatal("The actual closed malformed fixture did not reach the semantic member boundary", err)
			}
			if slots, err := GraphMemberSlots(f.root.Binding, root, &physical); err == nil || slots != nil {
				t.Fatal("A physically closed but semantically invalid provenance became usable member rows", slots, err)
			}
		})
	}
}
