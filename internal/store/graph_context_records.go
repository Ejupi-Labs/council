package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphRefHasBytes(ref domain.GraphArtifactRef, body []byte) bool {
	digest := sha256.Sum256(body)
	return ref.SHA256 == hex.EncodeToString(digest[:]) && ref.ByteLength == domain.Decimal(int64(len(body)))
}
func (s *Store) graphStoredRef(ctx context.Context, q queryer, graphID string, ref domain.GraphArtifactRef, kind string) error {
	if invalid := domain.ValidateGraphArtifactRef(ref); invalid != nil {
		return invalid
	}
	artifact, e := s.artifact(ctx, q, ref.ArtifactID)
	if e != nil {
		return e
	}
	if artifact.AttemptID != ref.ProducerAttemptID || artifact.Kind != kind || !artifact.Closed || artifact.Completeness != "complete" || artifact.SHA256 == nil || *artifact.SHA256 != ref.SHA256 || artifact.ByteLength != ref.ByteLength {
		return evidenceUnavailable()
	}
	a, e := s.attempt(ctx, q, artifact.AttemptID)
	if e != nil {
		return e
	}
	member, e := s.graphMembership(ctx, q, a.TaskID)
	if e != nil {
		return e
	}
	if member == nil || member.GraphID != graphID {
		return evidenceUnavailable()
	}
	return nil
}
func (s *Store) graphInputContext(ctx context.Context, q queryer, op domain.GraphInputOperation, input domain.GraphInputBinding, f domain.GraphInputCompletionFacts) error {
	if f.ContextPacket == nil || f.ConditionsBinding == nil || f.ConditionsRef == nil || op.Control.NodeID == nil {
		return evidenceUnavailable()
	}
	packet, conditions := *f.ContextPacket, *f.ConditionsBinding
	if len(encode(packet)) > 256<<10 || packet.GraphID != op.GraphID || packet.InputID != input.ID || packet.InputSHA256 != input.InputSHA256 || packet.ReceivingTaskVersion != input.ReceivingTaskVersion || !graphRefHasBytes(input.ContextPacket, []byte(encode(packet))) {
		return evidenceUnavailable()
	}
	digest, e := domain.GraphContextDigest(packet)
	if e != nil || digest != packet.PacketSHA256 {
		return evidenceUnavailable()
	}
	if invalid := domain.ValidateGraphConditionsBinding(conditions); invalid != nil {
		return invalid
	}
	digest, e = domain.GraphConditionsDigest(conditions)
	if e != nil || digest != conditions.ConditionsSHA256 || conditions.GraphID != op.GraphID || conditions.GraphVersion != input.GraphVersion || conditions.ReceiverTaskID != input.ReceivingTaskID || conditions.ReceiverTaskVersion != input.ReceivingTaskVersion || !graphRefHasBytes(*f.ConditionsRef, []byte(encode(conditions))) {
		return evidenceUnavailable()
	}
	task, e := s.task(ctx, q, input.ReceivingTaskID)
	if e != nil {
		return e
	}
	version, e := s.version(ctx, q, task.ID, input.ReceivingTaskVersion)
	if e != nil {
		return e
	}
	shared := ""
	if op.Definition.Original.SharedInstructions != nil {
		shared = *op.Definition.Original.SharedInstructions
	}
	for _, item := range []struct {
		ref  domain.GraphArtifactRef
		body []byte
		kind string
	}{{packet.GraphInstructionsRef, []byte(shared), "graph_original"}, {packet.NodeInstructionsRef, []byte(task.OriginalText), "graph_original"}} {
		if !graphRefHasBytes(item.ref, item.body) {
			return evidenceUnavailable()
		}
		if e = s.graphStoredRef(ctx, q, op.GraphID, item.ref, item.kind); e != nil {
			return e
		}
	}
	if (packet.GuidanceRef == nil) != (version.GuidanceText == "") {
		return evidenceUnavailable()
	}
	if packet.GuidanceRef != nil {
		if !graphRefHasBytes(*packet.GuidanceRef, []byte(version.GuidanceText)) {
			return evidenceUnavailable()
		}
		if e = s.graphStoredRef(ctx, q, op.GraphID, *packet.GuidanceRef, "graph_original"); e != nil {
			return e
		}
	}
	if packet.AcceptanceRef == nil || !graphRefHasBytes(*packet.AcceptanceRef, []byte(encode(version.Acceptance))) {
		return evidenceUnavailable()
	}
	if e = s.graphStoredRef(ctx, q, op.GraphID, *packet.AcceptanceRef, "graph_acceptance"); e != nil {
		return e
	}
	expectedUpstream, e := s.graphExpectedUpstreamSources(ctx, q, input)
	if e != nil {
		return e
	}
	if e = graphPacketContributionAccess(op, packet); e != nil {
		return e
	}
	receiver, global := 0, 0
	seenNodes := map[string]bool{}
	for _, source := range conditions.Sources {
		switch source.AuthorityScope {
		case "graph_shared":
			global++
			if source.OriginalRef != packet.GraphInstructionsRef || source.GuidanceRef != nil || source.AcceptanceRef != nil {
				return evidenceUnavailable()
			}
		case "receiver":
			receiver++
			if source.NodeID == nil || *source.NodeID != *op.Control.NodeID || source.OriginalRef != packet.NodeInstructionsRef || !reflect.DeepEqual(source.GuidanceRef, packet.GuidanceRef) || !reflect.DeepEqual(source.AcceptanceRef, packet.AcceptanceRef) {
				return evidenceUnavailable()
			}
		case "upstream_local":
			if source.NodeID == nil || source.TaskVersion == nil || seenNodes[*source.NodeID] {
				return evidenceUnavailable()
			}
			seenNodes[*source.NodeID] = true
			want, present := expectedUpstream[*source.NodeID]
			if !present || !reflect.DeepEqual(want.TaskVersion, source.TaskVersion) || want.OriginalRef != source.OriginalRef || !reflect.DeepEqual(want.GuidanceRef, source.GuidanceRef) || !reflect.DeepEqual(want.AcceptanceRef, source.AcceptanceRef) {
				return evidenceUnavailable()
			}
			delete(expectedUpstream, *source.NodeID)
			found := false
			for _, v := range input.ContributorVector {
				if v.NodeID == *source.NodeID && v.TaskVersion == *source.TaskVersion {
					found = true
				}
			}
			if !found {
				return evidenceUnavailable()
			}
			if e = s.graphStoredRef(ctx, q, op.GraphID, source.OriginalRef, "graph_original"); e != nil {
				return e
			}
			if source.GuidanceRef != nil {
				if e = s.graphStoredRef(ctx, q, op.GraphID, *source.GuidanceRef, "graph_original"); e != nil {
					return e
				}
			}
			if source.AcceptanceRef != nil {
				if e = s.graphStoredRef(ctx, q, op.GraphID, *source.AcceptanceRef, "graph_acceptance"); e != nil {
					return e
				}
			}
		default:
			return evidenceUnavailable()
		}
	}
	if receiver != 1 || global != 1 || len(expectedUpstream) != 0 {
		return evidenceUnavailable()
	}
	conditionsPresent := false
	for _, constraint := range packet.ConstraintRefs {
		if constraint.ArtifactRef == *f.ConditionsRef {
			conditionsPresent = true
		}
	}
	if !conditionsPresent {
		return evidenceUnavailable()
	}
	return nil
}
