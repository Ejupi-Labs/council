package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"sync"
)

// Stream serializes writes and checkpoints. Checkpoint coalescing is byte based;
// callers checkpoint at timer/control/fault boundaries to expose quiet prefixes.
type Stream struct {
	mu            sync.Mutex
	store         *Store
	file          *os.File
	metadata      Metadata
	maxBytes      int64
	written       int64
	durable       int64
	fault         error
	closed        bool
	lineBytes     int
	deferMetadata bool // Immutable Put publishes its one durable closed record.
}

func (w *Stream) ID() string { return w.metadata.ID }

func (w *Stream) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, ErrClosed
	}
	if w.fault != nil {
		return 0, w.fault
	}
	limitFault := false
	input := p
	if int64(len(input)) > w.maxBytes-w.written {
		input = input[:w.maxBytes-w.written]
		limitFault = true
	}
	n, err := w.file.Write(input)
	w.written += int64(n)
	if w.metadata.Kind == "stdout" {
		for _, b := range input[:n] {
			if b == '\n' {
				w.lineBytes = 0
			} else {
				w.lineBytes++
				if w.lineBytes > LineLimit {
					limitFault = true
				}
			}
		}
	}
	if err != nil {
		w.fault = wrapFault(err)
		return n, w.fault
	}
	if limitFault {
		w.metadata.Completeness = "incomplete"
		if err = w.checkpoint(context.Background()); err != nil {
			return n, err
		}
		w.fault = ErrLimit
		return n, ErrLimit
	}
	if !w.deferMetadata && w.written-w.durable >= 256<<10 {
		if err = w.checkpoint(context.Background()); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (w *Stream) checkpoint(ctx context.Context) error {
	if err := w.file.Sync(); err != nil {
		w.fault = wrapFault(err)
		return w.fault
	}
	m := w.metadata
	m.ByteLength = strconv.FormatInt(w.written, 10)
	if err := w.store.sink.SaveArtifact(ctx, m); err != nil {
		w.fault = wrapFault(err)
		return w.fault
	}
	w.metadata = m
	w.durable = w.written
	return nil
}

func (w *Stream) Checkpoint(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrClosed
	}
	if w.fault != nil {
		return w.fault
	}
	return w.checkpoint(ctx)
}

func (w *Stream) Close(ctx context.Context, complete bool) (Metadata, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.metadata, w.fault
	}
	w.closed = true
	prior := w.fault
	if prior != nil && prior != ErrLimit {
		if w.deferMetadata {
			return w.retainFailedImmutable(ctx, prior)
		}
		w.file.Close()
		return w.metadata, prior
	}
	if err := w.file.Sync(); err != nil {
		if w.deferMetadata {
			return w.retainFailedImmutable(ctx, wrapFault(err))
		}
		w.file.Close()
		w.fault = wrapFault(err)
		return w.metadata, w.fault
	}
	m := w.metadata
	m.Closed = true
	m.ByteLength = strconv.FormatInt(w.written, 10)
	m.Completeness = "incomplete"
	if complete && prior == nil {
		if _, err := w.file.Seek(0, 0); err != nil {
			if w.deferMetadata {
				return w.retainFailedImmutable(ctx, wrapFault(err))
			}
			w.file.Close()
			w.fault = wrapFault(err)
			return w.metadata, w.fault
		}
		h := sha256.New()
		if _, err := io.Copy(h, w.file); err != nil {
			if w.deferMetadata {
				return w.retainFailedImmutable(ctx, wrapFault(err))
			}
			w.file.Close()
			w.fault = wrapFault(err)
			return w.metadata, w.fault
		}
		sum := hex.EncodeToString(h.Sum(nil))
		m.SHA256 = &sum
		m.Completeness = "complete"
	}
	if err := w.file.Close(); err != nil {
		w.fault = wrapFault(err)
		return w.metadata, w.fault
	}
	if err := w.store.sink.SaveArtifact(ctx, m); err != nil {
		w.fault = wrapFault(err)
		return w.metadata, w.fault
	}
	w.metadata = m
	w.durable = w.written
	return m, prior
}
