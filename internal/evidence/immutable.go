package evidence

import (
	"context"
	"errors"
	"strconv"
)

// A failed immutable write retains what can still be proved. It does not promote
// unflushed bytes or rerun the writer. No complete hash is published on this path.
func (w *Stream) retainFailedImmutable(ctx context.Context, prior error) (Metadata, error) {
	m := w.metadata
	m.Completeness = "uncertain"
	syncErr := w.file.Sync()
	if syncErr == nil {
		m.Closed = true
		m.ByteLength = strconv.FormatInt(w.written, 10)
		m.Completeness = "incomplete"
	}
	closeErr := w.file.Close()
	if closeErr != nil {
		m.Closed = false
		m.ByteLength = w.metadata.ByteLength
		m.Completeness = "uncertain"
	}
	saveErr := w.store.sink.SaveArtifact(ctx, m)
	if saveErr == nil {
		w.metadata = m
	}
	w.fault = errors.Join(prior, wrapFault(syncErr), wrapFault(closeErr), wrapFault(saveErr))
	return w.metadata, w.fault
}
