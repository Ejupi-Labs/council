package engine

// continueRouting is called only after a fresh worker invocation has returned,
// including all reservation cleanup. It never scans historical terminal work.
func (e *Engine) continueRouting(previousID string) {
	if e.ctx.Err() != nil {
		return
	}
	a, err := e.db.Attempt(e.ctx, previousID)
	if err != nil {
		e.fence(previousID, "storage_failed", "The fresh settlement could not be inspected for continuation. No automatic replay is permitted.")
		return
	}
	if a.Outcome == nil || *a.Outcome != "blocked" || a.LaunchIntent || a.Cleanup.State != "verified" || a.PrimaryFailure == nil || a.PrimaryFailure.Code != "routing_unavailable" {
		return
	}
	detail, err := e.db.Task(e.ctx, a.TaskID)
	if err != nil {
		e.fence(previousID, "storage_failed", "The continuation task cannot be inspected. Dispatch remains fenced.")
		return
	}
	view, err := e.db.TaskRouting(e.ctx, a.TaskID)
	if err != nil {
		e.fence(previousID, "storage_failed", "The continuation authority cannot be inspected. Dispatch remains fenced.")
		return
	}
	if view.Authority == nil || view.Authority.TaskVersion != a.TaskVersion {
		return
	}
	facts, err := e.frozenRoutingFacts(e.ctx, detail.Task, *view.Authority, a.ExecutionMode)
	if err != nil {
		// Source or installation uncertainty is not a continuation category.
		if persistenceFault(err) {
			e.fence(previousID, "storage_failed", "Continuation prerequisites could not be inspected durably. No replay is permitted.")
		}
		return
	}
	e.control.Lock()
	if e.fenced {
		e.control.Unlock()
		return
	}
	admission, err := e.db.ContinueRouting(e.ctx, e.config.OwnerInstanceID, previousID, facts)
	if persistenceFault(err) {
		e.fenceLocked(previousID, "continuation_persistence_uncertain", "Automatic admission could not be acknowledged durably. Inspect the retained attempt and receipt; no replay is permitted.")
	}
	e.control.Unlock()
	if err == nil && admission != nil {
		e.signal()
	}
}
