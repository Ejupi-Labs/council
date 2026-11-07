package store

import (
	"context"
	"os"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// Actual metadata transitions establish A's accepted file result and B's
// independent, byte-matching selected final plus required approved review.
// Controlled artifact/proof bodies qualify SQL authority only; no filesystem
// composition, model inspection or external publication is asserted here.
func graphExternalAncestryFixture(t *testing.T, kind string, materialize bool) graphExternalScopeCase {
	t.Helper()
	ctx := context.Background()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	action := domain.ExternalActionInput{Action: "controlled declared two-node publication", Destination: "metadata-only destination", AcceptanceBasis: "command_completion", Command: domain.ExternalCommandInput{Executable: executable, Arguments: []string{"--fixed-graph-external-ancestry-metadata"}}}
	s, graph, root, rootProofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.FinalNodeKey = in.Nodes[1].Key
		in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
		in.Nodes[1].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
		in.Nodes[1].Workflow = &domain.WorkflowInput{Mode: "auto", Review: "required"}
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &action}
		edges := []domain.GraphDependencyInput{{From: in.Nodes[0].Key, To: in.Nodes[1].Key, Kind: kind}}
		in.Dependencies = &edges
	})
	firstInput, firstTrigger := graphReadyNodeFacts(t, s, graph, root, rootProofs, owner, graph.Nodes[0])
	firstInput.Capture.Authority.Delivery.LocalMode = "review_only"
	firstInput.Capture.Authority.Delivery.ExternalAction = nil
	firstInput.Capture.Authority.Delivery.Source = "graph_scope"
	firstInput.Capture.Authority.Delivery.PreferenceRevision = ""
	first, err := s.AdmitGraphNode(ctx, owner, graph.Graph.ID, firstTrigger, firstInput)
	if err != nil {
		t.Fatal("the legitimate intermediate file author was not admitted", err)
	}
	firstMat := graphMaterializationForDispatch(t, s, first, firstInput, root, owner)
	firstFacts := graphOutputForDispatch(t, s, first, firstMat, owner, false)
	if _, err = s.SettleWorkflowStep(ctx, first.Dispatch.AttemptID, firstFacts); err != nil {
		t.Fatal(err)
	}
	firstReview, firstReviewFacts, reviewedFirst := graphAcceptanceApprovedReview(t, s, first, firstFacts, owner)
	if reviewedFirst.NextAction != "accept" || !reviewedFirst.HasTaskDelta {
		t.Fatal("the actual intermediate review did not qualify A's file result", reviewedFirst)
	}
	firstTask, err := s.Task(ctx, first.Dispatch.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	hash, invalid := domain.WorkflowAuthoritySHA256(reviewedFirst.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	request := domain.NewID()
	accepted := domain.WorkflowAcceptance{ID: domain.NewID(), WorkflowID: reviewedFirst.ID, TaskID: reviewedFirst.TaskID, TaskVersion: reviewedFirst.TaskVersion, CandidateBinding: *firstFacts.CandidateBinding, AuthoritySHA256: hash, ReviewUses: reviewedFirst.EvidenceUses, CheckUses: []domain.EvidenceUse{}, FindingResolutionIDs: []string{}, ExternalResultRefs: []string{}, DeliveryResult: "review_only", AcceptedByRequestID: request}
	if result := s.AcceptWorkflow(ctx, firstTask.Task.ID, domain.WorkflowAcceptanceFacts{RequestID: request, ExpectedTaskRevision: firstTask.Task.Revision, ExpectedWorkflowRevision: reviewedFirst.Revision, OwnerInstanceID: owner, VerifiedArtifacts: firstReviewFacts.VerifiedArtifacts, Acceptance: accepted}); result.Status != 200 {
		t.Fatal("the real reviewed predecessor did not receive its actual acceptance", result)
	}
	view, err := s.Graph(ctx, graph.Graph.ID)
	if err != nil || view.Nodes[0].AcceptedResult == nil || view.Nodes[0].AcceptedResult.AcceptanceID != accepted.ID {
		t.Fatal("A lacks an actual exact accepted file head", view, err)
	}
	begin := graphResponseCandidateDependency(t, s, view, root, rootProofs, firstFacts, firstReviewFacts, firstReview.Dispatch.AttemptID, owner)
	if kind == "context" {
		edge, err := domain.GraphEdgeID(view.Graph.ID, view.Graph.GraphVersion, view.Nodes[0].ID, view.Nodes[1].ID, kind)
		if err != nil {
			t.Fatal(err)
		}
		begin.Contributions[0].EdgeID, begin.Contributions[0].EdgeKind = edge, kind
		for i := range begin.Contributions[0].EvidenceRefs {
			begin.Contributions[0].EvidenceRefs[i].EdgeID = edge
			begin.Contributions[0].EvidenceRefs[i].Ref = "edge-" + edge + "-" + domain.Decimal(int64(i))
		}
	}
	op, err := s.BeginGraphInput(ctx, owner, view.Graph.ID, view.Nodes[1].ID, begin)
	if err != nil {
		t.Fatal("the actual accepted dependency could not begin B's input", err)
	}
	if _, err = s.ClaimAttempt(ctx, op.Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	completion := graphResponseCompletionForOperation(t, s, op, begin)
	if err = s.CompleteGraphInput(ctx, owner, op.Attempt.ID, completion); err != nil {
		t.Fatal(err)
	}
	input, trigger := graphNodeFactsForInput(t, s, op, completion)
	secondTask, err := s.Task(ctx, view.Nodes[1].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	options := input.Capture.Authority.OptionSnapshot
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "review_only", ExternalAction: &action}
	registry, invalid := workflowpolicy.Conditions(secondTask.Task.OriginalText, secondTask.Version, options, input.Capture.Authority.Checks)
	if invalid != nil {
		t.Fatal(invalid)
	}
	instruction, conditions, err := workflowpolicy.ConditionDigests(secondTask.Task.OriginalText, secondTask.Version, registry, input.Capture.Authority.Checks)
	if err != nil {
		t.Fatal(err)
	}
	input.Capture.Authority.OptionSnapshot, input.Capture.Authority.Conditions = options, registry
	input.Capture.Authority.InstructionSHA256, input.Capture.Authority.Review = instruction, "required"
	input.Capture.Authority.ConditionsSHA256 = graphEffectiveWorkflowConditions(conditions, input.ConditionsBinding.ConditionsSHA256)
	final, err := s.AdmitGraphNode(ctx, owner, view.Graph.ID, trigger, input)
	if err != nil {
		t.Fatal("the independently authoring final node was not admitted", err)
	}
	finalMat := graphMaterializationForDispatch(t, s, final, input, root, owner)
	finalFacts := graphOutputForDispatch(t, s, final, finalMat, owner, false)
	if finalFacts.CandidateBinding.CandidateID == firstFacts.CandidateBinding.CandidateID || finalFacts.CandidateBinding.ManifestSHA256 != firstFacts.CandidateBinding.ManifestSHA256 || finalFacts.CandidateBinding.IndexSHA256 != firstFacts.CandidateBinding.IndexSHA256 {
		t.Fatal("B did not independently retain matching complete bytes under its own actual candidate")
	}
	graphFinalOutputFacts(t, s, final, &finalFacts)
	if settled, err := s.SettleWorkflowStep(ctx, final.Dispatch.AttemptID, finalFacts); err != nil || settled.NextAction != "review" {
		t.Fatal("the current final did not require its actual independent review", settled, err)
	}
	finalReview, finalReviewFacts, reviewedFinal := graphAcceptanceApprovedReview(t, s, final, finalFacts, owner)
	if reviewedFinal.NextAction != "external_delivery" || reviewedFinal.NextOwner != "council" {
		t.Fatal("the actual reviewed final did not reach its declared external stage", reviewedFinal)
	}
	stage := graphReviewAdmissionFacts(t, s, finalReview, finalFacts)
	stage.Plan.Role, stage.Plan.ExecutorKind = "external_delivery", "local_external"
	stage.Plan.LogicalKey, invalid = domain.WorkflowStageKey("external_delivery", 0, "", finalFacts.CandidateBinding.CandidateID)
	if invalid != nil {
		t.Fatal(invalid)
	}
	stage.Graph.SourceBasis.FinalVerificationRef = finalFacts.GraphOutput.Observation.Candidate.FinalVerificationRef
	stage.Graph.VerifiedArtifacts = finalReviewFacts.VerifiedArtifacts
	admitted, err := s.ContinueWorkflow(ctx, owner, finalReview.Dispatch.StepID, stage)
	if err != nil || admitted == nil || admitted.Attempt.ExecutorKind != "local_external" {
		t.Fatal("the actual approved final could not register its declared publisher", admitted, err)
	}
	d, err := s.GraphDispatchForAttempt(ctx, admitted.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if materialize {
		receiving := graphMaterializationForDispatch(t, s, d.Public, domain.GraphNodeAdmissionFacts{InputBinding: stage.Graph.InputBinding, VerifiedArtifacts: stage.Graph.VerifiedArtifacts}, root, owner)
		if err = s.RecordGraphMaterialization(ctx, owner, admitted.Attempt.ID, receiving); err != nil {
			t.Fatal("the observed receiving receipt could not be retained before the live GO decision", err)
		}
	} else if _, err = s.ClaimAttempt(ctx, admitted.Attempt.ID, owner); err != nil {
		t.Fatal(err)
	}
	a, err := s.Attempt(ctx, admitted.Attempt.ID)
	if err != nil || a.Phase != "preparing" || a.LaunchIntent || len(d.Public.Dispatch.ContributorVector) != 1 || d.Public.Dispatch.ContributorVector[0].AcceptanceID != accepted.ID || d.Public.Dispatch.ContributorVector[0].CandidateBinding == nil {
		t.Fatal("the actual external claim lost its accepted file dependency or preparing state", a, d, err)
	}
	budget, err := s.GraphBudget(ctx, view.Graph.ID)
	if err != nil || budget.Budget.ConsumedAdmissions != "4" {
		t.Fatal("two actual author/review pairs or the zero-unit publisher changed shared accounting", budget, err)
	}
	return graphExternalScopeCase{store: s, owner: owner, producer: final, dispatch: d.Public, attempt: a}
}

func TestGraphExternalPublicationRequiresFileEdgeAncestry(t *testing.T) {
	for _, boundary := range []string{"preparation", "go"} {
		for _, kind := range []string{"files", "context"} {
			t.Run(boundary+"/"+kind, func(t *testing.T) {
				f := graphExternalAncestryFixture(t, kind, boundary == "go")
				ctx := context.Background()
				var err error
				if boundary == "go" {
					_, err = f.store.RecordLaunchIntent(ctx, f.attempt.ID)
				} else {
					err = f.store.CheckGraphDispatchPreparation(ctx, f.owner, f.attempt.ID)
				}
				if kind == "files" && err != nil {
					t.Fatal("an actual file-authority path could not publish its reviewed current final", err)
				}
				if kind == "context" && err == nil {
					t.Fatal("context-only reachability plus copied matching bytes granted file publication authority")
				}
				a, readErr := f.store.Attempt(ctx, f.attempt.ID)
				if readErr != nil || a.LaunchIntent != (boundary == "go" && kind == "files") {
					t.Fatal("refused ancestry created a launch intent", a, readErr)
				}
				if err = f.store.checkAttemptClaim(ctx, f.store.db, f.owner, f.attempt.ID); err != nil {
					t.Fatal("the ancestry decision lost the actual retained external claim", err)
				}
			})
		}
	}
}
