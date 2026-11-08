package cli

import (
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const historyHelp = `
Metadata history (incomplete observational history, not a backup):
  council history capture --project ID [--request-id ID]
  council history show ID
  council history records ID --kind tasks|attempts [--limit N] [--cursor C]
  council history export-preview --snapshot ID --revision 1 --filename history-NAME.json [--request-id ID]
  council history export --preview ID --revision 1 [--request-id ID]
  council history operation ID
  council history reconcile ID --revision R [--request-id ID]
`

func addHistoryFlags(values map[string]bool) {
	for _, v := range []string{"snapshot", "preview", "filename", "kind"} {
		values[v] = true
	}
}
func historyCommand(name string) bool {
	switch name {
	case "history capture", "history show", "history records", "history export-preview", "history export", "history operation", "history reconcile":
		return true
	}
	return false
}
func historyMutation(op string) bool {
	switch op {
	case "history.snapshots.create", "history.exports.preview", "history.exports.create", "history.operations.reconcile":
		return true
	}
	return false
}
func parseHistory(c command, pos []string, flags map[string]string) (command, *domain.Error) {
	allowed := map[string]bool{"json": true, "api-version": true}
	allow := func(keys ...string) {
		for _, k := range keys {
			allowed[k] = true
		}
	}
	switch c.name {
	case "history capture":
		if len(pos) != 2 {
			return c, domain.Invalid("history capture command")
		}
		c.op, c.method, c.route = "history.snapshots.create", "POST", "/history/snapshots"
		allow("project", "request-id")
	case "history show", "history records", "history operation", "history reconcile":
		if len(pos) != 3 || !domain.ValidID(pos[2]) {
			return c, domain.Invalid("history resource identity")
		}
		c.params.ID = pos[2]
		c.method = "GET"
		switch c.name {
		case "history show":
			c.op, c.route = "history.snapshots.get", "/history/snapshots/"+pos[2]
		case "history records":
			c.op, c.route = "history.snapshots.records", "/history/snapshots/"+pos[2]+"/records"
			allow("kind", "limit", "cursor")
		case "history operation":
			c.op, c.route = "history.operations.get", "/history/operations/"+pos[2]
		case "history reconcile":
			c.op, c.method, c.route = "history.operations.reconcile", "POST", "/history/operations/"+pos[2]+"/reconciliations"
			allow("revision", "request-id")
		}
	case "history export-preview":
		if len(pos) != 2 {
			return c, domain.Invalid("history preview command")
		}
		c.op, c.method, c.route = "history.exports.preview", "POST", "/history/exports/preview"
		allow("snapshot", "revision", "filename", "request-id")
	case "history export":
		if len(pos) != 2 {
			return c, domain.Invalid("history export command")
		}
		c.op, c.method, c.route = "history.exports.create", "POST", "/history/exports"
		allow("preview", "revision", "request-id")
	default:
		return c, domain.Invalid("documented history command")
	}
	for key := range flags {
		if !allowed[key] {
			return c, domain.Invalid("options supported by this history command")
		}
	}
	if c.op == "history.snapshots.records" {
		q := domain.HistoryRecordsQuery{Kind: flags["kind"], Limit: flags["limit"], Cursor: flags["cursor"]}
		if invalid := domain.ValidateHistoryRecordsQuery(q); invalid != nil {
			return c, invalid
		}
		c.params.Query["kind"] = q.Kind
		if q.Limit != "" {
			c.params.Query["limit"] = q.Limit
		}
		if q.Cursor != "" {
			c.params.Query["cursor"] = q.Cursor
		}
	}
	if c.method == "GET" {
		return c, nil
	}
	var invalid *domain.Error
	c.requestID, invalid = mutationRequestID(flags)
	if invalid != nil {
		return c, invalid
	}
	switch c.op {
	case "history.snapshots.create":
		in := domain.CaptureHistoryInput{RequestID: c.requestID, ProjectID: flags["project"]}
		if invalid = domain.ValidateCaptureHistoryInput(in); invalid != nil {
			return c, invalid
		}
		return encodeExecution(c, in)
	case "history.exports.preview":
		in := domain.PreviewHistoryExportInput{RequestID: c.requestID, SnapshotID: flags["snapshot"], SnapshotRevision: flags["revision"], Filename: flags["filename"]}
		if invalid = domain.ValidatePreviewHistoryExportInput(in); invalid != nil {
			return c, invalid
		}
		return encodeExecution(c, in)
	case "history.exports.create":
		in := domain.ExecuteHistoryExportInput{RequestID: c.requestID, PreviewID: flags["preview"], PreviewRevision: flags["revision"]}
		if invalid = domain.ValidateExecuteHistoryExportInput(in); invalid != nil {
			return c, invalid
		}
		return encodeExecution(c, in)
	case "history.operations.reconcile":
		in := domain.ReconcileHistoryOperationInput{RequestID: c.requestID, ExpectedRevision: flags["revision"], Action: "verify_no_publication"}
		if invalid = domain.ValidateReconcileHistoryOperationInput(in); invalid != nil {
			return c, invalid
		}
		return encodeExecution(c, in)
	}
	return c, domain.Invalid("documented history operation")
}
