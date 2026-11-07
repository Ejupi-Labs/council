package domain

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestGraphAdmissionSharedBudgetDoesNotMasqueradeAsChild(t *testing.T) {
	child, root := strings.Repeat("1", 32), strings.Repeat("2", 32)
	ref := &GraphBudgetRef{GraphID: strings.Repeat("3", 32), BudgetOwnerTaskID: root}
	for _, value := range []any{
		Admission{GraphBudgetRef: ref, Task: TaskSummary{ID: child}, Budget: &TaskBudget{TaskID: root, RemainingAdmissions: "9007199254740993"}},
		WorkflowStepAdmission{GraphBudgetRef: ref, Attempt: Attempt{TaskID: child}, Budget: TaskBudget{TaskID: root, RemainingAdmissions: "9007199254740993"}},
	} {
		b, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var fields map[string]jsontext.Value
		if e = json.Unmarshal(b, &fields); e != nil {
			t.Fatal(e)
		}
		if string(fields["budget"]) != "null" || fields["graphBudgetRef"].Kind() != '{' || strings.Contains(string(b), "9007199254740993") {
			t.Fatal("root budget leaked as child budget", string(b))
		}
		if !strings.Contains(string(b), child) {
			t.Fatal("actual child identity lost")
		}
	}
}

func TestGraphProjectionPreservesStandaloneOptionalFields(t *testing.T) {
	b, e := json.Marshal(Admission{})
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]jsontext.Value
	if e = json.Unmarshal(b, &fields); e != nil {
		t.Fatal(e)
	}
	if _, ok := fields["budget"]; ok {
		t.Fatal("accepted standalone absent budget became null")
	}
	if _, ok := fields["graphBudgetRef"]; ok {
		t.Fatal("standalone graph default injected")
	}
	for _, value := range []any{Task{}, TaskSummary{}, BudgetEffect{}, WorkflowStepAdmission{}} {
		b, e := json.Marshal(value)
		if e != nil || strings.Contains(string(b), "graphRef") || strings.Contains(string(b), "graphBudgetRef") {
			t.Fatal("standalone field shape changed", string(b), e)
		}
	}
}
