package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

type graphRootFaultSink struct {
	base *fixtureSink
	kind string
}

func (s *graphRootFaultSink) LoadArtifact(ctx context.Context, id string) (evidence.Metadata, error) {
	return s.base.LoadArtifact(ctx, id)
}
func (s *graphRootFaultSink) SaveArtifact(ctx context.Context, meta evidence.Metadata) error {
	if err := s.base.SaveArtifact(ctx, meta); err != nil {
		return err
	}
	if meta.Kind == s.kind && meta.Closed && meta.Completeness == "complete" {
		return errors.New("controlled acknowledged-write loss")
	}
	return nil
}

func TestGraphRootCaptureRecordsActualOperationEffects(t *testing.T) {
	m, repo, _ := fixture(t)
	request := graphRootRequest(t, repo, repo)
	result, err := m.CaptureGraphRoot(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationEffects == nil || result.OperationEffects.ProducerAttemptID != request.OperationAttemptID || !result.EffectsKnown || result.EvidenceStatus != "complete" || result.EffectCount != "1" {
		t.Fatalf("actual root preparation was claimed effect-free or lacks closed operation evidence: %+v", result)
	}
	body, err := m.graphIndexLoad(context.Background(), *result.OperationEffects, "graph_operation_effects")
	if err != nil {
		t.Fatal(err)
	}
	var effect struct {
		Contract     string                   `json:"contract"`
		CaptureState string                   `json:"captureState"`
		SnapshotID   *string                  `json:"snapshotId"`
		Manifest     *domain.GraphArtifactRef `json:"manifest"`
		Index        *domain.GraphArtifactRef `json:"index"`
		StagedBlobs  *domain.GraphArtifactRef `json:"stagedBlobs"`
		EffectsKnown bool                     `json:"effectsKnown"`
	}
	if err = json.Unmarshal(body, &effect); err != nil || effect.Contract != "006-root-capture-effects-v1" || effect.CaptureState != "complete" || effect.SnapshotID == nil || *effect.SnapshotID != result.Snapshot.ID || !effect.EffectsKnown {
		t.Fatal("root operation record lost actual capture observation", err)
	}
	// The record carries actual registered facts; no fabricated no-effects count.
	if !bytes.Contains(body, []byte(result.Binding.Manifest.ArtifactID)) || !bytes.Contains(body, []byte(result.Binding.Index.ArtifactID)) || !bytes.Contains(body, []byte(result.Binding.StagedBlobs.ArtifactID)) {
		t.Fatal("root effect record omitted exact retained closure refs")
	}
}

func TestGraphRootCaptureRetainsOpaqueCaptureUncertaintyAndPartialSnapshot(t *testing.T) {
	for _, kind := range []string{"source_manifest", "graph_index_blobs"} {
		t.Run(kind, func(t *testing.T) {
			m, repo, lock := fixture(t)
			sink := &graphRootFaultSink{base: &fixtureSink{rows: map[string]evidence.Metadata{}}, kind: kind}
			artifacts, err := evidence.New(lock, sink)
			if err != nil {
				t.Fatal(err)
			}
			defer artifacts.Close()
			m.artifacts = artifacts
			request := graphRootRequest(t, repo, repo)
			before, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := m.CaptureGraphRoot(context.Background(), request)
			if err == nil || result.Binding.SourceSnapshotID != "" || result.OperationEffects == nil || result.EvidenceStatus != "uncertain" {
				t.Fatalf("lost root acknowledgement became no-effects/usable binding: %+v err=%v", result, err)
			}
			if kind == "source_manifest" && result.EffectsKnown {
				t.Fatal("opaque Capture failure claimed known effects")
			}
			if kind == "graph_index_blobs" && (result.Snapshot == nil || result.Snapshot.ManifestArtifactID == "" || result.Snapshot.IndexArtifactID == "") {
				t.Fatal("later closure error discarded actual completed source snapshot")
			}
			entries, readErr := os.ReadDir(filepath.Join(lock.Root(), "workspaces"))
			if readErr != nil || len(entries) != 1 || !entries[0].IsDir() {
				t.Fatal("fixture did not actually create a managed root workspace before lost acknowledgement", readErr)
			}
			if _, readErr = os.ReadFile(filepath.Join(lock.Root(), "workspaces", entries[0].Name(), ".git")); readErr != nil {
				t.Fatal("actual managed Git registration unavailable", readErr)
			}
			if _, verifyErr := artifacts.VerifyClosed(context.Background(), result.OperationEffects.ArtifactID); verifyErr != nil {
				t.Fatal("known root intent/effect record was discarded", verifyErr)
			}
			after, readErr := os.ReadFile(filepath.Join(repo, ".git", "index"))
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("failed root preparation changed original index", readErr)
			}
		})
	}
}
