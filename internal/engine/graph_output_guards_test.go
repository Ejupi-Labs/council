package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are actual registered-producer physical guard cases. They create no
// process and never manufacture native success, review authority or acceptance.
func graphGuardCandidateRequest(t *testing.T, f graphOutputProducerCase) graphStepOutputRequest {
	t.Helper()
	ctx := context.Background()
	e, a, snapshot := f.engine, f.attempt, f.materialized.Snapshot
	candidate, err := e.workspace.Candidate(ctx, snapshot)
	if err != nil || candidate == nil {
		t.Fatal(err)
	}
	index, err := e.workspace.RetainIndex(ctx, a.ID, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(e.lock.Root(), snapshot.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.db.PutCandidate(ctx, domain.Candidate{ID: candidate.ID, AttemptID: a.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: candidate.ManifestArtifactID,
		DeltaArtifactID: candidate.DeltaArtifactID, IndexArtifactID: index.ID, WorkspaceRef: filepath.ToSlash(relative), HasDelta: len(candidate.Changes) != 0, HasIndexDelta: false,
		Completeness: "complete", Verification: "pending", Integration: "not_performed", CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil {
		t.Fatal(view, err)
	}
	subject, err := e.candidateWorkflowSubject(ctx, *view.Workflow, candidate.ID)
	if err != nil || subject.Binding == nil {
		t.Fatal(err)
	}
	return graphStepOutputRequest{Attempt: a, Snapshot: snapshot, CandidateBinding: subject.Binding}
}

func graphGuardArtifactNames(t *testing.T, e *Engine) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.lock.Root(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestGraphStepOutputGuardRejectsDifferentClosedIndexWithoutWritingClosure(t *testing.T) {
	f := graphOutputAdmittedProducerFixture(t)
	e := f.engine
	ctx := context.Background()
	request := graphGuardCandidateRequest(t, f)
	// Retain the same actual managed index again. Its valid closed content/hash
	// cannot substitute for the exact artifact acknowledged by this candidate.
	other, err := e.workspace.RetainIndex(ctx, f.attempt.ID, f.materialized.Snapshot)
	if err != nil || other.SHA256 == nil || other.ID == request.CandidateBinding.IndexArtifactID || *other.SHA256 != request.CandidateBinding.IndexSHA256 {
		t.Fatal("Distinct actual unchanged index fixture precondition", other, err)
	}
	wrong := *request.CandidateBinding
	wrong.IndexArtifactID, wrong.IndexSHA256 = other.ID, *other.SHA256
	request.CandidateBinding = &wrong
	beforeArtifacts, err := e.db.Artifacts(ctx, f.attempt.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	beforeNames := graphGuardArtifactNames(t, e)
	facts, err := e.captureGraphStepOutput(ctx, request)
	if err == nil || facts == nil || facts.Observation.Candidate == nil || facts.Observation.Candidate.State == "complete" || facts.Observation.Candidate.ClosureRef != nil || facts.CandidateClosure != nil {
		t.Fatal("A different valid closed artifact became an exact selected candidate binding", facts, err)
	}
	afterArtifacts, err := e.db.Artifacts(ctx, f.attempt.ID, 100, "")
	if err != nil || !reflect.DeepEqual(beforeArtifacts, afterArtifacts) || !reflect.DeepEqual(beforeNames, graphGuardArtifactNames(t, e)) {
		t.Fatal("Wrong exact binding reached artifact creation", err)
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, f.attempt.ID)
	if err != nil || dispatch.Public.OutputObservation != nil {
		t.Fatal("Rejected binding changed terminal output association", dispatch, err)
	}
}

func TestGraphStepOutputGuardRejectsAlreadySettledProducerBeforeArtifactIO(t *testing.T) {
	f := graphOutputAdmittedProducerFixture(t)
	e := f.engine
	ctx := context.Background()
	request := graphGuardCandidateRequest(t, f)
	_, err := e.db.SettleWorkflowStep(ctx, f.attempt.ID, domain.WorkflowStepFacts{StepID: f.attempt.StepID, OwnerInstanceID: e.config.OwnerInstanceID,
		Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: true, EvidenceStatus: "incomplete", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"},
			Failure: primary("fixture_no_process", "This actual claimed fixture ran no process; its terminal state is deliberately nonqualifying.")}})
	if err != nil {
		t.Fatal("Actual nonqualifying producer settlement failed", err)
	}
	settled, err := e.db.Attempt(ctx, f.attempt.ID)
	if err != nil || settled.Phase != "settled" || settled.Outcome == nil || *settled.Outcome == "succeeded" || settled.LaunchIntent || settled.StartedAt != nil {
		t.Fatal("Settled no-process fixture precondition", settled, err)
	}
	beforeArtifacts, err := e.db.Artifacts(ctx, f.attempt.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	beforeNames := graphGuardArtifactNames(t, e)
	beforeDispatch, err := e.db.GraphDispatchForAttempt(ctx, f.attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if facts, err := e.captureGraphStepOutput(ctx, request); err == nil || facts != nil {
		t.Fatal("A settled producer was reopened for graph output capture", facts, err)
	}
	afterArtifacts, err := e.db.Artifacts(ctx, f.attempt.ID, 100, "")
	if err != nil || !reflect.DeepEqual(beforeArtifacts, afterArtifacts) || !reflect.DeepEqual(beforeNames, graphGuardArtifactNames(t, e)) {
		t.Fatal("Settled producer attempted new registered or unregistered artifact I/O", err)
	}
	afterDispatch, err := e.db.GraphDispatchForAttempt(ctx, f.attempt.ID)
	if err != nil || !reflect.DeepEqual(beforeDispatch, afterDispatch) {
		t.Fatal("Settled immutable output association changed", err)
	}
}
