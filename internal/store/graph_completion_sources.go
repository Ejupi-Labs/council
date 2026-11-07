package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) graphAssemblySnapshot(ctx context.Context, tx *sql.Tx, op domain.GraphInputOperation, f domain.GraphInputCompletionFacts) error {
	if f.SourceSnapshot == nil {
		return nil
	}
	source := *f.SourceSnapshot
	root := op.RootBinding
	if op.Kind != "node_input" || root == nil || source.AttemptID != op.Attempt.ID || source.ProjectIdentity != root.ProjectIdentity || source.BaseCommit != root.BaseCommit || source.SelectedRelativePath != root.SelectedRelativePath || !reflect.DeepEqual(source.Branch, root.Branch) {
		return evidenceUnavailable()
	}
	manifest, e := s.artifact(ctx, tx, source.ManifestArtifactID)
	if e != nil {
		return e
	}
	if manifest.SHA256 == nil || *manifest.SHA256 != source.ManifestSHA256 {
		return evidenceUnavailable()
	}
	if f.InputBinding != nil && (source.ManifestSHA256 != f.InputBinding.Manifest.SHA256 || source.IndexArtifactID != f.InputBinding.Index.ArtifactID) {
		return evidenceUnavailable()
	}
	ref := domain.GraphArtifactRef{ArtifactID: manifest.ID, ProducerAttemptID: manifest.AttemptID, SHA256: *manifest.SHA256, ByteLength: manifest.ByteLength}
	if ref.ProducerAttemptID != op.Attempt.ID {
		return evidenceUnavailable()
	}
	if e = s.verifyGraphRef(ctx, tx, ref, f.VerifiedArtifacts, "source_manifest"); e != nil {
		return e
	}
	index, e := s.artifact(ctx, tx, source.IndexArtifactID)
	if e != nil {
		return e
	}
	if index.SHA256 == nil || index.AttemptID != op.Attempt.ID {
		return evidenceUnavailable()
	}
	if e = s.verifyGraphRef(ctx, tx, domain.GraphArtifactRef{ArtifactID: index.ID, ProducerAttemptID: index.AttemptID, SHA256: *index.SHA256, ByteLength: index.ByteLength}, f.VerifiedArtifacts, "index_snapshot"); e != nil {
		return e
	}
	return s.putSourceSnapshot(ctx, tx, source, true)
}

func (s *Store) graphExpectedUpstreamSources(ctx context.Context, q queryer, input domain.GraphInputBinding) (map[string]domain.GraphConditionSource, error) {
	expected := map[string]domain.GraphConditionSource{}
	for _, contributor := range input.ContributorVector {
		source, e := s.graphInputRecord(ctx, q, input.GraphID, contributor.InputID)
		if e != nil {
			return nil, e
		}
		binding := source.Public.InputBinding
		conditions := source.ConditionsBinding
		if binding == nil || binding.ReceivingNodeID != contributor.NodeID || binding.ReceivingTaskID != contributor.TaskID || binding.ReceivingTaskVersion != contributor.TaskVersion || conditions == nil || source.Public.ConditionsRef == nil || !graphRefHasBytes(*source.Public.ConditionsRef, []byte(encode(conditions))) {
			return nil, evidenceUnavailable()
		}
		seal, e := domain.GraphConditionsDigest(*conditions)
		if e != nil || seal != conditions.ConditionsSHA256 {
			return nil, evidenceUnavailable()
		}
		if e = s.graphStoredRef(ctx, q, input.GraphID, *source.Public.ConditionsRef, "graph_conditions"); e != nil {
			return nil, e
		}
		found := false
		for _, item := range conditions.Sources {
			if item.AuthorityScope != "receiver" {
				continue
			}
			if found || item.NodeID == nil || *item.NodeID != contributor.NodeID || item.TaskVersion == nil || *item.TaskVersion != contributor.TaskVersion {
				return nil, evidenceUnavailable()
			}
			found = true
			expected[contributor.NodeID] = item
		}
		if !found {
			return nil, evidenceUnavailable()
		}
	}
	return expected, nil
}

func graphPacketAccess(packet domain.GraphContextPacket, name string, ref domain.GraphArtifactRef, scope string, paths []string) bool {
	for _, access := range packet.ProvidedAccess {
		if access.Ref == name && access.ArtifactRef == ref && access.Scope == scope && reflect.DeepEqual(access.Paths, paths) && (access.Mode == "file" || access.Mode == "inline") {
			return true
		}
	}
	return false
}

func graphPacketContributionAccess(op domain.GraphInputOperation, packet domain.GraphContextPacket) error {
	if len(packet.ProvidedAccess) > 64 {
		return domain.Invalid("bounded complete graph context access")
	}
	names := map[string]bool{}
	for _, access := range packet.ProvidedAccess {
		if access.Ref == "" || names[access.Ref] || access.Mode != "inline" && access.Mode != "file" {
			return evidenceUnavailable()
		}
		names[access.Ref] = true
	}
	for _, c := range op.Contributions {
		for _, required := range c.ConstraintRefs {
			found := false
			for _, supplied := range packet.ConstraintRefs {
				if reflect.DeepEqual(required, supplied) {
					found = true
				}
			}
			if !found || !graphPacketAccess(packet, required.Ref, required.ArtifactRef, required.AuthorityScope, required.Paths) {
				return evidenceUnavailable()
			}
		}
		for _, required := range c.EvidenceRefs {
			found := false
			for _, supplied := range packet.ContributedEvidence {
				if reflect.DeepEqual(required, supplied) {
					found = true
				}
			}
			if !found || !graphPacketAccess(packet, required.Ref, required.ArtifactRef, required.Scope, required.Paths) {
				return evidenceUnavailable()
			}
		}
	}
	return nil
}
