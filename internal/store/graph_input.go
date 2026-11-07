package store

import (
	"context"
	"database/sql"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphAuthorizedNode(h graphHeader, n domain.GraphNodeSummary) bool {
	if h.DispatchAuthority == nil {
		return false
	}
	for _, a := range h.DispatchAuthority.NodeAuthorities {
		if a.NodeID == n.ID && a.TaskVersion == n.TaskVersion && a.NodeControlRevision == n.ControlRevision {
			return true
		}
	}
	return false
}
func (s *Store) graphFreshTrigger(ctx context.Context, q queryer, h graphHeader, ownerID string, trigger domain.GraphFreshTrigger, nodeID ...string) error {
	if !domain.ValidID(trigger.ID) || trigger.OwnerGeneration != h.DispatchAuthority.Generation {
		return graphControlError()
	}
	if trigger.Kind == "explicit" {
		if trigger.ID != h.DispatchRequestID {
			if len(nodeID) != 1 {
				return graphControlError()
			}
			scoped, err := s.recoveryGraphScopedTrigger(ctx, q, h, ownerID, trigger, nodeID[0])
			if err != nil {
				return err
			}
			if !scoped {
				return graphControlError()
			}
		}
		return nil
	}
	if trigger.Kind == "response_ack" {
		sequence, _, e := s.graphResponseAckEvent(ctx, q, h, trigger.ID, ownerID)
		if e != nil {
			return e
		}
		highwater, invalid := domain.ParseCount(h.DispatchEventHighWater)
		if invalid != nil {
			return storageError()
		}
		if sequence <= highwater {
			return graphControlError()
		}
		return nil
	}
	if trigger.Kind != "completion" && trigger.Kind != "claim_release" {
		return domain.Invalid("fresh graph dispatch trigger")
	}
	source, e := s.attempt(ctx, q, trigger.ID)
	if e != nil {
		return e
	}
	if source.OwnerInstanceID != ownerID || source.Phase != "settled" || source.Cleanup.State != "verified" {
		return graphControlError()
	}
	claims, e := s.graphClaims(ctx, q)
	if e != nil {
		return e
	}
	if claimedAttempt(claims, source.ID) {
		return graphControlError()
	}
	highwater, invalid := domain.ParseCount(h.DispatchEventHighWater)
	if invalid != nil {
		return storageError()
	}
	var terminalSequence int64
	if e = q.QueryRowContext(ctx, "SELECT sequence FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='settled' ORDER BY sequence DESC LIMIT 1", source.ID).Scan(&terminalSequence); e != nil {
		return storageError()
	}
	if terminalSequence <= highwater {
		return graphControlError()
	}
	var preparations int
	if e = q.QueryRowContext(ctx, "SELECT count(*) FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='preparation_started'", source.ID).Scan(&preparations); e != nil {
		return storageError()
	}
	if preparations == 0 {
		var reservations int
		ownership := "d.native_consumption_key=e.attempt_id"
		if attemptExecutor(source) != "native" {
			ownership = "d.native_consumption_key IS NULL"
		}
		if e = q.QueryRowContext(ctx, "SELECT count(*) FROM attempt_events e JOIN graph_dispatches d ON d.attempt_id=e.attempt_id WHERE e.attempt_id=? AND json_extract(e.data,'$.kind')='capacity_reserved' AND json_extract(e.data,'$.metadata.ownerInstanceId')=? AND json_extract(e.data,'$.metadata.slot') IN ('1','2') AND "+ownership, source.ID, ownerID).Scan(&reservations); e != nil {
			return storageError()
		}
		if reservations == 0 {
			return graphControlError()
		}
	}
	if trigger.Kind == "completion" {
		member, e := s.graphMembership(ctx, q, source.TaskID)
		if e != nil {
			return e
		}
		if member == nil || member.GraphID != h.Summary.ID {
			return graphControlError()
		}
	}
	return nil
}

// A member resume is its own exact persisted registration. It never changes
// the shared graph registration used by independent siblings.
func (s *Store) recoveryGraphScopedTrigger(ctx context.Context, q queryer, h graphHeader, owner string, trigger domain.GraphFreshTrigger, nodeID string) (bool, error) {
	if trigger.Kind != "explicit" && trigger.Kind != "completion" && trigger.Kind != "claim_release" || !domain.ValidID(trigger.ID) || !domain.ValidID(nodeID) || h.DispatchAuthority == nil {
		return false, nil
	}
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM recovery_graph_scopes r
		JOIN recovery_checkpoints c ON c.id=r.checkpoint_id
		JOIN graph_nodes n ON n.id=r.node_id AND n.graph_id=r.graph_id AND n.task_id=r.task_id
		JOIN tasks t ON t.id=r.task_id
		WHERE ((?='explicit' AND r.request_id=?) OR (?='completion' AND EXISTS (SELECT 1 FROM graph_inputs i WHERE i.operation_attempt_id=? AND i.graph_id=r.graph_id AND i.node_id=r.node_id AND i.state='ready' AND json_extract(i.data,'$.Trigger.Kind')='explicit' AND json_extract(i.data,'$.Trigger.ID')=r.request_id)) OR (?='claim_release' AND json_type(r.data,'$.eventHighWater')='integer' AND EXISTS (SELECT 1 FROM attempt_events e WHERE e.attempt_id=? AND json_extract(e.data,'$.kind')='settled' AND e.sequence>json_extract(r.data,'$.eventHighWater'))))
		AND r.owner_instance_id=? AND r.graph_id=? AND r.node_id=?
		AND r.state='active' AND c.state='resumed' AND c.scope_kind='member'
		AND c.graph_id=r.graph_id AND c.node_id=r.node_id AND c.task_id=r.task_id
		AND r.graph_version=? AND r.graph_control_revision=? AND r.node_control_revision=n.control_revision
		AND r.generation=? AND t.current_version=CAST(json_extract(n.data,'$.summary.taskVersion') AS INTEGER)`, trigger.Kind, trigger.ID, trigger.Kind, trigger.ID, trigger.Kind, trigger.ID, owner, h.Summary.ID, nodeID,
		intRevision(h.Summary.GraphVersion), intRevision(h.Summary.ControlRevision), intRevision(trigger.OwnerGeneration)).Scan(&count)
	if err != nil {
		return false, storageError()
	}
	return count == 1, nil
}

// RecoveryScopedInput returns only a real completed member preparation whose
// exact node-scoped resume authority is still current. It admits nothing.
func (s *Store) RecoveryScopedInput(ctx context.Context, ownerID, attemptID string) (domain.GraphInputOperation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var graphID, nodeID string
	err := s.db.QueryRowContext(ctx, "SELECT graph_id,node_id FROM graph_inputs WHERE operation_attempt_id=? AND kind='node_input' AND state='ready'", attemptID).Scan(&graphID, &nodeID)
	if err == sql.ErrNoRows {
		return domain.GraphInputOperation{}, false, nil
	}
	if err != nil {
		return domain.GraphInputOperation{}, false, storageError()
	}
	op, err := s.graphInputOperation(ctx, s.db, ownerID, attemptID)
	if err != nil {
		return domain.GraphInputOperation{}, false, err
	}
	if op.Attempt.Phase != "settled" || op.Attempt.Outcome == nil || *op.Attempt.Outcome != "succeeded" || op.Attempt.Cleanup.State != "verified" || op.Attempt.EvidenceStatus != "complete" {
		return domain.GraphInputOperation{}, false, nil
	}
	h, err := s.graphHeader(ctx, s.db, graphID)
	if err != nil {
		return domain.GraphInputOperation{}, false, err
	}
	trigger := domain.GraphFreshTrigger{Kind: "completion", ID: attemptID, OwnerGeneration: op.Control.Generation}
	scoped, err := s.recoveryGraphScopedTrigger(ctx, s.db, h, ownerID, trigger, nodeID)
	return op, scoped, err
}

// RecoveryScopedPredecessor proves that a settled workflow stage belongs to
// the exact current member registration, not merely to the same graph.
func (s *Store) RecoveryScopedPredecessor(ctx context.Context, ownerID, attemptID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.attempt(ctx, s.db, attemptID)
	if err != nil {
		return false, err
	}
	if a.OwnerInstanceID != ownerID || a.Phase != "settled" || a.Cleanup.State != "verified" || a.EvidenceStatus != "complete" {
		return false, nil
	}
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM recovery_graph_scopes r
		JOIN recovery_checkpoints c ON c.id=r.checkpoint_id
		JOIN receipts receipt ON receipt.request_id=r.request_id AND receipt.operation='checkpoints.resume'
		JOIN graph_dispatches d ON d.graph_id=r.graph_id AND d.node_id=r.node_id AND d.task_id=r.task_id
		JOIN task_graphs g ON g.id=r.graph_id
		JOIN graph_nodes n ON n.id=r.node_id AND n.graph_id=r.graph_id
		WHERE d.attempt_id=? AND d.owner_instance_id=? AND r.owner_instance_id=?
		AND r.state='active' AND c.state='resumed' AND c.scope_kind='member'
		AND c.graph_id=r.graph_id AND c.node_id=r.node_id AND c.task_id=r.task_id
		AND d.graph_version=r.graph_version AND d.graph_control_revision=r.graph_control_revision
		AND d.node_control_revision=r.node_control_revision AND g.graph_version=r.graph_version
		AND g.control_revision=r.graph_control_revision AND n.control_revision=r.node_control_revision
		AND NOT EXISTS (SELECT 1 FROM capacity_claims claim WHERE claim.attempt_id=d.attempt_id)`, attemptID, ownerID, ownerID).Scan(&count)
	if err != nil {
		return false, storageError()
	}
	return count == 1, nil
}
func (s *Store) beginGraphInput(ctx context.Context, ownerID, graphID, nodeID string, f domain.GraphInputPreparationFacts) (domain.GraphInputOperation, error) {
	var out domain.GraphInputOperation
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		return s.beginGraphInputTx(ctx, tx, ownerID, graphID, nodeID, f, &out)
	})
	return out, err
}

func (s *Store) beginGraphInputTx(ctx context.Context, tx *sql.Tx, ownerID, graphID, nodeID string, f domain.GraphInputPreparationFacts, out *domain.GraphInputOperation) error {
	if !domain.ValidID(graphID) || !domain.ValidID(nodeID) || !domain.ValidID(ownerID) {
		return domain.Invalid("exact graph input operation membership")
	}
	if e := s.requireRecoveryAdmission(ctx, tx, "", graphID, nodeID); e != nil {
		return e
	}
	if e := s.requireRecoveryAncestors(ctx, tx, graphID, nodeID); e != nil {
		return e
	}
	h, e := s.graphHeader(ctx, tx, graphID)
	if e != nil {
		return e
	}
	g, e := s.gate(ctx, tx)
	if e != nil {
		return e
	}
	if e = checkGate(g); e != nil {
		return e
	}
	c := f.Control
	scoped, e := s.recoveryGraphScopedTrigger(ctx, tx, h, ownerID, f.Trigger, nodeID)
	if e != nil {
		return e
	}
	if g.OwnerInstanceID != ownerID || h.DispatchAuthority == nil || !scoped && (h.DispatchAuthority.OwnerInstanceID != ownerID || h.DispatchAuthority.State == "suspended") || h.Summary.Stop != nil || c.GraphID != graphID || c.RootTaskID != h.Summary.RootTaskID || c.OwnerInstanceID != ownerID || c.GraphVersion != h.Summary.GraphVersion || c.Generation != h.DispatchAuthority.Generation || c.GraphControlRevision != h.Summary.ControlRevision || c.BudgetAuthorityEpoch != h.Summary.BudgetAuthorityEpoch || c.NodeID == nil || *c.NodeID != nodeID || c.TaskID == nil || c.TaskVersion == nil || c.NodeControlRevision == nil || c.InputID != nil {
		return graphControlError()
	}
	if e = s.graphFreshTrigger(ctx, tx, h, ownerID, f.Trigger, nodeID); e != nil {
		return e
	}
	triggerAttempt := f.Trigger.ID
	if f.Trigger.Kind == "response_ack" {
		_, acknowledgement, e := s.graphResponseAckEvent(ctx, tx, h, f.Trigger.ID, ownerID)
		if e != nil {
			return e
		}
		triggerAttempt = acknowledgement.AttemptID
	}
	if f.Trigger.Kind != "explicit" && c.OperationAttemptID != triggerAttempt {
		return graphControlError()
	}
	b, e := s.rawTaskBudget(ctx, tx, h.Summary.RootTaskID)
	if e != nil {
		return e
	}
	if b == nil || b.CurrentWindow.Number != c.WindowNumber {
		return domain.Fail("budget_conflict", "The graph budget window changed before input registration.")
	}
	nodes, e := s.graphNodes(ctx, tx, graphID)
	if e != nil {
		return e
	}
	var receiver *graphNodeRecord
	for i := range nodes {
		if nodes[i].Summary.ID == nodeID {
			receiver = &nodes[i]
		}
	}
	if receiver == nil {
		return notFound()
	}
	n := receiver.Summary
	task, e := s.task(ctx, tx, n.TaskID)
	if e != nil {
		return e
	}
	if *c.TaskID != task.ID || *c.TaskVersion != task.CurrentVersion || *c.NodeControlRevision != n.ControlRevision || n.Stop != nil || !scoped && !graphAuthorizedNode(h, n) {
		return graphControlError()
	}
	if !reflect.DeepEqual(c.ContributorVector, f.ContributorVector) {
		return evidenceUnavailable()
	}
	if e = s.graphContributorsCurrent(ctx, tx, graphID, f.ContributorVector); e != nil {
		return e
	}
	definition, e := s.graphDefinition(ctx, tx, graphID, h.Summary.GraphVersion)
	if e != nil {
		return e
	}
	contributions, vector, e := s.graphDependencySelection(ctx, tx, h, definition, nodes, *receiver, f)
	if e != nil {
		return e
	}
	if e = s.graphResponseHeadsFresh(ctx, tx, h, f.Trigger, vector); e != nil {
		return e
	}
	if h.Summary.RootInputID == nil {
		return evidenceUnavailable()
	}
	rootRecord, e := s.graphInputRecord(ctx, tx, graphID, *h.Summary.RootInputID)
	if e != nil {
		return e
	}
	if rootRecord.PrivateRoot == nil || rootRecord.Public.Summary.State != "ready" || !reflect.DeepEqual(*rootRecord.PrivateRoot, f.RootBinding) {
		return evidenceUnavailable()
	}
	root := *rootRecord.PrivateRoot
	for _, pair := range []struct {
		ref  domain.GraphArtifactRef
		kind string
	}{{root.Manifest, "source_manifest"}, {root.Index, "index_snapshot"}, {root.StagedBlobs, "graph_index_blobs"}} {
		if e = s.verifyGraphRef(ctx, tx, pair.ref, f.VerifiedArtifacts, pair.kind); e != nil {
			return e
		}
	}
	if n.InputID != nil {
		old, e := s.graphInputRecord(ctx, tx, graphID, *n.InputID)
		if e != nil {
			return e
		}
		a, e := s.attempt(ctx, tx, old.Public.Summary.OperationAttemptID)
		if e != nil {
			return e
		}
		if a.Phase != "settled" || old.Public.Summary.State == "ready" {
			return domain.Fail("attempt_active", "This receiver already has a retained or owned preparation; observe it without repeating I/O.")
		}
	}
	var workflows int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM workflows WHERE task_id=? AND task_version=?", task.ID, intRevision(task.CurrentVersion)).Scan(&workflows); e != nil {
		return storageError()
	}
	if workflows != 0 {
		return domain.Fail("graph_input_stale", "This task version already captured workflow authority. Explicit dependency rebinding must preserve it in a new version.")
	}
	consumption := Fingerprint("graphs.input.prepare", struct{ Graph, Node, Version, Control, Generation, Trigger, Basis string }{graphID, nodeID, task.CurrentVersion, n.ControlRevision, c.Generation, f.Trigger.ID, root.BasisSHA256})
	var used int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM graph_inputs WHERE graph_id=? AND json_extract(data,'$.ConsumptionKey')=?", graphID, consumption).Scan(&used); e != nil {
		return storageError()
	}
	if used != 0 {
		return domain.Fail("attempt_active", "This fresh trigger already registered this exact receiver operation.")
	}
	now := time.Now().UTC()
	deadline, invalid := domain.ParseRoutingTime(h.PreparationDeadlineAt)
	if invalid != nil {
		return storageError()
	}
	if b.CurrentWindow.DeadlineAt != nil {
		common, invalid := domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
		if invalid != nil {
			return storageError()
		}
		deadline = now.Add(domain.GraphPreparationSeconds * time.Second)
		if common.Before(deadline) {
			deadline = common
		}
	}
	if !now.Before(deadline) {
		return domain.Fail("budget_expired", "The bounded graph preparation window expired before registration.")
	}
	rootTask, e := s.task(ctx, tx, h.Summary.RootTaskID)
	if e != nil {
		return e
	}
	rootVersion, e := s.version(ctx, tx, rootTask.ID, rootTask.CurrentVersion)
	if e != nil {
		return e
	}
	a := newAttempt(rootTask, rootVersion, f.Trigger.ID, ownerID, definition.Original.ExecutionMode, "")
	a.ExecutorKind = "local_graph"
	a.Role = "graph_input"
	a.ConnectionID = "local-graph"
	a.RequestedTarget = domain.ExecutionPins{}
	a.ResolvedTarget = nil
	a.DeadlineAt = deadline.Format("2006-01-02T15:04:05.000Z")
	inputID := domain.NewID()
	c.OperationAttemptID = a.ID
	c.InputID = ptr(inputID)
	c.ContributorVector = vector
	sequence, e := nextSequence(ctx, tx, "graph_inputs")
	if e != nil {
		return e
	}
	record := domain.GraphInputRecord{Trigger: f.Trigger, ConsumptionKey: consumption, ExpectedContributions: contributions, Sequence: domain.Decimal(sequence), PrivateRoot: &root, ExpectedAuthority: c, Public: domain.GraphInputDetail{Summary: domain.GraphInputSummary{ID: inputID, GraphID: graphID, GraphVersion: h.Summary.GraphVersion, RootTaskID: rootTask.ID, OperationAttemptID: a.ID, Kind: "node_input", State: "preparing", Current: true, ReceivingNodeID: ptr(nodeID), ReceivingTaskID: ptr(task.ID), ReceivingTaskVersion: ptr(task.CurrentVersion), RootBasisSHA256: ptr(root.BasisSHA256), CreatedAt: a.AdmittedAt, UpdatedAt: a.AdmittedAt}}}
	if e = s.insertAttempt(ctx, tx, a); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO graph_inputs(sequence,id,graph_id,graph_version,node_id,operation_attempt_id,root_input_id,kind,state,data) VALUES(?,?,?,?,?,?,?,'node_input','preparing',?)", sequence, inputID, graphID, intRevision(h.Summary.GraphVersion), nodeID, a.ID, *h.Summary.RootInputID, encode(record)); e != nil {
		return storageError()
	}
	rootTask.LatestAttemptID = ptr(a.ID)
	rootTask.StateRevision, e = increment(rootTask.StateRevision)
	if e != nil {
		return e
	}
	if e = saveTask(ctx, tx, rootTask); e != nil {
		return e
	}
	receiver.Summary.InputID = ptr(inputID)
	receiver.Summary.State = "preparing"
	receiver.Summary.NextOwner = "council"
	receiver.Summary.NextAction = "Prepare the exact receiving input and context."
	receiver.Summary.StateRevision, e = increment(receiver.Summary.StateRevision)
	if e != nil {
		return e
	}
	if e = saveGraphNode(ctx, tx, *receiver); e != nil {
		return e
	}
	h.Summary.State = "preparing"
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return e
	}
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return e
	}
	h.Summary.UpdatedAt = domain.Now()
	if e = saveGraphHeader(ctx, tx, h); e != nil {
		return e
	}
	*out, e = s.graphInputOperation(ctx, tx, ownerID, a.ID)
	return e
}
