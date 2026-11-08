package app

import (
	"context"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// HistoryController is implemented only by the current owner. Historical
// identifiers are observations, never client-provided effect or ownership proof.
type HistoryController interface {
	CaptureHistory(context.Context, domain.CaptureHistoryInput) domain.Result
	HistorySnapshot(context.Context, string) domain.Result
	HistoryRecords(context.Context, string, domain.HistoryRecordsQuery) domain.Result
	PreviewHistoryExport(context.Context, domain.PreviewHistoryExportInput) domain.Result
	ExecuteHistoryExport(context.Context, domain.ExecuteHistoryExportInput) domain.Result
	HistoryOperation(context.Context, string) domain.Result
	ReconcileHistoryOperation(context.Context, string, domain.ReconcileHistoryOperationInput) domain.Result
}

func (s *Service) executeHistory(ctx context.Context, op string, p Params) domain.Result {
	id := domain.MutationIdentity(p.Body)
	controller, ok := s.Execution.(HistoryController)
	if !ok {
		return domain.Failure(id, domain.Fail("capability_unavailable", "Metadata history requires a compatible current Council owner."))
	}
	switch op {
	case "history.snapshots.get", "history.snapshots.records", "history.operations.get":
		if len(p.Body) != 0 || !domain.ValidID(p.ID) {
			return domain.Failure(id, domain.Invalid("empty history read and exact identity"))
		}
		if op == "history.snapshots.records" {
			if invalid := domain.ValidateQuery(p.Query, "kind", "limit", "cursor"); invalid != nil {
				return domain.Failure(id, invalid)
			}
			for _, key := range []string{"kind", "limit", "cursor"} {
				if value, present := p.Query[key]; present && value == "" {
					return domain.Failure(id, domain.Invalid("nonempty history query"))
				}
			}
			q := domain.HistoryRecordsQuery{Kind: p.Query["kind"], Limit: p.Query["limit"], Cursor: p.Query["cursor"]}
			if invalid := domain.ValidateHistoryRecordsQuery(q); invalid != nil {
				return domain.Failure(id, invalid)
			}
			return controller.HistoryRecords(ctx, p.ID, q)
		}
		if invalid := domain.ValidateQuery(p.Query); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if op == "history.snapshots.get" {
			return controller.HistorySnapshot(ctx, p.ID)
		}
		return controller.HistoryOperation(ctx, p.ID)
	case "history.snapshots.create":
		if p.ID != "" {
			return domain.Failure(id, domain.Invalid("no target on history capture"))
		}
		if invalid := domain.ValidateQuery(p.Query); invalid != nil {
			return domain.Failure(id, invalid)
		}
		var in domain.CaptureHistoryInput
		if invalid := domain.StrictDecodeHistory(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidateCaptureHistoryInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.CaptureHistory(ctx, in)
	case "history.exports.preview":
		if p.ID != "" {
			return domain.Failure(id, domain.Invalid("no target on history preview"))
		}
		if invalid := domain.ValidateQuery(p.Query); invalid != nil {
			return domain.Failure(id, invalid)
		}
		var in domain.PreviewHistoryExportInput
		if invalid := domain.StrictDecodeHistory(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidatePreviewHistoryExportInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.PreviewHistoryExport(ctx, in)
	case "history.exports.create":
		if p.ID != "" {
			return domain.Failure(id, domain.Invalid("no target on history export"))
		}
		if invalid := domain.ValidateQuery(p.Query); invalid != nil {
			return domain.Failure(id, invalid)
		}
		var in domain.ExecuteHistoryExportInput
		if invalid := domain.StrictDecodeHistory(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidateExecuteHistoryExportInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.ExecuteHistoryExport(ctx, in)
	case "history.operations.reconcile":
		if !domain.ValidID(p.ID) {
			return domain.Failure(id, domain.Invalid("history operation identity"))
		}
		if invalid := domain.ValidateQuery(p.Query); invalid != nil {
			return domain.Failure(id, invalid)
		}
		var in domain.ReconcileHistoryOperationInput
		if invalid := domain.StrictDecodeHistory(p.Body, &in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		if invalid := domain.ValidateReconcileHistoryOperationInput(in); invalid != nil {
			return domain.Failure(id, invalid)
		}
		return controller.ReconcileHistoryOperation(ctx, p.ID, in)
	}
	return domain.Failure(id, domain.Fail("not_found", "This metadata history operation does not exist."))
}
