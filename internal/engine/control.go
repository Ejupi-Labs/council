package engine

import (
	"context"
	json "encoding/json/v2"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

// watchStop is called outside control after the durable Stop has been accepted. Its
// deadline belongs to that acceptance, not to a later process-cleanup operation.
func (e *Engine) watchStop(attemptID, acceptedAt string) {
	e.watchStopWithLimit(attemptID, acceptedAt, process.CleanupLimit)
}
func (e *Engine) watchStopWithLimit(attemptID, acceptedAt string, limit time.Duration) {
	e.control.Lock()
	if e.stopWatches == nil {
		e.stopWatches = make(map[string]struct{})
	}
	if _, exists := e.stopWatches[attemptID]; exists {
		e.control.Unlock()
		return
	}
	e.stopWatches[attemptID] = struct{}{}
	e.control.Unlock()
	accepted, err := time.Parse(time.RFC3339Nano, acceptedAt)
	if err != nil {
		accepted = time.Now().Add(-limit)
	}
	deadline := accepted.Add(limit)
	go func() {
		defer func() { e.control.Lock(); delete(e.stopWatches, attemptID); e.control.Unlock() }()
		timer := time.NewTimer(max(time.Until(deadline), 0))
		defer timer.Stop()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-e.watchEnd:
				return
			case <-tick.C:
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				a, err := e.db.Attempt(ctx, attemptID)
				cancel()
				if err == nil && a.Phase == "settled" {
					return
				}
			case <-timer.C:
				e.expireStopBudget(attemptID)
				return
			}
		}
	}()
}

func (e *Engine) expireStopBudget(attemptID string) {
	// Fence in memory first: an unavailable disk must not leave new dispatch open.
	// The short context bounds cooperative storage work; it is not a filesystem
	// hard-real-time promise and never permits a PID-based takeover.
	e.control.Lock()
	defer e.control.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wasFenced := e.fenced
	e.fenced = true
	a, err := e.db.Attempt(ctx, attemptID)
	if err == nil && a.Phase == "settled" {
		e.fenced = wasFenced
		return
	}
	if e.activeID == attemptID && e.activeCancel != nil {
		e.activeCancel()
	}
	incidentID := domain.NewID()
	reason := "stop_budget_elapsed"
	message := "The accepted Stop did not reach verified settlement within 15 seconds. Cleanup or evidence remains unresolved; dispatch is fenced."
	if err == nil && a.Stop == nil {
		reason = "settlement_budget_elapsed"
		message = "Execution termination did not reach verified settlement within 15 seconds. Cleanup or evidence remains unresolved; dispatch is fenced."
	}
	incident := domain.ExecutionIncident{ID: incidentID, AttemptID: &attemptID, Code: reason, Message: message, CreatedAt: domain.Now()}
	if fenceErr := e.db.Fence(ctx, incident); fenceErr != nil {
		// An uncertain acknowledgement can still have persisted the same incident.
		if _, lookupErr := e.db.Incident(ctx, incidentID); lookupErr != nil {
			return
		}
	}
	if err != nil {
		return
	}
	cleanup := a.Cleanup
	cleanup.State = "unknown"
	cleanup.IncidentID = &incidentID
	if cleanup.Scope == "" || (a.LaunchIntent && cleanup.Scope == "not_launched") {
		cleanup.Scope = cancellationScope()
	}
	metadata, _ := json.Marshal(struct {
		IncidentID string `json:"incidentId"`
		Reason     string `json:"reason"`
	}{incidentID, reason})
	// This is an observation, never a terminal. Workers may still be retaining
	// bytes, and the capacity claim stays held until actual verified cleanup.
	_, _ = e.db.RecordAttempt(ctx, attemptID, domain.AttemptUpdate{Kind: "cleanup_unknown", Cleanup: &cleanup, Metadata: metadata})
}
