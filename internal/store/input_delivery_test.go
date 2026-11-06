package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"testing"
)

func TestSuccessfulRootAndResultCannotWaiveIncompleteInput(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	project := openFixture(t, s, "input-delivery")
	owner := domain.NewID()
	result := s.CreateTask(ctx, owner, taskInput(project.ID))
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	admission := result.Envelope.Data.(domain.Admission)
	launchFixture(t, s, admission, owner)
	response := closedArtifact(t, s, admission.Attempt.ID, "final_response")
	if _, err := s.RecordAttempt(ctx, admission.Attempt.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &response.ID}); err != nil {
		t.Fatal(err)
	}
	zero := 0
	a, err := s.SettleAttempt(ctx, admission.Attempt.ID, domain.Settlement{Outcome: "succeeded", RootTerminal: true, InputComplete: false, ExitCode: &zero, UsableOutput: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"})
	if err != nil || a.Outcome == nil || *a.Outcome != "failed" || a.PrimaryFailure == nil {
		t.Fatalf("Incomplete input became success: %+v %v", a, err)
	}
}
