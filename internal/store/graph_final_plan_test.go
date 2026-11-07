package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Registered typed facts model a receiving candidate with no creation delta and
// an independently verified root-to-final graph delta. They qualify SQL planning
// and proof installation, not filesystem composition or a model invocation.
func graphZeroOwnFinalPlanFixture(t *testing.T) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, string) {
	t.Helper()
	s, d, f, owner := graphOutputConfiguredFixture(t, true, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.FinalNodeKey = in.Nodes[0].Key
		in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
	})
	ctx := context.Background()
	candidate, err := s.Candidate(ctx, f.GraphOutput.Observation.Candidate.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta {
		t.Fatal("fixture did not retain an actual own-zero candidate", candidate, err)
	}
	w, err := s.Workflow(ctx, d.Dispatch.TaskID, domain.WorkflowSelector{})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	closure := f.GraphOutput.CandidateClosure
	f.CandidateBinding = &domain.CandidateBinding{CandidateID: candidate.ID, RootSourceSnapshotID: candidate.RootSourceSnapshotID, SourceSnapshotID: candidate.SourceSnapshotID, ManifestArtifactID: closure.Manifest.ArtifactID, ManifestSHA256: closure.Manifest.SHA256, IndexArtifactID: closure.Index.ArtifactID, IndexSHA256: closure.Index.SHA256, ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
	f.ResponseBinding = nil
	f.GraphOutput.ResponseClosure = nil
	f.GraphOutput.Observation.Response.State, f.GraphOutput.Observation.Response.ClosureRef = "incomplete", nil
	graphFinalOutputFacts(t, s, d, &f)
	return s, d, f, owner
}

func TestGraphFinalPlanInstallsFreshReviewProofWithoutChangingNodeDeltaFacts(t *testing.T) {
	s, d, f, _ := graphZeroOwnFinalPlanFixture(t)
	f.DirectProven = true // This node-local fact must not prove inherited files.
	f.CumulativeDirectProven = false
	w, err := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f)
	if err != nil || w.NextAction != "review" || w.NextOwner != "council" || w.HasTaskDelta || w.HasTaskIndexDelta {
		t.Fatal("validated cumulative graph result did not receive its actual next review", w, err)
	}
	step, err := s.WorkflowStep(context.Background(), d.Dispatch.StepID)
	key, invalid := domain.WorkflowStageKey("review", 0, "", f.CandidateBinding.CandidateID)
	if err != nil || invalid != nil || step.AdvanceProof == nil || step.AdvanceProof.NextLogicalKey != key || step.AdvanceProof.CandidateID == nil || *step.AdvanceProof.CandidateID != f.CandidateBinding.CandidateID || step.AdvanceProof.GraphBudgetAuthorityEpoch != d.Dispatch.BudgetAuthorityEpoch {
		t.Fatal("same-terminal graph plan did not install its exact fresh next-stage proof", step, err, invalid)
	}
	c, err := s.Candidate(context.Background(), f.CandidateBinding.CandidateID)
	if err != nil || c.HasDelta || c.HasIndexDelta {
		t.Fatal("cumulative planning relabelled node authorship", c, err)
	}
}

func TestGraphFinalPlanUsesSeparateFreshCumulativeDirectProof(t *testing.T) {
	s, d, f, _ := graphZeroOwnFinalPlanFixture(t)
	f.DirectProven = false
	f.CumulativeDirectProven = true
	w, err := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f)
	if err != nil || w.NextAction != "accept" || w.NextOwner != "council" || w.LatestAcceptanceID != nil || w.HasTaskDelta || w.HasTaskIndexDelta {
		t.Fatal("fresh cumulative proof was lost or itself invented acceptance", w, err)
	}
	step, err := s.WorkflowStep(context.Background(), d.Dispatch.StepID)
	key, invalid := domain.WorkflowStageKey("accept", 0, "", f.CandidateBinding.CandidateID)
	if err != nil || invalid != nil || step.AdvanceProof == nil || step.AdvanceProof.NextLogicalKey != key {
		t.Fatal("fresh cumulative direct proof did not install the guarded acceptance action", step, err, invalid)
	}
}

func TestGraphFinalPlanCannotUseCumulativeFlagWithoutSelectedFinalAssociation(t *testing.T) {
	s, d, f, _ := graphZeroOwnFinalPlanFixture(t)
	f.CumulativeDirectProven = true
	f.GraphOutput.FinalVerification = nil
	f.GraphOutput.Observation.Candidate.FinalVerificationRef = nil
	w, err := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f)
	if err != nil || w.NextAction != "candidate_change_required" || w.NextOwner != "user" {
		t.Fatal("unbound controller flag enabled graph final authority", w, err)
	}
	step, err := s.WorkflowStep(context.Background(), d.Dispatch.StepID)
	if err != nil || step.AdvanceProof != nil {
		t.Fatal("missing final association created automatic proof", step, err)
	}
}

func TestGraphFinalPlanCannotUseEmptyCumulativeDelta(t *testing.T) {
	s, d, f, _ := graphZeroOwnFinalPlanFixture(t)
	f.CumulativeDirectProven = true
	f.GraphOutput.FinalVerification.HasCumulativeWorkingDelta = false
	f.GraphOutput.FinalVerification.HasCumulativeIndexDelta = false
	graphFinalOutputReseal(t, s, d, &f)
	w, err := s.SettleWorkflowStep(context.Background(), d.Dispatch.AttemptID, f)
	if err != nil || w.NextAction != "candidate_change_required" || w.NextOwner != "user" {
		t.Fatal("empty graph delta manufactured a file-final result", w, err)
	}
}

func TestGraphFinalPlanReadsSupportingStageBasisWithoutFabricatedOutput(t *testing.T) {
	s, d, producer, owner := graphZeroOwnFinalPlanFixture(t)
	ctx := context.Background()
	if _, err := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, producer); err != nil {
		t.Fatal(err)
	}
	stage := graphReviewAdmissionFacts(t, s, d, producer)
	stage.Graph.SourceBasis.FinalVerificationRef = producer.GraphOutput.Observation.Candidate.FinalVerificationRef
	stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, producer.GraphOutput.VerifiedArtifacts...)
	admitted, err := s.ContinueWorkflow(ctx, owner, d.Dispatch.StepID, stage)
	if err != nil || admitted == nil {
		t.Fatal("actual final author did not permit its exact supporting review", admitted, err)
	}
	facts := domain.WorkflowStepFacts{CumulativeDirectProven: true, VerifiedArtifacts: stage.Graph.VerifiedArtifacts}
	var found bool
	err = s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, err := s.attempt(ctx, tx, admitted.Attempt.ID)
		if err != nil {
			return err
		}
		plan, err := s.graphFinalPlanFacts(ctx, tx, admitted.Workflow, a, facts)
		if err != nil {
			return err
		}
		found = plan != nil && plan.CumulativeDirectProven && plan.CurrentSubject.CandidateID == producer.CandidateBinding.CandidateID && plan.VerificationBindingRef == *stage.Graph.SourceBasis.FinalVerificationRef
		return nil
	})
	if err != nil || !found || facts.GraphOutput != nil {
		t.Fatal("supporting stage could not derive exact final scope without claiming producer output", found, err)
	}
}
