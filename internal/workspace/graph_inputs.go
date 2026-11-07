package workspace

import (
	"context"
	json "encoding/json/v2"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphInputRetainRequest struct {
	GraphID, GraphVersion, RootTaskID, OperationAttemptID, InputID string
	ReceivingNodeID, ReceivingTaskID, ReceivingTaskVersion         string
	RootBinding                                                    domain.GraphRootBinding
	RootSource                                                     *Snapshot
	Context                                                        GraphContextRequest
}

type GraphInputProvenance struct {
	Contract        string                `json:"contract"`
	RootBasisSHA256 string                `json:"rootBasisSha256"`
	Slots           []GraphProvenanceSlot `json:"slots"`
}

type GraphProvenanceSlot struct {
	Dimension   string                  `json:"dimension"`
	Path        string                  `json:"path"`
	StateSHA256 string                  `json:"stateSha256"`
	Absent      bool                    `json:"absent"`
	Working     *Entry                  `json:"working"`
	Index       *GraphProvenanceIndex   `json:"index"`
	Origins     []GraphProvenanceOrigin `json:"origins"`
}

type GraphProvenanceIndex struct {
	Mode     string                  `json:"mode"`
	ObjectID string                  `json:"objectId"`
	BlobRef  domain.GraphArtifactRef `json:"blobRef"`
}

type GraphProvenanceOrigin struct {
	Kind            string                          `json:"kind"`
	RootBasisSHA256 string                          `json:"rootBasisSha256"`
	Node            *domain.GraphContributorBinding `json:"node"`
	InputOriginIDs  []string                        `json:"inputOriginIds"`
	OriginSHA256    string                          `json:"originSha256"`
}

func (m *Manager) RetainGraphInput(ctx context.Context, request GraphInputRetainRequest) (domain.GraphInputBinding, error) {
	var result domain.GraphInputBinding
	for _, id := range []string{request.GraphID, request.RootTaskID, request.OperationAttemptID, request.InputID, request.ReceivingNodeID, request.ReceivingTaskID} {
		if !validManagedID(id) {
			return result, ErrBlocked
		}
	}
	if _, valueErr := domain.ParseRevision(request.GraphVersion); valueErr != nil {
		return result, valueErr
	}
	if _, valueErr := domain.ParseRevision(request.ReceivingTaskVersion); valueErr != nil {
		return result, valueErr
	}
	if request.GraphID != request.RootBinding.GraphID || request.RootTaskID != request.RootBinding.RootTaskID || request.Context.GraphID != request.GraphID ||
		request.Context.InputID != request.InputID || request.Context.ReceivingNodeID != request.ReceivingNodeID || request.Context.ReceivingTaskVersion != request.ReceivingTaskVersion {
		return result, ErrBlocked
	}
	if err := m.graphRootSource(ctx, request.RootBinding, request.RootSource); err != nil {
		return result, err
	}
	manifest, _, err := m.graphIndexBasis(ctx, request.RootBinding.Manifest, request.RootBinding.Index)
	if err != nil {
		return result, err
	}
	staged, err := m.graphVerifyStaged(ctx, manifest, request.RootBinding.StagedBlobs)
	if err != nil {
		return result, err
	}
	provenance, err := graphInitialProvenance(request.RootBinding.BasisSHA256, manifest, staged)
	if err != nil {
		return result, err
	}
	provenanceBody, err := json.Marshal(provenance)
	if err != nil || len(provenanceBody) > 64<<20 {
		return result, ErrBlocked
	}
	copyRef := func(ref domain.GraphArtifactRef, sourceKind, targetKind string) (domain.GraphArtifactRef, error) {
		if err := ctx.Err(); err != nil {
			return domain.GraphArtifactRef{}, err
		}
		body, err := m.graphIndexLoad(ctx, ref, sourceKind)
		if err != nil {
			return domain.GraphArtifactRef{}, err
		}
		meta, err := m.artifacts.Put(ctx, request.OperationAttemptID, targetKind, body)
		if err != nil {
			return domain.GraphArtifactRef{}, err
		}
		return graphIndexRef(meta), nil
	}
	manifestRef, err := copyRef(request.RootBinding.Manifest, "source_manifest", "graph_input_manifest")
	if err != nil {
		return result, err
	}
	indexRef, err := copyRef(request.RootBinding.Index, "index_snapshot", "index_snapshot")
	if err != nil {
		return result, err
	}
	stagedRef, err := copyRef(request.RootBinding.StagedBlobs, "graph_index_blobs", "graph_index_blobs")
	if err != nil {
		return result, err
	}
	provenanceMeta, err := m.artifacts.Put(ctx, request.OperationAttemptID, "graph_input_provenance", provenanceBody)
	if err != nil {
		return result, err
	}
	binding := domain.GraphInputBinding{ID: request.InputID, GraphID: request.GraphID, GraphVersion: request.GraphVersion, RootBasisSHA256: request.RootBinding.BasisSHA256, OperationAttemptID: request.OperationAttemptID,
		ReceivingNodeID: request.ReceivingNodeID, ReceivingTaskID: request.ReceivingTaskID, ReceivingTaskVersion: request.ReceivingTaskVersion, Contributions: []domain.AcceptedContribution{}, ContributorVector: []domain.GraphContributorBinding{},
		Manifest: manifestRef, Index: indexRef, StagedBlobs: stagedRef, Provenance: graphIndexRef(provenanceMeta)}
	binding.InputSHA256, err = domain.GraphInputDigest(binding)
	if err != nil {
		return result, err
	}
	contextRequest := request.Context
	if contextRequest.InputSHA256 != "" && contextRequest.InputSHA256 != binding.InputSHA256 {
		return result, ErrBlocked
	}
	contextRequest.InputSHA256 = binding.InputSHA256
	packet, err := m.BuildGraphContext(ctx, contextRequest)
	if err != nil {
		return result, err
	}
	body, err := json.Marshal(packet)
	if err != nil {
		return result, err
	}
	packetMeta, err := m.artifacts.Put(ctx, request.OperationAttemptID, "graph_context", body)
	if err != nil {
		return result, err
	}
	binding.ContextPacket = graphIndexRef(packetMeta)
	if _, err = m.VerifyGraphInput(ctx, binding); err != nil {
		return result, err
	}
	return binding, nil
}

func graphInitialProvenance(rootSHA string, manifest Manifest, staged GraphIndexBlobManifest) (GraphInputProvenance, error) {
	result := GraphInputProvenance{Contract: "006-input-provenance-v1", RootBasisSHA256: rootSHA, Slots: []GraphProvenanceSlot{}}
	blobs := map[string]domain.GraphArtifactRef{}
	for _, entry := range staged.Entries {
		blobs[entry.ObjectID] = entry.ArtifactRef
	}
	appendSlot := func(slot GraphProvenanceSlot) error {
		body, err := json.Marshal(struct {
			Kind      string `json:"kind"`
			Root      string `json:"rootBasisSha256"`
			Dimension string `json:"dimension"`
			Path      string `json:"path"`
			State     string `json:"stateSha256"`
		}{"root", rootSHA, slot.Dimension, slot.Path, slot.StateSHA256})
		if err != nil {
			return err
		}
		slot.Origins = []GraphProvenanceOrigin{{Kind: "root", RootBasisSHA256: rootSHA, InputOriginIDs: []string{}, OriginSHA256: hashBytes(body)}}
		result.Slots = append(result.Slots, slot)
		return nil
	}
	for _, entry := range manifest.Entries {
		state := entry
		state.ArtifactID = ""
		body, err := json.Marshal(state)
		if err != nil {
			return result, err
		}
		value := entry
		if err = appendSlot(GraphProvenanceSlot{Dimension: "working", Path: entry.Path, StateSHA256: hashBytes(body), Working: &value}); err != nil {
			return result, err
		}
	}
	for _, entry := range manifest.Index {
		ref, ok := blobs[entry.Object]
		if !ok {
			return result, evidence.ErrCorrupt
		}
		body, err := json.Marshal(entry)
		if err != nil {
			return result, err
		}
		value := GraphProvenanceIndex{Mode: entry.Mode, ObjectID: entry.Object, BlobRef: ref}
		if err = appendSlot(GraphProvenanceSlot{Dimension: "index", Path: entry.Path, StateSHA256: hashBytes(body), Index: &value}); err != nil {
			return result, err
		}
	}
	sort.Slice(result.Slots, func(i, j int) bool {
		a, b := result.Slots[i], result.Slots[j]
		if a.Dimension != b.Dimension {
			return a.Dimension < b.Dimension
		}
		return a.Path < b.Path
	})
	return result, nil
}
