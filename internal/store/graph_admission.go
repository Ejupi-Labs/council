package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"reflect"

	"encoding/json/jsontext"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A graph binding changes only prospective graph capture. Standalone capture
// keeps its accepted preference/replay behavior.
type graphWorkflowAuthority struct {
	Scope            domain.DeliveryScope
	ConditionsSHA256 string
}

func graphEffectiveWorkflowConditions(node, graph string) string {
	sum := sha256.Sum256([]byte(encode(struct {
		Node  string `json:"nodeConditionsSha256"`
		Graph string `json:"graphConditionsSha256"`
	}{node, graph})))
	return hex.EncodeToString(sum[:])
}
func (s *Store) graphAdmissionControl(ctx context.Context, q queryer, ownerID, graphID string, trigger domain.GraphFreshTrigger, f domain.GraphNodeAdmissionFacts) (graphHeader, []graphNodeRecord, int, domain.GraphInputRecord, error) {
	var record domain.GraphInputRecord
	h, e := s.graphHeader(ctx, q, graphID)
	if e != nil {
		return h, nil, 0, record, e
	}
	gate, e := s.gate(ctx, q)
	if e != nil {
		return h, nil, 0, record, e
	}
	if e = checkGate(gate); e != nil {
		return h, nil, 0, record, e
	}
	c := f.Control
	if !domain.ValidID(ownerID) || gate.OwnerInstanceID != ownerID || h.DispatchAuthority == nil || h.DispatchAuthority.OwnerInstanceID != ownerID || h.DispatchAuthority.State == "suspended" || h.Summary.Stop != nil ||
		c.GraphID != graphID || c.GraphVersion != h.Summary.GraphVersion || c.RootTaskID != h.Summary.RootTaskID || c.OwnerInstanceID != ownerID || c.Generation != h.DispatchAuthority.Generation || c.GraphControlRevision != h.Summary.ControlRevision || c.BudgetAuthorityEpoch != h.Summary.BudgetAuthorityEpoch || c.NodeID == nil || *c.NodeID != f.NodeID || c.TaskID == nil || c.TaskVersion == nil || *c.TaskVersion != f.TaskVersion || c.NodeControlRevision == nil || c.InputID == nil || *c.InputID != f.InputBinding.ID {
		return h, nil, 0, record, graphControlError()
	}
	if e = s.graphFreshTrigger(ctx, q, h, ownerID, trigger); e != nil {
		return h, nil, 0, record, e
	}
	b, e := s.taskBudget(ctx, q, h.Summary.RootTaskID)
	if e != nil {
		return h, nil, 0, record, e
	}
	if b == nil || c.WindowNumber != b.CurrentWindow.Number {
		return h, nil, 0, record, graphControlError()
	}
	nodes, e := s.graphNodes(ctx, q, graphID)
	if e != nil {
		return h, nil, 0, record, e
	}
	index := -1
	for i, n := range nodes {
		if n.Summary.ID == f.NodeID {
			index = i
			break
		}
	}
	if index < 0 {
		return h, nil, 0, record, notFound()
	}
	n := nodes[index].Summary
	task, e := s.task(ctx, q, n.TaskID)
	if e != nil {
		return h, nil, 0, record, e
	}
	if task.ID != *c.TaskID || task.CurrentVersion != f.TaskVersion || n.TaskVersion != f.TaskVersion || n.ControlRevision != *c.NodeControlRevision || n.Stop != nil || !graphAuthorizedNode(h, n) || n.InputID == nil || *n.InputID != f.InputBinding.ID {
		return h, nil, 0, record, graphControlError()
	}
	record, e = s.graphInputRecord(ctx, q, graphID, f.InputBinding.ID)
	if e != nil {
		return h, nil, 0, record, e
	}
	if record.Public.Summary.State != "ready" || !record.Public.Summary.Current || record.Public.InputBinding == nil || record.Public.ConditionsRef == nil || record.ConditionsBinding == nil ||
		!reflect.DeepEqual(*record.Public.InputBinding, f.InputBinding) || !reflect.DeepEqual(*record.ConditionsBinding, f.ConditionsBinding) || *record.Public.ConditionsRef != f.ConditionsRef ||
		record.Public.Summary.ReceivingNodeID == nil || *record.Public.Summary.ReceivingNodeID != n.ID || record.Public.Summary.ReceivingTaskVersion == nil || *record.Public.Summary.ReceivingTaskVersion != f.TaskVersion ||
		c.OperationAttemptID != record.Public.Summary.OperationAttemptID || !reflect.DeepEqual(c.ContributorVector, f.InputBinding.ContributorVector) {
		return h, nil, 0, record, evidenceUnavailable()
	}
	if e = s.graphContributorsCurrent(ctx, q, graphID, f.InputBinding.ContributorVector); e != nil {
		return h, nil, 0, record, e
	}
	if e = s.graphResponseHeadsFresh(ctx, q, h, trigger, f.InputBinding.ContributorVector); e != nil {
		return h, nil, 0, record, e
	}
	if n.NodeAdmissionID == nil {
		// Fresh current revisions cannot reopen an unstarted receiver after an
		// ancestor Stop. Keep this check in the admission transaction, before
		// recording a preparation block or reserving any native work. Existing
		// admitted workflows retain their separate known-first-GO rules.
		definition, e := s.graphDefinition(ctx, q, graphID, h.Summary.GraphVersion)
		if e != nil {
			return h, nil, 0, record, e
		}
		ancestors := map[string]bool{n.Key: true}
		if definition.Original.Dependencies != nil {
			for range nodes {
				for _, edge := range *definition.Original.Dependencies {
					if ancestors[edge.To] {
						ancestors[edge.From] = true
					}
				}
			}
		}
		for _, candidate := range nodes {
			if candidate.Summary.ID != n.ID && ancestors[candidate.Summary.Key] && candidate.Summary.Stop != nil {
				return h, nil, 0, record, graphControlError()
			}
		}
	}
	// A failure-only observation grants no execution, target, claim or debit.
	// Its exact persisted input/control/vector are already checked above; physical
	// proof unavailability is precisely the blocker being recorded.
	if f.PreparationFailure != nil {
		return h, nodes, index, record, nil
	}
	for _, pair := range []struct {
		ref  domain.GraphArtifactRef
		kind string
	}{
		{f.InputBinding.Manifest, "graph_input_manifest"}, {f.InputBinding.Index, "index_snapshot"}, {f.InputBinding.StagedBlobs, "graph_index_blobs"}, {f.InputBinding.Provenance, "graph_input_provenance"}, {f.InputBinding.ContextPacket, "graph_context"}, {f.ConditionsRef, "graph_conditions"},
	} {
		if e = s.verifyGraphRef(ctx, q, pair.ref, f.VerifiedArtifacts, pair.kind); e != nil {
			return h, nil, 0, record, e
		}
	}
	basis := f.SourceBasis
	if basis.BasisKind != "graph_input" || basis.WorkflowID != "" || basis.StepID != "" || basis.InputID != f.InputBinding.ID || basis.InputSHA256 != f.InputBinding.InputSHA256 || basis.CandidateBinding != nil || basis.ResponseBinding != nil || basis.ExpectedManifest != f.InputBinding.Manifest || basis.ExpectedIndex != f.InputBinding.Index || !graphSHA256(basis.ExpectedWorkingEntriesSHA256) || !graphSHA256(basis.ExpectedIndexEntriesSHA256) {
		return h, nil, 0, record, evidenceUnavailable()
	}
	return h, nodes, index, record, nil
}
func graphSHA256(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func knownGraphAdmissionBlock(e error) *domain.Error {
	var de *domain.Error
	if !errors.As(e, &de) {
		return nil
	}
	switch de.Code {
	case "capacity_wait", "attempt_active", "budget_exhausted", "budget_expired", "routing_unavailable", "capability_unavailable", "check_unavailable", "source_capture_blocked":
		return de
	}
	return nil
}
func (s *Store) graphProjectAdmissionBlock(ctx context.Context, tx *sql.Tx, h graphHeader, nodes []graphNodeRecord, index int, inputID string, block *domain.Error) error {
	n := &nodes[index]
	waiting := block.Code == "capacity_wait" || block.Code == "attempt_active"
	n.Summary.State = "blocked"
	n.Summary.NextOwner = "user"
	n.Summary.NextAction = block.Message
	if waiting {
		n.Summary.State = "ready"
		n.Summary.NextOwner = "council"
		n.Summary.NextAction = "Wait for managed capacity without spending an admission."
	}
	n.Blocker = &domain.GraphBlockerSummary{Code: block.Code, Message: block.Message, NodeID: ptr(n.Summary.ID), InputID: ptr(inputID)}
	var e error
	n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
	if e != nil {
		return e
	}
	if e = saveGraphNode(ctx, tx, *n); e != nil {
		return e
	}
	h.Summary.State = "blocked"
	h.Summary.NextOwner = n.Summary.NextOwner
	h.Summary.NextAction = n.Summary.NextAction
	if waiting {
		h.DispatchAuthority.State = "capacity_wait"
	} else {
		// The node's user-owned blocker prevents a later completion/configuration
		// callback from silently rearming it. Other authorized siblings may proceed.
		h.DispatchAuthority.State = "active"
	}
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return e
	}
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return e
	}
	h.Summary.UpdatedAt = domain.Now()
	return saveGraphHeader(ctx, tx, h)
}
func (s *Store) admitGraphNode(ctx context.Context, ownerID, graphID string, trigger domain.GraphFreshTrigger, f domain.GraphNodeAdmissionFacts) (domain.GraphDispatchView, error) {
	var out domain.GraphDispatchView
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		return s.admitGraphNodeTx(ctx, tx, ownerID, graphID, trigger, f, &out)
	})
	if e != nil {
		return domain.GraphDispatchView{}, e
	}
	return out, nil
}

func (s *Store) admitGraphNodeTx(ctx context.Context, tx *sql.Tx, ownerID, graphID string, trigger domain.GraphFreshTrigger, f domain.GraphNodeAdmissionFacts, out *domain.GraphDispatchView) error {
	h, nodes, index, record, e := s.graphAdmissionControl(ctx, tx, ownerID, graphID, trigger, f)
	if e != nil {
		return e
	}
	n := &nodes[index]
	observed, e := graphAdmissionObserved(ctx, tx, graphAdmissionObservationKey(graphID, f, trigger))
	if e != nil {
		return e
	}
	if observed {
		return nil
	}
	if n.Summary.NodeAdmissionID != nil {
		return nil
	}
	if n.Summary.State == "blocked" {
		if trigger.Kind != "explicit" {
			return nil
		}
		// A fresh captured explicit intent can recheck a known pre-admission
		// block. This never resets an existing workflow or launched execution.
		n.Summary.State = "ready"
	}
	if n.Summary.State != "ready" {
		return graphControlError()
	}
	if f.PreparationFailure != nil {
		if f.PreparationFailure.Code == "" || f.PreparationFailure.Message == "" || len(f.PreparationFailure.Code) > 128 || len(f.PreparationFailure.Message) > 4096 {
			return domain.Invalid("bounded observed graph preparation failure")
		}
		if durabilityErrorCode(f.PreparationFailure) != "" {
			return f.PreparationFailure
		}
		if e = storeGraphAdmissionObservation(ctx, tx, graphID, f, trigger, nil, f.PreparationFailure); e != nil {
			return e
		}
		return s.graphProjectAdmissionBlock(ctx, tx, h, nodes, index, f.InputBinding.ID, f.PreparationFailure)
	}
	if _, e = tx.ExecContext(ctx, "SAVEPOINT graph_node_admission"); e != nil {
		return storageError()
	}
	result, e := s.graphAdmitInitial(ctx, tx, ownerID, h, *n, record, trigger, f)
	if e != nil {
		block := knownGraphAdmissionBlock(e)
		if block == nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "ROLLBACK TO graph_node_admission"); e != nil {
			return storageError()
		}
		if _, e = tx.ExecContext(ctx, "RELEASE graph_node_admission"); e != nil {
			return storageError()
		}
		if e = storeGraphAdmissionObservation(ctx, tx, graphID, f, trigger, nil, block); e != nil {
			return e
		}
		return s.graphProjectAdmissionBlock(ctx, tx, h, nodes, index, f.InputBinding.ID, block)
	}
	if _, e = tx.ExecContext(ctx, "RELEASE graph_node_admission"); e != nil {
		return storageError()
	}
	if e = storeGraphAdmissionObservation(ctx, tx, graphID, f, trigger, &result.Dispatch.ID, nil); e != nil {
		return e
	}
	*out = result
	return nil
}
func (s *Store) graphAdmitInitial(ctx context.Context, tx *sql.Tx, ownerID string, h graphHeader, node graphNodeRecord, input domain.GraphInputRecord, trigger domain.GraphFreshTrigger, f domain.GraphNodeAdmissionFacts) (domain.GraphDispatchView, error) {
	var out domain.GraphDispatchView
	t, e := s.task(ctx, tx, node.Summary.TaskID)
	if e != nil {
		return out, e
	}
	var count int
	if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM workflows WHERE task_id=? AND task_version=?", t.ID, intRevision(t.CurrentVersion)).Scan(&count); e != nil {
		return out, storageError()
	}
	if count != 0 {
		return out, domain.Fail("graph_input_stale", "This version already has immutable workflow authority. Use explicit graph controls.")
	}
	v, e := s.version(ctx, tx, t.ID, t.CurrentVersion)
	if e != nil {
		return out, e
	}
	if invalid := domain.CheckAuthoringCapability(v); invalid != nil {
		return out, invalid
	}
	definition, e := s.graphDefinition(ctx, tx, h.Summary.ID, h.Summary.GraphVersion)
	if e != nil {
		return out, e
	}
	var declaration *domain.GraphNodeInput
	for i := range definition.Original.Nodes {
		if definition.Original.Nodes[i].Key == node.Summary.Key {
			declaration = &definition.Original.Nodes[i]
			break
		}
	}
	if declaration == nil || f.Capture == nil {
		return out, workflowUnavailable()
	}
	execution, e := s.graphNodeOptions(ctx, tx, h.Summary.ID, node.Summary.ID, t.CurrentVersion)
	if e != nil {
		return out, e
	}
	if t.CurrentVersion != "1" && execution.RoutingBasis == nil {
		return out, domain.Fail("graph_input_stale", "This graph version lacks its frozen routing basis.")
	}
	b, e := s.taskBudget(ctx, tx, t.ID)
	if e != nil {
		return out, e
	}
	if b == nil {
		return out, storageError()
	}
	basis, e := s.graphVersionRoutingBasis(ctx, tx, h, execution)
	if e != nil {
		return out, e
	}
	if expected := basis.OptionSnapshot.ExpectedPolicyRevision; expected != "" && expected != basis.PolicySnapshot.Revision {
		return out, domain.RevisionConflict(basis.PolicySnapshot.Revision)
	}
	prior := domain.TaskRoutingAuthority{PolicySnapshot: basis.PolicySnapshot, OptionSnapshot: basis.OptionSnapshot}
	// Capture fresh receiving-input suitability once, against the immutable
	// per-version policy. Never replay a saved UseCurrentPolicy command here.
	authority, e := s.captureRouting(ctx, tx, t, v, nil, f.Routing, &prior, "graph")
	if e != nil {
		return out, e
	}
	a := newAttempt(t, v, h.DispatchRequestID, ownerID, execution.ExecutionMode, execution.FakeScenario)
	decision, e := s.admitRouting(ctx, tx, t, v, &a, authority, b, f.Routing, "manual", true)
	if e != nil {
		return out, e
	}
	if invalid := checkResolvedScenario(a, a.ExecutionMode, a.FakeScenario); invalid != nil {
		return out, invalid
	}
	options := domain.StartTaskInput{RequestID: h.DispatchRequestID, ProjectID: t.ProjectID, OriginalText: t.OriginalText, Workflow: execution.Workflow}
	scope := definition.Delivery
	if definition.Original.FinalNodeKey != node.Summary.Key {
		scope.LocalMode = "review_only"
		scope.ExternalAction = nil
		scope.Source = "graph_scope"
		scope.PreferenceRevision = ""
	}
	w, step, e := s.bindInitialWorkflowWithGraph(ctx, tx, t, v, &a, authority.ID, options, f.Capture, &graphWorkflowAuthority{Scope: scope, ConditionsSHA256: f.ConditionsBinding.ConditionsSHA256})
	if e != nil {
		return out, e
	}
	if e = insertWorkflowTx(ctx, tx, w); e != nil {
		return out, e
	}
	if e = s.insertAttempt(ctx, tx, a); e != nil {
		return out, e
	}
	if e = insertWorkflowStepTx(ctx, tx, step); e != nil {
		return out, e
	}
	if e = storeRoutingDecision(ctx, tx, decision); e != nil {
		return out, e
	}
	dispatchID := domain.NewID()
	source := f.SourceBasis
	source.WorkflowID = w.ID
	source.StepID = step.ID
	d := domain.GraphDispatch{ID: dispatchID, GraphID: h.Summary.ID, GraphVersion: h.Summary.GraphVersion, NodeID: node.Summary.ID, TaskID: t.ID, TaskVersion: v.Version, NodeAdmissionID: dispatchID, WorkflowID: w.ID, StepID: step.ID, AttemptID: a.ID, ExecutorKind: "native", Role: "author", InputID: f.InputBinding.ID, InputSHA256: f.InputBinding.InputSHA256, ContextPacket: f.InputBinding.ContextPacket, ContributorVector: f.InputBinding.ContributorVector, OwnerInstanceID: ownerID, GraphControlRevision: h.Summary.ControlRevision, NodeControlRevision: node.Summary.ControlRevision, BudgetOwnerTaskID: h.Summary.RootTaskID, BudgetAuthorityEpoch: h.Summary.BudgetAuthorityEpoch, WindowNumber: b.CurrentWindow.Number, ObservedBudgetRevision: b.Revision, SourceBasis: source, FreshTriggerID: trigger.ID, LogicalSlot: step.LogicalKey, ExecutionOrdinal: step.ExecutionOrdinal, NativeConsumptionKey: ptr(a.ID), CreatedAt: a.AdmittedAt}
	sequence, e := nextSequence(ctx, tx, "graph_dispatches")
	if e != nil {
		return out, e
	}
	out.Dispatch = d
	record := domain.GraphDispatchRecord{Sequence: domain.Decimal(sequence), Public: out}
	consumption := Fingerprint("graphs.node.admit", struct{ Graph, Node, Version, Input, Slot string }{d.GraphID, d.NodeID, d.TaskVersion, d.InputID, d.LogicalSlot})
	if _, e = tx.ExecContext(ctx, "INSERT INTO graph_dispatches(sequence,id,graph_id,graph_version,node_id,task_id,task_version,input_id,attempt_id,workflow_id,step_id,node_admission_id,logical_slot,execution_ordinal,owner_instance_id,graph_control_revision,node_control_revision,budget_epoch,window_number,fresh_trigger_id,consumption_key,native_consumption_key,data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", sequence, d.ID, d.GraphID, intRevision(d.GraphVersion), d.NodeID, d.TaskID, intRevision(d.TaskVersion), d.InputID, d.AttemptID, d.WorkflowID, d.StepID, d.NodeAdmissionID, d.LogicalSlot, intRevision(d.ExecutionOrdinal), ownerID, intRevision(d.GraphControlRevision), intRevision(d.NodeControlRevision), intRevision(d.BudgetAuthorityEpoch), intRevision(d.WindowNumber), d.FreshTriggerID, consumption, a.ID, encode(record)); e != nil {
		return out, storageError()
	}
	if e = s.claimCapacity(ctx, tx, a, ownerID); e != nil {
		return out, e
	}
	claims, e := s.graphClaims(ctx, tx)
	if e != nil {
		return out, e
	}
	slot := ""
	for _, claim := range claims {
		if claim.AttemptID == a.ID {
			slot = claim.Slot
		}
	}
	if slot == "" {
		return out, storageError()
	}
	if _, e = s.appendEvent(ctx, tx, a.ID, "capacity_reserved", jsontext.Value(encode(struct {
		Owner string `json:"ownerInstanceId"`
		Slot  string `json:"slot"`
	}{ownerID, slot}))); e != nil {
		return out, e
	}
	t.LatestAttemptID = ptr(a.ID)
	t.StateRevision, e = increment(t.StateRevision)
	if e != nil {
		return out, e
	}
	t.UpdatedAt = a.AdmittedAt
	if e = saveTask(ctx, tx, t); e != nil {
		return out, e
	}
	debits, invalid := domain.AddRoutingCounts(node.NativeDebits, "1")
	if invalid != nil {
		return out, invalid
	}
	node.NativeDebits = debits
	node.Summary.NodeAdmissionID = ptr(d.ID)
	node.Summary.LatestAttemptID = ptr(a.ID)
	node.Summary.State = "queued"
	node.Summary.NextOwner = "council"
	node.Summary.NextAction = "Observe the exact admitted native execution."
	node.Blocker = nil
	node.Summary.StateRevision, e = increment(node.Summary.StateRevision)
	if e != nil {
		return out, e
	}
	if e = saveGraphNode(ctx, tx, node); e != nil {
		return out, e
	}
	nodes, e := s.graphNodes(ctx, tx, h.Summary.ID)
	if e != nil {
		return out, e
	}
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return out, e
	}
	h.Summary.State = "queued"
	h.Summary.NextOwner = "council"
	h.Summary.NextAction = "Observe admitted graph work."
	h.DispatchAuthority.State = "active"
	h.Summary.UpdatedAt = a.AdmittedAt
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return out, e
	}
	if e = saveGraphHeader(ctx, tx, h); e != nil {
		return out, e
	}
	return out, nil
}
