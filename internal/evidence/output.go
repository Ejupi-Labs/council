package evidence

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type outputReference struct {
	attemptID, relative, identity string
	reservation                   *os.File
}

// OutputPath prepares a mutable native last-message target outside source and
// immutable artifacts. Keep its physical file open until import or release, so
// an unlinked target's inode/file ID cannot be recycled into a valid replacement.
// The caller must defer ReleaseOutput after a successful reservation. Only a
// target created by this store can be imported; imports consume the reservation.
func (s *Store) OutputPath(ctx context.Context, attemptID string) (path string, resultErr error) {
	if !validID(attemptID) {
		return "", errors.New("Invalid response attempt identity.")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.outputMu.Lock()
	closed := s.outputsClosed
	s.outputMu.Unlock()
	if closed {
		return "", ErrClosed
	}
	if err := s.lock.CheckHeld(); err != nil {
		return "", err
	}
	if err := s.root.Mkdir("response-staging", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	i, err := s.root.Lstat("response-staging")
	if err != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return "", ErrCorrupt
	}
	d, err := s.root.Open("response-staging")
	if err != nil {
		return "", err
	}
	err = localstate.ProtectPrivateFile(d, true)
	err = errors.Join(err, wrapFault(d.Close()))
	if err != nil {
		return "", err
	}
	id, err := NewID()
	if err != nil {
		return "", err
	}
	rel := "response-staging/" + id + ".txt"
	f, err := s.root.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	transferred := false
	defer func() {
		if !transferred {
			if closeErr := f.Close(); closeErr != nil {
				resultErr = errors.Join(resultErr, wrapFault(closeErr))
			}
		}
	}()
	if err = localstate.ProtectPrivateFile(f, false); err != nil {
		return "", err
	}
	identity, err := fileIdentity(f)
	if err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", wrapFault(err)
	}
	path = filepath.Join(s.lock.Root(), filepath.FromSlash(rel))
	s.outputMu.Lock()
	if s.outputsClosed {
		s.outputMu.Unlock()
		return "", ErrClosed
	}
	s.outputs[path] = outputReference{attemptID: attemptID, relative: rel, identity: identity, reservation: f}
	transferred = true
	s.outputMu.Unlock()
	return path, nil
}

// ReleaseOutput releases one owned staging reservation without deleting bytes.
// It is safe to defer this immediately after OutputPath; an import already
// consumed its reservation, so that deferred release becomes a no-op.
func (s *Store) ReleaseOutput(attemptID, path string) error {
	s.outputMu.Lock()
	ref, ok := s.outputs[path]
	if ok && ref.attemptID == attemptID {
		delete(s.outputs, path)
	}
	s.outputMu.Unlock()
	if !ok {
		return nil
	}
	if ref.attemptID != attemptID {
		return ErrCorrupt
	}
	return wrapFault(ref.reservation.Close())
}

func (s *Store) closeOutputs() error {
	s.outputMu.Lock()
	s.outputsClosed = true
	refs := s.outputs
	s.outputs = map[string]outputReference{}
	s.outputMu.Unlock()
	var result error
	for _, ref := range refs {
		result = errors.Join(result, wrapFault(ref.reservation.Close()))
	}
	return result
}

func (s *Store) ImportResponse(ctx context.Context, attemptID, path string, maxBytes int64) (metadata Metadata, resultErr error) {
	s.outputMu.Lock()
	ref, ok := s.outputs[path]
	if ok && ref.attemptID == attemptID {
		delete(s.outputs, path)
	}
	s.outputMu.Unlock()
	if !ok || ref.attemptID != attemptID {
		return Metadata{}, ErrCorrupt
	}
	defer func() {
		if closeErr := ref.reservation.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, wrapFault(closeErr))
		}
	}()
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	if maxBytes < 1 || maxBytes > StdoutLimit {
		return Metadata{}, errors.New("Invalid response byte limit.")
	}
	reserved, err := ref.reservation.Stat()
	if err != nil || !reserved.Mode().IsRegular() {
		return Metadata{}, ErrCorrupt
	}
	i, err := s.root.Lstat(ref.relative)
	if err != nil || !i.Mode().IsRegular() || !os.SameFile(reserved, i) {
		return Metadata{}, ErrCorrupt
	}
	f, err := s.root.Open(ref.relative)
	if err != nil {
		return Metadata{}, ErrCorrupt
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, wrapFault(closeErr))
		}
	}()
	if err = localstate.VerifyPrivateFile(f, false); err != nil {
		return Metadata{}, ErrCorrupt
	}
	identity, err := fileIdentity(f)
	if err != nil || identity != ref.identity {
		return Metadata{}, ErrCorrupt
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(reserved, opened) || !os.SameFile(i, opened) {
		return Metadata{}, ErrCorrupt
	}
	w, err := s.Create(ctx, attemptID, "final_response", maxBytes)
	if err != nil {
		return Metadata{}, err
	}
	_, copyErr := io.Copy(w, io.LimitReader(f, maxBytes+1))
	after, statErr := f.Stat()
	final, finalErr := s.root.Lstat(ref.relative)
	if finalErr != nil || !final.Mode().IsRegular() || !os.SameFile(reserved, final) || statErr != nil || !os.SameFile(i, after) || i.Size() != after.Size() || !i.ModTime().Equal(after.ModTime()) {
		copyErr = errors.Join(copyErr, ErrCorrupt)
	}
	m, closeErr := w.Close(ctx, copyErr == nil)
	return m, errors.Join(copyErr, closeErr)
}
