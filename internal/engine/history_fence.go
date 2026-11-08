package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Caller holds control. A failed incident/binding commit is itself an
// unrecorded cause and must remain fenced across shutdown.
func (e *Engine) fenceHistoryOperationLocked(operationID string) {
	e.fenced = true
	if _, err := e.db.FenceHistoryOperation(context.Background(), e.config.OwnerInstanceID, operationID); err != nil {
		e.fenceUncertain = true
	}
	e.cancelWorkersLocked()
}

func (e *Engine) fenceHistoryMutationLocked(operation, requestID, fingerprint string) {
	e.fenced = true
	if _, err := e.db.FenceHistoryMutation(context.Background(), e.config.OwnerInstanceID, operation, requestID, fingerprint); err != nil {
		e.fenceUncertain = true
	}
	e.cancelWorkersLocked()
}

func (e *Engine) guardHistoryMutationLocked(result domain.Result, operation, fingerprint string) {
	if result.Envelope.Error != nil && persistenceFault(result.Envelope.Error) {
		e.fenceHistoryMutationLocked(operation, result.Envelope.RequestID, fingerprint)
	}
}

func (e *Engine) guardHistoryOperationLocked(err error, operationID string) {
	if err != nil && persistenceFault(err) {
		e.fenceHistoryOperationLocked(operationID)
	}
}

// preserveUnrecordedHistoryFenceLocked is used when even the exact recovery
// receipt and prior incident state cannot be verified. No synthetic durable
// incident or absence proof is invented.
func (e *Engine) preserveUnrecordedHistoryFenceLocked() {
	e.fenced = true
	e.fenceUncertain = true
	e.cancelWorkersLocked()
}
