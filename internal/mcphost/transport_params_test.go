package mcphost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

type rawParamsOwnerSpy struct{ calls atomic.Int32 }

func (s *rawParamsOwnerSpy) Call(_ context.Context, req ownerclient.Request) domain.Result {
	s.calls.Add(1)
	return domain.Success(200, domain.NewID(), map[string]any{"operation": req.Operation}, nil)
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
			frame := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"` + tc.method + `","params":` + tc.params + `}` + "\n"
			go func() { _, _ = client.Write([]byte(frame)) }()
			select {
			case err := <-done:
				if !errors.Is(err, ErrInvalidFrame) {
					t.Fatalf("expected rejected frame: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("invalid params did not close")
			}
			if spy.calls.Load() != 0 {
				t.Fatal("invalid params reached the owner")
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
