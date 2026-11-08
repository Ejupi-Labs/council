package mcphost

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestUnknownMethodAndToolAreSanitizedProtocolErrors(t *testing.T) {
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), NewServer(nil), connection, connection) }()
	reader := bufio.NewReader(client)
	write := func(frame string) {
		t.Helper()
		if _, err := client.Write([]byte(frame + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []byte {
		t.Helper()
		frame, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		if !validFrame(frame) {
			t.Fatal("stdout contained an invalid protocol frame")
		}
		return frame
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
	var initialization struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(read(), &initialization) != nil || len(initialization.Result) == 0 {
		t.Fatal("initialize failed")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for _, tc := range []struct {
		frame    string
		id, code int
	}{
		{`{"jsonrpc":"2.0","id":2,"method":"private_unknown_method","params":{}}`, 2, -32601},
		{`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"private_unknown_tool","arguments":{}}}`, 3, -32602},
	} {
		write(tc.frame)
		frame := read()
		var response struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Error   *struct {
				Code    int             `json:"code"`
				Message string          `json:"message"`
				Data    json.RawMessage `json:"data"`
			} `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(frame, &response); err != nil || response.JSONRPC != "2.0" || response.ID != tc.id || response.Error == nil || response.Error.Code != tc.code || response.Error.Message != "MCP request failed" || len(response.Error.Data) != 0 || len(response.Result) != 0 {
			t.Fatalf("invalid or unsanitized JSON-RPC error for id %d", tc.id)
		}
		if strings.Contains(string(frame), "private_unknown") {
			t.Fatal("untrusted request content leaked into error")
		}
	}
	// Notifications never acquire a fabricated response ID.
	write(`{"jsonrpc":"2.0","method":"notifications/private_unknown","params":{}}`)
	write(`{"jsonrpc":"2.0","id":4,"method":"ping","params":{}}`)
	var pong struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(read(), &pong); err != nil || pong.ID != 4 || len(pong.Result) == 0 {
		t.Fatal("unknown notification produced stdout or ping failed")
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not exit after EOF")
	}
}
