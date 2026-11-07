package workspace

import (
	"context"
	json "encoding/json/v2"
	"reflect"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphOutputProof struct {
	Manifest    Manifest
	RawIndex    []byte
	StagedBlobs GraphIndexBlobManifest
	Artifacts   []domain.GraphArtifactRef
}

// VerifyCapturedGraphOutput verifies only retained output integrity. It does
// not resolve acceptance, inspect mutable workers or authorize any execution.
func (m *Manager) VerifyCapturedGraphOutput(ctx context.Context, captured GraphOutputCapture) (GraphOutputProof, error) {
	var empty GraphOutputProof
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := m.lock.CheckHeld(); err != nil {
		return empty, err
	}
	if invalid := domain.ValidateGraphOutputClosure(captured.Closure); invalid != nil {
		return empty, invalid
	}
	if captured.ClosureArtifact.ProducerAttemptID != captured.Closure.ProducerAttemptID {
		return empty, evidence.ErrCorrupt
	}
	body, err := m.graphIndexLoad(ctx, captured.ClosureArtifact, "graph_output_closure")
	if err != nil {
		return empty, err
	}
	var closure domain.GraphOutputClosure
	if err = json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)); err != nil || !reflect.DeepEqual(closure, captured.Closure) {
		return empty, evidence.ErrCorrupt
	}
	seal, err := domain.GraphOutputDigest(closure)
	if err != nil || seal != closure.ClosureSHA256 {
		return empty, evidence.ErrCorrupt
	}
	for _, ref := range []domain.GraphArtifactRef{closure.Manifest, closure.Index, closure.StagedBlobs} {
		if ref.ProducerAttemptID != closure.ProducerAttemptID {
			return empty, evidence.ErrCorrupt
		}
	}
	if closure.Kind == "candidate" {
		if _, err = m.graphIndexLoad(ctx, closure.Manifest, "candidate_manifest"); err != nil {
			return empty, err
		}
	}
	manifest, raw, err := m.graphIndexBasis(ctx, closure.Manifest, closure.Index)
	if err != nil {
		return empty, err
	}
	// graphVerifyStaged invokes graphWorkingPayloads before it verifies every
	// staged object; both retained byte sets share the combined workspace bound.
	staged, err := m.graphVerifyStaged(ctx, manifest, closure.StagedBlobs)
	if err != nil {
		return empty, err
	}
	refs := []domain.GraphArtifactRef{captured.ClosureArtifact, closure.Manifest, closure.Index, closure.StagedBlobs}
	if closure.ResponseArtifact != nil {
		if closure.ResponseArtifact.ProducerAttemptID != closure.ProducerAttemptID {
			return empty, evidence.ErrCorrupt
		}
		response, err := m.graphIndexLoad(ctx, *closure.ResponseArtifact, "final_response")
		if err != nil {
			return empty, err
		}
		if !utf8.Valid(response) {
			return empty, evidence.ErrCorrupt
		}
		refs = append(refs, *closure.ResponseArtifact)
	}
	payloads, err := m.graphClosurePayloadRefs(ctx, manifest, staged)
	if err != nil {
		return empty, err
	}
	refs, err = m.graphUniqueVerifiedRefs(ctx, append(refs, payloads...))
	if err != nil {
		return empty, err
	}
	return GraphOutputProof{Manifest: manifest, RawIndex: raw, StagedBlobs: staged, Artifacts: refs}, nil
}
