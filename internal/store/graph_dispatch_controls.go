package store

import (
	"context"
	"database/sql"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) graphDispatchForAttempt(ctx context.Context, q queryer, attemptID string) (domain.GraphDispatchRecord, error) {
	var graphID, id string
	if e := q.QueryRowContext(ctx, "SELECT graph_id,id FROM graph_dispatches WHERE attempt_id=?", attemptID).Scan(&graphID, &id); e != nil {
		return domain.GraphDispatchRecord{}, graphRowError(e)
	}
	return s.graphDispatchRecord(ctx, q, graphID, id)
}

// This is an authority check only. The controller independently rechecks the
// exact private workspace/context handles immediately before native preparation.
func (s *Store) CheckGraphDispatchPreparation(ctx context.Context, ownerID, attemptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	a, e := s.attempt(ctx, s.db, attemptID)
	if e != nil {
		return e
	}
	return beforeCommitError(ctx, s.graphDispatchControls(ctx, s.db, ownerID, a, false))
}

func (s *Store) graphDispatchControls(ctx context.Context, q queryer, ownerID string, a domain.Attempt, requireMaterialization bool) error {
	member, e := s.graphMembership(ctx, q, a.TaskID)
	if e != nil {
		return e
	}
	if member == nil {
		return nil
	}
	if member.Kind != "node" || member.NodeID == nil {
		return graphScopeError(member)
	}
	gate, e := s.gate(ctx, q)
	if e != nil {
		return e
	}
	if e = checkGate(gate); e != nil {
		return e
	}
	if ownerID != a.OwnerInstanceID || gate.OwnerInstanceID != ownerID || a.Phase != "preparing" || a.Stop != nil || a.PrimaryFailure != nil {
		return graphControlError()
	}
	if e = s.checkAttemptClaim(ctx, q, ownerID, a.ID); e != nil {
		return e
	}
	record, e := s.graphDispatchForAttempt(ctx, q, a.ID)
	if e != nil {
		return e
	}
	d := record.Public.Dispatch
	if d.GraphID != member.GraphID || d.NodeID != *member.NodeID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || d.OwnerInstanceID != ownerID || d.WorkflowID != a.WorkflowID || d.StepID != a.StepID || d.ExecutorKind != attemptExecutor(a) || d.Role != a.Role {
		return evidenceUnavailable()
	}
	h, e := s.graphHeader(ctx, q, member.GraphID)
	if e != nil {
		return e
	}
	if h.Summary.GraphVersion != d.GraphVersion || h.Summary.ControlRevision != d.GraphControlRevision || h.Summary.BudgetAuthorityEpoch != d.BudgetAuthorityEpoch || h.Summary.Stop != nil {
		return graphControlError()
	}
	nodes, e := s.graphNodes(ctx, q, d.GraphID)
	if e != nil {
		return e
	}
	found := false
	ancestors := map[string]bool{}
	for _, node := range nodes {
		n := node.Summary
		if n.ID != d.NodeID {
			continue
		}
		found = true
		if n.Stop != nil || n.TaskVersion != d.TaskVersion || n.ControlRevision != d.NodeControlRevision || n.InputID == nil || *n.InputID != d.InputID || n.NodeAdmissionID == nil || *n.NodeAdmissionID != d.NodeAdmissionID {
			return graphControlError()
		}
		ancestors[n.Key] = true
	}
	if !found {
		return evidenceUnavailable()
	}
	task, e := s.task(ctx, q, a.TaskID)
	if e != nil {
		return e
	}
	if task.CurrentVersion != d.TaskVersion {
		return graphControlError()
	}
	// Ancestor Stop prevents first GO. Once a receiver has actually started, an
	// ancestor's Stop does not retcon its already captured accepted contribution.
	anchor, e := s.graphNodeAdmissionAnchor(ctx, q, d)
	if e != nil {
		return e
	}
	if anchor.KnownFirstGOAt == nil {
		definition, e := s.graphDefinition(ctx, q, d.GraphID, d.GraphVersion)
		if e != nil {
			return e
		}
		if definition.Original.Dependencies != nil {
			for i := 0; i < len(nodes); i++ {
				for _, edge := range *definition.Original.Dependencies {
					if ancestors[edge.To] {
						ancestors[edge.From] = true
					}
				}
			}
		}
		for _, node := range nodes {
			if node.Summary.ID != d.NodeID && ancestors[node.Summary.Key] && node.Summary.Stop != nil {
				return graphControlError()
			}
		}
	}
	b, e := s.taskBudget(ctx, q, a.TaskID)
	if e != nil {
		return e
	}
	if b == nil || b.CurrentWindow.Number != d.WindowNumber {
		return graphControlError()
	}
	var deadline time.Time
	if b.CurrentWindow.DeadlineAt == nil {
		localDeadline, err := s.graphUnopenedLocalDeadline(ctx, q, a, b)
		if err != nil {
			return err
		}
		if localDeadline == nil {
			return graphControlError()
		}
		deadline = *localDeadline
	} else {
		var invalid *domain.Error
		deadline, invalid = domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
		if invalid != nil {
			return storageError()
		}
	}
	if !time.Now().UTC().Before(deadline) || expired(a) {
		return domain.Fail("budget_expired", "The common graph window or captured attempt deadline expired before preparation.")
	}
	input, e := s.graphInputRecord(ctx, q, d.GraphID, d.InputID)
	if e != nil {
		return e
	}
	if !input.Public.Summary.Current || input.Public.Summary.State != "ready" || input.Public.InputBinding == nil || input.Public.InputBinding.InputSHA256 != d.InputSHA256 || input.Public.InputBinding.ContextPacket != d.ContextPacket || !reflect.DeepEqual(input.Public.InputBinding.ContributorVector, d.ContributorVector) {
		return domain.Fail("graph_input_stale", "This receiving execution no longer has its exact current retained input.")
	}
	if e = s.graphContributorsCurrent(ctx, q, d.GraphID, d.ContributorVector); e != nil {
		return e
	}
	w, e := s.workflowByID(ctx, q, d.WorkflowID)
	if e != nil {
		return e
	}
	if e = s.graphStageFinalRef(ctx, q, w, d.SourceBasis, nil, false); e != nil {
		return e
	}
	if e = s.graphExternalDispatchScope(ctx, q, a, h, d, w, *input.Public.InputBinding, nodes); e != nil {
		return e
	}
	if requireMaterialization {
		r := record.Public.Materialization
		if r == nil || record.MaterializationArtifact == nil || record.PrivateMaterialization == nil || record.PrivateMaterialization.WorkspaceIdentity == "" || !domain.ValidSHA256(record.PrivateMaterialization.GitControlSHA256) || r.State != "complete" || !r.EffectsKnown || r.EvidenceStatus != "complete" || r.Failure != nil || r.SourceSnapshotID == nil || a.SourceSnapshotID == nil || *r.SourceSnapshotID != *a.SourceSnapshotID || !reflect.DeepEqual(r.SourceBasis, d.SourceBasis) {
			return evidenceUnavailable()
		}
	}
	return nil
}

func saveGraphDispatch(ctx context.Context, tx *sql.Tx, r domain.GraphDispatchRecord) error {
	var source *string
	if r.Public.Materialization != nil {
		source = r.Public.Materialization.SourceSnapshotID
	}
	var private any
	if r.PrivateMaterialization != nil {
		private = encode(r.PrivateMaterialization)
	}
	var artifact *string
	if r.MaterializationArtifact != nil {
		artifact = &r.MaterializationArtifact.ArtifactID
	}
	var output any
	if r.Public.OutputObservation != nil {
		output = encode(r.Public.OutputObservation)
	}
	_, e := tx.ExecContext(ctx, "UPDATE graph_dispatches SET materialization_artifact_id=?,source_snapshot_id=?,materialization_private_data=?,output_data=?,known_first_go_at=?,known_first_go_attempt_id=?,data=? WHERE id=?", artifact, source, private, output, r.KnownFirstGOAt, r.KnownFirstGOAttemptID, encode(r), r.Public.Dispatch.ID)
	if e != nil {
		return storageError()
	}
	return nil
}
