package engine

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// CaptureHistory records only the bounded structured projection of one selected
// project. It does not admit an agent or inspect source/artifact payloads.
func (e *Engine) CaptureHistory(ctx context.Context, in domain.CaptureHistoryInput) domain.Result {
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	e.historyMu.Lock()
	defer e.historyMu.Unlock()
	e.control.Lock()
	result := e.db.CaptureHistory(ctx, in)
	e.guardHistoryMutationLocked(result, "history.snapshots.create", store.Fingerprint("history.snapshots.create", in))
	e.control.Unlock()
	return result
}

func (e *Engine) HistorySnapshot(ctx context.Context, id string) domain.Result {
	requestID := domain.NewID()
	if !e.beginOwnerOperation() {
		return e.failure(requestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	snapshot, err := e.db.HistorySnapshot(ctx, id)
	if err != nil {
		return e.failure(requestID, err)
	}
	return domain.Success(200, requestID, struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}{snapshot.Summary}, nil)
}

func (e *Engine) HistoryRecords(ctx context.Context, id string, query domain.HistoryRecordsQuery) domain.Result {
	requestID := domain.NewID()
	if !e.beginOwnerOperation() {
		return e.failure(requestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	page, err := e.db.HistoryRecords(ctx, id, query)
	if err != nil {
		return e.failure(requestID, err)
	}
	return domain.Success(200, requestID, page, nil)
}
