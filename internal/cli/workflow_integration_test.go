package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestWorkflowCLIRealOwnerDirectProofAndProspectivePreference(t *testing.T) {
	f := workflowCLIFixture(t)
	var opened struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &opened)
	preference := workflowCLIMap(t, f.run("workflow", "preferences", "get"))
	if preference["localMode"] != "review_only" {
		t.Fatal("fresh full-access preference silently authorized local application")
	}
	update := []string{"workflow", "preferences", "update", "--delivery", "review_only", "--expected-revision", preference["revision"].(string), "--request-id", domain.NewID()}
	first := f.run(update...)
	if replay := f.run(update...); !reflect.DeepEqual(first, replay) {
		t.Fatal("preference lost-ack replay changed its original acknowledgement")
	}
	original, e := os.ReadFile(filepath.Join(f.repo, "docs", "note.md"))
	if e != nil {
		t.Fatal(e)
	}
	request := domain.NewID()
	args := []string{"tasks", "start", "--project", opened.Project.ID, "--task", `Replace "old" with "new" in docs/note.md only`, "--fake", "--fake-scenario", "workflow-document", "--request-id", request}
	start := f.run(args...)
	var admission domain.Admission
	decodeCLIData(t, start, &admission)
	if replay := f.run(args...); !reflect.DeepEqual(start, replay) {
		t.Fatal("new workflow receipt did not replay exactly")
	}
	current := awaitWorkflowCLI(t, f, admission.Task.ID, func(data map[string]any) bool {
		workflow, ok := data["workflow"].(map[string]any)
		return ok && workflow["state"] == "completed"
	})
	workflow := current["workflow"].(map[string]any)
	authority := workflow["authority"].(map[string]any)
	delivery := authority["delivery"].(map[string]any)
	if delivery["localMode"] != "review_only" {
		t.Fatal("task did not capture saved prospective delivery")
	}
	var budget domain.TaskBudgetView
	decodeCLIData(t, f.run("tasks", "budget", admission.Task.ID), &budget)
	if budget.Budget == nil || budget.Budget.GrantedAdmissions != "4" || budget.Budget.ConsumedAdmissions != "1" {
		t.Fatalf("direct task did not use one fresh four-unit authority: %+v", budget)
	}
	steps := workflowCLIMap(t, f.run("tasks", "steps", admission.Task.ID))["items"].([]any)
	native := 0
	builtin := 0
	for _, entry := range steps {
		step := entry.(map[string]any)
		if step["executorKind"] == "native" {
			native++
		}
		if step["role"] == "check" && step["executorKind"] == "local_check" {
			builtin++
		}
	}
	if native != 1 || builtin != 1 {
		t.Fatalf("direct proof fabricated extra model work or omitted owned builtin: %v", steps)
	}
	after, e := os.ReadFile(filepath.Join(f.repo, "docs", "note.md"))
	if e != nil || !reflect.DeepEqual(after, original) {
		t.Fatal("review-only workflow changed destination bytes")
	}
	var checks struct {
		Items []domain.CheckReceipt `json:"items"`
	}
	decodeCLIData(t, f.run("tasks", "checks", admission.Task.ID), &checks)
	if len(checks.Items) != 1 || !checks.Items[0].BuiltinCompleted || checks.Items[0].ExitCode != nil {
		t.Fatalf("builtin proof acquired fabricated process/exit facts: %+v", checks)
	}
}
