package mcphost

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestFrameReaderValidation(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		limit       int
		want        error
	}{
		{"valid", "{\"jsonrpc\":\"2.0\",\"method\":\"ping\"}\n", 128, nil},
		{"nested duplicate", "{\"params\":{\"x\":1,\"x\":2}}\n", 128, ErrInvalidFrame},
		{"unknown envelope", "{\"unexpected\":1}\n", 128, ErrInvalidFrame},
		{"invalid utf8", "{\"a\":\"\xff\"}\n", 128, ErrInvalidFrame},
		{"batch", "[]\n", 128, ErrInvalidFrame},
		{"truncated", "{}", 128, ErrTruncatedFrame},
		{"over limit", "{\"id\":1}\n", 8, ErrFrameTooLarge},
		{"exact limit", "{\"id\":1}\n", 9, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newFrameReader(io.NopCloser(strings.NewReader(tc.input)), tc.limit)
			got, err := io.ReadAll(r)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error %v; want %v", err, tc.want)
			}
			if tc.want != nil && len(got) != 0 {
				t.Fatal("invalid frame was delivered")
			}
			if tc.want == nil && string(got) != tc.input {
				t.Fatalf("frame %q", got)
			}
		})
	}
}

func TestFrameReaderActualWireLimit(t *testing.T) {
	const prefix = `{"id":1}`
	frame := prefix + strings.Repeat(" ", MaxFrameBytes-len(prefix)-1) + "\n"
	r := newFrameReader(io.NopCloser(strings.NewReader(frame)), MaxFrameBytes)
	if n, err := io.Copy(io.Discard, r); err != nil || n != MaxFrameBytes {
		t.Fatalf("exact limit: %d %v", n, err)
	}
	r = newFrameReader(io.NopCloser(strings.NewReader(frame[:len(frame)-1]+" \n")), MaxFrameBytes)
	if n, err := io.Copy(io.Discard, r); n != 0 || !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("over limit: %d %v", n, err)
	}
}

func TestProtocolWriterConcurrentFrames(t *testing.T) {
	var out bufferCloser
	w := &protocolWriter{destination: &out}
	var group sync.WaitGroup
	failures := make(chan error, 100)
	for range 100 {
		group.Go(func() { _, err := w.Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n")); failures <- err })
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 100 {
		t.Fatalf("got %d output frames", len(lines))
	}
	for _, line := range lines {
		if !validFrame([]byte(line + "\n")) {
			t.Fatal("interleaved output")
		}
	}
}

type bufferCloser struct{ bytes.Buffer }

func (*bufferCloser) Close() error { return nil }

func TestProtocolWriterRedactsOnlyRPCError(t *testing.T) {
	var out bufferCloser
	w := &protocolWriter{destination: &out}
	result := "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"text\":\"keep\"}}\n"
	if _, err := w.Write([]byte(result)); err != nil {
		t.Fatal(err)
	}
	if out.String() != result {
		t.Fatal("result modified")
	}
	out.Reset()
	_, err := w.Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":42,\"error\":{\"code\":-32602,\"message\":\"private-input\",\"data\":\"private-input\"}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "private-input") || !strings.Contains(out.String(), "-32602") || !strings.Contains(out.String(), "42") {
		t.Fatalf("unsafe error: %s", out.String())
	}
}
