package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// A scope fence is checked under the same Store transaction as each new
// admission. Already-admitted work retains its original execution authority.
func (s *Store) requireRecoveryAdmission(ctx context.Context, q queryer, taskID, graphID, nodeID string) error {
	if taskID != "" && graphID == "" {
		ref, err := s.graphMembership(ctx, q, taskID)
		if err != nil {
			return err
		}
		if ref != nil {
			graphID = ref.GraphID
			if ref.NodeID != nil {
				nodeID = *ref.NodeID
			}
		}
	}
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM recovery_checkpoints
        WHERE state IN ('pausing','paused') AND
        ((scope_kind='task' AND task_id=?) OR
         (scope_kind='member' AND (task_id=? OR (graph_id=? AND node_id=? AND node_id IS NOT NULL))) OR
         (scope_kind='graph' AND graph_id=? AND graph_id IS NOT NULL))`, taskID, taskID, graphID, nodeID, graphID).Scan(&count)
	if err != nil {
		return storageError()
	}
	if count != 0 {
		return domain.Fail("scope_paused", "This scope is paused. Inspect its checkpoint and explicitly resume before admitting new work.")
	}
	return nil
}

func (s *Store) recoveryGateOwner(ctx context.Context, tx *sql.Tx, ownerID string) error {
	gate, err := s.gate(ctx, tx)
	if err != nil {
		return err
	}
	if !domain.ValidID(ownerID) || gate.OwnerInstanceID != "" && gate.OwnerInstanceID != ownerID {
		return domain.Fail("owner_unavailable", "Contact the current Council owner before changing recovery controls.")
	}
	return nil
}

func recoveryNullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) saveNewRecoveryCheckpoint(ctx context.Context, tx *sql.Tx, checkpoint domain.RecoveryCheckpoint) error {
	scope := checkpoint.Scope
	_, err := tx.ExecContext(ctx, `INSERT INTO recovery_checkpoints(id,scope_kind,task_id,graph_id,node_id,revision,state,owner_instance_id,data) VALUES(?,?,?,?,?,1,?,?,?)`, checkpoint.ID, scope.Kind, recoveryNullable(scope.TaskID), recoveryNullable(scope.GraphID), recoveryNullable(scope.NodeID), checkpoint.State, checkpoint.OwnerInstanceID, encode(checkpoint))
	if err != nil {
		return storageError()
	}
	return nil
}

func (s *Store) requireNoDuplicatePause(ctx context.Context, q queryer, scope domain.RecoveryScope) error {
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM recovery_checkpoints WHERE state IN ('pausing','paused') AND scope_kind=? AND coalesce(task_id,'')=? AND coalesce(graph_id,'')=? AND coalesce(node_id,'')=?`, scope.Kind, scope.TaskID, scope.GraphID, scope.NodeID).Scan(&count)
	if err != nil {
		return storageError()
	}
	if count != 0 {
		return domain.Fail("scope_paused", "This exact scope already has an active checkpoint. Inspect it before starting another pause.")
	}
	return nil
}

func (s *Store) PauseTask(ctx context.Context, ownerID, taskID string, in domain.PauseTaskInput) domain.Result {
	if invalid := domain.ValidatePauseTaskInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !domain.ValidID(taskID) {
		return domain.Failure(in.RequestID, domain.Invalid("task identity"))
	}
	fp := Fingerprint("tasks.pause", struct {
		TaskID string
		Input  domain.PauseTaskInput
	}{taskID, in})
	return s.mutate(ctx, in.RequestID, "tasks.pause", fp, func(tx *sql.Tx) (int, any, error) {
		if err := s.recoveryGateOwner(ctx, tx, ownerID); err != nil {
			return 0, nil, err
		}
		task, err := s.task(ctx, tx, taskID)
		if err != nil {
			return 0, nil, err
		}
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		if gate.OwnerInstanceID == "" {
			attempt, err := s.latest(ctx, tx, task)
			if err != nil {
				return 0, nil, err
			}
			if attempt == nil || attempt.OwnerInstanceID != ownerID {
				return 0, nil, domain.Fail("owner_unavailable", "The selected work is not owned by this Council owner.")
			}
		}
		if task.Revision != in.ExpectedTaskRevision {
			return 0, nil, domain.RevisionConflict(task.Revision)
		}
		if task.CurrentVersion != in.TaskVersion {
			return 0, nil, domain.Fail("task_version_conflict", "Pause the current captured task version.")
		}
		scope := domain.RecoveryScope{Kind: "task", TaskID: taskID, TaskVersion: in.TaskVersion, ExpectedTaskRevision: in.ExpectedTaskRevision}
		membership, err := s.graphMembership(ctx, tx, taskID)
		if err != nil {
			return 0, nil, err
		}
		if membership != nil {
			if membership.Kind != "node" || membership.NodeID == nil {
				return 0, nil, graphScopeError(membership)
			}
			header, err := s.graphHeader(ctx, tx, membership.GraphID)
			if err != nil {
				return 0, nil, err
			}
			nodes, err := s.graphNodes(ctx, tx, membership.GraphID)
			if err != nil {
				return 0, nil, err
			}
			budget, err := s.rawTaskBudget(ctx, tx, header.Summary.RootTaskID)
			if err != nil {
				return 0, nil, err
			}
			if budget == nil {
				return 0, nil, domain.Fail("budget_conflict", "The graph budget is unavailable.")
			}
			found := false
			for _, node := range nodes {
				if node.Summary.ID == *membership.NodeID && node.Summary.TaskID == taskID {
					scope.Kind = "member"
					scope.GraphID = membership.GraphID
					scope.GraphVersion = header.Summary.GraphVersion
					scope.ExpectedGraphControlRevision = header.Summary.ControlRevision
					scope.NodeID = node.Summary.ID
					scope.ExpectedNodeControlRevision = node.Summary.ControlRevision
					scope.ExpectedBudgetAuthorityEpoch = header.Summary.BudgetAuthorityEpoch
					scope.ExpectedWindowNumber = budget.CurrentWindow.Number
					found = true
					break
				}
			}
			if !found {
				return 0, nil, storageError()
			}
		}
		if err := s.requireNoDuplicatePause(ctx, tx, scope); err != nil {
			return 0, nil, err
		}
		checkpoint, err := s.recoveryCheckpointSnapshot(ctx, tx, scope, ownerID)
		if err != nil {
			return 0, nil, err
		}
		if err = s.saveNewRecoveryCheckpoint(ctx, tx, checkpoint); err != nil {
			return 0, nil, err
		}
		task.StateRevision, err = increment(task.StateRevision)
		if err != nil {
			return 0, nil, err
		}
		task.UpdatedAt = domain.Now()
		if err = saveTask(ctx, tx, task); err != nil {
			return 0, nil, err
		}
		if scope.Kind == "member" {
			header, err := s.graphHeader(ctx, tx, scope.GraphID)
			if err != nil {
				return 0, nil, err
			}
			header.Summary.StateRevision, err = increment(header.Summary.StateRevision)
			if err != nil {
				return 0, nil, err
			}
			if err = saveGraphHeader(ctx, tx, header); err != nil {
				return 0, nil, err
			}
		}
		return 202, domain.RecoveryPauseResult{Checkpoint: checkpoint}, nil
	})
}

func (s *Store) PauseGraph(ctx context.Context, ownerID, graphID string, in domain.PauseGraphInput) domain.Result {
	if invalid := domain.ValidatePauseGraphInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !domain.ValidID(graphID) {
		return domain.Failure(in.RequestID, domain.Invalid("graph identity"))
	}
	fp := Fingerprint("graphs.pause", struct {
		GraphID string
		Input   domain.PauseGraphInput
	}{graphID, in})
	return s.mutate(ctx, in.RequestID, "graphs.pause", fp, func(tx *sql.Tx) (int, any, error) {
		if err := s.recoveryGateOwner(ctx, tx, ownerID); err != nil {
			return 0, nil, err
		}
		h, err := s.graphHeader(ctx, tx, graphID)
		if err != nil {
			return 0, nil, err
		}
		gate, err := s.gate(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		if gate.OwnerInstanceID == "" && (h.DispatchAuthority == nil || h.DispatchAuthority.OwnerInstanceID != ownerID) {
			return 0, nil, domain.Fail("owner_unavailable", "The selected graph is not owned by this Council owner.")
		}
		if h.Summary.GraphVersion != in.GraphVersion || h.Summary.ControlRevision != in.ExpectedGraphControlRevision || h.Summary.BudgetAuthorityEpoch != in.ExpectedBudgetAuthorityEpoch {
			return 0, nil, graphControlError()
		}
		budget, err := s.rawTaskBudget(ctx, tx, h.Summary.RootTaskID)
		if err != nil {
			return 0, nil, err
		}
		if budget == nil || budget.CurrentWindow.Number != in.ExpectedWindowNumber {
			return 0, nil, domain.Fail("budget_conflict", "Capture the current graph budget window before pausing.")
		}
		nodes, err := s.graphNodes(ctx, tx, graphID)
		if err != nil {
			return 0, nil, err
		}
		if len(nodes) != len(in.NodeAuthorities) {
			return 0, nil, graphControlError()
		}
		for i, node := range nodes {
			authority := in.NodeAuthorities[i]
			if authority.NodeID != node.Summary.ID || authority.TaskVersion != node.Summary.TaskVersion || authority.NodeControlRevision != node.Summary.ControlRevision {
				return 0, nil, graphControlError()
			}
		}
		scope := domain.RecoveryScope{Kind: "graph", GraphID: graphID, GraphVersion: in.GraphVersion, ExpectedGraphControlRevision: in.ExpectedGraphControlRevision, ExpectedBudgetAuthorityEpoch: in.ExpectedBudgetAuthorityEpoch, ExpectedWindowNumber: in.ExpectedWindowNumber, NodeAuthorities: append([]domain.GraphNodeAuthority(nil), in.NodeAuthorities...)}
		if err := s.requireNoDuplicatePause(ctx, tx, scope); err != nil {
			return 0, nil, err
		}
		checkpoint, err := s.recoveryCheckpointSnapshot(ctx, tx, scope, ownerID)
		if err != nil {
			return 0, nil, err
		}
		if err = s.saveNewRecoveryCheckpoint(ctx, tx, checkpoint); err != nil {
			return 0, nil, err
		}
		h.Summary.StateRevision, err = increment(h.Summary.StateRevision)
		if err != nil {
			return 0, nil, err
		}
		if h.DispatchAuthority != nil {
			h.DispatchAuthority.Generation, err = increment(h.DispatchAuthority.Generation)
			if err != nil {
				return 0, nil, err
			}
		}
		if err = saveGraphHeader(ctx, tx, h); err != nil {
			return 0, nil, err
		}
		return 202, domain.RecoveryPauseResult{Checkpoint: checkpoint}, nil
	})
}

// ActiveRecoveryCheckpoints returns only durable currently effective fences.
// It is an observation and never advances or resumes a checkpoint.
func (s *Store) ActiveRecoveryCheckpoints(ctx context.Context, taskID, graphID string) ([]domain.RecoveryCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if taskID != "" {
		ref, err := s.graphMembership(ctx, s.db, taskID)
		if err != nil {
			return nil, err
		}
		if ref != nil {
			graphID = ref.GraphID
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM recovery_checkpoints WHERE state IN ('pausing','paused') AND ((? != '' AND task_id=?) OR (? != '' AND graph_id=? AND (? = '' OR scope_kind='graph'))) ORDER BY rowid LIMIT 18`, taskID, taskID, graphID, graphID, taskID)
	if err != nil {
		return nil, storageError()
	}
	defer rows.Close()
	out := []domain.RecoveryCheckpoint{}
	activeScopes := map[string]bool{}
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			return nil, storageError()
		}
		cp, err := decode[domain.RecoveryCheckpoint](raw)
		if err != nil {
			return nil, storageError()
		}
		if err = recoveryTrackActiveCheckpoint(activeScopes, cp); err != nil {
			return nil, err
		}
		out = append(out, cp)
		if len(out) > maxRecoveryActiveCandidates {
			return nil, domain.Fail("checkpoint_scope_overflow", "Too many active pause records affect this task or graph; inspect recovery state before continuing.")
		}
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return out, nil
}
