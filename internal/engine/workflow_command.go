package engine

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type workflowCommandResult struct {
	Settlement              domain.Settlement
	Stdout, Stderr          evidence.Metadata
	StartedAt, FinishedAt   string
	Started, OutputComplete bool
}

func (e *Engine) registerWorkflowSnapshot(ctx context.Context, a domain.Attempt, snapshot *workspace.Snapshot, identity string) error {
	manifest, err := e.evidence.VerifyClosed(ctx, snapshot.ManifestArtifactID)
	if err != nil || manifest.SHA256 == nil {
		return evidence.ErrCorrupt
	}
	var branch *string
	if snapshot.Branch != "" {
		branch = &snapshot.Branch
	}
	return e.db.PutSourceSnapshot(ctx, domain.SourceSnapshot{ID: snapshot.ID, AttemptID: a.ID, ManifestArtifactID: snapshot.ManifestArtifactID, IndexArtifactID: snapshot.IndexArtifactID, ManifestSHA256: *manifest.SHA256, BaseCommit: snapshot.BaseCommit, Branch: branch, ProjectIdentity: identity, SelectedRelativePath: snapshot.SelectedSubfolder, CreatedAt: domain.Now()})
}

func (e *Engine) handoffWorkflowSubject(ctx context.Context, a domain.Attempt, subject workflowSubject) (*workspace.Snapshot, error) {
	manifest := subject.Source.ManifestArtifactID
	if subject.Binding != nil {
		manifest = subject.Binding.ManifestArtifactID
	}
	snapshot, err := e.workspace.Handoff(ctx, workspace.HandoffRequest{AttemptID: a.ID, Source: subject.snapshot(subject.Workflow.Authority.Delivery), CandidateManifestArtifactID: manifest, IndexArtifactID: subject.ResponseIndexID()})
	if err != nil {
		return nil, err
	}
	if err = e.registerWorkflowSnapshot(ctx, a, snapshot, subject.Workflow.Authority.Delivery.ProjectIdentity); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// This executor records owned native command observations only. It never parses
// TAP/Go/Cargo text into invented test counts or treats an exit as remote proof.
func (e *Engine) executeWorkflowCommand(parent context.Context, a domain.Attempt, snapshot *workspace.Snapshot, command domain.CommandDefinition, graph ...*graphCheckCommandContext) (workflowCommandResult, error) {
	result := workflowCommandResult{}
	if len(graph) > 1 {
		return result, domain.Invalid("one exact graph command context")
	}
	var graphContext *graphCheckCommandContext
	if len(graph) == 1 {
		graphContext = graph[0]
	}
	var environment []string
	if graphContext != nil {
		if !graphLocalCommandRole(a) || graphContext.Snapshot == nil || snapshot == nil || graphContext.Snapshot.ID != snapshot.ID || !graphContext.EffectsKnown {
			return result, domain.Fail("graph_input_stale", "The command has no exact acknowledged graph check source.")
		}
		if graphContext.Final != nil {
			if graphContext.Context == nil || graphContext.Context.Prepared.DescriptorPath == "" {
				return result, domain.Fail("graph_context_unavailable", "The final command has no exact private descriptor.")
			}
			var envErr error
			environment, envErr = graphFinalCommandEnvironment(graphContext.Context.Prepared.DescriptorPath)
			if envErr != nil {
				return result, envErr
			}
		}
	}
	timeout := time.Duration(command.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	program, err := workflowProgram(command.Executable)
	if err != nil || program.Executable != command.Executable || program.PhysicalIdentity != command.PhysicalIdentity {
		return result, domain.Fail("check_unavailable", "The captured native executable changed or is unavailable.")
	}
	cwd := filepath.Join(snapshot.ExecutionPath, filepath.FromSlash(command.RelativeWorkingDirectory))
	root, err := filepath.Rel(snapshot.ExecutionPath, cwd)
	if err != nil || !filepath.IsLocal(root) && root != "." {
		return result, domain.Fail("check_unavailable", "The native command working directory is outside its managed project.")
	}
	out, err := e.evidence.Create(ctx, a.ID, "stdout", evidence.StdoutLimit)
	if err != nil {
		return result, err
	}
	stderr, err := e.evidence.Create(ctx, a.ID, "stderr", evidence.StderrLimit)
	if err != nil {
		_, closeErr := out.Close(context.Background(), false)
		return result, errors.Join(err, closeErr)
	}
	closeUnlaunched := func(cause error) (workflowCommandResult, error) {
		result.Stdout, err = out.Close(context.Background(), false)
		var closeErr error
		result.Stderr, closeErr = stderr.Close(context.Background(), false)
		return result, errors.Join(cause, err, closeErr)
	}
	e.prepareGate.Lock()
	if err = e.recheckGraphCheckCommand(ctx, a, graphContext); err != nil {
		e.prepareGate.Unlock()
		return closeUnlaunched(err)
	}
	e.control.Lock()
	var guardErr error
	if e.fenced {
		guardErr = domain.Fail("dispatch_fenced", "The owner was fenced before local command preparation.")
	} else {
		_, guardErr = e.db.RecordLaunchIntent(ctx, a.ID)
	}
	e.control.Unlock()
	if guardErr != nil {
		e.prepareGate.Unlock()
		return closeUnlaunched(guardErr)
	}
	prepare := process.Prepare
	if e.config.Prepare != nil {
		prepare = e.config.Prepare
	}
	owned, err := prepare(ctx, process.NativeSpec{Executable: command.Executable, Args: command.Arguments, Cwd: cwd, Stdin: []byte{}, Stdout: out, Stderr: stderr, Env: environment})
	e.prepareGate.Unlock()
	if err != nil {
		if errors.Is(err, process.ErrOwnershipUncertain) {
			result.Settlement = domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "unknown", Scope: cancellationScope()}, EvidenceStatus: "uncertain", Failure: primary("ownership_uncertain", "The local command preparation scope is unresolved.")}
			e.fence(a.ID, "ownership_uncertain", "The local command preparation scope could not be verified. Its effects must not be replayed.")
		}
		return closeUnlaunched(err)
	}
	guardErr = e.recheckGraphCheckCommand(ctx, a, graphContext)
	e.control.Lock()
	if e.fenced {
		guardErr = domain.Fail("dispatch_fenced", "The owner was fenced before local command start.")
	} else if guardErr == nil {
		_, guardErr = e.db.RecordLaunchIntent(ctx, a.ID)
	}
	if guardErr == nil && ctx.Err() == nil && !e.fenced {
		err = owned.Start()
		result.Started = err == nil
		if result.Started {
			result.StartedAt = domain.Now()
			err = e.record(a.ID, domain.AttemptUpdate{Kind: "local_command_started", Phase: "running", StartedAt: &result.StartedAt})
		}
	} else {
		err = guardErr
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = domain.Fail("dispatch_fenced", "The owner was fenced before the local command start.")
		}
	}
	e.control.Unlock()
	launchErr := err
	if persistenceFault(launchErr) {
		e.fence(a.ID, "storage_failed", "Local command launch acknowledgement is uncertain; cleanup continues without replay.")
	}
	if err != nil {
		e.recordFailure(a.ID, primary("launch_failed", "The local command did not complete its final launch guard or native start."))
	} else {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				e.recordFailure(a.ID, primary("deadline_exceeded", "The local command reached its captured time limit."))
			}
		case <-owned.RootDone():
		case <-owned.Faults():
			e.recordFailure(a.ID, primary("evidence_failed", "The local command input/output collector reported a failure."))
		}
	}
	startedCleanup := domain.Now()
	cleanupRecordErr := e.record(a.ID, domain.AttemptUpdate{Kind: "cleanup_started", Cleanup: &domain.Cleanup{State: "active", Scope: cancellationScope(), StartedAt: &startedCleanup}})
	if cleanupRecordErr != nil {
		e.fence(a.ID, "storage_failed", "Local command cleanup observation could not be acknowledged durably.")
	}
	deadline := time.Now().Add(process.CleanupLimit)
	if current, readErr := e.db.Attempt(context.Background(), a.ID); readErr == nil && current.Stop != nil {
		if stopped, parseErr := time.Parse(time.RFC3339Nano, current.Stop.AcceptedAt); parseErr == nil && stopped.Add(process.CleanupLimit).Before(deadline) {
			deadline = stopped.Add(process.CleanupLimit)
		}
	}
	e.watchStop(a.ID, deadline.Add(-process.CleanupLimit).UTC().Format(time.RFC3339Nano))
	cleanupCtx, cleanupCancel := context.WithDeadline(context.Background(), deadline)
	clean := owned.Cleanup(cleanupCtx)
	cleanupCancel()
	closeOwnedErr := owned.Close()
	result.Stdout, err = out.Close(context.Background(), clean.StdoutDrained && !clean.StdoutError)
	var closeErr error
	result.Stderr, closeErr = stderr.Close(context.Background(), clean.StderrDrained && !clean.StderrError)
	result.OutputComplete = err == nil && closeErr == nil && clean.StdoutDrained && clean.StderrDrained && !clean.StdoutError && !clean.StderrError
	result.FinishedAt = domain.Now()
	cleanup := domain.Cleanup{State: "verified", Scope: cancellationScope(), StartedAt: &startedCleanup, FinishedAt: &result.FinishedAt}
	if !clean.Verified || closeOwnedErr != nil {
		cleanup.State = "unknown"
		e.fence(a.ID, "cleanup_unresolved", "The local command scope could not be closed and verified; no stage or publication is replayed.")
	}
	state := "complete"
	if !result.OutputComplete {
		state = "incomplete"
	}
	if uncertainEvidence(err) || uncertainEvidence(closeErr) || persistenceFault(launchErr) || cleanupRecordErr != nil {
		state = "uncertain"
		e.fence(a.ID, "evidence_uncertain", "The local command output could not be acknowledged durably.")
	}
	var exit *int
	if clean.Root != nil && clean.Root.Known {
		exit = &clean.Root.ExitCode
	}
	result.Settlement = domain.Settlement{Outcome: "succeeded", Cleanup: cleanup, EvidenceStatus: state, RootTerminal: clean.Root != nil && clean.Root.Known, InputComplete: result.Started && clean.StdinState == process.StdinComplete && !clean.StdinError && clean.ExpectedInputBytes == 0 && clean.StdinBytesWritten == 0, ExitCode: exit, UsableOutput: result.OutputComplete}
	return result, errors.Join(launchErr, cleanupRecordErr, err, closeErr, closeOwnedErr)
}
