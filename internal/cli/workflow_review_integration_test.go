package cli

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
)

func TestWorkflowCLIActualReviewCorrectionAndExplicitFiniteResume(t *testing.T) {
	f := workflowCLIFixture(t)
	checksFile := filepath.Join(f.home, "controlled-checks.json")
	checks := []domain.CheckDeclaration{{Key: "controlled-validation", Executable: f.executable, Arguments: []string{"__fixture", "check", "--case", "workflow-validation"}}}
	raw, e := json.Marshal(checks)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(checksFile, raw, 0600); e != nil {
		t.Fatal(e)
	}
	var opened struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &opened)
	var admission domain.Admission
	decodeCLIData(t, f.run("tasks", "start", "--project", opened.Project.ID, "--task", "Make "+fixture.WorkflowFile+" contain exactly Council controlled workflow validation: VALID followed by CRLF. Inspect and correct the actual bytes.", "--fake", "--fake-scenario", "workflow-defect", "--review", "required", "--checks-file", checksFile, "--budget-admissions", "2"), &admission)
	if admission.Workflow == nil {
		t.Fatal("review fixture did not capture workflow authority", admission)
	}
	requiredCheckID := ""
	for _, definition := range admission.Workflow.Authority.Checks {
		if definition.Key == "controlled-validation" && definition.Required && definition.Kind == "command" {
			requiredCheckID = definition.ID
		}
	}
	if requiredCheckID == "" {
		t.Fatal("review fixture did not capture its required native check", admission.Workflow.Authority.Checks)
	}
	var checkReceipts struct {
		Items []domain.CheckReceipt `json:"items"`
	}
	var detail domain.TaskDetail
	blocked := awaitWorkflowCLI(t, f, admission.Task.ID, func(data map[string]any) bool {
		w, ok := data["workflow"].(map[string]any)
		if !ok || w["state"] != "blocked" || w["nextOwner"] != "user" || w["nextAction"] != "budget_exhausted" {
			return false
		}
		decodeCLIData(t, f.run("tasks", "checks", admission.Task.ID), &checkReceipts)
		decodeCLIData(t, f.run("tasks", "get", admission.Task.ID), &detail)
		return len(checkReceipts.Items) == 1 && checkReceipts.Items[0].DefinitionID == requiredCheckID && checkReceipts.Items[0].Outcome == "failed" && detail.LatestAttempt != nil && detail.LatestAttempt.ID == checkReceipts.Items[0].AttemptID && detail.LatestAttempt.Phase == "settled" && detail.LatestAttempt.Role == "check"
	})
	failedCheck := checkReceipts.Items[0]
	if !failedCheck.RootTerminal || failedCheck.ExitCode == nil || *failedCheck.ExitCode != 1 || !failedCheck.InputComplete || !failedCheck.OutputComplete || failedCheck.Cleanup.State != "verified" {
		t.Fatal("budget exhaustion hid the actual required check's known failed result", failedCheck)
	}
	before := f.run("tasks", "reviews", admission.Task.ID)
	var reviews struct {
		Items []domain.ReviewRecord `json:"items"`
	}
	decodeCLIData(t, before, &reviews)
	if len(reviews.Items) != 1 || reviews.Items[0].Outcome != "changes_required" || len(reviews.Items[0].Findings) == 0 || len(reviews.Items[0].Report.InspectedEvidence) == 0 {
		t.Fatalf("review did not actually expose the authored defect: %+v", reviews)
	}
	blocking := false
	for _, finding := range reviews.Items[0].Findings {
		blocking = blocking || finding.Finding.Severity == "blocking"
	}
	if !blocking {
		t.Fatal("changes_required review did not retain its actual blocking finding", reviews.Items[0])
	}
	originalReview := reviews.Items[0]
	var steps struct {
		Items []domain.WorkflowStep `json:"items"`
	}
	decodeCLIData(t, f.run("tasks", "steps", admission.Task.ID), &steps)
	native := []string{}
	for _, s := range steps.Items {
		if s.ExecutorKind == "native" {
			native = append(native, s.Role)
		}
	}
	if !reflect.DeepEqual(native, []string{"author", "review"}) {
		t.Fatalf("old two-unit task invented a correction: %v", native)
	}
	decodeCLIData(t, f.run("tasks", "get", admission.Task.ID), &detail)
	var budget domain.TaskBudgetView
	decodeCLIData(t, f.run("tasks", "budget", admission.Task.ID), &budget)
	f.run("tasks", "budget", "grant", admission.Task.ID, "--expected-revision", detail.Task.Revision, "--expected-budget-revision", budget.Budget.Revision, "--additional-admissions", "2")
	afterGrant := workflowCLIMap(t, f.run("tasks", "workflow", admission.Task.ID))["workflow"].(map[string]any)
	if afterGrant["state"] != "blocked" || afterGrant["currentStepId"] != blocked["workflow"].(map[string]any)["currentStepId"] {
		t.Fatal("grant silently rearmed automatic proof or launched work")
	}
	decodeCLIData(t, f.run("tasks", "get", admission.Task.ID), &detail)
	args := []string{"tasks", "advance", admission.Task.ID, "--expected-revision", detail.Task.Revision, "--workflow-revision", afterGrant["revision"].(string), "--task-version", detail.Task.CurrentVersion, "--request-id", domain.NewID()}
	first := f.run(args...)
	if replay := f.run(args...); !reflect.DeepEqual(first, replay) {
		t.Fatal("explicit advancement replay changed its admitted step")
	}
	awaitWorkflowCLI(t, f, admission.Task.ID, func(data map[string]any) bool {
		w, ok := data["workflow"].(map[string]any)
		return ok && w["state"] == "completed"
	})
	decodeCLIData(t, f.run("tasks", "steps", admission.Task.ID), &steps)
	native = nil
	for _, s := range steps.Items {
		if s.ExecutorKind == "native" {
			native = append(native, s.Role)
		}
	}
	if !reflect.DeepEqual(native, []string{"author", "review", "correction", "review"}) {
		t.Fatalf("correction lineage did not retain four real native roles: %v", native)
	}
	decodeCLIData(t, f.run("tasks", "reviews", admission.Task.ID), &reviews)
	if len(reviews.Items) != 2 || reviews.Items[0].Outcome != "changes_required" || reviews.Items[1].Outcome != "approved" || reviews.Items[0].CandidateBinding == nil || reviews.Items[1].CandidateBinding == nil || reviews.Items[0].CandidateBinding.CandidateID == reviews.Items[1].CandidateBinding.CandidateID || !reflect.DeepEqual(originalReview, reviews.Items[0]) {
		t.Fatalf("new proof rewrote or reused stale candidate review: %+v", reviews)
	}
	decodeCLIData(t, f.run("tasks", "budget", admission.Task.ID), &budget)
	if budget.Budget.ConsumedAdmissions != "4" || budget.Budget.RemainingAdmissions != "0" {
		t.Fatalf("four roles did not consume exactly four captured units: %+v", budget)
	}
}
