package engine

import (
	"bytes"
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func validGraphArtifactReference(ref domain.GraphArtifactRef) bool {
	_, invalid := domain.ParseCount(ref.ByteLength)
	return invalid == nil && domain.ValidID(ref.ArtifactID) && domain.ValidID(ref.ProducerAttemptID) && domain.ValidSHA256(ref.SHA256)
}

func graphArtifactMatches(meta evidence.Metadata, ref domain.GraphArtifactRef, kind string) bool {
	return meta.ID == ref.ArtifactID && meta.AttemptID == ref.ProducerAttemptID &&
		meta.Kind == kind && meta.Closed && meta.Completeness == "complete" &&
		meta.SHA256 != nil && *meta.SHA256 == ref.SHA256 && meta.ByteLength == ref.ByteLength
}

// This proves exact registered closed evidence, not graph membership, acceptance
// or execution authority. Those bindings must also be checked by the store.
func (e *Engine) verifyGraphArtifact(ctx context.Context, ref domain.GraphArtifactRef, kind string) (evidence.Metadata, error) {
	if err := ctx.Err(); err != nil {
		return evidence.Metadata{}, err
	}
	if !validGraphArtifactReference(ref) || kind == "" {
		return evidence.Metadata{}, evidence.ErrCorrupt
	}
	meta, err := e.evidence.VerifyClosed(ctx, ref.ArtifactID)
	if err != nil {
		return meta, err
	}
	if !graphArtifactMatches(meta, ref, kind) {
		return meta, evidence.ErrCorrupt
	}
	return meta, nil
}

// readGraphArtifact hashes the actual copied bytes in one pass. Failure never
// returns a clipped original, partial payload or bytes from another producer.
func (e *Engine) readGraphArtifact(ctx context.Context, ref domain.GraphArtifactRef, kind string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validGraphArtifactReference(ref) || kind == "" {
		return nil, evidence.ErrCorrupt
	}
	n, _ := domain.ParseCount(ref.ByteLength)
	if maxBytes < 0 || n > maxBytes {
		return nil, evidence.ErrLimit
	}
	var output bytes.Buffer
	meta, err := e.evidence.CopyTo(ctx, ref.ArtifactID, &output, maxBytes)
	if err != nil {
		return nil, err
	}
	if !graphArtifactMatches(meta, ref, kind) {
		return nil, evidence.ErrCorrupt
	}
	return output.Bytes(), nil
}
