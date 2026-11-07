package engine

import (
	"context"
	json "encoding/json/v2"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/process"
)

type recoveryHeldStdout struct {
	target  io.Writer
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *recoveryHeldStdout) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.target.Write(p)
}

// Actual standalone fake author runs under an owned native process. A blocked
// real stdout drain makes the watchdog/cleanup ordering deterministic without
// inventing a cleanup result or an output artifact.
func TestRecoveryStandaloneAuthorActualWatchdogClosure(t *testing.T) {
	ctx := context.Background()
	e, project, repo := testEngine(t)
	stopFixtureScheduler(t, e)
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("old\n")
	if err := os.WriteFile(filepath.Join(repo, "docs", "note.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	interopGit(t, repo, "add", "--", "docs/note.md")
	interopGit(t, repo, "commit", "--quiet", "-m", "controlled standalone document")
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	e.config.Prepare = func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		spec.Stdout = &recoveryHeldStdout{target: spec.Stdout, entered: entered, release: release}
		return process.Prepare(ctx, spec)
	}
	started := e.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Replace \"old\" with \"new\" in docs/note.md only", ExecutionMode: "fake", FakeScenario: "workflow-document"})
	if started.Envelope.Error != nil {
		t.Fatal(started)
	}
	admission := decodeAdmission(t, started)
	attempt, err := e.db.ClaimAttempt(ctx, admission.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); e.run(ctx, attempt) }()
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("real standalone fake worker did not produce output")
	}
	e.expireStopBudget(attempt.ID)
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || len(gate.ActiveClaims) != 1 {
		t.Fatal("actual watchdog did not retain claim", gate, err)
	}
	incidentID := *gate.IncidentID
	unblock()
	select {
	case <-done:
	case <-time.After(35 * time.Second):
		t.Fatal("real standalone worker did not finish")
	}
	settled, err := e.db.Attempt(ctx, attempt.ID)
	if err != nil || settled.Phase != "settled" || settled.Cleanup.State != "verified" || settled.EvidenceStatus != "complete" || settled.CandidateID == nil {
		t.Fatal("standalone author did not retain complete owned closure", settled, err)
	}
	sequence, seal, err := e.db.RecoveryStandaloneSealForAttempt(ctx, attempt.ID)
	if err != nil || sequence == "" || seal.AttemptID != attempt.ID || seal.Binding.CandidateID != *settled.CandidateID {
		t.Fatal("standalone closure seal unavailable", sequence, seal, err)
	}
	inventory, err := e.db.RecoveryEvidenceInventory(ctx, attempt.ID)
	if err != nil || inventory.CleanupEventSequence == "" {
		t.Fatal("owned cleanup and evidence inventory missing", inventory, err)
	}
	var member *domain.ArtifactRecord
	for i := range inventory.Artifacts {
		if inventory.Artifacts[i].Kind == "source_file" {
			member = &inventory.Artifacts[i]
			break
		}
	}
	if member == nil {
		t.Fatal("retained source member missing")
	}
	path := filepath.Join(e.lock.Root(), filepath.FromSlash(member.PrivateRef))
	originalMember, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(append([]byte{}, originalMember...), byte('!')), 0600); err != nil {
		t.Fatal(err)
	}
	view, err := e.db.RecoveryIncident(ctx, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	bad := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: view.Revision, Action: "verify_and_reconcile"}
	blocked := recoveryEngineResult(t, e.ReconcileIncident(ctx, incidentID, bad))
	if blocked.Incident.State != "unresolved" {
		t.Fatal("corrupt retained member cleared incident", blocked)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || len(gate.ActiveClaims) != 1 {
		t.Fatal("corrupt source released ownership", gate, err)
	}
	if err = os.WriteFile(path, originalMember, 0600); err != nil {
		t.Fatal(err)
	}
	good := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: blocked.Incident.Revision, Action: "verify_and_reconcile"}
	accepted := e.ReconcileIncident(ctx, incidentID, good)
	result := recoveryEngineResult(t, accepted)
	if result.Incident.State != "reconciled" {
		t.Fatal("actual standalone author closure did not reconcile", result)
	}
	gate, err = e.db.Gate(ctx)
	if err != nil || gate.State != "open" || len(gate.ActiveClaims) != 0 {
		t.Fatal("verified standalone recovery did not release exact claim", gate, err)
	}
	replay := e.ReconcileIncident(ctx, incidentID, good)
	again := recoveryEngineResult(t, replay)
	first, _ := json.Marshal(result)
	second, _ := json.Marshal(again)
	if string(first) != string(second) || accepted.Envelope.Receipt.RequestID != replay.Envelope.Receipt.RequestID {
		t.Fatal("standalone recovery replay changed decision", accepted, replay)
	}
	after, err := e.db.Attempt(ctx, attempt.ID)
	if err != nil || after.Revision != settled.Revision {
		t.Fatal("reconciliation replayed the actual attempt", after, settled, err)
	}
	if body, err := os.ReadFile(filepath.Join(repo, "docs", "note.md")); err != nil || string(body) != string(original) {
		t.Fatal("reconciliation changed the user's checkout", err)
	}
}
