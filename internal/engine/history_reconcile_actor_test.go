package engine

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

func TestHistoryReconciliationWaitsForRegisteredPublisher(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-actor-race.json"})).Preview
	create := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: preview.Revision}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	e.config.HistoryTestHook = func(stage string) error {
		if stage == "after_create_receipt" {
			close(entered)
			<-release
		}
		return nil
	}
	published := make(chan domain.Result, 1)
	go func() { published <- e.ExecuteHistoryExport(ctx, create) }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("publisher did not reach registered receipt barrier")
	}
	saved, err := e.db.Replay(ctx, create.RequestID, store.HistoryCreateOperation, store.HistoryCreateFingerprint(create))
	if err != nil || saved == nil {
		t.Fatal("missing publisher receipt", err)
	}
	op := historyResultData[struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}](t, *saved).Operation
	if op.State != "prepared" {
		t.Fatal("fixture did not pause before permit", op)
	}
	if e.historyMu.TryLock() {
		e.historyMu.Unlock()
		t.Fatal("publisher did not retain its admission barrier")
	}
	started, reconciled := make(chan struct{}), make(chan domain.Result, 1)
	go func() {
		close(started)
		reconciled <- e.ReconcileHistoryOperation(ctx, op.OperationID, domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: op.Revision, Action: "verify_no_publication"})
	}()
	<-started
	select {
	case result := <-reconciled:
		t.Fatalf("reconciliation ran while publication authority was still active: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err = os.Lstat(preview.DestinationPath); !os.IsNotExist(err) {
		t.Fatal("output existed before the active publisher was released", err)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-published:
		if result.Envelope.Error != nil || historyResultData[struct {
			Operation domain.HistoryExportOperation `json:"operation"`
		}](t, result).Operation.State != "published" {
			t.Fatal("publisher did not finish", result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("publisher did not finish")
	}
	select {
	case result := <-reconciled:
		if result.Envelope.Error == nil || result.Envelope.Error.Code != "revision_conflict" {
			t.Fatal("stale reconciliation made a no-publication decision", result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("reconciliation remained blocked after publisher exit")
	}
	data, err := os.ReadFile(preview.DestinationPath)
	if err != nil || len(data) == 0 {
		t.Fatal("published output not retained", err)
	}
	again := e.ExecuteHistoryExport(ctx, create)
	if again.Envelope.Error != nil || again.Envelope.Receipt == nil || *again.Envelope.Receipt != *saved.Envelope.Receipt {
		t.Fatal("receipt changed on replay", again)
	}
	if after, err := os.ReadFile(preview.DestinationPath); err != nil || string(after) != string(data) {
		t.Fatal("receipt replay modified output", err)
	}
}
