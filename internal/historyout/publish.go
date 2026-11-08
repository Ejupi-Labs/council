// Package historyout publishes one bounded generated metadata document under
// an already provisioned Council-private root. It never provisions the root.
package historyout

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

const MaxPayload = 4 << 20

var (
	ErrUnsupported = errors.New("history output safety cannot be established")
	ErrConflict    = errors.New("history output target or root changed")
	ErrInvalid     = errors.New("invalid history output")
	namePattern    = regexp.MustCompile(`^` + regexp.QuoteMeta(localstate.HistoryOutputPrefix) + `[A-Za-z0-9][A-Za-z0-9_-]{0,50}\.json$`)
)

// RootIdentity is controller-private durable state, never client proof. Both
// identities must be checked again under held native handles for every effect.
type RootIdentity struct {
	StateRoot  string `json:"stateRoot"`
	ExportRoot string `json:"exportRoot"`
}

// Preview is controller-created, persisted privately and never decoded from an
// external caller. Name and root identity are revalidated before the permit.
type Preview struct {
	Name string       `json:"name"`
	Root RootIdentity `json:"root"`
}

type Result struct {
	PermitAccepted  bool
	CreateAttempted bool
	Created         bool
	BytesWritten    int
	FileFlushed     bool
	NameVerified    bool
	DirectorySync   string // "synced", "unavailable", or "unknown"
	FileIdentity    string // private owner observation, not a public receipt
}

type Root struct {
	mu       sync.Mutex
	lock     *localstate.Lock
	native   nativeRoot
	identity RootIdentity
	closed   bool
	// Test-only fault barrier; production leaves this nil.
	testHook func(string) error
}

func ValidFilename(name string) bool {
	if !namePattern.MatchString(name) {
		return false
	}
	base := strings.ToUpper(strings.TrimSuffix(name, ".json"))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$":
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

// Open validates the already-provisioned v2 private root. No caller can use
// this method to create or modify a path.
func Open(lock *localstate.Lock) (*Root, error) {
	if lock == nil || lock.CheckHeld() != nil {
		return nil, ErrUnsupported
	}
	native, identity, err := openNativeRoot(lock.Root())
	if err != nil {
		return nil, err
	}
	if err = lock.CheckHeld(); err != nil {
		_ = native.close()
		return nil, ErrUnsupported
	}
	return &Root{lock: lock, native: native, identity: identity}, nil
}

func (r *Root) Directory() string { return r.lock.Root() }

// SetTestHook installs the internal fixture barrier for a held root. Production
// callers leave it nil; it never supplies authority or bypasses native checks.
func (r *Root) SetTestHook(hook func(string) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.testHook = hook
}

func (r *Root) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.native.close()
}
func (r *Root) revalidate(expected RootIdentity) error {
	if r.closed || r.lock.CheckHeld() != nil || expected != r.identity {
		return ErrConflict
	}
	actual, err := r.native.identity()
	if err != nil {
		return err
	}
	if actual != expected {
		return ErrConflict
	}
	return nil
}

// Preview has no output side effect. Its Root value must be retained privately
// with the request and checked again after reopening an owner.
func (r *Root) Preview(name string) (Preview, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ValidFilename(name) {
		return Preview{}, ErrInvalid
	}
	if err := r.revalidate(r.identity); err != nil {
		return Preview{}, err
	}
	if err := r.native.requireAbsent(name); err != nil {
		return Preview{}, err
	}
	return Preview{Name: name, Root: r.identity}, nil
}

// Publish invokes permit exactly once immediately before the exclusive create.
// The engine callback must return nil only after positively acknowledging the
// exact durable single-publication permit. Any callback error prevents create.
// Once create is attempted the caller must persist the returned observations; an
// interrupted, unacknowledged or partial outcome must never be retried or removed.
func (r *Root) Publish(ctx context.Context, p Preview, payload []byte, permit func(context.Context) error) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := Result{DirectorySync: "unknown"}
	if !ValidFilename(p.Name) || len(payload) == 0 || len(payload) > MaxPayload || permit == nil {
		return result, ErrInvalid
	}
	if err := r.revalidate(p.Root); err != nil {
		return result, err
	}
	if err := r.native.requireAbsent(p.Name); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := permit(ctx); err != nil {
		return result, err
	}
	result.PermitAccepted = true
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := r.revalidate(p.Root); err != nil {
		return result, err
	}
	result.CreateAttempted = true
	result, err := r.native.publish(ctx, p.Name, payload, result, r.testHook)
	if err != nil {
		return result, fmt.Errorf("history publication could not be verified: %w", err)
	}
	return result, nil
}
