package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"reflect"
)

// Only current reads use this overlay. Mutation receipts and historical row JSON
// retain the exact summary captured when their original request committed.
func (s *Store) currentTaskSummary(ctx context.Context, q queryer, t domain.Task, v domain.TaskVersion, a *domain.Attempt, accepted bool) (domain.TaskSummary, error) {
	out := taskSummary(t, v, a, accepted)
	view, e := s.selectedWorkflow(ctx, q, t.ID, domain.WorkflowSelector{TaskVersion: t.CurrentVersion})
	if e != nil {
		return out, e
	}
	if view.Workflow == nil {
		return out, nil
	}
	w := *view.Workflow
	if a != nil && a.Cleanup.State == "unknown" {
		return out, nil
	}
	if a != nil && a.TaskVersion == t.CurrentVersion && a.Stop != nil && a.Phase != "settled" {
		out.Status = "cancelling"
		out.NextAction = "Wait for the accepted Stop and retained settlement. Next owner: council."
		return out, nil
	}
	if w.State == "completed" {
		out.Status = "completed"
		out.Stage = "settled"
		switch {
		case view.Acceptance != nil:
			record := view.Acceptance
			if w.SelectedCandidateID == nil || record.CandidateBinding.CandidateID != *w.SelectedCandidateID {
				return out, storageError()
			}
			switch record.DeliveryResult {
			case "review_only":
				out.NextAction = "Inspect the exact verified candidate and retained review-only acceptance. Next owner: user."
			case "applied":
				out.NextAction = "Inspect the verified local application and retained destination evidence. Next owner: user."
			default:
				return out, storageError()
			}
		case w.Completion != nil:
			completion := w.Completion
			if w.SelectedResponseBinding == nil || !reflect.DeepEqual(*w.SelectedResponseBinding, completion.ResponseBinding) || completion.Kind != "response_only_user_review" && completion.Kind != "response_only_user_review_with_evidence" {
				return out, storageError()
			}
			record, e := storedWorkflowRecord[domain.ResponseAcceptance](ctx, q, "response_acceptances", completion.ResponseAcceptanceID)
			if e != nil {
				return out, e
			}
			if record.TaskID != t.ID || record.TaskVersion != t.CurrentVersion || record.AttemptID != completion.ResponseBinding.ProducerAttemptID || record.ResponseArtifactID != completion.ResponseBinding.ResponseArtifactID || record.ResponseSHA256 != completion.ResponseBinding.ResponseSHA256 {
				return out, storageError()
			}
			out.NextAction = "Inspect the exact selected response and retained user-review acceptance. Next owner: user."
		default:
			return out, storageError()
		}
		return out, nil
	}
	out.Status = w.State
	out.NextAction = w.NextAction + ". Next owner: " + w.NextOwner + "."
	if w.NextAction == "response_acceptance_pending" {
		out.Status = "response_acceptance_pending"
		out.NextAction = "Review the exact selected response and optionally record user acceptance. Next owner: user."
	}
	if w.CurrentStepID != nil {
		step, e := s.workflowStep(ctx, q, *w.CurrentStepID)
		if e != nil {
			return out, e
		}
		if step.WorkflowID != w.ID {
			return out, storageError()
		}
		out.Stage = step.Role
	} else {
		out.Stage = "awaiting_execution"
	}
	return out, nil
}
