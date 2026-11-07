package workspace

import (
	json "encoding/json/v2"
	"sort"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphComposeValidate(request GraphCompositionRequest) error {
	if !validManagedID(request.InputID) || len(request.Contributions) > 64 {
		return ErrBlocked
	}
	root := request.Root
	if e := domain.ValidateGraphRootBinding(root.Binding); e != nil {
		return e
	}
	seal, err := domain.GraphRootDigest(root.Binding)
	if err != nil || seal != root.Binding.BasisSHA256 {
		return ErrBlocked
	}
	if err = graphComposeJSONRef(root.Binding.Manifest, root.Manifest); err != nil {
		return err
	}
	if err = graphComposeJSONRef(root.Binding.StagedBlobs, root.StagedBlobs); err != nil {
		return err
	}
	if err = graphComposeValidatePayloads(root.Manifest, root.StagedBlobs); err != nil {
		return err
	}
	rootProvenance, err := graphInitialProvenance(root.Binding.BasisSHA256, root.Manifest, root.StagedBlobs)
	if err != nil {
		return err
	}
	rootStates := graphComposeSlotMap(rootProvenance)
	edges := map[string]bool{}
	for _, input := range request.Contributions {
		c := input.Binding
		for _, id := range []string{c.EdgeID, c.FromNodeID, c.TaskID, c.AcceptanceID, c.ProducerAttemptID, c.InputID} {
			if !validManagedID(id) {
				return ErrBlocked
			}
		}
		if edges[c.EdgeID] || c.EdgeKind != "files" && c.EdgeKind != "context" || c.AcceptanceKind != "workflow" && c.AcceptanceKind != "response" {
			return ErrBlocked
		}
		edges[c.EdgeID] = true
		if _, e := domain.ParseRevision(c.TaskVersion); e != nil {
			return e
		}
		if e := domain.ValidateGraphInputBinding(input.InputBinding); e != nil {
			return e
		}
		in := input.InputBinding
		currentShape := in
		currentShape.Contributions = []domain.AcceptedContribution{c}
		currentShape.ContributorVector = []domain.GraphContributorBinding{graphComposeContributor(c)}
		if e := domain.ValidateGraphInputBinding(currentShape); e != nil {
			return e
		}
		if in.GraphID != root.Binding.GraphID || in.RootBasisSHA256 != root.Binding.BasisSHA256 || in.ID != c.InputID || in.ReceivingNodeID != c.FromNodeID || in.ReceivingTaskID != c.TaskID || in.ReceivingTaskVersion != c.TaskVersion {
			return ErrBlocked
		}
		inputSeal, e := domain.GraphInputDigest(in)
		if e != nil || inputSeal != in.InputSHA256 {
			return ErrBlocked
		}
		if err = graphComposeJSONRef(in.Manifest, input.InputManifest); err != nil {
			return err
		}
		if err = graphComposeJSONRef(in.Provenance, input.InputProvenance); err != nil {
			return err
		}
		if err = graphComposeValidateProvenance(rootStates, root.Binding.BasisSHA256, input); err != nil {
			return err
		}
		closure := input.OutputClosure
		if e := domain.ValidateGraphOutputClosure(closure); e != nil {
			return e
		}
		if closure.GraphID != root.Binding.GraphID || closure.NodeID != c.FromNodeID || closure.ProducerAttemptID != c.ProducerAttemptID || closure.ReceivedInputID != c.InputID || c.OutputClosureRef.ProducerAttemptID != c.ProducerAttemptID {
			return ErrBlocked
		}
		closureSeal, e := domain.GraphOutputDigest(closure)
		if e != nil || closureSeal != closure.ClosureSHA256 {
			return ErrBlocked
		}
		if err = graphComposeJSONRef(c.OutputClosureRef, closure); err != nil {
			return err
		}
		if err = graphComposeJSONRef(closure.Manifest, input.OutputManifest); err != nil {
			return err
		}
		if err = graphComposeJSONRef(closure.StagedBlobs, input.OutputStagedBlobs); err != nil {
			return err
		}
		if err = graphComposeValidatePayloads(input.OutputManifest, input.OutputStagedBlobs); err != nil {
			return err
		}
		basis := domain.GraphSourceBasis{BasisKind: "node_subject", CandidateBinding: c.CandidateBinding, ResponseBinding: c.ResponseBinding, ExpectedManifest: closure.Manifest, ExpectedIndex: closure.Index}
		if err = graphCheckSelectedSubject(basis); err != nil {
			return err
		}
		if closure.Kind == "candidate" && (c.CandidateBinding == nil || closure.CandidateID == nil || *closure.CandidateID != c.CandidateBinding.CandidateID) || closure.Kind == "response" && (c.ResponseBinding == nil || closure.ResponseArtifact == nil || closure.ResponseArtifact.ArtifactID != c.ResponseBinding.ResponseArtifactID || closure.ResponseArtifact.SHA256 != c.ResponseBinding.ResponseSHA256 || closure.ResponseArtifact.ByteLength != c.ResponseBinding.ResponseByteLength) {
			return ErrBlocked
		}
	}
	return nil
}

func graphComposeJSONRef(ref domain.GraphArtifactRef, value any) error {
	if e := domain.ValidateGraphArtifactRef(ref); e != nil {
		return e
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) > 64<<20 || ref.SHA256 != hashBytes(body) || ref.ByteLength != strconv.Itoa(len(body)) {
		return ErrBlocked
	}
	return nil
}

func graphComposeValidatePayloads(manifest Manifest, staged GraphIndexBlobManifest) error {
	if err := validateHandoffManifest(manifest); err != nil {
		return err
	}
	if err := graphValidateSemanticManifest(manifest); err != nil {
		return err
	}
	if staged.Contract != "006-index-blobs-v1" || len(staged.Entries) > MaxEntries {
		return ErrBlocked
	}
	objects := graphIndexObjects(manifest.Index)
	if len(objects) != len(staged.Entries) {
		return ErrBlocked
	}
	working := map[string]bool{}
	var total int64
	for _, entry := range manifest.Entries {
		if entry.Type == "file" {
			working[entry.SHA256+":"+strconv.FormatInt(entry.Size, 10)] = true
			total += entry.Size
		}
	}
	extra := map[string]bool{}
	for i, blob := range staged.Entries {
		if blob.ObjectID != objects[i] || !graphObjectID(blob.ObjectID) || blob.ContentSHA256 != blob.ArtifactRef.SHA256 || blob.ByteLength != blob.ArtifactRef.ByteLength {
			return ErrBlocked
		}
		if e := domain.ValidateGraphArtifactRef(blob.ArtifactRef); e != nil {
			return e
		}
		count, e := domain.ParseCount(blob.ByteLength)
		if e != nil {
			return e
		}
		if count > MaxFileBytes {
			return ErrBlocked
		}
		key := blob.ContentSHA256 + ":" + blob.ByteLength
		if !working[key] && !extra[key] {
			total += count
			extra[key] = true
		}
		if total > MaxTotalBytes {
			return ErrBlocked
		}
	}
	return nil
}

func graphComposeValidateProvenance(rootStates map[string]GraphProvenanceSlot, rootSHA string, input VerifiedContribution) error {
	prov := input.InputProvenance
	if prov.Contract != "006-input-provenance-v1" || prov.RootBasisSHA256 != rootSHA || len(prov.Slots) > 2*MaxEntries {
		return ErrBlocked
	}
	blobs := map[string]GraphIndexBlob{}
	for _, slot := range prov.Slots {
		if slot.Index != nil && !slot.Absent {
			b := slot.Index.BlobRef
			blob := GraphIndexBlob{ObjectID: slot.Index.ObjectID, ContentSHA256: b.SHA256, ByteLength: b.ByteLength, ArtifactRef: b}
			if old, ok := blobs[blob.ObjectID]; ok && (old.ContentSHA256 != blob.ContentSHA256 || old.ByteLength != blob.ByteLength) {
				return ErrBlocked
			}
			blobs[blob.ObjectID] = blob
		}
	}
	staged := GraphIndexBlobManifest{Contract: "006-index-blobs-v1", Entries: []GraphIndexBlob{}}
	for _, blob := range blobs {
		staged.Entries = append(staged.Entries, blob)
	}
	sort.Slice(staged.Entries, func(i, j int) bool { return staged.Entries[i].ObjectID < staged.Entries[j].ObjectID })
	if err := graphComposeValidatePayloads(input.InputManifest, staged); err != nil {
		return err
	}
	if err := graphComposeJSONRef(input.InputBinding.StagedBlobs, staged); err != nil {
		return err
	}
	actual, err := graphComposeManifestStates(input.InputManifest, staged)
	if err != nil {
		return err
	}
	vector := map[string]domain.GraphContributorBinding{}
	for _, node := range input.InputBinding.ContributorVector {
		vector[node.NodeID] = node
	}
	hasFiles := false
	for _, edge := range input.InputBinding.Contributions {
		if edge.EdgeKind == "files" {
			hasFiles = true
		}
	}
	previous := ""
	seen := map[string]bool{}
	for _, slot := range prov.Slots {
		key := graphComposeKey(slot.Dimension, slot.Path)
		if key <= previous || !safePath(slot.Path) || excluded(slot.Path) != "" || slot.Dimension != "working" && slot.Dimension != "index" || len(slot.Origins) == 0 || len(slot.Origins) > 16 {
			return ErrBlocked
		}
		previous = key
		seen[key] = true
		expected, ok := actual[key]
		if !ok {
			expected = graphComposeAbsent(key)
		}
		if !graphComposeStatesEqual(slot, expected) || slot.StateSHA256 != expected.StateSHA256 {
			return ErrBlocked
		}
		if slot.Absent && (slot.Working != nil || slot.Index != nil) || slot.Dimension == "working" && !slot.Absent && slot.Index != nil || slot.Dimension == "index" && !slot.Absent && slot.Working != nil {
			return ErrBlocked
		}
		originSeen := map[string]bool{}
		for _, origin := range slot.Origins {
			if origin.RootBasisSHA256 != rootSHA || !domain.ValidSHA256(origin.OriginSHA256) || originSeen[origin.OriginSHA256] {
				return ErrBlocked
			}
			originSeen[origin.OriginSHA256] = true
			if origin.Kind == "root" {
				rootState, ok := rootStates[key]
				if !ok {
					rootState = graphComposeRootAbsent(rootSHA, key)
				}
				if origin.Node != nil || len(origin.InputOriginIDs) != 0 || !graphComposeStatesEqual(slot, rootState) {
					return ErrBlocked
				}
			} else if origin.Kind == "node" {
				if !hasFiles || origin.Node == nil {
					return ErrBlocked
				}
				known, ok := vector[origin.Node.NodeID]
				if !ok {
					return ErrBlocked
				}
				a, _ := json.Marshal(known)
				b, _ := json.Marshal(origin.Node)
				if string(a) != string(b) {
					return ErrBlocked
				}
				prior := ""
				for _, id := range origin.InputOriginIDs {
					if !domain.ValidSHA256(id) || id <= prior || id == origin.OriginSHA256 {
						return ErrBlocked
					}
					prior = id
				}
			} else {
				return ErrBlocked
			}
			digest, e := graphComposeOriginDigest(origin, slot)
			if e != nil || digest != origin.OriginSHA256 {
				return ErrBlocked
			}
		}
	}
	for key := range actual {
		if !seen[key] {
			return ErrBlocked
		}
	}
	for key := range rootStates {
		if !seen[key] {
			return ErrBlocked
		}
	}
	return nil
}
