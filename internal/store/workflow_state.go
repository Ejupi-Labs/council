package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
)

func (s *Store) workflowByID(ctx context.Context, q queryer, id string) (domain.Workflow, error) {
	var raw, authority, taskID, state string
	var version, revision, stateRevision int64
	e := q.QueryRowContext(ctx, "SELECT task_id,task_version,revision,state_revision,state,authority_data,data FROM workflows WHERE id=?", id).Scan(&taskID, &version, &revision, &stateRevision, &state, &authority, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Workflow{}, notFound()
	}
	if e != nil {
		return domain.Workflow{}, storageError()
	}
	w, e := decode[domain.Workflow](raw)
	if e != nil {
		return w, storageError()
	}
	a, e := decode[domain.WorkflowAuthority](authority)
	if e != nil || !domain.ValidID(w.ID) || w.ID != id || w.TaskID != taskID || w.TaskVersion != domain.Decimal(version) || !validStoredRevision(revision, w.Revision) || !validStoredRevision(stateRevision, w.StateRevision) || w.State != state || a.TaskID != taskID || a.TaskVersion != w.TaskVersion || !reflect.DeepEqual(a, w.Authority) {
		return w, storageError()
	}
	if a.ExecutionMode != "native" && a.ExecutionMode != "fake" {
		return w, storageError()
	}
	if w.SelectedCandidateID != nil && w.SelectedResponseBinding != nil {
		return w, storageError()
	}
	return w, nil
}
func saveWorkflow(ctx context.Context, tx *sql.Tx, w domain.Workflow) error {
	if !domain.ValidID(w.ID) {
		return storageError()
	}
	revision, e := domain.ParseRevision(w.Revision)
	if e != nil {
		return storageError()
	}
	state, e := domain.ParseRevision(w.StateRevision)
	if e != nil {
		return storageError()
	}
	result, err := tx.ExecContext(ctx, "UPDATE workflows SET revision=?,state_revision=?,state=?,data=? WHERE id=?", revision, state, w.State, encode(w), w.ID)
	if err != nil {
		return storageError()
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return storageError()
	}
	return nil
}
func (s *Store) selectedWorkflow(ctx context.Context, q queryer, taskID string, selector domain.WorkflowSelector) (domain.WorkflowDetail, error) {
	out := domain.WorkflowDetail{TaskID: taskID}
	if !domain.ValidID(taskID) {
		return out, domain.Invalid("taskId")
	}
	if e := domain.ValidateWorkflowSelector(selector); e != nil {
		return out, e
	}
	t, e := s.task(ctx, q, taskID)
	if e != nil {
		return out, e
	}
	var id string
	if selector.WorkflowID != "" {
		w, e := s.workflowByID(ctx, q, selector.WorkflowID)
		if e != nil {
			return out, e
		}
		if w.TaskID != taskID {
			return out, notFound()
		}
		out.TaskVersion = w.TaskVersion
		out.Workflow = &w
	} else {
		version := selector.TaskVersion
		if version == "" {
			version = t.CurrentVersion
		}
		out.TaskVersion = version
		if _, e = s.version(ctx, q, taskID, version); e != nil {
			return out, e
		}
		e = q.QueryRowContext(ctx, "SELECT id FROM workflows WHERE task_id=? AND task_version=?", taskID, intRevision(version)).Scan(&id)
		if errors.Is(e, sql.ErrNoRows) {
			return out, nil
		}
		if e != nil {
			return out, storageError()
		}
		w, e := s.workflowByID(ctx, q, id)
		if e != nil {
			return out, e
		}
		out.Workflow = &w
	}
	if out.Workflow.LatestAcceptanceID != nil {
		var raw string
		e = q.QueryRowContext(ctx, "SELECT data FROM workflow_acceptances WHERE id=? AND workflow_id=?", *out.Workflow.LatestAcceptanceID, out.Workflow.ID).Scan(&raw)
		if e != nil {
			return out, storageError()
		}
		a, e := decode[domain.WorkflowAcceptance](raw)
		if e != nil || a.ID != *out.Workflow.LatestAcceptanceID || a.WorkflowID != out.Workflow.ID || a.TaskID != taskID || a.TaskVersion != out.TaskVersion {
			return out, storageError()
		}
		out.Acceptance = &a
	}
	return out, nil
}
func (s *Store) Workflow(ctx context.Context, taskID string, selector domain.WorkflowSelector) (domain.WorkflowDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selectedWorkflow(ctx, s.db, taskID, selector)
}
func optionalStringMatches(p *string, v sql.NullString) bool {
	return p == nil && !v.Valid || p != nil && v.Valid && *p == v.String
}
func (s *Store) workflowStep(ctx context.Context, q queryer, id string) (domain.WorkflowStep, error) {
	var raw, workflowID, logical, role, executor, status string
	var ordinal, executionOrdinal int64
	var cycle int
	var attempt, candidate, response, predecessor, replacement sql.NullString
	e := q.QueryRowContext(ctx, "SELECT workflow_id,ordinal,execution_ordinal,logical_key,role,executor_kind,cycle,status,attempt_id,input_candidate_id,input_response_artifact_id,predecessor_step_id,replacement_of_step_id,data FROM workflow_steps WHERE id=?", id).Scan(&workflowID, &ordinal, &executionOrdinal, &logical, &role, &executor, &cycle, &status, &attempt, &candidate, &response, &predecessor, &replacement, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.WorkflowStep{}, notFound()
	}
	if e != nil {
		return domain.WorkflowStep{}, storageError()
	}
	step, e := decode[domain.WorkflowStep](raw)
	if e != nil || step.ID != id || step.WorkflowID != workflowID || step.Ordinal != domain.Decimal(ordinal) || step.ExecutionOrdinal != domain.Decimal(executionOrdinal) || step.LogicalKey != logical || step.Role != role || step.ExecutorKind != executor || step.Cycle != cycle || step.Status != status || !optionalStringMatches(step.AttemptID, attempt) || !optionalStringMatches(step.InputCandidateID, candidate) || !optionalStringMatches(step.PredecessorStepID, predecessor) || !optionalStringMatches(step.ReplacementOfStepID, replacement) {
		return step, storageError()
	}
	if step.InputResponseBinding == nil && response.Valid || step.InputResponseBinding != nil && (!response.Valid || step.InputResponseBinding.ResponseArtifactID != response.String) {
		return step, storageError()
	}
	return step, nil
}
func (s *Store) WorkflowStep(ctx context.Context, id string) (domain.WorkflowStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !domain.ValidID(id) {
		return domain.WorkflowStep{}, domain.Invalid("stepId")
	}
	return s.workflowStep(ctx, s.db, id)
}
func storedWorkflowRecord[T any](ctx context.Context, q queryer, table, id string) (T, error) {
	var zero T
	var raw string
	if !domain.ValidID(id) {
		return zero, domain.Invalid("record identity")
	}
	e := q.QueryRowContext(ctx, "SELECT data FROM "+table+" WHERE id=?", id).Scan(&raw)
	if errors.Is(e, sql.ErrNoRows) {
		return zero, notFound()
	}
	if e != nil {
		return zero, storageError()
	}
	out, e := decode[T](raw)
	if e != nil {
		return zero, storageError()
	}
	return out, nil
}
func (s *Store) WorkflowReview(ctx context.Context, id string) (domain.ReviewRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := storedWorkflowRecord[domain.ReviewRecord](ctx, s.db, "workflow_reviews", id)
	if e == nil && r.ID != id {
		return r, storageError()
	}
	return r, e
}
func (s *Store) WorkflowCheck(ctx context.Context, id string) (domain.CheckReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := storedWorkflowRecord[domain.CheckReceipt](ctx, s.db, "workflow_checks", id)
	if e == nil && r.ID != id {
		return r, storageError()
	}
	return r, e
}
func (s *Store) DeliveryOperation(ctx context.Context, id string) (domain.DeliveryOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := storedWorkflowRecord[domain.DeliveryOperation](ctx, s.db, "delivery_operations", id)
	if e == nil && r.ID != id {
		return r, storageError()
	}
	return r, e
}
