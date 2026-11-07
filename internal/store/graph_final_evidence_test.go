package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// These are actual admission/claim/record/settlement transactions with typed
// controlled proofs, not an OS process or model-inspection qualification.
func TestGraphFinalEvidenceRequiresExactOriginDispatchBasis(t *testing.T) {
	for _, scope := range []string{"current_final", "node_only", "unselected_ref"} {
		t.Run(scope, func(t *testing.T) {
			s, producer, author, owner := graphZeroOwnFinalPlanFixture(t)
			ctx := context.Background()
			// Equal complete bytes in another closed artifact are not the exact
			// association selected in the producer's same-terminal observation.
			unselected, unselectedProof := graphPayload(t, s, producer.Dispatch.AttemptID, "graph_final_verification", []byte(encode(author.GraphOutput.FinalVerification)))
			if _, err := s.SettleWorkflowStep(ctx, producer.Dispatch.AttemptID, author); err != nil {
				t.Fatal(err)
			}
			stage := graphReviewAdmissionFacts(t, s, producer, author)
			stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, author.GraphOutput.VerifiedArtifacts...)
			stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, unselectedProof)
			stage.Graph.SourceBasis.FinalVerificationRef = author.GraphOutput.Observation.Candidate.FinalVerificationRef
			admitted, err := s.ContinueWorkflow(ctx, owner, producer.Dispatch.StepID, stage)
			if err != nil || admitted == nil {
				t.Fatal("registered current final could not admit its controlled supporting stage", admitted, err)
			}
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
			if _, err = s.RecordLaunchIntent(ctx, admitted.Attempt.ID); err != nil {
				t.Fatal(err)
			}
			if scope != "current_final" {
				// The current registrar refuses these scopes before admission.
				// Inject labelled historical/cache-corruption metadata only after
				// a valid exact admission and GO, so the terminal-time filter is
				// still independently exercised without weakening runtime guards.
				if err = s.executionWrite(ctx, func(tx *sql.Tx) error {
					origin, err := s.graphDispatchForAttempt(ctx, tx, admitted.Attempt.ID)
					if err != nil {
						return err
					}
					origin.Public.Dispatch.SourceBasis.FinalVerificationRef = nil
					if scope == "unselected_ref" {
						origin.Public.Dispatch.SourceBasis.FinalVerificationRef = &unselected
					}
					return saveGraphDispatch(ctx, tx, origin)
				}); err != nil {
					t.Fatal(err)
				}
			}
			raw, proof := graphPayload(t, s, admitted.Attempt.ID, "review_result", []byte("controlled complete approved review"))
			if _, err = s.RecordAttempt(ctx, admitted.Attempt.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &raw.ArtifactID}); err != nil {
				t.Fatal(err)
			}
			conditions := []string{}
			for _, condition := range admitted.Workflow.Authority.Conditions {
				conditions = append(conditions, condition.Key)
			}
			app := domain.Applicability{ScopeKind: "whole_candidate", Paths: []string{}, ConditionKeys: conditions, DefinitionSHA256: strings.Repeat("b", 64), BasisSHA256: strings.Repeat("c", 64)}
			supplied := []domain.ReviewInspection{{Ref: "candidate-manifest", Scope: "whole_candidate", Paths: []string{}}}
			report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: domain.ReviewSubject{Kind: "candidate", ID: author.CandidateBinding.CandidateID, SHA256: author.CandidateBinding.ManifestSHA256}, ConditionsSHA256: admitted.Workflow.Authority.ConditionsSHA256, Outcome: "approved", InspectedEvidence: supplied, Findings: []domain.ReviewFinding{}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
			record := domain.ReviewRecord{ID: domain.NewID(), WorkflowID: admitted.Workflow.ID, StepID: admitted.Step.ID, AttemptID: admitted.Attempt.ID, CandidateBinding: author.CandidateBinding, ReviewContract: domain.ReviewContract, ResultArtifactID: raw.ArtifactID, ResultSHA256: raw.SHA256, Applicability: app, Report: report, SuppliedEvidence: supplied, Findings: []domain.WorkflowFinding{}, Outcome: "approved", CreatedAt: domain.Now()}
			task, err := s.Task(ctx, admitted.Attempt.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			exit := 0
			proofs := append(append([]domain.VerifiedResponse{}, stage.Graph.VerifiedArtifacts...), mat.VerifiedArtifacts...)
			proofs = append(proofs, proof)
			facts := domain.WorkflowStepFacts{StepID: admitted.Step.ID, OwnerInstanceID: owner, TaskRevision: task.Task.Revision, WorkflowRevision: admitted.Workflow.Revision, CandidateBinding: author.CandidateBinding, Applicability: &app, Report: &record, VerifiedArtifacts: proofs, Settlement: domain.Settlement{Outcome: "succeeded", RootTerminal: true, InputComplete: true, ExitCode: &exit, UsableOutput: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"}}
			w, err := s.SettleWorkflowStep(ctx, admitted.Attempt.ID, facts)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if scope == "current_final" {
				want = 1
			}
			if len(w.EvidenceUses) != want {
				t.Fatal("node-only or unselected final evidence remained current cumulative authority", w.EvidenceUses)
			}
			var retained string
			if err = s.db.QueryRow("SELECT data FROM workflow_reviews WHERE id=?", record.ID).Scan(&retained); err != nil || retained != encode(record) {
				t.Fatal("scope filtering rewrote or removed actual review history", retained, err)
			}
		})
	}
}
