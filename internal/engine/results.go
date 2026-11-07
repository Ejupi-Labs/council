package engine

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ejupi-Labs/council/v2/internal/adapters"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/project"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

func primary(code, message string) *domain.PrimaryFailure {
	return &domain.PrimaryFailure{Code: code, Message: message, ObservedAt: domain.Now()}
}
func (e *Engine) record(id string, u domain.AttemptUpdate) error {
	_, err := e.db.RecordAttempt(context.Background(), id, u)
	return err
}
func (e *Engine) recordFailure(id string, f *domain.PrimaryFailure) {
	e.control.Lock()
	err := e.record(id, domain.AttemptUpdate{Kind: "failure_observed", Failure: f})
	e.control.Unlock()
	if err != nil {
		e.fence(id, "storage_failed", "A known execution failure could not be retained. Do not replay the attempt.")
	}
}

type protocolWriter struct {
	engine *Engine
	id     string
	raw    *evidence.Stream
	parser adapters.Parser
}

func (w protocolWriter) Write(b []byte) (int, error) {
	n, err := w.raw.Write(b)
	if err != nil {
		return n, err
	}
	w.engine.control.Lock()
	defer w.engine.control.Unlock()
	n, err = w.parser.Write(b)
	if err != nil {
		r := w.parser.Result()
		if r.Failure != nil {
			if recordErr := w.engine.record(w.id, domain.AttemptUpdate{Kind: "failure_observed", Failure: r.Failure}); recordErr != nil {
				w.engine.fenceLocked(w.id, "storage_failed", "A protocol failure could not be retained durably. Further dispatch remains fenced.")
			}
		}
	}
	return n, err
}

func (e *Engine) run(parent context.Context, a domain.Attempt) {
	graphNative, graphEffectsKnown := false, true
	var receiving *graphReceivingSource
	var graphContext *graphNativeContext
	contextClosed := false
	closeGraphContext := func() error {
		if graphContext == nil || contextClosed {
			return nil
		}
		contextClosed = true
		err := graphContext.Close()
		if err != nil {
			graphEffectsKnown = false
		}
		return err
	}
	graphEffectRefs := func() []domain.GraphArtifactRef {
		if graphContext != nil && graphContext.Prepared.EffectsArtifact != nil {
			return []domain.GraphArtifactRef{*graphContext.Prepared.EffectsArtifact}
		}
		return nil
	}
	settlePreparation := func(f *domain.PrimaryFailure, causes ...error) {
		if !graphNative {
			e.settleUnlaunched(a, f, causes...)
			return
		}
		if closeErr := closeGraphContext(); closeErr != nil {
			causes = append(causes, closeErr)
		}
		e.settleUnlaunchedEffects(a, f, graphEffectsKnown, graphEffectRefs(), true, causes...)
	}
	defer func() {
		if err := closeGraphContext(); err != nil {
			e.fence(a.ID, "graph_context_close_uncertain", "The private graph context handle did not close cleanly; preserve its exact preparation ownership.")
		}
	}()
	deadline, err := time.Parse(time.RFC3339Nano, a.DeadlineAt)
	if err != nil {
		settlePreparation(primary("deadline_exceeded", "The captured deadline is invalid."))
		return
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	detail, err := e.db.TaskVersion(ctx, a.TaskID, a.TaskVersion)
	if err != nil {
		failure := primary("storage_failed", "The captured task version cannot be read.")
		if interrupted := ctx.Err(); interrupted != nil {
			failure = primary("preparation_interrupted", "Task preparation ended before its captured version was read.")
			err = interrupted // A cancelled read attempted no metadata commit.
		}
		settlePreparation(failure, err)
		return
	}
	if a.ExecutorKind == "local_graph" {
		e.runGraphLocal(ctx, a)
		return
	}
	if scopeErr := e.db.CheckStandaloneAuthority(ctx, a.TaskID, false); scopeErr != nil {
		if scope := domain.SafeError(scopeErr); scope.GraphRef != nil {
			if scope.GraphRef.Kind == "node" && a.ExecutorKind == "local_check" && a.Role == "check" {
				// This exact local role enters its own guarded node-subject path.
			} else if scope.GraphRef.Kind == "node" && a.ExecutorKind == "local_external" && a.Role == "external_delivery" {
				// This command requires exact captured final-only publication scope.
			} else if scope.GraphRef.Kind == "node" && a.ExecutorKind == "local_application" && a.Role == "local_delivery" {
				// The application requires its separate verified graph delivery binding.
			} else if scope.GraphRef.Kind == "node" && a.ExecutorKind == "native" && (a.Role == "author" || a.Role == "review" || a.Role == "correction") {
				graphNative = true
			} else {
				settlePreparation(primary("capability_unavailable", "This graph executor has no verified receiving path. No ordinary task executor was substituted."))
				return
			}
		} else {
			settlePreparation(primary("preparation_interrupted", "Execution scope could not be inspected before preparation."), scopeErr)
			return
		}
	}
	if a.WorkflowID != "" && a.ExecutorKind != "native" {
		e.runWorkflowLocal(ctx, a, detail)
		return
	}
	derivedRole := a.Role == "review" || a.Role == "correction"
	if a.ResolvedTarget == nil || a.RoutingDecisionID == nil || a.ResolvedTarget.ConnectionID != a.ConnectionID {
		settlePreparation(primary("pin_unavailable", "The captured task target does not match its admitted native connection. No target was substituted."))
		return
	}
	p, identity, err := e.db.Project(ctx, mustTaskProject(e.db, ctx, a.TaskID))
	if err != nil {
		settlePreparation(primary("project_unavailable", "The saved project cannot be read."))
		return
	}
	if !graphNative {
		if err = project.ValidateIdentity(p.ResolvedPath, identity); err != nil {
			settlePreparation(primary("project_identity_changed", "The saved project identity changed. Restore its original folder before another explicit attempt."))
			return
		}
	}
	view, err := e.db.TaskRouting(ctx, a.TaskID)
	if err != nil || view.Authority == nil {
		settlePreparation(primary("routing_adoption_required", "This task has no captured routing authority. Adopt it explicitly before starting new work."), err)
		return
	}
	if view.Authority.TaskVersion != a.TaskVersion {
		settlePreparation(primary("guidance_changed", "Guidance changed before dispatch. Start against the current version explicitly."))
		return
	}
	assessment := view.Authority.SuitabilityAssessment
	if sourceErr := recheckNativeOriginal(ctx, p.ResolvedPath, assessment, derivedRole || graphNative); sourceErr != nil {
		settlePreparation(primary(sourceErr.Code, sourceErr.Message))
		return
	}
	if stopped, routingErr := e.guardRouting(ctx, a, assessment); routingErr != nil || stopped {
		if routingErr != nil {
			settlePreparation(primary(domain.SafeError(routingErr).Code, domain.SafeError(routingErr).Message), routingErr)
		}
		return
	}
	invocationPins := detail.Version.Pins
	invocationPins.HarnessID = a.ResolvedTarget.HarnessID
	invocationPins.Model = a.ResolvedTarget.ModelSelector
	// Codex alone writes its private response file. Claude returns terminal text
	// through the parsed stdout protocol and never receives a response path.
	outputPath := ""
	if a.ConnectionID == "codex-native" {
		outputPath, err = e.evidence.OutputPath(ctx, a.ID)
		if err != nil {
			settlePreparation(primary("evidence_failed", "Cannot prepare a private final response location."), err)
			return
		}
		defer func() {
			if releaseErr := e.evidence.ReleaseOutput(a.ID, outputPath); releaseErr != nil {
				e.fence(a.ID, "response_reservation_uncertain", "The private response reservation could not be released cleanly. Preserve its evidence and inspect the retained incident before further dispatch.")
			}
		}()
	}
	resolve := e.config.Resolve
	if a.ExecutionMode == "fake" {
		resolve = func(context.Context, domain.ExecutionPins) (string, error) { return os.Executable() }
	}
	var launch adapters.Launch
	var stage *workflowNativeContext
	var snapshot *workspace.Snapshot
	if derivedRole {
		launch, stage, err = e.prepareWorkflowNative(ctx, a, detail, outputPath, resolve, graphNative)
	} else {
		launch, err = adapters.BuildResolvedNative(ctx, detail.OriginalText, detail.Version, *a.ResolvedTarget, outputPath, resolve)
	}
	if graphNative && stage != nil && stage.handoffAttempted {
		// A Handoff has already created receiving state even if a later control
		// guard refuses observation before a receipt exists. Only the exact
		// acknowledged association below may promote this to known ownership.
		graphEffectsKnown = false
	}
	if err != nil {
		if graphNative && stage != nil && stage.handoffAttempted {
			// A completed Handoff can precede a linked-context/schema/prompt error.
			// Retain that actual prefix before settling; it must not disappear or
			// become an invitation to reseed the original graph input.
			var prefixErr error
			receiving, graphEffectsKnown, prefixErr = e.retainGraphHandoffPrefix(ctx, a, stage)
			err = errors.Join(err, prefixErr)
		}
		settlePreparation(primary(domain.SafeError(err).Code, domain.SafeError(err).Message), err)
		return
	}
	if graphNative {
		var handoff *workspace.Snapshot
		if stage != nil {
			handoff = stage.snapshot
		}
		var preparationErr error
		receiving, preparationErr = e.prepareGraphReceivingSource(ctx, a, handoff)
		if receiving != nil && receiving.Materialized.Receipt.ID != "" {
			m := receiving.Materialized
			graphEffectsKnown = m.Receipt.EffectsKnown && m.Receipt.EvidenceStatus != "uncertain" && validGraphArtifactReference(m.ReceiptArtifact)
			if validGraphArtifactReference(m.ReceiptArtifact) {
				if recordErr := e.recordGraphReceivingSource(ctx, a, receiving); recordErr != nil {
					graphEffectsKnown = false
					preparationErr = errors.Join(preparationErr, recordErr)
				}
			}
			if stage != nil && stage.handoffAttempted {
				graphEffectsKnown = graphEffectsKnown && preparationErr == nil && m.Receipt.State == "complete" && m.Snapshot != nil
			}
		}
		if preparationErr != nil || receiving == nil || receiving.Materialized.Snapshot == nil {
			if preparationErr == nil {
				preparationErr = domain.Fail("graph_materialization_unavailable", "The graph receiver has no complete exact source.")
			}
			settlePreparation(primary("graph_materialization_unavailable", "The exact graph receiving source could not be prepared and retained."), preparationErr)
			return
		}
		snapshot = receiving.Materialized.Snapshot
		if guardErr := e.guardGraphNativePreparation(ctx, a); guardErr != nil {
			settlePreparation(primary("launch_blocked", "Graph authority changed before private context preparation."), guardErr)
			return
		}
		var final *graphFinalVerification
		if stage != nil {
			final = stage.final
		}
		launch, graphContext, err = e.prepareGraphNativeContext(ctx, a, launch, final)
		if err == nil && graphContext != nil && stage != nil && final != nil {
			for _, source := range graphContext.Prepared.Supplemental {
				scope := "whole_candidate"
				if len(source.Paths) != 0 {
					scope = "declared_subset"
				}
				stage.supplied = append(stage.supplied, domain.ReviewInspection{Ref: source.Ref, Scope: scope, Paths: append([]string{}, source.Paths...)})
			}
			paths := map[string]bool{}
			for _, name := range stage.paths {
				paths[name] = true
			}
			for _, change := range final.Changes {
				if change.Before != nil && !paths[change.Path] {
					paths[change.Path] = true
					stage.paths = append(stage.paths, change.Path)
				}
			}
		}
		if graphContext != nil {
			graphEffectsKnown = graphEffectsKnown && graphContext.EffectsKnown && graphContext.EvidenceStatus != "uncertain" && !uncertainEvidence(err)
		}
		if err != nil || graphContext == nil {
			settlePreparation(primary("graph_context_unavailable", "Complete exact graph context could not be prepared for the receiving worker."), err)
			return
		}
	}
	if a.ExecutionMode == "fake" {
		auxiliaryPath := outputPath
		if a.ConnectionID == "claude-native" {
			auxiliaryPath = e.lock.Root()
		}
		if graphNative && a.Role == "author" {
			fixtureOutput := outputPath
			if a.ConnectionID == "claude-native" {
				fixtureOutput = ""
			}
			launch.Args, err = fixture.GraphArgsWithContext(invocationPins.HarnessID, a.FakeScenario, "author", fixtureOutput, graphContext.Prepared.Directory)
		} else if graphNative && a.Role == "review" {
			fixtureOutput := outputPath
			if a.ConnectionID == "claude-native" {
				fixtureOutput = ""
			}
			launch.Args, err = fixture.GraphReviewArgsWithContext(invocationPins.HarnessID, fixtureOutput, graphContext.Prepared.Directory, snapshot.WorkspacePath)
		} else if derivedRole || a.FakeScenario == "workflow-document" || a.FakeScenario == "workflow-defect" {
			fixtureOutput := outputPath
			if a.ConnectionID == "claude-native" {
				fixtureOutput = ""
			}
			role, scenario := "author", a.FakeScenario
			if a.Role == "review" {
				role, scenario = "review", "workflow-review-blocking"
				if stage != nil && stage.step.Cycle > 0 {
					scenario = "workflow-review-clean"
				}
			} else if a.Role == "correction" {
				role, scenario = "correction", "workflow-correction"
			}
			launch.Args, err = fixture.WorkflowArgs(invocationPins.HarnessID, scenario, role, fixtureOutput)
		} else {
			launch.Args, err = fixture.ArgsForHarness(invocationPins.HarnessID, a.FakeScenario, auxiliaryPath)
		}
		if err != nil {
			settlePreparation(primary("capability_unavailable", "The requested fixed fixture is unsupported."))
			return
		}
		launch.Observations = adapters.UnknownObservations("autonomous")
		launch.Observations.Permissions.Limitations = []string{"Local fake execution; no provider or live account was invoked."}
	}
	prompt, err := e.evidence.Put(ctx, a.ID, "prompt", launch.Prompt)
	if err != nil {
		settlePreparation(primary("evidence_failed", "The complete prompt could not be retained."), err)
		return
	}
	_ = prompt
	if graphNative {
		// The exact closed receiving input was materialized and atomically retained
		// above. No current-checkout capture is permitted for this graph worker.
	} else if stage != nil {
		snapshot = stage.snapshot
	} else {
		snapshot, err = e.workspace.Capture(ctx, workspace.Request{AttemptID: a.ID, ProjectPath: p.ResolvedPath})
	}
	if err != nil {
		message := "Current source could not be captured faithfully. Inspect source prerequisites and retain the original working tree."
		if errors.Is(err, workspace.ErrBlocked) && utf8.ValidString(err.Error()) && len(err.Error()) <= 512 {
			message = err.Error()
		}
		settlePreparation(primary("source_capture_blocked", message), err)
		return
	}
	if !graphNative {
		if identityErr := project.ValidateIdentity(p.ResolvedPath, identity); identityErr != nil {
			settlePreparation(primary("project_identity_changed", "The saved project identity changed during source capture. No worker was prepared."))
			return
		}
	}
	if sourceErr := recheckNativeOriginal(ctx, p.ResolvedPath, assessment, derivedRole || graphNative); sourceErr != nil {
		settlePreparation(primary(sourceErr.Code, sourceErr.Message))
		return
	}
	if !graphNative && !derivedRole && assessment.SourceProof != nil {
		if sourceErr := routing.RecheckManaged(ctx, snapshot.ExecutionPath, *assessment.SourceProof); sourceErr != nil {
			settlePreparation(primary(sourceErr.Code, sourceErr.Message))
			return
		}
	}
	if markerErr := e.verifyWorkflowMarkers(ctx, a, snapshot); markerErr != nil {
		settlePreparation(primary("candidate_stale", "The captured check convention markers changed before native execution."), markerErr)
		return
	}
	if !graphNative {
		manifest, err := e.evidence.VerifyClosed(ctx, snapshot.ManifestArtifactID)
		if err != nil || manifest.SHA256 == nil {
			settlePreparation(primary("evidence_failed", "The captured source manifest is not verifiable."), err)
			return
		}
		var branch *string
		if snapshot.Branch != "" {
			branch = &snapshot.Branch
		}
		err = e.db.PutSourceSnapshot(ctx, domain.SourceSnapshot{ID: snapshot.ID, AttemptID: a.ID, ManifestArtifactID: snapshot.ManifestArtifactID, IndexArtifactID: snapshot.IndexArtifactID, ManifestSHA256: *manifest.SHA256, BaseCommit: snapshot.BaseCommit, Branch: branch, ProjectIdentity: identity, SelectedRelativePath: snapshot.SelectedSubfolder, CreatedAt: domain.Now()})
		if err != nil {
			failure := primary("evidence_failed", "Source evidence registration did not complete.")
			if persistenceFault(err) {
				failure = primary("storage_failed", "Source evidence registration failed.")
			}
			settlePreparation(failure, err)
			return
		}
		if stopped, routingErr := e.guardRouting(ctx, a, assessment); routingErr != nil || stopped {
			if routingErr != nil {
				settlePreparation(primary(domain.SafeError(routingErr).Code, domain.SafeError(routingErr).Message), routingErr)
			}
			return
		}
	}
	out, err := e.evidence.Create(ctx, a.ID, "stdout", evidence.StdoutLimit)
	if err != nil {
		settlePreparation(primary("evidence_failed", "Cannot create the private stdout artifact."), err)
		return
	}
	stderr, err := e.evidence.Create(ctx, a.ID, "stderr", evidence.StderrLimit)
	if err != nil {
		_, closeErr := out.Close(context.Background(), false)
		settlePreparation(primary("evidence_failed", "Cannot create the private stderr artifact."), errors.Join(err, closeErr))
		return
	}
	parserFactory := adapters.NewParser
	if a.Role == "review" {
		parserFactory = adapters.NewReviewParser
	}
	parser, err := parserFactory(a.ConnectionID, invocationPins, out.ID(), func(event adapters.Event) error {
		if err := out.Checkpoint(context.Background()); err != nil {
			return err
		}
		if a.ExecutionMode == "fake" {
			event.Observations = nil
		}
		return e.record(a.ID, domain.AttemptUpdate{Kind: event.Kind, Metadata: jsontext.Value(event.Metadata), TargetObservations: event.Observations, Failure: event.Failure})
	})
	if err != nil {
		_, outCloseErr := out.Close(context.Background(), false)
		_, stderrCloseErr := stderr.Close(context.Background(), false)
		settlePreparation(primary("capability_unavailable", "The captured native parser is unavailable."), errors.Join(err, outCloseErr, stderrCloseErr))
		return
	}
	e.prepareGate.Lock()
	if graphNative {
		guardErr := e.recheckGraphReceivingSource(ctx, a, receiving)
		if guardErr == nil {
			_, guardErr = e.recheckGraphNativeContext(ctx, a, graphContext)
		}
		if guardErr != nil {
			e.prepareGate.Unlock()
			if uncertainEvidence(guardErr) {
				graphEffectsKnown = false
			}
			_, outCloseErr := out.Close(context.Background(), false)
			_, stderrCloseErr := stderr.Close(context.Background(), false)
			settlePreparation(primary("launch_blocked", "The exact graph source or private context changed before native preparation."), errors.Join(guardErr, outCloseErr, stderrCloseErr))
			return
		}
	}
	e.control.Lock()
	_, guardErr := e.db.RecordLaunchIntent(ctx, a.ID)
	e.control.Unlock()
	if guardErr != nil {
		e.prepareGate.Unlock()
		if persistenceFault(guardErr) {
			e.fence(a.ID, "storage_failed", "The durable native preparation guard could not be acknowledged. Further dispatch remains fenced.")
		}
		_, outCloseErr := out.Close(context.Background(), false)
		_, stderrCloseErr := stderr.Close(context.Background(), false)
		settlePreparation(primary("launch_blocked", "The durable preparation guard rejected native creation. No worker was launched."), errors.Join(guardErr, outCloseErr, stderrCloseErr))
		return
	}
	prepare := process.Prepare
	if e.config.Prepare != nil {
		prepare = e.config.Prepare
	}
	owned, err := prepare(ctx, process.NativeSpec{Executable: launch.Executable, Args: launch.Args, Cwd: snapshot.ExecutionPath, Stdin: launch.Prompt, Stdout: protocolWriter{e, a.ID, out, parser}, Stderr: stderr})
	e.prepareGate.Unlock()
	if err != nil {
		if errors.Is(err, process.ErrOwnershipUncertain) {
			e.fence(a.ID, "ownership_uncertain", "Native preparation cleanup could not be verified. Do not replay or dispatch more work.")
		}
		_, outCloseErr := out.Close(context.Background(), false)
		_, stderrCloseErr := stderr.Close(context.Background(), false)
		if errors.Is(err, process.ErrOwnershipUncertain) {
			closeErr := closeGraphContext()
			settlement := domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "unknown", Scope: cancellationScope()}, EvidenceStatus: "uncertain", Failure: primary("ownership_uncertain", "Native preparation cleanup could not be verified.")}
			if graphNative {
				settlement.EffectsKnown = graphEffectsKnown && closeErr == nil
			}
			facts := e.workflowFailedSettlementFacts(a, settlement)
			if refs := graphEffectRefs(); len(refs) != 0 {
				proofs, proofErr := e.verifyGraphClosureAnchors(context.Background(), refs, refs)
				if proofErr == nil {
					facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, proofs...)
				} else {
					settlement.EffectsKnown = false
				}
			}
			e.control.Lock()
			_ = e.settleExecutionLocked(context.Background(), a, settlement, facts)
			e.control.Unlock()
		} else {
			settlePreparation(primary("launch_failed", "The native process could not be prepared. No automatic retry was made."), errors.Join(err, outCloseErr, stderrCloseErr))
		}
		return
	}
	nativeStarted := false
	if graphNative {
		err = e.recheckGraphReceivingSource(ctx, a, receiving)
		if err == nil {
			_, err = e.recheckGraphNativeContext(ctx, a, graphContext)
		}
		if uncertainEvidence(err) {
			graphEffectsKnown = false
			e.fence(a.ID, "graph_preparation_uncertain", "The receiving source or private context failed the final integrity check; retain the exact preparation claim.")
		}
	}
	e.control.Lock()
	if err == nil {
		_, err = e.db.RecordLaunchIntent(context.Background(), a.ID)
	}
	if err == nil && ctx.Err() == nil {
		err = owned.Start()
		nativeStarted = err == nil
	} else if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		at := domain.Now()
		err = e.record(a.ID, domain.AttemptUpdate{Kind: "worker_started", Phase: "running", StartedAt: &at, TargetObservations: &launch.Observations})
	}
	e.control.Unlock()
	if err != nil {
		if persistenceFault(err) {
			e.fence(a.ID, "storage_failed", "Native launch state could not be acknowledged durably. Cleanup continues and replay is prohibited.")
		}
		e.recordFailure(a.ID, primary("launch_failed", "The final launch guard or native start failed. No automatic retry was made."))
	} else {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				e.recordFailure(a.ID, primary("deadline_exceeded", "The captured execution deadline expired."))
			}
		case <-parser.Failed():
			if failure := parser.Result().Failure; failure != nil {
				e.recordFailure(a.ID, failure)
			}
		case fault := <-owned.Faults():
			if fault.Stream == "stdin" {
				e.recordFailure(a.ID, primary("input_delivery_failed", "The complete original prompt could not be delivered to the native process."))
				break
			}
			r := parser.Result()
			if r.Failure != nil {
				e.recordFailure(a.ID, r.Failure)
			} else {
				e.recordFailure(a.ID, primary("evidence_failed", "A native output collector failed or reached its retained byte limit."))
			}
		case <-owned.RootDone():
			root, ok := owned.RootOutcome()
			if !ok || !root.Known || root.ExitCode != 0 {
				e.recordFailure(a.ID, primary("native_exit_failed", "The native root exited unsuccessfully or its exit could not be verified."))
			}
		}
	}
	started := domain.Now()
	if err := e.record(a.ID, domain.AttemptUpdate{Kind: "cleanup_started", Cleanup: &domain.Cleanup{State: "active", Scope: cancellationScope(), StartedAt: &started}}); err != nil {
		e.fence(a.ID, "storage_failed", "Cleanup observation could not be retained. Dispatch remains fenced while owned cleanup continues.")
	}
	settlementDeadline := time.Now().Add(process.CleanupLimit)
	if current, err := e.db.Attempt(context.Background(), a.ID); err == nil && current.Stop != nil {
		if accepted, err := time.Parse(time.RFC3339Nano, current.Stop.AcceptedAt); err == nil && accepted.Add(process.CleanupLimit).Before(settlementDeadline) {
			settlementDeadline = accepted.Add(process.CleanupLimit)
		}
	}
	e.watchStop(a.ID, settlementDeadline.Add(-process.CleanupLimit).UTC().Format(time.RFC3339Nano))
	settlementCtx, settlementCancel := context.WithDeadline(context.Background(), settlementDeadline)
	defer settlementCancel()
	cleanupCtx, cleanupCancel := context.WithDeadline(context.Background(), settlementDeadline)
	clean := owned.Cleanup(cleanupCtx)
	cleanupCancel()
	if a.ExecutionMode == "fake" && a.FakeScenario == "cleanup_unknown" {
		clean.Verified = false
		clean.Detail = "Explicit local fixture injected unresolved cleanup; no escaped process is simulated as owned."
	}
	if clean.Verified && clean.Root != nil && clean.Root.Known && clean.StdoutDrained && clean.StderrDrained && !clean.StdoutError && !clean.StderrError {
		if _, proofErr := e.db.RecordRecoveryCleanup(settlementCtx, e.config.OwnerInstanceID, a.ID, clean); proofErr != nil {
			e.fence(a.ID, "cleanup_proof_unavailable", "Verified cleanup could not be recorded before the ownership handle closed.")
			e.recordFailure(a.ID, primary("storage_failed", "The exact owned cleanup observation could not be retained durably."))
		}
	}
	ownedCloseErr := owned.Close()
	if graphNative && ownedCloseErr != nil {
		graphEffectsKnown = false
		e.fence(a.ID, "ownership_uncertain", "The native ownership handle could not close cleanly. Preserve the exact graph claim.")
	}
	if closeErr := closeGraphContext(); closeErr != nil {
		e.fence(a.ID, "graph_context_close_uncertain", "The private graph context handle did not close cleanly. Preserve the exact preparation claim.")
		e.recordFailure(a.ID, primary("graph_context_close_uncertain", "Private graph context ownership could not be closed cleanly."))
	}
	if graphNative && graphContext != nil && graphContext.Final != nil {
		if _, contextErr := e.workspace.RecheckGraphContext(settlementCtx, graphContext.Prepared); contextErr != nil {
			graphEffectsKnown = false
			e.fence(a.ID, "graph_context_evidence_uncertain", "The exact cumulative context copies or descriptor could not be reverified after native cleanup; preserve their claim.")
			e.recordFailure(a.ID, primary("graph_context_evidence_uncertain", "The cumulative context did not retain its complete exact physical basis through native settlement."))
		}
	}
	inputComplete := nativeStarted && clean.StdinState == process.StdinComplete && !clean.StdinError && clean.StdinBytesWritten == clean.ExpectedInputBytes && clean.ExpectedInputBytes == int64(len(launch.Prompt))
	inputDelivered := inputComplete
	inputMetadata, _ := json.Marshal(map[string]any{"state": clean.StdinState, "expectedBytes": domain.Decimal(clean.ExpectedInputBytes), "writtenBytes": domain.Decimal(clean.StdinBytesWritten), "complete": inputComplete})
	inputProofErr := e.record(a.ID, domain.AttemptUpdate{Kind: "input_delivery", Metadata: inputMetadata})
	if inputProofErr != nil {
		inputComplete = false
		e.fence(a.ID, "storage_failed", "Native input-delivery proof could not be retained durably.")
		e.recordFailure(a.ID, primary("storage_failed", "Native input-delivery proof was not acknowledged durably."))
	}
	if nativeStarted && !inputDelivered {
		e.recordFailure(a.ID, primary("input_delivery_failed", "The native process did not receive the complete original prompt through its input pipe."))
	}
	protocolErr := parser.Finish()
	r := parser.Result()
	if protocolErr != nil && r.Failure != nil {
		e.recordFailure(a.ID, r.Failure)
	}
	stdoutMeta, outErr := out.Close(context.Background(), clean.StdoutDrained && !clean.StdoutError)
	_, stderrErr := stderr.Close(context.Background(), clean.StderrDrained && !clean.StderrError)
	evidenceState := "complete"
	if graphNative && !graphEffectsKnown {
		evidenceState = "uncertain"
	}
	if inputProofErr != nil {
		evidenceState = "uncertain"
	}
	if outErr != nil || stderrErr != nil || clean.StdoutError || clean.StderrError || !clean.StdoutDrained || !clean.StderrDrained {
		if evidenceState != "uncertain" {
			evidenceState = "incomplete"
		}
		e.recordFailure(a.ID, primary("evidence_failed", "Output could not be retained completely and durably."))
		if (outErr != nil && !errors.Is(outErr, evidence.ErrLimit)) || (stderrErr != nil && !errors.Is(stderrErr, evidence.ErrLimit)) {
			evidenceState = "uncertain"
			e.fence(a.ID, "evidence_failed", "Output durability is uncertain. Retained bytes must be inspected before further dispatch.")
		}
	}
	_ = stdoutMeta
	var response evidence.Metadata
	usable := false
	retainResponse := clean.Verified
	if a.ConnectionID == "claude-native" {
		retainResponse = retainResponse && protocolErr == nil && r.Terminal && r.Succeeded && (r.FinalResponse != nil || a.Role == "review" && len(r.StructuredOutput) > 0) && inputComplete && clean.Root != nil && clean.Root.Known && clean.Root.ExitCode == 0 && evidenceState == "complete"
	}
	if retainResponse {
		if a.ConnectionID == "claude-native" && a.Role == "review" {
			response, err = e.evidence.Put(settlementCtx, a.ID, "review_result", r.StructuredOutput)
		} else if a.ConnectionID == "claude-native" {
			response, err = e.evidence.Put(settlementCtx, a.ID, "final_response", []byte(*r.FinalResponse))
		} else {
			response, err = e.evidence.ImportResponse(settlementCtx, a.ID, outputPath, 64<<20)
		}
		if err == nil {
			_, err = e.evidence.VerifyClosed(settlementCtx, response.ID)
			if err == nil {
				chunk, readErr := e.readResponse(settlementCtx, response)
				usable = readErr == nil && utf8.Valid(chunk) && strings.TrimSpace(string(chunk)) != ""
				if readErr == nil && a.Role == "review" && a.ConnectionID == "codex-native" {
					response, readErr = e.evidence.Put(settlementCtx, a.ID, "review_result", chunk)
				}
				if readErr != nil {
					err = readErr
				}
			}
		}
		if err == nil {
			err = e.record(a.ID, domain.AttemptUpdate{Kind: "response_retained", ResponseArtifactID: &response.ID})
		}
		if errors.Is(err, evidence.ErrCorrupt) {
			evidenceState = "uncertain"
			e.recordFailure(a.ID, primary("evidence_corrupt", "The registered final response failed current integrity verification."))
			e.fence(a.ID, "evidence_corrupt", "Final response identity or bytes failed integrity verification. Further dispatch remains fenced.")
		}
		if uncertainEvidence(err) {
			evidenceState = "uncertain"
			e.fence(a.ID, "storage_failed", "Final response registration could not be acknowledged durably. Further dispatch remains fenced.")
		}
		if err != nil && a.ConnectionID == "claude-native" {
			if evidenceState != "uncertain" {
				evidenceState = "incomplete"
			}
			e.recordFailure(a.ID, primary("evidence_failed", "The native terminal response could not be retained and verified durably."))
		}
	}
	var candidate *workspace.Candidate
	if clean.Verified {
		candidate, err = e.workspace.Candidate(settlementCtx, snapshot)
		if candidate != nil {
			_, hasIndex := workflowChangeFlags(candidate.Changes)
			rel, relErr := filepath.Rel(e.lock.Root(), candidate.WorkspacePath)
			if relErr == nil {
				retained := domain.Candidate{ID: candidate.ID, AttemptID: a.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: candidate.ManifestArtifactID, DeltaArtifactID: candidate.DeltaArtifactID, WorkspaceRef: filepath.ToSlash(rel), HasDelta: len(candidate.Changes) > 0, HasIndexDelta: hasIndex, Verification: "pending", Integration: "not_performed", CreatedAt: domain.Now()}
				if a.WorkflowID != "" {
					retained.Completeness = "complete"
					if candidate.Partial {
						retained.Completeness = "partial"
					}
					index, indexErr := e.workspace.RetainIndex(settlementCtx, a.ID, snapshot)
					relErr = indexErr
					retained.IndexArtifactID = index.ID
				}
				if relErr == nil {
					relErr = e.db.PutCandidate(context.Background(), retained)
				}
			}
			if relErr != nil {
				err = relErr
			}
		}
		if err != nil {
			if evidenceState != "uncertain" {
				evidenceState = "incomplete"
			}
			if uncertainEvidence(err) {
				evidenceState = "uncertain"
				e.fence(a.ID, "candidate_persistence_uncertain", "Candidate evidence could not be acknowledged durably. Further dispatch is fenced; owned work is not replayed.")
			}
			e.recordFailure(a.ID, primary("candidate_incomplete", "The candidate could not be captured completely. Original project state remains authoritative."))
		}
	}
	if candidate != nil && !candidate.Partial && len(candidate.Changes) > 0 && err == nil {
		usable = true
	}
	if !usable && clean.Verified {
		e.recordFailure(a.ID, primary("result_unusable", "No complete useful final response or authored candidate could be verified. Retained partial evidence remains available."))
	}
	finished := domain.Now()
	cleanup := domain.Cleanup{State: "verified", Scope: cancellationScope(), StartedAt: &started, FinishedAt: &finished}
	if !clean.Verified {
		cleanup.State = "unknown"
		e.fence(a.ID, "cleanup_unresolved", "Owned process cleanup could not be verified. Capacity remains held; do not replay or dispatch more work.")
	}
	var code *int
	if clean.Root != nil && clean.Root.Known {
		code = &clean.Root.ExitCode
	}
	outcome := "succeeded"
	// The store applies ordered primary failure/Stop first, then enforces terminal,
	// root exit, usable durable evidence and scoped cleanup before success.
	if (candidate == nil || err != nil) && evidenceState != "uncertain" {
		evidenceState = "incomplete"
	}
	settlement := domain.Settlement{Outcome: outcome, Cleanup: cleanup, EvidenceStatus: evidenceState, RootTerminal: r.Terminal && r.Succeeded, InputComplete: inputComplete, ExitCode: code, UsableOutput: usable}
	if graphNative {
		settlement.EffectsKnown = graphEffectsKnown
	}
	var workflowFacts domain.WorkflowStepFacts
	if a.WorkflowID != "" {
		var factsErr error
		workflowFacts, factsErr = e.workflowNativeFacts(settlementCtx, a, stage, settlement, snapshot)
		if factsErr != nil {
			failure := domain.SafeError(factsErr)
			settlement.Failure = primary(failure.Code, failure.Message)
			settlement.EvidenceStatus = "incomplete"
			if uncertainEvidence(factsErr) {
				settlement.EvidenceStatus = "uncertain"
				e.fence(a.ID, "workflow_evidence_uncertain", "Workflow result integrity or persistence could not be established; no stage is replayed.")
			}
		}
	}
	// Only a controller-verified complete standalone author workspace may
	// receive a recovery seal. Normal work does not need this extra journal.
	if !graphNative && a.Role == "author" && workflowFacts.CandidateBinding != nil && settlement.EvidenceStatus == "complete" && clean.Verified {
		e.control.Lock()
		recoveryFenced := e.fenced
		e.control.Unlock()
		if recoveryFenced {
			if sealErr := e.db.RecordRecoveryStandaloneSeal(settlementCtx, e.config.OwnerInstanceID, a.ID, *workflowFacts.CandidateBinding, workflowFacts.VerifiedArtifacts); sealErr != nil && persistenceFault(sealErr) {
				e.fence(a.ID, "recovery_seal_uncertain", "Standalone recovery evidence could not be retained durably; preserve the claim.")
			}
		}
	}
	if graphNative && len(graphEffectRefs()) != 0 {
		proofs, proofErr := e.verifyGraphClosureAnchors(settlementCtx, graphEffectRefs(), graphEffectRefs())
		if proofErr != nil {
			settlement.EvidenceStatus, settlement.EffectsKnown = "uncertain", false
			settlement.Failure = primary("graph_context_evidence_uncertain", "The exact preparation-effects reference could not be verified before terminal settlement.")
			e.fence(a.ID, "graph_context_evidence_uncertain", "Graph preparation effects cannot be acknowledged completely; preserve the exact claim.")
		} else {
			workflowFacts.VerifiedArtifacts = append(workflowFacts.VerifiedArtifacts, proofs...)
		}
	}
	e.control.Lock()
	settleErr := e.settleExecutionLocked(context.Background(), a, settlement, workflowFacts)
	e.control.Unlock()
	if settleErr != nil {
		e.fence(a.ID, "storage_failed", "Terminal execution state could not be acknowledged. Retain its evidence and do not replay.")
	}
}

func (e *Engine) guardGraphNativePreparation(ctx context.Context, a domain.Attempt) error {
	e.control.Lock()
	defer e.control.Unlock()
	if e.fenced {
		return domain.Fail("dispatch_fenced", "Resolve graph preparation ownership before creating private receiving state.")
	}
	return e.db.CheckGraphDispatchPreparation(ctx, e.config.OwnerInstanceID, a.ID)
}

// A prompt failure cannot erase a Handoff that already completed. This performs
// only exact selected-source observation and metadata settlement, never reseeding
// the graph input or preparing another process. Lack of acknowledgement keeps
// ownership unresolved even when process creation never happened.
func (e *Engine) retainGraphHandoffPrefix(parent context.Context, a domain.Attempt, stage *workflowNativeContext) (*graphReceivingSource, bool, error) {
	if stage == nil || !stage.handoffAttempted || !stage.effectsKnown || stage.snapshot == nil {
		return nil, false, domain.Fail("graph_materialization_unavailable", "The attempted graph Handoff has no complete acknowledged source to observe.")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
	defer cancel()
	receiving, observeErr := e.prepareGraphReceivingSource(ctx, a, stage.snapshot)
	if receiving == nil || !validGraphArtifactReference(receiving.Materialized.ReceiptArtifact) {
		if observeErr == nil {
			observeErr = domain.Fail("graph_materialization_unavailable", "The actual Handoff has no acknowledged exact materialization receipt.")
		}
		return receiving, false, observeErr
	}
	recordErr := e.recordGraphReceivingSource(ctx, a, receiving)
	m := receiving.Materialized
	known := observeErr == nil && recordErr == nil && m.Receipt.State == "complete" && m.Receipt.EffectsKnown && m.Receipt.EvidenceStatus == "complete" && m.Snapshot != nil
	return receiving, known, errors.Join(observeErr, recordErr)
}

func mustTaskProject(db *store.Store, ctx context.Context, id string) string {
	detail, err := db.Task(ctx, id)
	if err != nil {
		return ""
	}
	return detail.Task.ProjectID
}

func (e *Engine) readAllResponse(m evidence.Metadata) ([]byte, error) {
	return e.readResponse(context.Background(), m)
}
func (e *Engine) readResponse(ctx context.Context, m evidence.Metadata) ([]byte, error) {
	var body bytes.Buffer
	_, err := e.evidence.CopyTo(ctx, m.ID, &body, evidence.StdoutLimit)
	if err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func (e *Engine) settleUnlaunched(a domain.Attempt, f *domain.PrimaryFailure, causes ...error) {
	effectsKnown := a.ExecutorKind == "local_application"
	// Generic callers have created no graph receiving/context state. Once such
	// I/O begins, run uses its explicit holder-grounded settlement path instead.
	if a.ExecutorKind == "native" || graphLocalCommandRole(a) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		detail, err := e.db.Task(ctx, a.TaskID)
		cancel()
		if err == nil && detail.Task.GraphRef != nil && detail.Task.GraphRef.Kind == "node" {
			effectsKnown = true
		}
	}
	e.settleUnlaunchedEffects(a, f, effectsKnown, nil, false, causes...)
}

func (e *Engine) settleUnlaunchedEffects(a domain.Attempt, f *domain.PrimaryFailure, effectsKnown bool, effectRefs []domain.GraphArtifactRef, graphPreparation bool, causes ...error) {
	evidenceState := "incomplete"
	for _, cause := range causes {
		if uncertainEvidence(cause) {
			evidenceState = "uncertain"
			e.fence(a.ID, "preparation_persistence_uncertain", "Preparation evidence or persistence could not be verified. Dispatch remains fenced and no worker is replayed.")
		}
	}
	if f != nil && f.Code == "storage_failed" {
		evidenceState = "uncertain"
		e.fence(a.ID, "storage_failed", "Preparation state could not be read or acknowledged durably. Dispatch remains fenced.")
	}
	if graphPreparation && !effectsKnown {
		evidenceState = "uncertain"
		e.fence(a.ID, "graph_preparation_uncertain", "Graph preparation effects remain unresolved; preserve the exact receiving claim independently of process cleanup.")
	}
	outcome := "blocked"
	if deadline, err := time.Parse(time.RFC3339Nano, a.DeadlineAt); err == nil && !time.Now().Before(deadline) {
		f = primary("deadline_exceeded", "The captured execution deadline expired before worker instructions began.")
		outcome = "failed"
	}
	// This helper is used only before any application effect permit is issued.
	settlement := domain.Settlement{NoProcessCreated: true, EffectsKnown: effectsKnown, Outcome: outcome, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: evidenceState, Failure: f}
	facts := e.workflowFailedSettlementFacts(a, settlement)
	if len(effectRefs) != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		proofs, err := e.verifyGraphClosureAnchors(ctx, effectRefs, effectRefs)
		cancel()
		if err != nil {
			settlement.EvidenceStatus, settlement.EffectsKnown = "uncertain", false
			e.fence(a.ID, "graph_context_evidence_uncertain", "Private context effects could not be verified for settlement; retain their exact claim.")
		} else {
			facts.VerifiedArtifacts = append(facts.VerifiedArtifacts, proofs...)
		}
	}
	e.control.Lock()
	err := e.settleExecutionLocked(context.Background(), a, settlement, facts)
	e.control.Unlock()
	if err != nil {
		e.fence(a.ID, "storage_failed", "Preparation could not be settled durably. No automatic retry is permitted.")
	}
}

func (e *Engine) AcceptResponse(ctx context.Context, id string, in domain.ResponseAcceptanceInput) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	if previous, err := e.db.Replay(ctx, in.RequestID, "tasks.response.accept", store.ResponseAcceptanceFingerprint(id, in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if previous != nil {
		return *previous
	}
	registered, registrationErr := e.db.Artifact(ctx, in.ResponseArtifactID)
	if registrationErr != nil {
		return e.failure(in.RequestID, registrationErr)
	}
	attempt, attemptErr := e.db.Attempt(ctx, in.AttemptID)
	if attemptErr != nil {
		return e.failure(in.RequestID, attemptErr)
	}
	if registered.AttemptID != in.AttemptID || registered.Kind != "final_response" || !registered.Closed || registered.Completeness != "complete" || registered.SHA256 == nil || attempt.TaskID != id || attempt.ResponseArtifactID == nil || *attempt.ResponseArtifactID != registered.ID {
		return e.failure(in.RequestID, domain.Fail("acceptance_unavailable", "Select the exact complete final response belonging to this task attempt."))
	}
	m, err := e.evidence.VerifyClosed(ctx, in.ResponseArtifactID)
	if err != nil || m.SHA256 == nil {
		if errors.Is(err, evidence.ErrCorrupt) || (err == nil && m.SHA256 == nil) {
			e.fence(in.AttemptID, "evidence_corrupt", "A response failed fresh integrity verification. Dispatch remains fenced; the task was not accepted.")
		}
		return e.failure(in.RequestID, domain.Fail("acceptance_unavailable", "Fresh response verification did not complete or its integrity failed. Preserve its evidence and retry the same acceptance identity after inspection."))
	}
	workflowView, workflowErr := e.db.Workflow(ctx, id, domain.WorkflowSelector{})
	if workflowErr != nil {
		return e.failure(in.RequestID, workflowErr)
	}
	if workflowView.Workflow != nil {
		progress, progressErr := e.inspectWorkflowProgress(ctx, id, *workflowView.Workflow)
		if progressErr != nil {
			return e.failure(in.RequestID, progressErr)
		}
		binding := progress.subject.Response
		if binding == nil || binding.ProducerAttemptID != in.AttemptID || binding.ResponseArtifactID != in.ResponseArtifactID || binding.ResponseSHA256 != in.ResponseSHA256 || progress.version.Version.Version != in.TaskVersion || progress.decision.Action != "response_acceptance_pending" {
			return e.failure(in.RequestID, domain.Fail("acceptance_unavailable", "Accept the exact current deliverable producer only after every required review, check and scoped action qualifies."))
		}
		ids := append(progress.subject.artifactIDs(), progress.artifactIDs...)
		ids = append(ids, m.ID)
		proofs, proofErr := e.verifyWorkflowArtifacts(ctx, ids...)
		if proofErr != nil {
			return e.failure(in.RequestID, domain.Fail("evidence_unavailable", "The response acceptance evidence could not be verified freshly."))
		}
		graphProofs, proofErr := e.graphAcceptanceArtifactProofs(ctx, progress.subject)
		if proofErr != nil {
			return e.failure(in.RequestID, domain.Fail("evidence_unavailable", "The exact graph response producer and input could not be verified freshly for acknowledgement."))
		}
		proofs, proofErr = mergeGraphAcceptanceProofs(proofs, graphProofs)
		if proofErr != nil {
			return e.failure(in.RequestID, domain.Fail("evidence_unavailable", "The complete response acknowledgement proofs exceed their supported bound or disagree."))
		}
		e.control.Lock()
		// Two callers can both pass the initial replay check while verifying
		// physical evidence. Only the actual mutation may continue graph work.
		if previous, replayErr := e.db.Replay(ctx, in.RequestID, "tasks.response.accept", store.ResponseAcceptanceFingerprint(id, in)); replayErr != nil {
			e.control.Unlock()
			return e.failure(in.RequestID, replayErr)
		} else if previous != nil {
			e.control.Unlock()
			return *previous
		}
		if e.fenced {
			e.control.Unlock()
			return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Resolve ownership or persistence uncertainty before accepting the current workflow response."))
		}
		result := e.db.AcceptResponseWithWorkflow(ctx, id, in, proofs)
		e.guardMutationLocked(result)
		var acknowledged *domain.ResponseAcceptance
		if len(graphProofs) != 0 {
			acknowledged = freshGraphResponseAcknowledgement(id, in, result)
		}
		e.control.Unlock()
		if acknowledged != nil {
			e.continueGraphResponseAcknowledgement(*acknowledged)
		}
		return result
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Resolve ownership or persistence uncertainty before accepting this response."))
	}
	result := e.db.AcceptResponse(ctx, id, in, domain.VerifiedResponse{ArtifactID: m.ID, SHA256: *m.SHA256, PhysicalIdentity: m.Identity, ByteLength: m.ByteLength})
	e.guardMutationLocked(result)
	e.control.Unlock()
	return result
}

// This consumes only the just-committed acknowledgement wire. Receipt replays
// return before this point, and failed or uncertain writes never qualify.
func freshGraphResponseAcknowledgement(taskID string, in domain.ResponseAcceptanceInput, result domain.Result) *domain.ResponseAcceptance {
	if result.Status != 200 || result.Envelope.Error != nil || result.Envelope.RequestID != in.RequestID || result.Envelope.Receipt == nil ||
		result.Envelope.Receipt.RequestID != in.RequestID || result.Envelope.Receipt.Operation != "tasks.response.accept" {
		return nil
	}
	body, err := json.Marshal(result.Envelope.Data)
	if err != nil || len(body) > 64<<10 {
		return nil
	}
	var accepted struct {
		Task       domain.TaskSummary        `json:"task"`
		Acceptance domain.ResponseAcceptance `json:"acceptance"`
	}
	if json.Unmarshal(body, &accepted, json.RejectUnknownMembers(true)) != nil {
		return nil
	}
	task, ack := accepted.Task, accepted.Acceptance
	member := task.GraphRef
	if task.ID != taskID || task.CurrentVersion != in.TaskVersion || member == nil || member.Kind != "node" || member.NodeID == nil ||
		!domain.ValidID(member.GraphID) || !domain.ValidID(member.RootTaskID) || !domain.ValidID(*member.NodeID) ||
		!domain.ValidID(ack.ID) || ack.RequestID != in.RequestID || ack.TaskID != taskID || ack.TaskVersion != in.TaskVersion ||
		ack.AttemptID != in.AttemptID || ack.ResponseArtifactID != in.ResponseArtifactID || ack.ResponseSHA256 != in.ResponseSHA256 ||
		ack.Basis != "user_review" || ack.Confirmation != in.Confirmation {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, ack.AcceptedAt); err != nil {
		return nil
	}
	return &ack
}
