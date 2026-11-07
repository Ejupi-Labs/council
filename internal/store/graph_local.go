package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) graphInputOperation(ctx context.Context, q queryer, ownerID, attemptID string) (domain.GraphInputOperation, error) {
	var out domain.GraphInputOperation
	a, e := s.attempt(ctx, q, attemptID)
	if e != nil {
		return out, e
	}
	if a.ExecutorKind != "local_graph" || a.OwnerInstanceID != ownerID {
		return out, domain.Fail("owner_unavailable", "This is not the exact owned graph preparation operation.")
	}
	var graphID, inputID string
	e = q.QueryRowContext(ctx, "SELECT graph_id,id FROM graph_inputs WHERE operation_attempt_id=?", attemptID).Scan(&graphID, &inputID)
	if errors.Is(e, sql.ErrNoRows) {
		return out, notFound()
	}
	if e != nil {
		return out, storageError()
	}
	record, e := s.graphInputRecord(ctx, q, graphID, inputID)
	if e != nil {
		return out, e
	}
	h, e := s.graphHeader(ctx, q, graphID)
	if e != nil {
		return out, e
	}
	if a.TaskID != h.Summary.RootTaskID || record.ExpectedAuthority.OperationAttemptID != a.ID || record.ExpectedAuthority.OwnerInstanceID != ownerID || record.ExpectedAuthority.RootTaskID != a.TaskID {
		return out, storageError()
	}
	definition, e := s.graphDefinition(ctx, q, graphID, record.Public.Summary.GraphVersion)
	if e != nil {
		return out, e
	}
	out = domain.GraphInputOperation{CreateFacts: h.CreateFacts, ID: inputID, Kind: record.Public.Summary.Kind, GraphID: graphID, RootTaskID: a.TaskID, Attempt: a, RootBinding: record.PrivateRoot, Control: record.ExpectedAuthority, Definition: definition, InputRecord: record, DeadlineAt: a.DeadlineAt, Contributions: record.ExpectedContributions, ContributorVector: record.ExpectedAuthority.ContributorVector}
	if record.Public.InputBinding != nil {
		out.Contributions = record.Public.InputBinding.Contributions
	}
	if record.Public.Summary.ReceivingNodeID != nil {
		out.NodeAuthority = &domain.GraphNodeAuthority{NodeID: *record.Public.Summary.ReceivingNodeID, TaskVersion: *record.Public.Summary.ReceivingTaskVersion}
		if record.ExpectedAuthority.NodeControlRevision != nil {
			out.NodeAuthority.NodeControlRevision = *record.ExpectedAuthority.NodeControlRevision
		}
	}
	return out, nil
}
func (s *Store) graphInputOperationForAttemptPublic(ctx context.Context, ownerID, attemptID string) (domain.GraphInputOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return domain.GraphInputOperation{}, e
	}
	out, e := s.graphInputOperation(ctx, s.db, ownerID, attemptID)
	return out, beforeCommitError(ctx, e)
}
func (s *Store) graphLocalControls(ctx context.Context, q queryer, ownerID string, op domain.GraphInputOperation) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	a := op.Attempt
	g, e := s.gate(ctx, q)
	if e != nil {
		return e
	}
	if e = checkGate(g); e != nil {
		return e
	}
	if g.OwnerInstanceID != ownerID || a.OwnerInstanceID != ownerID {
		return domain.Fail("owner_unavailable", "The local graph operation belongs to a different owner epoch.")
	}
	if a.Phase != "preparing" || a.Stop != nil || a.PrimaryFailure != nil {
		return domain.Fail("graph_control_conflict", "This operation is stopped, settled or no longer eligible for local effects.")
	}
	if e = s.checkAttemptClaim(ctx, q, ownerID, a.ID); e != nil {
		return e
	}
	h, e := s.graphHeader(ctx, q, op.GraphID)
	if e != nil {
		return e
	}
	c := op.Control
	// A claimed operation owns its persisted exact scope. Later registrations
	// govern new admissions; their generation/suspension does not revoke this
	// independent I/O. Current controls, owner/claim, clock and inputs still do.
	if h.Summary.Stop != nil || h.Summary.State == "cancelled" || h.Summary.State == "cancelling" || h.Summary.State == "completed" || h.Summary.GraphVersion != c.GraphVersion || h.Summary.ControlRevision != c.GraphControlRevision || h.Summary.BudgetAuthorityEpoch != c.BudgetAuthorityEpoch || h.DispatchAuthority == nil || h.DispatchAuthority.OwnerInstanceID != ownerID {
		return domain.Fail("graph_control_conflict", "Current graph authority no longer permits this local operation.")
	}
	b, e := s.rawTaskBudget(ctx, q, op.RootTaskID)
	if e != nil {
		return e
	}
	if b == nil || b.CurrentWindow.Number != c.WindowNumber {
		return domain.Fail("budget_conflict", "The captured graph budget window changed before this local operation.")
	}
	now := time.Now().UTC()
	deadline, invalid := domain.ParseRoutingTime(a.DeadlineAt)
	if invalid != nil {
		return storageError()
	}
	if !now.Before(deadline) {
		return domain.Fail("budget_expired", "The bounded local graph preparation deadline expired.")
	}
	if b.CurrentWindow.DeadlineAt != nil {
		common, invalid := domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
		if invalid != nil {
			return storageError()
		}
		if !now.Before(common) {
			return domain.Fail("budget_expired", "The common graph window expired before this local operation.")
		}
	}
	if c.NodeID != nil {
		nodes, e := s.graphNodes(ctx, q, op.GraphID)
		if e != nil {
			return e
		}
		found := false
		for _, n := range nodes {
			if n.Summary.ID == *c.NodeID {
				found = true
				task, e := s.task(ctx, q, n.Summary.TaskID)
				if e != nil {
					return e
				}
				if c.TaskID == nil || *c.TaskID != task.ID || c.TaskVersion == nil || *c.TaskVersion != task.CurrentVersion || c.NodeControlRevision == nil || *c.NodeControlRevision != n.Summary.ControlRevision || n.Summary.Stop != nil {
					return domain.Fail("graph_input_stale", "The receiving node authority changed before input preparation.")
				}
			}
		}
		if !found {
			return storageError()
		}
	}
	return s.graphContributorsCurrent(ctx, q, op.GraphID, c.ContributorVector)
}
func (s *Store) graphContributorsCurrent(ctx context.Context, q queryer, graphID string, vector []domain.GraphContributorBinding) error {
	if len(vector) > domain.MaxGraphNodes {
		return domain.Invalid("bounded graph contributor vector")
	}
	if len(vector) == 0 {
		return nil
	}
	nodes, e := s.graphNodes(ctx, q, graphID)
	if e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, v := range vector {
		if seen[v.NodeID] {
			return evidenceUnavailable()
		}
		seen[v.NodeID] = true
		found := false
		for _, n := range nodes {
			if n.Summary.ID == v.NodeID {
				found = true
				t, e := s.task(ctx, q, n.Summary.TaskID)
				if e != nil {
					return e
				}
				r := n.Summary.AcceptedResult
				if t.ID != v.TaskID || t.CurrentVersion != v.TaskVersion || r == nil || r.TaskVersion != v.TaskVersion || r.AcceptanceKind != v.AcceptanceKind || r.AcceptanceID != v.AcceptanceID || r.ProducerAttemptID != v.ProducerAttemptID || r.InputID != v.InputID {
					return domain.Fail("graph_input_stale", "A required accepted contribution no longer identifies the current node outcome.")
				}
			}
		}
		if !found {
			return evidenceUnavailable()
		}
	}
	return nil
}
func (s *Store) checkGraphLocalOperationPublic(ctx context.Context, ownerID, attemptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	op, e := s.graphInputOperation(ctx, s.db, ownerID, attemptID)
	if e == nil {
		e = s.graphLocalControls(ctx, s.db, ownerID, op)
	}
	return beforeCommitError(ctx, e)
}
func (s *Store) verifyGraphRef(ctx context.Context, q queryer, r domain.GraphArtifactRef, proofs []domain.VerifiedResponse, kinds ...string) error {
	if invalid := domain.ValidateGraphArtifactRef(r); invalid != nil {
		return invalid
	}
	a, e := s.artifact(ctx, q, r.ArtifactID)
	if e != nil {
		return e
	}
	allowed := false
	for _, kind := range kinds {
		if a.Kind == kind {
			allowed = true
		}
	}
	if !allowed || a.AttemptID != r.ProducerAttemptID || a.SHA256 == nil || *a.SHA256 != r.SHA256 || a.ByteLength != r.ByteLength {
		return evidenceUnavailable()
	}
	return s.verifyWorkflowProofs(ctx, q, proofs, r.ArtifactID)
}
func graphCompletionFingerprint(f domain.GraphInputCompletionFacts) string {
	f.VerifiedArtifacts = nil
	return Fingerprint("graphs.input.complete", f)
}
func (s *Store) completeGraphInput(ctx context.Context, ownerID, attemptID string, f domain.GraphInputCompletionFacts) error {
	fingerprint := graphCompletionFingerprint(f)
	return s.executionWrite(ctx, func(tx *sql.Tx) error {
		op, e := s.graphInputOperation(ctx, tx, ownerID, attemptID)
		if e != nil {
			return e
		}
		record := op.InputRecord
		if record.CompletionFingerprint != "" {
			if record.CompletionFingerprint == fingerprint {
				return nil
			}
			return domain.Fail("request_conflict", "The completed graph operation already names different immutable input facts.")
		}
		if op.Attempt.Phase == "settled" {
			return domain.Fail("attempt_active", "This local operation is already terminal.")
		}
		switch f.State {
		case "ready", "blocked", "conflicted", "incomplete", "uncertain", "interrupted":
		default:
			return domain.Invalid("graph input completion state")
		}
		if f.State == "conflicted" && (op.Kind != "node_input" || f.ConflictArtifact == nil || f.InputBinding != nil || f.RootBinding != nil || !f.EffectsKnown || f.EvidenceStatus != "complete") {
			return evidenceUnavailable()
		}
		if f.EvidenceStatus != "complete" && f.EvidenceStatus != "incomplete" && f.EvidenceStatus != "uncertain" {
			return domain.Invalid("graph operation evidence status")
		}
		if _, invalid := domain.ParseCount(f.EffectCount); invalid != nil {
			return invalid
		}
		if f.Cleanup.State != "verified" && f.Cleanup.State != "unknown" {
			return domain.Invalid("known graph operation cleanup")
		}
		if f.Cleanup.Scope != "not_launched" {
			return domain.Invalid("no native launch for graph preparation")
		}
		h, e := s.graphHeader(ctx, tx, op.GraphID)
		if e != nil {
			return e
		}
		if f.State == "ready" {
			if f.Failure != nil || !f.EffectsKnown || f.EvidenceStatus != "complete" || f.Cleanup.State != "verified" {
				return evidenceUnavailable()
			}
			if e = s.graphLocalControls(ctx, tx, ownerID, op); e != nil {
				de := domain.SafeError(e)
				switch de.Code {
				case "graph_control_conflict", "graph_input_stale", "budget_expired", "budget_conflict", "owner_closing", "dispatch_fenced":
					f.State = "blocked"
					f.Failure = de
				default:
					return e
				}
			}
		}
		if f.OperationEffects != nil {
			if f.OperationEffects.ProducerAttemptID != attemptID {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, *f.OperationEffects, f.VerifiedArtifacts, "graph_operation_effects"); e != nil {
				return e
			}
		} else if f.EffectCount != "0" {
			return evidenceUnavailable()
		}
		if f.ConflictArtifact != nil {
			if f.ConflictArtifact.ProducerAttemptID != attemptID {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, *f.ConflictArtifact, f.VerifiedArtifacts, "graph_input_provenance"); e != nil {
				return e
			}
		}
		if f.RootBinding != nil {
			root := *f.RootBinding
			if invalid := domain.ValidateGraphRootBinding(root); invalid != nil {
				return invalid
			}
			digest, e := domain.GraphRootDigest(root)
			if e != nil || digest != root.BasisSHA256 {
				return evidenceUnavailable()
			}
			if op.Kind != "root_basis" || root.GraphID != op.GraphID || root.RootTaskID != op.RootTaskID || root.OperationAttemptID != attemptID || root.ProjectID != h.Summary.ProjectID || root.ProjectIdentity != h.CreateFacts.ProjectIdentity || root.RepositoryIdentity != h.CreateFacts.RepositoryIdentity || root.SelectedRelativePath != h.CreateFacts.SelectedRelativePath || root.BaseCommit != h.CreateFacts.BaseCommit || !reflect.DeepEqual(root.Branch, h.CreateFacts.Branch) {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, root.Manifest, f.VerifiedArtifacts, "source_manifest"); e != nil {
				return e
			}
			if e = s.verifyGraphRef(ctx, tx, root.Index, f.VerifiedArtifacts, "index_snapshot"); e != nil {
				return e
			}
			if e = s.verifyGraphRef(ctx, tx, root.StagedBlobs, f.VerifiedArtifacts, "graph_index_blobs"); e != nil {
				return e
			}
			if f.SourceSnapshot == nil || f.SourceSnapshot.ID != root.SourceSnapshotID || f.SourceSnapshot.AttemptID != attemptID || f.SourceSnapshot.ManifestArtifactID != root.Manifest.ArtifactID || f.SourceSnapshot.IndexArtifactID != root.Index.ArtifactID || f.SourceSnapshot.ManifestSHA256 != root.Manifest.SHA256 || f.SourceSnapshot.ProjectIdentity != root.ProjectIdentity || f.SourceSnapshot.BaseCommit != root.BaseCommit || f.SourceSnapshot.SelectedRelativePath != root.SelectedRelativePath || !reflect.DeepEqual(f.SourceSnapshot.Branch, root.Branch) {
				return evidenceUnavailable()
			}
			if e = s.putSourceSnapshot(ctx, tx, *f.SourceSnapshot, true); e != nil {
				return e
			}
			public := domain.GraphRootPublic(root)
			record.Public.RootBinding = &public
			record.PrivateRoot = &root
		}
		if f.InputBinding != nil {
			input := *f.InputBinding
			if invalid := domain.ValidateGraphInputBinding(input); invalid != nil {
				return invalid
			}
			if op.Kind != "node_input" || input.ID != op.ID || input.GraphID != op.GraphID || input.GraphVersion != op.Control.GraphVersion || input.OperationAttemptID != attemptID || op.Control.NodeID == nil || input.ReceivingNodeID != *op.Control.NodeID || op.Control.TaskID == nil || input.ReceivingTaskID != *op.Control.TaskID || op.Control.TaskVersion == nil || input.ReceivingTaskVersion != *op.Control.TaskVersion || !reflect.DeepEqual(input.ContributorVector, op.Control.ContributorVector) {
				return evidenceUnavailable()
			}
			digest, e := domain.GraphInputDigest(input)
			if e != nil || digest != input.InputSHA256 {
				return evidenceUnavailable()
			}
			for _, pair := range []struct {
				ref  domain.GraphArtifactRef
				kind string
			}{{input.Manifest, "graph_input_manifest"}, {input.Index, "index_snapshot"}, {input.StagedBlobs, "graph_index_blobs"}, {input.Provenance, "graph_input_provenance"}, {input.ContextPacket, "graph_context"}} {
				if pair.ref.ProducerAttemptID != attemptID {
					return evidenceUnavailable()
				}
				if e = s.verifyGraphRef(ctx, tx, pair.ref, f.VerifiedArtifacts, pair.kind); e != nil {
					return e
				}
			}
			if f.ContextPacket == nil || f.ConditionsRef == nil || f.ContextPacket.GraphID != op.GraphID || f.ContextPacket.InputID != input.ID || f.ContextPacket.InputSHA256 != input.InputSHA256 || f.ContextPacket.ReceivingTaskVersion != input.ReceivingTaskVersion {
				return evidenceUnavailable()
			}
			if op.RootBinding == nil || input.RootBasisSHA256 != op.RootBinding.BasisSHA256 || !reflect.DeepEqual(input.Contributions, op.InputRecord.ExpectedContributions) {
				return evidenceUnavailable()
			}
			if e = s.graphInputContext(ctx, tx, op, input, f); e != nil {
				return e
			}
			record.Public.InputBinding = &input
			record.Public.Summary.InputSHA256 = ptr(input.InputSHA256)
			record.Public.Summary.RootBasisSHA256 = ptr(input.RootBasisSHA256)
		}
		if op.Kind == "node_input" && f.SourceSnapshot != nil {
			if e = s.graphAssemblySnapshot(ctx, tx, op, f); e != nil {
				return e
			}
		}
		if f.State == "ready" && (op.Kind == "root_basis" && f.RootBinding == nil || op.Kind == "node_input" && f.InputBinding == nil) {
			return evidenceUnavailable()
		}
		if f.ConditionsRef != nil {
			if f.ConditionsRef.ProducerAttemptID != attemptID {
				return evidenceUnavailable()
			}
			if e = s.verifyGraphRef(ctx, tx, *f.ConditionsRef, f.VerifiedArtifacts, "graph_conditions"); e != nil {
				return e
			}
			record.Public.ConditionsRef = f.ConditionsRef
		}
		record.ConditionsBinding = f.ConditionsBinding
		record.Public.ContextPacket = f.ContextPacket
		record.Public.OperationEffects = f.OperationEffects
		record.Public.ConflictArtifact = f.ConflictArtifact
		record.Public.Summary.State = f.State
		record.Public.Summary.Failure = f.Failure
		record.Public.Summary.Current = f.State == "ready"
		record.Public.Summary.UpdatedAt = domain.Now()
		record.CompletionFingerprint = fingerprint
		var manifest, index, blobs, provenance, contextArtifact, conditions, source any
		if op.Kind == "node_input" && f.SourceSnapshot != nil {
			source = f.SourceSnapshot.ID
		}
		if record.Public.RootBinding != nil {
			r := record.Public.RootBinding
			manifest = r.Manifest.ArtifactID
			index = r.Index.ArtifactID
			blobs = r.StagedBlobs.ArtifactID
			source = r.SourceSnapshotID
			record.Public.Summary.RootBasisSHA256 = ptr(r.BasisSHA256)
		}
		if record.Public.InputBinding != nil {
			r := record.Public.InputBinding
			manifest = r.Manifest.ArtifactID
			index = r.Index.ArtifactID
			blobs = r.StagedBlobs.ArtifactID
			provenance = r.Provenance.ArtifactID
			contextArtifact = r.ContextPacket.ArtifactID
		}
		if record.Public.ConditionsRef != nil {
			conditions = record.Public.ConditionsRef.ArtifactID
		}
		if _, e = tx.ExecContext(ctx, "UPDATE graph_inputs SET state=?,manifest_artifact_id=?,index_artifact_id=?,staged_blobs_artifact_id=?,provenance_artifact_id=?,context_artifact_id=?,conditions_artifact_id=?,source_snapshot_id=?,data=? WHERE id=?", f.State, manifest, index, blobs, provenance, contextArtifact, conditions, source, encode(record), op.ID); e != nil {
			return storageError()
		}
		a, e := s.attempt(ctx, tx, attemptID)
		if e != nil {
			return e
		}
		outcome := "failed"
		if f.State == "ready" {
			outcome = "succeeded"
		} else if f.State == "blocked" || f.State == "conflicted" {
			outcome = "blocked"
		} else if f.State == "interrupted" {
			outcome = "interrupted"
		}
		var failure *domain.PrimaryFailure
		if f.Failure != nil {
			failure = &domain.PrimaryFailure{Code: f.Failure.Code, Message: f.Failure.Message}
		}
		a, e = s.settle(ctx, tx, a, domain.Settlement{Outcome: outcome, Cleanup: f.Cleanup, EvidenceStatus: f.EvidenceStatus, Failure: failure, EffectsKnown: f.EffectsKnown, InputComplete: f.State == "ready", UsableOutput: f.State == "ready", NoProcessCreated: true})
		if e != nil {
			return e
		}
		if a.Stop != nil {
			record.Public.Summary.Current = false
			record.Public.Summary.State = "blocked"
			if _, e = tx.ExecContext(ctx, "UPDATE graph_inputs SET state='blocked',data=? WHERE id=?", encode(record), op.ID); e != nil {
				return storageError()
			}
		}
		if op.Control.NodeID != nil {
			nodes, readErr := s.graphNodes(ctx, tx, op.GraphID)
			if readErr != nil {
				return readErr
			}
			for i := range nodes {
				n := &nodes[i]
				if n.Summary.ID != *op.Control.NodeID || op.Control.NodeControlRevision == nil || n.Summary.ControlRevision != *op.Control.NodeControlRevision || h.Summary.Stop != nil || n.Summary.Stop != nil || graphRetainedTerminal(n.Summary.State) {
					continue
				}
				n.Summary.State = "blocked"
				n.Summary.NextOwner = "user"
				n.Summary.NextAction = "Inspect retained input preparation evidence."
				if f.State == "ready" && a.Stop == nil {
					n.Summary.State = "ready"
					n.Summary.NextOwner = "council"
					n.Summary.NextAction = "Admit this exact input under the shared graph allowance."
				}
				n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
				if e != nil {
					return e
				}
				if e = saveGraphNode(ctx, tx, *n); e != nil {
					return e
				}
			}
			h.Summary.Counts, e = graphCounts(nodes)
			if e != nil {
				return e
			}
		}
		h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
		if e != nil {
			return e
		}
		h.Summary.UpdatedAt = domain.Now()
		if h.Summary.Stop != nil {
			pending, readErr := s.graphPending(ctx, tx, h)
			if readErr != nil {
				return readErr
			}
			h.Summary.State = "cancelled"
			if len(pending) != 0 {
				h.Summary.State = "cancelling"
			}
			h.Summary.NextOwner = "user"
			h.Summary.NextAction = "Inspect stopped work and retained partial results."
		} else if a.Outcome != nil && *a.Outcome == "succeeded" {
			h.Summary.State = "queued"
			h.Summary.NextAction = "Prepare current ready node inputs."
			h.Summary.NextOwner = "council"
		} else if op.Kind == "node_input" && f.EffectsKnown && f.EvidenceStatus != "uncertain" && f.Cleanup.State == "verified" {
			h, e = s.scopeGraphInputFailure(ctx, tx, h, op)
			if e != nil {
				return e
			}
		} else {
			h.Summary.State = "blocked"
			h.Summary.NextAction = "Inspect retained graph preparation evidence and resume explicitly."
			h.Summary.NextOwner = "user"
			if h.DispatchAuthority != nil {
				h.DispatchAuthority.State = "suspended"
			}
		}
		return saveGraphHeader(ctx, tx, h)
	})
}
