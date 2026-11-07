package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphReceivingSourceUsesSealedInputAndAtomicReceiptBeforeNativeCreation(t *testing.T) {
	f := graphOutputAdmittedAttemptFixture(t)
	e, a := f.engine, f.attempt
	ctx := context.Background()
	userWorking, err := os.ReadFile(filepath.Join(f.repo, "source.txt"))
	if err != nil {
		t.Fatal(err)
	}
	receiving, err := e.prepareGraphReceivingSource(ctx, a, nil)
	if err != nil || receiving == nil || receiving.Materialized.Snapshot == nil || receiving.Materialized.Receipt.State != "complete" {
		t.Fatal("Exact admitted receiver did not materialize its closed input", receiving, err)
	}
	snapshot := receiving.Materialized.Snapshot
	if err := e.recordGraphReceivingSource(ctx, a, receiving); err != nil {
		t.Fatal("Exact source and receipt did not commit together", err)
	}
	if err := e.recordGraphReceivingSource(ctx, a, receiving); err != nil {
		t.Fatal("Same acknowledged receipt/source did not reconcile without another row", err)
	}
	current, err := e.db.Attempt(ctx, a.ID)
	if err != nil || current.SourceSnapshotID == nil || *current.SourceSnapshotID != snapshot.ID || current.LaunchIntent || current.StartedAt != nil {
		t.Fatal("Receiving registration implied native launch or lost exact source", current, err)
	}
	dispatch, err := e.db.GraphDispatchForAttempt(ctx, a.ID)
	if err != nil || dispatch.Public.Materialization == nil || dispatch.MaterializationArtifact == nil || dispatch.PrivateMaterialization == nil || dispatch.PrivateMaterialization.WorkspaceIdentity != receiving.Materialized.WorkspaceIdentity {
		t.Fatal("Atomic private receipt association is missing", dispatch, err)
	}
	if err := e.recheckGraphReceivingSource(ctx, a, receiving); err != nil {
		t.Fatal("Unchanged exact private receiver did not verify", err)
	}
	managedFile := filepath.Join(snapshot.ExecutionPath, "source.txt")
	before, err := os.ReadFile(managedFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedFile, append(append([]byte{}, before...), []byte("Unadmitted receiving edit\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.recheckGraphReceivingSource(ctx, a, receiving); err == nil {
		t.Fatal("Receiving bytes changed without blocking native creation")
	}
	if err := os.WriteFile(managedFile, before, 0600); err != nil {
		t.Fatal(err)
	}
	afterUser, err := os.ReadFile(filepath.Join(f.repo, "source.txt"))
	if err != nil || !bytes.Equal(userWorking, afterUser) {
		t.Fatal("Receiving work changed the user's current working bytes", err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || !bytes.Equal(f.originalIndex, afterIndex) {
		t.Fatal("Receiving work changed the user's current raw index", err)
	}
}
