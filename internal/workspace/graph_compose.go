package workspace

import (
	json "encoding/json/v2"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are already physically verified facts, not public artifact selectors or
// proof that an acceptance is current. Controller/Store guards own eligibility.
type GraphCompositionRoot struct {
	Binding     domain.GraphRootBinding
	Manifest    Manifest
	StagedBlobs GraphIndexBlobManifest
}

type VerifiedContribution struct {
	Binding           domain.AcceptedContribution
	InputBinding      domain.GraphInputBinding
	InputManifest     Manifest
	InputProvenance   GraphInputProvenance
	OutputClosure     domain.GraphOutputClosure
	OutputManifest    Manifest
	OutputStagedBlobs GraphIndexBlobManifest
	Artifacts         []domain.GraphArtifactRef // Private full physical-closure inventory.
}

type GraphCompositionRequest struct {
	InputID       string
	Root          GraphCompositionRoot
	Contributions []VerifiedContribution
	CaseSensitive bool
}

type CompositionPlan struct {
	RootBasisSHA256   string
	Manifest          Manifest
	StagedBlobs       GraphIndexBlobManifest
	Provenance        GraphInputProvenance
	Contributions     []domain.AcceptedContribution
	ContributorVector []domain.GraphContributorBinding
}

type InputConflict struct {
	Code             string                  `json:"code"`
	Dimension        string                  `json:"dimension"`
	Path             string                  `json:"path"`
	ContributionRefs []GraphProvenanceOrigin `json:"contributionRefs"`
	InputID          string                  `json:"inputId"`
}

func ComposeGraphInput(request GraphCompositionRequest) (CompositionPlan, []InputConflict, error) {
	var empty CompositionPlan
	if err := graphComposeValidate(request); err != nil {
		return empty, nil, err
	}
	inputs := append([]VerifiedContribution{}, request.Contributions...)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Binding.EdgeID < inputs[j].Binding.EdgeID })
	rootProvenance, err := graphInitialProvenance(request.Root.Binding.BasisSHA256, request.Root.Manifest, request.Root.StagedBlobs)
	if err != nil {
		return empty, nil, err
	}
	rootStates := graphComposeSlotMap(rootProvenance)
	candidates := map[string][]GraphProvenanceSlot{}
	for key, slot := range rootStates {
		candidates[key] = []GraphProvenanceSlot{slot}
	}
	vector := map[string]domain.GraphContributorBinding{}
	direct := []domain.AcceptedContribution{}
	exclusions := map[string]string{}
	for _, e := range request.Root.Manifest.Exclusions {
		exclusions[e.Path] = e.Reason
	}
	for _, input := range inputs {
		direct = append(direct, input.Binding)
		for _, ancestor := range input.InputBinding.ContributorVector {
			if err = graphComposeAddContributor(vector, ancestor); err != nil {
				return empty, nil, err
			}
		}
		current := graphComposeContributor(input.Binding)
		if err = graphComposeAddContributor(vector, current); err != nil {
			return empty, nil, err
		}
		if input.Binding.EdgeKind == "context" {
			continue
		}
		for _, e := range input.OutputManifest.Exclusions {
			if prior, ok := exclusions[e.Path]; ok && prior != e.Reason {
				return empty, nil, ErrBlocked
			}
			exclusions[e.Path] = e.Reason
		}
		before := graphComposeSlotMap(input.InputProvenance)
		after, err := graphComposeManifestStates(input.OutputManifest, input.OutputStagedBlobs)
		if err != nil {
			return empty, nil, err
		}
		keys := map[string]bool{}
		for key := range before {
			keys[key] = true
		}
		for key := range after {
			keys[key] = true
		}
		for key := range rootStates {
			keys[key] = true
		}
		for key := range keys {
			old, ok := before[key]
			if !ok {
				old = graphComposeRootAbsent(request.Root.Binding.BasisSHA256, key)
			}
			next, ok := after[key]
			if !ok {
				next = graphComposeAbsent(key)
			}
			if graphComposeStatesEqual(old, next) {
				next.Origins = graphComposeCopyOrigins(old.Origins)
			} else {
				parents := map[string]bool{}
				for _, origin := range old.Origins {
					parents[origin.OriginSHA256] = true
					for _, id := range origin.InputOriginIDs {
						parents[id] = true
					}
				}
				ids := make([]string, 0, len(parents))
				for id := range parents {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				node := current
				origin := GraphProvenanceOrigin{Kind: "node", RootBasisSHA256: request.Root.Binding.BasisSHA256, Node: &node, InputOriginIDs: ids}
				origin.OriginSHA256, err = graphComposeOriginDigest(origin, next)
				if err != nil {
					return empty, nil, err
				}
				next.Origins = []GraphProvenanceOrigin{origin}
			}
			candidates[key] = append(candidates[key], next)
			if _, ok := rootStates[key]; !ok {
				rootStates[key] = graphComposeRootAbsent(request.Root.Binding.BasisSHA256, key)
				candidates[key] = append(candidates[key], rootStates[key])
			}
		}
	}
	if len(vector) > 16 {
		return empty, nil, ErrBlocked
	}
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selected := []GraphProvenanceSlot{}
	conflicts := []InputConflict{}
	for _, key := range keys {
		slot, heads, conflict, err := graphComposeMaximal(candidates[key])
		if err != nil {
			return empty, nil, err
		}
		if conflict {
			dimension, path := graphComposeSplitKey(key)
			conflicts = append(conflicts, InputConflict{Code: "input_conflict", Dimension: dimension, Path: path, ContributionRefs: heads, InputID: request.InputID})
		} else {
			selected = append(selected, slot)
		}
	}
	if len(conflicts) != 0 {
		if err := graphComposeConflictBounds(conflicts); err != nil {
			return empty, nil, err
		}
		return empty, conflicts, nil
	}
	plan := CompositionPlan{RootBasisSHA256: request.Root.Binding.BasisSHA256, Manifest: Manifest{Entries: []Entry{}, Index: []IndexEntry{}, Exclusions: []Exclusion{}}, StagedBlobs: GraphIndexBlobManifest{Contract: "006-index-blobs-v1", Entries: []GraphIndexBlob{}}, Provenance: GraphInputProvenance{Contract: "006-input-provenance-v1", RootBasisSHA256: request.Root.Binding.BasisSHA256, Slots: selected}, Contributions: direct, ContributorVector: []domain.GraphContributorBinding{}}
	blobs := map[string]GraphIndexBlob{}
	for _, slot := range selected {
		if slot.Absent {
			continue
		}
		if slot.Dimension == "working" {
			plan.Manifest.Entries = append(plan.Manifest.Entries, *slot.Working)
		} else {
			state := slot.Index
			plan.Manifest.Index = append(plan.Manifest.Index, IndexEntry{Path: slot.Path, Mode: state.Mode, Object: state.ObjectID})
			blob := GraphIndexBlob{ObjectID: state.ObjectID, ContentSHA256: state.BlobRef.SHA256, ByteLength: state.BlobRef.ByteLength, ArtifactRef: state.BlobRef}
			if prior, ok := blobs[blob.ObjectID]; ok {
				if prior.ContentSHA256 != blob.ContentSHA256 || prior.ByteLength != blob.ByteLength {
					return empty, nil, ErrBlocked
				}
				if graphComposeRefLess(blob.ArtifactRef, prior.ArtifactRef) {
					blobs[blob.ObjectID] = blob
				}
			} else {
				blobs[blob.ObjectID] = blob
			}
		}
	}
	for path, reason := range exclusions {
		plan.Manifest.Exclusions = append(plan.Manifest.Exclusions, Exclusion{Path: path, Reason: reason})
	}
	sort.Slice(plan.Manifest.Entries, func(i, j int) bool { return plan.Manifest.Entries[i].Path < plan.Manifest.Entries[j].Path })
	sort.Slice(plan.Manifest.Index, func(i, j int) bool { return plan.Manifest.Index[i].Path < plan.Manifest.Index[j].Path })
	sort.Slice(plan.Manifest.Exclusions, func(i, j int) bool { return plan.Manifest.Exclusions[i].Path < plan.Manifest.Exclusions[j].Path })
	for _, blob := range blobs {
		plan.StagedBlobs.Entries = append(plan.StagedBlobs.Entries, blob)
	}
	sort.Slice(plan.StagedBlobs.Entries, func(i, j int) bool {
		return plan.StagedBlobs.Entries[i].ObjectID < plan.StagedBlobs.Entries[j].ObjectID
	})
	for _, v := range vector {
		plan.ContributorVector = append(plan.ContributorVector, v)
	}
	sort.Slice(plan.ContributorVector, func(i, j int) bool { return plan.ContributorVector[i].NodeID < plan.ContributorVector[j].NodeID })
	if err = graphComposeValidatePayloads(plan.Manifest, plan.StagedBlobs); err != nil {
		return empty, nil, err
	}
	conflicts = graphComposeNamespace(request, plan.Provenance.Slots, plan.Manifest)
	if len(conflicts) != 0 {
		if err := graphComposeConflictBounds(conflicts); err != nil {
			return empty, nil, err
		}
		return empty, conflicts, nil
	}
	body, err := json.Marshal(plan.Provenance)
	if err != nil || len(body) > 64<<20 {
		return empty, nil, ErrBlocked
	}
	// A plan must not alias caller-owned origin/binding/path slices or entries.
	body, err = json.Marshal(plan)
	if err != nil {
		return empty, nil, err
	}
	var detached CompositionPlan
	if err = json.Unmarshal(body, &detached); err != nil {
		return empty, nil, err
	}
	return detached, []InputConflict{}, nil
}
