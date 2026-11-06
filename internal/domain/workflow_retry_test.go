package domain

import "testing"

func TestWorkflowBlockedNativeRetryPlanCurrentExactOnly(t *testing.T) {
	for _, role := range []string{"author", "review", "correction"} {
		t.Run(role, func(t *testing.T) {
			id, stepID, attemptID, taskID := NewID(), NewID(), NewID(), NewID()
			cycle := 0
			if role == "correction" {
				cycle = 1
			}
			outcome := "blocked"
			w := Workflow{ID: id, TaskID: taskID, TaskVersion: "1", State: "blocked", CurrentStepID: &stepID, CorrectionCyclesUsed: cycle, Authority: WorkflowAuthority{ExecutionMode: "fake", FakeScenario: "response", Correction: CorrectionPolicy{MaxCycles: 1}}}
			subject := ""
			if role != "author" {
				candidate := NewID()
				w.SelectedCandidateID = &candidate
				subject = candidate
			}
			key, invalid := WorkflowStageKey(role, cycle, "", subject)
			if invalid != nil {
				t.Fatal(invalid)
			}
			step := WorkflowStep{ID: stepID, WorkflowID: id, Role: role, ExecutorKind: "native", Cycle: cycle, LogicalKey: key, Status: "blocked", AttemptID: &attemptID, InputCandidateID: w.SelectedCandidateID}
			a := Attempt{ID: attemptID, WorkflowID: id, StepID: stepID, TaskID: taskID, TaskVersion: "1", Role: role, ExecutorKind: "native", ExecutionMode: "fake", FakeScenario: "response", Phase: "settled", Outcome: &outcome, InputCandidateID: step.InputCandidateID, Cleanup: Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}
			plan := BlockedNativeRetryPlan(w, step, a)
			if plan == nil || plan.Role != role || plan.Cycle != cycle || plan.LogicalKey != step.LogicalKey || plan.ReplacementOfStepID == nil || *plan.ReplacementOfStepID != stepID || plan.PredecessorStepID == nil || *plan.PredecessorStepID != stepID {
				t.Fatal("current blocked slot cannot be retried explicitly", plan)
			}
			cases := map[string]func(*Workflow, *WorkflowStep, *Attempt){"launched": func(_ *Workflow, _ *WorkflowStep, a *Attempt) { a.LaunchIntent = true }, "stop": func(_ *Workflow, _ *WorkflowStep, a *Attempt) { a.Stop = &AcceptedStop{} }, "uncertain": func(_ *Workflow, _ *WorkflowStep, a *Attempt) { a.EvidenceStatus = "uncertain" }, "unknown_cleanup": func(_ *Workflow, _ *WorkflowStep, a *Attempt) { a.Cleanup.State = "unknown" }, "historical": func(w *Workflow, _ *WorkflowStep, _ *Attempt) { other := NewID(); w.CurrentStepID = &other }, "proof": func(_ *Workflow, s *WorkflowStep, _ *Attempt) { s.AdvanceProof = &WorkflowAdvanceProof{} }, "failed": func(_ *Workflow, _ *WorkflowStep, a *Attempt) { outcome := "failed"; a.Outcome = &outcome }, "mode": func(_ *Workflow, _ *WorkflowStep, a *Attempt) { a.ExecutionMode = "native" }, "wrong_input": func(w *Workflow, _ *WorkflowStep, _ *Attempt) { other := NewID(); w.SelectedCandidateID = &other }}
			for name, mutate := range cases {
				t.Run(name, func(t *testing.T) {
					changedW, changedS, changedA := w, step, a
					mutate(&changedW, &changedS, &changedA)
					if got := BlockedNativeRetryPlan(changedW, changedS, changedA); got != nil {
						t.Fatal("noncurrent/unsafe execution received retry authority", got)
					}
				})
			}
		})
	}
}
