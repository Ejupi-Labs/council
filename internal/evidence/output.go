package evidence

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type outputReference struct{ attemptID, relative, identity string }

// OutputPath prepares a mutable native last-message target outside source and
// immutable artifacts. Only a target created by this store can be imported.
func (s *Store) OutputPath(ctx context.Context, attemptID string) (string, error) {
	if !validID(attemptID) {
		return "", errors.New("Invalid response attempt identity.")
	}
	if err := ctx.Err(); err != nil {
		return "", err
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
	d.Close()
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
	if err = localstate.ProtectPrivateFile(f, false); err != nil {
		f.Close()
		return "", err
	}
	identity, err := fileIdentity(f)
	if err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(s.lock.Root(), filepath.FromSlash(rel))
	s.outputMu.Lock()
	s.outputs[path] = outputReference{attemptID, rel, identity}
	s.outputMu.Unlock()
	return path, nil
}

func (s *Store) ImportResponse(ctx context.Context, attemptID, path string, maxBytes int64) (Metadata, error) {
	if maxBytes < 1 || maxBytes > StdoutLimit {
		return Metadata{}, errors.New("Invalid response byte limit.")
	}
	s.outputMu.Lock()
	ref, ok := s.outputs[path]
	s.outputMu.Unlock()
	if !ok || ref.attemptID != attemptID {
		return Metadata{}, ErrCorrupt
	}
	i, err := s.root.Lstat(ref.relative)
	if err != nil || !i.Mode().IsRegular() {
		return Metadata{}, ErrCorrupt
	}
	f, err := s.root.Open(ref.relative)
	if err != nil {
		return Metadata{}, ErrCorrupt
	}
	defer f.Close()
	if err = localstate.VerifyPrivateFile(f, false); err != nil {
		return Metadata{}, ErrCorrupt
	}
	identity, err := fileIdentity(f)
	if err != nil || identity != ref.identity {
		return Metadata{}, ErrCorrupt
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(i, opened) {
		return Metadata{}, ErrCorrupt
	}
	w, err := s.Create(ctx, attemptID, "final_response", maxBytes)
	if err != nil {
		return Metadata{}, err
	}
	_, copyErr := io.Copy(w, io.LimitReader(f, maxBytes+1))
	after, statErr := f.Stat()
	if statErr != nil || !os.SameFile(i, after) || i.Size() != after.Size() || !i.ModTime().Equal(after.ModTime()) {
		copyErr = ErrCorrupt
	}
	m, closeErr := w.Close(ctx, copyErr == nil)
	if copyErr != nil {
		return m, copyErr
	}
	return m, closeErr
}
