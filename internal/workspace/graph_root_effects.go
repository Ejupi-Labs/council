package workspace

import (
	"context"
	json "encoding/json/v2"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This records one source-capture observation, not a count of filesystem
// syscalls. A closed in-flight intent remains usable for inspection if the
// separate terminal observation's acknowledgement is lost.
type graphRootOperationEffects struct {
	Contract           string                   `json:"contract"`
	GraphID            string                   `json:"graphId"`
	RootTaskID         string                   `json:"rootTaskId"`
	OperationAttemptID string                   `json:"operationAttemptId"`
	CaptureState       string                   `json:"captureState"`
	SnapshotID         *string                  `json:"snapshotId"`
	Manifest           *domain.GraphArtifactRef `json:"manifest"`
	Index              *domain.GraphArtifactRef `json:"index"`
	StagedBlobs        *domain.GraphArtifactRef `json:"stagedBlobs"`
	EffectCount        string                   `json:"effectCount"`
	EffectsKnown       bool                     `json:"effectsKnown"`
	EvidenceStatus     string                   `json:"evidenceStatus"`
}

func (m *Manager) retainGraphRootEffects(ctx context.Context, attemptID string, record graphRootOperationEffects) (domain.GraphArtifactRef, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	meta, err := m.artifacts.Put(ctx, attemptID, "graph_operation_effects", body)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	return graphIndexRef(meta), nil
}
