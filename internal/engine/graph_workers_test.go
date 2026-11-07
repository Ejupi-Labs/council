package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Worker ownership is per actual attempt. Stopping one must not lose the other
// cancellation handle; a later owner-wide fence must still reach both handles.
func TestActiveWorkerOwnershipCancelsExactAttemptAndRetainsSibling(t *testing.T) {
	first, second := domain.NewID(), domain.NewID()
	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
	t.Cleanup(firstCancel)
	t.Cleanup(secondCancel)
	e := &Engine{activeWorkers: map[string]context.CancelFunc{first: firstCancel, second: secondCancel}}
	e.control.Lock()
	if !e.cancelWorkerLocked(first) || e.cancelWorkerLocked(domain.NewID()) {
		t.Fatal("Cancellation did not select the exact owned attempt")
	}
	if firstCtx.Err() == nil || secondCtx.Err() != nil || len(e.activeWorkers) != 2 {
		t.Fatal("Stop lost ownership or cancelled the independent sibling")
	}
	e.cancelWorkersLocked()
	if secondCtx.Err() == nil || len(e.activeWorkers) != 2 {
		t.Fatal("Owner-wide cancellation lost a worker or fabricated settlement")
	}
	delete(e.activeWorkers, first)
	if _, retained := e.activeWorkers[second]; !retained {
		t.Fatal("First settlement erased the sibling's ownership")
	}
	e.control.Unlock()
}

func TestActiveWorkerCancellationHandlesEmptyOwner(t *testing.T) {
	e := &Engine{}
	e.control.Lock()
	defer e.control.Unlock()
	if e.cancelWorkerLocked(domain.NewID()) {
		t.Fatal("An unowned attempt was reported as cancelled")
	}
	e.cancelWorkersLocked()
}
