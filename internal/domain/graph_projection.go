package domain

import json "encoding/json/v2"

// A graph-owned wire admission references shared authority explicitly. Canonical
// root budget facts belong in internal controller/store facts, never in this
// public field. The alias branch preserves accepted standalone omissions.
func (in Admission) MarshalJSON() ([]byte, error) {
	type standalone Admission
	if in.GraphBudgetRef == nil {
		return json.Marshal(standalone(in))
	}
	return json.Marshal(struct {
		Workflow        *Workflow        `json:"workflow,omitzero"`
		Step            *WorkflowStep    `json:"step,omitempty"`
		RoutingDecision *RoutingDecision `json:"routingDecision,omitempty"`
		Budget          *TaskBudget      `json:"budget"`
		GraphBudgetRef  *GraphBudgetRef  `json:"graphBudgetRef"`
		Task            TaskSummary      `json:"task"`
		Version         TaskVersion      `json:"version"`
		Attempt         Attempt          `json:"attempt"`
	}{in.Workflow, in.Step, in.RoutingDecision, nil, in.GraphBudgetRef, in.Task, in.Version, in.Attempt})
}

func (in WorkflowStepAdmission) MarshalJSON() ([]byte, error) {
	type standalone WorkflowStepAdmission
	if in.GraphBudgetRef == nil {
		return json.Marshal(standalone(in))
	}
	return json.Marshal(struct {
		Workflow        Workflow         `json:"workflow"`
		Step            WorkflowStep     `json:"step"`
		Attempt         Attempt          `json:"attempt"`
		Budget          *TaskBudget      `json:"budget"`
		GraphBudgetRef  *GraphBudgetRef  `json:"graphBudgetRef"`
		RoutingDecision *RoutingDecision `json:"routingDecision,omitzero"`
	}{in.Workflow, in.Step, in.Attempt, nil, in.GraphBudgetRef, in.RoutingDecision})
}
