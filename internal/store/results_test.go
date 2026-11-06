package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
	"testing"
)

func closedArtifact(t *testing.T, s *Store, attemptID, kind string) domain.ArtifactRecord {
	t.Helper()
	id := domain.NewID()
	hash := strings.Repeat("a", 64)
	record := domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: id, AttemptID: attemptID, Kind: kind, ByteLength: "12", Closed: true, Completeness: "complete", SHA256: &hash, CreatedAt: domain.Now()}, PrivateRef: "artifacts/" + id + ".bin", PhysicalIdentity: "fixture:" + id}
	if e := s.PutArtifact(context.Background(), record); e != nil {
		t.Fatal(e)
	}
	return record
}
func launchFixture(t *testing.T, s *Store, admission domain.Admission, owner string) {
	t.Helper()
	ctx := context.Background()
	if _, e := s.ClaimAttempt(ctx, admission.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	manifest := closedArtifact(t, s, admission.Attempt.ID, "source_manifest")
	index := closedArtifact(t, s, admission.Attempt.ID, "index_snapshot")
	snapshot := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: admission.Attempt.ID, ManifestArtifactID: manifest.ID, IndexArtifactID: index.ID, ManifestSHA256: *manifest.SHA256, CreatedAt: domain.Now()}
	if e := s.PutSourceSnapshot(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RecordLaunchIntent(ctx, admission.Attempt.ID); e != nil {
		t.Fatal(e)
	}
}
func TestResponseAcceptanceRequiresFreshBoundProofAndImmutableTerminal(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "response")
	in := taskInput(p.ID)
	in.Acceptance = &domain.AcceptanceInput{Deliverable: "response_only"}
	owner := domain.NewID()
	r := s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	admission := r.Envelope.Data.(domain.Admission)
	launchFixture(t, s, admission, owner)
	artifact := closedArtifact(t, s, admission.Attempt.ID, "final_response")
	if _, e := s.RecordAttempt(ctx, admission.Attempt.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &artifact.ID}); e != nil {
		t.Fatal(e)
	}
	zero := 0
	a, e := s.SettleAttempt(ctx, admission.Attempt.ID, domain.Settlement{Outcome: "succeeded", RootTerminal: true, InputComplete: true, ExitCode: &zero, UsableOutput: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"})
	if e != nil || a.Outcome == nil || *a.Outcome != "succeeded" {
		t.Fatal(a, e)
	}
	accept := domain.ResponseAcceptanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: "1", AttemptID: a.ID, ResponseArtifactID: artifact.ID, ResponseSHA256: *artifact.SHA256, Confirmation: "reviewed_response_and_conditions"}
	r = s.AcceptResponse(ctx, admission.Task.ID, accept, domain.VerifiedResponse{})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "acceptance_unavailable" {
		t.Fatal(r)
	}
	proof := domain.VerifiedResponse{ArtifactID: artifact.ID, SHA256: *artifact.SHA256, PhysicalIdentity: artifact.PhysicalIdentity, ByteLength: artifact.ByteLength}
	r = s.AcceptResponse(ctx, admission.Task.ID, accept, proof)
	if r.Status != 200 {
		t.Fatal(r)
	}
	r = s.AcceptResponse(ctx, admission.Task.ID, accept, domain.VerifiedResponse{})
	if r.Status != 200 {
		t.Fatal("replay attempted a new proof check", r)
	}
	accept.RequestID = domain.NewID()
	accept.ExpectedRevision = "2"
	r = s.AcceptResponse(ctx, admission.Task.ID, accept, proof)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "acceptance_conflict" {
		t.Fatal(r)
	}
	detail, e := s.Task(ctx, admission.Task.ID)
	if e != nil || detail.Acceptance == nil || detail.Task.Revision != "2" {
		t.Fatal(detail, e)
	}
}
func TestAttemptReferencesAndArtifactPrefixesAreChecked(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	admission, _ := admitted(t, s)
	id := domain.NewID()
	if _, e := s.RecordAttempt(ctx, admission.Attempt.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &id}); e == nil {
		t.Fatal("unregistered response accepted")
	}
	artifact := domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: id, AttemptID: admission.Attempt.ID, Kind: "stdout", ByteLength: "9007199254740993", Completeness: "collecting", CreatedAt: domain.Now()}, PrivateRef: "artifacts/" + id + ".bin", PhysicalIdentity: "physical"}
	if e := s.PutArtifact(ctx, artifact); e != nil {
		t.Fatal(e)
	}
	artifact.ByteLength = "9007199254740992"
	if e := s.PutArtifact(ctx, artifact); e == nil {
		t.Fatal("prefix moved backwards")
	}
	artifact.PrivateRef = "../arbitrary"
	if e := s.PutArtifact(ctx, artifact); e == nil {
		t.Fatal("arbitrary path accepted")
	}
	events, e := s.Events(ctx, admission.Attempt.ID, 1, "")
	if e != nil || events.NextCursor == nil {
		t.Fatal(events, e)
	}
	if _, e = s.Events(ctx, domain.NewID(), 1, *events.NextCursor); e == nil {
		t.Fatal("cursor accepted on other attempt")
	}
}
