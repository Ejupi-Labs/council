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
	parser *adapters.CodexParser
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
	deadline, err := time.Parse(time.RFC3339Nano, a.DeadlineAt)
	if err != nil {
		e.settleUnlaunched(a, primary("deadline_exceeded", "The captured deadline is invalid."))
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
		e.settleUnlaunched(a, failure, err)
		return
	}
	p, identity, err := e.db.Project(ctx, mustTaskProject(e.db, ctx, a.TaskID))
	if err != nil {
		e.settleUnlaunched(a, primary("project_unavailable", "The saved project cannot be read."))
		return
	}
	if err = project.ValidateIdentity(p.ResolvedPath, identity); err != nil {
		e.settleUnlaunched(a, primary("project_identity_changed", "The saved project identity changed. Restore its original folder before another explicit attempt."))
		return
	}
	outputPath, err := e.evidence.OutputPath(ctx, a.ID)
	if err != nil {
		e.settleUnlaunched(a, primary("evidence_failed", "Cannot prepare a private final response location."), err)
		return
	}
	defer func() {
		if releaseErr := e.evidence.ReleaseOutput(a.ID, outputPath); releaseErr != nil {
			e.fence(a.ID, "response_reservation_uncertain", "The private response reservation could not be released cleanly. Preserve its evidence and inspect the retained incident before further dispatch.")
		}
	}()
	resolve := e.config.Resolve
	if a.ExecutionMode == "fake" {
		resolve = func(context.Context, domain.ExecutionPins) (string, error) { return os.Executable() }
	}
	launch, err := adapters.BuildCodex(ctx, detail.OriginalText, detail.Version, outputPath, resolve)
	if err != nil {
		e.settleUnlaunched(a, primary(domain.SafeError(err).Code, domain.SafeError(err).Message))
		return
	}
	if a.ExecutionMode == "fake" {
		launch.Args, err = fixture.Args(a.FakeScenario, outputPath)
		if err != nil {
			e.settleUnlaunched(a, primary("capability_unavailable", "The requested fixed fixture is unsupported."))
			return
		}
		launch.Observations = adapters.UnknownObservations("autonomous")
		launch.Observations.Permissions.Limitations = []string{"Local fake execution; no provider or live account was invoked."}
	}
	prompt, err := e.evidence.Put(ctx, a.ID, "prompt", launch.Prompt)
	if err != nil {
		e.settleUnlaunched(a, primary("evidence_failed", "The complete prompt could not be retained."), err)
		return
	}
	_ = prompt
	snapshot, err := e.workspace.Capture(ctx, workspace.Request{AttemptID: a.ID, ProjectPath: p.ResolvedPath})
	if err != nil {
		message := "Current source could not be captured faithfully. Inspect source prerequisites and retain the original working tree."
		if errors.Is(err, workspace.ErrBlocked) && utf8.ValidString(err.Error()) && len(err.Error()) <= 512 {
			message = err.Error()
		}
		e.settleUnlaunched(a, primary("source_capture_blocked", message), err)
		return
	}
	manifest, err := e.evidence.VerifyClosed(ctx, snapshot.ManifestArtifactID)
	if err != nil || manifest.SHA256 == nil {
		e.settleUnlaunched(a, primary("evidence_failed", "The captured source manifest is not verifiable."), err)
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
		e.settleUnlaunched(a, failure, err)
		return
	}
	out, err := e.evidence.Create(ctx, a.ID, "stdout", evidence.StdoutLimit)
	if err != nil {
		e.settleUnlaunched(a, primary("evidence_failed", "Cannot create the private stdout artifact."), err)
		return
	}
	stderr, err := e.evidence.Create(ctx, a.ID, "stderr", evidence.StderrLimit)
	if err != nil {
		_, closeErr := out.Close(context.Background(), false)
		e.settleUnlaunched(a, primary("evidence_failed", "Cannot create the private stderr artifact."), errors.Join(err, closeErr))
		return
	}
	parser := adapters.NewCodexParser(detail.Version.Pins, func(event adapters.Event) error {
		if err := out.Checkpoint(context.Background()); err != nil {
			return err
		}
		if a.ExecutionMode == "fake" {
			event.Observations = nil
		}
		return e.record(a.ID, domain.AttemptUpdate{Kind: event.Kind, Metadata: jsontext.Value(event.Metadata), TargetObservations: event.Observations, Failure: event.Failure})
	})
	e.control.Lock()
	_, guardErr := e.db.RecordLaunchIntent(ctx, a.ID)
	e.control.Unlock()
	if guardErr != nil {
		if persistenceFault(guardErr) {
			e.fence(a.ID, "storage_failed", "The durable native preparation guard could not be acknowledged. Further dispatch remains fenced.")
		}
		_, outCloseErr := out.Close(context.Background(), false)
		_, stderrCloseErr := stderr.Close(context.Background(), false)
		e.settleUnlaunched(a, primary("launch_blocked", "The durable preparation guard rejected native creation. No worker was launched."), errors.Join(guardErr, outCloseErr, stderrCloseErr))
		return
	}
	owned, err := process.Prepare(ctx, process.NativeSpec{Executable: launch.Executable, Args: launch.Args, Cwd: snapshot.ExecutionPath, Stdin: launch.Prompt, Stdout: protocolWriter{e, a.ID, out, parser}, Stderr: stderr})
	if err != nil {
		if errors.Is(err, process.ErrOwnershipUncertain) {
			e.fence(a.ID, "ownership_uncertain", "Native preparation cleanup could not be verified. Do not replay or dispatch more work.")
		}
		_, outCloseErr := out.Close(context.Background(), false)
		_, stderrCloseErr := stderr.Close(context.Background(), false)
		if errors.Is(err, process.ErrOwnershipUncertain) {
			e.control.Lock()
			_, _ = e.db.SettleAttempt(context.Background(), a.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "unknown", Scope: cancellationScope()}, EvidenceStatus: "uncertain", Failure: primary("ownership_uncertain", "Native preparation cleanup could not be verified.")})
			e.control.Unlock()
		} else {
			e.settleUnlaunched(a, primary("launch_failed", "The native process could not be prepared. No automatic retry was made."), errors.Join(err, outCloseErr, stderrCloseErr))
		}
		return
	}
	nativeStarted := false
	e.control.Lock()
	_, err = e.db.RecordLaunchIntent(context.Background(), a.ID)
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
	_ = owned.Close()
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
	if a.ExecutionMode == "fake" && a.FakeScenario == "cleanup_unknown" {
		clean.Verified = false
		clean.Detail = "Explicit local fixture injected unresolved cleanup; no escaped process is simulated as owned."
	}
	protocolErr := parser.Finish()
	r := parser.Result()
	if protocolErr != nil && r.Failure != nil {
		e.recordFailure(a.ID, r.Failure)
	}
	stdoutMeta, outErr := out.Close(context.Background(), clean.StdoutDrained && !clean.StdoutError)
	_, stderrErr := stderr.Close(context.Background(), clean.StderrDrained && !clean.StderrError)
	evidenceState := "complete"
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
	if clean.Verified {
		response, err = e.evidence.ImportResponse(settlementCtx, a.ID, outputPath, 64<<20)
		if err == nil {
			_, err = e.evidence.VerifyClosed(settlementCtx, response.ID)
			if err == nil {
				chunk, readErr := e.readResponse(settlementCtx, response)
				usable = readErr == nil && utf8.Valid(chunk) && strings.TrimSpace(string(chunk)) != ""
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
	}
	var candidate *workspace.Candidate
	if clean.Verified {
		candidate, err = e.workspace.Candidate(settlementCtx, snapshot)
		if candidate != nil {
			hasIndex := false
			for _, change := range candidate.Changes {
				if change.BeforeIndex != nil || change.AfterIndex != nil {
					hasIndex = true
				}
			}
			rel, relErr := filepath.Rel(e.lock.Root(), candidate.WorkspacePath)
			if relErr == nil {
				relErr = e.db.PutCandidate(context.Background(), domain.Candidate{ID: candidate.ID, AttemptID: a.ID, SourceSnapshotID: snapshot.ID, ManifestArtifactID: candidate.ManifestArtifactID, DeltaArtifactID: candidate.DeltaArtifactID, WorkspaceRef: filepath.ToSlash(rel), HasDelta: len(candidate.Changes) > 0, HasIndexDelta: hasIndex, Verification: "pending", Integration: "not_performed", CreatedAt: domain.Now()})
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
	e.control.Lock()
	_, settleErr := e.db.SettleAttempt(context.Background(), a.ID, domain.Settlement{Outcome: outcome, Cleanup: cleanup, EvidenceStatus: evidenceState, RootTerminal: r.Terminal && r.Succeeded, InputComplete: inputComplete, ExitCode: code, UsableOutput: usable})
	e.control.Unlock()
	if settleErr != nil {
		e.fence(a.ID, "storage_failed", "Terminal execution state could not be acknowledged. Retain its evidence and do not replay.")
	}
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
	outcome := "blocked"
	if deadline, err := time.Parse(time.RFC3339Nano, a.DeadlineAt); err == nil && !time.Now().Before(deadline) {
		f = primary("deadline_exceeded", "The captured execution deadline expired before worker instructions began.")
		outcome = "failed"
	}
	e.control.Lock()
	_, err := e.db.SettleAttempt(context.Background(), a.ID, domain.Settlement{NoProcessCreated: true, Outcome: outcome, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: evidenceState, Failure: f})
	e.control.Unlock()
	if err != nil {
		e.fence(a.ID, "storage_failed", "Preparation could not be settled durably. No automatic retry is permitted.")
	}
}

func (e *Engine) AcceptResponse(ctx context.Context, id string, in domain.ResponseAcceptanceInput) domain.Result {
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
	e.control.Lock()
	result := e.db.AcceptResponse(ctx, id, in, domain.VerifiedResponse{ArtifactID: m.ID, SHA256: *m.SHA256, PhysicalIdentity: m.Identity, ByteLength: m.ByteLength})
	e.guardMutationLocked(result)
	e.control.Unlock()
	return result
}
