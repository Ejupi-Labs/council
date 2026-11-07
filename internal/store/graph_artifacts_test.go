package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphArtifactMetadata(attemptID, kind string) domain.ArtifactRecord {
	id := domain.NewID()
	return domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: id, AttemptID: attemptID, Kind: kind, ByteLength: "12", Closed: true, Completeness: "complete", SHA256: ptr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), CreatedAt: domain.Now()}, PrivateRef: "artifacts/" + id + ".bin", PhysicalIdentity: "typed-fake-artifact"}
}
func TestGraphArtifactKindsRequireRegisteredProducerScope(t *testing.T) {
	s, graph, _, owner := graphLocalFixture(t)
	ctx := context.Background()
	id := graph.PreparationAttemptID
	if _, e := s.ClaimAttempt(ctx, id, owner); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"graph_index_blob", "graph_index_blobs", "graph_operation_effects", "graph_original", "graph_acceptance", "graph_conditions", "graph_input_manifest", "graph_input_provenance", "graph_context"} {
		if e := s.PutArtifact(ctx, graphArtifactMetadata(id, kind)); e != nil {
			t.Fatal("registered real root operation lost its graph artifact kind", kind, e)
		}
	}
	for _, kind := range []string{"graph_materialization", "graph_output_closure", "graph_final_verification", "graph_cumulative_delta"} {
		if e := s.PutArtifact(ctx, graphArtifactMetadata(id, kind)); e == nil {
			t.Fatal("root operation impersonated node execution/output", kind)
		}
	}
	p := openFixture(t, s, "non-graph-artifact-producer")
	in := taskInput(p.ID)
	r := s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	standalone := r.Envelope.Data.(domain.Admission)
	if e := s.PutArtifact(ctx, graphArtifactMetadata(standalone.Attempt.ID, "graph_index_blobs")); e == nil {
		t.Fatal("standalone attempt invented graph membership")
	}
}
