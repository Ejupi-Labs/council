package workspace

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

type GraphMaterializeRequest struct {
	GraphID, ReceivingAttemptID, Role string
	RootBinding                       domain.GraphRootBinding
	RootSource                        *Snapshot
	InputBinding                      domain.GraphInputBinding
	SourceBasis                       domain.GraphSourceBasis
}

type GraphHandoffObservationRequest struct {
	GraphID, ReceivingAttemptID, Role string
	InputBinding                      domain.GraphInputBinding
	SourceBasis                       domain.GraphSourceBasis
	Snapshot                          *Snapshot
}

type GraphMaterialization struct {
	Receipt           domain.GraphMaterializationReceipt
	ReceiptArtifact   domain.GraphArtifactRef
	WorkspaceIdentity string
	Snapshot          *Snapshot
}

type VerifiedGraphInput struct {
	Binding       domain.GraphInputBinding
	Manifest      Manifest
	RawIndex      []byte
	StagedBlobs   GraphIndexBlobManifest
	ContextPacket domain.GraphContextPacket
	Provenance    []byte
	ArtifactRefs  []domain.GraphArtifactRef
}

type MaterializationProof struct {
	Artifacts                                []domain.GraphArtifactRef
	WorkingEntriesSHA256, IndexEntriesSHA256 string
}

// GraphMaterializationEffects is retained operation evidence. Native identity is
// returned separately as private runtime proof and never serialized here.
type GraphMaterializationEffects struct {
	Contract          string                   `json:"contract"`
	WorkspaceID       string                   `json:"workspaceId"`
	Registration      string                   `json:"registration"`
	StagedBlobs       domain.GraphArtifactRef  `json:"stagedBlobs"`
	ObjectEffects     *domain.GraphArtifactRef `json:"objectEffects"`
	ObjectEffectCount string                   `json:"objectEffectCount"`
	EffectsKnown      bool                     `json:"effectsKnown"`
}

func (m *Manager) MaterializeGraphInput(ctx context.Context, request GraphMaterializeRequest) (result GraphMaterialization, operationErr error) {
	if !validManagedID(request.ReceivingAttemptID) || request.Role != "author" || request.SourceBasis.BasisKind != "graph_input" {
		return result, ErrBlocked
	}
	receiptID, err := evidence.NewID()
	if err != nil {
		return result, err
	}
	result.Receipt = domain.GraphMaterializationReceipt{ID: receiptID, GraphID: request.GraphID, InputID: request.InputBinding.ID, InputSHA256: request.InputBinding.InputSHA256,
		OperationAttemptID: request.ReceivingAttemptID, ReceivingAttemptID: request.ReceivingAttemptID, SourceBasis: request.SourceBasis, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", State: "blocked"}
	effects := GraphMaterializationEffects{Contract: "006-materialization-effects-v1", Registration: "not_started", StagedBlobs: request.InputBinding.StagedBlobs, ObjectEffectCount: "0", EffectsKnown: true}
	var stream *evidence.Stream
	phase := "validate_root"
	defer func() {
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if stream != nil {
			body, writeErr := json.Marshal(effects)
			if writeErr == nil {
				_, writeErr = stream.Write(body)
			}
			meta, closeErr := stream.Close(settlementCtx, writeErr == nil)
			if writeErr == nil && closeErr == nil {
				ref := graphIndexRef(meta)
				result.Receipt.OperationEffects = &ref
			} else {
				result.Receipt.EvidenceStatus = "uncertain"
				operationErr = errors.Join(operationErr, writeErr, closeErr)
			}
		}
		result.Receipt.EffectsKnown = effects.EffectsKnown
		if operationErr != nil {
			result.Receipt.State = "blocked"
			if effects.Registration != "not_started" {
				result.Receipt.State = "incomplete"
			}
			if !effects.EffectsKnown || result.Receipt.EvidenceStatus != "complete" {
				result.Receipt.State = "uncertain"
			}
			result.Receipt.Failure = domain.Fail("graph_materialization_failed", "The exact graph input could not be materialized and verified.")
		}
		if operationErr != nil {
			operationErr = fmt.Errorf("Graph materialization %s: %w", phase, operationErr)
		}
		result.Receipt.CompletedAt = domain.Now()
		body, marshalErr := json.Marshal(result.Receipt)
		if marshalErr != nil {
			operationErr = errors.Join(operationErr, marshalErr)
			result.Receipt.EvidenceStatus = "uncertain"
			return
		}
		meta, saveErr := m.artifacts.Put(settlementCtx, request.ReceivingAttemptID, "graph_materialization", body)
		if saveErr != nil {
			result.Receipt.State = "uncertain"
			result.Receipt.EvidenceStatus = "uncertain"
			operationErr = errors.Join(operationErr, saveErr)
			return
		}
		result.ReceiptArtifact = graphIndexRef(meta)
	}()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if request.GraphID != request.RootBinding.GraphID || request.GraphID != request.InputBinding.GraphID || request.RootSource == nil || request.InputBinding.RootBasisSHA256 != request.RootBinding.BasisSHA256 {
		return result, ErrBlocked
	}
	if err = m.graphRootSource(ctx, request.RootBinding, request.RootSource); err != nil {
		return result, err
	}
	phase = "verify_input"
	verified, err := m.VerifyGraphInput(ctx, request.InputBinding)
	if err != nil {
		return result, err
	}
	if err = graphCheckSourceBasis(request.SourceBasis, request.InputBinding, verified.Manifest); err != nil {
		return result, err
	}
	id, err := evidence.NewID()
	if err != nil {
		return result, err
	}
	effects.WorkspaceID = id
	stream, err = m.artifacts.Create(ctx, request.ReceivingAttemptID, "graph_operation_effects", MaxFileBytes)
	if err != nil {
		result.Receipt.EvidenceStatus = "uncertain"
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	dir := "workspaces/" + id
	if err = m.privateDir(dir); err != nil {
		return result, err
	}
	destination := filepath.Join(m.lock.Root(), filepath.FromSlash(dir))
	effects.Registration = "in_flight"
	effects.EffectsKnown = false
	if _, err = m.git(ctx, request.RootSource.RepositoryPath, nil, "worktree", "add", "--no-checkout", "--detach", destination, request.RootBinding.BaseCommit); err != nil {
		return result, err
	}
	effects.Registration = "registered"
	effects.EffectsKnown = true
	result.Receipt.EffectCount = "1"
	root, err := m.root.OpenRoot(dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	control, err := root.ReadFile(".git")
	if err != nil || len(control) > 4096 {
		return result, ErrBlocked
	}
	source := &Snapshot{ID: id, AttemptID: request.ReceivingAttemptID, ProjectPath: request.RootSource.ProjectPath, RepositoryPath: request.RootSource.RepositoryPath,
		SelectedSubfolder: request.RootSource.SelectedSubfolder, WorkspacePath: destination, ExecutionPath: filepath.Join(destination, filepath.FromSlash(request.RootSource.SelectedSubfolder)),
		BaseCommit: request.RootBinding.BaseCommit, Branch: request.RootSource.Branch, Manifest: verified.Manifest, GitControlSHA256: hashBytes(control)}
	inspection, err := project.Inspect(ctx, destination, destination)
	if err != nil {
		return result, err
	}
	result.WorkspaceIdentity = inspection.Identity
	phase = "restore_staged"
	objects, err := m.RestoreGraphIndexObjects(ctx, GraphIndexRestoreRequest{OperationAttemptID: request.ReceivingAttemptID, Target: source,
		WorkingManifest: request.InputBinding.Manifest, RawIndex: request.InputBinding.Index, StagedBlobs: request.InputBinding.StagedBlobs})
	effects.ObjectEffects = objects.EffectsArtifact
	effects.ObjectEffectCount = objects.EffectCount
	effects.EffectsKnown = effects.EffectsKnown && objects.EffectsKnown
	if count, countErr := strconv.ParseInt(objects.EffectCount, 10, 64); countErr == nil {
		result.Receipt.EffectCount = strconv.FormatInt(count+1, 10)
	}
	if err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if _, err = m.git(ctx, destination, nil, "read-tree", "--empty"); err != nil {
		return result, err
	}
	var indexInput bytes.Buffer
	for _, entry := range verified.Manifest.Index {
		indexInput.WriteString(entry.Mode + " " + entry.Object + "	" + entry.Path + "\x00")
	}
	if indexInput.Len() != 0 {
		if _, err = m.git(ctx, destination, indexInput.Bytes(), "update-index", "-z", "--index-info"); err != nil {
			return result, err
		}
	}
	index, err := m.index(ctx, destination)
	if err != nil || !indexEqual(index, verified.Manifest.Index) {
		return result, fmt.Errorf("%w Rebuilt graph staging differs.", ErrBlocked)
	}
	phase = "materialize_working"
	if err = m.materialize(ctx, root, verified.Manifest); err != nil {
		return result, err
	}
	actual, err := m.inventory(ctx, request.ReceivingAttemptID, destination, root, index, false)
	actual.Exclusions = verified.Manifest.Exclusions
	if err != nil || !sameManifest(actual, verified.Manifest) {
		return result, fmt.Errorf("%w Actual graph input working bytes differ.", ErrBlocked)
	}
	if err = project.ValidateIdentity(destination, result.WorkspaceIdentity); err != nil {
		return result, err
	}
	body, err := json.Marshal(verified.Manifest)
	if err != nil {
		return result, err
	}
	phase = "retain_receiving_source"
	manifestMeta, err := m.artifacts.Put(ctx, request.ReceivingAttemptID, "source_manifest", body)
	if err != nil {
		result.Receipt.EvidenceStatus = "uncertain"
		return result, err
	}
	source.ManifestArtifactID = manifestMeta.ID
	indexMeta, err := m.RetainIndex(ctx, request.ReceivingAttemptID, source)
	if err != nil {
		result.Receipt.EvidenceStatus = "uncertain"
		return result, err
	}
	source.IndexArtifactID = indexMeta.ID
	manifestRef, indexRef := graphIndexRef(manifestMeta), graphIndexRef(indexMeta)
	workingSHA, indexSHA, err := GraphManifestDigests(actual)
	if err != nil {
		return result, err
	}
	if workingSHA != request.SourceBasis.ExpectedWorkingEntriesSHA256 || indexSHA != request.SourceBasis.ExpectedIndexEntriesSHA256 {
		return result, ErrBlocked
	}
	result.Snapshot = source
	result.Receipt.SourceSnapshotID = &source.ID
	result.Receipt.ObservedManifest = &manifestRef
	result.Receipt.ObservedIndex = &indexRef
	result.Receipt.ObservedWorkingEntriesSHA256 = &workingSHA
	result.Receipt.ObservedIndexEntriesSHA256 = &indexSHA
	stagedSHA := request.InputBinding.StagedBlobs.SHA256
	result.Receipt.StagedBlobsSHA256 = &stagedSHA
	result.Receipt.State = "complete"
	return result, nil
}

func (m *Manager) graphRootSource(ctx context.Context, binding domain.GraphRootBinding, source *Snapshot) error {
	_, _, err := m.graphRootBasis(ctx, binding, source)
	return err
}

// graphRootBasis returns only this call's freshly verified inventories. Reusing
// them within the caller avoids a duplicate immediate pass, never a later proof.
func (m *Manager) graphRootBasis(ctx context.Context, binding domain.GraphRootBinding, source *Snapshot) (Manifest, GraphIndexBlobManifest, error) {
	var manifest Manifest
	var staged GraphIndexBlobManifest
	if valueErr := domain.ValidateGraphRootBinding(binding); valueErr != nil {
		return manifest, staged, valueErr
	}
	seal, err := domain.GraphRootDigest(binding)
	if err != nil || seal != binding.BasisSHA256 || source == nil || source.ID != binding.SourceSnapshotID || source.AttemptID != binding.OperationAttemptID ||
		source.ManifestArtifactID != binding.Manifest.ArtifactID || source.IndexArtifactID != binding.Index.ArtifactID ||
		source.SelectedSubfolder != binding.SelectedRelativePath || source.BaseCommit != binding.BaseCommit {
		return manifest, staged, ErrBlocked
	}
	if err = project.ValidateIdentity(source.ProjectPath, binding.ProjectIdentity); err != nil {
		return manifest, staged, err
	}
	if err = project.ValidateIdentity(source.RepositoryPath, binding.RepositoryIdentity); err != nil {
		return manifest, staged, err
	}
	base, err := m.value(ctx, source.RepositoryPath, "cat-file", "-t", binding.BaseCommit)
	if err != nil || base != "commit" {
		return manifest, staged, ErrBlocked
	}
	manifest, _, err = m.graphIndexBasis(ctx, binding.Manifest, binding.Index)
	if err != nil {
		return manifest, staged, err
	}
	staged, err = m.graphVerifyStaged(ctx, manifest, binding.StagedBlobs)
	return manifest, staged, err
}
