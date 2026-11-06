package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestWorkflowCaptureAutoDefaultAndExplicitEmptyChecks(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath := testEngine(t)
	stopFixtureScheduler(t, e)
	if err := os.WriteFile(filepath.Join(projectPath, "package.json"), []byte("{\"scripts\":{\"test\":\"npm run tests\"}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	empty := []domain.CheckDeclaration{}
	input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Inspect this project.", ExecutionMode: "fake", FakeScenario: "response", Workflow: &domain.WorkflowInput{Checks: &empty}}
	result := e.StartTask(ctx, input)
	if result.Status != 201 {
		t.Fatal(result)
	}
	admission := decodeAdmission(t, result)
	if admission.Workflow == nil || admission.Step == nil || admission.Workflow.Authority.OptionSnapshot.Mode != "auto" || admission.Attempt.Role != "author" {
		t.Fatal("Normal start did not capture the Auto author workflow", admission)
	}
	authority := admission.Workflow.Authority
	if authority.OptionSnapshot.Checks == nil || len(*authority.OptionSnapshot.Checks) != 0 || len(authority.Checks) != 0 || len(authority.CheckGaps) != 0 {
		t.Fatal("Explicit empty checks were replaced by inferred commands", authority)
	}
	savedProject, projectIdentity, err := e.db.Project(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	// TEMP may use an alias (Windows short paths or Darwin /var). The captured
	// destination must use the saved canonical path and name the selected folder.
	selectedInfo, selectedErr := os.Stat(projectPath)
	capturedInfo, capturedErr := os.Stat(authority.Delivery.Destination)
	if authority.Delivery.LocalMode != "review_only" || authority.Delivery.ProjectID != projectID || authority.Delivery.ProjectIdentity != projectIdentity || authority.Delivery.RepositoryIdentity == "" || authority.Delivery.Destination != savedProject.ResolvedPath || selectedErr != nil || capturedErr != nil || !os.SameFile(selectedInfo, capturedInfo) {
		t.Fatalf("Default delivery scope lost its canonical/physical binding or granted application: selected=%q canonical=%q captured=%+v selectedError=%v capturedError=%v", projectPath, savedProject.ResolvedPath, authority.Delivery, selectedErr, capturedErr)
	}
}

func TestWorkflowCaptureOmittedAutoAndAuthorOnlyRemainDistinct(t *testing.T) {
	ctx := context.Background()
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	for _, standalone := range []bool{false, true} {
		input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Inspect this project.", ExecutionMode: "fake", FakeScenario: "response"}
		if standalone {
			input.Workflow = &domain.WorkflowInput{Mode: "author_only"}
		}
		result := e.StartTask(ctx, input)
		if result.Status != 201 {
			t.Fatal(result)
		}
		admission := decodeAdmission(t, result)
		if standalone {
			if admission.Workflow != nil || admission.Attempt.WorkflowID != "" || admission.Attempt.Role != "" {
				t.Fatal("Author only acquired workflow authority", admission)
			}
		} else if admission.Workflow == nil || admission.Workflow.Authority.OptionSnapshot.Mode != "auto" {
			t.Fatal("Omitted workflow did not resolve to Auto", admission)
		}
	}
}

func TestWorkflowCaptureReplayPrecedesCurrentSourceInspection(t *testing.T) {
	ctx := context.Background()
	e, projectID, projectPath := testEngine(t)
	stopFixtureScheduler(t, e)
	input := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: projectID, OriginalText: "Inspect this project.", ExecutionMode: "fake", FakeScenario: "response"}
	first := e.StartTask(ctx, input)
	if first.Status != 201 || decodeAdmission(t, first).Workflow == nil {
		t.Fatal(first)
	}
	// A marker that cannot be a supported regular file invalidates fresh capture.
	// Receipt replay must still return the exact original workflow admission.
	if err := os.Mkdir(filepath.Join(projectPath, "package.json"), 0700); err != nil {
		t.Fatal(err)
	}
	replayed := e.StartTask(ctx, input)
	if replayed.Status != first.Status || replayed.Envelope.Receipt == nil || first.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *first.Envelope.Receipt {
		t.Fatal("Replay re-read current convention metadata", replayed)
	}
	input.RequestID = domain.NewID()
	refused := e.StartTask(ctx, input)
	if refused.Envelope.Error == nil || refused.Envelope.Error.Code != "evidence_unavailable" {
		t.Fatal("Fresh capture accepted an invalid marker", refused)
	}
}
