package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphStageCapacityError(e error) bool {
	de := knownGraphAdmissionBlock(e)
	return de != nil && (de.Code == "capacity_wait" || de.Code == "attempt_active")
}

func (s *Store) graphStageObservation(ctx context.Context, tx *sql.Tx, scope graphStageScope, w domain.Workflow, f domain.WorkflowStepAdmissionFacts) (graphAdmissionObservation, error) {
	var last sql.NullInt64
	if e := tx.QueryRowContext(ctx, "SELECT max(execution_ordinal) FROM workflow_steps WHERE workflow_id=? AND logical_key=?", w.ID, f.Plan.LogicalKey).Scan(&last); e != nil {
		return graphAdmissionObservation{}, storageError()
	}
	execution := "1"
	if last.Valid {
		var e error
		execution, e = increment(domain.Decimal(last.Int64))
		if e != nil {
			return graphAdmissionObservation{}, e
		}
	}
	trigger := scope.Facts.Trigger
	sourceClass := "settled"
	if trigger.Kind == "explicit" {
		sourceClass = "explicit"
	}
	// Generation changes for other nodes cannot refresh the same admitted slot's
	// already consumed source event. The node-admission/workflow/predecessor do.
	key := Fingerprint("graphs.workflow.stage.observation", struct{ Graph, Admission, Workflow, Predecessor, Input, Slot, Execution, SourceClass, SourceID string }{scope.Header.Summary.ID, scope.Facts.NodeAdmissionID, w.ID, scope.Facts.Control.OperationAttemptID, scope.Facts.InputBinding.ID, f.Plan.LogicalKey, execution, sourceClass, trigger.ID})
	return graphAdmissionObservation{ID: domain.NewID(), GraphID: scope.Header.Summary.ID, NodeID: scope.Node.Summary.ID, TaskVersion: w.TaskVersion, InputID: scope.Facts.InputBinding.ID, LogicalSlot: f.Plan.LogicalKey, ExecutionOrdinal: execution, Generation: scope.Facts.Control.Generation, Trigger: trigger, ConsumptionKey: key, ObservedAt: domain.Now()}, nil
}
func appendGraphStageObservation(ctx context.Context, tx *sql.Tx, o graphAdmissionObservation) error {
	seq, e := nextSequence(ctx, tx, "graph_admission_observations")
	if e != nil {
		return e
	}
	o.Sequence = domain.Decimal(seq)
	_, e = tx.ExecContext(ctx, "INSERT INTO graph_admission_observations VALUES(?,?,?,?,?,?,?,?,?,?)", seq, o.ID, o.GraphID, o.NodeID, intRevision(o.TaskVersion), o.InputID, o.ConsumptionKey, o.DispatchID, o.Outcome, encode(o))
	if e != nil {
		return storageError()
	}
	return nil
}

func (s *Store) admitWorkflowContinuation(ctx context.Context, tx *sql.Tx, ownerID string, t *domain.Task, w *domain.Workflow, b *domain.TaskBudget, f domain.WorkflowStepAdmissionFacts, requestID, consumption string) (*domain.WorkflowStepAdmission, error) {
	if f.Graph == nil {
		a, e := s.admitWorkflowStage(ctx, tx, ownerID, t, w, b, f, requestID, consumption)
		if e != nil {
			return nil, e
		}
		return &a, nil
	}
	scope, e := s.graphWorkflowStageScope(ctx, tx, ownerID, *t, *w, b, f)
	if e != nil {
		return nil, e
	}
	if scope == nil {
		return nil, evidenceUnavailable()
	}
	observation, e := s.graphStageObservation(ctx, tx, *scope, *w, f)
	if e != nil {
		return nil, e
	}
	used, e := graphAdmissionObserved(ctx, tx, observation.ConsumptionKey)
	if e != nil {
		return nil, e
	}
	if used {
		return nil, noWorkflowContinuation
	}
	if scope.Facts.Trigger.Kind == "claim_release" {
		var source, predecessor int64
		query := "SELECT sequence FROM attempt_events WHERE attempt_id=? AND json_extract(data,'$.kind')='settled' ORDER BY sequence DESC LIMIT 1"
		if e = tx.QueryRowContext(ctx, query, scope.Facts.Trigger.ID).Scan(&source); e != nil {
			return nil, storageError()
		}
		if e = tx.QueryRowContext(ctx, query, scope.Facts.Control.OperationAttemptID).Scan(&predecessor); e != nil {
			return nil, storageError()
		}
		if source <= predecessor {
			return nil, noWorkflowContinuation
		}
	}
	if _, e = tx.ExecContext(ctx, "SAVEPOINT graph_stage_admission"); e != nil {
		return nil, storageError()
	}
	admission, e := s.admitWorkflowStage(ctx, tx, ownerID, t, w, b, f, requestID, consumption)
	if e != nil {
		if !graphStageCapacityError(e) {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, "ROLLBACK TO graph_stage_admission"); e != nil {
			return nil, storageError()
		}
		if _, e = tx.ExecContext(ctx, "RELEASE graph_stage_admission"); e != nil {
			return nil, storageError()
		}
		observation.Outcome = "capacity_wait"
		observation.Failure = domain.Fail("capacity_wait", "Wait for a fresh actual managed-capacity release before this graph workflow stage.")
		if e = appendGraphStageObservation(ctx, tx, observation); e != nil {
			return nil, e
		}
		if e = s.projectGraphStageWait(ctx, tx, scope.Header.Summary.ID, w.ID, scope.Node.Summary.ID); e != nil {
			return nil, e
		}
		return nil, nil
	}
	if _, e = tx.ExecContext(ctx, "RELEASE graph_stage_admission"); e != nil {
		return nil, storageError()
	}
	registered, e := s.graphDispatchForAttempt(ctx, tx, admission.Attempt.ID)
	if e != nil {
		return nil, e
	}
	observation.Outcome = "admitted"
	observation.DispatchID = &registered.Public.Dispatch.ID
	if e = appendGraphStageObservation(ctx, tx, observation); e != nil {
		return nil, e
	}
	return &admission, nil
}
func (s *Store) projectGraphStageWait(ctx context.Context, tx *sql.Tx, graphID, workflowID, nodeID string) error {
	// The failed admission may have changed detached Go values before rollback.
	// Reload every mutable projection from the acknowledged transaction state.
	w, e := s.workflowByID(ctx, tx, workflowID)
	if e != nil {
		return e
	}
	w.State = "blocked"
	w.NextAction = "capacity_wait"
	w.NextOwner = "council"
	w.UpdatedAt = domain.Now()
	w.StateRevision, e = increment(w.StateRevision)
	if e != nil {
		return e
	}
	if e = saveWorkflow(ctx, tx, w); e != nil {
		return e
	}
	task, e := s.task(ctx, tx, w.TaskID)
	if e != nil {
		return e
	}
	task.StateRevision, e = increment(task.StateRevision)
	if e != nil {
		return e
	}
	if e = saveTask(ctx, tx, task); e != nil {
		return e
	}
	h, e := s.graphHeader(ctx, tx, graphID)
	if e != nil {
		return e
	}
	nodes, e := s.graphNodes(ctx, tx, graphID)
	if e != nil {
		return e
	}
	for i := range nodes {
		n := &nodes[i]
		if n.Summary.ID != nodeID {
			continue
		}
		n.Summary.State = "ready"
		n.Summary.NextAction = "capacity_wait"
		n.Summary.NextOwner = "council"
		n.Blocker = &domain.GraphBlockerSummary{Code: "capacity_wait", Message: "The next exact workflow stage is waiting for managed capacity.", NodeID: &n.Summary.ID, InputID: n.Summary.InputID}
		n.Summary.StateRevision, e = increment(n.Summary.StateRevision)
		if e != nil {
			return e
		}
		if e = saveGraphNode(ctx, tx, *n); e != nil {
			return e
		}
	}
	if h.DispatchAuthority != nil {
		h.DispatchAuthority.State = "capacity_wait"
	}
	h.Summary.State = "queued"
	h.Summary.NextOwner = "council"
	h.Summary.NextAction = "Wait for managed capacity to continue the exact current workflow stage."
	h.Summary.UpdatedAt = domain.Now()
	h.Summary.Counts, e = graphCounts(nodes)
	if e != nil {
		return e
	}
	h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
	if e != nil {
		return e
	}
	return saveGraphHeader(ctx, tx, h)
}
