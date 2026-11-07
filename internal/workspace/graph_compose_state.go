package workspace

import (
	json "encoding/json/v2"
	"sort"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphComposeKey(dimension, path string) string { return dimension + ":" + path }
func graphComposeSplitKey(key string) (string, string) {
	dimension, path, _ := strings.Cut(key, ":")
	return dimension, path
}
func graphComposeSlotMap(provenance GraphInputProvenance) map[string]GraphProvenanceSlot {
	out := map[string]GraphProvenanceSlot{}
	for _, slot := range provenance.Slots {
		out[graphComposeKey(slot.Dimension, slot.Path)] = slot
	}
	return out
}

func graphComposeAbsent(key string) GraphProvenanceSlot {
	dimension, path := graphComposeSplitKey(key)
	body, _ := json.Marshal(struct {
		Absent bool `json:"absent"`
	}{true})
	return GraphProvenanceSlot{Dimension: dimension, Path: path, StateSHA256: hashBytes(body), Absent: true, Origins: []GraphProvenanceOrigin{}}
}
func graphComposeRootAbsent(rootSHA, key string) GraphProvenanceSlot {
	slot := graphComposeAbsent(key)
	origin := GraphProvenanceOrigin{Kind: "root", RootBasisSHA256: rootSHA, InputOriginIDs: []string{}}
	origin.OriginSHA256, _ = graphComposeOriginDigest(origin, slot)
	slot.Origins = []GraphProvenanceOrigin{origin}
	return slot
}

func graphComposeManifestStates(manifest Manifest, staged GraphIndexBlobManifest) (map[string]GraphProvenanceSlot, error) {
	states := map[string]GraphProvenanceSlot{}
	blobs := map[string]domain.GraphArtifactRef{}
	for _, blob := range staged.Entries {
		blobs[blob.ObjectID] = blob.ArtifactRef
	}
	for _, entry := range manifest.Entries {
		state := entry
		state.ArtifactID = ""
		body, err := json.Marshal(state)
		if err != nil {
			return nil, err
		}
		value := entry
		states[graphComposeKey("working", entry.Path)] = GraphProvenanceSlot{Dimension: "working", Path: entry.Path, StateSHA256: hashBytes(body), Working: &value}
	}
	for _, entry := range manifest.Index {
		ref, ok := blobs[entry.Object]
		if !ok {
			return nil, ErrBlocked
		}
		body, err := json.Marshal(entry)
		if err != nil {
			return nil, err
		}
		value := GraphProvenanceIndex{Mode: entry.Mode, ObjectID: entry.Object, BlobRef: ref}
		states[graphComposeKey("index", entry.Path)] = GraphProvenanceSlot{Dimension: "index", Path: entry.Path, StateSHA256: hashBytes(body), Index: &value}
	}
	return states, nil
}

func graphComposeStatesEqual(a, b GraphProvenanceSlot) bool {
	if a.Dimension != b.Dimension || a.Path != b.Path || a.Absent != b.Absent {
		return false
	}
	if a.Absent {
		return true
	}
	if a.Dimension == "working" {
		if a.Working == nil || b.Working == nil {
			return false
		}
		x, y := *a.Working, *b.Working
		x.ArtifactID = ""
		y.ArtifactID = ""
		return x == y
	}
	return a.Index != nil && b.Index != nil && a.Index.Mode == b.Index.Mode && a.Index.ObjectID == b.Index.ObjectID && a.Index.BlobRef.SHA256 == b.Index.BlobRef.SHA256 && a.Index.BlobRef.ByteLength == b.Index.BlobRef.ByteLength
}

func graphComposeOriginDigest(origin GraphProvenanceOrigin, slot GraphProvenanceSlot) (string, error) {
	var body []byte
	var err error
	if origin.Kind == "root" {
		body, err = json.Marshal(struct {
			Kind      string `json:"kind"`
			Root      string `json:"rootBasisSha256"`
			Dimension string `json:"dimension"`
			Path      string `json:"path"`
			State     string `json:"stateSha256"`
		}{"root", origin.RootBasisSHA256, slot.Dimension, slot.Path, slot.StateSHA256})
	} else {
		if origin.Node == nil {
			return "", ErrBlocked
		}
		parents := append([]string{}, origin.InputOriginIDs...)
		sort.Strings(parents)
		body, err = json.Marshal(struct {
			Kind      string                         `json:"kind"`
			Root      string                         `json:"rootBasisSha256"`
			Dimension string                         `json:"dimension"`
			Path      string                         `json:"path"`
			State     string                         `json:"stateSha256"`
			Node      domain.GraphContributorBinding `json:"node"`
			Parents   []string                       `json:"inputOriginIds"`
		}{"node", origin.RootBasisSHA256, slot.Dimension, slot.Path, slot.StateSHA256, *origin.Node, parents})
	}
	return hashBytes(body), err
}

func graphComposeCopyOrigins(origins []GraphProvenanceOrigin) []GraphProvenanceOrigin {
	out := append([]GraphProvenanceOrigin{}, origins...)
	for i := range out {
		out[i].InputOriginIDs = append([]string{}, out[i].InputOriginIDs...)
	}
	return out
}

func graphComposeRefLess(a, b domain.GraphArtifactRef) bool {
	if a.ArtifactID != b.ArtifactID {
		return a.ArtifactID < b.ArtifactID
	}
	return a.ProducerAttemptID < b.ProducerAttemptID
}
func graphComposeStateLess(a, b GraphProvenanceSlot) bool {
	if a.Absent {
		return false
	}
	if a.Dimension == "working" {
		return a.Working.ArtifactID < b.Working.ArtifactID
	}
	return graphComposeRefLess(a.Index.BlobRef, b.Index.BlobRef)
}

func graphComposeMaximal(states []GraphProvenanceSlot) (GraphProvenanceSlot, []GraphProvenanceOrigin, bool, error) {
	type head struct {
		origin GraphProvenanceOrigin
		state  GraphProvenanceSlot
	}
	byID := map[string]head{}
	for _, state := range states {
		for _, origin := range state.Origins {
			if previous, ok := byID[origin.OriginSHA256]; ok {
				if !graphComposeStatesEqual(previous.state, state) {
					return GraphProvenanceSlot{}, nil, false, ErrBlocked
				}
				oldBody, _ := json.Marshal(previous.origin)
				newBody, _ := json.Marshal(origin)
				if string(oldBody) != string(newBody) {
					return GraphProvenanceSlot{}, nil, false, ErrBlocked
				}
				if graphComposeStateLess(state, previous.state) {
					byID[origin.OriginSHA256] = head{origin, state}
				}
			} else {
				byID[origin.OriginSHA256] = head{origin, state}
			}
		}
	}
	maximal := []head{}
	for id, value := range byID {
		superseded := false
		for otherID, other := range byID {
			if id == otherID {
				continue
			}
			for _, ancestor := range other.origin.InputOriginIDs {
				if ancestor == id {
					superseded = true
					break
				}
			}
			if superseded {
				break
			}
		}
		if !superseded {
			maximal = append(maximal, value)
		}
	}
	if len(maximal) == 0 || len(maximal) > 16 {
		return GraphProvenanceSlot{}, nil, false, ErrBlocked
	}
	sort.Slice(maximal, func(i, j int) bool { return maximal[i].origin.OriginSHA256 < maximal[j].origin.OriginSHA256 })
	selected := maximal[0].state
	origins := []GraphProvenanceOrigin{}
	conflict := false
	for _, value := range maximal {
		origins = append(origins, value.origin)
		if !graphComposeStatesEqual(selected, value.state) {
			conflict = true
		}
		if !conflict && graphComposeStateLess(value.state, selected) {
			selected = value.state
		}
	}
	selected.Origins = graphComposeCopyOrigins(origins)
	return selected, origins, conflict, nil
}

func graphComposeContributor(c domain.AcceptedContribution) domain.GraphContributorBinding {
	return domain.GraphContributorBinding{NodeID: c.FromNodeID, TaskID: c.TaskID, TaskVersion: c.TaskVersion, AcceptanceKind: c.AcceptanceKind, AcceptanceID: c.AcceptanceID, ProducerAttemptID: c.ProducerAttemptID, InputID: c.InputID, CandidateBinding: c.CandidateBinding, ResponseBinding: c.ResponseBinding, OutputClosureRef: c.OutputClosureRef}
}
func graphComposeAddContributor(all map[string]domain.GraphContributorBinding, value domain.GraphContributorBinding) error {
	if old, ok := all[value.NodeID]; ok {
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(value)
		if string(a) != string(b) {
			return ErrBlocked
		}
		return nil
	}
	all[value.NodeID] = value
	return nil
}
