package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Actual DAO transitions with controlled typed proof metadata. These tests do
// not claim an OS child, physical member verification or model inspection.
func graphFinalStageFixture(t *testing.T) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, domain.WorkflowStepAdmissionFacts, domain.GraphArtifactRef, string) {
	t.Helper()
	s, producer, author, owner := graphZeroOwnFinalPlanFixture(t)
	ctx := context.Background()
	unselected, proof := graphPayload(t, s, producer.Dispatch.AttemptID, "graph_final_verification", []byte(encode(author.GraphOutput.FinalVerification)))
	if unselected == *author.GraphOutput.Observation.Candidate.FinalVerificationRef || unselected.SHA256 != author.GraphOutput.Observation.Candidate.FinalVerificationRef.SHA256 {
		t.Fatal("fixture did not retain equal complete bytes under another exact artifact association")
	}
	if _, err := s.SettleWorkflowStep(ctx, producer.Dispatch.AttemptID, author); err != nil {
		t.Fatal(err)
	}
	stage := graphReviewAdmissionFacts(t, s, producer, author)
	stage.Graph.SourceBasis.FinalVerificationRef = author.GraphOutput.Observation.Candidate.FinalVerificationRef
	stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, author.GraphOutput.VerifiedArtifacts...)
	stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, proof)
	return s, producer, author, stage, unselected, owner
}

func TestGraphStageFinalRefAdmissionRequiresExactFreshSelectedBasis(t *testing.T) {
	for _, name := range []string{"current_final", "missing_ref", "unselected_equal_bytes", "missing_final_proof", "missing_delta_proof", "missing_closure_proof", "missing_original_proof"} {
		t.Run(name, func(t *testing.T) {
			s, producer, author, stage, unselected, owner := graphFinalStageFixture(t)
			ctx := context.Background()
			var remove string
			switch name {
			case "missing_ref":
				stage.Graph.SourceBasis.FinalVerificationRef = nil
			case "unselected_equal_bytes":
				stage.Graph.SourceBasis.FinalVerificationRef = &unselected
			case "missing_final_proof":
				remove = author.GraphOutput.Observation.Candidate.FinalVerificationRef.ArtifactID
			case "missing_delta_proof":
				remove = author.GraphOutput.FinalVerification.CumulativeDeltaRef.ArtifactID
			case "missing_closure_proof":
				remove = author.GraphOutput.Observation.Candidate.ClosureRef.ArtifactID
			case "missing_original_proof":
				remove = author.GraphOutput.FinalVerification.OriginalAccess[0].ArtifactRef.ArtifactID
			}
			if remove != "" {
				proofs := []domain.VerifiedResponse{}
				removed := false
				for _, proof := range stage.Graph.VerifiedArtifacts {
					if proof.ArtifactID == remove {
						removed = true
					} else {
						proofs = append(proofs, proof)
					}
				}
				if !removed {
					t.Fatal("negative fixture did not remove an actual fresh proof", remove)
				}
				stage.Graph.VerifiedArtifacts = proofs
			}
			admitted, err := s.ContinueWorkflow(ctx, owner, producer.Dispatch.StepID, stage)
			if name == "current_final" {
				if err != nil || admitted == nil || admitted.Attempt.InputCandidateID == nil || *admitted.Attempt.InputCandidateID != author.CandidateBinding.CandidateID {
					t.Fatal("exact selected final could not admit its captured review", admitted, err)
				}
				return
			}
			if err == nil || admitted != nil {
				t.Fatal("missing or unselected cumulative authority admitted a model stage", admitted, err)
			}
			budget, err := s.GraphBudget(ctx, producer.Dispatch.GraphID)
			var steps int
			if err != nil || budget.Budget.ConsumedAdmissions != "1" || s.db.QueryRow("SELECT count(*) FROM workflow_steps WHERE workflow_id=?", producer.Dispatch.WorkflowID).Scan(&steps) != nil || steps != 1 {
				t.Fatal("rejected final scope spent or registered native work", budget, steps, err)
			}
		})
	}
}

func TestGraphStageFinalRefIsRepeatedBeforePreparationAndGO(t *testing.T) {
	for _, boundary := range []string{"preparation", "go"} {
		t.Run(boundary, func(t *testing.T) {
			s, producer, _, stage, unselected, owner := graphFinalStageFixture(t)
			ctx := context.Background()
			admitted, err := s.ContinueWorkflow(ctx, owner, producer.Dispatch.StepID, stage)
			if err != nil || admitted == nil {
				t.Fatal(admitted, err)
			}
			if boundary == "go" {
				d, err := s.GraphDispatchForAttempt(ctx, admitted.Attempt.ID)
				if err != nil {
					t.Fatal(err)
				}
				root, err := s.GraphRoot(ctx, producer.Dispatch.GraphID)
				if err != nil {
					t.Fatal(err)
				}
				mat := graphMaterializationForDispatch(t, s, d.Public, domain.GraphNodeAdmissionFacts{InputBinding: stage.Graph.InputBinding, VerifiedArtifacts: stage.Graph.VerifiedArtifacts}, root, owner)
				if err = s.RecordGraphMaterialization(ctx, owner, admitted.Attempt.ID, mat); err != nil {
					t.Fatal(err)
				}
			} else if _, err = s.ClaimAttempt(ctx, admitted.Attempt.ID, owner); err != nil {
				t.Fatal(err)
			}
			if err = s.CheckGraphDispatchPreparation(ctx, owner, admitted.Attempt.ID); err != nil {
				t.Fatal("current exact final failed before the injected cache corruption", err)
			}
			var terminalBefore string
			if err = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", producer.Dispatch.AttemptID).Scan(&terminalBefore); err != nil {
				t.Fatal(err)
			}
			// Deliberate private-cache corruption: change only the selected
			// association to another already closed, equal-byte final artifact.
			// No public action can rewrite a settled producer or its terminal.
			if err = s.executionWrite(ctx, func(tx *sql.Tx) error {
				current, err := s.graphDispatchForAttempt(ctx, tx, producer.Dispatch.AttemptID)
				if err != nil {
					return err
				}
				current.Public.OutputObservation.Candidate.FinalVerificationRef = &unselected
				return saveGraphDispatch(ctx, tx, current)
			}); err != nil {
				t.Fatal(err)
			}
			if boundary == "go" {
				_, err = s.RecordLaunchIntent(ctx, admitted.Attempt.ID)
			} else {
				err = s.CheckGraphDispatchPreparation(ctx, owner, admitted.Attempt.ID)
			}
			if err == nil {
				t.Fatal("changed selected-final association retained preparation or GO authority")
			}
			var terminalAfter string
			if err = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", producer.Dispatch.AttemptID).Scan(&terminalAfter); err != nil || terminalAfter != terminalBefore {
				t.Fatal("cache counterexample rewrote immutable producer history", err)
			}
			a, err := s.Attempt(ctx, admitted.Attempt.ID)
			if err != nil || a.LaunchIntent {
				t.Fatal("refused scope created a launch intent", a, err)
			}
			if err = s.checkAttemptClaim(ctx, s.db, owner, a.ID); err != nil {
				t.Fatal("refused GO lost the exact receiving claim", err)
			}
		})
	}
}
