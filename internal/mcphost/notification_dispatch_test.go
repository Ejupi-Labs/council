package mcphost

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestRawToolCallNotificationCannotDispatchMutation(t *testing.T) {
	const id = "0123456789abcdef0123456789abcdef"
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
		var message map[string]json.RawMessage
		if json.Unmarshal(frame, &message) != nil {
			t.Fatal("invalid protocol response")
		}
		return message
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
	if _, ok := read()["result"]; !ok {
		t.Fatal("initialization failed")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	write(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"council_drafts_create","arguments":{"input":{"requestId":"` + id + `","projectId":"` + id + `","originalText":"fixture"}}}}`)
	// A subsequent ping proves the notification was consumed and did not
	// produce a fabricated response before checking the owner call count.
	write(`{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}`)
	message := read()
	if string(message["id"]) != "2" || message["result"] == nil {
		t.Fatal("notification unexpectedly produced a response")
	}
	if spy.calls.Load() != 0 {
		t.Fatal("uncorrelated mutation reached the owner")
	}
	write(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"council_drafts_create","arguments":{"input":{"requestId":"` + id + `","projectId":"` + id + `","originalText":"fixture"}}}}`)
	message = read()
	if string(message["id"]) != "3" || message["result"] == nil || spy.calls.Load() != 1 {
		t.Fatal("correlated control did not dispatch exactly once")
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
