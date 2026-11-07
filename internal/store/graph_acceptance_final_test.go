package store

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
)

// These helpers qualify actual SQL admission/evidence/delivery/acceptance joins
// with explicitly controlled artifact proofs. Physical delivery is a separate
// Engine/workspace qualification; no model or filesystem success is fabricated.
func graphAcceptanceApprovedReview(t *testing.T, s *Store, producer domain.GraphDispatchView, author domain.WorkflowStepFacts, owner string) (domain.GraphDispatchView, domain.WorkflowStepFacts, domain.Workflow) {
	t.Helper()
	ctx := context.Background()
	stage := graphReviewAdmissionFacts(t, s, producer, author)
	stage.Graph.VerifiedArtifacts = append(stage.Graph.VerifiedArtifacts, author.GraphOutput.VerifiedArtifacts...)
	if author.GraphOutput.Observation.Candidate != nil {
		stage.Graph.SourceBasis.FinalVerificationRef = author.GraphOutput.Observation.Candidate.FinalVerificationRef
	}
	admitted, err := s.ContinueWorkflow(ctx, owner, producer.Dispatch.StepID, stage)
	if err != nil || admitted == nil || admitted.Step.Role != "review" {
		t.Fatal("registered candidate did not admit its actual required review", admitted, err)
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
	raw, proof := graphPayload(t, s, admitted.Attempt.ID, "review_result", []byte("controlled complete approved cumulative review"))
	if _, err = s.RecordAttempt(ctx, admitted.Attempt.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &raw.ArtifactID}); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, condition := range admitted.Workflow.Authority.Conditions {
		keys = append(keys, condition.Key)
	}
	app := domain.Applicability{ScopeKind: "whole_candidate", Paths: []string{}, ConditionKeys: keys, DefinitionSHA256: strings.Repeat("b", 64), BasisSHA256: strings.Repeat("c", 64)}
	supplied := []domain.ReviewInspection{{Ref: "candidate-manifest", Scope: "whole_candidate", Paths: []string{}}}
	report := domain.ReviewReport{SchemaVersion: domain.ReviewContract, Subject: domain.ReviewSubject{Kind: "candidate", ID: author.CandidateBinding.CandidateID, SHA256: author.CandidateBinding.ManifestSHA256}, ConditionsSHA256: admitted.Workflow.Authority.ConditionsSHA256, Outcome: "approved", InspectedEvidence: supplied, Findings: []domain.ReviewFinding{}, Gaps: []domain.ReviewGap{}, ResolvedFindingIDs: []string{}}
	record := domain.ReviewRecord{ID: domain.NewID(), WorkflowID: admitted.Workflow.ID, StepID: admitted.Step.ID, AttemptID: admitted.Attempt.ID, CandidateBinding: author.CandidateBinding, ReviewContract: domain.ReviewContract, ResultArtifactID: raw.ArtifactID, ResultSHA256: raw.SHA256, Applicability: app, Report: report, SuppliedEvidence: supplied, Findings: []domain.WorkflowFinding{}, Outcome: "approved", CreatedAt: domain.Now()}
	task, err := s.Task(ctx, admitted.Attempt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	proofs := append(append([]domain.VerifiedResponse{}, stage.Graph.VerifiedArtifacts...), mat.VerifiedArtifacts...)
	proofs = append(proofs, proof)
	exit := 0
	facts := domain.WorkflowStepFacts{StepID: admitted.Step.ID, OwnerInstanceID: owner, TaskRevision: task.Task.Revision, WorkflowRevision: admitted.Workflow.Revision, CandidateBinding: author.CandidateBinding, Applicability: &app, Report: &record, VerifiedArtifacts: proofs, Settlement: domain.Settlement{Outcome: "succeeded", RootTerminal: true, InputComplete: true, ExitCode: &exit, UsableOutput: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "windows_job"}, EvidenceStatus: "complete"}}
	w, err := s.SettleWorkflowStep(ctx, admitted.Attempt.ID, facts)
	if err != nil || len(w.EvidenceUses) != 1 || w.EvidenceUses[0].EvidenceID != record.ID || w.EvidenceUses[0].Kind != "review" {
		t.Fatal("actual approved review was not current qualifying evidence", w, err)
	}
	return d.Public, facts, w
}

type graphFinalAcceptanceCase struct {
	store       *Store
	owner       string
	producer    domain.GraphDispatchView
	author      domain.WorkflowStepFacts
	review      domain.GraphDispatchView
	reviewFacts domain.WorkflowStepFacts
	delivery    *graphDeliveryDAOCase
	proofs      []domain.VerifiedResponse
	unselected  *domain.GraphArtifactRef
}

func graphFinalAcceptanceFixture(t *testing.T, delivery string) graphFinalAcceptanceCase {
	t.Helper()
	ctx := context.Background()
	s, graph, root, rootProofs, owner := graphReadyBasis(t, func(in *domain.GraphStartInput) {
		in.Nodes = in.Nodes[:1]
		in.FinalNodeKey = in.Nodes[0].Key
		in.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"}
		in.Nodes[0].Acceptance = &domain.AcceptanceInput{Deliverable: "candidate"}
		in.Nodes[0].Workflow = &domain.WorkflowInput{Mode: "auto", Review: "required"}
	})
	input, trigger := graphReadyNodeFacts(t, s, graph, root, rootProofs, owner, graph.Nodes[0])
	task, err := s.Task(ctx, graph.Nodes[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	// Capture apply/required-review before initial admission, including its exact
	// registry and effective graph digest. Saved policy is never reread later.
	options := input.Capture.Authority.OptionSnapshot
	options.Delivery = &domain.WorkflowDeliveryInput{LocalMode: "apply_verified_local"}
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
		t.Fatal(err)
	}
	mat := graphMaterializationForDispatch(t, s, producer, input, root, owner)
	author := graphOutputForDispatch(t, s, producer, mat, owner, true)
	candidate, err := s.Candidate(ctx, author.GraphOutput.Observation.Candidate.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta {
		t.Fatal("fixture did not retain a truthful own-zero final candidate", candidate, err)
	}
	w, err := s.Workflow(ctx, producer.Dispatch.TaskID, domain.WorkflowSelector{})
	if err != nil || w.Workflow == nil {
		t.Fatal(w, err)
	}
	closure := author.GraphOutput.CandidateClosure
	author.CandidateBinding = &domain.CandidateBinding{CandidateID: candidate.ID, RootSourceSnapshotID: candidate.RootSourceSnapshotID, SourceSnapshotID: candidate.SourceSnapshotID, ManifestArtifactID: closure.Manifest.ArtifactID, ManifestSHA256: closure.Manifest.SHA256, IndexArtifactID: closure.Index.ArtifactID, IndexSHA256: closure.Index.SHA256, ConditionsSHA256: w.Workflow.Authority.ConditionsSHA256}
	author.ResponseBinding, author.GraphOutput.ResponseClosure = nil, nil
	author.GraphOutput.Observation.Response.State, author.GraphOutput.Observation.Response.ClosureRef = "incomplete", nil
	graphFinalOutputFacts(t, s, producer, &author)
	if settled, err := s.SettleWorkflowStep(ctx, producer.Dispatch.AttemptID, author); err != nil || settled.NextAction != "review" || settled.HasTaskDelta || settled.HasTaskIndexDelta {
		t.Fatal("own-zero cumulative result did not require its captured independent review", settled, err)
	}
	review, reviewFacts, reviewed := graphAcceptanceApprovedReview(t, s, producer, author, owner)
	if reviewed.NextAction != "apply" || reviewed.NextOwner != "council" {
		t.Fatal("approved current final did not require captured local delivery", reviewed)
	}
	f := graphFinalAcceptanceCase{store: s, owner: owner, producer: producer, author: author, review: review, reviewFacts: reviewFacts, proofs: reviewFacts.VerifiedArtifacts}
	if delivery == "missing" {
		return f
	}
	stage := graphReviewAdmissionFacts(t, s, review, author)
	stage.Plan.Role, stage.Plan.ExecutorKind = "local_delivery", "local_application"
	stage.Plan.LogicalKey, invalid = domain.WorkflowStageKey("local_delivery", 0, "", author.CandidateBinding.CandidateID)
	if invalid != nil {
		t.Fatal(invalid)
	}
	stage.Graph.SourceBasis.FinalVerificationRef = author.GraphOutput.Observation.Candidate.FinalVerificationRef
	stage.Graph.VerifiedArtifacts = reviewFacts.VerifiedArtifacts
	admitted, err := s.ContinueWorkflow(ctx, owner, review.Dispatch.StepID, stage)
	if err != nil || admitted == nil {
		t.Fatal("review did not admit its actual local delivery", admitted, err)
	}
	a, err := s.ClaimAttempt(ctx, admitted.Attempt.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := domain.GraphDeliveryScopeDigest(admitted.Workflow.Authority.Delivery)
	if err != nil {
		t.Fatal(err)
	}
	authority, invalid := domain.WorkflowAuthoritySHA256(admitted.Workflow.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	binding := domain.GraphDeliveryBinding{GraphID: d.Public.Dispatch.GraphID, GraphVersion: d.Public.Dispatch.GraphVersion, GraphControlRevision: d.Public.Dispatch.GraphControlRevision, RootBinding: domain.GraphRootPublic(root), FinalNodeID: d.Public.Dispatch.NodeID, FinalTaskID: a.TaskID, FinalTaskVersion: a.TaskVersion, FinalWorkflowID: a.WorkflowID, FinalCandidateBinding: *author.CandidateBinding, FinalInputID: d.Public.Dispatch.InputID, ContributorVector: d.Public.Dispatch.ContributorVector, FinalVerificationRef: *stage.Graph.SourceBasis.FinalVerificationRef, OtherNodeAcceptances: []domain.GraphContributorBinding{}, PreDeliveryEvidenceUses: admitted.Workflow.EvidenceUses, DeliveryScopeSHA256: scope}
	local := graphDeliveryDAOCase{store: s, owner: owner, dispatch: d.Public.Dispatch, attempt: a, facts: domain.GraphDeliveryFacts{Binding: binding, VerifiedArtifacts: reviewFacts.VerifiedArtifacts}, operation: domain.DeliveryOperation{ID: domain.NewID(), WorkflowID: a.WorkflowID, StepID: a.StepID, AttemptID: a.ID, RequestID: a.StartRequestID, CandidateBinding: *author.CandidateBinding, EvidenceUses: admitted.Workflow.EvidenceUses, Scope: admitted.Workflow.Authority.Delivery, AuthoritySHA256: authority, Status: "preparing", LastEffectSequence: "0", ConflictCount: "0", Conflicts: []domain.DeliveryConflict{}, CreatedAt: domain.Now()}}
	local.reseal(t)
	unselected, _ := graphPayload(t, s, a.ID, "graph_delivery", []byte(encode(local.facts.Binding)))
	f.unselected = &unselected
	if err = s.BeginDelivery(ctx, owner, local.operation, &local.facts); err != nil {
		t.Fatal(err)
	}
	local.ready(t)
	if err = s.ReadyDelivery(ctx, owner, local.operation, &local.facts); err != nil {
		t.Fatal(err)
	}
	intent, err := s.PermitDeliveryEffect(ctx, local.permit(t))
	if err != nil {
		t.Fatal(err)
	}
	after, afterProof := graphPayload(t, s, a.ID, "destination_manifest", []byte("controlled complete observed destination after the actual recorded effect"))
	ack := intent
	ack.State = "applied"
	ack.ObservedAfterEvidence = &domain.EvidenceReference{Ref: "controlled-native-result", SHA256: after.SHA256}
	if delivery == "uncertain" {
		ack.State, ack.ObservedAfterEvidence = "uncertain", nil
	}
	if err = s.RecordDeliveryEffect(ctx, ack); err != nil {
		t.Fatal(err)
	}
	op, err := s.DeliveryOperation(ctx, local.operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	op.Status = "applied"
	op.FinishedAt = ptr(domain.Now())
	op.AfterDestination = &domain.DestinationEvidence{ProjectIdentity: op.Scope.ProjectIdentity, RepositoryIdentity: op.Scope.RepositoryIdentity, Branch: op.Scope.Branch, BaseCommit: root.BaseCommit, ManifestArtifactID: after.ArtifactID, ManifestSHA256: after.SHA256}
	proofs := append(append([]domain.VerifiedResponse{}, local.facts.VerifiedArtifacts...), afterProof)
	for _, id := range []string{op.PlanArtifactID, op.BeforeDestination.ManifestArtifactID} {
		record, err := s.Artifact(ctx, id)
		if err != nil || record.SHA256 == nil {
			t.Fatal(record, err)
		}
		proofs = append(proofs, domain.VerifiedResponse{ArtifactID: id, SHA256: *record.SHA256, ByteLength: record.ByteLength, PhysicalIdentity: record.PhysicalIdentity})
	}
	settlement := domain.Settlement{Outcome: "succeeded", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "complete"}
	if delivery == "uncertain" {
		op.Status, op.AfterDestination = "uncertain", nil
		settlement.Outcome, settlement.EffectsKnown, settlement.EvidenceStatus = "blocked", false, "uncertain"
	}
	task, err = s.Task(ctx, a.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if settled, err := s.SettleWorkflowStep(ctx, a.ID, domain.WorkflowStepFacts{StepID: a.StepID, OwnerInstanceID: owner, TaskRevision: task.Task.Revision, WorkflowRevision: admitted.Workflow.Revision, CandidateBinding: author.CandidateBinding, VerifiedArtifacts: proofs, Operation: &op, Settlement: settlement}); err != nil || delivery == "applied" && settled.NextAction != "accept" {
		t.Fatal("recorded delivery did not settle honestly", settled, err)
	}
	local.operation = op
	f.delivery, f.proofs = &local, proofs
	return f
}

func graphFinalAcceptanceFacts(t *testing.T, f graphFinalAcceptanceCase) domain.WorkflowAcceptanceFacts {
	t.Helper()
	ctx := context.Background()
	task, err := f.store.Task(ctx, f.producer.Dispatch.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := f.store.Workflow(ctx, task.Task.ID, domain.WorkflowSelector{})
	if err != nil || detail.Workflow == nil {
		t.Fatal(detail, err)
	}
	w := *detail.Workflow
	hash, invalid := domain.WorkflowAuthoritySHA256(w.Authority)
	if invalid != nil {
		t.Fatal(invalid)
	}
	request := domain.NewID()
	record := domain.WorkflowAcceptance{ID: domain.NewID(), WorkflowID: w.ID, TaskID: w.TaskID, TaskVersion: w.TaskVersion, CandidateBinding: *f.author.CandidateBinding, AuthoritySHA256: hash, ReviewUses: w.EvidenceUses, CheckUses: []domain.EvidenceUse{}, FindingResolutionIDs: []string{}, ExternalResultRefs: []string{}, DeliveryResult: "applied", AcceptedByRequestID: request}
	if f.delivery != nil {
		record.DeliveryOperationID = &f.delivery.operation.ID
	}
	return domain.WorkflowAcceptanceFacts{RequestID: request, ExpectedTaskRevision: task.Task.Revision, ExpectedWorkflowRevision: w.Revision, OwnerInstanceID: f.owner, VerifiedArtifacts: f.proofs, Acceptance: record}
}

func TestGraphFinalAcceptanceUsesRecordedReviewedCumulativeDelivery(t *testing.T) {
	f := graphFinalAcceptanceFixture(t, "applied")
	ctx := context.Background()
	budget, err := f.store.GraphBudget(ctx, f.producer.Dispatch.GraphID)
	if err != nil || budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("reviewed local delivery did not retain its actual two model admissions", budget, err)
	}
	facts := graphFinalAcceptanceFacts(t, f)
	result := f.store.AcceptWorkflow(ctx, f.producer.Dispatch.TaskID, facts)
	if result.Status != 200 {
		t.Fatal("actual reviewed and recorded delivery of an own-zero final could not be accepted", result)
	}
	view, err := f.store.Graph(ctx, f.producer.Dispatch.GraphID)
	if err != nil || view.Graph.State != "completed" || view.Graph.Counts.Completed != "1" || view.Nodes[0].AcceptedResult == nil || view.Nodes[0].AcceptedResult.AcceptanceID != facts.Acceptance.ID || view.Nodes[0].AcceptedResult.ProducerAttemptID != f.producer.Dispatch.AttemptID || view.Nodes[0].LatestAttemptID == nil || *view.Nodes[0].LatestAttemptID != f.delivery.attempt.ID {
		t.Fatal("final acceptance lost its real producer or supporting delivery", view, err)
	}
	candidate, err := f.store.Candidate(ctx, f.author.CandidateBinding.CandidateID)
	if err != nil || candidate.HasDelta || candidate.HasIndexDelta {
		t.Fatal("cumulative acceptance invented node authorship", candidate, err)
	}
	if replay := f.store.AcceptWorkflow(ctx, f.producer.Dispatch.TaskID, facts); replay.Status != 200 || replay.Envelope.Receipt == nil || replay.Envelope.Receipt.RequestID != facts.RequestID {
		t.Fatal("exact acceptance replay did not retain its original receipt", replay)
	}
	var acceptances, effects int
	if f.store.db.QueryRow("SELECT count(*) FROM workflow_acceptances WHERE workflow_id=?", f.producer.Dispatch.WorkflowID).Scan(&acceptances) != nil || acceptances != 1 || f.store.db.QueryRow("SELECT count(*) FROM delivery_effects WHERE operation_id=?", f.delivery.operation.ID).Scan(&effects) != nil || effects != 2 {
		t.Fatal("acceptance replay repeated an acknowledgement or filesystem action", acceptances, effects)
	}
	after, err := f.store.GraphBudget(ctx, f.producer.Dispatch.GraphID)
	if err != nil || after.Budget.ConsumedAdmissions != budget.Budget.ConsumedAdmissions || after.Budget.CurrentWindow.Number != budget.Budget.CurrentWindow.Number {
		t.Fatal("nonexecuting acceptance spent or renewed the model allowance", after, err)
	}
}

func TestGraphFinalAcceptanceRejectsMismatchedRecordedDeliveryAssociation(t *testing.T) {
	f := graphFinalAcceptanceFixture(t, "applied")
	ctx := context.Background()
	var operationBefore string
	if err := f.store.db.QueryRow("SELECT data FROM delivery_operations WHERE id=?", f.delivery.operation.ID).Scan(&operationBefore); err != nil {
		t.Fatal(err)
	}
	// Deliberate private-cache corruption retains both the real applied005
	// operation and its exact effect journal. Equal complete binding bytes under
	// an unselected artifact do not replace that operation's captured reference.
	cached := graphDeliveryRecord{OperationID: f.delivery.operation.ID, Binding: f.delivery.facts.Binding, BindingRef: *f.unselected}
	if _, err := f.store.db.Exec("UPDATE graph_delivery_bindings SET binding_artifact_id=?,data=? WHERE operation_id=?", f.unselected.ArtifactID, encode(cached), f.delivery.operation.ID); err != nil {
		t.Fatal(err)
	}
	facts := graphFinalAcceptanceFacts(t, f)
	if result := f.store.AcceptWorkflow(ctx, f.producer.Dispatch.TaskID, facts); result.Status == 200 {
		t.Fatal("an unselected delivery binding replaced the recorded applied association", result)
	}
	var operationAfter string
	var acceptances, effects int
	if err := f.store.db.QueryRow("SELECT data FROM delivery_operations WHERE id=?", f.delivery.operation.ID).Scan(&operationAfter); err != nil || operationAfter != operationBefore || f.store.db.QueryRow("SELECT count(*) FROM workflow_acceptances WHERE workflow_id=?", f.producer.Dispatch.WorkflowID).Scan(&acceptances) != nil || acceptances != 0 || f.store.db.QueryRow("SELECT count(*) FROM delivery_effects WHERE operation_id=?", f.delivery.operation.ID).Scan(&effects) != nil || effects != 2 {
		t.Fatal("rejected acknowledgement rewrote delivery history or partly accepted", operationAfter, acceptances, effects, err)
	}
}

func TestGraphFinalAcceptanceCannotPromoteMissingOrUncertainDelivery(t *testing.T) {
	for _, state := range []string{"missing", "uncertain"} {
		t.Run(state, func(t *testing.T) {
			f := graphFinalAcceptanceFixture(t, state)
			facts := graphFinalAcceptanceFacts(t, f)
			if result := f.store.AcceptWorkflow(context.Background(), f.producer.Dispatch.TaskID, facts); result.Status == 200 {
				t.Fatal("incomplete required delivery was promoted into accepted graph output", result)
			}
			var accepted int
			if err := f.store.db.QueryRow("SELECT count(*) FROM workflow_acceptances WHERE workflow_id=?", f.producer.Dispatch.WorkflowID).Scan(&accepted); err != nil || accepted != 0 {
				t.Fatal("rejected delivery acknowledgement partly installed acceptance", accepted, err)
			}
		})
	}
}
