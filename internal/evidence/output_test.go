package evidence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMutableResponseImportChecksOwnedIdentityAndBounds(t *testing.T) {
	s, _, _ := setup(t)
	ctx := context.Background()
	path, e := s.OutputPath(ctx, attempt)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("complete🙂"), 0600); e != nil {
		t.Fatal(e)
	}
	m, e := s.ImportResponse(ctx, attempt, path, 100)
	if e != nil || m.Completeness != "complete" {
		t.Fatalf("import: %+v %v", m, e)
	}
	if _, e = s.ImportResponse(ctx, attempt, filepath.Join(t.TempDir(), "arbitrary"), 100); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("arbitrary path: %v", e)
	}
	path, e = s.OutputPath(ctx, attempt)
	if e != nil {
		t.Fatal(e)
	}
	os.Remove(path)
	os.WriteFile(path, []byte("replacement"), 0600)
	if _, e = s.ImportResponse(ctx, attempt, path, 100); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("replaced staging target: %v", e)
	}
	path, e = s.OutputPath(ctx, attempt)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("too long"), 0600)
	m, e = s.ImportResponse(ctx, attempt, path, 3)
	if !errors.Is(e, ErrLimit) || m.Completeness != "incomplete" || m.ByteLength != "3" {
		t.Fatalf("oversized import: %+v %v", m, e)
	}
}

func TestFailedFsyncNeverPublishesTailAndExactChunkBoundary(t *testing.T) {
	s, _, lock := setup(t)
	ctx := context.Background()
	raw := bytes.Repeat([]byte{0xff, 0x00, 0xf0, 0x9f, 0x99, 0x82}, ChunkLimit/3)
	m, e := s.Put(ctx, attempt, "stdout", raw)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.Read(ctx, m.ID, ReadOptions{})
	if e != nil || c.NextOffset != "65536" || c.EOF {
		t.Fatalf("boundary %+v %v", c, e)
	}
	if e = os.Remove(filepath.Join(lock.Root(), filepath.FromSlash(m.RelativePath))); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(ctx, m.ID, ReadOptions{}); !errors.Is(e, ErrCorrupt) {
		t.Fatalf("missing artifact: %v", e)
	}
	w, e := s.Create(ctx, attempt, "stderr", 100)
	if e != nil {
		t.Fatal(e)
	}
	w.Write([]byte("unflushed"))
	w.file.Close()
	if e = w.Checkpoint(ctx); e == nil {
		t.Fatal("closed file fsync succeeded")
	}
	c, e = s.Read(ctx, w.ID(), ReadOptions{})
	if e != nil || c.Through != "0" {
		t.Fatalf("published failed flush %+v %v", c, e)
	}
	w.Close(ctx, false)
}
