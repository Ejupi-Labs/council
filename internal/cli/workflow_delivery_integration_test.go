package cli

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestWorkflowCLIExplicitPublisherRecordsOnlyCommandCompletionAndReplaysOnce(t *testing.T) {
	f := workflowCLIFixture(t)
	destination := filepath.Join(f.home, "controlled-publication-marker.txt")
	declaration := domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &domain.ExternalActionInput{Action: "record controlled local publication marker", Destination: destination, AcceptanceBasis: "command_completion", Command: domain.ExternalCommandInput{Executable: f.executable, Arguments: []string{"__fixture", "publish", "--case", "workflow-publication", "--destination", destination}}}}
	raw, e := json.Marshal(declaration)
	if e != nil {
		t.Fatal(e)
	}
	filename := filepath.Join(f.home, "explicit-delivery.json")
	if e = os.WriteFile(filename, raw, 0600); e != nil {
		t.Fatal(e)
	}
	var opened struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &opened)
	args := []string{"tasks", "start", "--project", opened.Project.ID, "--task", `Replace "old" with "new" in docs/note.md only`, "--fake", "--fake-scenario", "workflow-document", "--delivery-file", filename, "--request-id", domain.NewID()}
	first := f.run(args...)
	var admission domain.Admission
	decodeCLIData(t, first, &admission)
	awaitWorkflowCLI(t, f, admission.Task.ID, func(data map[string]any) bool {
		w, ok := data["workflow"].(map[string]any)
		return ok && w["state"] == "completed"
	})
	bytes, e := os.ReadFile(destination)
	if e != nil || string(bytes) != "Council controlled local publication marker (005-v1).\n" {
		t.Fatalf("declared fixed child did not produce its actual one attributable effect: %v", e)
	}
	var deliveries struct {
		Items []domain.WorkflowDeliveryView `json:"items"`
	}
	decodeCLIData(t, f.run("tasks", "deliveries", admission.Task.ID), &deliveries)
	external := 0
	for _, item := range deliveries.Items {
		if item.Kind == "external_command" {
			external++
			if item.External == nil || item.External.Outcome != "command_succeeded" || item.External.Action.AcceptanceBasis != "command_completion" || !reflect.DeepEqual(item.External.Command.Arguments, declaration.ExternalAction.Command.Arguments) {
				t.Fatalf("command result/scope changed or remote state was fabricated: %+v", item)
			}
		}
		if item.Kind == "local_application" {
			t.Fatal("review-only external declaration silently applied the local candidate")
		}
	}
	if external != 1 {
		t.Fatalf("publisher result count: %d", external)
	}
	if replay := f.run(args...); !reflect.DeepEqual(first, replay) {
		t.Fatal("publisher-bearing Start lost-ack replay changed original acknowledgement")
	}
	after, e := os.ReadFile(destination)
	if e != nil || !reflect.DeepEqual(bytes, after) {
		t.Fatal("known receipt replay relaunched or altered publication effect")
	}
	var budget domain.TaskBudgetView
	decodeCLIData(t, f.run("tasks", "budget", admission.Task.ID), &budget)
	if budget.Budget.ConsumedAdmissions != "1" {
		t.Fatalf("local checks/external child fabricated model admissions: %+v", budget)
	}
}
