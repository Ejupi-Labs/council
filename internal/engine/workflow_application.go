package engine

import (
	"context"
	json "encoding/json/v2"
	"errors"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type deliveryPlanAction struct {
	NativeActionID, EffectID, RelativePath, Kind, Action, PrivateQuarantineRef string
	Before, IntendedAfter                                                      *domain.EvidenceReference
}
type deliveryPlan struct {
	Contract, OperationID, WorkflowID, StepID, CandidateID string
	BeforeManifest, IntendedManifest                       domain.EvidenceReference
	Actions                                                []deliveryPlanAction
}

func applicationEvidence(meta *evidence.Metadata, name string) *domain.EvidenceReference {
	if meta == nil || meta.SHA256 == nil {
		return nil
	}
	return &domain.EvidenceReference{Ref: name, ArtifactID: meta.ID, SHA256: *meta.SHA256}
}

func applicationDestination(meta evidence.Metadata, subject workflowSubject, graph ...*domain.GraphRootView) *domain.DestinationEvidence {
	if !domain.ValidID(meta.ID) || meta.SHA256 == nil || len(graph) > 1 {
		return nil
	}
	branch, base := subject.RootSource.Branch, subject.RootSource.BaseCommit
	if len(graph) == 1 && graph[0] != nil {
		branch, base = graph[0].Branch, graph[0].BaseCommit
	}
	return &domain.DestinationEvidence{ProjectIdentity: subject.Workflow.Authority.Delivery.ProjectIdentity, RepositoryIdentity: subject.Workflow.Authority.Delivery.RepositoryIdentity, Branch: branch, BaseCommit: base, ManifestArtifactID: meta.ID, ManifestSHA256: *meta.SHA256}
}

func (e *Engine) runWorkflowApplication(ctx context.Context, a domain.Attempt, step domain.WorkflowStep, subject workflowSubject) {
	if subject.Binding == nil || subject.Candidate == nil {
		e.settleUnlaunched(a, primary("application_unsupported", "Response-only work has no candidate delta to apply."))
		return
	}
	if err := e.guardLocalStage(ctx, a); err != nil {
		e.settleUnlaunched(a, primary("launch_blocked", "The local application lost current execution authority."), err)
		return
	}
	graphDelivery, graphErr := e.prepareGraphDelivery(ctx, a, subject)
	if graphErr != nil {
		if graphDelivery == nil {
			e.settleUnlaunched(a, primary("graph_final_unqualified", "The exact cumulative final could not authorize graph delivery."), graphErr)
		} else {
			refs := []domain.GraphArtifactRef{}
			if validGraphArtifactReference(graphDelivery.Facts.BindingRef) {
				refs = append(refs, graphDelivery.Facts.BindingRef)
			}
			e.settleUnlaunchedEffects(a, primary("graph_final_unqualified", "Graph delivery preparation did not retain a complete verified basis."), graphDelivery.EffectsKnown, refs, true, graphErr)
		}
		return
	}
	var graphFacts *domain.GraphDeliveryFacts
	var graphRoot *domain.GraphRootView
	if graphDelivery != nil {
		graphFacts, graphRoot = &graphDelivery.Facts, &graphDelivery.Facts.Binding.RootBinding
	}
	digest, invalid := domain.WorkflowAuthoritySHA256(subject.Workflow.Authority)
	if invalid != nil {
		e.settleUnlaunched(a, primary("authority_required", invalid.Message))
		return
	}
	op := domain.DeliveryOperation{ID: domain.NewID(), WorkflowID: a.WorkflowID, StepID: step.ID, AttemptID: a.ID, RequestID: a.StartRequestID, CandidateBinding: *subject.Binding, EvidenceUses: subject.Workflow.EvidenceUses, Scope: subject.Workflow.Authority.Delivery, AuthoritySHA256: digest, Status: "preparing", CreatedAt: domain.Now(), LastEffectSequence: "0", Conflicts: []domain.DeliveryConflict{}, ConflictCount: "0", NextAction: "prepare_destination"}
	if graphDelivery != nil {
		ref := graphDelivery.Facts.BindingRef
		op.GraphDeliveryRef = &ref
	}
	e.control.Lock()
	var err error
	if e.fenced {
		err = domain.Fail("dispatch_fenced", "The owner was fenced before application preparation.")
	} else {
		err = e.db.BeginDelivery(ctx, e.config.OwnerInstanceID, op, graphFacts)
	}
	e.control.Unlock()
	if err != nil {
		failure := primary("application_unsupported", "The durable local operation could not be prepared.")
		if graphDelivery != nil {
			problem := domain.SafeError(err)
			failure = primary(problem.Code, problem.Message)
		}
		e.settleUnlaunched(a, failure, err)
		return
	}
	root := subject
	root.Source, root.Manifest = subject.RootSource, subject.RootManifest
	applySource := root.snapshot(op.Scope)
	if graphDelivery != nil {
		applySource = graphDelivery.RootSource
	}
	application, prepareErr := e.workspace.PrepareApply(ctx, workspace.ApplyRequest{AttemptID: a.ID, OperationID: op.ID, Source: applySource, SelectedCandidateID: subject.Binding.CandidateID, SelectedAttemptID: subject.Candidate.AttemptID, CandidateManifestArtifactID: subject.Binding.ManifestArtifactID, CandidateManifestSHA256: subject.Binding.ManifestSHA256, ProjectIdentity: op.Scope.ProjectIdentity, RepositoryIdentity: op.Scope.RepositoryIdentity})
	facts := domain.WorkflowStepFacts{StepID: step.ID, OwnerInstanceID: e.config.OwnerInstanceID, CandidateBinding: subject.Binding, Settlement: domain.Settlement{NoProcessCreated: true, EffectsKnown: true, Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "complete"}}
	if graphDelivery != nil {
		facts.VerifiedArtifacts = append([]domain.VerifiedResponse{}, graphFacts.VerifiedArtifacts...)
	}
	if prepareErr != nil {
		op.Status, op.NextAction = "conflict", "inspect_destination"
		code := "application_unsupported"
		if errors.Is(prepareErr, workspace.ErrApplyConflict) {
			code = "integration_conflict"
		}
		facts.Settlement.Failure = primary(code, "The destination could not satisfy the captured application basis. No destination action was permitted.")
		op.Conflicts = append(op.Conflicts, domain.DeliveryConflict{Code: code})
		op.ConflictCount = "1"
		if uncertainEvidence(prepareErr) {
			op.Status, op.NextAction = "interrupted", "inspect_preparation"
			facts.Settlement.EvidenceStatus = "uncertain"
			if graphDelivery != nil {
				facts.Settlement.EffectsKnown = false
			}
			facts.Settlement.Failure = primary("application_preparation_uncertain", "Private preparation evidence durability is unresolved. No destination action was permitted.")
			e.fence(a.ID, "application_preparation_uncertain", "Application preparation durability is unresolved; no path action is replayed.")
		}
		finished := domain.Now()
		op.FinishedAt = &finished
		facts.Operation = &op
		if graphDelivery != nil {
			e.finishWorkflowLocalFinal(a, subject, facts, graphDelivery.Final)
		} else {
			e.finishWorkflowLocal(a, subject, facts)
		}
		return
	}
	defer application.Close()
	op.BeforeDestination = applicationDestination(application.BeforeManifestArtifact, subject, graphRoot)
	plan := deliveryPlan{Contract: "005-delivery-plan-v1", OperationID: op.ID, WorkflowID: a.WorkflowID, StepID: step.ID, CandidateID: subject.Binding.CandidateID, Actions: []deliveryPlanAction{}}
	plan.BeforeManifest = *applicationEvidence(&application.BeforeManifestArtifact, "before-destination")
	plan.IntendedManifest = *applicationEvidence(&application.IntendedManifestArtifact, "intended-destination")
	pathEffects := map[string]string{}
	actions := application.Actions()
	for _, action := range actions {
		if pathEffects[action.RelativePath] == "" {
			pathEffects[action.RelativePath] = domain.NewID()
		}
		plan.Actions = append(plan.Actions, deliveryPlanAction{NativeActionID: action.ID, EffectID: pathEffects[action.RelativePath], RelativePath: action.RelativePath, Kind: action.Kind, Action: action.Action, PrivateQuarantineRef: action.QuarantineRef, Before: applicationEvidence(action.BeforeEvidence, "before:"+action.RelativePath), IntendedAfter: applicationEvidence(action.IntendedAfterEvidence, "after:"+action.RelativePath)})
	}
	raw, planErr := json.Marshal(plan)
	if planErr == nil && len(raw) > 64<<20 {
		planErr = domain.Fail("input_too_large", "The complete application plan exceeds its retained evidence bound; no path was truncated.")
	}
	if planErr == nil {
		meta, putErr := e.evidence.Put(ctx, a.ID, "delivery_plan", raw)
		planErr = putErr
		if putErr == nil {
			op.PlanArtifactID = meta.ID
		}
	}
	if planErr == nil && e.config.BeforeApplicationReady != nil {
		planErr = e.config.BeforeApplicationReady(ctx, a)
	}
	if planErr == nil {
		graphFacts, planErr = e.refreshGraphDelivery(ctx, a, subject, graphDelivery)
	}
	if planErr == nil {
		op.Status, op.NextAction = "ready", "apply_permitted_path"
		e.control.Lock()
		if e.fenced {
			planErr = domain.Fail("dispatch_fenced", "The owner was fenced before application readiness.")
		} else {
			planErr = e.db.ReadyDelivery(ctx, e.config.OwnerInstanceID, op, graphFacts)
		}
		e.control.Unlock()
	}
	known, effects := true, false
	if planErr != nil {
		// A rejected Ready transition did not grant a destination effect. Keep
		// the acknowledged binding; preparation artifacts remain attempt-owned.
		// A graph's private proof may be corrupt while its committed operation
		// remains readable. Preserve that exact acknowledged prefix even for
		// an uncertain Ready result; never invent an accepted plan from memory.
		if !uncertainEvidence(planErr) || graphDelivery != nil {
			if acknowledged, readErr := e.db.DeliveryOperation(context.Background(), op.ID); readErr == nil {
				op.PlanArtifactID, op.BeforeDestination = acknowledged.PlanArtifactID, acknowledged.BeforeDestination
			}
		}
		op.Status, op.NextAction = "interrupted", "inspect_preparation"
		facts.Settlement.Failure = primary("application_unsupported", "The complete application plan was not acknowledged. No destination action was executed.")
		if uncertainEvidence(planErr) {
			facts.Settlement.EvidenceStatus = "uncertain"
			if graphDelivery != nil {
				known = false
				op.Status = "uncertain"
			}
			e.fence(a.ID, "application_preparation_uncertain", "Application preparation durability is unresolved; no path action is replayed.")
		}
	} else {
		for index, action := range actions {
			intent := domain.DeliveryEffect{OperationID: op.ID, EffectID: pathEffects[action.RelativePath], RelativePath: action.RelativePath, Kind: action.Kind, Action: action.Action, State: "intent", BeforeEvidence: plan.Actions[index].Before, IntendedAfterEvidence: plan.Actions[index].IntendedAfter}
			freshGraph, graphProofErr := e.refreshGraphDelivery(ctx, a, subject, graphDelivery)
			e.control.Lock()
			var permit domain.DeliveryEffect
			permitCalled := false
			permitErr := errors.Join(ctx.Err(), graphProofErr)
			if permitErr == nil && e.fenced {
				permitErr = domain.Fail("dispatch_fenced", "The owner was fenced before another application permit.")
			}
			if permitErr == nil {
				task, taskErr := e.db.Task(ctx, a.TaskID)
				view, workflowErr := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
				permitErr = errors.Join(taskErr, workflowErr)
				if permitErr == nil && view.Workflow != nil {
					permitCalled = true
					permit, permitErr = e.db.PermitDeliveryEffect(ctx, domain.DeliveryPermitFacts{OperationID: op.ID, OwnerInstanceID: e.config.OwnerInstanceID, TaskRevision: task.Task.Revision, WorkflowRevision: view.Workflow.Revision, Effect: intent, Graph: freshGraph})
				} else if permitErr == nil {
					permitErr = domain.Fail("workflow_revision_conflict", "The current application workflow is unavailable.")
				}
			}
			e.control.Unlock()
			if permitErr != nil {
				op.Status, op.NextAction = "cancelled", "inspect_remaining_paths"
				if effects {
					op.Status = "partial"
				}
				if graphProofErr != nil && uncertainEvidence(graphProofErr) {
					known = false
					op.Status = "uncertain"
					e.fence(a.ID, "graph_delivery_evidence_uncertain", "The exact retained cumulative delivery evidence could not be verified before another effect.")
				}
				if permitCalled && uncertainEvidence(permitErr) {
					known = false
					op.Status = "uncertain"
					e.fence(a.ID, "application_permit_uncertain", "A filesystem-effect permit acknowledgement is unknown; preserve its intent and do not execute or repeat it.")
				}
				break
			}
			// The single committed permit is linearized above. Native filesystem
			// I/O, including required metadata restoration, never holds control.
			if e.config.BeforeApplicationAction != nil {
				e.config.BeforeApplicationAction(ctx, a, action)
			}
			observed, actionErr := application.ExecuteAction(ctx, action.ID)
			result := permit
			result.State, result.ObservedAt = observed.State, domain.Now()
			result.ObservedIdentity, result.NamespaceDurability = observed.Identity, observed.NamespaceDurability
			result.ObservedAfterEvidence = applicationEvidence(observed.ObservedEvidence, "observed:"+action.RelativePath)
			if !observed.Known {
				result.State = "uncertain"
			}
			e.control.Lock()
			recordErr := e.db.RecordDeliveryEffect(context.Background(), result)
			e.control.Unlock()
			if recordErr != nil {
				known = false
				op.Status, op.NextAction = "uncertain", "inspect_retained_effect"
				e.fence(a.ID, "application_effect_acknowledgement_uncertain", "The actual filesystem effect could not be acknowledged durably. Preserve quarantine and do not replay it.")
				break
			}
			if e.config.AfterApplicationAction != nil {
				e.config.AfterApplicationAction(ctx, a, action, observed)
			}
			effects = effects || result.State == "applied"
			if actionErr != nil || result.State != "applied" {
				op.Status, op.NextAction = "conflict", "inspect_retained_effect"
				if effects {
					op.Status = "partial"
				}
				if result.State == "uncertain" {
					known = false
					op.Status = "uncertain"
				}
				op.Conflicts = append(op.Conflicts, domain.DeliveryConflict{RelativePath: action.RelativePath, Code: "integration_conflict"})
				op.ConflictCount = domain.Decimal(int64(len(op.Conflicts)))
				break
			}
			if index == len(actions)-1 {
				op.AfterDestination = applicationDestination(application.AfterManifestArtifact, subject, graphRoot)
				if op.AfterDestination != nil {
					op.Status, op.NextAction = "applied", "verify_delivery"
				} else {
					known = false
					op.Status, op.NextAction = "uncertain", "inspect_destination"
				}
			}
		}
	}
	closeErr := application.Close()
	if closeErr != nil {
		known = false
		op.Status, op.NextAction = "uncertain", "inspect_retained_handles"
		e.fence(a.ID, "application_close_uncertain", "Application handles or private staged metadata did not close cleanly.")
	}
	if stored, readErr := e.db.DeliveryOperation(context.Background(), op.ID); readErr == nil {
		op.LastEffectSequence, op.IncidentID = stored.LastEffectSequence, stored.IncidentID
	} else {
		known = false
		op.Status = "uncertain"
		e.fence(a.ID, "storage_failed", "The settled application journal cannot be inspected.")
	}
	finished := domain.Now()
	op.FinishedAt = &finished
	if graphDelivery != nil {
		facts.VerifiedArtifacts = append([]domain.VerifiedResponse{}, graphDelivery.Facts.VerifiedArtifacts...)
	}
	facts.Operation, facts.Settlement.EffectsKnown = &op, known
	if op.Status == "applied" && known {
		facts.Settlement.Outcome, facts.Settlement.UsableOutput = "succeeded", true
	} else if op.Status == "cancelled" {
		facts.Settlement.Outcome = "cancelled"
	} else {
		facts.Settlement.Outcome = "failed"
	}
	if !known {
		facts.Settlement.EvidenceStatus = "uncertain"
	}
	if graphDelivery != nil {
		e.finishWorkflowLocalFinal(a, subject, facts, graphDelivery.Final)
	} else {
		e.finishWorkflowLocal(a, subject, facts)
	}
}
