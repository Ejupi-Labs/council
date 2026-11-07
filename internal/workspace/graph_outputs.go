package workspace

import (
	"context"
	json "encoding/json/v2"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphOutputCaptureRequest struct {
	GraphID, NodeID, ProducerAttemptID, ReceivedInputID, Kind string
	Source                                                    *Snapshot
	CandidateBinding                                          *domain.CandidateBinding
	ResponseBinding                                           *domain.ResponseBinding
	Manifest, Index                                           domain.GraphArtifactRef
	SourceMaterialization                                     *domain.GraphMaterializationReceipt
}

type GraphOutputCapture struct {
	Closure         domain.GraphOutputClosure
	ClosureArtifact domain.GraphArtifactRef
}

func (m *Manager) CaptureGraphOutput(ctx context.Context, request GraphOutputCaptureRequest) (GraphOutputCapture, error) {
	var result GraphOutputCapture
	for _, id := range []string{request.GraphID, request.NodeID, request.ProducerAttemptID, request.ReceivedInputID} {
		if !validManagedID(id) {
			return result, ErrBlocked
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := m.lock.CheckHeld(); err != nil {
		return result, err
	}
	if request.Source == nil || request.Source.AttemptID != request.ProducerAttemptID || request.Manifest.ProducerAttemptID != request.ProducerAttemptID || request.Index.ProducerAttemptID != request.ProducerAttemptID {
		return result, ErrBlocked
	}
	manifest, _, err := m.graphIndexBasis(ctx, request.Manifest, request.Index)
	if err != nil {
		return result, err
	}
	if _, _, err = m.graphWorkingPayloads(ctx, manifest); err != nil {
		return result, err
	}
	closure := domain.GraphOutputClosure{GraphID: request.GraphID, NodeID: request.NodeID, ReceivedInputID: request.ReceivedInputID, Kind: request.Kind, ProducerAttemptID: request.ProducerAttemptID, Manifest: request.Manifest, Index: request.Index}
	switch request.Kind {
	case "candidate":
		binding := request.CandidateBinding
		if binding == nil || request.ResponseBinding != nil || binding.SourceSnapshotID != request.Source.ID || !validManagedID(binding.RootSourceSnapshotID) || !domain.ValidSHA256(binding.ConditionsSHA256) {
			return result, ErrBlocked
		}
		if err = graphCheckSelectedSubject(domain.GraphSourceBasis{BasisKind: "node_subject", CandidateBinding: binding, ExpectedManifest: request.Manifest, ExpectedIndex: request.Index}); err != nil {
			return result, err
		}
		if !validManagedID(binding.CandidateID) {
			return result, ErrBlocked
		}
		meta, err := m.artifacts.VerifyClosed(ctx, request.Manifest.ArtifactID)
		if err != nil || meta.Kind != "candidate_manifest" {
			return result, evidence.ErrCorrupt
		}
		staged, err := m.CaptureGraphIndex(ctx, GraphIndexCaptureRequest{ProducerAttemptID: request.ProducerAttemptID, Source: request.Source, WorkingManifest: request.Manifest, RawIndex: request.Index})
		if err != nil {
			return result, err
		}
		root, err := m.graphIndexTarget(request.Source)
		if err != nil {
			return result, err
		}
		defer root.Close()
		index, err := m.index(ctx, request.Source.WorkspacePath)
		if err != nil || !indexEqual(index, manifest.Index) {
			return result, ErrBlocked
		}
		actual, err := m.inventory(ctx, request.ProducerAttemptID, request.Source.WorkspacePath, root, index, false, manifest.Entries)
		actual.Exclusions = manifest.Exclusions
		if err != nil || !sameManifest(actual, manifest) {
			return result, ErrBlocked
		}
		id := binding.CandidateID
		closure.CandidateID = &id
		closure.StagedBlobs = staged.StagedBlobs
	case "response":
		binding := request.ResponseBinding
		if binding == nil || request.CandidateBinding != nil || binding.SourceSnapshotID != request.Source.ID || binding.ProducerAttemptID != request.ProducerAttemptID || !domain.ValidSHA256(binding.ConditionsSHA256) {
			return result, ErrBlocked
		}
		if err = graphCheckSelectedSubject(domain.GraphSourceBasis{BasisKind: "node_subject", ResponseBinding: binding, ExpectedManifest: request.Manifest, ExpectedIndex: request.Index}); err != nil {
			return result, err
		}
		ref := domain.GraphArtifactRef{ArtifactID: binding.ResponseArtifactID, ProducerAttemptID: binding.ProducerAttemptID, SHA256: binding.ResponseSHA256, ByteLength: binding.ResponseByteLength}
		body, err := m.graphIndexLoad(ctx, ref, "final_response")
		if err != nil {
			return result, err
		}
		if !utf8.Valid(body) {
			return result, evidence.ErrCorrupt
		}
		staged, err := m.graphResponseSourceStaging(ctx, request, manifest)
		if err != nil {
			return result, err
		}
		closure.ResponseArtifact = &ref
		closure.StagedBlobs = staged
	default:
		return result, ErrBlocked
	}
	closure.ClosureSHA256, err = domain.GraphOutputDigest(closure)
	if err != nil {
		return result, err
	}
	if valueErr := domain.ValidateGraphOutputClosure(closure); valueErr != nil {
		return result, valueErr
	}
	body, err := json.Marshal(closure)
	if err != nil {
		return result, err
	}
	if len(body) > 64<<20 {
		return result, ErrBlocked
	}
	meta, err := m.artifacts.Put(ctx, request.ProducerAttemptID, "graph_output_closure", body)
	if err != nil {
		return result, err
	}
	return GraphOutputCapture{Closure: closure, ClosureArtifact: graphIndexRef(meta)}, nil
}

func (m *Manager) graphResponseSourceStaging(ctx context.Context, request GraphOutputCaptureRequest, manifest Manifest) (domain.GraphArtifactRef, error) {
	var zero domain.GraphArtifactRef
	receipt := request.SourceMaterialization
	if receipt == nil {
		return zero, ErrBlocked
	}
	if valueErr := domain.ValidateGraphMaterializationReceipt(*receipt); valueErr != nil {
		return zero, valueErr
	}
	if receipt.State != "complete" || receipt.GraphID != request.GraphID || receipt.InputID != request.ReceivedInputID || receipt.ReceivingAttemptID != request.ProducerAttemptID || receipt.SourceSnapshotID == nil ||
		*receipt.SourceSnapshotID != request.Source.ID || receipt.ObservedManifest == nil || receipt.ObservedIndex == nil || receipt.ObservedWorkingEntriesSHA256 == nil || receipt.ObservedIndexEntriesSHA256 == nil ||
		receipt.OperationEffects == nil || receipt.StagedBlobsSHA256 == nil {
		return zero, ErrBlocked
	}
	if receipt.OperationEffects.ProducerAttemptID != request.ProducerAttemptID || receipt.ObservedManifest.ProducerAttemptID != request.ProducerAttemptID || receipt.ObservedIndex.ProducerAttemptID != request.ProducerAttemptID ||
		receipt.ObservedManifest.ArtifactID != request.Source.ManifestArtifactID || receipt.ObservedIndex.ArtifactID != request.Source.IndexArtifactID {
		return zero, evidence.ErrCorrupt
	}
	// Existing workflow response bindings retain a separately observed after
	// index, even when its semantic entries are unchanged. Preserve that exact
	// acknowledged reference; independently verify the actual pre-command source.
	sourceManifest, _, err := m.graphIndexBasis(ctx, *receipt.ObservedManifest, *receipt.ObservedIndex)
	if err != nil {
		return zero, err
	}
	if !sameManifest(sourceManifest, manifest) {
		return zero, ErrBlocked
	}
	workingSHA, indexSHA, err := GraphManifestDigests(sourceManifest)
	if err != nil || workingSHA != *receipt.ObservedWorkingEntriesSHA256 || indexSHA != *receipt.ObservedIndexEntriesSHA256 ||
		workingSHA != receipt.SourceBasis.ExpectedWorkingEntriesSHA256 || indexSHA != receipt.SourceBasis.ExpectedIndexEntriesSHA256 {
		return zero, evidence.ErrCorrupt
	}
	body, err := m.graphIndexLoad(ctx, *receipt.OperationEffects, "graph_operation_effects")
	if err != nil {
		return zero, err
	}
	var effects GraphMaterializationEffects
	if err = json.Unmarshal(body, &effects, json.RejectUnknownMembers(true)); err != nil || effects.Contract != "006-materialization-effects-v1" || effects.Registration != "registered" || !effects.EffectsKnown || effects.WorkspaceID != request.Source.ID ||
		effects.StagedBlobs.SHA256 != *receipt.StagedBlobsSHA256 || effects.ObjectEffects == nil {
		return zero, evidence.ErrCorrupt
	}
	if effects.ObjectEffects.ProducerAttemptID != request.ProducerAttemptID {
		return zero, evidence.ErrCorrupt
	}
	if err = m.graphVerifyRef(ctx, *effects.ObjectEffects); err != nil {
		return zero, err
	}
	if _, err = m.graphVerifyStaged(ctx, sourceManifest, effects.StagedBlobs); err != nil {
		return zero, err
	}
	// This exact immutable source closure can precede the author's current
	// candidate/index. Never infer source staging from the after-state workspace.
	body, err = m.graphIndexLoad(ctx, effects.StagedBlobs, "graph_index_blobs")
	if err != nil {
		return zero, err
	}
	meta, err := m.artifacts.Put(ctx, request.ProducerAttemptID, "graph_index_blobs", body)
	if err != nil {
		return zero, err
	}
	return graphIndexRef(meta), nil
}
