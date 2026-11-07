package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// Captured program identities and artifact bytes below are controlled DAO
// facts. No program is executed and no filesystem-integrity claim is made.
func graphLocalCheckStageFixture(t *testing.T) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, domain.WorkflowStepAdmissionFacts, string) {
	t.Helper()
	declarations := []domain.CheckDeclaration{{Key: "verify", Executable: "go", Arguments: []string{"test", "./..."}}}
	s, graph, root, proofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.Nodes[0].Workflow = &domain.WorkflowInput{Mode: "auto", Review: "auto", Checks: &declarations}
	})
	ctx := context.Background()
	facts, trigger := graphReadyNodeFacts(t, s, graph, root, proofs, owner, graph.Nodes[0])
	definition, invalid := workflowpolicy.CaptureCheck(declarations[0], domain.NewID(), workflowpolicy.CheckProgram{Name: "go", Executable: filepath.Join(t.TempDir(), "go.exe"), PhysicalIdentity: "typed-controlled-go-check-program"})
	if invalid != nil {
		t.Fatal(invalid)
	}
	task, err := s.Task(ctx, graph.Nodes[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	capture := *facts.Capture
	capture.Authority.Checks = []domain.CheckDefinition{definition}
	registry, invalid := workflowpolicy.Conditions(task.Task.OriginalText, task.Version, capture.Authority.OptionSnapshot, capture.Authority.Checks)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instruction, nodeConditions, err := workflowpolicy.ConditionDigests(task.Task.OriginalText, task.Version, registry, capture.Authority.Checks)
	if err != nil {
		t.Fatal(err)
	}
	capture.Authority.Conditions, capture.Authority.InstructionSHA256 = registry, instruction
	capture.Authority.ConditionsSHA256 = graphEffectiveWorkflowConditions(nodeConditions, facts.ConditionsBinding.ConditionsSHA256)
	facts.Capture = &capture
	initial, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, trigger, facts)
	if err != nil || initial.Dispatch.ID == "" {
		t.Fatal(initial, err)
	}
	materialization := graphMaterializationForDispatch(t, s, initial, facts, root, owner)
	author := graphOutputForDispatch(t, s, initial, materialization, owner, false)
	if _, err = s.SettleWorkflowStep(ctx, initial.Dispatch.AttemptID, author); err != nil {
		t.Fatal(err)
	}
	stage := graphReviewAdmissionFacts(t, s, initial, author)
	key, invalid := domain.WorkflowStageKey("check", 0, definition.ID, author.CandidateBinding.CandidateID)
	if invalid != nil {
		t.Fatal(invalid)
	}
	stage.Plan.Role, stage.Plan.ExecutorKind, stage.Plan.LogicalKey, stage.Plan.DefinitionID = "check", "local_check", key, &definition.ID
	return s, initial, author, stage, owner
}

func graphLocalCheckClaimFixture(t *testing.T) (*Store, domain.GraphDispatchView, domain.WorkflowStepFacts, string) {
	t.Helper()
	s, initial, author, stage, owner := graphLocalCheckStageFixture(t)
	ctx := context.Background()
	admitted, err := s.ContinueWorkflow(ctx, owner, initial.Dispatch.StepID, stage)
	if err != nil || admitted == nil {
		t.Fatal("actual captured required check did not receive its local execution", admitted, err)
	}
	if _, err = s.ClaimAttempt(ctx, admitted.Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	dispatch, err := s.GraphDispatchForAttempt(ctx, admitted.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, dispatch.Public, author, owner
}

func TestGraphLocalCheckUnknownContextEffectsRetainClaimDespiteKnownNoProcessCleanup(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "known_no_effects", false: "unknown_context_effects"}[known], func(t *testing.T) {
			s, dispatch, author, owner := graphLocalCheckClaimFixture(t)
			ctx := context.Background()
			effects, proof := graphPayload(t, s, dispatch.Dispatch.AttemptID, "graph_operation_effects", []byte(`{"controlled":"retained context copy prefix"}`))
			evidence := "incomplete"
			if !known {
				evidence = "uncertain"
			}
			f := domain.WorkflowStepFacts{StepID: dispatch.Dispatch.StepID, OwnerInstanceID: owner, CandidateBinding: author.CandidateBinding, VerifiedArtifacts: append(author.VerifiedArtifacts, proof), Settlement: domain.Settlement{Outcome: "blocked", NoProcessCreated: true, EffectsKnown: known, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: evidence, Failure: &domain.PrimaryFailure{Code: "controlled_context_unavailable", Message: "The typed context-copy preparation could not finish."}}}
			if _, err := s.SettleWorkflowStep(ctx, dispatch.Dispatch.AttemptID, f); err != nil {
				t.Fatal(err)
			}
			a, err := s.Attempt(ctx, dispatch.Dispatch.AttemptID)
			if err != nil || a.Cleanup.State != "verified" || a.LaunchIntent || a.StartedAt != nil || a.Outcome == nil || *a.Outcome == "succeeded" || a.EvidenceStatus != evidence {
				t.Fatal("context uncertainty invented a process or qualifying outcome", a, err)
			}
			gate, err := s.Gate(ctx)
			wantClaims := 0
			wantGate := "open"
			if !known {
				wantClaims, wantGate = 1, "fenced"
			}
			if err != nil || gate.State != wantGate || len(gate.ActiveClaims) != wantClaims || !known && gate.ActiveClaims[0].AttemptID != a.ID {
				t.Fatal("known process cleanup lost exact unknown-context ownership", gate, err)
			}
			step, err := s.WorkflowStep(ctx, a.StepID)
			if err != nil || step.AdvanceProof != nil {
				t.Fatal("incomplete context created automatic workflow authority", step, err)
			}
			retained, err := s.Artifact(ctx, effects.ArtifactID)
			if err != nil || retained.AttemptID != a.ID || retained.SHA256 == nil || *retained.SHA256 != effects.SHA256 {
				t.Fatal("context prefix evidence disappeared", retained, err)
			}
			budget, err := s.GraphBudget(ctx, dispatch.Dispatch.GraphID)
			if err != nil || budget.Budget.ConsumedAdmissions != "1" {
				t.Fatal("local context preparation spent or refunded a model unit", budget, err)
			}
		})
	}
}
