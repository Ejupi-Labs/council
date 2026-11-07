package workspace

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

// ObserveGraphHandoff observes an actual005 receiving source; it never invokes
// the original-input seeder or chooses a newer mutable workflow subject.
func (m *Manager) ObserveGraphHandoff(ctx context.Context, request GraphHandoffObservationRequest) (result GraphMaterialization, operationErr error) {
	if !validManagedID(request.ReceivingAttemptID) || request.SourceBasis.BasisKind != "node_subject" || request.Role != "review" && request.Role != "correction" && request.Role != "check" && request.Role != "external_delivery" {
		return result, ErrBlocked
	}
	id, err := evidence.NewID()
	if err != nil {
		return result, err
	}
	result.Receipt = domain.GraphMaterializationReceipt{ID: id, GraphID: request.GraphID, InputID: request.InputBinding.ID, InputSHA256: request.InputBinding.InputSHA256, OperationAttemptID: request.ReceivingAttemptID, ReceivingAttemptID: request.ReceivingAttemptID, SourceBasis: request.SourceBasis, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", State: "blocked"}
	defer func() {
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if operationErr != nil {
			result.Receipt.State = "blocked"
			if result.Receipt.EvidenceStatus != "complete" {
				result.Receipt.State = "uncertain"
			}
			result.Receipt.Failure = domain.Fail("graph_handoff_failed", "The admitted selected node subject could not be verified.")
		}
		result.Receipt.CompletedAt = domain.Now()
		body, err := json.Marshal(result.Receipt)
		if err != nil {
			result.Receipt.EvidenceStatus = "uncertain"
			operationErr = errors.Join(operationErr, err)
			return
		}
		meta, err := m.artifacts.Put(settlementCtx, request.ReceivingAttemptID, "graph_materialization", body)
		if err != nil {
			result.Receipt.State = "uncertain"
			result.Receipt.EvidenceStatus = "uncertain"
			operationErr = errors.Join(operationErr, err)
			return
		}
		result.ReceiptArtifact = graphIndexRef(meta)
	}()
	if request.GraphID != request.InputBinding.GraphID || request.Snapshot == nil || request.Snapshot.AttemptID != request.ReceivingAttemptID {
		return result, ErrBlocked
	}
	if _, err = m.VerifyGraphInput(ctx, request.InputBinding); err != nil {
		return result, err
	}
	source := request.Snapshot
	root, err := m.graphIndexTarget(source)
	if err != nil {
		return result, err
	}
	defer root.Close()
	expected, _, err := m.graphIndexBasis(ctx, request.SourceBasis.ExpectedManifest, request.SourceBasis.ExpectedIndex)
	if err != nil {
		return result, err
	}
	if err = graphCheckSourceBasis(request.SourceBasis, request.InputBinding, expected); err != nil {
		return result, err
	}
	if err = graphCheckSelectedSubject(request.SourceBasis); err != nil {
		return result, err
	}
	manifestRef, err := m.graphRetainedSourceRef(ctx, source.ManifestArtifactID, source.AttemptID, "source_manifest")
	if err != nil {
		return result, err
	}
	indexRef, err := m.graphRetainedSourceRef(ctx, source.IndexArtifactID, source.AttemptID, "index_snapshot")
	if err != nil {
		return result, err
	}
	observed, _, err := m.graphIndexBasis(ctx, manifestRef, indexRef)
	if err != nil || !sameManifest(observed, expected) {
		return result, evidence.ErrCorrupt
	}
	inspection, err := project.Inspect(ctx, source.WorkspacePath, source.WorkspacePath)
	if err != nil {
		return result, err
	}
	result.WorkspaceIdentity = inspection.Identity
	staged, err := m.CaptureGraphIndex(ctx, GraphIndexCaptureRequest{ProducerAttemptID: source.AttemptID, Source: source, WorkingManifest: manifestRef, RawIndex: indexRef})
	if err != nil {
		result.Receipt.EvidenceStatus = "uncertain"
		return result, err
	}
	index, err := m.index(ctx, source.WorkspacePath)
	if err != nil || !indexEqual(index, expected.Index) {
		return result, ErrBlocked
	}
	actual, err := m.inventory(ctx, source.AttemptID, source.WorkspacePath, root, index, false, expected.Entries)
	actual.Exclusions = expected.Exclusions
	if err != nil || !sameManifest(actual, expected) {
		return result, fmt.Errorf("%w Actual selected-subject handoff differs.", ErrBlocked)
	}
	if err = project.ValidateIdentity(source.WorkspacePath, result.WorkspaceIdentity); err != nil {
		return result, err
	}
	stagedManifest, err := m.graphVerifyStaged(ctx, expected, staged.StagedBlobs)
	if err != nil {
		return result, err
	}
	objects := GraphObjectEffects{Contract: "006-graph-object-effects-v1", Effects: []GraphObjectEffect{}, EffectsKnown: true}
	for _, blob := range stagedManifest.Entries {
		objects.Effects = append(objects.Effects, GraphObjectEffect{Kind: "git_object", ObjectID: blob.ObjectID, Action: "already_present", SourceArtifactID: blob.ArtifactRef.ArtifactID, ByteLength: blob.ByteLength, ObservedAt: domain.Now()})
	}
	body, err := json.Marshal(objects)
	if err != nil {
		return result, err
	}
	objectMeta, err := m.artifacts.Put(ctx, source.AttemptID, "graph_operation_effects", body)
	if err != nil {
		result.Receipt.EvidenceStatus = "uncertain"
		return result, err
	}
	objectRef := graphIndexRef(objectMeta)
	effects := GraphMaterializationEffects{Contract: "006-materialization-effects-v1", WorkspaceID: source.ID, Registration: "registered", StagedBlobs: staged.StagedBlobs, ObjectEffects: &objectRef, ObjectEffectCount: strconv.Itoa(len(objects.Effects)), EffectsKnown: true}
	body, err = json.Marshal(effects)
	if err != nil {
		return result, err
	}
	effectsMeta, err := m.artifacts.Put(ctx, source.AttemptID, "graph_operation_effects", body)
	if err != nil {
		result.Receipt.EvidenceStatus = "uncertain"
		return result, err
	}
	effectsRef := graphIndexRef(effectsMeta)
	workingSHA, indexSHA, err := GraphManifestDigests(actual)
	if err != nil {
		return result, err
	}
	result.Snapshot = source
	result.Receipt.SourceSnapshotID = &source.ID
	result.Receipt.ObservedManifest = &manifestRef
	result.Receipt.ObservedIndex = &indexRef
	result.Receipt.ObservedWorkingEntriesSHA256 = &workingSHA
	result.Receipt.ObservedIndexEntriesSHA256 = &indexSHA
	result.Receipt.StagedBlobsSHA256 = &staged.StagedBlobs.SHA256
	result.Receipt.OperationEffects = &effectsRef
	result.Receipt.EffectCount = strconv.Itoa(len(objects.Effects) + 1)
	result.Receipt.State = "complete"
	return result, nil
}

func graphCheckSelectedSubject(basis domain.GraphSourceBasis) error {
	if basis.BasisKind != "node_subject" {
		return ErrBlocked
	}
	manifest, index := basis.ExpectedManifest, basis.ExpectedIndex
	if manifest.ProducerAttemptID != index.ProducerAttemptID {
		return ErrBlocked
	}
	if candidate := basis.CandidateBinding; candidate != nil {
		if basis.ResponseBinding != nil || manifest.ArtifactID != candidate.ManifestArtifactID || manifest.SHA256 != candidate.ManifestSHA256 || index.ArtifactID != candidate.IndexArtifactID || index.SHA256 != candidate.IndexSHA256 {
			return ErrBlocked
		}
		return nil
	}
	if response := basis.ResponseBinding; response != nil {
		if manifest.ProducerAttemptID != response.ProducerAttemptID || manifest.SHA256 != response.ManifestSHA256 || index.ArtifactID != response.IndexArtifactID || index.SHA256 != response.IndexSHA256 {
			return ErrBlocked
		}
		return nil
	}
	return ErrBlocked
}

func (m *Manager) graphRetainedSourceRef(ctx context.Context, id, attempt, kind string) (domain.GraphArtifactRef, error) {
	meta, err := m.artifacts.VerifyClosed(ctx, id)
	if err != nil {
		return domain.GraphArtifactRef{}, err
	}
	if meta.AttemptID != attempt || meta.Kind != kind || meta.SHA256 == nil {
		return domain.GraphArtifactRef{}, evidence.ErrCorrupt
	}
	return graphIndexRef(meta), nil
}
