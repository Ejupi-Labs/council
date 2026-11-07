package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Actual Store transitions with controlled closed metadata proofs; the typed GO
// observation below does not claim an OS child or provider invocation.
func graphReviewResumeFixture(t *testing.T, recovered bool, replacementWindow ...int) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, domain.GraphDetail, domain.WorkflowStepAdmissionFacts, string) {
	t.Helper()
	s, d, author, owner := graphOutputSettlementFixture(t)
	ctx := context.Background()
	if _, err := s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, author); err != nil {
		t.Fatal(err)
	}
	view, err := s.Graph(ctx, d.Dispatch.GraphID)
	if err != nil {
		t.Fatal(err)
	}
	grant := graphGrantInput(view.Graph)
	grant.AdditionalAdmissions = "1"
	if len(replacementWindow) != 0 {
		grant.NewWindowSeconds = &replacementWindow[0]
	}
	if result := s.AmendGraphBudget(ctx, owner, view.Graph.ID, grant); result.Status != 200 {
		t.Fatal(result)
	}
	if recovered {
		owner = domain.NewID()
		if err = s.RecoverOwner(ctx, owner); err != nil {
			t.Fatal(err)
		}
	}
	view, err = s.Graph(ctx, d.Dispatch.GraphID)
	if err != nil || view.Graph.BudgetAuthorityEpoch != "2" || view.DispatchAuthority == nil || view.DispatchAuthority.State != "suspended" {
		t.Fatal("fixture did not retain the old unarmed registration after grant/recovery", view, err)
	}
	stage := graphReviewAdmissionFacts(t, s, d, author)
	stage.Graph.Control.OwnerInstanceID = owner
	return s, d, author, view, stage, owner
}

func TestGraphAdvanceResumesCurrentReviewUnderFreshAuthority(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(map[bool]string{false: "current_owner_after_grant", true: "new_owner_after_grant"}[recovered], func(t *testing.T) {
			s, prior, author, before, stage, owner := graphReviewResumeFixture(t, recovered)
			ctx := context.Background()
			anchorBefore, err := s.GraphDispatchForAttempt(ctx, prior.Dispatch.AttemptID)
			if err != nil {
				t.Fatal(err)
			}
			var terminalBefore string
			if err = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", prior.Dispatch.AttemptID).Scan(&terminalBefore); err != nil {
				t.Fatal(err)
			}
			if automatic, err := s.ContinueWorkflow(ctx, owner, prior.Dispatch.StepID, stage); err != nil || automatic != nil {
				t.Fatal("grant/recovery rearmed the original automatic callback", automatic, err)
			}
			in := graphAdvanceInput(before)
			result := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{Stages: []domain.WorkflowStepAdmissionFacts{stage}})
			if result.Status != 200 {
				t.Fatal("fresh explicit current review resume unavailable", result)
			}
			data, err := decode[domain.GraphAdvanceResult](encode(result.Envelope.Data))
			if err != nil || len(data.Dispatches) != 1 || len(data.PreparationAttemptIDs) != 0 || len(data.ReboundNodes) != 0 {
				t.Fatal(data, err)
			}
			d := data.Dispatches[0]
			if d.Dispatch.OwnerInstanceID != owner || d.Dispatch.Role != "review" || d.Dispatch.AttemptID == prior.Dispatch.AttemptID || d.Dispatch.WorkflowID != prior.Dispatch.WorkflowID || d.Dispatch.NodeAdmissionID != prior.Dispatch.NodeAdmissionID || d.Dispatch.InputID != prior.Dispatch.InputID || d.Dispatch.SourceBasis.BasisKind != "node_subject" || !reflect.DeepEqual(d.Dispatch.SourceBasis.CandidateBinding, author.CandidateBinding) || d.Dispatch.BudgetAuthorityEpoch != "2" || d.Dispatch.FreshTriggerID != in.RequestID {
				t.Fatal("explicit resume replaced the author or used a different admitted source", d)
			}
			after, err := s.Graph(ctx, before.Graph.ID)
			if err != nil || after.Nodes[0].TaskVersion != before.Nodes[0].TaskVersion || after.Budget.Budget.ConsumedAdmissions != "2" || after.Budget.Budget.AutomaticAdmissions != "0" || after.Budget.Budget.GrantedAdmissions != before.Budget.Budget.GrantedAdmissions || !reflect.DeepEqual(after.Budget.Budget.CurrentWindow, before.Budget.Budget.CurrentWindow) {
				t.Fatal("resume refilled/reopened the shared window or did not spend one manual unit", after, err)
			}
			stored, err := s.GraphDispatchForAttempt(ctx, d.Dispatch.AttemptID)
			if err != nil {
				t.Fatal(err)
			}
			if recovered {
				private, err := decode[struct {
					RequestID *string `json:"ownerResumeRequestId"`
				}](encode(stored))
				if err != nil || private.RequestID == nil || *private.RequestID != in.RequestID {
					t.Fatal("new-owner dispatch lost its committed explicit owner authorization", private, err)
				}
			}
			root, err := s.GraphRoot(ctx, before.Graph.ID)
			if err != nil {
				t.Fatal(err)
			}
			materialization := graphMaterializationForDispatch(t, s, d, domain.GraphNodeAdmissionFacts{InputBinding: stage.Graph.InputBinding, VerifiedArtifacts: stage.Graph.VerifiedArtifacts}, root, owner)
			if err = s.CheckGraphDispatchPreparation(ctx, owner, d.Dispatch.AttemptID); err != nil {
				t.Fatal("healthy explicit owner resume could not pass its exact claimed pre-I/O guard", err)
			}
			if err = s.RecordGraphMaterialization(ctx, owner, d.Dispatch.AttemptID, materialization); err != nil {
				t.Fatal(err)
			}
			if _, err = s.RecordLaunchIntent(ctx, d.Dispatch.AttemptID); err != nil {
				t.Fatal(err)
			}
			at := domain.Now()
			if _, err = s.RecordAttempt(ctx, d.Dispatch.AttemptID, domain.AttemptUpdate{Kind: "worker_started", Phase: "running", StartedAt: &at}); err != nil {
				t.Fatal("resumed actual-dispatch GO could not join its original node anchor", err)
			}
			anchorAfter, err := s.GraphDispatchForAttempt(ctx, prior.Dispatch.AttemptID)
			if err != nil || !reflect.DeepEqual(anchorAfter.Public.Dispatch, anchorBefore.Public.Dispatch) || anchorAfter.KnownFirstGOAttemptID == nil || *anchorAfter.KnownFirstGOAttemptID != d.Dispatch.AttemptID || anchorAfter.KnownFirstGOAt == nil {
				t.Fatal("resume rewrote the original anchor or lost paired first-GO provenance", anchorAfter, err)
			}
			var terminalAfter string
			if err = s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", prior.Dispatch.AttemptID).Scan(&terminalAfter); err != nil || terminalAfter != terminalBefore {
				t.Fatal("resume rewrote the original author terminal", err)
			}
			if automatic, err := s.ContinueWorkflow(ctx, owner, prior.Dispatch.StepID, stage); err != nil || automatic != nil {
				t.Fatal("old author completion replayed after the explicit review admission", automatic, err)
			}
			expected, err := decode[domain.Envelope](encode(result.Envelope))
			if err != nil {
				t.Fatal(err)
			}
			replay := s.AdvanceGraph(ctx, owner, before.Graph.ID, in, domain.GraphAdvanceFacts{})
			if replay.Status != 200 || !reflect.DeepEqual(expected, replay.Envelope) {
				t.Fatal("explicit resume replay created new stage intent or changed its result", replay)
			}
			badExit := 17
			if _, err = s.SettleWorkflowStep(ctx, d.Dispatch.AttemptID, domain.WorkflowStepFacts{StepID: d.Dispatch.StepID, OwnerInstanceID: owner, Settlement: domain.Settlement{Outcome: "failed", RootTerminal: true, ExitCode: &badExit, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "controlled_review_end", Message: "The typed test review ended without a qualifying report."}}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGraphAdvanceResumeRejectsChangedOrIncompleteStageAuthority(t *testing.T) {
	for _, change := range []string{"wrong_owner", "wrong_epoch", "wrong_node_version", "wrong_input", "wrong_subject", "missing_manifest_proof", "accepted_stop"} {
		t.Run(change, func(t *testing.T) {
			s, prior, author, view, stage, owner := graphReviewResumeFixture(t, true)
			in := graphAdvanceInput(view)
			switch change {
			case "wrong_owner":
				stage.Graph.Control.OwnerInstanceID = domain.NewID()
			case "wrong_epoch":
				in.ExpectedBudgetAuthorityEpoch = "1"
			case "wrong_node_version":
				wrong := "2"
				stage.Graph.Control.TaskVersion = &wrong
			case "wrong_input":
				wrong := domain.NewID()
				stage.Graph.Control.InputID = &wrong
			case "wrong_subject":
				wrong := *stage.Graph.SourceBasis.CandidateBinding
				wrong.CandidateID = domain.NewID()
				stage.Graph.SourceBasis.CandidateBinding = &wrong
			case "missing_manifest_proof":
				proofs := []domain.VerifiedResponse{}
				for _, proof := range stage.Graph.VerifiedArtifacts {
					if proof.ArtifactID != author.CandidateBinding.ManifestArtifactID {
						proofs = append(proofs, proof)
					}
				}
				stage.Graph.VerifiedArtifacts = proofs
			case "accepted_stop":
				stop := s.StopGraphNode(context.Background(), owner, view.Graph.ID, view.Nodes[0].Key, domain.GraphNodeStopInput{RequestID: domain.NewID(), ExpectedGraphControlRevision: view.Graph.ControlRevision, NodeID: view.Nodes[0].ID, ExpectedNodeControlRevision: view.Nodes[0].ControlRevision})
				if stop.Status != 202 {
					t.Fatal(stop)
				}
			}
			var attempts int
			if err := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts); err != nil {
				t.Fatal(err)
			}
			result := s.AdvanceGraph(context.Background(), owner, view.Graph.ID, in, domain.GraphAdvanceFacts{Stages: []domain.WorkflowStepAdmissionFacts{stage}})
			if result.Envelope.Error == nil || result.Envelope.Receipt != nil {
				t.Fatal("unsafe explicit stage authority committed", change, result)
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count); err != nil || count != attempts {
				t.Fatal("refused resume allocated work", attempts, count, err)
			}
			stored, err := s.GraphDispatchForAttempt(context.Background(), prior.Dispatch.AttemptID)
			if err != nil || stored.Public.OutputObservation == nil {
				t.Fatal("refused resume lost retained producer output", stored, err)
			}
		})
	}
}
