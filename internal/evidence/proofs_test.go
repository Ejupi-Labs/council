package evidence

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCachedRangeIntegrityAndFreshWholeProof(t *testing.T) {
	s, _, lock := setup(t)
	ctx := context.Background()
	body := bytes.Repeat([]byte("01234567"), 3*ChunkLimit/8)
	m, err := s.Put(ctx, attempt, "final_response", body)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Read(ctx, m.ID, ReadOptions{Offset: "0", Limit: 8})
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(first.BytesBase64)
	if !bytes.Equal(decoded, body[:8]) {
		t.Fatal("Changed returned bytes")
	}
	file, err := os.OpenFile(filepath.Join(lock.Root(), filepath.FromSlash(m.RelativePath)), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteAt([]byte("corrupt!"), 2*ChunkLimit); err != nil {
		t.Fatal(err)
	}
	file.Close()
	// An unchanged requested range still has its expected bytes. This does not
	// assert whole-file freshness; fresh acceptance verification must detect damage.
	if _, err = s.Read(ctx, m.ID, ReadOptions{Limit: 8}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, m.ID, ReadOptions{Offset: "131072", Limit: 8}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Changed cached chunk accepted: %v", err)
	}
	if _, err = s.VerifyClosed(ctx, m.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Fresh whole proof used cache: %v", err)
	}
	var copy bytes.Buffer
	if _, err = s.CopyTo(ctx, m.ID, &copy, StdoutLimit); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Full copy accepted corruption: %v", err)
	}
}

func TestLinearCopyChecksBytesActuallyReturned(t *testing.T) {
	s, _, _ := setup(t)
	ctx := context.Background()
	body := bytes.Repeat([]byte("linear copy\x00\xff"), 650000)
	m, err := s.Put(ctx, attempt, "source_file", body)
	if err != nil {
		t.Fatal(err)
	}
	var copied bytes.Buffer
	if _, err = s.CopyTo(ctx, m.ID, &copied, int64(len(body))); err != nil || !bytes.Equal(copied.Bytes(), body) {
		t.Fatalf("Linear copy changed bytes: %v", err)
	}
	if _, err = s.CopyTo(ctx, m.ID, io.Discard, int64(len(body)-1)); !errors.Is(err, ErrCorrupt) {
		t.Fatal("Copy ignored explicit byte budget")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.VerifyClosed(cancelled, m.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Cancelled verification was misreported as corruption: %v", err)
	}
	if _, err = s.CopyTo(cancelled, m.ID, io.Discard, StdoutLimit); !errors.Is(err, context.Canceled) {
		t.Fatalf("Copy ignored cancellation: %v", err)
	}
}
