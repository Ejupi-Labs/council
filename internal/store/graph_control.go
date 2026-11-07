package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphRetainedTerminal(state string) bool {
	switch state {
	case "completed", "failed", "interrupted", "cancelled":
		return true
	}
	return false
}
func graphControlError() *domain.Error {
	return domain.Fail("graph_control_conflict", "The exact graph or node control changed. Read its current state before a new intent.")
}
func saveGraphNode(ctx context.Context, tx *sql.Tx, n graphNodeRecord) error {
	_, e := tx.ExecContext(ctx, "UPDATE graph_nodes SET control_revision=?,state_revision=?,state=?,current_input_id=?,node_admission_id=?,native_debits=?,automatic_debits=?,data=? WHERE id=?", intRevision(n.Summary.ControlRevision), intRevision(n.Summary.StateRevision), n.Summary.State, n.Summary.InputID, n.Summary.NodeAdmissionID, countInt(n.NativeDebits), countInt(n.AutomaticDebits), encode(n), n.Summary.ID)
	if e != nil {
		return storageError()
	}
	return nil
}
func graphCounts(nodes []graphNodeRecord) (domain.GraphCounts, error) {
	c := graphZeroCounts(0)
	c.Required = domain.Decimal(int64(len(nodes)))
	c.Waiting = "0"
	for _, n := range nodes {
		var target *string
		switch n.Summary.State {
		case "waiting":
			target = &c.Waiting
		case "ready":
			target = &c.Ready
		case "preparing":
			target = &c.Preparing
		case "queued":
			target = &c.Queued
		case "running":
			target = &c.Running
		case "blocked":
			target = &c.Blocked
		case "cancelling":
			target = &c.Cancelling
		case "cancelled":
			target = &c.Cancelled
		case "failed":
			target = &c.Failed
		case "interrupted":
			target = &c.Interrupted
		case "completed":
			target = &c.Completed
		default:
			return c, storageError()
		}
		next, invalid := domain.AddRoutingCounts(*target, "1")
		if invalid != nil {
			return c, invalid
		}
		*target = next
	}
	return c, nil
}
func (s *Store) graphPending(ctx context.Context, q queryer, h graphHeader) ([]domain.Attempt, error) {
	rows, e := q.QueryContext(ctx, "SELECT data FROM attempts WHERE phase!='settled' AND (task_id=? OR task_id IN(SELECT task_id FROM graph_nodes WHERE graph_id=?)) ORDER BY sequence LIMIT 65", h.Summary.RootTaskID, h.Summary.ID)
	if e != nil {
		return nil, storageError()
	}
	out := []domain.Attempt{}
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			rows.Close()
			return nil, storageError()
		}
		a, e := decode[domain.Attempt](raw)
		if e != nil {
			rows.Close()
			return nil, storageError()
		}
		out = append(out, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil || len(out) > 64 {
		return nil, storageError()
	}
	return out, nil
}
func (s *Store) acceptGraphAttemptStop(ctx context.Context, tx *sql.Tx, a domain.Attempt, requestID string) (domain.Attempt, error) {
	if a.Phase == "settled" {
		return a, nil
	}
	neverPrepared := a.Phase == "queued" && !a.LaunchIntent
	if a.Stop == nil {
		sequence, e := s.appendEvent(ctx, tx, a.ID, "stop_accepted", nil)
		if e != nil {
			return a, e
		}
		a.Stop = &domain.AcceptedStop{RequestID: requestID, AcceptedAt: domain.Now(), EventSequence: sequence}
		a.Phase = "cancelling"
		task, e := s.task(ctx, tx, a.TaskID)
		if e != nil {
			return a, e
		}
		task.Revision, e = increment(task.Revision)
		if e != nil {
			return a, e
		}
		task.UpdatedAt = domain.Now()
		if e = saveTask(ctx, tx, task); e != nil {
			return a, e
		}
		if e = s.bumpState(ctx, tx, &a); e != nil {
			return a, e
		}
	}
	claims, e := s.graphClaims(ctx, tx)
	if e != nil {
		return a, e
	}
	claimed := false
	reservedGraph := false
	for _, claim := range claims {
		if claim.AttemptID == a.ID {
			claimed = true
			reservedGraph = claim.GraphID != nil && claim.NodeID != nil && !claim.LegacyBarrier
		}
	}
	if !a.LaunchIntent && (!claimed || neverPrepared && reservedGraph) {
		settled, e := s.settle(ctx, tx, a, domain.Settlement{Outcome: "cancelled", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", EffectsKnown: true, NoProcessCreated: true})
		if e != nil {
			return a, e
		}
		if a.ExecutorKind == "local_graph" {
			var graphID, inputID string
			if e = tx.QueryRowContext(ctx, "SELECT graph_id,id FROM graph_inputs WHERE operation_attempt_id=?", a.ID).Scan(&graphID, &inputID); e != nil {
				return a, storageError()
			}
			record, e := s.graphInputRecord(ctx, tx, graphID, inputID)
			if e != nil {
				return a, e
			}
			record.Public.Summary.State = "blocked"
			record.Public.Summary.Current = false
			record.Public.Summary.UpdatedAt = domain.Now()
			record.Public.Summary.Failure = domain.Fail("operation_cancelled", "Graph preparation was stopped before local work began.")
			if _, e = tx.ExecContext(ctx, "UPDATE graph_inputs SET state='blocked',data=? WHERE id=?", encode(record), inputID); e != nil {
				return a, storageError()
			}
		}
		return settled, nil
	}
	return a, nil
}
func (s *Store) stopGraph(ctx context.Context, ownerID, graphID string, in domain.GraphStopInput) domain.Result {
	if invalid := domain.ValidateGraphStopInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	return s.mutate(ctx, in.RequestID, "graphs.stop", GraphStopFingerprint(graphID, in), func(tx *sql.Tx) (int, any, error) {
		h, e := s.graphHeader(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		if h.Summary.ControlRevision != in.ExpectedGraphControlRevision {
			return 0, nil, graphControlError()
		}
		gate, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if !domain.ValidID(ownerID) || gate.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "Stop must contact the current graph owner.")
		}
		if h.Summary.Stop != nil || h.Summary.State == "completed" {
			reason := "already_stopped"
			if h.Summary.Stop == nil {
				reason = "already_settled"
			}
			return 200, domain.GraphStopResult{Graph: h.Summary, StopAccepted: false, Reason: reason, TargetAttemptIDs: []string{}, AffectedNodeIDs: []string{}}, nil
		}
		pending, e := s.graphPending(ctx, tx, h)
		if e != nil {
			return 0, nil, e
		}
		nodes, e := s.graphNodes(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		remainingScope := len(pending) > 0
		for _, n := range nodes {
			if !graphRetainedTerminal(n.Summary.State) {
				remainingScope = true
			}
		}
		if !remainingScope {
			return 200, domain.GraphStopResult{Graph: h.Summary, StopAccepted: false, Reason: "already_settled", TargetAttemptIDs: []string{}, AffectedNodeIDs: []string{}}, nil
		}
		targets := []string{}
		activeTasks := map[string]bool{}
		for _, a := range pending {
			stopped, e := s.acceptGraphAttemptStop(ctx, tx, a, in.RequestID)
			if e != nil {
				return 0, nil, e
			}
			targets = append(targets, a.ID)
			if stopped.Phase != "settled" {
				activeTasks[a.TaskID] = true
			}
		}
		h.Summary.ControlRevision, e = increment(h.Summary.ControlRevision)
		if e != nil {
			return 0, nil, e
		}
		record := &domain.GraphStopRecord{RequestID: in.RequestID, AcceptedAt: domain.Now(), GraphControlRevision: h.Summary.ControlRevision, TargetAttemptIDs: targets}
		h.Summary.Stop = record
		h.Summary.State = "cancelled"
		if len(activeTasks) > 0 {
			h.Summary.State = "cancelling"
		}
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect stopped work and retained partial results."
		if h.DispatchAuthority != nil {
			h.DispatchAuthority.State = "suspended"
		}
		affected := []string{}
		for i := range nodes {
			n := &nodes[i]
			affected = append(affected, n.Summary.ID)
			if !graphRetainedTerminal(n.Summary.State) {
				n.Summary.State = "cancelled"
				if activeTasks[n.Summary.TaskID] {
					n.Summary.State = "cancelling"
				}
				n.Summary.NextAction = "graph_stopped"
				n.Summary.NextOwner = "user"
				n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
				if e != nil {
					return 0, nil, e
				}
				if e = saveGraphNode(ctx, tx, *n); e != nil {
					return 0, nil, e
				}
			}
		}
		h.Summary.Counts, e = graphCounts(nodes)
		if e != nil {
			return 0, nil, e
		}
		h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		h.Summary.UpdatedAt = domain.Now()
		if e = saveGraphHeader(ctx, tx, h); e != nil {
			return 0, nil, e
		}
		return 202, domain.GraphStopResult{Graph: h.Summary, StopAccepted: true, Reason: "accepted", TargetAttemptIDs: targets, AffectedNodeIDs: affected}, nil
	})
}
func (s *Store) graphNodeKnownStarted(ctx context.Context, q queryer, n graphNodeRecord) (bool, error) {
	if n.Summary.NodeAdmissionID == nil {
		return false, nil
	}
	var when, attempt sql.NullString
	if e := q.QueryRowContext(ctx, "SELECT known_first_go_at,known_first_go_attempt_id FROM graph_dispatches WHERE id=? AND node_id=?", *n.Summary.NodeAdmissionID, n.Summary.ID).Scan(&when, &attempt); e != nil {
		return false, storageError()
	}
	return when.Valid && attempt.Valid, nil
}
func (s *Store) stopGraphNode(ctx context.Context, ownerID, graphID, nodeKey string, in domain.GraphNodeStopInput) domain.Result {
	if invalid := domain.ValidateGraphNodeStopInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	return s.mutate(ctx, in.RequestID, "graphs.nodes.stop", GraphNodeStopFingerprint(graphID, nodeKey, in), func(tx *sql.Tx) (int, any, error) {
		h, e := s.graphHeader(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		if h.Summary.ControlRevision != in.ExpectedGraphControlRevision {
			return 0, nil, graphControlError()
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if !domain.ValidID(ownerID) || g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "Stop must contact the current graph owner.")
		}
		nodes, e := s.graphNodes(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		selected := -1
		keys := map[string]string{}
		for i, n := range nodes {
			keys[n.Summary.Key] = n.Summary.ID
			if n.Summary.Key == nodeKey {
				selected = i
			}
		}
		if selected < 0 {
			return 0, nil, notFound()
		}
		node := nodes[selected].Summary
		if node.ID != in.NodeID || node.ControlRevision != in.ExpectedNodeControlRevision {
			return 0, nil, graphControlError()
		}
		if h.Summary.Stop != nil || node.Stop != nil && node.Stop.NodeControlRevision != nil && *node.Stop.NodeControlRevision == node.ControlRevision {
			return 200, domain.GraphStopResult{Graph: h.Summary, Node: &node, StopAccepted: false, Reason: "already_stopped", TargetAttemptIDs: []string{}, AffectedNodeIDs: []string{}}, nil
		}
		definition, e := s.graphDefinition(ctx, tx, graphID, h.Summary.GraphVersion)
		if e != nil {
			return 0, nil, e
		}
		descendants := map[string]bool{nodeKey: true}
		for changed := true; changed; {
			changed = false
			if definition.Original.Dependencies != nil {
				for _, edge := range *definition.Original.Dependencies {
					if descendants[edge.From] && !descendants[edge.To] {
						descendants[edge.To] = true
						changed = true
					}
				}
			}
		}
		affected := map[string]bool{node.ID: true}
		for _, n := range nodes {
			if n.Summary.ID == node.ID || !descendants[n.Summary.Key] {
				continue
			}
			started, e := s.graphNodeKnownStarted(ctx, tx, n)
			if e != nil {
				return 0, nil, e
			}
			if !started {
				affected[n.Summary.ID] = true
			}
		}
		pending, e := s.graphPending(ctx, tx, h)
		if e != nil {
			return 0, nil, e
		}
		remainingScope := false
		for _, n := range nodes {
			if affected[n.Summary.ID] && !graphRetainedTerminal(n.Summary.State) {
				remainingScope = true
			}
		}
		targets := []string{}
		active := map[string]bool{}
		for _, a := range pending {
			match := false
			for _, n := range nodes {
				if n.Summary.TaskID == a.TaskID && affected[n.Summary.ID] {
					match = true
				}
			}
			if a.ExecutorKind == "local_graph" {
				var receiving sql.NullString
				if e = tx.QueryRowContext(ctx, "SELECT node_id FROM graph_inputs WHERE operation_attempt_id=?", a.ID).Scan(&receiving); e != nil {
					return 0, nil, storageError()
				}
				match = receiving.Valid && affected[receiving.String]
			}
			if match {
				remainingScope = true
				stopped, e := s.acceptGraphAttemptStop(ctx, tx, a, in.RequestID)
				if e != nil {
					return 0, nil, e
				}
				targets = append(targets, a.ID)
				if stopped.Phase != "settled" {
					active[a.TaskID] = true
				}
			}
		}
		if !remainingScope {
			return 200, domain.GraphStopResult{Graph: h.Summary, Node: &node, StopAccepted: false, Reason: "already_settled", TargetAttemptIDs: []string{}, AffectedNodeIDs: []string{}}, nil
		}
		affectedIDs := []string{}
		for i := range nodes {
			n := &nodes[i]
			if !affected[n.Summary.ID] {
				continue
			}
			affectedIDs = append(affectedIDs, n.Summary.ID)
			n.Summary.ControlRevision, e = increment(n.Summary.ControlRevision)
			if e != nil {
				return 0, nil, e
			}
			n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
			if e != nil {
				return 0, nil, e
			}
			n.Summary.NextOwner = "user"
			if n.Summary.ID == node.ID {
				n.Summary.Stop = &domain.GraphStopRecord{RequestID: in.RequestID, AcceptedAt: domain.Now(), GraphControlRevision: h.Summary.ControlRevision, NodeControlRevision: ptr(n.Summary.ControlRevision), TargetAttemptIDs: targets}
				if !graphRetainedTerminal(n.Summary.State) {
					n.Summary.State = "cancelled"
					if active[n.Summary.TaskID] {
						n.Summary.State = "cancelling"
					}
				}
				if !graphRetainedTerminal(n.Summary.State) || n.Summary.State == "cancelled" || n.Summary.State == "cancelling" {
					n.Summary.NextAction = "node_stopped"
				}
			} else if !graphRetainedTerminal(n.Summary.State) {
				n.Summary.State = "blocked"
				n.Summary.NextAction = "dependency_stopped"
			}
			if e = saveGraphNode(ctx, tx, *n); e != nil {
				return 0, nil, e
			}
		}
		if h.DispatchAuthority != nil {
			remaining := []domain.GraphNodeAuthority{}
			for _, authority := range h.DispatchAuthority.NodeAuthorities {
				if !affected[authority.NodeID] {
					remaining = append(remaining, authority)
				}
			}
			h.DispatchAuthority.NodeAuthorities = remaining
			if len(remaining) == 0 {
				h.DispatchAuthority.State = "suspended"
			}
		}
		h.Summary.State = "blocked"
		h.Summary.NextAction = "Inspect stopped and dependent nodes; independent authorized work remains eligible."
		h.Summary.NextOwner = "user"
		if h.DispatchAuthority != nil && h.DispatchAuthority.State != "suspended" {
			h.Summary.NextOwner = "council"
		}
		h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		h.Summary.UpdatedAt = domain.Now()
		h.Summary.Counts, e = graphCounts(nodes)
		if e != nil {
			return 0, nil, e
		}
		if e = saveGraphHeader(ctx, tx, h); e != nil {
			return 0, nil, e
		}
		node = nodes[selected].Summary
		return 202, domain.GraphStopResult{Graph: h.Summary, Node: &node, StopAccepted: true, Reason: "accepted", TargetAttemptIDs: targets, AffectedNodeIDs: affectedIDs}, nil
	})
}
