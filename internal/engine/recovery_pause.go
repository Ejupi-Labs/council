package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Pause controls serialize with the owner preparation and launch boundaries.
// The store commits the admission fence together with its durable receipt.
func (e *Engine) PauseTask(ctx context.Context, id string, in domain.PauseTaskInput) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	e.control.Lock()
	result := e.db.PauseTask(ctx, e.config.OwnerInstanceID, id, in)
	e.guardMutationLocked(result)
	e.control.Unlock()
	return result
}

func (e *Engine) PauseGraph(ctx context.Context, id string, in domain.PauseGraphInput) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	e.prepareGate.Lock()
	defer e.prepareGate.Unlock()
	e.control.Lock()
	result := e.db.PauseGraph(ctx, e.config.OwnerInstanceID, id, in)
	e.guardMutationLocked(result)
	e.control.Unlock()
	return result
}
