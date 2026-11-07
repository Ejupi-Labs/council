package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/store"
)

// The child intentionally holds a real private owner lock and is killed without
// Close. No provider or credential data is read by this controlled subprocess.
func TestRecoveryOwnerEpochAbruptProcessLoss(t *testing.T) {
	if os.Getenv("COUNCIL_EPOCH_PROCESS_FIXTURE") == "1" {
		root := os.Getenv("COUNCIL_EPOCH_FIXTURE_ROOT")
		ready := os.Getenv("COUNCIL_EPOCH_FIXTURE_READY")
		lock, err := localstate.Acquire(root)
		if err != nil {
			t.Fatal(err)
		}
		db, err := store.Open(lock.DatabasePath())
		if err != nil {
			t.Fatal(err)
		}
		_, err = New(db, lock, Config{OwnerInstanceID: domain.NewID()})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(ready, []byte("ready\n"), 0600); err != nil {
			t.Fatal(err)
		}
		select {}
	}
	root := filepath.Join(t.TempDir(), "private")
	ready := filepath.Join(t.TempDir(), "ready")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestRecoveryOwnerEpochAbruptProcessLoss$")
	cmd.Env = append(os.Environ(), "COUNCIL_EPOCH_PROCESS_FIXTURE=1", "COUNCIL_EPOCH_FIXTURE_ROOT="+root, "COUNCIL_EPOCH_FIXTURE_READY="+ready)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	deadline := time.Now().Add(20 * time.Second)
	for {
		body, e := os.ReadFile(ready)
		if e == nil && string(body) == "ready\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned child did not publish its epoch-ready marker", e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	db, err := store.Open(lock.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	next, err := New(db, lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	gate, err := db.Gate(context.Background())
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatal("abrupt owner death did not retain unknown epoch fence", gate, err)
	}
	incident, err := db.Incident(context.Background(), *gate.IncidentID)
	if err != nil || incident.Code != "owner_epoch_uncertain" {
		t.Fatal(incident, err)
	}
}

func TestRecoveryOwnerEpochFailedFirstFencePersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	e.db.BeforeCommit = func() error { return errors.New("first incident could not be committed") }
	e.fence("", "first_unrecorded_cause", "The first fence observation did not persist.")
	e.db.BeforeCommit = nil
	e.control.Lock()
	uncertain := e.fenceUncertain
	e.control.Unlock()
	if !uncertain {
		t.Fatal("failed first fence did not retain in-memory uncertainty")
	}
	gate, err := e.db.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatal("fixture did not demonstrate an unpersisted first fence", gate, err)
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	gate, err = next.db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil {
		t.Fatal("dirty epoch did not fence the lost first cause after restart", gate, err)
	}
	incident, err := next.db.Incident(ctx, *gate.IncidentID)
	if err != nil || incident.Code != "owner_epoch_uncertain" {
		t.Fatal(incident, err)
	}
	if result := next.StartTask(ctx, domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "Must remain fenced", ExecutionMode: "fake"}); result.Envelope.Error == nil {
		t.Fatal("restart admitted work after a lost first fence", result)
	}
}

func TestRecoveryOwnerEpochDrainsRegisteredWatchdogsBeforeCleanClose(t *testing.T) {
	e, _, _ := testEngine(t)
	id := domain.NewID()
	e.watchStopWithLimit(id, domain.Now(), time.Hour)
	e.control.Lock()
	_, registered := e.stopWatches[id]
	e.control.Unlock()
	if !registered {
		t.Fatal("actual watchdog did not register before shutdown")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e.control.Lock()
	remaining := len(e.stopWatches)
	e.control.Unlock()
	if remaining != 0 {
		t.Fatal("owner closed before registered watchdog exited", remaining)
	}
	e.watchStopWithLimit(domain.NewID(), domain.Now(), time.Millisecond)
	e.control.Lock()
	remaining = len(e.stopWatches)
	e.control.Unlock()
	if remaining != 0 {
		t.Fatal("watchdog registered after the owner drain boundary", remaining)
	}
	next, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	gate, err := next.db.Gate(context.Background())
	if err != nil || gate.State != "open" {
		t.Fatal("verified watchdog drain did not permit clean handoff", gate, err)
	}
}
