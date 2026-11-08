package mcphost

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"math/big"
	"strconv"
	"strings"
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
	for len(r.frame) == 0 {
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
		if !validFrame(frame) {
			return r.fail(ErrInvalidFrame)
		}
		if r.initialization != nil && !r.initialization.admit(frame) {
			return r.fail(ErrInvalidFrame)
		}
		if !validToolParams(frame) {
			// The SDK discards unknown tool parameters. Route a valid request
			// envelope to its guaranteed-invalid empty tool name instead, so it
			// produces a normal, correlated Invalid Params response without ever
			// seeing or executing the rejected arguments. Invalid notifications
			// are consumed silently, as required by JSON-RPC.
			replacement, valid := rejectedToolParams(frame)
			if !valid {
				return r.fail(ErrInvalidFrame)
			}
			if replacement == nil {
				continue
			}
			frame = replacement
		}
		frame, err := encodeIncomingIDs(frame)
		if err != nil {
			return r.fail(ErrInvalidFrame)
		}
		r.frame = frame
	}
	n := copy(p, r.frame)
	r.frame = r.frame[n:]
	return n, nil
}

// rejectedToolParams preserves the exact correlation ID on a well-formed
// request and removes every untrusted parameter. Council's fixed catalog has
// no empty tool name, so the SDK rejects this synthetic call before dispatch.
// Invalid envelopes retain the existing fail-closed transport behavior.
func rejectedToolParams(frame []byte) ([]byte, bool) {
	var fields map[string]json.RawMessage
	var version string
	if json.Unmarshal(frame, &fields) != nil || json.Unmarshal(fields["jsonrpc"], &version) != nil || version != "2.0" {
		return nil, false
	}
	if _, ok := fields["result"]; ok {
		return nil, false
	}
	if _, ok := fields["error"]; ok {
		return nil, false
	}
	id, present := fields["id"]
	if !present {
		return nil, true
	}
	if _, ok := encodeWireID(id); !ok {
		return nil, false
	}
	result := make([]byte, 0, len(id)+90)
	result = append(result, `{"jsonrpc":"2.0","id":`...)
	result = append(result, id...)
	result = append(result, `,"method":"tools/call","params":{"name":"","arguments":{}}}`...)
	return append(result, '\n'), true
}

// The pinned SDK decodes incoming JSON numbers through float64 before using
// them as JSON-RPC IDs. Transform client IDs into disjoint tagged strings at
// the transport boundary, then restore response IDs in protocolWriter. This
// avoids precision loss and keeps a numeric ID distinct from a string ID,
// without retaining per-request state or changing Council's durable IDs.
const sdkNumberIDPrefix = "council:wire:number:"
const sdkStringIDPrefix = "council:wire:string:"

func encodeWireID(id json.RawMessage) (string, bool) {
	if jsontext.Value(id).Kind() == '"' {
		var value string
		if json.Unmarshal(id, &value) == nil {
			return sdkStringIDPrefix + value, true
		}
		return "", false
	}
	value, ok := canonicalInteger(string(id))
	if !ok {
		return "", false
	}
	return sdkNumberIDPrefix + value, true
}

// JSON permits multiple spellings for the same integer, including 1.0,
// 1e0 and -0. Canonicalizing them without float64 makes cancellation match
// the corresponding request and bounds expansion to the input frame limit.
func canonicalInteger(raw string) (string, bool) {
	if !json.Valid([]byte(raw)) || raw == "" {
		return "", false
	}
	negative := strings.HasPrefix(raw, "-")
	if negative {
		raw = raw[1:]
	}
	if len(raw) == 0 || raw[0] < '0' || raw[0] > '9' {
		return "", false
	}
	exponentText := "0"
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		exponentText = raw[i+1:]
		raw = raw[:i]
	}
	fraction := 0
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		fraction = len(raw) - i - 1
		raw = raw[:i] + raw[i+1:]
	}
	digits := strings.TrimLeft(raw, "0")
	if digits == "" {
		return "0", true
	}
	// Cap exponent digits before integer parsing. This bounds processing even
	// for tiny frames purporting to represent astronomically large numbers.
	negativeExponent := strings.HasPrefix(exponentText, "-")
	exponentText = strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(exponentText, "+"), "-"), "0")
	if exponentText == "" {
		exponentText = "0"
	} else if negativeExponent {
		exponentText = "-" + exponentText
	}
	if len(exponentText) > 20 {
		return "", false
	}
	exponent, ok := new(big.Int).SetString(exponentText, 10)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimRight(digits, "0")
	trailing := len(digits) - len(trimmed)
	digits = trimmed
	exponent.Sub(exponent, big.NewInt(int64(fraction)))
	exponent.Add(exponent, big.NewInt(int64(trailing)))
	if exponent.Sign() < 0 {
		return "", false
	}
	if exponent.Sign() != 0 {
		digits += "e" + exponent.String()
	}
	if negative {
		digits = "-" + digits
	}
	return digits, true
}

// Favor ordinary decimal integer output for interoperability with JSON
// clients that decode IDs directly into an integer type. Very large integral
// exponents stay compact to prevent a small request from amplifying output.
func renderCanonicalInteger(canonical string) string {
	i := strings.IndexByte(canonical, 'e')
	if i < 0 {
		return canonical
	}
	exponent, err := strconv.ParseInt(canonical[i+1:], 10, 64)
	if err != nil || exponent > (1<<20)-int64(i) {
		return canonical
	}
	return canonical[:i] + strings.Repeat("0", int(exponent))
}

func marshalIDString(value string) []byte {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	// Encoding a valid Go string cannot fail with this in-memory writer.
	_ = encoder.Encode(value)
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

func encodeIncomingIDs(frame []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(frame, &fields); err != nil {
		return nil, err
	}
	var method string
	rawMethod, present := fields["method"]
	if !present || jsontext.Value(rawMethod).Kind() != '"' || json.Unmarshal(rawMethod, &method) != nil {
		// A response to a server-initiated request is in the other JSON-RPC
		// direction and must retain the server's original correlation ID.
		return frame, nil
	}
	if id, present := fields["id"]; present {
		encoded, ok := encodeWireID(id)
		if !ok {
			return nil, ErrInvalidFrame
		}
		replacement := marshalIDString(encoded)
		var err error
		frame, err = replaceFrameValue(frame, []string{"id"}, replacement)
		if err != nil {
			return nil, err
		}
	}
	if method == "notifications/cancelled" {
		var params map[string]json.RawMessage
		if json.Unmarshal(fields["params"], &params) == nil && params != nil {
			if id, present := params["requestId"]; present {
				encoded, ok := encodeWireID(id)
				if !ok {
					return nil, ErrInvalidFrame
				}
				replacement := marshalIDString(encoded)
				var err error
				frame, err = replaceFrameValue(frame, []string{"params", "requestId"}, replacement)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return frame, nil
}

// Replace just the requested JSON value. All other original bytes, including
// the owner's nested input body, retain their exact spelling and whitespace.
func replaceFrameValue(frame []byte, path []string, replacement []byte) ([]byte, error) {
	start, end, ok := findJSONValue(frame, path)
	if !ok {
		return nil, ErrInvalidFrame
	}
	out := make([]byte, 0, len(frame)-(end-start)+len(replacement))
	out = append(out, frame[:start]...)
	out = append(out, replacement...)
	out = append(out, frame[end:]...)
	return out, nil
}

func findJSONValue(raw []byte, path []string) (int, int, bool) {
	if len(path) == 0 {
		return 0, len(raw), true
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.ReadToken()
	if err != nil || token.Kind() != '{' {
		return 0, 0, false
	}
	for decoder.PeekKind() != '}' {
		key, err := decoder.ReadToken()
		if err != nil || key.Kind() != '"' {
			return 0, 0, false
		}
		name := key.String()
		value, err := decoder.ReadValue()
		if err != nil {
			return 0, 0, false
		}
		end := int(decoder.InputOffset())
		start := end - len(value)
		if name == path[0] {
			innerStart, innerEnd, ok := findJSONValue(value, path[1:])
			return start + innerStart, start + innerEnd, ok
		}
	}
	return 0, 0, false
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
	encoded := p
	if _, method := frame["method"]; !method {
		_, hasResult := frame["result"]
		_, hasError := frame["error"]
		if hasResult || hasError {
			var value string
			if id, ok := frame["id"]; ok && json.Unmarshal(id, &value) == nil {
				switch {
				case strings.HasPrefix(value, sdkNumberIDPrefix):
					canonical, ok := canonicalInteger(strings.TrimPrefix(value, sdkNumberIDPrefix))
					if !ok {
						w.failure = ErrProtocolOutput
						return 0, ErrProtocolOutput
					}
					var err error
					encoded, err = replaceFrameValue(encoded, []string{"id"}, []byte(renderCanonicalInteger(canonical)))
					if err != nil {
						w.failure = ErrProtocolOutput
						return 0, ErrProtocolOutput
					}
				case strings.HasPrefix(value, sdkStringIDPrefix):
					id := marshalIDString(strings.TrimPrefix(value, sdkStringIDPrefix))
					var err error
					encoded, err = replaceFrameValue(encoded, []string{"id"}, id)
					if err != nil {
						w.failure = ErrProtocolOutput
						return 0, ErrProtocolOutput
					}
				}
			}
		}
	}
	if raw, ok := frame["error"]; ok {
		var e struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(raw, &e) != nil {
			w.failure = ErrProtocolOutput
			return 0, ErrProtocolOutput
		}
		redacted, _ := json.Marshal(struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{e.Code, "MCP request failed"})
		var err error
		encoded, err = replaceFrameValue(encoded, []string{"error"}, redacted)
		if err != nil {
			w.failure = ErrProtocolOutput
			return 0, ErrProtocolOutput
		}
	}
	if n, err := w.destination.Write(encoded); err != nil || n != len(encoded) {
		w.failure = ErrProtocolOutput
		return 0, ErrProtocolOutput
	}
	return len(p), nil
}

func (w *protocolWriter) Close() error { return w.destination.Close() }
func (w *protocolWriter) err() error   { w.mu.Lock(); defer w.mu.Unlock(); return w.failure }
