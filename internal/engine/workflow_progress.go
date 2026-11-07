package engine

import (
	"context"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/project"
	workflowpolicy "github.com/Ejupi-Labs/council/v2/internal/workflow"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type workflowProgress struct {
	final                                    *graphFinalVerification
	subject                                  workflowSubject
	detail                                   domain.TaskDetail
	version                                  domain.TaskVersionDetail
	decision                                 workflowpolicy.PlanDecision
	review                                   *domain.ReviewRecord
	checks                                   []domain.CheckReceipt
	reviewUses, checkUses                    []domain.EvidenceUse
	resolutionIDs, externalRefs, artifactIDs []string
	operationID                              *string
	direct, hasDelta, hasIndexDelta          bool
}

func workflowResolutionMatches(subject workflowSubject, resolution domain.FindingResolution) bool {
	return subject.Binding != nil && resolution.CorrectedCandidateID != nil && *resolution.CorrectedCandidateID == subject.Binding.CandidateID ||
		subject.Response != nil && resolution.CorrectedResponseBinding != nil && *resolution.CorrectedResponseBinding == *subject.Response
}

func workflowUse(subject workflowSubject, kind, id string, applicability domain.Applicability) domain.EvidenceUse {
	use := domain.EvidenceUse{Kind: kind, EvidenceID: id, Applicability: applicability}
	if subject.Binding != nil {
		use.CandidateID = subject.Binding.CandidateID
	} else {
		use.ResponseArtifactID = subject.Response.ResponseArtifactID
	}
	return use
}

func (e *Engine) currentWorkflowEvidence(ctx context.Context, subject workflowSubject, final ...*graphFinalVerification) ([]domain.EvidenceUse, *domain.ReviewRecord, []domain.CheckReceipt, []string, error) {
	uses, checks, ids := []domain.EvidenceUse{}, []domain.CheckReceipt{}, []string{}
	var review *domain.ReviewRecord
	if len(subject.Workflow.EvidenceUses) > domain.MaxWorkflowChecks+1 {
		return nil, nil, nil, nil, domain.Fail("evidence_unavailable", "The current evidence index exceeds its captured bound.")
	}
	for _, use := range subject.Workflow.EvidenceUses {
		scope := domain.CheckScope{Kind: use.Applicability.ScopeKind, Paths: use.Applicability.Paths}
		current, err := e.workflowApplicability(ctx, subject, scope, use.Applicability.ConditionKeys, use.Applicability.DefinitionSHA256, final...)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !workflowpolicy.Applicable(use.Applicability, current) {
			continue
		}
		switch use.Kind {
		case "review":
			if review != nil {
				return nil, nil, nil, nil, domain.Fail("evidence_unavailable", "The selected subject has ambiguous current review evidence.")
			}
			record, err := e.db.WorkflowReview(ctx, use.EvidenceID)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if record.WorkflowID != subject.Workflow.ID || record.ReviewContract != domain.ReviewContract || record.Applicability.DefinitionSHA256 != workflowHash(adapters.ReviewSchema()) || !workflowpolicy.Applicable(record.Applicability, current) {
				return nil, nil, nil, nil, domain.Fail("evidence_unavailable", "The registered review does not match its current applicability.")
			}
			review = &record
			ids = append(ids, record.ResultArtifactID)
		case "check":
			record, err := e.db.WorkflowCheck(ctx, use.EvidenceID)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if record.WorkflowID != subject.Workflow.ID || !workflowpolicy.Applicable(record.Applicability, current) {
				return nil, nil, nil, nil, domain.Fail("evidence_unavailable", "The registered check does not match its current applicability.")
			}
			checks = append(checks, record)
			if record.StdoutArtifactID != nil {
				ids = append(ids, *record.StdoutArtifactID)
			}
			if record.StderrArtifactID != nil {
				ids = append(ids, *record.StderrArtifactID)
			}
		default:
			return nil, nil, nil, nil, domain.Fail("evidence_unavailable", "The current evidence index contains an unsupported record kind.")
		}
		uses = append(uses, workflowUse(subject, use.Kind, use.EvidenceID, current))
	}
	if _, err := e.verifyWorkflowArtifacts(ctx, ids...); err != nil {
		return nil, nil, nil, nil, err
	}
	return uses, review, checks, ids, nil
}

func (e *Engine) inspectWorkflowProgress(ctx context.Context, taskID string, w domain.Workflow) (workflowProgress, error) {
	out := workflowProgress{checks: []domain.CheckReceipt{}, reviewUses: []domain.EvidenceUse{}, checkUses: []domain.EvidenceUse{}, resolutionIDs: []string{}, externalRefs: []string{}, artifactIDs: []string{}}
	var err error
	out.detail, err = e.db.Task(ctx, taskID)
	if err != nil {
		return out, err
	}
	out.version, err = e.db.TaskVersion(ctx, taskID, w.TaskVersion)
	if err != nil {
		return out, err
	}
	out.subject, err = e.selectedWorkflowSubject(ctx, w)
	if err != nil {
		return out, err
	}
	out.hasDelta, out.hasIndexDelta = workflowChangeFlags(workspace.CompareManifests(out.subject.RootManifest, out.subject.Manifest))
	out.direct = e.workflowDirectProof(ctx, out.subject, out.version.OriginalText, out.version.Version) == nil
	out.final, err = e.loadGraphFinalVerification(ctx, out.subject)
	if err != nil {
		return out, err
	}
	finalPlan, err := e.graphFinalPlanFacts(ctx, out.final, out.version.OriginalText, out.version.Version)
	if err != nil {
		return out, err
	}
	uses, review, checks, ids, err := e.currentWorkflowEvidence(ctx, out.subject, out.final)
	if err != nil {
		return out, err
	}
	out.review, out.checks, out.artifactIDs = review, checks, ids
	for _, use := range uses {
		if use.Kind == "review" {
			out.reviewUses = append(out.reviewUses, use)
		} else {
			out.checkUses = append(out.checkUses, use)
		}
	}
	input := workflowpolicy.PlanInput{Authority: w.Authority, ResponseOnly: out.version.Version.Acceptance.Deliverable == "response_only", HasSubject: true, HasDelta: out.hasDelta, HasIndexDelta: out.hasIndexDelta, DirectProven: out.direct, GraphFinal: finalPlan, Review: out.review, Checks: checks, BlockingFindingIDs: []string{}, CorrectionCyclesUsed: w.CorrectionCyclesUsed}
	cursor := ""
	for {
		page, listErr := e.db.WorkflowFindings(ctx, taskID, domain.WorkflowSelector{WorkflowID: w.ID}, 100, cursor)
		if listErr != nil {
			return out, listErr
		}
		for _, finding := range page.Items {
			if finding.Finding.Severity != "blocking" {
				continue
			}
			resolved := false
			for _, resolution := range finding.Resolutions {
				current := workflowResolutionMatches(out.subject, resolution)
				if current {
					resolved = true
					out.resolutionIDs = append(out.resolutionIDs, resolution.ID)
				}
			}
			if !resolved {
				if finding.ReviewID != "" {
					input.NeedsFindingReview = true
				}
				input.BlockingFindingIDs = append(input.BlockingFindingIDs, finding.ID)
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if w.CurrentStepID != nil {
		step, readErr := e.db.WorkflowStep(ctx, *w.CurrentStepID)
		if readErr != nil {
			return out, readErr
		}
		if step.AttemptID != nil {
			attempt, readErr := e.db.Attempt(ctx, *step.AttemptID)
			if readErr != nil {
				return out, readErr
			}
			if attempt.Outcome != nil && *attempt.Outcome != "succeeded" {
				qualifyingCheck := false
				for _, check := range checks {
					if check.AttemptID == attempt.ID && check.Outcome == "failed" && !check.SourceChanged && check.RootTerminal && check.ExitCode != nil && check.Cleanup.State == "verified" && check.InputComplete && check.OutputComplete {
						qualifyingCheck = true
					}
				}
				if !qualifyingCheck {
					input.StageFailure = "previous_stage_did_not_qualify"
					if attempt.PrimaryFailure != nil {
						input.StageFailure = attempt.PrimaryFailure.Code
					}
				}
			}
		}
	}
	cursor = ""
	for {
		page, listErr := e.db.WorkflowDeliveries(ctx, taskID, domain.WorkflowSelector{WorkflowID: w.ID}, 100, cursor)
		if listErr != nil {
			return out, listErr
		}
		for _, delivery := range page.Items {
			if delivery.Operation != nil && out.subject.Binding != nil && delivery.Operation.CandidateBinding == *out.subject.Binding {
				for _, destination := range []*domain.DestinationEvidence{delivery.Operation.BeforeDestination, delivery.Operation.AfterDestination} {
					if destination != nil {
						out.artifactIDs = append(out.artifactIDs, destination.ManifestArtifactID)
					}
				}
				if delivery.Operation.PlanArtifactID != "" {
					out.artifactIDs = append(out.artifactIDs, delivery.Operation.PlanArtifactID)
				}
				input.LocalOutcome = delivery.Operation.Status
				input.LocalApplied = delivery.Operation.Status == "applied"
				if input.LocalApplied {
					id := delivery.Operation.ID
					out.operationID = &id
				}
			}
			if delivery.External != nil {
				step, readErr := e.db.WorkflowStep(ctx, delivery.External.StepID)
				if readErr != nil {
					return out, readErr
				}
				current := out.subject.Binding != nil && step.InputCandidateID != nil && *step.InputCandidateID == out.subject.Binding.CandidateID ||
					out.subject.Response != nil && step.InputResponseBinding != nil && *step.InputResponseBinding == *out.subject.Response
				if current {
					input.ExternalOutcome, input.ExternalComplete = delivery.External.Outcome, delivery.External.Outcome == "command_succeeded"
					if input.ExternalComplete {
						out.externalRefs = append(out.externalRefs, step.ID)
					}
					if delivery.External.StdoutArtifactID != nil {
						out.artifactIDs = append(out.artifactIDs, *delivery.External.StdoutArtifactID)
					}
					if delivery.External.StderrArtifactID != nil {
						out.artifactIDs = append(out.artifactIDs, *delivery.External.StderrArtifactID)
					}
				}
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	out.decision = workflowpolicy.Plan(input)
	return out, nil
}

func (e *Engine) workflowAdmissionFacts(ctx context.Context, progress workflowProgress, previousStepID string, explicit ...*domain.GraphAdvanceInput) (domain.WorkflowStepAdmissionFacts, error) {
	w, decision := progress.subject.Workflow, progress.decision
	cycle := w.CorrectionCyclesUsed
	if decision.Role == "correction" {
		cycle++
	}
	subjectID := progress.subject.reviewSubject().ID
	key, invalid := domain.WorkflowStageKey(decision.Role, cycle, decision.CheckID, subjectID)
	if invalid != nil {
		return domain.WorkflowStepAdmissionFacts{}, invalid
	}
	plan := domain.WorkflowStagePlan{Role: decision.Role, ExecutorKind: decision.ExecutorKind, LogicalKey: key, Cycle: cycle, InputCandidateID: w.SelectedCandidateID, InputResponseBinding: w.SelectedResponseBinding, PredecessorStepID: &previousStepID, ReasonCodes: decision.ReasonCodes}
	if decision.CheckID != "" {
		plan.DefinitionID = &decision.CheckID
	}
	facts := domain.WorkflowStepAdmissionFacts{Plan: plan, ExpectedTaskRevision: progress.detail.Task.Revision, ExpectedWorkflowRevision: w.Revision}
	if decision.ExecutorKind == "native" {
		var err error
		facts.Routing, err = e.nativeWorkflowRoutingFacts(ctx, w, progress.detail.Task.ProjectID)
		if err != nil {
			return facts, err
		}
	}
	graphFacts, err := e.graphWorkflowStageFacts(ctx, progress, previousStepID, explicit...)
	if err != nil {
		return facts, err
	}
	facts.Graph = graphFacts
	return facts, nil
}

func (e *Engine) nativeWorkflowRoutingFacts(ctx context.Context, w domain.Workflow, projectID string) (domain.RoutingFacts, error) {
	var facts domain.RoutingFacts
	view, err := e.db.TaskRouting(ctx, w.TaskID)
	if err != nil {
		return facts, err
	}
	if view.Authority == nil {
		return facts, domain.Fail("routing_adoption_required", "The native workflow stage has no captured routing authority.")
	}
	p, identity, err := e.db.Project(ctx, projectID)
	if err != nil {
		return facts, err
	}
	if identity != w.Authority.Delivery.ProjectIdentity {
		return facts, domain.Fail("project_identity_changed", "The native workflow destination identity no longer matches its captured authority.")
	}
	if err := project.ValidateIdentity(p.ResolvedPath, identity); err != nil {
		return facts, err
	}
	facts.Assessment = view.Authority.SuitabilityAssessment
	facts.Contexts, err = e.routingContexts(ctx, w.Authority.ExecutionMode)
	return facts, err
}

// The caller invokes this only after one fresh worker returned and 004A had the
// first opportunity to create its replacement. There is no recovery/history scan.
func (e *Engine) continueWorkflow(previousID string) {
	if e.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()
	a, err := e.db.Attempt(ctx, previousID)
	if err != nil || a.WorkflowID == "" {
		return
	}
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil {
		return
	}
	w := *view.Workflow
	if w.State == "completed" || w.CurrentStepID == nil || *w.CurrentStepID != a.StepID {
		return
	}
	step, err := e.db.WorkflowStep(ctx, a.StepID)
	if err != nil || step.AdvanceProof == nil || step.AttemptID == nil || *step.AttemptID != previousID {
		return
	}
	progress, err := e.inspectWorkflowProgress(ctx, a.TaskID, w)
	if err != nil {
		if uncertainEvidence(err) {
			e.fence(a.ID, "workflow_evidence_uncertain", "The fresh workflow prerequisite could not be verified. No historical stage is replayed.")
		}
		return
	}
	if progress.decision.Action == "accept" {
		e.acceptCurrentWorkflow(ctx, progress, domain.NewID(), step.ID)
		return
	}
	if progress.decision.Role == "" {
		return
	}
	facts, err := e.workflowAdmissionFacts(ctx, progress, step.ID)
	if err != nil {
		return
	}
	e.prepareGate.Lock()
	e.control.Lock()
	if !e.fenced {
		_, err = e.db.ContinueWorkflow(ctx, e.config.OwnerInstanceID, step.ID, facts)
		if persistenceFault(err) {
			e.fenceLocked(a.ID, "workflow_admission_uncertain", "The next workflow admission could not be acknowledged durably. Its identity must not be replayed as fresh work.")
		}
	}
	e.control.Unlock()
	e.prepareGate.Unlock()
	if err == nil {
		e.signal()
	}
}

func (e *Engine) acceptCurrentWorkflow(ctx context.Context, progress workflowProgress, requestID, automaticPredecessorStepID string) domain.Result {
	w := progress.subject.Workflow
	if progress.decision.Action != "accept" || progress.subject.Binding == nil {
		return e.failure(requestID, domain.Fail("acceptance_unavailable", "The current candidate still requires verification or delivery evidence."))
	}
	ids := append(progress.subject.artifactIDs(), progress.artifactIDs...)
	verified, err := e.verifyWorkflowArtifacts(ctx, ids...)
	if err != nil {
		return e.failure(requestID, domain.Fail("evidence_unavailable", "Fresh acceptance evidence could not be verified."))
	}
	graphProofs, err := e.graphAcceptanceArtifactProofs(ctx, progress.subject)
	if err != nil {
		return e.failure(requestID, domain.Fail("evidence_unavailable", "The exact selected graph output and input could not be verified freshly for acknowledgement."))
	}
	finalProofs, err := e.graphFinalAcceptanceProofs(ctx, progress.subject, progress.operationID)
	if err != nil {
		return e.failure(requestID, domain.Fail("evidence_unavailable", "The current cumulative final and recorded graph delivery could not be verified freshly for acknowledgement."))
	}
	graphProofs, err = mergeGraphAcceptanceProofs(graphProofs, finalProofs)
	if err != nil {
		return e.failure(requestID, domain.Fail("evidence_unavailable", "The complete cumulative acknowledgement proof set exceeds its bound."))
	}
	verified, err = mergeGraphAcceptanceProofs(verified, graphProofs)
	if err != nil {
		return e.failure(requestID, domain.Fail("evidence_unavailable", "The complete acceptance proof set could not be retained within its bound."))
	}
	authority, authorityErr := domain.WorkflowAuthoritySHA256(w.Authority)
	if authorityErr != nil {
		return e.failure(requestID, authorityErr)
	}
	deliveryResult := "review_only"
	if w.Authority.Delivery.LocalMode == "apply_verified_local" {
		deliveryResult = "applied"
	}
	acceptance := domain.WorkflowAcceptance{ID: domain.NewID(), WorkflowID: w.ID, TaskID: w.TaskID, TaskVersion: w.TaskVersion, CandidateBinding: *progress.subject.Binding, AuthoritySHA256: authority, ReviewUses: progress.reviewUses, CheckUses: progress.checkUses, FindingResolutionIDs: progress.resolutionIDs, DeliveryOperationID: progress.operationID, ExternalResultRefs: progress.externalRefs, DeliveryResult: deliveryResult, AcceptedAt: domain.Now(), AcceptedByRequestID: requestID}
	facts := domain.WorkflowAcceptanceFacts{AutomaticPredecessorStepID: automaticPredecessorStepID, RequestID: requestID, ExpectedTaskRevision: progress.detail.Task.Revision, ExpectedWorkflowRevision: w.Revision, OwnerInstanceID: e.config.OwnerInstanceID, VerifiedArtifacts: verified, Acceptance: acceptance}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(requestID, domain.Fail("dispatch_fenced", "Resolve ownership or persistence uncertainty before accepting workflow completion."))
	}
	result := e.db.AcceptWorkflow(ctx, w.TaskID, facts)
	e.guardMutationLocked(result)
	e.control.Unlock()
	return result
}
