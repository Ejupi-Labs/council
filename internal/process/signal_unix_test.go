//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestUnixSignalErrorsRequireReapAndEmptyGroupProof(t *testing.T) {
	for _, tc := range []struct {
		name         string
		probeError   error
		denials      int
		persistent   bool
		wantVerified bool
	}{
		{name: "zombie delivery errors then observed absence", wantVerified: true},
		{name: "temporary EPERM then actual absence", probeError: unix.EPERM, denials: 3, wantVerified: true},
		{name: "temporary EACCES then actual absence", probeError: unix.EACCES, denials: 3, wantVerified: true},
		{name: "persistent EPERM stays uncertain at deadline", probeError: unix.EPERM, persistent: true},
		{name: "persistent EACCES stays uncertain at deadline", probeError: unix.EACCES, persistent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := Prepare(context.Background(), selfSpec(t, "argv"))
			if err != nil {
				t.Fatal(err)
			}
			defer o.Close()
			if err := o.Start(); err != nil {
				t.Fatal(err)
			}
			before := waitRoot(t, o)
			if !before.Known || before.ExitCode != 0 {
				t.Fatal(before)
			}
			n := o.native.(*unixOwner)
			// Kill this retained, unreaped child only. Darwin's group API may
			// report EPERM while the resulting zombie still anchors the PGID.
			if err := n.command.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var signals []unix.Signal
			probes := 0
			postReapSignal := false
			probeBeforeReap := false
			n.signalGroup = func(pgid int, signal unix.Signal) error {
				mu.Lock()
				defer mu.Unlock()
				signals = append(signals, signal)
				if signal != 0 {
					postReapSignal = postReapSignal || n.reaped // n.mu is held for delivery.
					return unix.EPERM
				}
				n.mu.Lock()
				probeBeforeReap = probeBeforeReap || !n.reaped
				n.mu.Unlock()
				probes++
				if tc.probeError != nil && (tc.persistent || probes <= tc.denials) {
					return tc.probeError
				}
				return unix.Kill(pgid, 0) // Actual absence remains mandatory.
			}
			ctx, cancel := context.WithTimeout(context.Background(), GracePeriod+time.Second)
			defer cancel()
			result := o.Cleanup(ctx)
			// A caller deadline can win the observation select just before the
			// retained cleanup result is published. Join that bounded collector
			// before inspecting its final diagnostic category.
			select {
			case <-o.cleanupDone:
				result = o.cleanupSnapshot()
			case <-time.After(time.Second):
				t.Fatal("bounded cleanup did not publish its final observation")
			}
			if result.Verified != tc.wantVerified || !strings.Contains(result.Detail, "TERM=EPERM, KILL=EPERM") {
				t.Fatalf("signal delivery was confused with scoped absence proof: %+v", result)
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.probeError != nil && probes <= tc.denials {
				t.Fatalf("temporary denial was not retried through actual absence: probes=%d", probes)
			}
			if tc.persistent && (probes < 2 || !strings.Contains(result.Detail, "Last group probe: "+signalStatus(tc.probeError))) {
				t.Fatalf("persistent permission denial was not retained at the original deadline: probes=%d result=%+v", probes, result)
			}
			if postReapSignal || probeBeforeReap || len(signals) < 3 || signals[0] != unix.SIGTERM || signals[1] != unix.SIGKILL {
				t.Fatalf("cleanup violated final-attempt/reap/probe ordering: signals=%v postReapSignal=%v probeBeforeReap=%v", signals, postReapSignal, probeBeforeReap)
			}
			after, _ := o.RootOutcome()
			if !after.Known || after.ExitCode != before.ExitCode || after.ObservedAt != before.ObservedAt {
				t.Fatalf("cleanup delivery error replaced the retained root result: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestUnixSignalErrorsCannotSkipGuardianReap(t *testing.T) {
	o, err := Prepare(context.Background(), selfSpec(t, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	waitRoot(t, o)
	n := o.native.(*unixOwner)
	var mu sync.Mutex
	var signals []unix.Signal
	n.signalGroup = func(_ int, signal unix.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		signals = append(signals, signal)
		if signal == 0 {
			return unix.ESRCH
		}
		return unix.EPERM // Do not actually kill the still-living guardian.
	}
	ctx, cancel := context.WithTimeout(context.Background(), GracePeriod+100*time.Millisecond)
	defer cancel()
	if result := o.Cleanup(ctx); result.Verified {
		t.Fatalf("unobserved guardian exit became verified cleanup: %+v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, signal := range signals {
		if signal == 0 {
			t.Fatal("group absence was probed before retained guardian exit")
		}
	}
}

func TestUnixSignalStatusDoesNotExposeArbitraryErrors(t *testing.T) {
	if got := signalStatus(errors.New("disposable private path must not appear")); got != "other_error" {
		t.Fatalf("native cleanup disclosed arbitrary error text: %q", got)
	}
}
