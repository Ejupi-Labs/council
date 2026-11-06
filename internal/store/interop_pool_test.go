package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestMixedHarnessesShareOneClaimAndCaptureActualConnection(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	project := openFixture(t, s, "mixed")
	owner := domain.NewID()
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	starts := []domain.Admission{}
	for _, harness := range []string{"codex", "claude-code"} {
		in := taskInput(project.ID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness, Model: "native-alias"}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		starts = append(starts, r.Envelope.Data.(domain.Admission))
	}
	if starts[0].Attempt.ConnectionID != "codex-native" || starts[1].Attempt.ConnectionID != "claude-native" {
		t.Fatal(starts)
	}
	if _, err := s.ClaimAttempt(ctx, starts[0].Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimAttempt(ctx, starts[1].Attempt.ID, owner); err == nil || domain.SafeError(err).Code != "attempt_active" {
		t.Fatal("second harness bypassed pool", err)
	}
	if a, err := s.Attempt(ctx, starts[1].Attempt.ID); err != nil || a.Phase != "queued" || a.SourceSnapshotID != nil || a.LaunchIntent {
		t.Fatal(a, err)
	}
	if _, err := s.SettleAttempt(ctx, starts[0].Attempt.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); err != nil {
		t.Fatal(err)
	}
	if a, err := s.ClaimAttempt(ctx, starts[1].Attempt.ID, owner); err != nil || a.Phase != "preparing" || a.ConnectionID != "claude-native" {
		t.Fatal(a, err)
	}
	// Changed intent under the original ID conflicts even though the target exists.
	conflict := taskInput(project.ID)
	conflict.RequestID = starts[0].Attempt.StartRequestID
	conflict.Pins = &domain.ExecutionPins{HarnessID: "claude-code"}
	if r := s.CreateTask(ctx, owner, conflict); r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal(r)
	}
}

func populateMigrationHistory(t *testing.T, s *Store, projectID, owner string) {
	t.Helper()
	ctx := context.Background()
	draft := s.CreateDraft(ctx, domain.NewID(), "migration-draft", projectID, "immutable saved draft")
	if draft.Envelope.Error != nil {
		t.Fatal(draft)
	}
	in := taskInput(projectID)
	in.Acceptance = &domain.AcceptanceInput{Deliverable: "response_only"}
	r := s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	admission := r.Envelope.Data.(domain.Admission)
	launchFixture(t, s, admission, owner)
	a, err := s.Attempt(ctx, admission.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	manifest := closedArtifact(t, s, a.ID, "candidate_manifest")
	delta := closedArtifact(t, s, a.ID, "candidate_delta")
	candidate := domain.Candidate{ID: domain.NewID(), AttemptID: a.ID, SourceSnapshotID: *a.SourceSnapshotID, ManifestArtifactID: manifest.ID, DeltaArtifactID: delta.ID, WorkspaceRef: "workspaces/" + a.ID, Verification: "pending", Integration: "not_integrated", CreatedAt: domain.Now()}
	if err = s.PutCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	response := closedArtifact(t, s, a.ID, "final_response")
	if _, err = s.RecordAttempt(ctx, a.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &response.ID}); err != nil {
		t.Fatal(err)
	}
	zero := 0
	if _, err = s.SettleAttempt(ctx, a.ID, domain.Settlement{Outcome: "succeeded", RootTerminal: true, InputComplete: true, ExitCode: &zero, UsableOutput: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"}); err != nil {
		t.Fatal(err)
	}
	accept := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: "1", AttemptID: a.ID, ResponseArtifactID: response.ID, ResponseSHA256: *response.SHA256, Confirmation: "reviewed_response_and_conditions"}
	proof := domain.VerifiedResponse{ArtifactID: response.ID, SHA256: *response.SHA256, PhysicalIdentity: response.PhysicalIdentity, ByteLength: response.ByteLength}
	if r = s.AcceptResponse(ctx, admission.Task.ID, accept, proof); r.Status != 200 {
		t.Fatal(r)
	}
}
