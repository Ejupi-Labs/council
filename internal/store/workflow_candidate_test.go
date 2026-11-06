package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
	"testing"
)

// Metadata-only DAO proof; actual raw-index/manifest agreement is separately
// exercised through owned workspace handoff fixtures, not invented here.
func workflowTestArtifact(t *testing.T, s *Store, attemptID, kind string) string {
	t.Helper()
	id := domain.NewID()
	hash := strings.Repeat("a", 64)
	e := s.PutArtifact(context.Background(), domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: id, AttemptID: attemptID, Kind: kind, ByteLength: "1", Closed: true, Completeness: "complete", SHA256: &hash, CreatedAt: domain.Now()}, PrivateRef: "artifacts/" + id + ".bin", PhysicalIdentity: "labelled-dao-fixture"})
	if e != nil {
		t.Fatal(e)
	}
	return id
}
func TestWorkflowCandidateRequiresDistinctOwnedCurrentIndex(t *testing.T) {
	for _, which := range []string{"missing", "original_source", "foreign_attempt", "wrong_root", "current_same_attempt"} {
		t.Run(which, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			owner, in, facts := workflowAdmissionFixture(t, s)
			r := s.CreateTaskWithWorkflow(ctx, owner, in, facts)
			if r.Status != 201 {
				t.Fatal(r)
			}
			admission := r.Envelope.Data.(domain.Admission)
			if _, e := s.ClaimAttempt(ctx, admission.Attempt.ID, owner); e != nil {
				t.Fatal(e)
			}
			manifest := workflowTestArtifact(t, s, admission.Attempt.ID, "source_manifest")
			originalIndex := workflowTestArtifact(t, s, admission.Attempt.ID, "index_snapshot")
			source := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: admission.Attempt.ID, ManifestArtifactID: manifest, IndexArtifactID: originalIndex, ManifestSHA256: strings.Repeat("a", 64), ProjectIdentity: facts.Capture.Authority.Delivery.ProjectIdentity, SelectedRelativePath: ".", CreatedAt: domain.Now()}
			if e := s.PutSourceSnapshot(ctx, source); e != nil {
				t.Fatal(e)
			}
			current := ""
			switch which {
			case "original_source":
				current = originalIndex
			case "current_same_attempt", "wrong_root":
				current = workflowTestArtifact(t, s, admission.Attempt.ID, "index_snapshot")
			case "foreign_attempt":
				p := openFixture(t, s, domain.NewID())
				other := s.CreateTask(ctx, owner, taskInput(p.ID))
				if other.Status != 201 {
					t.Fatal(other)
				}
				current = workflowTestArtifact(t, s, other.Envelope.Data.(domain.Admission).Attempt.ID, "index_snapshot")
			}
			candidate := domain.Candidate{ID: domain.NewID(), WorkflowID: admission.Workflow.ID, CreatedByStepID: admission.Step.ID, ConditionsSHA256: admission.Workflow.Authority.ConditionsSHA256, Completeness: "complete", AttemptID: admission.Attempt.ID, SourceSnapshotID: source.ID, ManifestArtifactID: workflowTestArtifact(t, s, admission.Attempt.ID, "candidate_manifest"), DeltaArtifactID: workflowTestArtifact(t, s, admission.Attempt.ID, "candidate_delta"), IndexArtifactID: current, WorkspaceRef: "workspaces/" + source.ID, HasDelta: true, CreatedAt: domain.Now()}
			if which == "wrong_root" {
				candidate.RootSourceSnapshotID = domain.NewID()
			}
			e := s.PutCandidate(ctx, candidate)
			if which == "current_same_attempt" {
				if e != nil {
					t.Fatal("valid current index refused", e)
				}
				saved, e := s.Candidate(ctx, candidate.ID)
				if e != nil || saved.IndexArtifactID != current || saved.RootSourceSnapshotID != source.ID {
					t.Fatal("candidate lost current-index association", saved, e)
				}
			} else {
				if e == nil {
					t.Fatal("candidate accepted missing/stale/foreign index", which)
				}
				var count int
				if e = s.db.QueryRow("SELECT count(*) FROM candidates WHERE id=?", candidate.ID).Scan(&count); e != nil || count != 0 {
					t.Fatal("rejected candidate was partially retained", count, e)
				}
			}
		})
	}
}
