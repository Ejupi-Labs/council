package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
	"time"
)

var noWorkflowContinuation = errors.New("no fresh workflow continuation")

func (s *Store) ContinueWorkflow(ctx context.Context, ownerID, predecessorStepID string, f domain.WorkflowStepAdmissionFacts) (*domain.WorkflowStepAdmission, error) {
	var out *domain.WorkflowStepAdmission
	e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		previous, e := s.workflowStep(ctx, tx, predecessorStepID)
		if e != nil {
			return e
		}
		p := previous.AdvanceProof
		if p == nil || !workflowStepTerminal(previous.Status) || p.PredecessorStepID != previous.ID || p.OwnerInstanceID != ownerID || f.Plan.PredecessorStepID == nil || *f.Plan.PredecessorStepID != previous.ID {
			return noWorkflowContinuation
		}
		w, e := s.workflowByID(ctx, tx, previous.WorkflowID)
		if e != nil {
			return e
		}
		t, e := s.task(ctx, tx, w.TaskID)
		if e != nil {
			return e
		}
		b, e := s.taskBudget(ctx, tx, t.ID)
		if e != nil {
			return e
		}
		if b == nil || w.CurrentStepID == nil || *w.CurrentStepID != previous.ID || w.TaskVersion != t.CurrentVersion || t.Revision != p.TaskRevision || w.Revision != p.WorkflowRevision || b.Revision != p.BudgetRevision || b.CurrentWindow.Number != p.WindowNumber || f.ExpectedTaskRevision != t.Revision || f.ExpectedWorkflowRevision != w.Revision || f.Plan.LogicalKey != p.NextLogicalKey || !reflect.DeepEqual(w.SelectedCandidateID, p.CandidateID) || !reflect.DeepEqual(w.SelectedResponseBinding, p.ResponseBinding) {
			return noWorkflowContinuation
		}
		var used int
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM workflow_steps WHERE consumption_key=?", previous.ID).Scan(&used); e != nil {
			return storageError()
		}
		if used != 0 {
			return noWorkflowContinuation
		}
		admission, e := s.admitWorkflowStage(ctx, tx, ownerID, &t, &w, b, f, domain.NewID(), previous.ID)
		if e != nil {
			return e
		}
		out = &admission
		return nil
	})
	if errors.Is(e, noWorkflowContinuation) {
		return nil, nil
	}
	if block := knownWorkflowAdmissionBlock(e); block != nil {
		if projectionErr := s.projectWorkflowAdmissionBlock(ctx, ownerID, predecessorStepID, f, block); projectionErr != nil {
			return nil, projectionErr
		}
		return nil, e
	}
	return out, e
}

func (s *Store) admitWorkflowStage(ctx context.Context, tx *sql.Tx, ownerID string, t *domain.Task, w *domain.Workflow, b *domain.TaskBudget, f domain.WorkflowStepAdmissionFacts, requestID, consumption string) (domain.WorkflowStepAdmission, error) {
	var out domain.WorkflowStepAdmission
	g, e := s.gate(ctx, tx)
	if e != nil {
		return out, e
	}
	if e = checkGate(g); e != nil {
		return out, e
	}
	if !domain.ValidID(ownerID) || g.OwnerInstanceID != "" && g.OwnerInstanceID != ownerID {
		return out, domain.Fail("owner_unavailable", "The current owner must admit this workflow stage.")
	}
	if w.TaskVersion != t.CurrentVersion || w.Revision != f.ExpectedWorkflowRevision || t.Revision != f.ExpectedTaskRevision || w.State == "cancelled" || w.State == "completed" {
		return out, domain.Fail("workflow_revision_conflict", "Reload the current workflow before admitting a stage.")
	}
	old, e := s.latest(ctx, tx, *t)
	if e != nil {
		return out, e
	}
	if old != nil && (old.Phase != "settled" || old.Cleanup.State != "verified") {
		return out, domain.Fail("attempt_active", "Retain existing owned work until it settles with verified cleanup.")
	}
	if b == nil {
		return out, domain.Fail("routing_adoption_required", "Explicit finite budget is required before a stage.")
	}
	var deadline time.Time
	if b.CurrentWindow.DeadlineAt == nil {
		if f.Plan.Role != "author" || f.Plan.ExecutorKind != "native" {
			return out, domain.Fail("budget_expired", "No open common task window permits this stage.")
		}
	} else {
		var invalid *domain.Error
		deadline, invalid = domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
		if invalid != nil {
			return out, storageError()
		}
		if !time.Now().UTC().Before(deadline) {
			return out, domain.Fail("budget_expired", "The common task window expired before stage admission.")
		}
	}
	p := f.Plan
	var retryPlan *domain.WorkflowStagePlan
	if p.ReplacementOfStepID != nil {
		if w.CurrentStepID == nil || *w.CurrentStepID != *p.ReplacementOfStepID {
			return out, domain.Fail("workflow_revision_conflict", "Only the currently blocked native execution can be retried explicitly.")
		}
		blocked, err := s.workflowStep(ctx, tx, *p.ReplacementOfStepID)
		if err != nil {
			return out, err
		}
		if blocked.AttemptID == nil {
			return out, evidenceUnavailable()
		}
		prior, err := s.attempt(ctx, tx, *blocked.AttemptID)
		if err != nil {
			return out, err
		}
		retryPlan = domain.BlockedNativeRetryPlan(*w, blocked, prior)
		if retryPlan == nil || p.Role != retryPlan.Role || p.ExecutorKind != retryPlan.ExecutorKind || p.Cycle != retryPlan.Cycle || p.LogicalKey != retryPlan.LogicalKey || !reflect.DeepEqual(p.DefinitionID, retryPlan.DefinitionID) || !reflect.DeepEqual(p.InputCandidateID, retryPlan.InputCandidateID) || !reflect.DeepEqual(p.InputResponseBinding, retryPlan.InputResponseBinding) || !reflect.DeepEqual(p.PredecessorStepID, retryPlan.PredecessorStepID) {
			return out, domain.Fail("authority_required", "Only a clean current never-prepared blocked native slot has explicit retry authority.")
		}
	}
	if !reflect.DeepEqual(p.InputCandidateID, w.SelectedCandidateID) || !reflect.DeepEqual(p.InputResponseBinding, w.SelectedResponseBinding) {
		return out, evidenceUnavailable()
	}
	subject := ""
	if p.InputCandidateID != nil {
		subject = *p.InputCandidateID
	} else if p.InputResponseBinding != nil {
		subject = p.InputResponseBinding.ResponseArtifactID
	}
	definition := ""
	if p.DefinitionID != nil {
		definition = *p.DefinitionID
	}
	key, invalid := domain.WorkflowStageKey(p.Role, p.Cycle, definition, subject)
	if invalid != nil {
		return out, invalid
	}
	if p.LogicalKey != key {
		return out, domain.Invalid("canonical workflow logical slot")
	}
	executor := "native"
	switch p.Role {
	case "author":
		if retryPlan == nil && (w.CurrentStepID != nil || w.SelectedCandidateID != nil || w.SelectedResponseBinding != nil) {
			return out, domain.Fail("authority_required", "Use explicit current workflow advancement to retain the selected deliverable.")
		}
	case "review", "correction":
	case "check":
		executor = "local_check"
		if _, e = workflowDefinition(*w, definition); e != nil {
			return out, e
		}
	case "local_delivery":
		executor = "local_application"
		if w.Authority.Delivery.LocalMode != "apply_verified_local" {
			return out, domain.Fail("delivery_scope_required", "No prospective local application authority was captured.")
		}
	case "external_delivery":
		executor = "local_external"
		if w.Authority.Delivery.ExternalAction == nil {
			return out, domain.Fail("delivery_scope_required", "No explicit external command scope was captured.")
		}
	default:
		return out, domain.Invalid("supported next workflow role")
	}
	if p.ExecutorKind != executor {
		return out, domain.Invalid("matching workflow executor kind")
	}
	if p.Role == "correction" && retryPlan == nil && (p.Cycle != w.CorrectionCyclesUsed+1 || p.Cycle > w.Authority.Correction.MaxCycles) {
		return out, domain.Fail("authority_required", "The captured finite correction cycle is unavailable.")
	}
	if p.Role != "correction" && p.Cycle != w.CorrectionCyclesUsed {
		return out, domain.Invalid("current workflow cycle")
	}
	if p.Role != "author" && (p.PredecessorStepID == nil || w.CurrentStepID == nil || *p.PredecessorStepID != *w.CurrentStepID) {
		return out, domain.Fail("workflow_revision_conflict", "Only the current logical slot can admit its next execution.")
	}
	previous := domain.WorkflowStep{Ordinal: "0"}
	if p.PredecessorStepID != nil {
		previous, e = s.workflowStep(ctx, tx, *p.PredecessorStepID)
		if e != nil {
			return out, e
		}
	}
	if p.Role != "author" && !workflowStepTerminal(previous.Status) {
		return out, domain.Fail("attempt_active", "The predecessor step has not settled.")
	}
	v, e := s.version(ctx, tx, t.ID, t.CurrentVersion)
	if e != nil {
		return out, e
	}
	a := newAttempt(*t, v, requestID, ownerID, w.Authority.ExecutionMode, w.Authority.FakeScenario)
	a.WorkflowID = w.ID
	a.StepID = domain.NewID()
	a.Role = p.Role
	a.ExecutorKind = executor
	a.InputCandidateID = p.InputCandidateID
	a.Trigger = "workflow_stage"
	var decision *domain.RoutingDecision
	if executor == "native" {
		if invalid := domain.CheckAuthoringCapability(v); invalid != nil {
			return out, invalid
		}
		authority, e := s.routingAuthority(ctx, tx, t.ID, t.CurrentVersion)
		if e != nil {
			return out, e
		}
		if authority == nil {
			return out, domain.Fail("routing_adoption_required", "No captured routing authority permits this model stage.")
		}
		d, e := s.admitRouting(ctx, tx, *t, v, &a, *authority, b, f.Routing, "manual")
		if e != nil {
			return out, e
		}
		if invalid := checkResolvedScenario(a, a.ExecutionMode, a.FakeScenario); invalid != nil {
			return out, invalid
		}
		decision = &d
	} else {
		switch executor {
		case "local_check":
			a.ConnectionID = "local-check"
		case "local_application":
			a.ConnectionID = "local-application"
		case "local_external":
			a.ConnectionID = "local-external"
		}
		a.ResolvedTarget = nil
		a.RoutingDecisionID = nil
		commandTimeout := 0
		if p.Role == "check" {
			definition, err := workflowDefinition(*w, definition)
			if err != nil {
				return out, err
			}
			if definition.Command != nil {
				commandTimeout = definition.Command.TimeoutSeconds
			}
		}
		if p.Role == "external_delivery" && w.Authority.Delivery.ExternalAction != nil {
			commandTimeout = 300
			if w.Authority.Delivery.ExternalAction.Command.TimeoutSeconds != nil {
				commandTimeout = *w.Authority.Delivery.ExternalAction.Command.TimeoutSeconds
			}
		}
		if commandTimeout > 0 {
			admitted, _ := domain.ParseRoutingTime(a.AdmittedAt)
			commandDeadline := admitted.Add(time.Duration(commandTimeout) * time.Second)
			if commandDeadline.Before(timeMust(a.DeadlineAt)) {
				a.DeadlineAt = commandDeadline.Format("2006-01-02T15:04:05.000Z")
			}
		}
		if deadline.Before(timeMust(a.DeadlineAt)) {
			a.DeadlineAt = *b.CurrentWindow.DeadlineAt
		}
	}
	ordinal := "1"
	if p.PredecessorStepID != nil {
		ordinal, e = increment(previous.Ordinal)
		if e != nil {
			return out, e
		}
	}
	execution := "1"
	var lastExecution sql.NullInt64
	if e = tx.QueryRowContext(ctx, "SELECT max(execution_ordinal) FROM workflow_steps WHERE workflow_id=? AND logical_key=?", w.ID, p.LogicalKey).Scan(&lastExecution); e != nil {
		return out, storageError()
	}
	if lastExecution.Valid && executor == "native" && retryPlan == nil {
		return out, domain.Fail("authority_required", "A native logical slot can be retried only through its clean current explicit replacement.")
	}
	if lastExecution.Valid {
		execution, e = increment(domain.Decimal(lastExecution.Int64))
		if e != nil {
			return out, e
		}
	}
	if p.ReplacementOfStepID != nil {
		replaced, err := s.workflowStep(ctx, tx, *p.ReplacementOfStepID)
		if err != nil {
			return out, err
		}
		if replaced.WorkflowID != w.ID || replaced.LogicalKey != p.LogicalKey || replaced.Role != p.Role || replaced.Cycle != p.Cycle || !workflowStepTerminal(replaced.Status) {
			return out, domain.Invalid("same-slot replacement")
		}
	}
	step := domain.WorkflowStep{ID: a.StepID, WorkflowID: w.ID, Ordinal: ordinal, ExecutionOrdinal: execution, ReplacementOfStepID: p.ReplacementOfStepID, Role: p.Role, ExecutorKind: executor, Cycle: p.Cycle, LogicalKey: p.LogicalKey, ReasonCodes: p.ReasonCodes, DefinitionID: p.DefinitionID, InputCandidateID: p.InputCandidateID, InputResponseBinding: p.InputResponseBinding, PredecessorStepID: p.PredecessorStepID, AttemptID: ptr(a.ID), Trigger: "workflow_stage", Status: "ready", CreatedAt: a.AdmittedAt}
	if e = s.insertAttempt(ctx, tx, a); e != nil {
		return out, e
	}
	if e = insertWorkflowStepTx(ctx, tx, step); e != nil {
		return out, e
	}
	if consumption != "" {
		if _, e = tx.ExecContext(ctx, "UPDATE workflow_steps SET consumption_key=? WHERE id=?", consumption, step.ID); e != nil {
			return out, storageError()
		}
	}
	if decision != nil {
		if e = storeRoutingDecision(ctx, tx, *decision); e != nil {
			return out, e
		}
	}
	t.LatestAttemptID = ptr(a.ID)
	t.Revision, e = increment(t.Revision)
	if e != nil {
		return out, e
	}
	t.StateRevision, e = increment(t.StateRevision)
	if e != nil {
		return out, e
	}
	t.UpdatedAt = a.AdmittedAt
	if e = saveTask(ctx, tx, *t); e != nil {
		return out, e
	}
	w.CurrentStepID = ptr(step.ID)
	w.Revision, e = increment(w.Revision)
	if e != nil {
		return out, e
	}
	w.StateRevision, e = increment(w.StateRevision)
	if e != nil {
		return out, e
	}
	w.State = "queued"
	w.NextAction = "observe_attempt"
	w.NextOwner = "council"
	w.UpdatedAt = a.AdmittedAt
	if p.Role == "correction" {
		w.CorrectionCyclesUsed = p.Cycle
	}
	if e = saveWorkflow(ctx, tx, *w); e != nil {
		return out, e
	}
	return domain.WorkflowStepAdmission{Workflow: *w, Step: step, Attempt: a, Budget: *b, RoutingDecision: decision}, nil
}
func timeMust(value string) time.Time { t, _ := domain.ParseRoutingTime(value); return t }
