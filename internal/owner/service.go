// Package owner bootstraps a continuing exact-binary owner without task or token argv.
package owner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

const InternalArg = "__council_owner_v1"

type Verify func(context.Context, localstate.Descriptor) bool

// Ensure converges competing clients on the OS lock winner. It never opens SQLite,
// takes over a PID, deletes a lock or replaces an incompatible live owner.
func Ensure(ctx context.Context, root string, verify Verify) (localstate.Descriptor, error) {
	return ensureWithin(ctx, root, verify, 10*time.Second)
}

func ensureWithin(ctx context.Context, root string, verify Verify, timeout time.Duration) (localstate.Descriptor, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return localstate.Descriptor{}, err
	}
	if verify == nil {
		return localstate.Descriptor{}, errors.New("Owner verification is required.")
	}
	probe := func() (localstate.Descriptor, bool) {
		d, e := localstate.ReadDescriptor(root)
		return d, e == nil && verify(ctx, d)
	}
	if d, ok := probe(); ok {
		return d, nil
	}
	lock, err := localstate.Acquire(root)
	if err != nil && !errors.Is(err, localstate.ErrLocked) {
		return localstate.Descriptor{}, err
	}
	if err == nil {
		if e := lock.Close(); e != nil {
			return localstate.Descriptor{}, e
		}
		executable, e := os.Executable()
		if e != nil {
			return localstate.Descriptor{}, e
		}
		command := exec.Command(executable, InternalArg)
		configureBackground(command)
		if e = command.Start(); e != nil {
			return localstate.Descriptor{}, errors.New("Cannot launch the continuing Council owner.")
		}
		go func() { _ = command.Wait() }()
	}
	// A startup owner may hold the lock before it publishes a descriptor. Bounded
	// readiness observation gives that owner time; it never bypasses its lock.
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return localstate.Descriptor{}, ctx.Err()
		case <-tick.C:
			if d, ok := probe(); ok {
				return d, nil
			}
		}
	}
}
