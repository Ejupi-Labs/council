package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphComposedInputPreservesSelectedContextAndExactAttribution(t *testing.T) {
	f := graphMaterializeFixture(t)
	branch := graphPhysicalContributionFixture(t, f, 1, "context-branch.txt")
	ctx := context.Background()
	put := func(kind, body string) domain.GraphArtifactRef {
		meta, err := f.m.artifacts.Put(ctx, branch.snapshot.AttemptID, kind, []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		return graphIndexRef(meta)
	}
	conditionRef := put("graph_original", "Exact attributed upstream condition.\r\n")
	evidenceRef := put("final_response", "Exact selected upstream explanation 東京\r\n")
	c := &branch.selection.Binding
	nodeID, version := c.FromNodeID, c.TaskVersion
	condition := domain.GraphScopedReference{Ref: "upstream-condition", ArtifactRef: conditionRef, AuthorityScope: "upstream_local", NodeID: &nodeID, TaskVersion: &version, Paths: []string{}}
	evidence := domain.GraphContributedEvidence{Ref: "upstream-explanation", EdgeID: c.EdgeID, SourceNodeID: c.FromNodeID, AcceptanceKind: c.AcceptanceKind, AcceptanceID: c.AcceptanceID, ProducerAttemptID: c.ProducerAttemptID,
		ResultKind: "response", ResultID: evidenceRef.ArtifactID, ArtifactRef: evidenceRef, Scope: "whole_response", Paths: []string{}}
	c.ConstraintRefs = []domain.GraphScopedReference{condition}
	c.EvidenceRefs = []domain.GraphContributedEvidence{evidence}
	request := graphPhysicalComposeRequest(t, f, branch)
	before, err := os.ReadDir(filepath.Join(f.m.lock.Root(), "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	omitted, err := f.m.RetainComposedGraphInput(ctx, request)
	if err == nil || omitted.Binding != nil {
		t.Fatal("Selected upstream context was silently omitted from a complete usable input", err)
	}
	after, err := os.ReadDir(filepath.Join(f.m.lock.Root(), "workspaces"))
	if err != nil || len(after) != len(before) {
		t.Fatal("Missing selected context reached managed workspace creation", err)
	}
	request.Base.Context.ConstraintRefs = append(request.Base.Context.ConstraintRefs, condition)
	request.Base.Context.ContributedEvidence = append(request.Base.Context.ContributedEvidence, evidence)
	request.Base.Context.SupportedAccess = append(request.Base.Context.SupportedAccess,
		domain.GraphProvidedAccess{Ref: condition.Ref, ArtifactRef: conditionRef, Scope: "upstream_local", Paths: []string{}, Mode: "file"},
		domain.GraphProvidedAccess{Ref: evidence.Ref, ArtifactRef: evidenceRef, Scope: evidence.Scope, Paths: []string{}, Mode: "file"})
	retained, err := f.m.RetainComposedGraphInput(ctx, request)
	if err != nil || retained.Binding == nil || retained.State != "complete" {
		t.Fatal("Complete attributed selected context was rejected", err)
	}
	verified, err := f.m.VerifyGraphInput(ctx, *retained.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified.ContextPacket.ConstraintRefs) == 0 || len(verified.ContextPacket.ContributedEvidence) != 1 {
		t.Fatal("Complete context was not retained")
	}
	// These deliberately malformed retained packets test the loaded-input guard;
	// they are not valid controller-authorized historical transitions.
	for _, failure := range []string{"missing-evidence", "changed-authority"} {
		t.Run(failure, func(t *testing.T) {
			packet := verified.ContextPacket
			packet.ConstraintRefs = append([]domain.GraphScopedReference{}, packet.ConstraintRefs...)
			packet.ContributedEvidence = append([]domain.GraphContributedEvidence{}, packet.ContributedEvidence...)
			packet.ProvidedAccess = append([]domain.GraphProvidedAccess{}, packet.ProvidedAccess...)
			if failure == "missing-evidence" {
				packet.ContributedEvidence = []domain.GraphContributedEvidence{}
				kept := []domain.GraphProvidedAccess{}
				for _, access := range packet.ProvidedAccess {
					if access.ArtifactRef != evidenceRef {
						kept = append(kept, access)
					}
				}
				packet.ProvidedAccess = kept
			} else {
				for i := range packet.ConstraintRefs {
					if packet.ConstraintRefs[i].Ref == condition.Ref {
						packet.ConstraintRefs[i].AuthorityScope = "graph_shared"
					}
				}
			}
			packet.PacketSHA256, err = domain.GraphContextDigest(packet)
			if err != nil {
				t.Fatal(err)
			}
			packetRef, err := f.m.graphRetainJSON(ctx, request.Base.OperationAttemptID, "graph_context", packet)
			if err != nil {
				t.Fatal(err)
			}
			binding := *retained.Binding
			binding.ContextPacket = packetRef
			if _, err = f.m.VerifyGraphInput(ctx, binding); err == nil {
				t.Fatal("A self-sealed loaded packet dropped or reattributed selected context", failure)
			}
		})
	}
}
