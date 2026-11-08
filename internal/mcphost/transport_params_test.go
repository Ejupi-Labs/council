package mcphost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type rawParamsOwnerSpy struct{ calls atomic.Int32 }

func (s *rawParamsOwnerSpy) Call(_ context.Context, req ownerclient.Request) domain.Result {
	s.calls.Add(1)
	return domain.Success(200, domain.NewID(), map[string]any{"operation": req.Operation}, nil)
}

type rawBodyOwnerSpy struct{ bodies chan string }

func (s *rawBodyOwnerSpy) Call(_ context.Context, req ownerclient.Request) domain.Result {
	s.bodies <- string(req.Body)
	return domain.Success(200, domain.NewID(), map[string]any{"operation": req.Operation}, nil)
}

func TestRawSDKPreservesExactOwnerBodyWithLargeID(t *testing.T) {
	spy := &rawBodyOwnerSpy{bodies: make(chan string, 1)}
	server, err := NewHost(spy)
	if err != nil {
		t.Fatal(err)
	}
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, connection, connection) }()
	reader := bufio.NewReader(client)
	write := func(frame string) {
		t.Helper()
		if _, err := client.Write([]byte(frame + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []byte {
		t.Helper()
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
	read()
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	body := `{ "requestId" : "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "expectedRevision" : "1", "mode" : "auto\u006eomous" }`
	write(`{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/call","params":{"name":"council_policy_set","arguments":{"input":` + body + `}}}`)
	line := read()
	var response struct {
		ID    json.RawMessage `json:"id"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(line, &response) != nil || string(response.ID) != "9007199254740993" || len(response.Error) != 0 {
		t.Fatalf("large-ID call failed: %s", line)
	}
	select {
	case got := <-spy.bodies:
		if got != body {
			t.Fatalf("owner body changed: %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("owner not called")
	}
	write(`{"jsonrpc":"2.0","id":18446744073709551617,"method":"ping"}`)
	line = read()
	if json.Unmarshal(line, &response) != nil || string(response.ID) != "18446744073709551617" {
		t.Fatalf("integer ID above int64 changed: %s", line)
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not close")
	}
}

func TestWireIDCanonicalizationAndDirection(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		ok        bool
	}{
		{"9007199254740993", "9007199254740993", true},
		{"1000", "1e3", true}, {"1e3", "1e3", true}, {"1000.0", "1e3", true},
		{"-0", "0", true}, {"0e999999999999999999999999", "0", true},
		{"1e67108863", "1e67108863", true}, {"1.5", "", false},
		{"1e999999999999999999999999", "", false},
	} {
		got, ok := canonicalInteger(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s => %q, %v; want %q, %v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
	if got := renderCanonicalInteger("1e67108863"); got != "1e67108863" {
		t.Fatal("enormous exponent was expanded")
	}
	response := []byte(`{"jsonrpc":"2.0","id":"council:wire:number:123","result": { "value": 1 }}` + "\n")
	if got, err := encodeIncomingIDs(response); err != nil || !bytes.Equal(got, response) {
		t.Fatal("server-originated request response was rewritten")
	}
	request := []byte(`{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/call","params": { "_meta": { "progressToken" : 9007199254740993 }, "arguments" : { "input" : { "requestId" : "literal", "mode" : "auto\u006eomous" } } }}` + "\n")
	wantBody := `"params": { "_meta": { "progressToken" : 9007199254740993 }, "arguments" : { "input" : { "requestId" : "literal", "mode" : "auto\u006eomous" } } }`
	got, err := encodeIncomingIDs(request)
	if err != nil || !strings.Contains(string(got), wantBody) {
		t.Fatal("unrelated parameter bytes or progress token changed")
	}
	var out bufferCloser
	w := &protocolWriter{destination: &out}
	serverRequest := `{"jsonrpc":"2.0","id":"council:wire:number:123","method":"sampling/createMessage","params":{}}` + "\n"
	if _, err := w.Write([]byte(serverRequest)); err != nil || out.String() != serverRequest {
		t.Fatal("server-originated request was rewritten")
	}
}

func TestRawSDKPreservesLargeIDAndCancellationIsolation(t *testing.T) {
	started := make(chan string, 5)
	cancelled := make(chan string, 5)
	release := make(chan struct{})
	server := NewServer(func(s *mcp.Server) {
		s.AddTool(&mcp.Tool{Name: "wait-id", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var input struct {
				Slot string `json:"slot"`
			}
			if json.Unmarshal(req.Params.Arguments, &input) != nil {
				return nil, context.Canceled
			}
			started <- input.Slot
			select {
			case <-ctx.Done():
				cancelled <- input.Slot
				return nil, ctx.Err()
			case <-release:
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: input.Slot}}}, nil
			}
		})
	})
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, connection, connection) }()
	reader := bufio.NewReader(client)
	write := func(frame string) {
		t.Helper()
		if _, err := client.Write([]byte(frame + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []byte {
		t.Helper()
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
	read()
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for _, tc := range []struct{ id, slot string }{
		{`9007199254740992`, "adjacent"}, {`9007199254740993`, "target"}, {`"council:wire:number:9007199254740993"`, "string"},
		{`1e3`, "normalized"}, {`"wire\u002dtest"`, "escaped"},
	} {
		write(`{"jsonrpc":"2.0","id":` + tc.id + `,"method":"tools/call","params":{"name":"wait-id","arguments":{"slot":"` + tc.slot + `"}}}`)
		select {
		case got := <-started:
			if got != tc.slot {
				t.Fatalf("started %q", got)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("handler did not start")
		}
	}
	write(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":9007199254740993}}`)
	select {
	case got := <-cancelled:
		if got != "target" {
			t.Fatalf("cancelled %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("target was not cancelled")
	}
	select {
	case got := <-cancelled:
		t.Fatalf("wrong request cancelled: %s", got)
	case <-time.After(100 * time.Millisecond):
	}
	for _, tc := range []struct{ id, slot string }{{`1000.0`, "normalized"}, {`"wire-test"`, "escaped"}} {
		write(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":` + tc.id + `}}`)
		select {
		case got := <-cancelled:
			if got != tc.slot {
				t.Fatalf("cancelled %q, want %q", got, tc.slot)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s cancellation did not match", tc.slot)
		}
	}
	close(release)
	seen := map[string]bool{}
	for len(seen) < 2 {
		line := read()
		var result struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(line, &result) != nil {
			t.Fatal("invalid response")
		}
		if len(result.Result) != 0 {
			seen[string(result.ID)] = true
		}
	}
	if !seen[`9007199254740992`] || !seen[`"council:wire:number:9007199254740993"`] {
		t.Fatalf("IDs changed or collided: %v", seen)
	}
	write(`{"jsonrpc":"2.0","id":"council:wire:number:123","method":""}`)
	for {
		line := read()
		var response struct {
			ID    json.RawMessage `json:"id"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &response) != nil {
			t.Fatalf("invalid response: %s", line)
		}
		id := string(response.ID)
		if id == `"council:wire:number:123"` {
			break
		}
		if (id != `9007199254740993` && id != `1000` && id != `"wire-test"`) || len(response.Error) == 0 {
			t.Fatalf("unexpected response while waiting for empty-method ID: %s", line)
		}
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not close")
	}
}

func TestRawToolParametersRejectUnknownBeforeOwner(t *testing.T) {
	for _, tc := range []struct {
		name, params string
		method       string
	}{
		{"call unknown", `{"name":"council_policy_get","arguments":{},"unexpected":true}`, "tools/call"},
		{"call request state", `{"name":"council_policy_get","arguments":{},"requestState":"x"}`, "tools/call"},
		{"call input responses", `{"name":"council_policy_get","arguments":{},"inputResponses":{}}`, "tools/call"},
		{"call null arguments", `{"name":"council_policy_get","arguments":null}`, "tools/call"},
		{"call null name", `{"name":null,"arguments":{}}`, "tools/call"},
		{"call missing name", `{"arguments":{}}`, "tools/call"},
		{"call null metadata", `{"name":"council_policy_get","_meta":null}`, "tools/call"},
		{"list unknown", `{"unexpected":true}`, "tools/list"},
		{"list null cursor", `{"cursor":null}`, "tools/list"},
		{"list numeric cursor", `{"cursor":3}`, "tools/list"},
		{"list name", `{"name":"council_policy_get"}`, "tools/list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := &rawParamsOwnerSpy{}
			server, err := NewHost(spy)
			if err != nil {
				t.Fatal(err)
			}
			client, connection := net.Pipe()
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(10 * time.Second))
			done := make(chan error, 1)
			go func() { done <- Serve(context.Background(), server, connection, connection) }()
			reader := bufio.NewReader(client)
			init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}` + "\n"
			if _, err := client.Write([]byte(init)); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadBytes('\n'); err != nil {
				t.Fatal(err)
			}
			// Send the rejected request, an equally invalid notification and a
			// ping in one stream. Responses may be concurrent and unordered.
			frame := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
				`{"jsonrpc":"2.0","id":"invalid-2","method":"` + tc.method + `","params":` + tc.params + `}` + "\n" +
				`{"jsonrpc":"2.0","method":"` + tc.method + `","params":` + tc.params + `}` + "\n" +
				`{"jsonrpc":"2.0","id":3,"method":"ping"}` + "\n"
			go func() { _, _ = client.Write([]byte(frame)) }()
			seen := map[string]bool{}
			for range 2 {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					t.Fatalf("session did not survive invalid params: %v", err)
				}
				var response struct {
					ID     json.RawMessage `json:"id"`
					Result json.RawMessage `json:"result"`
					Error  *struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if json.Unmarshal(line, &response) != nil {
					t.Fatal("invalid protocol response")
				}
				switch string(response.ID) {
				case `"invalid-2"`:
					if response.Error == nil || response.Error.Code != -32602 || response.Error.Message != "MCP request failed" || len(response.Result) != 0 {
						t.Fatalf("invalid params did not produce a sanitized correlated error: %s", line)
					}
				case `3`:
					if response.Error != nil || len(response.Result) == 0 {
						t.Fatalf("subsequent ping failed: %s", line)
					}
				default:
					t.Fatalf("unexpected response to notification or foreign ID: %s", line)
				}
				if seen[string(response.ID)] {
					t.Fatal("duplicate response ID")
				}
				seen[string(response.ID)] = true
			}
			if spy.calls.Load() != 0 {
				t.Fatal("invalid params reached the owner")
			}
			_ = client.Close()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("server did not close")
			}
		})
	}
}

func TestRawToolParamsAllowExtensibleMetadataAndValidCalls(t *testing.T) {
	spy := &rawParamsOwnerSpy{}
	server, err := NewHost(spy)
	if err != nil {
		t.Fatal(err)
	}
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, connection, connection) }()
	reader := bufio.NewReader(client)
	write := func(frame string) {
		t.Helper()
		if _, err := client.Write([]byte(frame + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]json.RawMessage {
		t.Helper()
		frame, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]json.RawMessage
		if json.Unmarshal(frame, &result) != nil {
			t.Fatal("invalid protocol frame")
		}
		return result
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{"experimental":{"fixture":{}}},"clientInfo":{"name":"fixture","version":"1","extension":"allowed"},"extension":"allowed"}}`)
	if _, ok := read()["result"]; !ok {
		t.Fatal("initialize with extensions failed")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	write(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"fixture":{"nested":null}}}}`)
	if _, ok := read()["result"]; !ok {
		t.Fatal("valid list with metadata failed")
	}
	write(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"council_policy_get","arguments":{},"_meta":{"fixture":{"nested":null}}}}`)
	if _, ok := read()["result"]; !ok {
		t.Fatal("valid call with metadata failed")
	}
	if spy.calls.Load() != 1 {
		t.Fatal("valid call did not reach owner exactly once")
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not close")
	}
}

func TestRejectedToolParamsPreservesCorrelationAndRejectsInvalidEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		name, frame, wantID string
		valid, notification bool
	}{
		{"numeric", `{"jsonrpc":"2.0","id":123,"method":"tools/list","params":{"bad":true}}`, `123`, true, false},
		{"string", `{"jsonrpc":"2.0","id":"a\\\"b","method":"tools/list","params":{"bad":true}}`, `"a\\\"b"`, true, false},
		{"escaped version", `{"jsonrpc":"2\u002e0","id":1,"method":"tools/list","params":{"bad":true}}`, `1`, true, false},
		{"notification", `{"jsonrpc":"2.0","method":"tools/call","params":{"bad":true}}`, ``, true, true},
		{"null ID", `{"jsonrpc":"2.0","id":null,"method":"tools/list","params":{"bad":true}}`, ``, false, false},
		{"fractional ID", `{"jsonrpc":"2.0","id":1.5,"method":"tools/list","params":{"bad":true}}`, ``, false, false},
		{"invalid version", `{"jsonrpc":"1.0","id":1,"method":"tools/list","params":{"bad":true}}`, ``, false, false},
		{"response member", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"bad":true},"result":{}}`, ``, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := rejectedToolParams([]byte(tc.frame))
			if valid != tc.valid {
				t.Fatalf("valid=%v, want %v", valid, tc.valid)
			}
			if !valid || tc.notification {
				if got != nil {
					t.Fatal("invalid or notification frame was forwarded")
				}
				return
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(got, &fields) != nil || string(fields["id"]) != tc.wantID {
				t.Fatalf("correlation not preserved: %s", got)
			}
			var params struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(fields["params"], &params) != nil || params.Name != "" {
				t.Fatal("rejected arguments were not replaced with an invalid tool")
			}
		})
	}
	for _, item := range ToolCatalog() {
		if item.Name == "" {
			t.Fatal("synthetic rejection name must not be a registered tool")
		}
	}
}
