package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestMixedHarnessesShareTwoContextClaimsAndCaptureActualConnection(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	project := openFixture(t, s, "mixed")
	owner := domain.NewID()
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	starts := []domain.Admission{}
	for _, harness := range []string{"codex", "claude-code", "codex", "claude-code"} {
		in := taskInput(project.ID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness, Model: "native-alias"}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		starts = append(starts, r.Envelope.Data.(domain.Admission))
	}
	for i, connection := range []string{"codex-native", "claude-native", "codex-native", "claude-native"} {
		a := starts[i].Attempt
		if a.ConnectionID != connection || a.ResolvedTarget == nil || a.ResolvedTarget.ConnectionID != connection ||
			a.ResolvedTarget.ContextID != connection || a.ResolvedTarget.ModelSelector != "native-alias" {
			t.Fatal("An admission lost its exact harness/context/model capture", i, a)
		}
	}
	for _, admitted := range starts[:2] {
		if a, err := s.ClaimAttempt(ctx, admitted.Attempt.ID, owner); err != nil || a.Phase != "preparing" ||
			!reflect.DeepEqual(a.ResolvedTarget, admitted.Attempt.ResolvedTarget) || a.ConnectionID != admitted.Attempt.ConnectionID {
			t.Fatal("Two distinct supported contexts did not obtain their actual slots", a, err)
		}
	}
	claims, err := s.GraphClaims(ctx)
	if err != nil || len(claims) != 2 || claims[0].Slot != "1" || claims[1].Slot != "2" ||
		claims[0].AttemptID != starts[0].Attempt.ID || claims[1].AttemptID != starts[1].Attempt.ID ||
		claims[0].ContextID == nil || claims[1].ContextID == nil || *claims[0].ContextID != "codex-native" || *claims[1].ContextID != "claude-native" ||
		claims[0].ExecutorKind != "native" || claims[1].ExecutorKind != "native" || claims[0].LegacyBarrier || claims[1].LegacyBarrier {
		t.Fatal("The shared pool did not retain exactly two independently captured native contexts", claims, err)
	}
	for _, waiting := range starts[2:] {
		if _, err := s.ClaimAttempt(ctx, waiting.Attempt.ID, owner); err == nil || domain.SafeError(err).Code != "attempt_active" {
			t.Fatal("A further mixed-harness claim bypassed the bounded pool", waiting.Attempt.ID, err)
		}
		if a, err := s.Attempt(ctx, waiting.Attempt.ID); err != nil || a.Phase != "queued" || a.SourceSnapshotID != nil || a.LaunchIntent ||
			!reflect.DeepEqual(a.ResolvedTarget, waiting.Attempt.ResolvedTarget) || a.ConnectionID != waiting.Attempt.ConnectionID {
			t.Fatal("A refused claim changed its captured target or created launch/source effects", a, err)
		}
	}
	if _, err := s.SettleAttempt(ctx, starts[0].Attempt.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckAttemptClaim(ctx, owner, starts[1].Attempt.ID); err != nil {
		t.Fatal("Releasing Codex discarded the independently held Claude claim", err)
	}
	// One global slot is now free, but Claude's exact context is still occupied.
	if _, err := s.ClaimAttempt(ctx, starts[3].Attempt.ID, owner); err == nil || domain.SafeError(err).Code != "attempt_active" {
		t.Fatal("A free slot incorrectly bypassed the occupied native context", err)
	}
	if a, err := s.ClaimAttempt(ctx, starts[2].Attempt.ID, owner); err != nil || a.Phase != "preparing" || a.ConnectionID != "codex-native" ||
		!reflect.DeepEqual(a.ResolvedTarget, starts[2].Attempt.ResolvedTarget) {
		t.Fatal("The exact released context/slot did not admit its queued captured target", a, err)
	}
	claims, err = s.GraphClaims(ctx)
	if err != nil || len(claims) != 2 || claims[0].Slot != "1" || claims[0].AttemptID != starts[2].Attempt.ID ||
		claims[1].Slot != "2" || claims[1].AttemptID != starts[1].Attempt.ID {
		t.Fatal("A specific release replaced or multiplied another owner's claim", claims, err)
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
