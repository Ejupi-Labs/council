package workspace

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"sort"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

func GraphManifestDigests(manifest Manifest) (string, string, error) {
	manifest.Entries = append([]Entry{}, manifest.Entries...)
	manifest.Index = append([]IndexEntry{}, manifest.Index...)
	manifest.Exclusions = append([]Exclusion{}, manifest.Exclusions...)
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path < manifest.Entries[j].Path })
	sort.Slice(manifest.Index, func(i, j int) bool { return manifest.Index[i].Path < manifest.Index[j].Path })
	sort.Slice(manifest.Exclusions, func(i, j int) bool { return manifest.Exclusions[i].Path < manifest.Exclusions[j].Path })
	if err := graphValidateSemanticManifest(manifest); err != nil {
		return "", "", err
	}
	type member struct {
		Path   string `json:"path"`
		Type   string `json:"type"`
		Mode   uint32 `json:"mode"`
		Size   int64  `json:"size"`
		SHA256 string `json:"contentSha256"`
		Target string `json:"target"`
	}
	entries := make([]member, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		entries = append(entries, member{entry.Path, entry.Type, entry.Mode, entry.Size, entry.SHA256, entry.Target})
	}
	body, err := json.Marshal(struct {
		Entries    []member    `json:"entries"`
		Exclusions []Exclusion `json:"exclusions"`
	}{entries, manifest.Exclusions})
	if err != nil {
		return "", "", err
	}
	index, err := graphIndexDigest(manifest.Index)
	return hashBytes(body), index, err
}

func (m *Manager) graphVerifyRef(ctx context.Context, ref domain.GraphArtifactRef) error {
	meta, err := m.artifacts.VerifyClosed(ctx, ref.ArtifactID)
	if err != nil {
		return err
	}
	if meta.SHA256 == nil || *meta.SHA256 != ref.SHA256 || meta.AttemptID != ref.ProducerAttemptID || meta.ByteLength != ref.ByteLength {
		return evidence.ErrCorrupt
	}
	return nil
}

func (m *Manager) graphVerifyStaged(ctx context.Context, manifest Manifest, ref domain.GraphArtifactRef) (GraphIndexBlobManifest, error) {
	var staged GraphIndexBlobManifest
	body, err := m.graphIndexLoad(ctx, ref, "graph_index_blobs")
	if err != nil {
		return staged, err
	}
	if err = json.Unmarshal(body, &staged, json.RejectUnknownMembers(true)); err != nil || staged.Contract != "006-index-blobs-v1" || len(staged.Entries) > MaxEntries {
		return staged, evidence.ErrCorrupt
	}
	objects := graphIndexObjects(manifest.Index)
	if len(objects) != len(staged.Entries) {
		return staged, evidence.ErrCorrupt
	}
	working, total, err := m.graphWorkingPayloads(ctx, manifest)
	if err != nil {
		return staged, err
	}
	extra := map[string]bool{}
	for i, blob := range staged.Entries {
		if blob.ObjectID != objects[i] || blob.ContentSHA256 != blob.ArtifactRef.SHA256 || blob.ByteLength != blob.ArtifactRef.ByteLength {
			return staged, evidence.ErrCorrupt
		}
		data, err := m.graphIndexBlobBytes(ctx, blob)
		if err != nil {
			return staged, err
		}
		key := blob.ContentSHA256 + ":" + blob.ByteLength
		if _, ok := working[key]; !ok && !extra[key] {
			total += int64(len(data))
			extra[key] = true
		}
		if total > MaxTotalBytes {
			return staged, ErrBlocked
		}
	}
	return staged, nil
}

func (m *Manager) VerifyGraphInput(ctx context.Context, binding domain.GraphInputBinding) (VerifiedGraphInput, error) {
	var result VerifiedGraphInput
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := m.lock.CheckHeld(); err != nil {
		return result, err
	}
	if !validManagedID(binding.ID) || !validManagedID(binding.GraphID) || !validManagedID(binding.OperationAttemptID) || !validManagedID(binding.ReceivingTaskID) || binding.ReceivingNodeID == "" || len(binding.Contributions) > 64 || len(binding.ContributorVector) > 16 || !domain.ValidSHA256(binding.RootBasisSHA256) {
		return result, ErrBlocked
	}
	if valueErr := domain.ValidateGraphInputBinding(binding); valueErr != nil {
		return result, valueErr
	}
	seal, err := domain.GraphInputDigest(binding)
	if err != nil || seal != binding.InputSHA256 {
		return result, evidence.ErrCorrupt
	}
	for _, ref := range []domain.GraphArtifactRef{binding.Manifest, binding.Index, binding.StagedBlobs, binding.Provenance, binding.ContextPacket} {
		if ref.ProducerAttemptID != binding.OperationAttemptID {
			return result, evidence.ErrCorrupt
		}
	}
	if _, err = m.graphIndexLoad(ctx, binding.Manifest, "graph_input_manifest"); err != nil {
		return result, err
	}
	manifest, raw, err := m.graphIndexBasis(ctx, binding.Manifest, binding.Index)
	if err != nil {
		return result, err
	}
	staged, err := m.graphVerifyStaged(ctx, manifest, binding.StagedBlobs)
	if err != nil {
		return result, err
	}
	provenance, err := m.graphIndexLoad(ctx, binding.Provenance, "graph_input_provenance")
	if err != nil || !jsontext.Value(provenance).IsValid() {
		return result, evidence.ErrCorrupt
	}
	packetBody, err := m.graphIndexLoad(ctx, binding.ContextPacket, "graph_context")
	if err != nil {
		return result, err
	}
	if len(packetBody) > 256<<10 {
		return result, ErrBlocked
	}
	var packet domain.GraphContextPacket
	if err = json.Unmarshal(packetBody, &packet, json.RejectUnknownMembers(true)); err != nil {
		return result, evidence.ErrCorrupt
	}
	packetSeal, err := domain.GraphContextDigest(packet)
	if err != nil || packetSeal != packet.PacketSHA256 || packet.GraphID != binding.GraphID || packet.InputID != binding.ID || packet.InputSHA256 != binding.InputSHA256 || packet.ReceivingTaskVersion != binding.ReceivingTaskVersion || len(packet.ProvidedAccess) > 64 {
		return result, evidence.ErrCorrupt
	}
	required := []domain.GraphArtifactRef{packet.GraphInstructionsRef, packet.NodeInstructionsRef}
	refs := []domain.GraphArtifactRef{binding.Manifest, binding.Index, binding.StagedBlobs, binding.Provenance, binding.ContextPacket, packet.GraphInstructionsRef, packet.NodeInstructionsRef}
	if packet.GuidanceRef != nil {
		refs = append(refs, *packet.GuidanceRef)
		required = append(required, *packet.GuidanceRef)
	}
	if packet.AcceptanceRef != nil {
		refs = append(refs, *packet.AcceptanceRef)
		required = append(required, *packet.AcceptanceRef)
	}
	if packet.DerivedDigestRef != nil {
		refs = append(refs, *packet.DerivedDigestRef)
	}
	for _, ref := range packet.ConstraintRefs {
		refs = append(refs, ref.ArtifactRef)
		required = append(required, ref.ArtifactRef)
	}
	for _, ref := range packet.ContributedEvidence {
		refs = append(refs, ref.ArtifactRef)
		required = append(required, ref.ArtifactRef)
	}
	for _, ref := range packet.ProvidedAccess {
		refs = append(refs, ref.ArtifactRef)
	}
	if err = graphCompleteContextAccess(required, packet.ProvidedAccess); err != nil {
		return result, err
	}
	if err = graphSelectedContextCoverage(binding.Contributions, packet.ConstraintRefs, packet.ContributedEvidence, packet.ProvidedAccess); err != nil {
		return result, err
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
		if err = m.graphVerifyRef(ctx, ref); err != nil {
			return result, err
		}
		seen[ref.ArtifactID] = ref
		unique = append(unique, ref)
	}
	if len(unique) > 256 {
		return result, ErrBlocked
	}
	return VerifiedGraphInput{Binding: binding, Manifest: manifest, RawIndex: raw, StagedBlobs: staged, ContextPacket: packet, Provenance: provenance, ArtifactRefs: unique}, nil
}

func graphCheckSourceBasis(basis domain.GraphSourceBasis, input domain.GraphInputBinding, manifest Manifest) error {
	if valueErr := domain.ValidateGraphSourceBasis(basis); valueErr != nil {
		return valueErr
	}
	if basis.InputID != input.ID || basis.InputSHA256 != input.InputSHA256 || !validManagedID(basis.WorkflowID) || !validManagedID(basis.StepID) {
		return ErrBlocked
	}
	if basis.BasisKind == "graph_input" && (basis.CandidateBinding != nil || basis.ResponseBinding != nil || basis.ExpectedManifest != input.Manifest || basis.ExpectedIndex != input.Index) {
		return ErrBlocked
	}
	working, index, err := GraphManifestDigests(manifest)
	if err != nil {
		return err
	}
	if working != basis.ExpectedWorkingEntriesSHA256 || index != basis.ExpectedIndexEntriesSHA256 {
		return fmt.Errorf("%w Graph semantic source binding differs.", ErrBlocked)
	}
	return nil
}

func (m *Manager) RecheckGraphMaterialization(ctx context.Context, receipt domain.GraphMaterializationReceipt, snapshot *Snapshot, workspaceIdentity string) (MaterializationProof, error) {
	var result MaterializationProof
	if valueErr := domain.ValidateGraphMaterializationReceipt(receipt); valueErr != nil {
		return result, valueErr
	}
	if receipt.SourceBasis.BasisKind == "node_subject" {
		if err := graphCheckSelectedSubject(receipt.SourceBasis); err != nil {
			return result, err
		}
	}
	if receipt.State != "complete" || !receipt.EffectsKnown || receipt.EvidenceStatus != "complete" || receipt.Failure != nil || receipt.SourceSnapshotID == nil || receipt.ObservedManifest == nil || receipt.ObservedIndex == nil || receipt.ObservedWorkingEntriesSHA256 == nil || receipt.ObservedIndexEntriesSHA256 == nil || receipt.StagedBlobsSHA256 == nil || receipt.OperationEffects == nil || snapshot == nil || snapshot.ID != *receipt.SourceSnapshotID || snapshot.AttemptID != receipt.ReceivingAttemptID || receipt.OperationAttemptID != receipt.ReceivingAttemptID {
		return result, ErrBlocked
	}
	root, err := m.graphIndexTarget(snapshot)
	if err != nil {
		return result, err
	}
	defer root.Close()
	body, err := m.graphIndexLoad(ctx, *receipt.OperationEffects, "graph_operation_effects")
	if err != nil {
		return result, err
	}
	var effects GraphMaterializationEffects
	if err = json.Unmarshal(body, &effects, json.RejectUnknownMembers(true)); err != nil || effects.Contract != "006-materialization-effects-v1" || effects.Registration != "registered" || !effects.EffectsKnown || effects.WorkspaceID != snapshot.ID || workspaceIdentity == "" || effects.StagedBlobs.SHA256 != *receipt.StagedBlobsSHA256 || effects.ObjectEffects == nil {
		return result, evidence.ErrCorrupt
	}
	if err = project.ValidateIdentity(snapshot.WorkspacePath, workspaceIdentity); err != nil {
		return result, err
	}
	expected, _, err := m.graphIndexBasis(ctx, receipt.SourceBasis.ExpectedManifest, receipt.SourceBasis.ExpectedIndex)
	if err != nil {
		return result, err
	}
	observed, _, err := m.graphIndexBasis(ctx, *receipt.ObservedManifest, *receipt.ObservedIndex)
	if err != nil || !sameManifest(expected, observed) {
		return result, evidence.ErrCorrupt
	}
	if receipt.ObservedManifest.ArtifactID != snapshot.ManifestArtifactID || receipt.ObservedIndex.ArtifactID != snapshot.IndexArtifactID || receipt.ObservedManifest.ProducerAttemptID != snapshot.AttemptID || receipt.ObservedIndex.ProducerAttemptID != snapshot.AttemptID {
		return result, evidence.ErrCorrupt
	}
	staged, err := m.graphVerifyStaged(ctx, expected, effects.StagedBlobs)
	if err != nil {
		return result, err
	}
	if err = m.graphVerifyRef(ctx, *effects.ObjectEffects); err != nil {
		return result, err
	}
	objectByID := map[string]GraphIndexBlob{}
	for _, entry := range staged.Entries {
		objectByID[entry.ObjectID] = entry
	}
	if err = m.graphReadBlobs(ctx, snapshot.WorkspacePath, graphIndexObjects(expected.Index), func(object string, body []byte) error {
		entry := objectByID[object]
		if hashBytes(body) != entry.ContentSHA256 || strconv.Itoa(len(body)) != entry.ByteLength {
			return evidence.ErrCorrupt
		}
		return nil
	}); err != nil {
		return result, err
	}
	indexPath, err := m.value(ctx, snapshot.WorkspacePath, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return result, err
	}
	before, err := readIndex(indexPath)
	if err != nil {
		return result, err
	}
	index, err := m.index(ctx, snapshot.WorkspacePath)
	if err != nil || !indexEqual(index, expected.Index) {
		return result, ErrBlocked
	}
	actual, err := m.inventory(ctx, snapshot.AttemptID, snapshot.WorkspacePath, root, index, false, expected.Entries)
	actual.Exclusions = expected.Exclusions
	if err != nil || !sameManifest(actual, expected) {
		return result, fmt.Errorf("%w The receiving workspace changed before native preparation.", ErrBlocked)
	}
	after, err := readIndex(indexPath)
	if err != nil || !bytes.Equal(before, after) {
		return result, ErrBlocked
	}
	if err = project.ValidateIdentity(snapshot.WorkspacePath, workspaceIdentity); err != nil {
		return result, err
	}
	working, indexSHA, err := GraphManifestDigests(actual)
	if err != nil || working != receipt.SourceBasis.ExpectedWorkingEntriesSHA256 || indexSHA != receipt.SourceBasis.ExpectedIndexEntriesSHA256 || working != *receipt.ObservedWorkingEntriesSHA256 || indexSHA != *receipt.ObservedIndexEntriesSHA256 {
		return result, ErrBlocked
	}
	return MaterializationProof{Artifacts: []domain.GraphArtifactRef{receipt.SourceBasis.ExpectedManifest, receipt.SourceBasis.ExpectedIndex, *receipt.ObservedManifest, *receipt.ObservedIndex, effects.StagedBlobs, *receipt.OperationEffects, *effects.ObjectEffects}, WorkingEntriesSHA256: working, IndexEntriesSHA256: indexSHA}, nil
}
