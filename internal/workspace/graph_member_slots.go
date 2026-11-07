package workspace

import (
	json "encoding/json/v2"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// GraphMemberSlots validates already physically verified retained facts for a
// read-only member projection. It grants no acceptance or execution authority.
func GraphMemberSlots(root domain.GraphRootBinding, proof GraphRootProof, input *VerifiedGraphInput) ([]GraphProvenanceSlot, error) {
	if invalid := domain.ValidateGraphRootBinding(root); invalid != nil {
		return nil, invalid
	}
	seal, err := domain.GraphRootDigest(root)
	if err != nil || seal != root.BasisSHA256 {
		return nil, ErrBlocked
	}
	if err = graphComposeJSONRef(root.Manifest, proof.Manifest); err != nil {
		return nil, err
	}
	if err = graphComposeJSONRef(root.StagedBlobs, proof.StagedBlobs); err != nil {
		return nil, err
	}
	if err = graphComposeValidatePayloads(proof.Manifest, proof.StagedBlobs); err != nil {
		return nil, err
	}
	original, err := graphInitialProvenance(root.BasisSHA256, proof.Manifest, proof.StagedBlobs)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return original.Slots, nil
	}
	binding := input.Binding
	if invalid := domain.ValidateGraphInputBinding(binding); invalid != nil {
		return nil, invalid
	}
	seal, err = domain.GraphInputDigest(binding)
	if err != nil || seal != binding.InputSHA256 || binding.GraphID != root.GraphID || binding.RootBasisSHA256 != root.BasisSHA256 {
		return nil, ErrBlocked
	}
	for _, ref := range []domain.GraphArtifactRef{binding.Manifest, binding.Index, binding.StagedBlobs, binding.Provenance, binding.ContextPacket} {
		if ref.ProducerAttemptID != binding.OperationAttemptID {
			return nil, ErrBlocked
		}
	}
	if err = graphComposeJSONRef(binding.Manifest, input.Manifest); err != nil {
		return nil, err
	}
	if err = graphComposeJSONRef(binding.StagedBlobs, input.StagedBlobs); err != nil {
		return nil, err
	}
	if len(input.RawIndex) > int(MaxFileBytes) || binding.Index.SHA256 != hashBytes(input.RawIndex) || binding.Index.ByteLength != strconv.Itoa(len(input.RawIndex)) || len(input.Provenance) > int(MaxFileBytes) {
		return nil, ErrBlocked
	}
	var provenance GraphInputProvenance
	if err = json.Unmarshal(input.Provenance, &provenance, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if err = graphComposeJSONRef(binding.Provenance, provenance); err != nil {
		return nil, err
	}
	facts := VerifiedContribution{InputBinding: binding, InputManifest: input.Manifest, InputProvenance: provenance}
	if err = graphComposeValidateProvenance(graphComposeSlotMap(original), root.BasisSHA256, facts); err != nil {
		return nil, err
	}
	// These are decoded/copied value facts. The Engine separately verifies the
	// immutable graph's files-only reachability for mixed-edge node origins.
	return provenance.Slots, nil
}
