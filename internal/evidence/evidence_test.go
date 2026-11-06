package evidence

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

type memorySink struct {
	mu      sync.Mutex
	records map[string]Metadata
	fail    bool
	saves   int
}

func (s *memorySink) SaveArtifact(_ context.Context, m Metadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.fail {
		return errors.New("injected commit failure")
	}
	s.records[m.ID] = m
	return nil
}
func (s *memorySink) LoadArtifact(_ context.Context, id string) (Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.records[id]
	if !ok {
		return m, os.ErrNotExist
	}
	return m, nil
}
func setup(t *testing.T) (*Store, *memorySink, *localstate.Lock) {
	t.Helper()
	lock, e := localstate.Acquire(filepath.Join(t.TempDir(), "private"))
	if e != nil {
		t.Fatal(e)
	}
	sink := &memorySink{records: map[string]Metadata{}}
	s, e := New(lock, sink)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close(); lock.Close() })
	return s, sink, lock
}

const attempt = "1234567890abcdef1234567890abcdef"

func TestExactByteSnapshotAndImmutableHash(t *testing.T) {
	s, _, _ := setup(t)
	ctx := context.Background()
	w, e := s.Create(ctx, attempt, "stdout", 1000)
	if e != nil {
		t.Fatal(e)
	}
	raw := []byte("ä🙂\x00\xff\n")
	if _, e = w.Write(raw); e != nil {
		t.Fatal(e)
	}
	c, e := s.Read(ctx, w.ID(), ReadOptions{})
	if e != nil || c.Through != "0" {
		t.Fatalf("uncommitted prefix: %+v %v", c, e)
	}
	if e = w.Checkpoint(ctx); e != nil {
		t.Fatal(e)
	}
	c, e = s.Read(ctx, w.ID(), ReadOptions{Limit: 2})
	if e != nil {
		t.Fatal(e)
	}
	bound := c.Through
	var got []byte
	for {
		b, _ := base64.StdEncoding.DecodeString(c.BytesBase64)
		got = append(got, b...)
		if c.EOF {
			break
		}
		c, e = s.Read(ctx, w.ID(), ReadOptions{Offset: c.NextOffset, Through: &bound, Limit: 2})
		if e != nil {
			t.Fatal(e)
		}
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("bytes changed: %x != %x", got, raw)
	}
	if _, e = w.Write([]byte("tail")); e != nil {
		t.Fatal(e)
	}
	m, e := w.Close(ctx, true)
	if e != nil || m.SHA256 == nil {
		t.Fatalf("close: %+v %v", m, e)
	}
	c, e = s.Read(ctx, w.ID(), ReadOptions{Through: &bound})
	if e != nil || !c.EOF || c.Through != bound || !c.StreamClosed {
		t.Fatalf("fixed bound: %+v %v", c, e)
	}
	if _, e = s.VerifyClosed(ctx, m.ID); e != nil {
		t.Fatal(e)
	}
}

func TestCorruptionRedirectionAndRange(t *testing.T) {
	s, sink, lock := setup(t)
	ctx := context.Background()
	m, e := s.Put(ctx, attempt, "final_response", []byte("original"))
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range []ReadOptions{{Offset: "-1"}, {Offset: "01"}, {Offset: "9223372036854775808"}, {Offset: "9"}, {Limit: ChunkLimit + 1}} {
		if _, e = s.Read(ctx, m.ID, o); e == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
	path := filepath.Join(lock.Root(), filepath.FromSlash(m.RelativePath))
	if e = os.WriteFile(path, []byte("modified"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.VerifyClosed(ctx, m.ID); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("modified evidence: %v", e)
	}
	m.RelativePath = "../arbitrary"
	sink.SaveArtifact(ctx, m)
	if _, e = s.Read(ctx, m.ID, ReadOptions{}); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("redirected metadata: %v", e)
	}
}

func TestLimitAndFailedCheckpointRetainKnownPrefix(t *testing.T) {
	s, sink, _ := setup(t)
	ctx := context.Background()
	w, e := s.Create(ctx, attempt, "stderr", 5)
	if e != nil {
		t.Fatal(e)
	}
	n, e := w.Write([]byte("abcdef"))
	if n != 5 || !errors.Is(e, ErrLimit) {
		t.Fatalf("limit %d %v", n, e)
	}
	m, e := w.Close(ctx, true)
	if !errors.Is(e, ErrLimit) || m.Completeness != "incomplete" || m.ByteLength != "5" || m.SHA256 != nil {
		t.Fatalf("limit close: %+v %v", m, e)
	}
	w, e = s.Create(ctx, attempt, "stdout", 100)
	if e != nil {
		t.Fatal(e)
	}
	w.Write([]byte("durable"))
	if e = w.Checkpoint(ctx); e != nil {
		t.Fatal(e)
	}
	w.Write([]byte("orphan"))
	sink.fail = true
	if e = w.Checkpoint(ctx); e == nil {
		t.Fatal("injected failure accepted")
	}
	sink.fail = false
	c, e := s.Read(ctx, w.ID(), ReadOptions{})
	if e != nil || c.Through != "7" {
		t.Fatalf("overclaimed prefix: %+v %v", c, e)
	}
	if _, e = w.Write([]byte("more")); e == nil {
		t.Fatal("faulted writer continued")
	}
	if _, e = w.Close(ctx, false); e == nil {
		t.Fatal("faulted close succeeded")
	}
}

func TestLineBoundAndCoalescing(t *testing.T) {
	s, sink, _ := setup(t)
	ctx := context.Background()
	w, e := s.Create(ctx, attempt, "stdout", StdoutLimit)
	if e != nil {
		t.Fatal(e)
	}
	for range 100 {
		if _, e = w.Write([]byte("small\n")); e != nil {
			t.Fatal(e)
		}
	}
	if sink.saves != 1 {
		t.Fatalf("metadata per token: %d", sink.saves)
	}
	if _, e = w.Write(bytes.Repeat([]byte("x"), LineLimit+1)); !errors.Is(e, ErrLimit) {
		t.Fatalf("line bound: %v", e)
	}
	m, _ := w.Close(ctx, false)
	if !m.Closed || m.Completeness != "incomplete" {
		t.Fatalf("line metadata %+v", m)
	}
}
