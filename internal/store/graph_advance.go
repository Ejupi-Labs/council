package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) advanceGraph(ctx context.Context, ownerID, graphID string, in domain.GraphAdvanceInput, f domain.GraphAdvanceFacts) domain.Result {
	if !domain.ValidID(ownerID) || !domain.ValidID(graphID) {
		return domain.Failure(in.RequestID, domain.Invalid("exact graph and mutation owner"))
	}
	return s.mutate(ctx, in.RequestID, "graphs.advance", GraphAdvanceFingerprint(graphID, in), func(tx *sql.Tx) (int, any, error) {
		return s.advanceGraphTx(ctx, tx, ownerID, graphID, in, f)
	})
}

func (s *Store) advanceGraphTx(ctx context.Context, tx *sql.Tx, ownerID, graphID string, in domain.GraphAdvanceInput, f domain.GraphAdvanceFacts) (int, any, error) {
	if e := s.requireRecoveryAdmission(ctx, tx, "", graphID, ""); e != nil {
		return 0, nil, e
	}
	h, e := s.graphHeader(ctx, tx, graphID)
	if e != nil {
		return 0, nil, e
	}
	gate, e := s.gate(ctx, tx)
	if e != nil {
		return 0, nil, e
	}
	if e = checkGate(gate); e != nil {
		return 0, nil, e
	}
	if gate.OwnerInstanceID != ownerID {
		return 0, nil, domain.Fail("owner_unavailable", "Capture explicit graph intent for the current owner.")
	}
	if h.Summary.GraphVersion != in.GraphVersion || h.Summary.ControlRevision != in.ExpectedGraphControlRevision {
		return 0, nil, graphControlError()
	}
	budget, e := s.rawTaskBudget(ctx, tx, h.Summary.RootTaskID)
	if e != nil {
		return 0, nil, e
	}
	if budget == nil || h.Summary.BudgetAuthorityEpoch != in.ExpectedBudgetAuthorityEpoch || budget.CurrentWindow.Number != in.ExpectedWindowNumber {
		return 0, nil, domain.Fail("budget_conflict", "Capture the graph's current budget authority and window before explicit Advance.")
	}
	nodes, e := s.graphNodes(ctx, tx, graphID)
	if e != nil {
		return 0, nil, e
	}
	// Controller facts may have been captured immediately before an independent
	// member was paused. Drop only that member's prospective work under the
	// same transaction; dependency guards still reject paused ancestors.
	f, pausedNodes, e := s.graphAdvanceWithoutPausedMembers(ctx, tx, graphID, in, f)
	if e != nil {
		return 0, nil, e
	}
	relevant, stopAllowed, e := s.graphAdvanceRelevantNodes(ctx, tx, h, nodes, in, f)
	if e != nil {
		return 0, nil, e
	}
	authorities := append([]domain.GraphNodeAuthority{}, in.NodeAuthorities...)
	captured := map[string]bool{}
	eligible := false
	for _, authority := range authorities {
		captured[authority.NodeID] = true
		found := false
		for _, n := range nodes {
			if n.Summary.ID != authority.NodeID {
				continue
			}
			found = true
			task, e := s.task(ctx, tx, n.Summary.TaskID)
			if e != nil {
				return 0, nil, e
			}
			if relevant[authority.NodeID] && (task.CurrentVersion != authority.TaskVersion || n.Summary.TaskVersion != authority.TaskVersion || n.Summary.ControlRevision != authority.NodeControlRevision) {
				if !stopAllowed[authority.NodeID] || task.CurrentVersion != authority.TaskVersion || !graphAdvanceStopOnlyDrift(n.Summary, authority) {
					return 0, nil, graphControlError()
				}
			}
			if relevant[authority.NodeID] && !pausedNodes[authority.NodeID] && n.Summary.Stop == nil && !graphRetainedTerminal(n.Summary.State) {
				eligible = true
			}
		}
		if !found {
			return 0, nil, graphControlError()
		}
	}
	for id := range relevant {
		if !captured[id] {
			return 0, nil, graphControlError()
		}
	}
	// A stopped graph requires its own explicit resumption transition. Current
	// input preparation, initial admissions and workflow stages below share
	// this registration/receipt transaction and their existing guarded helpers.
	if h.Summary.Stop != nil {
		return 0, nil, domain.Fail("capability_unavailable", "Stopped graph scope requires its exact guarded resumption transition.")
	}
	rebound := []domain.GraphReboundNode{}
	preparations := append([]domain.GraphInputPreparationFacts{}, f.Preparations...)
	named := []string{}
	if in.RebindNodeIDs != nil {
		named = *in.RebindNodeIDs
	}
	if len(preparations) > domain.MaxGraphNodes || len(named) > len(preparations) {
		return 0, nil, domain.Invalid("bounded freshly verified preparations for each explicitly named rebind")
	}
	byNode := map[string]int{}
	for i, preparation := range preparations {
		if preparation.Control.NodeID == nil {
			return 0, nil, graphControlError()
		}
		id := *preparation.Control.NodeID
		if _, duplicate := byNode[id]; duplicate {
			return 0, nil, domain.Invalid("one input preparation per named node")
		}
		byNode[id] = i
	}
	for _, preparation := range preparations {
		id := *preparation.Control.NodeID
		authorized, explicitRebind := false, false
		for _, authority := range in.NodeAuthorities {
			if authority.NodeID == id && preparation.Control.TaskVersion != nil && *preparation.Control.TaskVersion == authority.TaskVersion && preparation.Control.NodeControlRevision != nil && *preparation.Control.NodeControlRevision == authority.NodeControlRevision {
				authorized = true
			}
		}
		for _, namedID := range named {
			if namedID == id {
				explicitRebind = true
			}
		}
		if !authorized {
			return 0, nil, graphControlError()
		}
		if !explicitRebind {
			for _, node := range nodes {
				if node.Summary.ID == id && (node.Summary.InputID != nil || node.Summary.NodeAdmissionID != nil) {
					return 0, nil, domain.Fail("graph_input_stale", "An existing received/workflow input requires its own explicit guarded transition; do not silently rebind it.")
				}
			}
		}
	}
	for _, id := range named {
		preparationIndex, supplied := byNode[id]
		nodeIndex, authorityIndex := -1, -1
		for i := range nodes {
			if nodes[i].Summary.ID == id {
				nodeIndex = i
			}
		}
		for i := range authorities {
			if authorities[i].NodeID == id {
				authorityIndex = i
			}
		}
		if !supplied || nodeIndex < 0 || authorityIndex < 0 {
			return 0, nil, graphControlError()
		}
		derived, e := s.rebindGraphNode(ctx, tx, ownerID, h, nodes, nodeIndex, in.RequestID, preparations[preparationIndex])
		if e != nil {
			return 0, nil, e
		}
		rebound = append(rebound, derived)
		node := nodes[nodeIndex].Summary
		authorities[authorityIndex] = domain.GraphNodeAuthority{NodeID: node.ID, TaskVersion: node.TaskVersion, NodeControlRevision: node.ControlRevision}
		control := preparations[preparationIndex].Control
		control.TaskVersion, control.NodeControlRevision = ptr(node.TaskVersion), ptr(node.ControlRevision)
		preparations[preparationIndex].Control = control
		eligible = true
	}
	generation := "1"
	if h.DispatchAuthority != nil {
		generation, e = increment(h.DispatchAuthority.Generation)
		if e != nil {
			return 0, nil, e
		}
	}
	var highwater int64
	if e = tx.QueryRowContext(ctx, "SELECT coalesce(max(sequence),0) FROM attempt_events").Scan(&highwater); e != nil || highwater < 0 {
		return 0, nil, storageError()
	}
	state := "active"
	now := time.Now().UTC()
	deadline := h.PreparationDeadlineAt
	if budget.CurrentWindow.DeadlineAt != nil {
		deadline = *budget.CurrentWindow.DeadlineAt
	}
	end, invalid := domain.ParseRoutingTime(deadline)
	if invalid != nil {
		return 0, nil, storageError()
	}
	if !eligible || !now.Before(end) {
		state = "suspended"
	}
	h.DispatchAuthority = &domain.GraphDispatchAuthorityView{OwnerInstanceID: ownerID, Generation: generation, GraphControlRevision: h.Summary.ControlRevision, BudgetAuthorityEpoch: h.Summary.BudgetAuthorityEpoch, WindowNumber: budget.CurrentWindow.Number, NodeAuthorities: authorities, State: state}
	h.DispatchEventHighWater, h.DispatchRequestID = domain.Decimal(highwater), in.RequestID
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return 0, nil, e
	}
	h.Summary.UpdatedAt = now.Format("2006-01-02T15:04:05.000Z")
	if h.Summary.State != "completed" {
		h.Summary.NextOwner = "user"
		h.Summary.NextAction = "Inspect the current required outcomes or grant an explicit new common window."
		if state == "active" {
			h.Summary.NextOwner = "council"
			h.Summary.NextAction = "Prepare eligible current nodes from this explicit graph intent."
			if h.Summary.State != "running" && h.Summary.State != "preparing" && h.Summary.State != "cancelling" {
				h.Summary.State = "queued"
			}
		} else if h.Summary.State != "running" && h.Summary.State != "preparing" && h.Summary.State != "cancelling" {
			h.Summary.State = "blocked"
		}
	}
	if len(rebound) != 0 {
		h.Summary.Counts, e = graphCounts(nodes)
		if e != nil {
			return 0, nil, e
		}
	}
	if e = saveGraphHeader(ctx, tx, h); e != nil {
		return 0, nil, e
	}
	attempts := []string{}
	for _, preparation := range preparations {
		preparation.Control.Generation = generation
		preparation.Trigger = domain.GraphFreshTrigger{Kind: "explicit", ID: in.RequestID, OwnerGeneration: generation}
		var operation domain.GraphInputOperation
		if e = s.beginGraphInputTx(ctx, tx, ownerID, graphID, *preparation.Control.NodeID, preparation, &operation); e != nil {
			return 0, nil, e
		}
		attempts = append(attempts, operation.Attempt.ID)
	}
	initial, e := s.advanceGraphInitialAdmissions(ctx, tx, ownerID, h, in, f.Admissions)
	if e != nil {
		return 0, nil, e
	}
	dispatches, e := s.advanceGraphStages(ctx, tx, ownerID, h, in, f.Stages)
	if e != nil {
		return 0, nil, e
	}
	dispatches = append(initial, dispatches...)
	if len(attempts) != 0 || len(f.Stages) != 0 || len(f.Admissions) != 0 {
		h, e = s.graphHeader(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
	}
	view, e := s.graphBudgetView(ctx, tx, h)
	if e != nil {
		return 0, nil, e
	}
	return 200, domain.GraphAdvanceResult{Graph: h.Summary, PreparationAttemptIDs: attempts, Dispatches: dispatches, ReboundNodes: rebound, Budget: view, DispatchAuthority: h.DispatchAuthority}, nil
}

func (s *Store) graphAdvanceWithoutPausedMembers(ctx context.Context, tx *sql.Tx, graphID string, in domain.GraphAdvanceInput, source domain.GraphAdvanceFacts) (domain.GraphAdvanceFacts, map[string]bool, error) {
	paused := map[string]bool{}
	checkpoints, err := s.recoveryActiveGraphCheckpoints(ctx, tx, graphID)
	if err != nil {
		return domain.GraphAdvanceFacts{}, nil, err
	}
	for _, checkpoint := range checkpoints {
		if checkpoint.Scope.Kind == "member" {
			paused[checkpoint.Scope.NodeID] = true
		}
	}
	if in.RebindNodeIDs != nil {
		for _, nodeID := range *in.RebindNodeIDs {
			if paused[nodeID] {
				return domain.GraphAdvanceFacts{}, nil, domain.Fail("scope_paused", "Resume the selected member before explicitly rebinding its input.")
			}
		}
	}
	if len(source.Preparations) > domain.MaxGraphNodes || len(source.Admissions) > domain.MaxGraphNodes || len(source.Stages) > domain.MaxGraphNodes {
		return domain.GraphAdvanceFacts{}, nil, domain.Invalid("bounded graph advance facts")
	}
	out := domain.GraphAdvanceFacts{}
	for _, item := range source.Preparations {
		if item.Control.NodeID == nil || !paused[*item.Control.NodeID] {
			out.Preparations = append(out.Preparations, item)
		}
	}
	for _, item := range source.Admissions {
		if !paused[item.NodeID] {
			out.Admissions = append(out.Admissions, item)
		}
	}
	for _, item := range source.Stages {
		if item.Graph == nil || item.Graph.Control.NodeID == nil || !paused[*item.Graph.Control.NodeID] {
			out.Stages = append(out.Stages, item)
		}
	}
	return out, paused, nil
}
