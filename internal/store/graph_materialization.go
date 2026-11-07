package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) recordGraphMaterialization(ctx context.Context, ownerID, attemptID string, f domain.GraphMaterializationFacts) error {
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, attemptID)
		if e != nil {
			return e
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		if a.OwnerInstanceID != ownerID || g.OwnerInstanceID != ownerID {
			return graphControlError()
		}
		record, e := s.graphDispatchForAttempt(ctx, tx, attemptID)
		if e != nil {
			return e
		}
		d, r := record.Public.Dispatch, f.Receipt
		if d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID || d.OwnerInstanceID != ownerID || r.GraphID != d.GraphID || r.InputID != d.InputID || r.InputSHA256 != d.InputSHA256 || r.OperationAttemptID != a.ID || r.ReceivingAttemptID != a.ID || !domain.ValidID(r.ID) || !reflect.DeepEqual(r.SourceBasis, d.SourceBasis) {
			return evidenceUnavailable()
		}
		if record.Public.Materialization != nil {
			if reflect.DeepEqual(*record.Public.Materialization, r) && record.MaterializationArtifact != nil && *record.MaterializationArtifact == f.ReceiptArtifact && reflect.DeepEqual(record.PrivateMaterialization, &domain.GraphPrivateMaterialization{WorkspaceIdentity: f.WorkspaceIdentity, GitControlSHA256: f.GitControlSHA256}) {
				return nil
			}
			return domain.Fail("graph_control_conflict", "This execution already has an immutable materialization observation.")
		}
		if a.Phase != "preparing" && a.Phase != "cancelling" || a.LaunchIntent {
			return domain.Fail("attempt_active", "Retain materialization before native preparation or launch.")
		}
		if e = s.checkAttemptClaim(ctx, tx, ownerID, a.ID); e != nil {
			return e
		}
		if f.ReceiptArtifact.ProducerAttemptID != a.ID || !graphRefHasBytes(f.ReceiptArtifact, []byte(encode(r))) {
			return evidenceUnavailable()
		}
		if e = s.verifyGraphRef(ctx, tx, f.ReceiptArtifact, f.VerifiedArtifacts, "graph_materialization"); e != nil {
			return e
		}
		if _, invalid := domain.ParseCount(r.EffectCount); invalid != nil {
			return invalid
		}
		if _, invalid := domain.ParseRoutingTime(r.CompletedAt); invalid != nil {
			return invalid
		}
		switch r.State {
		case "complete", "blocked", "incomplete", "uncertain":
		default:
			return domain.Invalid("observed materialization state")
		}
		switch r.EvidenceStatus {
		case "complete", "incomplete", "uncertain":
		default:
			return domain.Invalid("observed materialization evidence status")
		}
		if r.OperationEffects != nil {
			if r.OperationEffects.ProducerAttemptID != a.ID {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, *r.OperationEffects, f.VerifiedArtifacts, "graph_operation_effects"); e != nil {
				return e
			}
		}
		if r.EffectCount != "0" && r.OperationEffects == nil {
			return evidenceUnavailable()
		}
		complete := r.State == "complete"
		if complete && (!r.EffectsKnown || r.EvidenceStatus != "complete" || r.Failure != nil || f.SourceSnapshot == nil || r.ObservedManifest == nil || r.ObservedIndex == nil || r.SourceSnapshotID == nil || r.ObservedWorkingEntriesSHA256 == nil || r.ObservedIndexEntriesSHA256 == nil || *r.ObservedWorkingEntriesSHA256 != d.SourceBasis.ExpectedWorkingEntriesSHA256 || *r.ObservedIndexEntriesSHA256 != d.SourceBasis.ExpectedIndexEntriesSHA256 || f.WorkspaceIdentity == "" || len(f.WorkspaceIdentity) > 4096 || !domain.ValidSHA256(f.GitControlSHA256)) {
			return evidenceUnavailable()
		}
		if r.ObservedManifest != nil {
			if r.ObservedManifest.ProducerAttemptID != a.ID {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, *r.ObservedManifest, f.VerifiedArtifacts, "source_manifest"); e != nil {
				return e
			}
		}
		if r.ObservedIndex != nil {
			if r.ObservedIndex.ProducerAttemptID != a.ID {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, *r.ObservedIndex, f.VerifiedArtifacts, "index_snapshot"); e != nil {
				return e
			}
		}
		input, e := s.graphInputRecord(ctx, tx, d.GraphID, d.InputID)
		if e != nil {
			return e
		}
		if input.PrivateRoot == nil || input.Public.InputBinding == nil {
			return evidenceUnavailable()
		}
		if complete {
			if r.StagedBlobsSHA256 == nil || !domain.ValidSHA256(*r.StagedBlobsSHA256) {
				return evidenceUnavailable()
			}
			if d.SourceBasis.BasisKind == "graph_input" {
				if *r.StagedBlobsSHA256 != input.Public.InputBinding.StagedBlobs.SHA256 {
					return evidenceUnavailable()
				}
			} else {
				found := false
				for _, proof := range f.VerifiedArtifacts {
					if proof.SHA256 != *r.StagedBlobsSHA256 {
						continue
					}
					artifact, err := s.artifact(ctx, tx, proof.ArtifactID)
					if err != nil {
						return err
					}
					if artifact.AttemptID == a.ID && artifact.Kind == "graph_index_blobs" {
						ref := domain.GraphArtifactRef{ArtifactID: artifact.ID, ProducerAttemptID: a.ID, SHA256: proof.SHA256, ByteLength: proof.ByteLength}
						if e = s.verifyGraphRef(ctx, tx, ref, f.VerifiedArtifacts, "graph_index_blobs"); e != nil {
							return e
						}
						found = true
						break
					}
				}
				if !found {
					return evidenceUnavailable()
				}
			}
		}
		if f.SourceSnapshot != nil {
			source := *f.SourceSnapshot
			root := *input.PrivateRoot
			if r.SourceSnapshotID == nil || *r.SourceSnapshotID != source.ID || r.ObservedManifest == nil || r.ObservedIndex == nil || source.AttemptID != a.ID || source.ManifestArtifactID != r.ObservedManifest.ArtifactID || source.IndexArtifactID != r.ObservedIndex.ArtifactID || source.ManifestSHA256 != r.ObservedManifest.SHA256 || source.ProjectIdentity != root.ProjectIdentity || source.BaseCommit != root.BaseCommit || !reflect.DeepEqual(source.Branch, root.Branch) || source.SelectedRelativePath != root.SelectedRelativePath {
				return evidenceUnavailable()
			}
			if a.SourceSnapshotID == nil {
				if e = s.putSourceSnapshot(ctx, tx, source, true); e != nil {
					return e
				}
			} else {
				old, e := s.sourceSnapshot(ctx, tx, *a.SourceSnapshotID)
				if e != nil {
					return e
				}
				if !reflect.DeepEqual(old, source) {
					return evidenceUnavailable()
				}
			}
		} else if r.SourceSnapshotID != nil {
			return evidenceUnavailable()
		}
		record.Public.Materialization = &r
		record.MaterializationArtifact = &f.ReceiptArtifact
		record.PrivateMaterialization = &domain.GraphPrivateMaterialization{WorkspaceIdentity: f.WorkspaceIdentity, GitControlSHA256: f.GitControlSHA256}
		if e = saveGraphDispatch(ctx, tx, record); e != nil {
			return e
		}
		if !r.EffectsKnown || r.EvidenceStatus == "uncertain" || r.State == "uncertain" {
			incident := domain.ExecutionIncident{ID: domain.NewID(), AttemptID: &a.ID, Code: "graph_materialization_uncertain", Message: "Receiving materialization effects or evidence remain uncertain; retain the exact ownership and do not repeat preparation.", CreatedAt: domain.Now()}
			if e = s.fence(ctx, tx, incident); e != nil {
				return e
			}
		}
		_, e = s.appendEvent(ctx, tx, a.ID, "graph_materialization_observed", nil)
		return e
	})
}

func (s *Store) graphMaterializationUncertain(ctx context.Context, q queryer, a domain.Attempt) (bool, error) {
	member, e := s.graphMembership(ctx, q, a.TaskID)
	if e != nil || member == nil || member.Kind != "node" {
		return false, e
	}
	record, e := s.graphDispatchForAttempt(ctx, q, a.ID)
	if e != nil {
		return false, e
	}
	r := record.Public.Materialization
	return r != nil && (!r.EffectsKnown || r.EvidenceStatus == "uncertain" || r.State == "uncertain"), nil
}
