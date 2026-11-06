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
	gate, err := e.db.Gate(ctx)
	if err != nil {
		return err
	}
	if current.Phase != "preparing" || current.Stop != nil || current.OwnerInstanceID != e.config.OwnerInstanceID || task.Task.CurrentVersion != a.TaskVersion || gate.State != "open" || gate.ActiveAttemptID == nil || *gate.ActiveAttemptID != a.ID {
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
	applicability, err := e.workflowApplicability(ctx, subject, definition.Scope, definition.CoveredConditionKeys, definition.DefinitionSHA256)
	if err != nil {
		e.settleUnlaunched(a, primary("evidence_unavailable", "The actual check subject basis cannot be verified."), err)
		return
	}
	started := domain.Now()
	receipt := domain.CheckReceipt{ID: domain.NewID(), WorkflowID: a.WorkflowID, StepID: a.StepID, AttemptID: a.ID, DefinitionID: definition.ID, DefinitionSHA256: definition.DefinitionSHA256, InputCandidateBinding: subject.Binding, InputResponseBinding: subject.Response, Applicability: applicability, StartedAt: &started, CreatedAt: started, Outcome: "unavailable", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}}
	settlement := domain.Settlement{Outcome: "failed", NoProcessCreated: true, Cleanup: receipt.Cleanup, EvidenceStatus: "complete"}
	if definition.Kind == "builtin" && definition.BuiltinID == "literal_document_replace/1" {
		if err = e.guardLocalStage(ctx, a); err != nil {
			e.settleUnlaunched(a, primary("launch_blocked", "The local proof lost its current execution authority."), err)
			return
		}
		proofErr := e.workflowDirectProof(ctx, subject, detail.OriginalText, detail.Version)
		if proofErr == nil {
			receipt.BuiltinCompleted, receipt.Outcome = true, "passed"
			settlement.BuiltinCompleted, settlement.UsableOutput, settlement.Outcome = true, true, "succeeded"
		} else {
			settlement.Failure = primary("check_unavailable", "The previously admitted exact document proof could not be reproduced from current retained evidence.")
		}
	} else if definition.Kind == "command" && definition.Command != nil {
		snapshot, captureErr := e.handoffWorkflowSubject(ctx, a, subject)
		if captureErr != nil {
			e.settleUnlaunched(a, primary("source_capture_blocked", "The actual check subject could not be materialized."), captureErr)
			return
		}
		if markerErr := e.verifyWorkflowMarkers(ctx, a, snapshot); markerErr != nil {
			e.settleUnlaunched(a, primary("candidate_stale", "A retained check convention marker changed."), markerErr)
			return
		}
		observed, commandErr := e.executeWorkflowCommand(ctx, a, snapshot, *definition.Command)
		if !observed.Started && observed.Settlement.Cleanup.State == "" {
			e.settleUnlaunched(a, primary("launch_failed", "The captured local check could not be prepared."), commandErr)
			return
		}
		settlement = observed.Settlement
		receipt.ObservedExecutable = definition.Command
		receipt.StartedAt, receipt.FinishedAt = &observed.StartedAt, &observed.FinishedAt
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
		e.settleUnlaunched(a, primary("check_unavailable", "The captured check executor is unsupported."))
		return
	}
	finished := domain.Now()
	if receipt.FinishedAt == nil {
		receipt.FinishedAt = &finished
	}
	facts := domain.WorkflowStepFacts{StepID: step.ID, OwnerInstanceID: e.config.OwnerInstanceID, CandidateBinding: subject.Binding, ResponseBinding: subject.Response, Applicability: &applicability, Check: &receipt, Settlement: settlement}
	facts.DirectProven = e.workflowDirectProof(ctx, subject, detail.OriginalText, detail.Version) == nil
	e.finishWorkflowLocal(a, subject, facts)
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

func (e *Engine) finishWorkflowLocal(a domain.Attempt, subject workflowSubject, facts domain.WorkflowStepFacts) {
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
	var reuseIDs []string
	facts.ReuseEvidenceUses, _, _, reuseIDs, err = e.currentWorkflowEvidence(ctx, verifiedSubject)
	if err != nil {
		e.fence(a.ID, "evidence_uncertain", "Current local evidence applicability cannot be verified.")
		return
	}
	ids := append(subject.artifactIDs(), verifiedSubject.artifactIDs()...)
	ids = append(ids, reuseIDs...)
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
	facts.VerifiedArtifacts, err = e.verifyWorkflowArtifacts(ctx, ids...)
	err = errors.Join(err, streamErr)
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
