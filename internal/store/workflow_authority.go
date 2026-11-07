package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) UpdateGuidanceWithWorkflow(ctx context.Context, taskID string, in domain.GuidanceInput, f domain.WorkflowAdmissionFacts) domain.Result {
	return s.updateGuidance(ctx, taskID, in, f.Routing, f.Capture)
}

func (s *Store) captureUnstartedWorkflow(ctx context.Context, tx *sql.Tx, t domain.Task, v domain.TaskVersion, requestID string, options domain.WorkflowInput, capture *domain.WorkflowCapture, routingID, mode, scenario string) (domain.Workflow, error) {
	g, e := s.gate(ctx, tx)
	if e != nil {
		return domain.Workflow{}, e
	}
	owner := g.OwnerInstanceID
	if !domain.ValidID(owner) {
		old, e := s.latest(ctx, tx, t)
		if e != nil {
			return domain.Workflow{}, e
		}
		if old == nil {
			return domain.Workflow{}, workflowUnavailable()
		}
		owner = old.OwnerInstanceID
	}
	if mode != "fake" && mode != "native" {
		return domain.Workflow{}, domain.Invalid("retained workflow execution mode")
	}
	a := newAttempt(t, v, requestID, owner, mode, scenario)
	in := domain.StartTaskInput{RequestID: requestID, ProjectID: t.ProjectID, OriginalText: t.OriginalText, Workflow: &options}
	w, _, e := s.bindInitialWorkflow(ctx, tx, t, v, &a, routingID, in, capture)
	if e != nil {
		return w, e
	}
	w.CurrentStepID = nil
	w.State = "blocked"
	w.NextAction = "start_author"
	w.NextOwner = "user"
	return w, nil
}

func (s *Store) AdoptWorkflow(ctx context.Context, ownerID, taskID string, in domain.WorkflowAdoptInput, f domain.WorkflowAdmissionFacts) domain.Result {
	if invalid := domain.ValidateWorkflowAdoptInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	return s.mutate(ctx, in.RequestID, "tasks.workflow.adopt", WorkflowAdoptFingerprint(taskID, in), func(tx *sql.Tx) (int, any, error) {
		if e := s.requireStandaloneAuthority(ctx, tx, taskID, false); e != nil {
			return 0, nil, e
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		if !domain.ValidID(ownerID) || g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The current owner must capture this prospective workflow authority.")
		}
		t, e := s.task(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		if t.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		if t.CurrentVersion != in.TaskVersion {
			return 0, nil, domain.Fail("task_version_conflict", "Adopt only the current immutable version.")
		}
		selected, e := s.selectedWorkflow(ctx, tx, taskID, domain.WorkflowSelector{})
		if e != nil {
			return 0, nil, e
		}
		if selected.Workflow != nil {
			return 0, nil, domain.Fail("workflow_revision_conflict", "This version already has captured workflow authority.")
		}
		old, e := s.latest(ctx, tx, t)
		if e != nil {
			return 0, nil, e
		}
		if old == nil || old.Phase != "settled" || old.Cleanup.State != "verified" {
			return 0, nil, domain.Fail("attempt_active", "Settle prior owned work before prospective adoption.")
		}
		b, e := s.taskBudget(ctx, tx, taskID)
		if e != nil {
			return 0, nil, e
		}
		prior, e := s.routingAuthority(ctx, tx, taskID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		if b == nil || prior == nil {
			return 0, nil, domain.Fail("routing_adoption_required", "Explicitly adopt finite routing budget before workflow authority; no migration grants additional units.")
		}
		v, e := s.version(ctx, tx, taskID, t.CurrentVersion)
		if e != nil {
			return 0, nil, e
		}
		v.Version, e = increment(v.Version)
		if e != nil {
			return 0, nil, e
		}
		v.CreatedAt = domain.Now()
		t.CurrentVersion = v.Version
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return 0, nil, e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		t.UpdatedAt = v.CreatedAt
		if _, e = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,?,?)", taskID, intRevision(v.Version), encode(v)); e != nil {
			return 0, nil, storageError()
		}
		authority, e := s.captureRouting(ctx, tx, t, v, nil, f.Routing, prior, "user")
		if e != nil {
			return 0, nil, e
		}
		mode, scenario := old.ExecutionMode, old.FakeScenario
		if f.Capture != nil && f.Capture.Authority.ExecutionMode != "" {
			mode = f.Capture.Authority.ExecutionMode
			scenario = f.Capture.Authority.FakeScenario
		}
		if mode != old.ExecutionMode || scenario != old.FakeScenario {
			return 0, nil, domain.Invalid("retained standalone execution mode for adoption")
		}
		w, e := s.captureUnstartedWorkflow(ctx, tx, t, v, in.RequestID, *in.Workflow, f.Capture, authority.ID, mode, scenario)
		if e != nil {
			return 0, nil, e
		}
		if e = insertWorkflowTx(ctx, tx, w); e != nil {
			return 0, nil, e
		}
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		return 200, domain.WorkflowDetail{TaskID: taskID, TaskVersion: v.Version, Workflow: &w}, nil
	})
}
