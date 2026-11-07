package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

// GraphRootCaptureRequest is an internal operation request. The controller
// registers the actual root operation and supplies current scope/deadline facts.
type GraphRootCaptureRequest struct {
	GraphID, RootTaskID, OperationAttemptID   string
	ProjectID, ProjectPath, SelectedSubfolder string
	ProjectIdentity, RepositoryIdentity       string
}

type GraphRootCapture struct {
	OperationEffects *domain.GraphArtifactRef
	EffectCount      string
	EffectsKnown     bool
	EvidenceStatus   string
	Binding          domain.GraphRootBinding
	Snapshot         *Snapshot
}

type GraphRootProof struct {
	Artifacts   []domain.GraphArtifactRef
	Manifest    Manifest
	StagedBlobs GraphIndexBlobManifest
}

func (m *Manager) VerifyGraphRoot(ctx context.Context, binding domain.GraphRootBinding, source *Snapshot) (GraphRootProof, error) {
	var result GraphRootProof
	manifest, staged, err := m.graphRootBasis(ctx, binding, source)
	if err != nil {
		return result, err
	}
	refs := []domain.GraphArtifactRef{binding.Manifest, binding.Index, binding.StagedBlobs}
	for _, entry := range manifest.Entries {
		if entry.Type != "file" {
			continue
		}
		meta, err := m.artifacts.VerifyClosed(ctx, entry.ArtifactID)
		if err != nil {
			return result, err
		}
		refs = append(refs, graphIndexRef(meta))
	}
	for _, blob := range staged.Entries {
		refs = append(refs, blob.ArtifactRef)
	}
	seen := map[string]domain.GraphArtifactRef{}
	unique := make([]domain.GraphArtifactRef, 0, len(refs))
	for _, ref := range refs {
		if previous, ok := seen[ref.ArtifactID]; ok {
			if previous != ref {
				return result, evidence.ErrCorrupt
			}
			continue
		}
		seen[ref.ArtifactID] = ref
		unique = append(unique, ref)
	}
	return GraphRootProof{Artifacts: unique, Manifest: manifest, StagedBlobs: staged}, nil
}

func (m *Manager) CaptureGraphRoot(ctx context.Context, request GraphRootCaptureRequest) (result GraphRootCapture, operationErr error) {
	result.EffectsKnown = true
	result.EffectCount = "0"
	result.EvidenceStatus = "complete"
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !validManagedID(request.GraphID) || !validManagedID(request.RootTaskID) || !validManagedID(request.OperationAttemptID) || !validManagedID(request.ProjectID) || request.ProjectIdentity == "" || request.RepositoryIdentity == "" {
		return result, ErrBlocked
	}
	if err := project.ValidateIdentity(request.ProjectPath, request.ProjectIdentity); err != nil {
		return result, err
	}
	inspection, err := project.Inspect(ctx, request.ProjectPath, request.ProjectPath)
	if err != nil {
		return result, err
	}
	if inspection.Identity != request.ProjectIdentity {
		return result, ErrBlocked
	}
	repository := inspection.Observation.RepositoryRoot
	if err = project.ValidateIdentity(repository, request.RepositoryIdentity); err != nil {
		return result, err
	}
	record := graphRootOperationEffects{Contract: "006-root-capture-effects-v1", GraphID: request.GraphID, RootTaskID: request.RootTaskID, OperationAttemptID: request.OperationAttemptID, CaptureState: "in_flight", EffectCount: "0", EffectsKnown: false, EvidenceStatus: "incomplete"}
	intent, intentErr := m.retainGraphRootEffects(ctx, request.OperationAttemptID, record)
	if intentErr != nil {
		result.EvidenceStatus = "uncertain"
		return result, intentErr
	}
	result.OperationEffects = &intent
	defer func() {
		if operationErr != nil {
			if errors.Is(operationErr, evidence.ErrDurability) {
				result.EvidenceStatus = "uncertain"
			} else if result.EvidenceStatus == "complete" {
				result.EvidenceStatus = "incomplete"
			}
		}
		record.EffectCount = result.EffectCount
		record.EffectsKnown = result.EffectsKnown
		record.EvidenceStatus = result.EvidenceStatus
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		ref, saveErr := m.retainGraphRootEffects(settlementCtx, request.OperationAttemptID, record)
		if saveErr != nil {
			result.EvidenceStatus = "uncertain"
			operationErr = errors.Join(operationErr, saveErr)
			return
		}
		result.OperationEffects = &ref
	}()
	result.EffectsKnown = false
	source, err := m.Capture(ctx, Request{AttemptID: request.OperationAttemptID, ProjectPath: inspection.Observation.ResolvedPath, SelectedSubfolder: request.SelectedSubfolder})
	if err != nil {
		record.CaptureState = "uncertain"
		result.EvidenceStatus = "uncertain"
		return result, err
	}
	result.Snapshot = source
	result.EffectsKnown = true
	result.EffectCount = "1"
	record.CaptureState = "complete"
	record.SnapshotID = &source.ID
	if source.RepositoryPath != repository || inspection.Observation.Commit == nil || source.BaseCommit != *inspection.Observation.Commit {
		return result, fmt.Errorf("%w The graph source repository/base changed during capture.", ErrBlocked)
	}
	if inspection.Observation.Branch != nil {
		if source.Branch != "refs/heads/"+*inspection.Observation.Branch {
			return result, fmt.Errorf("%w The graph source branch changed during capture.", ErrBlocked)
		}
	} else if source.Branch != "HEAD" {
		return result, ErrBlocked
	}
	loadRef := func(id, kind string) (domain.GraphArtifactRef, error) {
		meta, err := m.artifacts.VerifyClosed(ctx, id)
		if err != nil {
			return domain.GraphArtifactRef{}, err
		}
		if meta.AttemptID != request.OperationAttemptID || meta.Kind != kind || meta.SHA256 == nil {
			return domain.GraphArtifactRef{}, evidence.ErrCorrupt
		}
		return graphIndexRef(meta), nil
	}
	manifest, err := loadRef(source.ManifestArtifactID, "source_manifest")
	if err != nil {
		return result, err
	}
	record.Manifest = &manifest
	index, err := loadRef(source.IndexArtifactID, "index_snapshot")
	if err != nil {
		return result, err
	}
	record.Index = &index
	staged, err := m.CaptureGraphIndex(ctx, GraphIndexCaptureRequest{ProducerAttemptID: request.OperationAttemptID, Source: source, WorkingManifest: manifest, RawIndex: index})
	if err != nil {
		return result, err
	}
	record.StagedBlobs = &staged.StagedBlobs
	if err = project.ValidateIdentity(inspection.Observation.ResolvedPath, request.ProjectIdentity); err != nil {
		return result, err
	}
	if err = project.ValidateIdentity(repository, request.RepositoryIdentity); err != nil {
		return result, err
	}
	var branch *string
	if inspection.Observation.Branch != nil {
		value := source.Branch
		branch = &value
	}
	binding := domain.GraphRootBinding{GraphID: request.GraphID, RootTaskID: request.RootTaskID, OperationAttemptID: request.OperationAttemptID, SourceSnapshotID: source.ID,
		Manifest: manifest, Index: index, StagedBlobs: staged.StagedBlobs, ProjectID: request.ProjectID,
		ProjectIdentity: request.ProjectIdentity, RepositoryIdentity: request.RepositoryIdentity,
		SelectedRelativePath: source.SelectedSubfolder, BaseCommit: source.BaseCommit, Branch: branch}
	binding.BasisSHA256, err = domain.GraphRootDigest(binding)
	if err != nil {
		return result, err
	}
	result.Binding = binding
	return result, nil
}
