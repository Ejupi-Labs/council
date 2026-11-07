package engine

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

// Local executors share the owned claim and captured task clock with native work.
func (e *Engine) runWorkflowLocal(ctx context.Context, a domain.Attempt, detail domain.TaskVersionDetail) {
	view, err := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if err != nil || view.Workflow == nil {
		e.settleUnlaunched(a, primary("evidence_unavailable", "The admitted local workflow cannot be inspected."), err)
		return
	}
	step, err := e.db.WorkflowStep(ctx, a.StepID)
	if err != nil || step.AttemptID == nil || *step.AttemptID != a.ID || view.Workflow.CurrentStepID == nil || *view.Workflow.CurrentStepID != step.ID {
		e.settleUnlaunched(a, primary("candidate_stale", "The local stage is not the current owned workflow step."), err)
		return
	}
	subject, err := e.selectedWorkflowSubject(ctx, *view.Workflow)
	if err != nil {
		e.settleUnlaunched(a, primary("evidence_unavailable", "The exact local stage subject cannot be verified."), err)
		return
	}
	if subject.Binding != nil && (step.InputCandidateID == nil || *step.InputCandidateID != subject.Binding.CandidateID) ||
		subject.Response != nil && (step.InputResponseBinding == nil || *step.InputResponseBinding != *subject.Response) {
		e.settleUnlaunched(a, primary("candidate_stale", "The selected subject changed after the local stage was admitted."))
		return
	}
	switch {
	case a.Role == "check" && a.ExecutorKind == "local_check":
		e.runWorkflowCheck(ctx, a, step, detail, subject)
	case a.Role == "local_delivery" && a.ExecutorKind == "local_application":
		e.runWorkflowApplication(ctx, a, step, subject)
	case a.Role == "external_delivery" && a.ExecutorKind == "local_external":
		e.runWorkflowExternal(ctx, a, step, subject)
	default:
		e.settleUnlaunched(a, primary("capability_unavailable", "The admitted local role/executor combination is unsupported."))
	}
}

func (e *Engine) guardLocalStage(ctx context.Context, a domain.Attempt) error {
	e.control.Lock()
	defer e.control.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.fenced {
		return domain.Fail("dispatch_fenced", "Resolve retained ownership or persistence uncertainty before local work.")
	}
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil {
		return err
	}
	task, err := e.db.Task(ctx, a.TaskID)
	if err != nil {
		return err
	}
	if err := e.db.CheckAttemptClaim(ctx, e.config.OwnerInstanceID, a.ID); err != nil {
		return err
	}
	if current.Phase != "preparing" || current.Stop != nil || current.OwnerInstanceID != e.config.OwnerInstanceID || task.Task.CurrentVersion != a.TaskVersion {
		return domain.Fail("attempt_active", "The local stage lost its current version, owner, clock or execution claim.")
	}
	now := domain.Now()
	return e.record(a.ID, domain.AttemptUpdate{Kind: "local_stage_started", StartedAt: &now})
}

func (e *Engine) runWorkflowCheck(ctx context.Context, a domain.Attempt, step domain.WorkflowStep, detail domain.TaskVersionDetail, subject workflowSubject) {
	var definition *domain.CheckDefinition
	for _, captured := range subject.Workflow.Authority.Checks {
		if step.DefinitionID != nil && captured.ID == *step.DefinitionID {
			copy := captured
			definition = &copy
			break
		}
	}
	if definition == nil {
		e.settleUnlaunched(a, primary("check_unavailable", "The local stage has no matching captured check definition."))
		return
	}
	member, err := e.graphCheckContextMembership(ctx, a)
	if err != nil {
		e.settleUnlaunched(a, primary("graph_input_stale", "The exact local check membership cannot be verified."), err)
		return
	}
	var graphContext *graphCheckCommandContext
	if member != nil {
		final, err := e.graphFinalContextForDispatch(ctx, a, subject)
		if err != nil {
			e.settleUnlaunched(a, primary("graph_input_stale", "The local check has no exact current cumulative basis."), err)
			return
		}
		graphContext = &graphCheckCommandContext{Final: final, EffectsKnown: true}
	}
	settlePreparation := func(failure *domain.PrimaryFailure, causes ...error) {
		if graphContext == nil {
			e.settleUnlaunched(a, failure, causes...)
			return
		}
		if closeErr := graphContext.close(); closeErr != nil {
			causes = append(causes, closeErr)
		}
		for _, cause := range causes {
			if uncertainEvidence(cause) && (graphContext.Snapshot != nil || graphContext.Context != nil || !graphContext.EffectsKnown) {
				graphContext.EffectsKnown = false
			}
		}
		e.settleUnlaunchedEffects(a, failure, graphContext.EffectsKnown, graphContext.effectRefs(), true, causes...)
	}
	defer func() {
		if closeErr := graphContext.close(); closeErr != nil {
			e.fence(a.ID, "graph_context_close_uncertain", "The local graph context handle could not close cleanly; retain its exact ownership.")
		}
	}()
	var final *graphFinalVerification
	if graphContext != nil {
		final = graphContext.Final
	}
	applicability, err := e.workflowApplicability(ctx, subject, definition.Scope, definition.CoveredConditionKeys, definition.DefinitionSHA256, final)
	if err != nil {
		settlePreparation(primary("evidence_unavailable", "The actual check subject basis cannot be verified."), err)
		return
	}
	started := domain.Now()
	receipt := domain.CheckReceipt{ID: domain.NewID(), WorkflowID: a.WorkflowID, StepID: a.StepID, AttemptID: a.ID, DefinitionID: definition.ID, DefinitionSHA256: definition.DefinitionSHA256, InputCandidateBinding: subject.Binding, InputResponseBinding: subject.Response, Applicability: applicability, StartedAt: &started, CreatedAt: started, Outcome: "unavailable", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}}
	settlement := domain.Settlement{Outcome: "failed", NoProcessCreated: true, Cleanup: receipt.Cleanup, EvidenceStatus: "complete", EffectsKnown: graphContext != nil}
	if definition.Kind == "builtin" && definition.BuiltinID == "literal_document_replace/1" {
		if graphContext != nil {
			if err = e.guardGraphCheckPreparation(ctx, a); err != nil {
				settlePreparation(primary("launch_blocked", "The local proof lost its exact graph authority."), err)
				return
			}
		}
		if err = e.guardLocalStage(ctx, a); err != nil {
			settlePreparation(primary("launch_blocked", "The local proof lost its current execution authority."), err)
			return
		}
		proofErr := e.workflowDirectProof(ctx, subject, detail.OriginalText, detail.Version)
		if final != nil {
			proofErr = e.graphFinalDirectProof(ctx, final, detail.OriginalText, detail.Version)
		}
		if proofErr == nil {
			receipt.BuiltinCompleted, receipt.Outcome = true, "passed"
			settlement.BuiltinCompleted, settlement.UsableOutput, settlement.Outcome = true, true, "succeeded"
		} else {
			settlement.Failure = primary("check_unavailable", "The previously admitted exact document proof could not be reproduced from current retained evidence.")
		}
	} else if definition.Kind == "command" && definition.Command != nil {
		var snapshot *workspace.Snapshot
		var captureErr error
		if graphContext == nil {
			snapshot, captureErr = e.handoffWorkflowSubject(ctx, a, subject)
		} else {
			captureErr = e.prepareGraphCheckHandoff(ctx, a, subject, graphContext)
			snapshot = graphContext.Snapshot
			if captureErr == nil && (snapshot == nil || !graphContext.EffectsKnown) {
				captureErr = domain.Fail("graph_materialization_unavailable", "The local graph check has no complete acknowledged receiving source.")
			}
		}
		if captureErr != nil {
			settlePreparation(primary("source_capture_blocked", "The actual check subject could not be materialized."), captureErr)
			return
		}
		if graphContext != nil && final != nil {
			if err = e.guardGraphCheckPreparation(ctx, a); err != nil {
				settlePreparation(primary("launch_blocked", "Graph authority changed before local context copying."), err)
				return
			}
			graphContext.Context, err = e.prepareGraphCheckContext(ctx, a, final)
			if graphContext.Context != nil {
				graphContext.EffectsKnown = graphContext.EffectsKnown && graphContext.Context.EffectsKnown && graphContext.Context.EvidenceStatus != "uncertain" && !uncertainEvidence(err)
			}
			if err != nil || graphContext.Context == nil {
				settlePreparation(primary("graph_context_unavailable", "The complete final local-check context could not be prepared."), err)
				return
			}
		}
		if markerErr := e.verifyWorkflowMarkers(ctx, a, snapshot); markerErr != nil {
			settlePreparation(primary("candidate_stale", "A retained check convention marker changed."), markerErr)
			return
		}
		observed, commandErr := e.executeWorkflowCommand(ctx, a, snapshot, *definition.Command, graphContext)
		if graphContext != nil && uncertainEvidence(commandErr) {
			graphContext.EffectsKnown = false
		}
		if !observed.Started && observed.Settlement.Cleanup.State == "" {
			settlePreparation(primary("launch_failed", "The captured local check could not be prepared."), commandErr)
			return
		}
		settlement = observed.Settlement
		receipt.ObservedExecutable = definition.Command
		receipt.StartedAt, receipt.FinishedAt = &observed.StartedAt, &observed.FinishedAt
		if graphContext != nil && !observed.Started {
			receipt.StartedAt = nil
		}
		receipt.RootTerminal, receipt.InputComplete, receipt.OutputComplete, receipt.ExitCode, receipt.Cleanup = settlement.RootTerminal, settlement.InputComplete, observed.OutputComplete, settlement.ExitCode, settlement.Cleanup
		if domain.ValidID(observed.Stdout.ID) {
			receipt.StdoutArtifactID = &observed.Stdout.ID
		}
		if domain.ValidID(observed.Stderr.ID) {
			receipt.StderrArtifactID = &observed.Stderr.ID
		}
		if commandErr == nil && settlement.RootTerminal && settlement.InputComplete && receipt.OutputComplete && settlement.Cleanup.State == "verified" && settlement.ExitCode != nil {
			receipt.Outcome = "failed"
			settlement.Outcome = "failed"
			if slices.Contains(definition.Command.SuccessExitCodes, *settlement.ExitCode) {
				receipt.Outcome, settlement.Outcome = "passed", "succeeded"
			}
		}
		candidate, captureErr := e.retainLocalCandidate(context.Background(), a, snapshot)
		if captureErr != nil {
			settlement.EvidenceStatus = "incomplete"
			settlement.Failure = primary("candidate_incomplete", "The check's actual post-command source and index could not be retained completely.")
			if uncertainEvidence(captureErr) {
				settlement.EvidenceStatus = "uncertain"
				e.fence(a.ID, "candidate_persistence_uncertain", "The post-check source/index could not be acknowledged durably.")
			}
		} else if candidate.HasDelta || candidate.HasIndexDelta {
			receipt.SourceChanged, receipt.ResultingCandidateID = true, &candidate.ID
		}
	} else {
		settlePreparation(primary("check_unavailable", "The captured check executor is unsupported."))
		return
	}
	if graphContext != nil {
		settlement.EffectsKnown = graphContext.EffectsKnown
		if graphContext.Context != nil {
			verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			_, contextErr := e.workspace.RecheckGraphContext(verifyCtx, graphContext.Context.Prepared)
			cancel()
			if contextErr != nil {
				graphContext.EffectsKnown, settlement.EffectsKnown = false, false
			}
		}
		if closeErr := graphContext.close(); closeErr != nil {
			settlement.EffectsKnown = false
		}
		if !settlement.EffectsKnown {
			receipt.Outcome, receipt.BuiltinCompleted = "unavailable", false
			settlement.EvidenceStatus, settlement.Outcome = "uncertain", "blocked"
			settlement.Failure = primary("graph_context_evidence_uncertain", "The local check's exact receiving or context effects remain unresolved.")
			e.fence(a.ID, "graph_context_evidence_uncertain", "Preserve the local graph check claim independently of verified process cleanup.")
		}
	}
	finished := domain.Now()
	if receipt.FinishedAt == nil {
		receipt.FinishedAt = &finished
	}
	facts := domain.WorkflowStepFacts{StepID: step.ID, OwnerInstanceID: e.config.OwnerInstanceID, CandidateBinding: subject.Binding, ResponseBinding: subject.Response, Applicability: &applicability, Check: &receipt, Settlement: settlement}
	facts.DirectProven = e.workflowDirectProof(ctx, subject, detail.OriginalText, detail.Version) == nil
	if final != nil {
		facts.CumulativeDirectProven = e.graphFinalDirectProof(ctx, final, detail.OriginalText, detail.Version) == nil
	}
	e.finishWorkflowLocal(a, subject, facts, graphContext)
}

func (e *Engine) retainLocalCandidate(ctx context.Context, a domain.Attempt, snapshot *workspace.Snapshot) (domain.Candidate, error) {
	var out domain.Candidate
	candidate, captureErr := e.workspace.Candidate(ctx, snapshot)
	if candidate == nil {
		return out, captureErr
	}
	index, indexErr := e.workspace.RetainIndex(ctx, a.ID, snapshot)
	if indexErr != nil {
		return out, errors.Join(captureErr, indexErr)
	}
	relative, err := filepath.Rel(e.lock.Root(), candidate.WorkspacePath)
	if err != nil {
		return out, err
	}
	changed, indexChanged := workflowChangeFlags(candidate.Changes)
	completeness := "complete"
	if candidate.Partial || captureErr != nil {
		completeness = "partial"
	}
	out = domain.Candidate{ID: candidate.ID, AttemptID: a.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: candidate.ManifestArtifactID, DeltaArtifactID: candidate.DeltaArtifactID, IndexArtifactID: index.ID, WorkspaceRef: filepath.ToSlash(relative), HasDelta: changed, HasIndexDelta: indexChanged, Completeness: completeness, CreatedAt: domain.Now()}
	if err = e.db.PutCandidate(ctx, out); err != nil {
		return out, errors.Join(captureErr, err)
	}
	saved, readErr := e.db.Candidate(ctx, out.ID)
	return saved, errors.Join(captureErr, readErr)
}

func (e *Engine) finishWorkflowLocal(a domain.Attempt, subject workflowSubject, facts domain.WorkflowStepFacts, graph ...*graphCheckCommandContext) {
	if len(graph) > 1 {
		e.fence(a.ID, "evidence_uncertain", "A local settlement received ambiguous graph context ownership.")
		return
	}
	var graphContext *graphCheckCommandContext
	if len(graph) == 1 {
		graphContext = graph[0]
	}
	e.finishWorkflowLocalContext(a, subject, facts, graphContext, nil)
}

// Final delivery carries cumulative provenance without inventing a check
// receiving workspace, materialization receipt or context-copy holder.
func (e *Engine) finishWorkflowLocalFinal(a domain.Attempt, subject workflowSubject, facts domain.WorkflowStepFacts, final *graphFinalVerification) {
	e.finishWorkflowLocalContext(a, subject, facts, nil, final)
}

func (e *Engine) finishWorkflowLocalContext(a domain.Attempt, subject workflowSubject, facts domain.WorkflowStepFacts, graphContext *graphCheckCommandContext, capturedFinal *graphFinalVerification) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil {
		e.fence(a.ID, "storage_failed", "The local settlement cannot inspect its owned attempt.")
		return
	}
	verifiedSubject := subject
	if facts.Check != nil && facts.Check.SourceChanged && facts.Check.ResultingCandidateID != nil {
		changed, changedErr := e.candidateWorkflowSubject(ctx, subject.Workflow, *facts.Check.ResultingCandidateID)
		if changedErr != nil {
			e.fence(a.ID, "evidence_uncertain", "The check-created candidate cannot be verified.")
			return
		}
		facts.ResultCandidateBinding = changed.Binding
		verifiedSubject = changed
	}
	facts.HasTaskDelta, facts.HasTaskIndexDelta = workflowChangeFlags(workspace.CompareManifests(verifiedSubject.RootManifest, verifiedSubject.Manifest))
	if graphContext != nil && graphContext.Final != nil {
		capturedFinal = graphContext.Final
	}
	var final *graphFinalVerification
	var graphErr, finalScopeErr error
	if capturedFinal != nil && facts.ResultCandidateBinding == nil {
		final, finalScopeErr = e.graphFinalContextForDispatch(ctx, a, subject)
		if finalScopeErr == nil && (final == nil || final.BindingRef != capturedFinal.BindingRef || final.Binding.BindingSHA256 != capturedFinal.Binding.BindingSHA256) {
			finalScopeErr = domain.Fail("graph_input_stale", "The local stage's cumulative final basis changed before settlement.")
		}
		if finalScopeErr == nil {
			plan, planErr := e.graphFinalPlanFacts(ctx, final, final.version.OriginalText, final.version.Version)
			if planErr != nil {
				finalScopeErr = planErr
			} else if plan == nil {
				finalScopeErr = domain.Fail("evidence_unavailable", "The exact cumulative final proof could not be reconstructed.")
			} else {
				facts.CumulativeDirectProven = plan.CumulativeDirectProven
			}
		}
		if finalScopeErr != nil {
			final = nil
			facts.CumulativeDirectProven = false
			facts.ReuseEvidenceUses = nil
			facts.Settlement.Outcome, facts.Settlement.EvidenceStatus = "blocked", "incomplete"
			failure := domain.SafeError(finalScopeErr)
			facts.Settlement.Failure = primary(failure.Code, failure.Message)
			if facts.Check != nil {
				facts.Check.Outcome, facts.Check.BuiltinCompleted = "unavailable", false
			}
			if uncertainEvidence(finalScopeErr) {
				graphErr = finalScopeErr
			}
		}
	}
	if facts.ResultCandidateBinding != nil {
		// A changed check result receives its own new capture below. Its input
		// final is provenance and cannot supply direct proof for the new output.
		facts.CumulativeDirectProven = false
	}
	var reuseIDs []string
	if finalScopeErr == nil {
		facts.ReuseEvidenceUses, _, _, reuseIDs, err = e.currentWorkflowEvidence(ctx, verifiedSubject, final)
		if err != nil {
			e.fence(a.ID, "evidence_uncertain", "Current local evidence applicability cannot be verified.")
			return
		}
	}
	if graphContext != nil && graphContext.Snapshot != nil {
		output, outputErr := e.captureGraphStepOutput(ctx, graphStepOutputRequest{Attempt: a, Snapshot: graphContext.Snapshot, CandidateBinding: facts.ResultCandidateBinding})
		facts.GraphOutput = output
		graphErr = errors.Join(graphErr, outputErr)
		if output != nil && output.FinalVerification != nil {
			// Before terminal association there is no selected new-final context.
			facts.ReuseEvidenceUses = nil
			proven, directErr := e.graphCapturedFinalDirectProof(ctx, a, output)
			facts.CumulativeDirectProven = proven
			graphErr = errors.Join(graphErr, directErr)
		}
	}
	suppliedProofs, suppliedProofErr := mergeGraphAcceptanceProofs(nil, facts.VerifiedArtifacts)
	ids := append(subject.artifactIDs(), verifiedSubject.artifactIDs()...)
	ids = append(ids, reuseIDs...)
	for _, proof := range suppliedProofs {
		ids = append(ids, proof.ArtifactID)
	}
	if graphContext != nil {
		for _, ref := range graphContext.effectRefs() {
			ids = append(ids, ref.ArtifactID)
		}
	}
	if facts.Operation != nil {
		for _, destination := range []*domain.DestinationEvidence{facts.Operation.BeforeDestination, facts.Operation.AfterDestination} {
			if destination != nil {
				ids = append(ids, destination.ManifestArtifactID)
			}
		}
		if facts.Operation.PlanArtifactID != "" {
			ids = append(ids, facts.Operation.PlanArtifactID)
		}
	}
	if current.SourceSnapshotID != nil {
		source, readErr := e.db.SourceSnapshot(ctx, *current.SourceSnapshotID)
		if readErr != nil {
			e.fence(a.ID, "storage_failed", "The local source evidence cannot be inspected.")
			return
		}
		ids = append(ids, source.ManifestArtifactID, source.IndexArtifactID)
	}
	if current.CandidateID != nil {
		candidate, readErr := e.db.Candidate(ctx, *current.CandidateID)
		if readErr != nil {
			e.fence(a.ID, "storage_failed", "The post-command candidate cannot be inspected.")
			return
		}
		ids = append(ids, candidate.ManifestArtifactID, candidate.DeltaArtifactID, candidate.IndexArtifactID)
	}
	appendStream := func(id *string, nonqualifying bool) error {
		if id == nil {
			return nil
		}
		metadata, readErr := e.db.Artifact(ctx, *id)
		if readErr != nil {
			return readErr
		}
		if metadata.Closed && metadata.Completeness == "complete" && metadata.SHA256 != nil {
			ids = append(ids, *id)
			return nil
		}
		if !nonqualifying {
			return domain.Fail("evidence_unavailable", "A qualifying command result has no complete output artifact.")
		}
		return nil
	}
	var streamErr error
	if facts.Check != nil {
		nonqualifying := facts.Check.Outcome != "passed" && facts.Check.Outcome != "failed"
		streamErr = errors.Join(appendStream(facts.Check.StdoutArtifactID, nonqualifying), appendStream(facts.Check.StderrArtifactID, nonqualifying))
	}
	if facts.External != nil {
		nonqualifying := facts.External.Outcome != "command_succeeded" && facts.External.Outcome != "command_failed"
		streamErr = errors.Join(streamErr, appendStream(facts.External.StdoutArtifactID, nonqualifying), appendStream(facts.External.StderrArtifactID, nonqualifying))
	}
	freshProofs, verifyErr := e.verifyWorkflowArtifacts(ctx, ids...)
	combinedProofs := append(append([]domain.VerifiedResponse{}, freshProofs...), suppliedProofs...)
	facts.VerifiedArtifacts, err = mergeGraphAcceptanceProofs(nil, combinedProofs)
	if err == nil && final != nil {
		facts.VerifiedArtifacts, err = mergeGraphAcceptanceProofs(facts.VerifiedArtifacts, final.VerifiedArtifacts)
	}
	err = errors.Join(err, verifyErr, suppliedProofErr, streamErr, graphErr)
	if err != nil {
		facts.Settlement.EvidenceStatus = "uncertain"
		facts.Settlement.Failure = primary("evidence_corrupt", "Fresh local settlement evidence could not be verified.")
		if facts.Check != nil {
			facts.Check.Outcome = "unavailable"
			facts.Check.BuiltinCompleted = false
		}
		if facts.External != nil {
			facts.External.Outcome = "uncertain"
		}
		facts.Applicability = nil
		e.fence(a.ID, "evidence_uncertain", "Local settlement evidence integrity or durability is unresolved; no successor is admitted.")
	}
	task, taskErr := e.db.Task(ctx, a.TaskID)
	view, workflowErr := e.db.Workflow(ctx, a.TaskID, domain.WorkflowSelector{WorkflowID: a.WorkflowID})
	if taskErr != nil || workflowErr != nil || view.Workflow == nil {
		e.fence(a.ID, "storage_failed", "Current local settlement revisions cannot be inspected.")
		return
	}
	facts.TaskRevision, facts.WorkflowRevision = task.Task.Revision, view.Workflow.Revision
	if deadline, parseErr := time.Parse(time.RFC3339Nano, a.DeadlineAt); parseErr == nil && !time.Now().Before(deadline) && current.Stop == nil && current.PrimaryFailure == nil && facts.Settlement.Failure == nil {
		facts.Settlement.Failure = primary("deadline_exceeded", "The captured local execution deadline expired before its required settlement completed.")
		facts.Settlement.Outcome = "failed"
	}
	e.control.Lock()
	_, err = e.db.SettleWorkflowStep(ctx, a.ID, facts)
	e.control.Unlock()
	if err != nil {
		e.fence(a.ID, "storage_failed", "The terminal local workflow result could not be acknowledged durably. Preserve its evidence; do not replay it.")
	}
}
