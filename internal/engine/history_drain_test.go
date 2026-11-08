package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryOwnerCloseDrainsAdmittedPublisher(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-drain-test.json"})).Preview
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
	done := make(chan domain.Result, 1)
	go func() {
		done <- e.ExecuteHistoryExport(ctx, domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: preview.Revision})
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("publisher did not reach the durable receipt boundary")
	}
	closed := make(chan error, 1)
	go func() { closed <- e.Close() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		e.lifecycleMu.Lock()
		closing, active := e.lifecycleClosing, e.lifecycleActive
		e.lifecycleMu.Unlock()
		if closing {
			if active == 0 {
				t.Fatal("admitted publisher was not retained during shutdown")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner never closed admission")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-closed:
		t.Fatalf("owner closed before publisher drained: %v", err)
	default:
	}
	if result := e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project}); result.Envelope.Error == nil || result.Envelope.Error.Code != "owner_closing" {
		t.Fatalf("closing owner accepted a new history mutation: %+v", result)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-done:
		if result.Envelope.Error != nil || historyResultData[struct {
			Operation domain.HistoryExportOperation `json:"operation"`
		}](t, result).Operation.State != "published" {
			t.Fatalf("admitted publisher did not complete its exact effect: %+v", result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("publisher did not drain")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("owner did not finish closing")
	}
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatalf("clean drained handoff was unexpectedly fenced: %+v %v", gate, err)
	}
}

func TestHistoryOwnerCloseTimeoutRetainsDirtyEpoch(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	summary := historyResultData[struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}](t, e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project})).Snapshot
	snapshot, err := e.db.HistorySnapshot(ctx, summary.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, invalid := domain.EncodeHistoryExport(snapshot)
	if invalid != nil {
		t.Fatal(invalid)
	}
	preview := historyResultData[struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}](t, e.PreviewHistoryExport(ctx, domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: summary.SnapshotID, SnapshotRevision: "1", Filename: "history-timeout-test.json"})).Preview
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
	done := make(chan domain.Result, 1)
	go func() {
		done <- e.ExecuteHistoryExport(ctx, domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview.PreviewID, PreviewRevision: preview.Revision})
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("publisher did not reach receipt barrier")
	}
	closed := make(chan error, 1)
	go func() { closed <- e.Close() }()
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("timed-out owner unexpectedly reported a clean close")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("owner did not enforce the real drain timeout")
	}
	select {
	case result := <-done:
		t.Fatalf("fixture publisher unexpectedly drained: %+v", result)
	default:
	}
	if result := e.CaptureHistory(ctx, domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project}); result.Envelope.Error == nil || result.Envelope.Error.Code != "owner_closing" {
		t.Fatalf("timed-out owner admitted new work: %+v", result)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-done:
		if result.Envelope.Error != nil || historyResultData[struct {
			Operation domain.HistoryExportOperation `json:"operation"`
		}](t, result).Operation.State != "published" {
			t.Fatalf("retained publisher failed to finish: %+v", result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("publisher did not exit after release")
	}
	path := filepath.Join(e.lock.Root(), preview.Filename)
	before, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, wantBytes) {
		t.Fatal("drained publisher output differed from captured bytes", err)
	}
	// Close deliberately left the handles open after timeout. The actor has now
	// exited, so this controlled fixture may release those old handles explicitly.
	if err = e.workspace.Close(); err != nil {
		t.Fatal(err)
	}
	if err = e.evidence.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatalf("timed-out epoch did not fence its successor: %+v %v", gate, err)
	}
	incident, err := e.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.Code != "owner_epoch_uncertain" {
		t.Fatalf("missing unknown owner incident: %+v %v", incident, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("successor changed the retained output", err)
	}
}
