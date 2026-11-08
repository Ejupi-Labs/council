package mcphost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
)

type dispatchFixture struct{ calls atomic.Int32 }

func (f *dispatchFixture) Call(_ context.Context, req ownerclient.Request) domain.Result {
	f.calls.Add(1)
	if req.Operation != "meta" {
		panic("unexpected fixture operation")
	}
	return domain.Success(200, "0123456789abcdef0123456789abcdef", jsontext.Value(`{"large":9007199254740993,"decimal":1.234567890123456789}`), nil)
}

func TestDispatchValidatesBeforeOwnerAndPreservesResultNumbers(t *testing.T) {
	fixture := &dispatchFixture{}
	server, err := NewHost(fixture)
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
		if err := json.Unmarshal(frame, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
	if _, ok := read()["result"]; !ok {
		t.Fatal("initialization failed")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	write(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"council_owner_meta","arguments":{}}}`)
	result := read()
	if !bytes.Contains(result["result"], []byte(`9007199254740993`)) || !bytes.Contains(result["result"], []byte(`1.234567890123456789`)) {
		t.Fatalf("owner numbers changed: %s", result["result"])
	}
	var call struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(result["result"], &call); err != nil || len(call.Content) != 1 {
		t.Fatal("invalid tool result")
	}
	if !json.Valid(call.Structured) || !bytes.Equal([]byte(call.Content[0].Text), call.Structured) {
		t.Fatal("text and structured result differ")
	}
	write(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"council_tasks_start","arguments":{"input":{"requestId":"0123456789abcdef0123456789abcdef","unknown":true}}}}`)
	result = read()
	if !strings.Contains(string(result["result"]), `"isError":true`) || fixture.calls.Load() != 1 {
		t.Fatal("invalid mutation reached owner")
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("host did not exit")
	}
}
