package evidence

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type immutableAcknowledgementSink struct {
	*memorySink
	afterCommit error
	beforeSave  func()
	calls       int
}

func (s *immutableAcknowledgementSink) SaveArtifact(ctx context.Context, m Metadata) error {
	s.calls++
	if s.beforeSave != nil {
		s.beforeSave()
	}
	// Cancellation here is known to precede the metadata transaction. It must
	// remain distinguishable from a committed record whose acknowledgement is lost.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.memorySink.SaveArtifact(ctx, m); err != nil {
		return err
	}
	return s.afterCommit
}

func TestImmutableFinalPublicationLostAcknowledgementDoesNotRetry(t *testing.T) {
	for _, withCancellation := range []bool{false, true} {
		name := "lost_acknowledgement"
		if withCancellation {
			name = "lost_acknowledgement_and_cancellation"
		}
		t.Run(name, func(t *testing.T) {
			s, memory, _ := setup(t)
			uncertain := domain.Fail("write_uncertain", "The final metadata committed but its acknowledgement was lost.")
			var ackErr error = uncertain
			if withCancellation {
				ackErr = errors.Join(uncertain, context.Canceled)
			}
			sink := &immutableAcknowledgementSink{memorySink: memory, afterCommit: ackErr}
			s.sink = sink
			body := bytes.Repeat([]byte("Exact café 東京\r\n\x00\xff"), 20000)
			w, err := s.create(context.Background(), attempt, "source_file", int64(len(body)), false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Write(body); err != nil {
				t.Fatal(err)
			}
			if sink.calls != 0 {
				t.Fatal("Immutable bytes published intermediate metadata")
			}
			sink.beforeSave = func() {
				if w.file.Fd() != ^uintptr(0) {
					t.Fatal("Final metadata was published before the physical writer closed")
				}
			}
			returned, err := w.Close(context.Background(), true)
			var problem *domain.Error
			if !errors.As(err, &problem) || problem.Code != "write_uncertain" || !errors.Is(err, ErrDurability) {
				t.Fatalf("Committed publication lost its uncertainty: %v", err)
			}
			if returned.Closed || returned.Completeness == "complete" || returned.SHA256 != nil {
				t.Fatal("An unacknowledged publication was returned as acknowledged complete metadata")
			}
			retained, err := memory.LoadArtifact(context.Background(), w.ID())
			if err != nil || !retained.Closed || retained.Completeness != "complete" || retained.SHA256 == nil {
				t.Fatalf("Committed closed metadata was lost: %+v %v", retained, err)
			}
			if sink.calls != 1 || memory.saves != 1 {
				t.Fatal("A lost acknowledgement replayed or downgraded publication")
			}
			if _, err = w.Close(context.Background(), false); !errors.As(err, &problem) || problem.Code != "write_uncertain" {
				t.Fatal("Repeated Close erased publication uncertainty")
			}
			if _, err = w.Write([]byte("must not replay")); !errors.Is(err, ErrClosed) {
				t.Fatal("The completed writer accepted replayed bytes")
			}
			if sink.calls != 1 || memory.saves != 1 {
				t.Fatal("Repeated cleanup published metadata again")
			}
			var copied bytes.Buffer
			if _, err = s.CopyTo(context.Background(), retained.ID, &copied, int64(len(body))); err != nil || !bytes.Equal(copied.Bytes(), body) {
				t.Fatalf("Committed physical bytes or complete hash changed: %v", err)
			}
		})
	}
}

func TestImmutableKnownPrecommitCancellationDoesNotBecomeDurabilityFault(t *testing.T) {
	s, memory, _ := setup(t)
	sink := &immutableAcknowledgementSink{memorySink: memory}
	s.sink = sink
	w, err := s.create(context.Background(), attempt, "source_file", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(w, "known complete physical bytes"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = w.Close(ctx, true)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrDurability) {
		t.Fatalf("Known precommit cancellation became durability uncertainty: %v", err)
	}
	if memory.saves != 0 || len(memory.records) != 0 {
		t.Fatal("Cancelled publication committed metadata")
	}
	if _, err = w.Close(context.Background(), true); !errors.Is(err, context.Canceled) {
		t.Fatal("Cleanup silently retried a cancelled immutable publication")
	}
	if sink.calls != 1 || memory.saves != 0 {
		t.Fatal("Cancelled publication was replayed")
	}
}

func TestImmutableFileFaultSurvivesCancelledPublication(t *testing.T) {
	s, memory, _ := setup(t)
	sink := &immutableAcknowledgementSink{memorySink: memory}
	s.sink = sink
	w, err := s.create(context.Background(), attempt, "source_file", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(w, "known bytes before the file fault"); err != nil {
		t.Fatal(err)
	}
	// Lose the actual native descriptor before its durability boundary. The
	// adjacent expected publication cancellation cannot erase that file fault.
	if err = w.file.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m, err := w.Close(ctx, true)
	if !errors.Is(err, ErrDurability) || !errors.Is(err, context.Canceled) || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Cancellation concealed a genuine native file fault: %v", err)
	}
	if m.Closed || m.Completeness == "complete" || m.SHA256 != nil || memory.saves != 0 {
		t.Fatal("Uncertain physical bytes became complete evidence")
	}
	if _, err = w.Close(context.Background(), true); !errors.Is(err, ErrDurability) {
		t.Fatal("Repeated cleanup erased the native file fault")
	}
	if sink.calls != 1 {
		t.Fatal("Failed publication was replayed")
	}
}
