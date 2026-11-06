package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// This exercises the actual command/process/evidence seam under a standalone
// owned claim. Local zero-unit workflow admission is a separate store proof.
func TestWorkflowCommandRetainsNonzeroExitWithoutInventingProtocolOrTests(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath := testEngine(t)
	stopFixtureScheduler(t, e)
	if err := os.WriteFile(filepath.Join(projectPath, fixture.WorkflowFile), []byte(fixture.WorkflowInvalid), 0600); err != nil {
		t.Fatal(err)
	}
	admitted := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Run the controlled owned command seam.", ExecutionMode: "fake", Workflow: &domain.WorkflowInput{Mode: "author_only"}})
	if admitted.Status != 201 {
		t.Fatal(admitted)
	}
	a := decodeAdmission(t, admitted).Attempt
	claimed, err := e.db.ClaimAttempt(ctx, a.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.workspace.Capture(ctx, workspace.Request{AttemptID: a.ID, ProjectPath: projectPath})
	if err != nil {
		t.Fatal(err)
	}
	_, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.registerWorkflowSnapshot(ctx, claimed, snapshot, identity); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	program, err := workflowProgram(executable)
	if err != nil {
		t.Fatal(err)
	}
	command := domain.CommandDefinition{Executable: program.Executable, PhysicalIdentity: program.PhysicalIdentity, Arguments: []string{"__fixture", "check", "--case", "workflow-validation"}, RelativeWorkingDirectory: ".", TimeoutSeconds: 10, SuccessExitCodes: []int{0}}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	observed, err := e.executeWorkflowCommand(bounded, claimed, snapshot, command)
	if err != nil || !observed.Started || !observed.Settlement.RootTerminal || observed.Settlement.ExitCode == nil || *observed.Settlement.ExitCode != 1 || observed.Settlement.Cleanup.State != "verified" || !observed.Settlement.InputComplete || !observed.OutputComplete {
		t.Fatal("The actual failed verifier lost root/drain/ownership facts", observed, err)
	}
	stdout, err := e.readResponse(ctx, observed.Stdout)
	if err != nil || string(stdout) != "Council fixed local verifier (005-v1).\n" {
		t.Fatal("Plain stdout was parsed or replaced by invented protocol/test counts", string(stdout), err)
	}
	if observed.Stdout.Kind != "stdout" || observed.Stderr.Kind != "stderr" || observed.Settlement.BuiltinCompleted {
		t.Fatal("Command observations invented builtin/model facts", observed)
	}
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || current.PrimaryFailure != nil {
		t.Fatal("Generic executor turned a known exit into an operational protocol failure", current, err)
	}
	settlement := observed.Settlement
	settlement.Outcome = "failed"
	if _, err = e.db.SettleAttempt(ctx, a.ID, settlement); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(projectPath, fixture.WorkflowFile))
	if err != nil || string(original) != fixture.WorkflowInvalid {
		t.Fatal("The verifier changed the original project", err)
	}
}
