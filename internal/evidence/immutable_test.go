package evidence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImmutablePutPublishesOneCompleteRecordAfterDurableBytes(t *testing.T) {
	s, sink, _ := setup(t)
	body := bytes.Repeat([]byte("binary\x00\xff\n"), 100000)
	m, err := s.Put(context.Background(), attempt, "source_file", body)
	if err != nil {
		t.Fatal(err)
	}
	if sink.saves != 1 || !m.Closed || m.Completeness != "complete" || m.SHA256 == nil {
		t.Fatalf("Immutable publication: saves%d %+v", sink.saves, m)
	}
	var copied bytes.Buffer
	if _, err = s.CopyTo(context.Background(), m.ID, &copied, int64(len(body))); err != nil || !bytes.Equal(copied.Bytes(), body) {
		t.Fatalf("Incomplete publication: %v", err)
	}
}

func TestFailedImmutableRetainsBytesWithoutPublishingCompleteHash(t *testing.T) {
	s, sink, lock := setup(t)
	w, err := s.create(context.Background(), attempt, "source_file", 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write([]byte("retained partial bytes")); err != nil {
		t.Fatal(err)
	}
	w.fault = errors.New("injected write failure after a known prefix")
	m, err := w.Close(context.Background(), true)
	if err == nil || sink.saves != 1 || m.Completeness != "incomplete" || m.SHA256 != nil || m.ByteLength != "22" {
		t.Fatalf("Fault became complete: %+v %v", m, err)
	}
	data, readErr := os.ReadFile(filepath.Join(lock.Root(), filepath.FromSlash(m.RelativePath)))
	if readErr != nil || string(data) != "retained partial bytes" {
		t.Fatalf("Lost known bytes: %q %v", data, readErr)
	}
}
