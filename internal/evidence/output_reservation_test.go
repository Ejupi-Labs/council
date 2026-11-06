package evidence

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestOutputReservationConsumedOnImportOutcomes(t *testing.T) {
	for _, name := range []string{"complete", "missing", "invalid bound", "cancelled", "limit", "metadata failure"} {
		t.Run(name, func(t *testing.T) {
			s, sink, _ := setup(t)
			ctx := context.Background()
			path, err := s.OutputPath(ctx, attempt)
			if err != nil {
				t.Fatal(err)
			}
			held := s.outputs[path].reservation
			if held == nil {
				t.Fatal("staging target has no held physical reservation")
			}
			if err = os.WriteFile(path, []byte("complete bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			limit := int64(100)
			switch name {
			case "missing":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "invalid bound":
				limit = 0
			case "limit":
				limit = 3
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "metadata failure":
				sink.fail = true
			}
			_, err = s.ImportResponse(ctx, attempt, path, limit)
			if name == "complete" && err != nil {
				t.Fatal(err)
			}
			if name != "complete" && err == nil {
				t.Fatalf("%s import succeeded", name)
			}
			if name == "limit" && (!errors.Is(err, ErrLimit) || errors.Is(err, ErrDurability)) {
				t.Fatalf("limit classification: %v", err)
			}
			if name == "cancelled" && (err != context.Canceled || errors.Is(err, ErrDurability)) {
				t.Fatalf("cancellation classification: %v", err)
			}
			if held.Fd() != ^uintptr(0) {
				t.Fatalf("reservation handle leaked after %s", name)
			}
			if len(s.outputs) != 0 {
				t.Fatalf("reservation remained registered after %s", name)
			}
			if err = s.ReleaseOutput(attempt, path); err != nil {
				t.Fatalf("deferred release after import: %v", err)
			}
		})
	}
}

func TestOutputReservationReleaseAndStoreShutdown(t *testing.T) {
	s, _, _ := setup(t)
	ctx := context.Background()
	path, err := s.OutputPath(ctx, attempt)
	if err != nil {
		t.Fatal(err)
	}
	held := s.outputs[path].reservation
	other := "abcdefabcdefabcdefabcdefabcdefab"
	if err = s.ReleaseOutput(other, path); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("cross-attempt release: %v", err)
	}
	if _, err = s.ImportResponse(ctx, other, path, 100); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("cross-attempt import: %v", err)
	}
	if _, err = held.Stat(); err != nil {
		t.Fatalf("another attempt consumed a live reservation: %v", err)
	}
	if err = s.ReleaseOutput(attempt, path); err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseOutput(attempt, path); err != nil {
		t.Fatalf("release is not idempotent: %v", err)
	}
	if held.Fd() != ^uintptr(0) {
		t.Fatal("release leaked reservation handle")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("release deleted retained bytes: %v", err)
	}
	var handles []*os.File
	for range 16 {
		path, err = s.OutputPath(ctx, attempt)
		if err != nil {
			t.Fatal(err)
		}
		handles = append(handles, s.outputs[path].reservation)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if len(s.outputs) != 0 {
		t.Fatal("shutdown kept registered reservations")
	}
	for _, f := range handles {
		if f.Fd() != ^uintptr(0) {
			t.Fatal("shutdown leaked reservation handle")
		}
	}
	if _, err = s.OutputPath(ctx, attempt); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed store accepted new reservation: %v", err)
	}
}

func TestOutputReservationCloseFaultIsNotHidden(t *testing.T) {
	for _, operation := range []string{"import", "release", "shutdown"} {
		t.Run(operation, func(t *testing.T) {
			s, _, _ := setup(t)
			path, err := s.OutputPath(context.Background(), attempt)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.outputs[path].reservation.Close(); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "import":
				_, err = s.ImportResponse(context.Background(), attempt, path, 100)
			case "release":
				err = s.ReleaseOutput(attempt, path)
			case "shutdown":
				err = s.Close()
			}
			if !errors.Is(err, ErrDurability) || !errors.Is(err, os.ErrClosed) {
				t.Fatalf("%s hid close uncertainty: %v", operation, err)
			}
			if len(s.outputs) != 0 {
				t.Fatal("failed close kept a live reservation entry")
			}
		})
	}
}
