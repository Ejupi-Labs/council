package store

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// These controller facts qualify only DAO metadata/ordering. Real filesystem
// freshness and owned native settlement are independent engine/workspace gates.
func workflowAuthorSettlementFixture(t *testing.T, s *Store, configure ...func(*domain.StartTaskInput, *domain.WorkflowAdmissionFacts)) (domain.Admission, string, domain.WorkflowStepFacts) {
	t.Helper()
	ctx := context.Background()
	owner, in, facts := workflowAdmissionFixture(t, s)
	in.OriginalText = `Replace "old" with "new" in docs/note.md only`
	for _, apply := range configure {
		apply(&in, &facts)
	}
	p, e := s.Policy(ctx)
	if e != nil {
		t.Fatal(e)
	}
	v := resolvedVersion("", p, in)
	definitions, gaps, validationErr := workflowpolicy.Checks(workflowpolicy.CheckInput{Original: in.OriginalText, Version: v, Options: *in.Workflow})
	if validationErr != nil || len(gaps) != 0 || len(definitions) != 1 {
		t.Fatal(definitions, gaps, validationErr)
	}
	registry, validationErr := workflowpolicy.Conditions(in.OriginalText, v, *in.Workflow, definitions)
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	instruction, conditions, e := workflowpolicy.ConditionDigests(in.OriginalText, v, registry, definitions)
	if e != nil {
		t.Fatal(e)
	}
	facts.Capture.Authority.Checks = definitions
	facts.Capture.Authority.Conditions = registry
	facts.Capture.Authority.InstructionSHA256 = instruction
	facts.Capture.Authority.ConditionsSHA256 = conditions
	r := s.CreateTaskWithWorkflow(ctx, owner, in, facts)
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	launchFixture(t, s, a, owner)
	current, e := s.Attempt(ctx, a.Attempt.ID)
	if e != nil || current.SourceSnapshotID == nil {
		t.Fatal(current, e)
	}
	source, e := s.SourceSnapshot(ctx, *current.SourceSnapshotID)
	if e != nil {
		t.Fatal(e)
	}
	manifest := workflowTestArtifact(t, s, a.Attempt.ID, "candidate_manifest")
	delta := workflowTestArtifact(t, s, a.Attempt.ID, "candidate_delta")
	index := workflowTestArtifact(t, s, a.Attempt.ID, "index_snapshot")
	c := domain.Candidate{ID: domain.NewID(), WorkflowID: a.Workflow.ID, AttemptID: a.Attempt.ID, SourceSnapshotID: source.ID, ManifestArtifactID: manifest, DeltaArtifactID: delta, IndexArtifactID: index, WorkspaceRef: "workspaces/" + source.ID, HasDelta: true, Completeness: "complete", CreatedAt: domain.Now()}
	if e = s.PutCandidate(ctx, c); e != nil {
		t.Fatal(e)
	}
	c, e = s.Candidate(ctx, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	proofs := []domain.VerifiedResponse{}
	page, e := s.Artifacts(ctx, a.Attempt.ID, 100, "")
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range page.Items {
		record, e := s.Artifact(ctx, m.ID)
		if e != nil {
			t.Fatal(e)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: m.ID, SHA256: *m.SHA256, PhysicalIdentity: record.PhysicalIdentity, ByteLength: m.ByteLength})
	}
	binding := domain.CandidateBinding{CandidateID: c.ID, RootSourceSnapshotID: source.ID, SourceSnapshotID: source.ID, ManifestArtifactID: manifest, ManifestSHA256: strings.Repeat("a", 64), IndexArtifactID: index, IndexSHA256: strings.Repeat("a", 64), ConditionsSHA256: a.Workflow.Authority.ConditionsSHA256}
	exit := 0
	return a, owner, domain.WorkflowStepFacts{HasTaskDelta: true, DirectProven: true, VerifiedArtifacts: proofs, StepID: a.Step.ID, OwnerInstanceID: owner, TaskRevision: "1", WorkflowRevision: "1", CandidateBinding: &binding, Settlement: domain.Settlement{Outcome: "succeeded", Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete", RootTerminal: true, InputComplete: true, ExitCode: &exit, UsableOutput: true}}
}

func TestWorkflowSettlementAtomicProofAndLocalZeroUnitContinuation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, owner, facts := workflowAuthorSettlementFixture(t, s)
	w, e := s.SettleWorkflowStep(ctx, a.Attempt.ID, facts)
	if e != nil {
		t.Fatal("atomic workflow settlement unavailable", e)
	}
	if w.SelectedCandidateID == nil || *w.SelectedCandidateID != facts.CandidateBinding.CandidateID || w.State == "completed" {
		t.Fatal("settlement lost subject or falsely accepted author result", w)
	}
	step, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil || step.Status != "succeeded" || step.AdvanceProof == nil {
		t.Fatal("terminal/proof not atomic", step, e)
	}
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: step.AdvanceProof.NextLogicalKey, Cycle: 0, DefinitionID: &w.Authority.Checks[0].ID, InputCandidateID: w.SelectedCandidateID, PredecessorStepID: &step.ID}
	requestFacts := domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: "1", ExpectedWorkflowRevision: w.Revision}
	next, e := s.ContinueWorkflow(ctx, owner, step.ID, requestFacts)
	if e != nil || next == nil {
		t.Fatal("fresh authorized local continuation unavailable", next, e)
	}
	if next.Attempt.ExecutorKind != "local_check" || next.Attempt.ConnectionID != "local-check" || next.Attempt.ResolvedTarget != nil || next.Attempt.RoutingDecisionID != nil || next.Budget.ConsumedAdmissions != "1" {
		t.Fatal("local check invented native target/debit", next)
	}
	repeated, e := s.ContinueWorkflow(ctx, owner, step.ID, requestFacts)
	if e != nil || repeated != nil {
		t.Fatal("predecessor proof admitted twice", repeated, e)
	}
	if _, attempts, steps, _ := workflowCounts(t, s); attempts != 2 || steps != 2 {
		t.Fatal("continuation duplicated work", attempts, steps)
	}
}

func TestWorkflowSettlementMissingFreshProofRetainsUnsettledEvidence(t *testing.T) {
	s := testStore(t)
	a, _, facts := workflowAuthorSettlementFixture(t, s)
	facts.VerifiedArtifacts = facts.VerifiedArtifacts[:1]
	if _, e := s.SettleWorkflowStep(context.Background(), a.Attempt.ID, facts); e == nil {
		t.Fatal("partial fresh-proof set qualified full candidate")
	}
	current, e := s.Attempt(context.Background(), a.Attempt.ID)
	if e != nil || current.Phase == "settled" {
		t.Fatal("failed settlement partially committed terminal", current, e)
	}
	var count int
	if e = s.db.QueryRow("SELECT count(*) FROM attempt_terminals WHERE attempt_id=?", a.Attempt.ID).Scan(&count); e != nil || count != 0 {
		t.Fatal("missing proofs committed terminal", count, e)
	}
}

func TestWorkflowStopAfterSettlementSuppressesOnlyFreshNextStep(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	a, owner, facts := workflowAuthorSettlementFixture(t, s)
	w, e := s.SettleWorkflowStep(ctx, a.Attempt.ID, facts)
	if e != nil {
		t.Fatal(e)
	}
	step, e := s.WorkflowStep(ctx, a.Step.ID)
	if e != nil || step.AdvanceProof == nil {
		t.Fatal(step, e)
	}
	r := s.StopAttempt(ctx, a.Attempt.ID, domain.NewID())
	if r.Status != 200 {
		t.Fatal(r)
	}
	current, e := s.Task(ctx, a.Task.ID)
	if e != nil || current.Task.Revision == "1" {
		t.Fatal("terminal Stop did not suppress fresh authority", current, e)
	}
	plan := domain.WorkflowStagePlan{Role: "check", ExecutorKind: "local_check", LogicalKey: step.AdvanceProof.NextLogicalKey, DefinitionID: &w.Authority.Checks[0].ID, InputCandidateID: w.SelectedCandidateID, PredecessorStepID: &step.ID}
	next, e := s.ContinueWorkflow(ctx, owner, step.ID, domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: "1", ExpectedWorkflowRevision: w.Revision})
	if e != nil || next != nil {
		t.Fatal("Stop re-armed predecessor", next, e)
	}
	old, e := s.Attempt(ctx, a.Attempt.ID)
	if e != nil || old.Outcome == nil || *old.Outcome != "succeeded" || old.Stop != nil {
		t.Fatal("terminal Stop rewrote author success", old, e)
	}
	if _, attempts, _, _ := workflowCounts(t, s); attempts != 1 {
		t.Fatal("Stop admitted successor", attempts)
	}
}
