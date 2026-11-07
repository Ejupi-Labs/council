package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

type graphExternalScopeCase struct {
	store    *Store
	owner    string
	producer domain.GraphDispatchView
	dispatch domain.GraphDispatchView
	attempt  domain.Attempt
}

// Real DAO transitions establish the final producer, review, external stage and
// claim. Typed controller artifacts qualify metadata joins only; no command or
// external publication is executed or asserted by these tests.
func graphExternalScopeFixture(t *testing.T, materialize bool) graphExternalScopeCase {
	t.Helper()
	ctx := context.Background()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	action := domain.ExternalActionInput{Action: "controlled declared metadata command", Destination: "metadata-only destination",
		AcceptanceBasis: "command_completion", Command: domain.ExternalCommandInput{Executable: executable, Arguments: []string{"--fixed-graph-external-metadata"}}}
	s, graph, root, rootProofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.FinalNodeKey = in.Nodes[0].Key
		in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
		in.Nodes[0].Workflow = &domain.WorkflowInput{Mode: "auto", Review: "required"}
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &action}
	})
	input, trigger := graphReadyNodeFacts(t, s, graph, root, rootProofs, owner, graph.Nodes[0])
	task, err := s.Task(ctx, graph.Nodes[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	options := input.Capture.Authority.OptionSnapshot
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &action}
	registry, invalid := workflowpolicy.Conditions(task.Task.OriginalText, task.Version, options, input.Capture.Authority.Checks)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instruction, conditions, err := workflowpolicy.ConditionDigests(task.Task.OriginalText, task.Version, registry, input.Capture.Authority.Checks)
	if err != nil {
		t.Fatal(err)
	}
	input.Capture.Authority.OptionSnapshot, input.Capture.Authority.Conditions = options, registry
	input.Capture.Authority.InstructionSHA256, input.Capture.Authority.Review = instruction, "required"
	input.Capture.Authority.ConditionsSHA256 = graphEffectiveWorkflowConditions(conditions, input.ConditionsBinding.ConditionsSHA256)
	producer, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, input)
	if err != nil {
		t.Fatal("The exact captured external scope was not admitted before author work", err)
	}
	mat := graphMaterializationForDispatch(t, s, producer, input, root, owner)
	author := graphOutputForDispatch(t, s, producer, mat, owner, true)
	candidate, err := s.Candidate(ctx, author.GraphOutput.Observation.Candidate.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta {
		t.Fatal("The final fixture did not retain an actual own-zero candidate", candidate, err)
	}
	w, err := s.Workflow(ctx, producer.Dispatch.TaskID, domain.WorkflowSelector{})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	closure := author.GraphOutput.CandidateClosure
	author.CandidateBinding = &domain.CandidateBinding{CandidateID: candidate.ID, RootSourceSnapshotID: candidate.RootSourceSnapshotID, SourceSnapshotID: candidate.SourceSnapshotID,
		ManifestArtifactID: closure.Manifest.ArtifactID, ManifestSHA256: closure.Manifest.SHA256, IndexArtifactID: closure.Index.ArtifactID, IndexSHA256: closure.Index.SHA256,
		ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
	author.ResponseBinding, author.GraphOutput.ResponseClosure = nil, nil
	author.GraphOutput.Observation.Response.State, author.GraphOutput.Observation.Response.ClosureRef = "incomplete", nil
	graphFinalOutputFacts(t, s, producer, &author)
	if settled, err := s.SettleWorkflowStep(ctx, producer.Dispatch.AttemptID, author); err != nil || settled.NextAction != "review" {
		t.Fatal("The captured final publisher did not require its actual review", settled, err)
	}
	reviewer, reviewFacts, reviewed := graphAcceptanceApprovedReview(t, s, producer, author, owner)
	if reviewed.NextAction != "external_delivery" || reviewed.NextOwner != "council" {
		t.Fatal("The actual approved final did not require its captured external command", reviewed)
	}
	stage := graphReviewAdmissionFacts(t, s, reviewer, author)
	stage.Plan.Role, stage.Plan.ExecutorKind = "external_delivery", "local_external"
	stage.Plan.LogicalKey, invalid = domain.WorkflowStageKey("external_delivery", 0, "", author.CandidateBinding.CandidateID)
	if invalid != nil {
		t.Fatal(invalid)
	}
	stage.Graph.SourceBasis.FinalVerificationRef = author.GraphOutput.Observation.Candidate.FinalVerificationRef
	stage.Graph.VerifiedArtifacts = reviewFacts.VerifiedArtifacts
	admitted, err := s.ContinueWorkflow(ctx, owner, reviewer.Dispatch.StepID, stage)
	if err != nil || admitted == nil || admitted.Attempt.ExecutorKind != "local_external" || admitted.Step.Role != "external_delivery" {
		t.Fatal("The real review did not admit its exact zero-unit publisher", admitted, err)
	}
	d, err := s.GraphDispatchForAttempt(ctx, admitted.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if materialize {
		// This helper performs the first real claim itself. Until receiving
		// artifact ownership is supported, a failure here is a setup boundary,
		// not qualification of the subsequent GO guard.
		receiving := graphMaterializationForDispatch(t, s, d.Public, domain.GraphNodeAdmissionFacts{InputBinding: stage.Graph.InputBinding, VerifiedArtifacts: stage.Graph.VerifiedArtifacts}, root, owner)
		if err = s.RecordGraphMaterialization(ctx, owner, admitted.Attempt.ID, receiving); err != nil {
			t.Fatal("The real publisher receiving observation could not be recorded", err)
		}
	} else if _, err = s.ClaimAttempt(ctx, admitted.Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	a, err := s.Attempt(ctx, admitted.Attempt.ID)
	if err != nil || a.Phase != "preparing" || a.LaunchIntent || a.Role != "external_delivery" || a.ExecutorKind != "local_external" {
		t.Fatal("The publisher has no exact preparing local claim", a, err)
	}
	budget, err := s.GraphBudget(ctx, graph.Graph.ID)
	if err != nil || budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("The local external stage spent a third model unit", budget, err)
	}
	return graphExternalScopeCase{store: s, owner: owner, producer: producer, dispatch: d.Public, attempt: a}
}

func TestGraphExternalReceivingArtifactsRequireOnlyExactPublisherScope(t *testing.T) {
	f := graphExternalScopeFixture(t, false)
	ctx := context.Background()
	if err := f.store.CheckGraphDispatchPreparation(ctx, f.owner, f.attempt.ID); err != nil {
		t.Fatal("Exact captured publisher cannot prepare its receiving workspace", err)
	}
	for _, kind := range []string{"graph_index_blob", "graph_index_blobs", "graph_operation_effects", "graph_materialization"} {
		if err := f.store.PutArtifact(ctx, graphArtifactMetadata(f.attempt.ID, kind)); err != nil {
			t.Error("The actual claimed final publisher cannot retain its receiving/effect artifact", kind, err)
		}
	}
	for _, kind := range []string{"graph_output_closure", "graph_final_verification", "graph_cumulative_delta", "graph_delivery"} {
		if err := f.store.PutArtifact(ctx, graphArtifactMetadata(f.attempt.ID, kind)); err == nil {
			t.Error("A publisher acquired candidate, cumulative or application authority", kind)
		}
	}
}

func TestGraphExternalScopeIsRepeatedBeforePreparationAndGO(t *testing.T) {
	for _, boundary := range []string{"preparation", "go"} {
		for _, change := range []string{"current", "removed_action", "changed_command"} {
			t.Run(boundary+"/"+change, func(t *testing.T) {
				f := graphExternalScopeFixture(t, boundary == "go")
				ctx := context.Background()
				if err := f.store.CheckGraphDispatchPreparation(ctx, f.owner, f.attempt.ID); err != nil {
					t.Fatal("The actual unchanged publisher cannot pass preparation", err)
				}
				var terminalBefore string
				if err := f.store.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", f.producer.Dispatch.AttemptID).Scan(&terminalBefore); err != nil {
					t.Fatal(err)
				}
				if change != "current" {
					// Deliberate private metadata corruption, after a valid real
					// admission. No public action can rewrite this captured scope.
					if err := f.store.executionWrite(ctx, func(tx *sql.Tx) error {
						w, err := f.store.workflowByID(ctx, tx, f.attempt.WorkflowID)
						if err != nil {
							return err
						}
						if change == "removed_action" {
							w.Authority.Delivery.ExternalAction = nil
						} else {
							w.Authority.Delivery.ExternalAction.Command.Arguments = append(w.Authority.Delivery.ExternalAction.Command.Arguments, "changed-after-admission")
						}
						if err = saveWorkflow(ctx, tx, w); err != nil {
							return err
						}
						// Keep both canonical copies internally consistent. Otherwise
						// workflowByID rejects only a storage representation mismatch,
						// before any semantic definition/scope guard can be tested.
						_, err = tx.ExecContext(ctx, "UPDATE workflows SET authority_data=? WHERE id=?", encode(w.Authority), w.ID)
						return err
					}); err != nil {
						t.Fatal(err)
					}
				}
				readback, readErr := f.store.Workflow(ctx, f.attempt.TaskID, domain.WorkflowSelector{WorkflowID: f.attempt.WorkflowID})
				if readErr != nil || readback.Workflow == nil {
					t.Fatal("The labelled scope counterexample is not a valid stored workflow representation", readback, readErr)
				}
				action := readback.Workflow.Authority.Delivery.ExternalAction
				if change == "removed_action" && action != nil || change != "removed_action" && action == nil {
					t.Fatal("The actual scope counterexample did not retain its selected mutation", change, action)
				}
				if change == "changed_command" && (len(action.Command.Arguments) == 0 || action.Command.Arguments[len(action.Command.Arguments)-1] != "changed-after-admission") {
					t.Fatal("The command mutation was not present in both stored authority copies", action)
				}
				var err error
				if boundary == "go" {
					_, err = f.store.RecordLaunchIntent(ctx, f.attempt.ID)
				} else {
					err = f.store.CheckGraphDispatchPreparation(ctx, f.owner, f.attempt.ID)
				}
				if change == "current" {
					if err != nil {
						t.Fatal("The exact current external scope was refused", err)
					}
				} else if err == nil {
					t.Fatal("Missing or changed captured external permission retained preparation/GO authority")
				}
				var terminalAfter string
				if err := f.store.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", f.producer.Dispatch.AttemptID).Scan(&terminalAfter); err != nil || terminalBefore != terminalAfter {
					t.Fatal("The scope counterexample rewrote immutable selected producer history", err)
				}
				actual, err := f.store.Attempt(ctx, f.attempt.ID)
				if err != nil || actual.LaunchIntent != (boundary == "go" && change == "current") {
					t.Fatal("The refused external scope created a launch intent", actual, err)
				}
				if err = f.store.checkAttemptClaim(ctx, f.store.db, f.owner, f.attempt.ID); err != nil {
					t.Fatal("The scope decision lost the actual publisher claim", err)
				}
			})
		}
	}
}
