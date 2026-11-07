package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// RecoveryResumeFacts are captured by the controller after receipt replay and
// before the mutation. Artifact proofs remain controller facts, never JSON
// supplied by the API caller.
type RecoveryResumeFacts struct {
	Workflow   *domain.WorkflowStepAdmissionFacts
	GraphInput *domain.GraphAdvanceInput
	Graph      *domain.GraphAdvanceFacts
}

func RecoveryResumeFingerprint(checkpointID string, in domain.ResumeCheckpointInput) string {
	return Fingerprint("checkpoints.resume", struct {
		CheckpointID string
		Input        domain.ResumeCheckpointInput
	}{checkpointID, in})
}

// ResumeCheckpoint clears only the selected durable pause. It does not admit a
// worker without controller-prepared and transactionally rechecked facts.
func (s *Store) ResumeCheckpoint(ctx context.Context, ownerID, checkpointID string, in domain.ResumeCheckpointInput, prepared ...RecoveryResumeFacts) domain.Result {
	if invalid := domain.ValidateResumeCheckpointInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !domain.ValidID(checkpointID) {
		return domain.Failure(in.RequestID, domain.Invalid("checkpoint identity"))
	}
	fp := RecoveryResumeFingerprint(checkpointID, in)
	return s.mutate(ctx, in.RequestID, "checkpoints.resume", fp, func(tx *sql.Tx) (int, any, error) {
		if len(prepared) > 1 {
			return 0, nil, domain.Invalid("one prepared resume plan")
		}
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		if !domain.ValidID(ownerID) || gate.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "Use the current Council owner to resume this checkpoint.")
		}
		if err = checkGate(gate); err != nil {
			return 0, nil, err
		}
		var raw, state string
		var revision int64
		if err = tx.QueryRowContext(ctx, "SELECT revision,state,data FROM recovery_checkpoints WHERE id=?", checkpointID).Scan(&revision, &state, &raw); err != nil {
			if err == sql.ErrNoRows {
				return 0, nil, notFound()
			}
			return 0, nil, storageError()
		}
		cp, err := decode[domain.RecoveryCheckpoint](raw)
		if err != nil || cp.ID != checkpointID || !validStoredRevision(revision, cp.Revision) || cp.State != state {
			return 0, nil, storageError()
		}
		if cp.Revision != in.ExpectedCheckpointRevision {
			return 0, nil, domain.RevisionConflict(cp.Revision)
		}
		if cp.State != "paused" {
			return 0, nil, domain.Fail("checkpoint_not_paused", "Wait for a durable paused checkpoint and inspect any blockers.")
		}
		if !recoverySameScope(cp.Scope, in.Scope) {
			return 0, nil, domain.Fail("checkpoint_scope_conflict", "Resume the exact checkpoint scope and captured instruction versions.")
		}
		current := cp
		if err = s.recoveryCheckpointProjection(ctx, tx, &current); err != nil {
			return 0, nil, err
		}
		if current.State != "paused" || len(current.Blockers) != 0 || !reflect.DeepEqual(current.CompletedAttemptIDs, cp.CompletedAttemptIDs) {
			return 0, nil, domain.Fail("checkpoint_blocked", "The checkpoint's cleanup, evidence or retained work changed; inspect current recovery state.")
		}
		if err = s.recoveryResumeAuthority(ctx, tx, cp.Scope, in.Scope); err != nil {
			return 0, nil, err
		}
		// Invalidate pre-pause callback authority before clearing the fence.
		if err = s.recoveryResumeInvalidate(ctx, tx, in.Scope); err != nil {
			return 0, nil, err
		}
		cp.Revision, err = increment(cp.Revision)
		if err != nil {
			return 0, nil, err
		}
		cp.State, cp.UpdatedAt = "resumed", domain.Now()
		cp.NextAction = "Request the next eligible work explicitly; current inputs and evidence must be verified again."
		result, err := tx.ExecContext(ctx, "UPDATE recovery_checkpoints SET revision=?,state=?,data=? WHERE id=? AND revision=? AND state='paused'", intRevision(cp.Revision), cp.State, encode(cp), cp.ID, revision)
		if err != nil {
			return 0, nil, storageError()
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return 0, nil, storageError()
		}
		blockers, err := s.recoveryResumeOverlaps(ctx, tx, in.Scope)
		if err != nil {
			return 0, nil, err
		}
		admitted := []string{}
		var graphResult *domain.GraphAdvanceResult
		if (len(blockers) == 0 || in.Scope.Kind == "graph") && len(prepared) == 1 {
			admitted, graphResult, err = s.recoveryResumeAdmit(ctx, tx, ownerID, checkpointID, in, prepared[0])
			if err != nil {
				return 0, nil, err
			}
		} else if len(blockers) == 0 {
			blockers = append(blockers, "continuation_not_prepared")
		}
		nextAction := cp.NextAction
		if len(blockers) != 0 {
			nextAction = "Inspect the remaining blockers before admitting unfinished work."
		} else if len(admitted) != 0 {
			nextAction = "Inspect the newly admitted unfinished work."
		} else {
			block := ""
			if in.Scope.Kind == "member" && graphResult != nil {
				nodes, readErr := s.graphNodes(ctx, tx, in.Scope.GraphID)
				if readErr != nil {
					return 0, nil, readErr
				}
				for _, node := range nodes {
					if node.Summary.ID == in.Scope.NodeID && node.Blocker != nil {
						block = node.Blocker.Code
						break
					}
				}
			} else if graphResult != nil && graphResult.DispatchAuthority != nil && graphResult.DispatchAuthority.State == "capacity_wait" {
				block = "capacity_wait"
			}
			if block == "capacity_wait" || block == "attempt_active" {
				blockers = append(blockers, block)
				nextAction = "Wait for a verified capacity release to continue the registered work."
			} else if block != "" {
				blockers = append(blockers, block)
				nextAction = "Inspect the recorded blocker before admitting unfinished work."
			} else {
				blockers = append(blockers, "no_eligible_unfinished_work")
				nextAction = "Inspect the checkpoint and current work before taking another action."
			}
		}
		return 200, struct {
			Checkpoint         domain.RecoveryCheckpoint  `json:"checkpoint"`
			AdmittedAttemptIDs []string                   `json:"admittedAttemptIds"`
			Graph              *domain.GraphAdvanceResult `json:"graph,omitempty"`
			Blockers           []string                   `json:"blockers"`
			NextAction         string                     `json:"nextAction"`
		}{cp, admitted, graphResult, blockers, nextAction}, nil
	})
}

func (s *Store) recoveryResumeAdmit(ctx context.Context, tx *sql.Tx, ownerID, checkpointID string, in domain.ResumeCheckpointInput, facts RecoveryResumeFacts) ([]string, *domain.GraphAdvanceResult, error) {
	ids := []string{}
	switch in.Scope.Kind {
	case "task":
		if facts.Workflow == nil && facts.GraphInput == nil && facts.Graph == nil {
			return ids, nil, nil
		}
		if facts.Workflow == nil || facts.GraphInput != nil || facts.Graph != nil {
			return nil, nil, domain.Fail("continuation_not_prepared", "Prepare and verify the current unfinished workflow before resuming.")
		}
		f := *facts.Workflow
		if f.ExpectedTaskRevision != in.Scope.ExpectedTaskRevision {
			return nil, nil, domain.Fail("checkpoint_scope_conflict", "The prepared workflow belongs to a different task revision.")
		}
		task, err := s.task(ctx, tx, in.Scope.TaskID)
		if err != nil {
			return nil, nil, err
		}
		f.ExpectedTaskRevision = task.Revision
		advance := domain.WorkflowAdvanceInput{RequestID: in.RequestID, ExpectedRevision: task.Revision, WorkflowRevision: f.ExpectedWorkflowRevision, TaskVersion: in.Scope.TaskVersion}
		_, value, err := s.workflowExplicitStageTx(ctx, tx, ownerID, task.ID, advance, f)
		if err != nil {
			return nil, nil, err
		}
		admission, ok := value.(domain.WorkflowStepAdmission)
		if !ok || admission.Attempt.ID == "" {
			return nil, nil, storageError()
		}
		return append(ids, admission.Attempt.ID), nil, nil
	case "member":
		return s.recoveryResumeMember(ctx, tx, ownerID, checkpointID, in, facts)
	case "graph":
		if facts.Workflow != nil || facts.GraphInput == nil || facts.Graph == nil {
			return nil, nil, domain.Fail("continuation_not_prepared", "Prepare and verify the current graph inputs before resuming.")
		}
		advance := *facts.GraphInput
		if advance.RequestID != in.RequestID || advance.GraphVersion != in.Scope.GraphVersion || advance.ExpectedGraphControlRevision != in.Scope.ExpectedGraphControlRevision || advance.ExpectedBudgetAuthorityEpoch != in.Scope.ExpectedBudgetAuthorityEpoch || advance.ExpectedWindowNumber != in.Scope.ExpectedWindowNumber || !reflect.DeepEqual(advance.NodeAuthorities, in.Scope.NodeAuthorities) || advance.RebindNodeIDs != nil && len(*advance.RebindNodeIDs) != 0 {
			return nil, nil, domain.Fail("checkpoint_scope_conflict", "The prepared graph facts do not match this exact checkpoint authority.")
		}
		h, err := s.graphHeader(ctx, tx, in.Scope.GraphID)
		if err != nil {
			return nil, nil, err
		}
		advance.ExpectedGraphControlRevision = h.Summary.ControlRevision
		graphFacts, err := s.recoveryResumeGraphFacts(ctx, tx, in.Scope, *facts.Graph, h.Summary.ControlRevision)
		if err != nil {
			return nil, nil, err
		}
		_, value, err := s.advanceGraphTx(ctx, tx, ownerID, in.Scope.GraphID, advance, graphFacts)
		if err != nil {
			return nil, nil, err
		}
		result, ok := value.(domain.GraphAdvanceResult)
		if !ok {
			return nil, nil, storageError()
		}
		ids = append(ids, result.PreparationAttemptIDs...)
		for _, dispatch := range result.Dispatches {
			ids = append(ids, dispatch.Dispatch.AttemptID)
		}
		return ids, &result, nil
	default:
		return nil, nil, domain.Invalid("resume scope")
	}
}

// Keep frozen source/proof facts intact; only a server-validated graph control
// generation may be rebound, and nodes with their own pause are excluded.
func (s *Store) recoveryResumeGraphFacts(ctx context.Context, tx *sql.Tx, scope domain.RecoveryScope, source domain.GraphAdvanceFacts, revision string) (domain.GraphAdvanceFacts, error) {
	paused := map[string]bool{}
	pauses, err := s.recoveryActiveGraphCheckpoints(ctx, tx, scope.GraphID)
	if err != nil {
		return domain.GraphAdvanceFacts{}, err
	}
	for _, pause := range pauses {
		if pause.Scope.Kind == "member" {
			paused[pause.Scope.NodeID] = true
		}
	}
	eligible := func(nodeID string) (bool, error) {
		if paused[nodeID] {
			return false, nil
		}
		err := s.requireRecoveryAncestors(ctx, tx, scope.GraphID, nodeID)
		var fault *domain.Error
		if errors.As(err, &fault) && fault.Code == "scope_paused" {
			return false, nil
		}
		return err == nil, err
	}
	if len(source.Preparations) > domain.MaxGraphNodes || len(source.Admissions) > domain.MaxGraphNodes || len(source.Stages) > domain.MaxGraphNodes {
		return domain.GraphAdvanceFacts{}, domain.Invalid("bounded graph continuation facts")
	}
	out := domain.GraphAdvanceFacts{}
	for _, item := range source.Preparations {
		if item.Control.NodeID == nil || item.Control.GraphID != scope.GraphID || item.Control.GraphControlRevision != scope.ExpectedGraphControlRevision {
			return out, graphControlError()
		}
		allowed, err := eligible(*item.Control.NodeID)
		if err != nil {
			return out, err
		}
		if allowed {
			item.Control.GraphControlRevision = revision
			out.Preparations = append(out.Preparations, item)
		}
	}
	for _, item := range source.Admissions {
		if item.Control.NodeID == nil || item.Control.GraphID != scope.GraphID || item.Control.GraphControlRevision != scope.ExpectedGraphControlRevision {
			return out, graphControlError()
		}
		allowed, err := eligible(*item.Control.NodeID)
		if err != nil {
			return out, err
		}
		if allowed {
			item.Control.GraphControlRevision = revision
			out.Admissions = append(out.Admissions, item)
		}
	}
	for _, item := range source.Stages {
		if item.Graph == nil || item.Graph.Control.NodeID == nil || item.Graph.Control.GraphID != scope.GraphID || item.Graph.Control.GraphControlRevision != scope.ExpectedGraphControlRevision {
			return out, graphControlError()
		}
		allowed, err := eligible(*item.Graph.Control.NodeID)
		if err != nil {
			return out, err
		}
		if allowed {
			graph := *item.Graph
			graph.Control.GraphControlRevision = revision
			item.Graph = &graph
			out.Stages = append(out.Stages, item)
		}
	}
	return out, nil
}

func (s *Store) recoveryResumeMember(ctx context.Context, tx *sql.Tx, ownerID, checkpointID string, in domain.ResumeCheckpointInput, facts RecoveryResumeFacts) ([]string, *domain.GraphAdvanceResult, error) {
	ids := []string{}
	if facts.Workflow != nil || facts.Graph == nil || facts.GraphInput == nil {
		return nil, nil, domain.Fail("continuation_not_prepared", "Prepare the selected member's current input and evidence before resuming.")
	}
	a := facts.GraphInput
	if a.RequestID != in.RequestID || a.GraphVersion != in.Scope.GraphVersion || a.ExpectedGraphControlRevision != in.Scope.ExpectedGraphControlRevision || a.ExpectedBudgetAuthorityEpoch != in.Scope.ExpectedBudgetAuthorityEpoch || a.ExpectedWindowNumber != in.Scope.ExpectedWindowNumber || a.RebindNodeIDs != nil && len(*a.RebindNodeIDs) != 0 {
		return nil, nil, graphControlError()
	}
	count := len(facts.Graph.Preparations) + len(facts.Graph.Admissions) + len(facts.Graph.Stages)
	if count == 0 {
		return ids, nil, nil
	}
	if count != 1 {
		return nil, nil, domain.Invalid("one selected member continuation")
	}
	h, err := s.graphHeader(ctx, tx, in.Scope.GraphID)
	if err != nil || h.DispatchAuthority == nil {
		return nil, nil, graphControlError()
	}
	nodes, err := s.graphNodes(ctx, tx, in.Scope.GraphID)
	if err != nil || len(a.NodeAuthorities) != len(nodes) {
		return nil, nil, graphControlError()
	}
	var node domain.GraphNodeSummary
	for i, current := range nodes {
		captured := a.NodeAuthorities[i]
		if current.Summary.ID != captured.NodeID {
			return nil, nil, graphControlError()
		}
		if current.Summary.ID == in.Scope.NodeID {
			if captured.TaskVersion != in.Scope.TaskVersion || captured.NodeControlRevision != in.Scope.ExpectedNodeControlRevision {
				return nil, nil, graphControlError()
			}
			node = current.Summary
		}
	}
	if node.ID == "" || node.TaskID != in.Scope.TaskID {
		return nil, nil, graphControlError()
	}
	if _, err = tx.ExecContext(ctx, "UPDATE recovery_graph_scopes SET state='superseded' WHERE graph_id=? AND node_id=? AND state='active'", in.Scope.GraphID, in.Scope.NodeID); err != nil {
		return nil, nil, storageError()
	}
	var eventHighWater int64
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0) FROM attempt_events").Scan(&eventHighWater); err != nil {
		return nil, nil, storageError()
	}
	registration := struct {
		Scope          domain.RecoveryScope `json:"scope"`
		EventHighWater int64                `json:"eventHighWater"`
	}{in.Scope, eventHighWater}
	_, err = tx.ExecContext(ctx, `INSERT INTO recovery_graph_scopes(checkpoint_id,request_id,graph_id,node_id,task_id,owner_instance_id,graph_version,graph_control_revision,node_control_revision,generation,state,data) VALUES(?,?,?,?,?,?,?,?,?,?,'active',?)`, checkpointID, in.RequestID, in.Scope.GraphID, in.Scope.NodeID, in.Scope.TaskID, ownerID, intRevision(in.Scope.GraphVersion), intRevision(h.Summary.ControlRevision), intRevision(node.ControlRevision), intRevision(h.DispatchAuthority.Generation), encode(registration))
	if err != nil {
		return nil, nil, storageError()
	}
	validControl := func(c domain.GraphDispatchBinding) bool {
		return c.GraphID == in.Scope.GraphID && c.GraphVersion == in.Scope.GraphVersion && c.GraphControlRevision == in.Scope.ExpectedGraphControlRevision && c.BudgetAuthorityEpoch == in.Scope.ExpectedBudgetAuthorityEpoch && c.WindowNumber == in.Scope.ExpectedWindowNumber && c.NodeID != nil && *c.NodeID == in.Scope.NodeID && c.TaskID != nil && *c.TaskID == in.Scope.TaskID && c.TaskVersion != nil && *c.TaskVersion == in.Scope.TaskVersion && c.NodeControlRevision != nil && *c.NodeControlRevision == in.Scope.ExpectedNodeControlRevision
	}
	bind := func(c *domain.GraphDispatchBinding) {
		c.OwnerInstanceID = ownerID
		c.Generation = h.DispatchAuthority.Generation
		c.NodeControlRevision = ptr(node.ControlRevision)
	}
	trigger := domain.GraphFreshTrigger{Kind: "explicit", ID: in.RequestID, OwnerGeneration: h.DispatchAuthority.Generation}
	result := &domain.GraphAdvanceResult{Graph: h.Summary, PreparationAttemptIDs: []string{}, Dispatches: []domain.GraphDispatchView{}, ReboundNodes: []domain.GraphReboundNode{}}
	if len(facts.Graph.Preparations) != 0 {
		f := facts.Graph.Preparations[0]
		if !validControl(f.Control) {
			return nil, nil, graphControlError()
		}
		bind(&f.Control)
		f.Trigger = trigger
		var op domain.GraphInputOperation
		if err = s.beginGraphInputTx(ctx, tx, ownerID, in.Scope.GraphID, in.Scope.NodeID, f, &op); err != nil {
			return nil, nil, err
		}
		ids = append(ids, op.Attempt.ID)
		result.PreparationAttemptIDs = append(result.PreparationAttemptIDs, op.Attempt.ID)
	} else if len(facts.Graph.Admissions) != 0 {
		f := facts.Graph.Admissions[0]
		if f.NodeID != in.Scope.NodeID || !validControl(f.Control) {
			return nil, nil, graphControlError()
		}
		bind(&f.Control)
		var dispatch domain.GraphDispatchView
		if err = s.admitGraphNodeTx(ctx, tx, ownerID, in.Scope.GraphID, trigger, f, &dispatch); err != nil {
			return nil, nil, err
		}
		if dispatch.Dispatch.AttemptID != "" {
			ids = append(ids, dispatch.Dispatch.AttemptID)
			result.Dispatches = append(result.Dispatches, dispatch)
		}
	} else {
		f := facts.Graph.Stages[0]
		if f.Graph == nil || !validControl(f.Graph.Control) || f.ExpectedTaskRevision != in.Scope.ExpectedTaskRevision {
			return nil, nil, graphControlError()
		}
		graph := *f.Graph
		bind(&graph.Control)
		graph.Trigger = trigger
		f.Graph = &graph
		task, readErr := s.task(ctx, tx, in.Scope.TaskID)
		if readErr != nil {
			return nil, nil, readErr
		}
		f.ExpectedTaskRevision = task.Revision
		workflow, readErr := s.selectedWorkflow(ctx, tx, task.ID, domain.WorkflowSelector{})
		if readErr != nil || workflow.Workflow == nil {
			return nil, nil, workflowUnavailable()
		}
		budget, readErr := s.taskBudget(ctx, tx, task.ID)
		if readErr != nil {
			return nil, nil, readErr
		}
		w := *workflow.Workflow
		admission, admitErr := s.admitWorkflowContinuation(ctx, tx, ownerID, &task, &w, budget, f, in.RequestID, "")
		if admitErr != nil {
			return nil, nil, admitErr
		}
		if admission != nil {
			ids = append(ids, admission.Attempt.ID)
			dispatch, readErr := s.graphDispatchForAttempt(ctx, tx, admission.Attempt.ID)
			if readErr != nil {
				return nil, nil, readErr
			}
			result.Dispatches = append(result.Dispatches, dispatch.Public)
		}
	}
	h, err = s.graphHeader(ctx, tx, in.Scope.GraphID)
	if err != nil {
		return nil, nil, err
	}
	result.Graph, result.DispatchAuthority = h.Summary, h.DispatchAuthority
	result.Budget, err = s.graphBudgetView(ctx, tx, h)
	if err != nil {
		return nil, nil, err
	}
	return ids, result, nil
}

func recoverySameScope(saved, current domain.RecoveryScope) bool {
	if saved.Kind != current.Kind || saved.TaskID != current.TaskID || saved.TaskVersion != current.TaskVersion || saved.GraphID != current.GraphID || saved.GraphVersion != current.GraphVersion || saved.NodeID != current.NodeID {
		return false
	}
	if saved.Kind == "graph" {
		if len(saved.NodeAuthorities) != len(current.NodeAuthorities) {
			return false
		}
		for i := range saved.NodeAuthorities {
			if saved.NodeAuthorities[i].NodeID != current.NodeAuthorities[i].NodeID || saved.NodeAuthorities[i].TaskVersion != current.NodeAuthorities[i].TaskVersion {
				return false
			}
		}
	}
	return true
}

func (s *Store) recoveryResumeAuthority(ctx context.Context, tx *sql.Tx, saved, scope domain.RecoveryScope) error {
	budgetTaskID := scope.TaskID
	if scope.Kind != "graph" {
		task, err := s.task(ctx, tx, scope.TaskID)
		if err != nil {
			return err
		}
		if task.CurrentVersion != scope.TaskVersion || task.Revision != scope.ExpectedTaskRevision {
			return domain.Fail("checkpoint_scope_conflict", "Task instructions or revision changed; inspect the current task.")
		}
		member, err := s.graphMembership(ctx, tx, task.ID)
		if err != nil {
			return err
		}
		if scope.Kind == "task" && member != nil || scope.Kind == "member" && (member == nil || member.NodeID == nil || member.GraphID != scope.GraphID || *member.NodeID != scope.NodeID) {
			return domain.Fail("checkpoint_scope_conflict", "The task's current graph membership differs from the checkpoint.")
		}
		latest, err := s.latest(ctx, tx, task)
		if err != nil {
			return err
		}
		if latest != nil && latest.Stop != nil {
			return domain.Fail("scope_stopped", "Accepted Stop remains authoritative; inspect the retained result.")
		}
	}
	if scope.Kind != "task" {
		h, err := s.graphHeader(ctx, tx, scope.GraphID)
		if err != nil {
			return err
		}
		if h.Summary.Stop != nil || h.Summary.GraphVersion != scope.GraphVersion || h.Summary.ControlRevision != scope.ExpectedGraphControlRevision || h.Summary.BudgetAuthorityEpoch != scope.ExpectedBudgetAuthorityEpoch {
			return graphControlError()
		}
		budgetTaskID = h.Summary.RootTaskID
		nodes, err := s.graphNodes(ctx, tx, scope.GraphID)
		if err != nil {
			return err
		}
		found := false
		for i, n := range nodes {
			if scope.Kind == "member" && n.Summary.ID == scope.NodeID {
				found = n.Summary.TaskID == scope.TaskID && n.Summary.TaskVersion == scope.TaskVersion && n.Summary.ControlRevision == scope.ExpectedNodeControlRevision && n.Summary.Stop == nil
			}
			if scope.Kind == "graph" {
				if len(nodes) != len(scope.NodeAuthorities) {
					return graphControlError()
				}
				a := scope.NodeAuthorities[i]
				if n.Summary.ID != a.NodeID || n.Summary.TaskVersion != a.TaskVersion || n.Summary.ControlRevision != a.NodeControlRevision {
					return graphControlError()
				}
			}
		}
		if scope.Kind == "member" && !found {
			return graphControlError()
		}
	}
	budget, err := s.rawTaskBudget(ctx, tx, budgetTaskID)
	if err != nil {
		return err
	}
	if budget == nil || budget.RemainingAdmissions == "0" || scope.Kind != "task" && budget.CurrentWindow.Number != scope.ExpectedWindowNumber {
		return domain.Fail("budget_conflict", "The current task budget or graph window cannot authorize unfinished work.")
	}
	if budget.CurrentWindow.DeadlineAt != nil {
		deadline, invalid := domain.ParseRoutingTime(*budget.CurrentWindow.DeadlineAt)
		if invalid != nil || !time.Now().UTC().Before(deadline) {
			return domain.Fail("budget_conflict", "The current budget window has expired or cannot be verified.")
		}
	}
	_ = saved
	return nil
}

func (s *Store) recoveryResumeInvalidate(ctx context.Context, tx *sql.Tx, scope domain.RecoveryScope) error {
	if scope.Kind != "graph" {
		task, err := s.task(ctx, tx, scope.TaskID)
		if err != nil {
			return err
		}
		task.Revision, err = increment(task.Revision)
		if err != nil {
			return err
		}
		task.StateRevision, err = increment(task.StateRevision)
		if err != nil {
			return err
		}
		task.UpdatedAt = domain.Now()
		if err = saveTask(ctx, tx, task); err != nil {
			return err
		}
	}
	if scope.Kind == "member" {
		nodes, err := s.graphNodes(ctx, tx, scope.GraphID)
		if err != nil {
			return err
		}
		for i := range nodes {
			if nodes[i].Summary.ID != scope.NodeID {
				continue
			}
			nodes[i].Summary.ControlRevision, err = increment(nodes[i].Summary.ControlRevision)
			if err != nil {
				return err
			}
			nodes[i].Summary.StateRevision, err = increment(nodes[i].Summary.StateRevision)
			if err != nil {
				return err
			}
			return saveGraphNode(ctx, tx, nodes[i])
		}
		return graphControlError()
	}
	if scope.Kind == "graph" {
		h, err := s.graphHeader(ctx, tx, scope.GraphID)
		if err != nil {
			return err
		}
		h.Summary.ControlRevision, err = increment(h.Summary.ControlRevision)
		if err != nil {
			return err
		}
		h.Summary.StateRevision, err = increment(h.Summary.StateRevision)
		if err != nil {
			return err
		}
		h.Summary.UpdatedAt = domain.Now()
		if h.DispatchAuthority != nil {
			h.DispatchAuthority.Generation, err = increment(h.DispatchAuthority.Generation)
			if err != nil {
				return err
			}
			h.DispatchAuthority.GraphControlRevision = h.Summary.ControlRevision
			h.DispatchAuthority.State = "suspended"
		}
		return saveGraphHeader(ctx, tx, h)
	}
	return nil
}

func (s *Store) recoveryResumeOverlaps(ctx context.Context, tx *sql.Tx, scope domain.RecoveryScope) ([]string, error) {
	blockers := []string{}
	if scope.Kind == "task" {
		return blockers, nil
	}
	pauses, err := s.recoveryActiveGraphCheckpoints(ctx, tx, scope.GraphID)
	if err != nil {
		return nil, err
	}
	for _, pause := range pauses {
		if pause.Scope.Kind == "graph" || scope.Kind == "graph" || pause.Scope.NodeID == scope.NodeID {
			blockers = append(blockers, pause.ID)
		}
	}
	return blockers, nil
}
