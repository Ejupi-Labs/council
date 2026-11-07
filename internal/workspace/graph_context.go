package workspace

import (
	"context"
	json "encoding/json/v2"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/evidence"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type GraphContextRequest struct {
	GraphID, InputID, InputSHA256, ReceivingNodeID, ReceivingTaskVersion string
	GraphInstructionsRef, NodeInstructionsRef                            domain.GraphArtifactRef
	GuidanceRef, AcceptanceRef                                           *domain.GraphArtifactRef
	ConstraintRefs                                                       []domain.GraphScopedReference
	ContributedEvidence                                                  []domain.GraphContributedEvidence
	SupportedAccess                                                      []domain.GraphProvidedAccess
}

func (m *Manager) BuildGraphContext(ctx context.Context, request GraphContextRequest) (domain.GraphContextPacket, error) {
	var packet domain.GraphContextPacket
	if !validManagedID(request.GraphID) || !validManagedID(request.InputID) || !validManagedID(request.ReceivingNodeID) || !domain.ValidSHA256(request.InputSHA256) {
		return packet, ErrBlocked
	}
	if _, valueErr := domain.ParseRevision(request.ReceivingTaskVersion); valueErr != nil {
		return packet, valueErr
	}
	if err := ctx.Err(); err != nil {
		return packet, err
	}
	refs := map[string]domain.GraphArtifactRef{}
	add := func(name string, ref domain.GraphArtifactRef) error {
		if name == "" {
			return ErrBlocked
		}
		if prior, ok := refs[name]; ok && prior != ref {
			return evidence.ErrCorrupt
		}
		refs[name] = ref
		if len(refs) > 64 {
			return ErrBlocked
		}
		return nil
	}
	if err := add("graph-instructions", request.GraphInstructionsRef); err != nil {
		return packet, err
	}
	if err := add("node-instructions", request.NodeInstructionsRef); err != nil {
		return packet, err
	}
	if request.GuidanceRef != nil {
		if err := add("receiver-guidance", *request.GuidanceRef); err != nil {
			return packet, err
		}
	}
	if request.AcceptanceRef != nil {
		if err := add("receiver-acceptance", *request.AcceptanceRef); err != nil {
			return packet, err
		}
	}
	for _, ref := range request.ConstraintRefs {
		if err := add(ref.Ref, ref.ArtifactRef); err != nil {
			return packet, err
		}
	}
	for _, ref := range request.ContributedEvidence {
		if err := add(ref.Ref, ref.ArtifactRef); err != nil {
			return packet, err
		}
	}
	required := make([]domain.GraphArtifactRef, 0, len(refs))
	for _, ref := range refs {
		required = append(required, ref)
	}
	for _, ref := range request.SupportedAccess {
		if ref.Mode != "file" && ref.Mode != "inline" {
			return packet, ErrBlocked
		}
		if err := add(ref.Ref, ref.ArtifactRef); err != nil {
			return packet, err
		}
	}
	if err := graphCompleteContextAccess(required, request.SupportedAccess); err != nil {
		return packet, err
	}
	verified := map[string]domain.GraphArtifactRef{}
	for _, ref := range refs {
		if previous, ok := verified[ref.ArtifactID]; ok {
			if previous != ref {
				return packet, evidence.ErrCorrupt
			}
			continue
		}
		if err := m.graphVerifyRef(ctx, ref); err != nil {
			return packet, err
		}
		verified[ref.ArtifactID] = ref
	}
	id, err := evidence.NewID()
	if err != nil {
		return packet, err
	}
	packet = domain.GraphContextPacket{ID: id, GraphID: request.GraphID, InputID: request.InputID, InputSHA256: request.InputSHA256, ReceivingTaskVersion: request.ReceivingTaskVersion,
		GraphInstructionsRef: request.GraphInstructionsRef, NodeInstructionsRef: request.NodeInstructionsRef, GuidanceRef: request.GuidanceRef, AcceptanceRef: request.AcceptanceRef,
		ConstraintRefs: append([]domain.GraphScopedReference{}, request.ConstraintRefs...), ContributedEvidence: append([]domain.GraphContributedEvidence{}, request.ContributedEvidence...), ProvidedAccess: append([]domain.GraphProvidedAccess{}, request.SupportedAccess...)}
	for i := range packet.ConstraintRefs {
		packet.ConstraintRefs[i].Paths = append([]string{}, packet.ConstraintRefs[i].Paths...)
		sort.Strings(packet.ConstraintRefs[i].Paths)
	}
	for i := range packet.ContributedEvidence {
		packet.ContributedEvidence[i].Paths = append([]string{}, packet.ContributedEvidence[i].Paths...)
		sort.Strings(packet.ContributedEvidence[i].Paths)
	}
	for i := range packet.ProvidedAccess {
		packet.ProvidedAccess[i].Paths = append([]string{}, packet.ProvidedAccess[i].Paths...)
		sort.Strings(packet.ProvidedAccess[i].Paths)
	}
	sort.Slice(packet.ConstraintRefs, func(i, j int) bool { return packet.ConstraintRefs[i].Ref < packet.ConstraintRefs[j].Ref })
	sort.Slice(packet.ContributedEvidence, func(i, j int) bool { return packet.ContributedEvidence[i].Ref < packet.ContributedEvidence[j].Ref })
	sort.Slice(packet.ProvidedAccess, func(i, j int) bool { return packet.ProvidedAccess[i].Ref < packet.ProvidedAccess[j].Ref })
	packet.PacketSHA256, err = domain.GraphContextDigest(packet)
	if err != nil {
		return domain.GraphContextPacket{}, err
	}
	body, err := json.Marshal(packet)
	if err != nil {
		return domain.GraphContextPacket{}, err
	}
	if len(body) > 256<<10 {
		return domain.GraphContextPacket{}, ErrBlocked
	}
	return packet, nil
}
