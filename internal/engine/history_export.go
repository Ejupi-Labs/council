package engine

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/historyout"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

func historyOutputError(err error) *domain.Error {
	switch {
	case errors.Is(err, historyout.ErrInvalid):
		return domain.Invalid("history filename or output")
	case errors.Is(err, historyout.ErrConflict):
		return domain.Fail("revision_conflict", "The history output location or filename changed. Preview it again.")
	default:
		return domain.Fail("capability_unavailable", "The private history output location cannot be verified on this system.")
	}
}

// historyReplay must precede native output inspection: a lost response never
// creates a new preview, rechecks target absence, or authorizes another effect.
func (e *Engine) historyReplay(ctx context.Context, id, operation, fingerprint string) (*domain.Result, error) {
	return e.db.Replay(ctx, id, operation, fingerprint)
}

func (e *Engine) PreviewHistoryExport(ctx context.Context, in domain.PreviewHistoryExportInput) domain.Result {
	if invalid := domain.ValidatePreviewHistoryExportInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	e.historyMu.Lock()
	defer e.historyMu.Unlock()
	replay, err := e.historyReplay(ctx, in.RequestID, store.HistoryPreviewOperation, store.HistoryPreviewFingerprint(in))
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if replay != nil {
		return *replay
	}
	root, err := historyout.Open(e.lock)
	if err != nil {
		return domain.Failure(in.RequestID, historyOutputError(err))
	}
	defer root.Close()
	preview, err := root.Preview(in.Filename)
	if err != nil {
		return domain.Failure(in.RequestID, historyOutputError(err))
	}
	e.control.Lock()
	result := e.db.PreviewHistoryExport(ctx, e.config.OwnerInstanceID, in, root.Directory(), preview)
	e.guardHistoryMutationLocked(result, store.HistoryPreviewOperation, store.HistoryPreviewFingerprint(in))
	e.control.Unlock()
	return result
}

// historyCurrentResult preserves the original receipt while reporting the current
// durable operation state, which may have changed after that receipt was created.
func (e *Engine) historyCurrentResult(ctx context.Context, original domain.Result) domain.Result {
	if original.Envelope.Error != nil {
		return original
	}
	var saved struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	raw, err := json.Marshal(original.Envelope.Data)
	if err != nil || json.Unmarshal(raw, &saved) != nil || !domain.ValidID(saved.Operation.OperationID) {
		return e.failure(original.Envelope.RequestID, domain.Fail("storage_failed", "The saved history operation cannot be verified."))
	}
	current, err := e.db.HistoryExportOperation(ctx, saved.Operation.OperationID)
	if err != nil {
		return e.failure(original.Envelope.RequestID, err)
	}
	if current.State == "prepared" && current.NextAction == "wait" {
		// historyMu excludes any live synchronous publisher here. A prepared
		// receipt replayed after a lost acknowledgement or restart has no actor
		// to finish it; freeze it for explicit reconciliation, never replay it.
		settleCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		e.control.Lock()
		markErr := e.db.MarkHistoryExportPrePermitFailure(settleCtx, e.config.OwnerInstanceID, current.OperationID, "request_interrupted")
		if markErr != nil {
			e.guardHistoryOperationLocked(markErr, current.OperationID)
		}
		e.control.Unlock()
		cancel()
		if markErr != nil {
			return e.failure(original.Envelope.RequestID, markErr)
		}
		current, err = e.db.HistoryExportOperation(ctx, saved.Operation.OperationID)
		if err != nil {
			return e.failure(original.Envelope.RequestID, err)
		}
	}
	original.Envelope.Data = struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}{current}
	return original
}

func (e *Engine) ExecuteHistoryExport(ctx context.Context, in domain.ExecuteHistoryExportInput) domain.Result {
	if invalid := domain.ValidateExecuteHistoryExportInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	e.historyMu.Lock()
	defer e.historyMu.Unlock()
	replay, err := e.historyReplay(ctx, in.RequestID, store.HistoryCreateOperation, store.HistoryCreateFingerprint(in))
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	if replay != nil {
		return e.historyCurrentResult(ctx, *replay)
	}
	preview, native, err := e.db.HistoryExportPreview(ctx, in.PreviewID)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	snapshot, err := e.db.HistorySnapshot(ctx, preview.SnapshotID)
	if err != nil {
		return e.failure(in.RequestID, err)
	}
	payload, invalid := domain.EncodeHistoryExport(snapshot)
	if invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	root, err := historyout.Open(e.lock)
	if err != nil {
		return domain.Failure(in.RequestID, historyOutputError(err))
	}
	defer root.Close()
	root.SetTestHook(e.config.HistoryTestHook)
	e.control.Lock()
	created := e.db.CreateHistoryExport(ctx, e.config.OwnerInstanceID, in)
	if created.Envelope.Error != nil && persistenceFault(created.Envelope.Error) {
		// Bind the incident to the exact committed operation when its receipt
		// is readable, otherwise bind the pre-effect request and fingerprint.
		saved, replayErr := e.db.Replay(context.Background(), in.RequestID, store.HistoryCreateOperation, store.HistoryCreateFingerprint(in))
		if replayErr != nil {
			e.fenceLocked("", "history_fence_unbound", "History storage acknowledgement could not be attributed.")
		} else if saved == nil {
			e.fenceHistoryMutationLocked(store.HistoryCreateOperation, in.RequestID, store.HistoryCreateFingerprint(in))
		} else if opID := historyResultOperationID(*saved); opID != "" {
			e.fenceHistoryOperationLocked(opID)
		} else {
			e.fenceLocked("", "history_fence_unbound", "The saved history operation could not be attributed.")
		}
	}
	e.control.Unlock()
	if created.Envelope.Error != nil {
		if created.Envelope.Error.Code == "write_uncertain" {
			// The create receipt may already exist but no publisher was started.
			// If so, freeze the exact saved operation for reconciliation while
			// retaining the original uncertain response for this request.
			settleCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if saved, replayErr := e.historyReplay(settleCtx, in.RequestID, store.HistoryCreateOperation, store.HistoryCreateFingerprint(in)); replayErr == nil && saved != nil {
				_ = e.historyCurrentResult(settleCtx, *saved)
			}
			cancel()
		}
		return created
	}
	var envelope struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	raw, err := json.Marshal(created.Envelope.Data)
	if err != nil || json.Unmarshal(raw, &envelope) != nil || !domain.ValidID(envelope.Operation.OperationID) {
		return e.failure(in.RequestID, domain.Fail("storage_failed", "The created history operation cannot be verified."))
	}
	if e.config.HistoryTestHook != nil {
		if err = e.config.HistoryTestHook("after_create_receipt"); err != nil {
			return e.failure(in.RequestID, err)
		}
	}
	var effect domain.HistoryPublicationEffect
	observed, publishErr := root.Publish(ctx, native, payload, func(permitCtx context.Context) error {
		// Order an ambiguous permit acknowledgement against scheduler admission.
		// No worker may start between that fault and the in-memory/durable fence.
		e.control.Lock()
		defer e.control.Unlock()
		if e.fenced {
			return domain.Fail("dispatch_fenced", "History publication is blocked by an unresolved owner incident.")
		}
		var permitErr error
		effect, permitErr = e.db.PermitHistoryExport(permitCtx, e.config.OwnerInstanceID, envelope.Operation.OperationID, envelope.Operation.Revision)
		if permitErr != nil {
			e.guardHistoryOperationLocked(permitErr, envelope.Operation.OperationID)
		} else if e.config.HistoryTestHook != nil {
			permitErr = e.config.HistoryTestHook("after_permit")
		}
		return permitErr
	})
	if effect.EffectID != "" {
		// Finishing a permitted effect must still be attempted when the client
		// disconnects; owner shutdown waits for this registered synchronous call.
		settleCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		e.control.Lock()
		finishErr := e.db.FinishHistoryExport(settleCtx, e.config.OwnerInstanceID, effect, observed, len(payload), publishErr)
		cancel()
		if finishErr != nil {
			e.fenceHistoryOperationLocked(envelope.Operation.OperationID)
			e.control.Unlock()
			return e.failure(in.RequestID, finishErr)
		}
		e.control.Unlock()
	} else if publishErr != nil {
		// If the durable journal proves no permit was issued, freeze this
		// failed request for explicit reconciliation rather than leaving a
		// permanently waiting operation. An ambiguous committed permit must
		// never be inferred absent from the empty callback result.
		settleCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		e.control.Lock()
		reason := "request_interrupted"
		if errors.Is(publishErr, historyout.ErrConflict) {
			reason = "target_conflict"
		} else if errors.Is(publishErr, historyout.ErrUnsupported) {
			reason = "root_unavailable"
		}
		markErr := e.db.MarkHistoryExportPrePermitFailure(settleCtx, e.config.OwnerInstanceID, envelope.Operation.OperationID, reason)
		cancel()
		// A permit error was fenced inside its callback before releasing
		// control. A later journal failure receives its own exact operation link.
		if markErr != nil {
			e.guardHistoryOperationLocked(markErr, envelope.Operation.OperationID)
		}
		e.control.Unlock()
	}
	return e.historyCurrentResult(context.Background(), created)
}

func (e *Engine) HistoryOperation(ctx context.Context, id string) domain.Result {
	requestID := domain.NewID()
	if !e.beginOwnerOperation() {
		return e.failure(requestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	operation, err := e.db.HistoryExportOperation(ctx, id)
	if err != nil {
		return e.failure(requestID, err)
	}
	return domain.Success(200, requestID, struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}{operation}, nil)
}

func (e *Engine) ReconcileHistoryOperation(ctx context.Context, id string, in domain.ReconcileHistoryOperationInput) domain.Result {
	if invalid := domain.ValidateReconcileHistoryOperationInput(in); invalid != nil {
		return domain.Failure(in.RequestID, invalid)
	}
	if !e.beginOwnerOperation() {
		return e.failure(in.RequestID, ownerClosing())
	}
	defer e.endOwnerOperation()
	e.historyMu.Lock()
	defer e.historyMu.Unlock()
	e.control.Lock()
	actorsDrained := !e.fenceUncertain
	result := e.db.ReconcileHistoryExport(ctx, e.config.OwnerInstanceID, id, in, actorsDrained)
	if result.Envelope.Error != nil && persistenceFault(result.Envelope.Error) {
		verifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		saved, verifyErr := e.db.VerifyHistoryReconciliationReceipt(verifyCtx, e.config.OwnerInstanceID, id, in)
		if verifyErr != nil {
			e.preserveUnrecordedHistoryFenceLocked()
		} else if saved != nil {
			result = *saved
		} else if gate, gateErr := e.db.Gate(verifyCtx); gateErr != nil {
			e.preserveUnrecordedHistoryFenceLocked()
		} else if gate.State == "fenced" {
			e.fenced = true
		} else {
			// No prior global fence exists. Bind this unresolved mutation
			// to the exact history operation before allowing other work.
			e.fenceHistoryOperationLocked(id)
		}
		cancel()
	}
	if result.Envelope.Error == nil && !e.fenceUncertain {
		if gate, err := e.db.Gate(context.Background()); err == nil && gate.State == "open" {
			e.fenced = false
		}
	}
	e.control.Unlock()
	return result
}

func historyResultOperationID(result domain.Result) string {
	var data struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	raw, err := json.Marshal(result.Envelope.Data)
	if err != nil || json.Unmarshal(raw, &data) != nil || !domain.ValidID(data.Operation.OperationID) {
		return ""
	}
	return data.Operation.OperationID
}
