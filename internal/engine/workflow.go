package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

func (e *Engine) MutateWorkflow(ctx context.Context, op, id string, body []byte) domain.Result {
	requestID := domain.MutationIdentity(body)
	var input any
	switch op {
	case "workflow.preferences.update":
		input = &domain.DeliveryPreferenceInput{}
	case "tasks.workflow.adopt":
		input = &domain.WorkflowAdoptInput{}
	case "tasks.workflow.advance":
		input = &domain.WorkflowAdvanceInput{}
	case "tasks.workflow.check":
		input = &domain.WorkflowCheckInput{}
	default:
		return e.failure(requestID, domain.Fail("not_found", "This workflow mutation does not exist."))
	}
	if decodeErr := domain.StrictDecode(body, input); decodeErr != nil {
		return e.failure(requestID, decodeErr)
	}
	if op == "workflow.preferences.update" {
		if id != "" {
			return e.failure(requestID, domain.Invalid("no resource ID on delivery preference"))
		}
	} else if !domain.ValidID(id) {
		return e.failure(requestID, domain.Invalid("workflow task identity"))
	}
	var validationErr *domain.Error
	var fingerprint string
	switch in := input.(type) {
	case *domain.DeliveryPreferenceInput:
		validationErr = domain.ValidateDeliveryPreferenceInput(*in)
		fingerprint = store.DeliveryPreferenceFingerprint(*in)
	case *domain.WorkflowAdoptInput:
		validationErr = domain.ValidateWorkflowAdoptInput(*in)
		fingerprint = store.WorkflowAdoptFingerprint(id, *in)
	case *domain.WorkflowAdvanceInput:
		validationErr = domain.ValidateWorkflowAdvanceInput(*in)
		fingerprint = store.WorkflowAdvanceFingerprint(id, *in)
	case *domain.WorkflowCheckInput:
		validationErr = domain.ValidateWorkflowCheckInput(*in)
		fingerprint = store.WorkflowCheckFingerprint(id, *in)
	}
	if validationErr != nil {
		return e.failure(requestID, validationErr)
	}
	if previous, replayErr := e.db.Replay(ctx, requestID, op, fingerprint); replayErr != nil {
		return e.failure(requestID, replayErr)
	} else if previous != nil {
		return *previous
	}
	if in, ok := input.(*domain.DeliveryPreferenceInput); ok {
		e.prepareGate.Lock()
		defer e.prepareGate.Unlock()
		e.control.Lock()
		result := e.db.UpdateDeliveryPreference(ctx, *in)
		e.guardMutationLocked(result)
		e.control.Unlock()
		return result
	}
	switch in := input.(type) {
	case *domain.WorkflowAdoptInput:
		detail, readErr := e.db.Task(ctx, id)
		if readErr != nil {
			return e.failure(requestID, readErr)
		}
		if detail.LatestAttempt == nil {
			return e.failure(requestID, domain.Fail("authority_required", "Adopt a settled task with retained standalone execution authority."))
		}
		routingView, readErr := e.db.TaskRouting(ctx, id)
		if readErr != nil {
			return e.failure(requestID, readErr)
		}
		if routingView.Authority == nil {
			return e.failure(requestID, domain.Fail("routing_adoption_required", "Adopt finite routing and budget authority before workflow authority."))
		}
		prior := detail.LatestAttempt
		routingFacts, readErr := e.routingFactsForVersion(ctx, detail.Task.ProjectID, detail.Task.OriginalText, detail.Version, routingView.Authority.OptionSnapshot.Suitability, prior.ExecutionMode)
		if readErr != nil {
			return e.failure(requestID, readErr)
		}
		facts, readErr := e.captureWorkflow(ctx, detail.Task.ProjectID, detail.Task.OriginalText, detail.Version, in.Workflow, prior.ExecutionMode, prior.FakeScenario, routingFacts)
		if readErr != nil {
			return e.failure(requestID, readErr)
		}
		e.prepareGate.Lock()
		e.control.Lock()
		result := e.db.AdoptWorkflow(ctx, e.config.OwnerInstanceID, id, *in, facts)
		e.guardMutationLocked(result)
		e.control.Unlock()
		e.prepareGate.Unlock()
		return result
	case *domain.WorkflowAdvanceInput:
		return e.advanceCurrentWorkflow(ctx, id, *in, nil)
	case *domain.WorkflowCheckInput:
		advance := domain.WorkflowAdvanceInput{RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision, WorkflowRevision: in.WorkflowRevision, TaskVersion: in.TaskVersion}
		return e.advanceCurrentWorkflow(ctx, id, advance, in)
	}
	return e.failure(requestID, domain.Fail("not_found", "This workflow mutation does not exist."))
}

func (e *Engine) advanceCurrentWorkflow(ctx context.Context, taskID string, in domain.WorkflowAdvanceInput, check *domain.WorkflowCheckInput) domain.Result {
	view, err := e.db.Workflow(ctx, taskID, domain.WorkflowSelector{})
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if view.Workflow == nil {
		return e.failure(in.RequestID, domain.Fail("authority_required", "Adopt explicit Auto workflow authority before advancing this standalone task."))
	}
	w := *view.Workflow
	if w.TaskVersion != in.TaskVersion {
		return e.failure(in.RequestID, domain.Fail("task_version_conflict", "Advance only the current immutable task version."))
	}
	if w.Revision != in.WorkflowRevision {
		return e.failure(in.RequestID, domain.Fail("workflow_revision_conflict", "Reload the current workflow before advancing."))
	}
	if check == nil {
		if result, handled := e.retryBlockedWorkflow(ctx, taskID, w, in); handled {
			return result
		}
	}
	progress, err := e.inspectWorkflowProgress(ctx, taskID, w)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if progress.detail.Task.Revision != in.ExpectedRevision {
		return e.failure(in.RequestID, domain.RevisionConflict(progress.detail.Task.Revision))
	}
	if check != nil {
		found := false
		for _, definition := range w.Authority.Checks {
			found = found || definition.ID == check.CheckID
		}
		if !found {
			return e.failure(in.RequestID, domain.Fail("check_unavailable", "Select a check declared in this exact workflow authority."))
		}
		progress.decision.Role, progress.decision.ExecutorKind, progress.decision.Action, progress.decision.Owner, progress.decision.CheckID = "check", "local_check", "check", "council", check.CheckID
		progress.decision.ReasonCodes = []string{"explicit_current_check_request"}
	} else if progress.decision.Action == "accept" {
		return e.acceptCurrentWorkflow(ctx, progress, in.RequestID, "")
	}
	if progress.decision.Role == "" || w.CurrentStepID == nil {
		return e.failure(in.RequestID, domain.Fail("authority_required", "The current workflow is waiting for "+progress.decision.Action+". Inspect its retained evidence and next action."))
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, *w.CurrentStepID)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	return e.admitExplicitWorkflow(ctx, taskID, in, check, facts)
}

func (e *Engine) admitExplicitWorkflow(ctx context.Context, taskID string, in domain.WorkflowAdvanceInput, check *domain.WorkflowCheckInput, facts domain.WorkflowStepAdmissionFacts) domain.Result {
	e.prepareGate.Lock()
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		e.prepareGate.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Resolve ownership or persistence uncertainty before admitting work."))
	}
	var result domain.Result
	if check != nil {
		result = e.db.RecheckWorkflow(ctx, e.config.OwnerInstanceID, taskID, *check, facts)
	} else {
		result = e.db.AdvanceWorkflow(ctx, e.config.OwnerInstanceID, taskID, in, facts)
	}
	e.guardMutationLocked(result)
	e.control.Unlock()
	e.prepareGate.Unlock()
	e.signal()
	return result
}
