package workspace

import (
	"context"
	json "encoding/json/v2"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphContextRejectsMissingRequiredAccessPlan(t *testing.T) {
	f := graphMaterializeFixture(t)
	for _, test := range []struct {
		name   string
		change func(*testing.T, *GraphInputRetainRequest)
	}{
		{"no_supported_access", func(t *testing.T, r *GraphInputRetainRequest) { r.Context.SupportedAccess = nil }},
		{"receiver_original_uncovered", func(t *testing.T, r *GraphInputRetainRequest) {
			r.Context.SupportedAccess = r.Context.SupportedAccess[:1]
		}},
		{"separate_required_constraint_uncovered", func(t *testing.T, r *GraphInputRetainRequest) {
			meta, err := f.m.artifacts.Put(context.Background(), r.OperationAttemptID, "graph_original", []byte("Separate required graph-wide constraint 東京\r\n"))
			if err != nil {
				t.Fatal(err)
			}
			r.Context.ConstraintRefs = append(r.Context.ConstraintRefs, domain.GraphScopedReference{Ref: "required-separate-constraint", ArtifactRef: graphIndexRef(meta), AuthorityScope: "graph_shared", Paths: []string{}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := graphInitialRetainRequest(t, f)
			test.change(t, &request)
			binding, err := f.m.RetainGraphInput(context.Background(), request)
			if err == nil || binding.InputSHA256 != "" {
				t.Fatal("required original/constraint was published without a complete supported access plan", err)
			}
		})
	}
}

func TestGraphInputVerifierRejectsSelfSealedMissingAccessPlan(t *testing.T) {
	f := graphMaterializeFixture(t)
	verified, err := f.m.VerifyGraphInput(context.Background(), f.input)
	if err != nil {
		t.Fatal("actual positive retained input", err)
	}
	// Synthetic invalid packet, not a qualified historical transition: keeping
	// every byte/ref hash valid cannot substitute for a required access plan.
	packet := verified.ContextPacket
	packet.ProvidedAccess = []domain.GraphProvidedAccess{}
	packet.PacketSHA256, err = domain.GraphContextDigest(packet)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := f.m.artifacts.Put(context.Background(), f.input.OperationAttemptID, "graph_context", body)
	if err != nil {
		t.Fatal(err)
	}
	invalid := f.input
	invalid.ContextPacket = graphIndexRef(meta)
	if _, err = f.m.VerifyGraphInput(context.Background(), invalid); err == nil {
		t.Fatal("self-sealed context without access to required originals was accepted")
	}
}
