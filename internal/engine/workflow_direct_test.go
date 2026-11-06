package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestWorkflowDirectRealOwnedAuthorBuiltinAndReviewOnlyAcceptance(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath := testEngine(t)
	if err := os.Mkdir(filepath.Join(projectPath, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("\ufeffControlled exact source\r\nold\r\n東京\r\n")
	if err := os.WriteFile(filepath.Join(projectPath, "docs", "note.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, projectPath, "add", "--", "docs/note.md")
	interopGit(t, projectPath, "commit", "--quiet", "-m", "tracked direct document")
	indexBefore, err := os.ReadFile(filepath.Join(projectPath, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	result := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Replace \"old\" with \"new\" in docs/note.md only", ExecutionMode: "fake", FakeScenario: "workflow-document"})
	if result.Status != 201 {
		t.Fatal(result)
	}
	admission := decodeAdmission(t, result)
	if admission.Workflow == nil {
		t.Fatal("Normal Auto lacks workflow authority")
	}
	deadline := time.Now().Add(15 * time.Second)
	var current domain.WorkflowDetail
	for {
		var err error
		current, err = e.db.Workflow(ctx, admission.Task.ID, domain.WorkflowSelector{})
		if err != nil {
			t.Fatal(err)
		}
		if current.Workflow != nil && current.Workflow.State == "completed" {
			break
		}
		gate, err := e.db.Gate(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if gate.State == "fenced" || time.Now().After(deadline) {
			attempt, _ := e.db.Attempt(ctx, admission.Attempt.ID)
			progress, progressErr := e.inspectWorkflowProgress(ctx, admission.Task.ID, *current.Workflow)
			steps, stepsErr := e.db.WorkflowSteps(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
			for _, step := range steps.Items {
				if step.AttemptID != nil {
					supporting, _ := e.db.Attempt(ctx, *step.AttemptID)
					t.Logf("role=%s executor=%s primaryFailure=%+v", supporting.Role, supporting.ExecutorKind, supporting.PrimaryFailure)
				}
			}
			t.Fatalf("Actual Auto did not complete: workflow=%+v attempt=%+v gate=%+v progress=%+v progressError=%v steps=%+v stepsError=%v", *current.Workflow, attempt, gate, progress.decision, progressErr, steps, stepsErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if current.Acceptance == nil || current.Acceptance.DeliveryResult != "review_only" || current.Acceptance.DeliveryOperationID != nil {
		t.Fatal("Direct proof invented local application or omitted qualified acceptance", current)
	}
	steps, err := e.db.WorkflowSteps(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps.Items) != 2 || steps.Items[0].Role != "author" || steps.Items[1].Role != "check" || steps.Items[1].ExecutorKind != "local_check" {
		t.Fatal("Direct proof added model review work or omitted its owned proof", steps)
	}
	checks, err := e.db.WorkflowChecks(ctx, admission.Task.ID, domain.WorkflowSelector{}, 100, "")
	if err != nil || len(checks.Items) != 1 || !checks.Items[0].BuiltinCompleted || checks.Items[0].ExitCode != nil || checks.Items[0].RootTerminal {
		t.Fatal("Builtin proof fabricated process evidence", checks, err)
	}
	budget, err := e.db.TaskBudget(ctx, admission.Task.ID)
	if err != nil || budget.Budget == nil || budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("Builtin proof consumed a model unit", budget, err)
	}
	if current.Workflow.HasTaskIndexDelta || !current.Workflow.HasTaskDelta || current.Workflow.SelectedCandidateID == nil {
		t.Fatal("a tracked working-file change invented an index delta", current.Workflow)
	}
	candidate, err := e.db.Candidate(ctx, *current.Workflow.SelectedCandidateID)
	if err != nil || candidate.HasIndexDelta || !candidate.HasDelta {
		t.Fatal("candidate creation facts invented a staged change", candidate, err)
	}
	indexAfter, err := os.ReadFile(filepath.Join(projectPath, ".git", "index"))
	if err != nil || string(indexAfter) != string(indexBefore) {
		t.Fatal("tracked working-file fixture changed the actual user index", err)
	}
	after, err := os.ReadFile(filepath.Join(projectPath, "docs", "note.md"))
	if err != nil || string(after) != string(original) {
		t.Fatal("Review-only completion changed the user's project", err)
	}
}
