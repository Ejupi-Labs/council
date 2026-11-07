package workspace

import (
	"context"
	json "encoding/json/v2"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphOutputVerifyRequest struct {
	RootBinding  domain.GraphRootBinding
	Contribution domain.AcceptedContribution
	InputBinding domain.GraphInputBinding
}

// VerifyGraphOutput checks retained physical evidence only. Current acceptance,
// graph membership and dispatch authority remain controller/store guards.
func (m *Manager) VerifyGraphOutput(ctx context.Context, request GraphOutputVerifyRequest) (VerifiedContribution, error) {
	var result VerifiedContribution
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := m.lock.CheckHeld(); err != nil {
		return result, err
	}
	rootManifest, _, err := m.graphIndexBasis(ctx, request.RootBinding.Manifest, request.RootBinding.Index)
	if err != nil {
		return result, err
	}
	rootStaged, err := m.graphVerifyStaged(ctx, rootManifest, request.RootBinding.StagedBlobs)
	if err != nil {
		return result, err
	}
	input, err := m.VerifyGraphInput(ctx, request.InputBinding)
	if err != nil {
		return result, err
	}
	var provenance GraphInputProvenance
	if err = json.Unmarshal(input.Provenance, &provenance, json.RejectUnknownMembers(true)); err != nil {
		return result, evidence.ErrCorrupt
	}
	body, err := m.graphIndexLoad(ctx, request.Contribution.OutputClosureRef, "graph_output_closure")
	if err != nil {
		return result, err
	}
	var closure domain.GraphOutputClosure
	if err = json.Unmarshal(body, &closure, json.RejectUnknownMembers(true)); err != nil {
		return result, evidence.ErrCorrupt
	}
	manifest, _, err := m.graphIndexBasis(ctx, closure.Manifest, closure.Index)
	if err != nil {
		return result, err
	}
	staged, err := m.graphVerifyStaged(ctx, manifest, closure.StagedBlobs)
	if err != nil {
		return result, err
	}
	if closure.Kind == "candidate" {
		if _, err = m.graphIndexLoad(ctx, closure.Manifest, "candidate_manifest"); err != nil {
			return result, err
		}
	}
	if closure.ResponseArtifact != nil {
		if _, err = m.graphIndexLoad(ctx, *closure.ResponseArtifact, "final_response"); err != nil {
			return result, err
		}
	}
	result = VerifiedContribution{Binding: request.Contribution, InputBinding: request.InputBinding, InputManifest: input.Manifest, InputProvenance: provenance,
		OutputClosure: closure, OutputManifest: manifest, OutputStagedBlobs: staged}
	if err = graphComposeValidate(GraphCompositionRequest{InputID: request.InputBinding.ID,
		Root: GraphCompositionRoot{Binding: request.RootBinding, Manifest: rootManifest, StagedBlobs: rootStaged}, Contributions: []VerifiedContribution{result}}); err != nil {
		return VerifiedContribution{}, err
	}
	refs := append([]domain.GraphArtifactRef{}, input.ArtifactRefs...)
	refs = append(refs, request.RootBinding.Manifest, request.RootBinding.Index, request.RootBinding.StagedBlobs, request.Contribution.OutputClosureRef, closure.Manifest, closure.Index, closure.StagedBlobs)
	if closure.ResponseArtifact != nil {
		refs = append(refs, *closure.ResponseArtifact)
	}
	for _, source := range request.Contribution.ConstraintRefs {
		refs = append(refs, source.ArtifactRef)
	}
	for _, source := range request.Contribution.EvidenceRefs {
		refs = append(refs, source.ArtifactRef)
	}
	for _, ancestor := range request.InputBinding.ContributorVector {
		refs = append(refs, ancestor.OutputClosureRef)
	}
	for _, item := range []struct {
		manifest Manifest
		staged   GraphIndexBlobManifest
	}{{rootManifest, rootStaged}, {input.Manifest, input.StagedBlobs}, {manifest, staged}} {
		payloads, loadErr := m.graphClosurePayloadRefs(ctx, item.manifest, item.staged)
		if loadErr != nil {
			return VerifiedContribution{}, loadErr
		}
		refs = append(refs, payloads...)
	}
	result.Artifacts, err = m.graphUniqueVerifiedRefs(ctx, refs)
	if err != nil {
		return VerifiedContribution{}, err
	}
	return result, nil
}

func (m *Manager) graphClosurePayloadRefs(ctx context.Context, manifest Manifest, staged GraphIndexBlobManifest) ([]domain.GraphArtifactRef, error) {
	refs := []domain.GraphArtifactRef{}
	for _, entry := range manifest.Entries {
		if entry.Type != "file" {
			continue
		}
		meta, err := m.artifacts.VerifyClosed(ctx, entry.ArtifactID)
		if err != nil {
			return nil, err
		}
		refs = append(refs, graphIndexRef(meta))
	}
	for _, blob := range staged.Entries {
		refs = append(refs, blob.ArtifactRef)
	}
	return refs, nil
}

// No arbitrary SQL-anchor bound truncates this physical inventory. The caller
// selects bounded immutable top-level anchors only after every payload is read.
func (m *Manager) graphUniqueVerifiedRefs(ctx context.Context, refs []domain.GraphArtifactRef) ([]domain.GraphArtifactRef, error) {
	seen := map[string]domain.GraphArtifactRef{}
	for _, ref := range refs {
		if prior, ok := seen[ref.ArtifactID]; ok {
			if prior != ref {
				return nil, evidence.ErrCorrupt
			}
			continue
		}
		if err := m.graphVerifyRef(ctx, ref); err != nil {
			return nil, err
		}
		seen[ref.ArtifactID] = ref
	}
	result := make([]domain.GraphArtifactRef, 0, len(seen))
	for _, ref := range seen {
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ArtifactID < result[j].ArtifactID })
	return result, nil
}
