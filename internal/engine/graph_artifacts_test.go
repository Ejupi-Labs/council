package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

func TestGraphArtifactReadRequiresExactClosedProducerKindAndBytes(t *testing.T) {
	e, admission := watchFixture(t)
	ctx := context.Background()
	payload := []byte("\xef\xbb\xbfexact original 東京\r\n\x00\xff")
	meta, err := e.evidence.Put(ctx, admission.Attempt.ID, "prompt", payload)
	if err != nil || meta.SHA256 == nil {
		t.Fatal(meta, err)
	}
	ref := domain.GraphArtifactRef{ArtifactID: meta.ID, ProducerAttemptID: meta.AttemptID, SHA256: *meta.SHA256, ByteLength: meta.ByteLength}
	got, err := e.readGraphArtifact(ctx, ref, "prompt", 1024)
	if err != nil || string(got) != string(payload) {
		t.Fatal("Exact original bytes unavailable", got, err)
	}
	for name, change := range map[string]func(*domain.GraphArtifactRef){
		"foreign producer":    func(r *domain.GraphArtifactRef) { r.ProducerAttemptID = domain.NewID() },
		"wrong hash":          func(r *domain.GraphArtifactRef) { r.SHA256 = strings.Repeat("0", 64) },
		"wrong length":        func(r *domain.GraphArtifactRef) { r.ByteLength = "1" },
		"noncanonical length": func(r *domain.GraphArtifactRef) { r.ByteLength = "01" },
		"overflow length":     func(r *domain.GraphArtifactRef) { r.ByteLength = "9223372036854775808" },
		"malformed identity":  func(r *domain.GraphArtifactRef) { r.ArtifactID = "../evidence" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := ref
			change(&invalid)
			if bytes, err := e.readGraphArtifact(ctx, invalid, "prompt", 1024); err == nil || bytes != nil {
				t.Fatal("A different producer/hash/length supplied usable graph bytes", bytes, err)
			}
		})
	}
	if bytes, err := e.readGraphArtifact(ctx, ref, "source_manifest", 1024); !errors.Is(err, evidence.ErrCorrupt) || bytes != nil {
		t.Fatal("An unrelated artifact kind supplied graph bytes", bytes, err)
	}
	if bytes, err := e.readGraphArtifact(ctx, ref, "prompt", 1); !errors.Is(err, evidence.ErrLimit) || bytes != nil {
		t.Fatal("An oversized required original was returned or clipped", bytes, err)
	}
	verified, err := e.verifyGraphArtifact(ctx, ref, "prompt")
	if err != nil || verified.ID != meta.ID || verified.AttemptID != admission.Attempt.ID {
		t.Fatal("Exact registered evidence was not verified", verified, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.readGraphArtifact(cancelled, ref, "prompt", 1024); !errors.Is(err, context.Canceled) {
		t.Fatal("Cancelled access started new evidence work", err)
	}
}
