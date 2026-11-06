package domain

import "reflect"

// BlockedNativeRetryPlan recognizes only the current clean, never-prepared
// blocked native execution. A returned plan still requires current transactional
// owner/version/control/budget validation and fresh controller source evidence.
func BlockedNativeRetryPlan(w Workflow, step WorkflowStep, attempt Attempt) *WorkflowStagePlan {
	if w.State != "blocked" || w.CurrentStepID == nil || *w.CurrentStepID != step.ID || w.LatestAcceptanceID != nil || w.Completion != nil || step.WorkflowID != w.ID || attempt.WorkflowID != w.ID || attempt.StepID != step.ID || attempt.TaskID != w.TaskID || attempt.TaskVersion != w.TaskVersion || step.AttemptID == nil || *step.AttemptID != attempt.ID || step.Status != "blocked" || step.AdvanceProof != nil || step.ExecutorKind != "native" || attempt.ExecutorKind != "native" || attempt.Role != step.Role || attempt.Phase != "settled" || attempt.Outcome == nil || *attempt.Outcome != "blocked" || attempt.LaunchIntent || attempt.Stop != nil || attempt.Cleanup.State != "verified" || attempt.Cleanup.Scope != "not_launched" || (attempt.EvidenceStatus != "complete" && attempt.EvidenceStatus != "incomplete") || attempt.ExecutionMode != w.Authority.ExecutionMode || attempt.FakeScenario != w.Authority.FakeScenario {
		return nil
	}
	if !reflect.DeepEqual(step.InputCandidateID, w.SelectedCandidateID) || !reflect.DeepEqual(step.InputResponseBinding, w.SelectedResponseBinding) || !reflect.DeepEqual(attempt.InputCandidateID, step.InputCandidateID) {
		return nil
	}
	switch step.Role {
	case "author":
		if step.Cycle != 0 || w.CorrectionCyclesUsed != 0 || step.InputCandidateID != nil || step.InputResponseBinding != nil {
			return nil
		}
	case "review":
		if step.Cycle != w.CorrectionCyclesUsed {
			return nil
		}
	case "correction":
		if step.Cycle != w.CorrectionCyclesUsed || step.Cycle < 1 || step.Cycle > w.Authority.Correction.MaxCycles {
			return nil
		}
	default:
		return nil
	}
	definition, subject := "", ""
	if step.DefinitionID != nil {
		definition = *step.DefinitionID
	}
	if step.InputCandidateID != nil {
		subject = *step.InputCandidateID
	} else if step.InputResponseBinding != nil {
		subject = step.InputResponseBinding.ResponseArtifactID
	}
	key, invalid := WorkflowStageKey(step.Role, step.Cycle, definition, subject)
	if invalid != nil || key != step.LogicalKey {
		return nil
	}
	predecessor, replacement := step.ID, step.ID
	plan := &WorkflowStagePlan{Role: step.Role, ExecutorKind: "native", LogicalKey: step.LogicalKey, Cycle: step.Cycle, PredecessorStepID: &predecessor, ReplacementOfStepID: &replacement, ReasonCodes: []string{"explicit_current_blocked_retry"}}
	if step.DefinitionID != nil {
		value := *step.DefinitionID
		plan.DefinitionID = &value
	}
	if step.InputCandidateID != nil {
		value := *step.InputCandidateID
		plan.InputCandidateID = &value
	}
	if step.InputResponseBinding != nil {
		value := *step.InputResponseBinding
		plan.InputResponseBinding = &value
	}
	return plan
}
