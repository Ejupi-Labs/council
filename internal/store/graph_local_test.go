package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphLocalFixture(t *testing.T, configure ...func(*domain.GraphStartInput)) (*Store, domain.GraphAdmission, domain.GraphCreateFacts, string) {
	t.Helper()
	s := testStore(t)
	in, f, owner := graphTestStart(t, s)
	for _, apply := range configure {
		apply(&in)
	}
	r := s.CreateGraph(context.Background(), owner, in, f)
	if r.Status != 201 {
		t.Fatal(r)
	}
	return s, r.Envelope.Data.(domain.GraphAdmission), f, owner
}
func TestGraphLocalOperationRequiresCurrentExactClaim(t *testing.T) {
	s, admission, _, owner := graphLocalFixture(t)
	ctx := context.Background()
	id := admission.PreparationAttemptID
	op, e := s.GraphInputOperationForAttempt(ctx, owner, id)
	if e != nil || op.Attempt.ID != id || op.ID != *admission.Graph.RootInputID || op.Kind != "root_basis" || op.Control.OperationAttemptID != id || op.Definition.GraphID != admission.Graph.ID || op.InputRecord.Public.Summary.ID != op.ID {
		t.Fatal("exact persisted root operation tuple unavailable", op, e)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, id); e == nil {
		t.Fatal("local I/O was permitted before exact claim")
	}
	if _, e = s.ClaimAttempt(ctx, id, owner); e != nil {
		t.Fatal(e)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, id); e != nil {
		t.Fatal("owned current basis may perform bounded local work", e)
	}
	if e = s.CheckGraphLocalOperation(ctx, domain.NewID(), id); e == nil {
		t.Fatal("another owner borrowed a local operation")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = s.CheckGraphLocalOperation(cancelled, owner, id); !errors.Is(e, context.Canceled) {
		t.Fatal("known pre-I/O cancellation lost identity", e)
	}
	if r := s.StopAttempt(ctx, id, domain.NewID()); r.Status != 202 {
		t.Fatal(r)
	}
	if e = s.CheckGraphLocalOperation(ctx, owner, id); e == nil {
		t.Fatal("accepted Stop permitted another local effect")
	}
	if e = s.CheckAttemptClaim(ctx, owner, id); e != nil {
		t.Fatal("Stop acknowledgement released actual worker ownership", e)
	}
}

// Metadata/proof tuples here are fixed DAO facts, not actual filesystem verification.
// Engine/workspace tests qualify the artifact bytes and physical identities.
func graphLocalMetadata(t *testing.T, s *Store, attemptID, kind string) domain.ArtifactRecord {
	t.Helper()
	record := domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: domain.NewID(), AttemptID: attemptID, Kind: kind, ByteLength: "12", Closed: true, Completeness: "complete", SHA256: ptr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), CreatedAt: domain.Now()}, PhysicalIdentity: "typed-fake-artifact"}
	record.PrivateRef = "artifacts/" + record.ID + ".bin"
	e := s.executionWrite(context.Background(), func(tx *sql.Tx) error {
		sequence, e := nextSequence(context.Background(), tx, "artifacts")
		if e != nil {
			return e
		}
		_, e = tx.Exec("INSERT INTO artifacts VALUES(?,?,?,?,?,?,?)", sequence, record.ID, attemptID, record.PrivateRef, record.PhysicalIdentity, 12, encode(record.ArtifactMetadata))
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	return record
}
func TestGraphLocalCompletionRequiresFreshClosureAndSettlesOnce(t *testing.T) {
	s, admission, f, owner := graphLocalFixture(t)
	ctx := context.Background()
	attemptID := admission.PreparationAttemptID
	if _, e := s.ClaimAttempt(ctx, attemptID, owner); e != nil {
		t.Fatal(e)
	}
	manifest := graphLocalMetadata(t, s, attemptID, "source_manifest")
	index := graphLocalMetadata(t, s, attemptID, "index_snapshot")
	blobs := graphLocalMetadata(t, s, attemptID, "graph_index_blobs")
	ref := func(a domain.ArtifactRecord) domain.GraphArtifactRef {
		return domain.GraphArtifactRef{ArtifactID: a.ID, ProducerAttemptID: a.AttemptID, SHA256: *a.SHA256, ByteLength: a.ByteLength}
	}
	source := domain.SourceSnapshot{ID: domain.NewID(), AttemptID: attemptID, ManifestArtifactID: manifest.ID, IndexArtifactID: index.ID, ManifestSHA256: *manifest.SHA256, BaseCommit: f.BaseCommit, Branch: f.Branch, ProjectIdentity: f.ProjectIdentity, SelectedRelativePath: f.SelectedRelativePath, CreatedAt: domain.Now()}
	binding := domain.GraphRootBinding{GraphID: admission.Graph.ID, RootTaskID: admission.Graph.RootTaskID, OperationAttemptID: attemptID, SourceSnapshotID: source.ID, Manifest: ref(manifest), Index: ref(index), StagedBlobs: ref(blobs), ProjectID: admission.Graph.ProjectID, ProjectIdentity: f.ProjectIdentity, RepositoryIdentity: f.RepositoryIdentity, SelectedRelativePath: f.SelectedRelativePath, BaseCommit: f.BaseCommit, Branch: f.Branch}
	var e error
	binding.BasisSHA256, e = domain.GraphRootDigest(binding)
	if e != nil {
		t.Fatal(e)
	}
	facts := domain.GraphInputCompletionFacts{RootBinding: &binding, SourceSnapshot: &source, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, State: "ready"}
	if e = s.CompleteGraphInput(ctx, owner, attemptID, facts); e == nil {
		t.Fatal("unverified registered metadata became a current basis")
	}
	a, e := s.Attempt(ctx, attemptID)
	if e != nil || a.Phase != "preparing" {
		t.Fatal("rejected proof altered ownership", a, e)
	}
	for _, a := range []domain.ArtifactRecord{manifest, index, blobs} {
		facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, domain.VerifiedResponse{ArtifactID: a.ID, SHA256: *a.SHA256, PhysicalIdentity: a.PhysicalIdentity, ByteLength: a.ByteLength})
	}
	s.AfterCommit = func() error { return errors.New("fixed completion acknowledgement lost") }
	e = s.CompleteGraphInput(ctx, owner, attemptID, facts)
	var de *domain.Error
	if !errors.As(e, &de) || de.Code != "write_uncertain" {
		t.Fatal("committed completion lost acknowledgement uncertainty", e)
	}
	s.AfterCommit = nil
	a, e = s.Attempt(ctx, attemptID)
	if e != nil || a.Outcome == nil || *a.Outcome != "succeeded" || a.LaunchIntent || a.SourceSnapshotID == nil || *a.SourceSnapshotID != source.ID {
		t.Fatal("actual no-model local completion was not retained", a, e)
	}
	terminal := encode(a)
	if e = s.CompleteGraphInput(ctx, owner, attemptID, facts); e != nil {
		t.Fatal("same operation did not reconcile committed completion", e)
	}
	record, e := s.GraphInputRecord(ctx, admission.Graph.ID, *admission.Graph.RootInputID)
	if e != nil || record.Public.Summary.State != "ready" || record.PrivateRoot == nil || !reflect.DeepEqual(*record.PrivateRoot, binding) {
		t.Fatal("exact sealed private/public basis lost", record, e)
	}
	var terminals int
	if e = s.db.QueryRow("SELECT count(*) FROM attempt_terminals WHERE attempt_id=?", attemptID).Scan(&terminals); e != nil || terminals != 1 {
		t.Fatal("completion appended a terminal twice", terminals, e, terminal)
	}
	budget, e := s.GraphBudget(ctx, admission.Graph.ID)
	if e != nil || budget.Budget.ConsumedAdmissions != "0" || budget.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal("root local work spent/opened native authority", budget, e)
	}
}
