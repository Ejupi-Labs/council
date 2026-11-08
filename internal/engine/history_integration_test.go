package engine

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func historyResultData[T any](t *testing.T, result domain.Result) T {
	t.Helper()
	var value T
	if result.Envelope.Error != nil {
		t.Fatal(result.Envelope.Error)
	}
	b, err := json.Marshal(result.Envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHistoryActualOwnerCapturePublishAndReplay(t *testing.T) {
	ctx := context.Background()
	e, project, repo := testEngine(t)
	admission := admit(t, e, project, "response")
	settled(t, e, admission.Attempt.ID)
	capture := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project}
	first := e.CaptureHistory(ctx, capture)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, first).Snapshot
	if summary.TaskCount != "1" || summary.AttemptCount != "1" || summary.Completeness != domain.HistoryCompleteness {
		t.Fatal(summary)
	}
	if replay := e.CaptureHistory(ctx, capture); replay.Envelope.Error != nil || historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, replay).Snapshot.SnapshotID != summary.SnapshotID {
		t.Fatal("capture replay did not retain snapshot", replay)
	}
	previewInput := domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-engine-test.json"}
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, previewInput)).Preview
	execute := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"}
	firstExport := e.ExecuteHistoryExport(ctx, execute)
	operation := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, firstExport).Operation
	if operation.State != "published" || operation.Durability == nil {
		t.Fatal("publication not observed", operation)
	}
	payload, err := os.ReadFile(preview.DestinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), repo) || strings.Contains(string(payload), "Perform the explicit local fixture task") {
		t.Fatal("excluded project content escaped into the export")
	}
	var exported domain.HistoryPayload
	if err = json.Unmarshal(payload, &exported); err != nil || domain.ValidateHistoryPayload(exported) != nil {
		t.Fatal("invalid exported metadata", err)
	}
	replay := e.ExecuteHistoryExport(ctx, execute)
	next := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, replay).Operation
	if next.OperationID != operation.OperationID || next.State != "published" {
		t.Fatal("replay did not return exact published operation", next)
	}
	after, err := os.ReadFile(filepath.Join(e.lock.Root(), previewInput.Filename))
	if err != nil || string(after) != string(payload) {
		t.Fatal("replay changed output", err)
	}
	if result := e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: previewInput.Filename}); result.Envelope.Error == nil {
		t.Fatal("existing output was accepted for overwrite")
	}
}

func TestHistoryActualOwnerChangedTargetNeedsReconciliation(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	previewInput := domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-conflict-test.json"}
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, previewInput)).Preview
	if err := os.WriteFile(preview.DestinationPath, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	execute := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"}
	result := e.ExecuteHistoryExport(ctx, execute)
	operation := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, result).Operation
	if operation.State != "prepared" || operation.NextAction != "reconcile" {
		t.Fatal("pre-permit conflict did not become actionable", operation)
	}
	data, err := os.ReadFile(preview.DestinationPath)
	if err != nil || string(data) != "unrelated" {
		t.Fatal("unrelated destination changed", err)
	}
	reconcile := e.ReconcileHistoryOperation(ctx, operation.OperationID, domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: operation.Revision, Action: "verify_no_publication"})
	decision := historyResultData[domain.HistoryReconciliationResult](t, reconcile)
	if decision.Decision != "no_publication" {
		t.Fatal("verified no-publication did not reconcile", decision)
	}
	if data, err = os.ReadFile(preview.DestinationPath); err != nil || string(data) != "unrelated" {
		t.Fatal("reconciliation changed an unrelated destination", err)
	}
}

func TestHistoryActualOwnerLostCreateAcknowledgementRequiresReconciliation(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-lost-create.json"})).Preview
	in := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"}
	failed := false
	e.db.AfterCommit = func() error {
		if !failed {
			failed = true
			return errors.New("controlled lost acknowledgement")
		}
		return nil
	}
	first := e.ExecuteHistoryExport(ctx, in)
	e.db.AfterCommit = nil
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatal("lost commit was not reported", first)
	}
	retry := e.ExecuteHistoryExport(ctx, in)
	operation := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, retry).Operation
	if operation.State != "prepared" || operation.NextAction != "reconcile" {
		t.Fatal("stranded operation did not require reconciliation", operation)
	}
	if _, err := os.Lstat(preview.DestinationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retry created output after uncertain acknowledgement", err)
	}
	decision := historyResultData[domain.HistoryReconciliationResult](t, e.ReconcileHistoryOperation(ctx, operation.OperationID, domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: operation.Revision, Action: "verify_no_publication"}))
	if decision.Decision != "no_publication" {
		t.Fatal("linked no-publication incident did not reconcile", decision)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("resolved history-only incident left dispatch fenced", gate, err)
	}
	e.control.Lock()
	fenced := e.fenced
	e.control.Unlock()
	if fenced {
		t.Fatal("in-memory history fence remained after its durable incident resolved")
	}
}

func TestHistoryActualOwnerUnrecordedFencePreventsPermit(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-unrecorded-fence.json"})).Preview
	e.control.Lock()
	e.fenced = true
	e.fenceUncertain = true
	e.control.Unlock()
	result := e.ExecuteHistoryExport(ctx, domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"})
	operation := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, result).Operation
	if operation.State != "prepared" || operation.NextAction != "reconcile" {
		t.Fatal("fenced operation was not retained for inspection", operation)
	}
	if _, err := os.Lstat(preview.DestinationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("in-memory fence allowed publication", err)
	}
}

func TestHistoryReconciliationPreservesIndependentIncident(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-independent-fence.json"})).Preview
	in := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"}
	failed := false
	e.db.AfterCommit = func() error {
		if !failed {
			failed = true
			return errors.New("controlled lost acknowledgement")
		}
		return nil
	}
	first := e.ExecuteHistoryExport(ctx, in)
	e.db.AfterCommit = nil
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(first)
	}
	operation := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, e.ExecuteHistoryExport(ctx, in)).Operation
	e.fence("", "independent_test_incident", "An independent fixture incident remains unresolved.")
	decision := historyResultData[domain.HistoryReconciliationResult](t, e.ReconcileHistoryOperation(ctx, operation.OperationID, domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: operation.Revision, Action: "verify_no_publication"}))
	if decision.Decision != "no_publication" {
		t.Fatal(decision)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatal("history reconciliation removed another incident", gate, err)
	}
	incident, err := e.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.Code != "independent_test_incident" {
		t.Fatal("wrong incident remains", incident, err)
	}
}

func TestHistoryCaptureReceiptIncidentCanReconcile(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	in := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project}
	failed := false
	e.db.AfterCommit = func() error {
		if !failed {
			failed = true
			return errors.New("controlled capture acknowledgement loss")
		}
		return nil
	}
	first := e.CaptureHistory(ctx, in)
	e.db.AfterCommit = nil
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(first)
	}
	replay := e.CaptureHistory(ctx, in)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, replay).Snapshot
	if !domain.ValidID(summary.SnapshotID) {
		t.Fatal("exact captured receipt was not retained", summary)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatal("uncertain capture did not retain its incident", gate, err)
	}
	view, err := e.db.RecoveryIncident(ctx, *gate.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	reconciled := e.ReconcileIncident(ctx, *gate.IncidentID, domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: view.Revision, Action: "verify_and_reconcile"})
	if reconciled.Envelope.Error != nil {
		t.Fatal(reconciled.Envelope.Error)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("proven receipt incident did not reopen its gate", gate, err)
	}
}

func TestHistoryReconciliationLostAcknowledgementUsesExactReceipt(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-reconcile-ack.json"})).Preview
	if err := os.WriteFile(preview.DestinationPath, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	operation := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, e.ExecuteHistoryExport(ctx, domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: "1"})).Operation
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: operation.Revision, Action: "verify_no_publication"}
	failed := false
	e.db.AfterCommit = func() error {
		if !failed {
			failed = true
			return errors.New("controlled lost reconciliation acknowledgement")
		}
		return nil
	}
	first := e.ReconcileHistoryOperation(ctx, operation.OperationID, in)
	e.db.AfterCommit = nil
	decision := historyResultData[domain.HistoryReconciliationResult](t, first)
	if decision.Decision != "no_publication" {
		t.Fatal(decision)
	}
	replay := e.ReconcileHistoryOperation(ctx, operation.OperationID, in)
	again := historyResultData[domain.HistoryReconciliationResult](t, replay)
	if first.Envelope.Receipt == nil || replay.Envelope.Receipt == nil || *first.Envelope.Receipt != *replay.Envelope.Receipt || again.Decision != decision.Decision {
		t.Fatal("reconciliation acknowledgement did not preserve its exact receipt", first, replay)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("verified reconciliation left a new unbound incident", gate, err)
	}
}

func TestHistoryIncidentReconciliationLostAcknowledgementUsesExactReceipt(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	capture := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project}
	failed := false
	e.db.AfterCommit = func() error {
		if !failed {
			failed = true
			return errors.New("controlled lost capture acknowledgement")
		}
		return nil
	}
	first := e.CaptureHistory(ctx, capture)
	e.db.AfterCommit = nil
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(first)
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.IncidentID == nil {
		t.Fatal(gate, err)
	}
	view, err := e.db.RecoveryIncident(ctx, *gate.IncidentID)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: view.Revision, Action: "verify_and_reconcile"}
	failed = false
	e.db.AfterCommit = func() error {
		if !failed {
			failed = true
			return errors.New("controlled lost incident reconciliation acknowledgement")
		}
		return nil
	}
	result := e.ReconcileIncident(ctx, *gate.IncidentID, in)
	e.db.AfterCommit = nil
	if result.Envelope.Error != nil || result.Envelope.Receipt == nil {
		t.Fatal("exact committed incident decision was not recovered", result)
	}
	retry := e.ReconcileIncident(ctx, *gate.IncidentID, in)
	if retry.Envelope.Error != nil || retry.Envelope.Receipt == nil || *retry.Envelope.Receipt != *result.Envelope.Receipt {
		t.Fatal("incident retry changed the receipt", retry)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("verified incident decision remained globally fenced", gate, err)
	}
}
