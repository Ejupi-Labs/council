package mcphost

import (
	"bufio"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"sync"
	"time"
)

// MaxFrameBytes includes the terminating newline.
const MaxFrameBytes = 64 << 20

var (
	ErrInvalidFrame   = errors.New("invalid MCP frame")
	ErrFrameTooLarge  = errors.New("MCP frame exceeds limit")
	ErrTruncatedFrame = errors.New("incomplete MCP frame")
	ErrProtocolOutput = errors.New("MCP output failed")
)

// frameReader validates the entire frame before the SDK can observe any of it.
// It never returns bytes from two frames in a single read.
type frameReader struct {
	source         io.ReadCloser
	reader         *bufio.Reader
	limit          int
	frame          []byte
	mu             sync.Mutex
	failure        error
	initialization *initializationGate
}

type initializationGate struct {
	ready     chan struct{}
	closed    chan struct{}
	once      sync.Once
	closeOnce sync.Once
	observed  bool // accessed only by the reader goroutine
}

func newInitializationGate() *initializationGate {
	return &initializationGate{ready: make(chan struct{}), closed: make(chan struct{})}
}

func (g *initializationGate) acknowledge() { g.once.Do(func() { close(g.ready) }) }
func (g *initializationGate) close()       { g.closeOnce.Do(func() { close(g.closed) }) }

func (g *initializationGate) admit(frame []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(frame, &fields) != nil {
		return false
	}
	var method string
	if raw, ok := fields["method"]; ok && json.Unmarshal(raw, &method) != nil {
		return false
	}
	switch method {
	case "", "initialize", "server/discover", "notifications/cancelled", "ping":
		return true
	case "notifications/initialized":
		if _, hasID := fields["id"]; !hasID {
			g.observed = true
		}
		return true
	default:
		if !g.observed {
			return false
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-g.ready:
			return true
		case <-g.closed:
			return false
		case <-timer.C:
			return false
		}
	}
}

func newFrameReader(source io.ReadCloser, limit int) *frameReader {
	return &frameReader{source: source, reader: bufio.NewReader(source), limit: limit}
}

func (r *frameReader) fail(err error) (int, error) {
	r.mu.Lock()
	r.failure = err
	r.mu.Unlock()
	return 0, err
}

func (r *frameReader) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failure
}

func (r *frameReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.frame) == 0 {
		var frame []byte
		for {
			part, err := r.reader.ReadSlice('\n')
			if len(part) > r.limit-len(frame) {
				return r.fail(ErrFrameTooLarge)
			}
			frame = append(frame, part...)
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil {
				if errors.Is(err, io.EOF) && len(frame) == 0 {
					return 0, io.EOF
				}
				if errors.Is(err, io.EOF) {
					return r.fail(ErrTruncatedFrame)
				}
				return r.fail(ErrInvalidFrame)
			}
			break
		}
		if !validFrame(frame) || !validToolParams(frame) {
			return r.fail(ErrInvalidFrame)
		}
		if r.initialization != nil && !r.initialization.admit(frame) {
			return r.fail(ErrInvalidFrame)
		}
		r.frame = frame
	}
	n := copy(p, r.frame)
	r.frame = r.frame[n:]
	return n, nil
}

// Validate the 2025 tool-method envelope before the SDK can discard unknown
// parameters. Protocol metadata is an extensible object, not tool authority.
func validToolParams(frame []byte) bool {
	var outer map[string]json.RawMessage
	if json.Unmarshal(frame, &outer) != nil {
		return false
	}
	var method string
	if raw, ok := outer["method"]; !ok || json.Unmarshal(raw, &method) != nil {
		return true
	}
	if method != "tools/call" && method != "tools/list" {
		return true
	}
	raw, present := outer["params"]
	if !present {
		return method == "tools/list"
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(raw, &params) != nil || params == nil {
		return false
	}
	for key, value := range params {
		switch key {
		case "_meta":
			var meta map[string]json.RawMessage
			if json.Unmarshal(value, &meta) != nil || meta == nil {
				return false
			}
		case "name":
			var name string
			if method != "tools/call" || jsontext.Value(value).Kind() != '"' || json.Unmarshal(value, &name) != nil || name == "" {
				return false
			}
		case "arguments":
			var args map[string]json.RawMessage
			if method != "tools/call" || json.Unmarshal(value, &args) != nil || args == nil {
				return false
			}
		case "cursor":
			var cursor string
			if method != "tools/list" || jsontext.Value(value).Kind() != '"' || json.Unmarshal(value, &cursor) != nil {
				return false
			}
		default:
			return false
		}
	}
	if method == "tools/call" {
		_, ok := params["name"]
		return ok
	}
	return true
}

func (r *frameReader) Close() error {
	if r.initialization != nil {
		r.initialization.close()
	}
	return r.source.Close()
}

func validFrame(frame []byte) bool {
	if len(frame) == 0 || frame[len(frame)-1] != '\n' || !jsontext.Value(frame).IsValid() {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(frame, &fields); err != nil || fields == nil {
		return false
	}
	for key := range fields {
		switch key {
		case "jsonrpc", "id", "method", "params", "result", "error":
		default:
			return false
		}
	}
	return true
}

// protocolWriter redacts SDK-level JSON-RPC errors, whose messages may contain
// raw untrusted input. Normal result envelopes are passed through unchanged.
type protocolWriter struct {
	destination io.WriteCloser
	mu          sync.Mutex
	failure     error
}

func (w *protocolWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var frame map[string]json.RawMessage
	if !validFrame(p) || json.Unmarshal(p, &frame) != nil {
		w.failure = ErrProtocolOutput
		return 0, ErrProtocolOutput
	}
	if raw, ok := frame["error"]; ok {
		var e struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(raw, &e) != nil {
			w.failure = ErrProtocolOutput
			return 0, ErrProtocolOutput
		}
		frame["error"], _ = json.Marshal(struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{e.Code, "MCP request failed"})
		encoded, err := json.Marshal(frame)
		if err != nil {
			w.failure = ErrProtocolOutput
			return 0, ErrProtocolOutput
		}
		encoded = append(encoded, '\n')
		if n, err := w.destination.Write(encoded); err != nil || n != len(encoded) {
			w.failure = ErrProtocolOutput
			return 0, ErrProtocolOutput
		}
		return len(p), nil
	}
	if n, err := w.destination.Write(p); err != nil || n != len(p) {
		w.failure = ErrProtocolOutput
		return 0, ErrProtocolOutput
	}
	return len(p), nil
}

func (w *protocolWriter) Close() error { return w.destination.Close() }
func (w *protocolWriter) err() error   { w.mu.Lock(); defer w.mu.Unlock(); return w.failure }
