package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
)

// A fixed fake native process emits its init JSON, then sleeps. The barrier
// observes the actual open, durably checkpointed stream before shutdown.
func TestRecoveryShutdownDuringPartialOutputRetainsPrimaryFailure(t *testing.T) {
	ctx := context.Background()
	old, project, _ := testEngine(t)
	admission := admitClaude(t, old, project, "slow")
	var artifactID string
	awaitFixtureCondition(t, "fake worker did not publish partial stdout before shutdown", func() bool {
		a, err := old.db.Attempt(ctx, admission.Attempt.ID)
		if err != nil || a.Phase != "running" {
			return false
		}
		page, err := old.db.Artifacts(ctx, a.ID, 100, "")
		if err != nil {
			return false
		}
		for _, m := range page.Items {
			if m.Kind == "stdout" && !m.Closed && m.ByteLength != "0" {
				artifactID = m.ID
				return true
			}
		}
		return false
	})
	before, err := old.evidence.Read(ctx, artifactID, evidence.ReadOptions{Offset: "0"})
	if err != nil || before.StreamClosed {
		t.Fatal("partial stream was not readable while running", before, err)
	}
	prefix, err := base64.StdEncoding.DecodeString(before.BytesBase64)
	if err != nil || !bytes.Contains(prefix, []byte(`"type":"system"`)) || !bytes.Contains(prefix, []byte(`"subtype":"init"`)) {
		t.Fatal("fixture did not emit its actual partial native output", string(prefix), err)
	}
	old.recordFailure(admission.Attempt.ID, primary("fixture_primary_failure", "Controlled failure before shutdown."))
	observed, err := old.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || observed.PrimaryFailure == nil || observed.PrimaryFailure.Code != "fixture_primary_failure" || observed.Phase != "running" {
		t.Fatal("known earlier failure was not recorded while worker was running", observed, err)
	}
	stopped := old.Shutdown(ctx, old.config.OwnerInstanceID, domain.NewID())
	if stopped.Envelope.Error != nil || stopped.Envelope.Receipt == nil {
		t.Fatal("owner shutdown was not accepted", stopped)
	}
	if err = old.Close(); err != nil {
		t.Fatal("owner did not drain after shutdown", err)
	}
	closed, err := old.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || closed.Phase != "settled" || closed.Outcome == nil || *closed.Outcome != "failed" || closed.PrimaryFailure == nil || closed.PrimaryFailure.Code != "fixture_primary_failure" || closed.ResponseArtifactID != nil {
		t.Fatal("shutdown hid failure or fabricated final success", closed, err)
	}
	next, err := New(old.db, old.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	retained, err := next.db.Attempt(ctx, admission.Attempt.ID)
	if err != nil || retained.Revision != closed.Revision || retained.Outcome == nil || *retained.Outcome != *closed.Outcome || retained.PrimaryFailure == nil || *retained.PrimaryFailure != *closed.PrimaryFailure || retained.ResponseArtifactID != nil {
		t.Fatal("restart changed shutdown terminal or primary failure", retained, closed, err)
	}
	after, err := next.evidence.Read(ctx, artifactID, evidence.ReadOptions{Offset: "0"})
	if err != nil || !after.StreamClosed {
		t.Fatal("retained stdout was not inspectable after restart", after, err)
	}
	output, err := base64.StdEncoding.DecodeString(after.BytesBase64)
	if err != nil || !bytes.HasPrefix(output, prefix) {
		t.Fatal("shutdown or restart lost the actual captured output prefix", string(output), err)
	}
	history, err := next.db.Attempts(ctx, retained.TaskID, 100, "")
	if err != nil || len(history.Items) != 1 || history.Items[0].ID != retained.ID {
		t.Fatal("shutdown or restart dispatched a replacement", history, err)
	}
}
