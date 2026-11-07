package engine

import (
	"context"
	"errors"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/Ejupi-Labs/council/v2/internal/workspace"
)

type artifactSink struct{ db *store.Store }

func (s artifactSink) SaveArtifact(ctx context.Context, m evidence.Metadata) error {
	return s.db.PutArtifact(ctx, domain.ArtifactRecord{ArtifactMetadata: domain.ArtifactMetadata{ID: m.ID, AttemptID: m.AttemptID, Kind: m.Kind, ByteLength: m.ByteLength, Closed: m.Closed, Completeness: m.Completeness, SHA256: m.SHA256, CreatedAt: m.CreatedAt}, PrivateRef: m.RelativePath, PhysicalIdentity: m.Identity})
}
func (s artifactSink) LoadArtifact(ctx context.Context, id string) (evidence.Metadata, error) {
	r, e := s.db.Artifact(ctx, id)
	return evidence.Metadata{ID: r.ID, AttemptID: r.AttemptID, Kind: r.Kind, ByteLength: r.ByteLength, Closed: r.Closed, Completeness: r.Completeness, SHA256: r.SHA256, CreatedAt: r.CreatedAt, RelativePath: r.PrivateRef, Identity: r.PhysicalIdentity}, e
}

func New(db *store.Store, lock *localstate.Lock, config Config) (*Engine, error) {
	if !domain.ValidID(config.OwnerInstanceID) || lock.CheckHeld() != nil {
		return nil, errors.New("A current private owner identity is required.")
	}
	artifacts, err := evidence.New(lock, artifactSink{db})
	if err != nil {
		return nil, err
	}
	workspaces, err := workspace.New(lock, artifacts)
	if err != nil {
		artifacts.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	e := &Engine{db: db, lock: lock, config: config, evidence: artifacts, workspace: workspaces, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), done: make(chan struct{}), shutdown: make(chan struct{}), watchEnd: make(chan struct{}), activeWorkers: make(map[string]context.CancelFunc)}
	if err = db.RecoverOwner(ctx, config.OwnerInstanceID); err != nil {
		cancel()
		artifacts.Close()
		workspaces.Close()
		return nil, err
	}
	go e.schedule()
	return e, nil
}

func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}
func (e *Engine) ShutdownRequested() <-chan struct{} { return e.shutdown }
func (e *Engine) failure(id string, err error) domain.Result {
	return domain.Failure(id, domain.SafeError(err))
}

func (e *Engine) StartTask(ctx context.Context, in domain.StartTaskInput) domain.Result {
	if err := domain.ValidateStartTask(in); err != nil {
		return e.failure(in.RequestID, err)
	}
	if previous, err := e.db.Replay(ctx, in.RequestID, "tasks.start", store.TaskStartFingerprint(in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if previous != nil {
		return *previous
	}
	routingFacts, err := e.startRoutingFacts(ctx, in)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	acceptance, acceptanceErr := domain.ResolveAcceptance(in.Acceptance)
	if acceptanceErr != nil {
		return e.failure(in.RequestID, acceptanceErr)
	}
	facts, err := e.captureWorkflow(ctx, in.ProjectID, in.OriginalText, domain.TaskVersion{Acceptance: acceptance}, in.Workflow, in.ExecutionMode, in.FakeScenario, routingFacts)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Dispatch is fenced by unresolved ownership or persistence. Inspect the retained incident before admitting work."))
	}
	result := e.db.CreateTaskWithWorkflow(ctx, e.config.OwnerInstanceID, in, facts)
	e.guardMutationLocked(result)
	e.control.Unlock()
	e.signal()
	return result
}
func (e *Engine) Guidance(ctx context.Context, id string, in domain.GuidanceInput) domain.Result {
	if !domain.ValidID(id) {
		return e.failure(in.RequestID, domain.Invalid("task identity"))
	}
	if err := domain.ValidateGuidance(in); err != nil {
		return e.failure(in.RequestID, err)
	}
	if replayed, err := e.db.Replay(ctx, in.RequestID, "tasks.guidance", store.GuidanceFingerprint(id, in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if replayed != nil {
		return *replayed
	}
	if err := e.db.CheckStandaloneAuthority(ctx, id, false); err != nil {
		return e.failure(in.RequestID, err)
	}
	view, err := e.db.TaskRouting(ctx, id)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	var facts domain.RoutingFacts
	if view.Authority != nil {
		detail, err := e.db.Task(ctx, id)
		if err != nil {
			return e.failure(in.RequestID, err)
		}
		facts, err = e.guidanceRoutingFacts(ctx, detail, in, *view.Authority)
		if err != nil {
			return e.failure(in.RequestID, err)
		}
	}
	workflowFacts, err := e.guidanceWorkflowFacts(ctx, id, in, facts)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	e.control.Lock()
	result := e.db.UpdateGuidanceWithWorkflow(ctx, id, in, workflowFacts)
	e.guardMutationLocked(result)
	e.control.Unlock()
	e.signal()
	return result
}
func (e *Engine) StartAttempt(ctx context.Context, id string, in domain.StartAttemptInput) domain.Result {
	if !domain.ValidID(id) {
		return e.failure(in.RequestID, domain.Invalid("task identity"))
	}
	if err := domain.ValidateStartAttempt(in); err != nil {
		return e.failure(in.RequestID, err)
	}
	if prior, err := e.db.Replay(ctx, in.RequestID, "tasks.attempts.start", store.AttemptStartFingerprint(id, in)); err != nil {
		return e.failure(in.RequestID, err)
	} else if prior != nil {
		return *prior
	}
	if err := e.db.CheckStandaloneAuthority(ctx, id, false, in.TaskVersion); err != nil {
		return e.failure(in.RequestID, err)
	}
	view, err := e.db.TaskRouting(ctx, id)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if view.Authority == nil {
		return e.failure(in.RequestID, domain.Fail("routing_adoption_required", "Adopt explicit routing and budget authority before starting new work for this legacy task."))
	}
	if view.Authority.TaskVersion != in.TaskVersion {
		return e.failure(in.RequestID, domain.Fail("task_version_conflict", "Start against the current immutable task version."))
	}
	detail, err := e.db.Task(ctx, id)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	mode := in.ExecutionMode
	workflowView, err := e.db.Workflow(ctx, id, domain.WorkflowSelector{})
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if workflowView.Workflow != nil {
		mode = workflowView.Workflow.Authority.ExecutionMode
	}
	facts, err := e.frozenRoutingFacts(ctx, detail.Task, *view.Authority, mode)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return e.failure(in.RequestID, domain.Fail("dispatch_fenced", "Dispatch is fenced by unresolved ownership or persistence. Inspect the retained incident before admitting work."))
	}
	result := e.db.StartAttemptWithWorkflow(ctx, e.config.OwnerInstanceID, id, in, domain.WorkflowAdmissionFacts{Routing: facts})
	e.guardMutationLocked(result)
	e.control.Unlock()
	e.signal()
	return result
}
func (e *Engine) Stop(ctx context.Context, id, requestID string) domain.Result {
	e.control.Lock()
	result := e.db.StopAttempt(ctx, id, requestID)
	e.guardMutationLocked(result)
	if result.Envelope.Error == nil {
		e.cancelWorkerLocked(id)
	}
	e.control.Unlock()
	if result.Envelope.Error == nil {
		if a, err := e.db.Attempt(context.Background(), id); err == nil && a.Stop != nil {
			e.watchStop(id, a.Stop.AcceptedAt)
		}
	}
	e.signal()
	return result
}
func (e *Engine) Shutdown(ctx context.Context, ownerID, requestID string) domain.Result {
	if prior, err := e.db.Replay(ctx, requestID, "service.stop", store.ShutdownFingerprint(ownerID, requestID)); err != nil {
		return e.failure(requestID, err)
	} else if prior != nil {
		if ownerID == e.config.OwnerInstanceID && prior.Envelope.Error == nil {
			e.shutdownOnce.Do(func() { close(e.shutdown) })
		}
		return *prior
	}
	if ownerID != e.config.OwnerInstanceID {
		return e.failure(requestID, domain.Fail("owner_unavailable", "Stop must target the current owner instance explicitly."))
	}
	e.control.Lock()
	result := e.db.Shutdown(ctx, ownerID, requestID)
	e.guardMutationLocked(result)
	e.control.Unlock()
	if result.Envelope.Error == nil || (result.Envelope.Error != nil && persistenceFault(result.Envelope.Error)) {
		e.shutdownOnce.Do(func() { close(e.shutdown) })
	}
	return result
}

func (e *Engine) Close() error {
	e.closeOnce.Do(func() {
		_ = e.Shutdown(context.Background(), e.config.OwnerInstanceID, domain.NewID())
		e.cancel()
		<-e.done
		e.workers.Wait()
		if e.watchEnd != nil {
			close(e.watchEnd)
		}
		e.closeErr = errors.Join(e.workspace.Close(), e.evidence.Close())
	})
	return e.closeErr
}

func (e *Engine) fence(id, code, message string) {
	e.control.Lock()
	defer e.control.Unlock()
	e.fenceLocked(id, code, message)
}

// guardMutationLocked closes dispatch before the scheduler can observe a write
// whose acknowledgement failed. The request identity is never an attempt ID.
func (e *Engine) guardMutationLocked(result domain.Result) {
	if result.Envelope.Error != nil && persistenceFault(result.Envelope.Error) {
		e.fenceLocked("", "mutation_persistence_uncertain", "A control or admission write could not be acknowledged durably. Dispatch remains fenced; retry the same request identity to inspect its receipt.")
	}
}

// fenceLocked is also used at protocol write boundaries already ordered by control.
func (e *Engine) fenceLocked(id, code, message string) {
	if e.fenced {
		return
	}
	e.fenced = true
	var attempt *string
	if id != "" {
		attempt = &id
	}
	_ = e.db.Fence(context.Background(), domain.ExecutionIncident{ID: domain.NewID(), AttemptID: attempt, Code: code, Message: message, CreatedAt: domain.Now()})
	e.cancelWorkersLocked()
}

// cancelWorkerLocked requests termination without claiming cleanup or releasing
// another worker's ownership. The caller holds control.
func (e *Engine) cancelWorkerLocked(attemptID string) bool {
	cancel, owned := e.activeWorkers[attemptID]
	if owned && cancel != nil {
		cancel()
	}
	return owned
}

// cancelWorkersLocked reaches every actual owned worker during a shared fence.
// Handles remain registered until their own goroutines finish real settlement.
func (e *Engine) cancelWorkersLocked() {
	for _, cancel := range e.activeWorkers {
		if cancel != nil {
			cancel()
		}
	}
}

func (e *Engine) schedule() {
	defer close(e.done)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-e.ctx.Done():
			e.stopPending()
			return
		case <-tick.C:
		case <-e.wake:
		}
		if _, err := e.db.SweepQueued(e.ctx, e.config.OwnerInstanceID); err != nil {
			if e.ctx.Err() != nil && !persistenceFault(err) {
				continue
			}
			e.fence("", "storage_failed", "Queued controls or deadlines could not be retained. Dispatch remains fenced.")
			continue
		}
		pending, err := e.db.PendingAttempts(e.ctx, e.config.OwnerInstanceID, 100)
		if err != nil {
			if e.ctx.Err() != nil {
				continue
			}
			e.fence("", "storage_failed", "Execution observation failed. Dispatch remains fenced until recovery.")
			continue
		}
		for _, a := range pending {
			e.control.Lock()
			_, active := e.activeWorkers[a.ID]
			if a.Phase == "cancelling" && !active {
				_, err = e.db.SettleAttempt(e.ctx, a.ID, domain.Settlement{Outcome: "cancelled", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"})
				e.control.Unlock()
				if err != nil {
					e.fence(a.ID, "storage_failed", "Queued cancellation could not be retained.")
				}
				continue
			}
			if a.Phase != "queued" || e.fenced {
				e.control.Unlock()
				continue
			}
			// Claim validates obsolete versions and expired deadlines before capacity.
			claimed, claimErr := e.db.ClaimAttempt(e.ctx, a.ID, e.config.OwnerInstanceID)
			if claimErr != nil || claimed.Phase != "preparing" {
				if persistenceFault(claimErr) {
					e.fenceLocked(a.ID, "storage_failed", "The execution claim could not be acknowledged durably. No automatic dispatch or replay is permitted.")
				}
				e.control.Unlock()
				continue
			}
			workerCtx, cancel := context.WithCancel(e.ctx)
			e.activeWorkers[a.ID] = cancel
			e.workers.Add(1)
			e.control.Unlock()
			go func(a domain.Attempt) {
				defer e.workers.Done()
				defer cancel()
				e.run(workerCtx, a)
				e.control.Lock()
				delete(e.activeWorkers, a.ID)
				e.control.Unlock()
				e.continueRouting(a.ID)
				e.continueWorkflow(a.ID)
				e.continueGraphInputs(a.ID)
				e.continueGraphAdmissions(a.ID)
				e.signal()
			}(claimed)
		}
	}
}

func (e *Engine) stopPending() {
	ctx := context.Background()
	for {
		pending, err := e.db.PendingAttempts(ctx, e.config.OwnerInstanceID, 100)
		if err != nil {
			return
		}
		progress := false
		for _, a := range pending {
			e.control.Lock()
			result := e.db.StopAttempt(ctx, a.ID, domain.NewID())
			if _, active := e.activeWorkers[a.ID]; active {
				e.cancelWorkerLocked(a.ID)
			} else if result.Envelope.Error == nil {
				cleanup := domain.Cleanup{State: "verified", Scope: "not_launched"}
				if a.LaunchIntent {
					cleanup.State = "unknown"
					cleanup.Scope = cancellationScope()
				}
				_, settleErr := e.db.SettleAttempt(ctx, a.ID, domain.Settlement{Outcome: "cancelled", Cleanup: cleanup, EvidenceStatus: "incomplete"})
				progress = settleErr == nil
			}
			e.control.Unlock()
			if result.Envelope.Error == nil {
				if current, err := e.db.Attempt(ctx, a.ID); err == nil && current.Stop != nil {
					e.watchStop(a.ID, current.Stop.AcceptedAt)
				}
			}
		}
		if !progress {
			return
		}
	}
}
