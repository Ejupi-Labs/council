package mcphost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSessionInitializationAndVersionNegotiation(t *testing.T) {
	for _, version := range []struct{ request, want string }{
		{"2025-06-18", "2025-06-18"},
		{"2025-11-25", "2025-11-25"},
		{"2099-01-01", "2025-11-25"},
	} {
		t.Run(version.request, func(t *testing.T) {
			client, connection := net.Pipe()
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(10 * time.Second))
			done := make(chan error, 1)
			go func() { done <- Serve(context.Background(), NewServer(nil), connection, connection) }()
			reader := bufio.NewReader(client)
			write := func(s string) {
				t.Helper()
				if _, err := client.Write([]byte(s + "\n")); err != nil {
					t.Fatal(err)
				}
			}
			read := func() map[string]json.RawMessage {
				t.Helper()
				data, err := reader.ReadBytes('\n')
				if err != nil {
					t.Fatal(err)
				}
				var msg map[string]json.RawMessage
				if err := json.Unmarshal(data, &msg); err != nil {
					t.Fatal(err)
				}
				return msg
			}
			write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + version.request + `","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
			response := read()
			if !strings.Contains(string(response["result"]), `"protocolVersion":"`+version.want+`"`) {
				t.Fatalf("negotiation: %s", response["result"])
			}
			write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
			write(`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
			if _, ok := read()["result"]; !ok {
				t.Fatal("initialized tools/list failed")
			}
			_ = client.Close()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("server did not stop at EOF")
			}
		})
	}
}

func TestSessionPipelinedCallBeforeInitializedNeverDispatches(t *testing.T) {
	var calls atomic.Int32
	server := NewServer(func(s *mcp.Server) {
		s.AddTool(&mcp.Tool{Name: "fixture", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			return &mcp.CallToolResult{}, nil
		})
	})
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
	pipeline := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fixture","arguments":{}}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	go func() { _, _ = client.Write([]byte(pipeline)) }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidFrame) {
			t.Fatalf("expected rejected pre-initialization frame: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not reject pipeline")
	}
	if calls.Load() != 0 {
		t.Fatal("pre-initialization call dispatched")
	}
}

func TestSessionInvalidHandshakeNeverEnablesDispatch(t *testing.T) {
	var calls atomic.Int32
	server := NewServer(func(s *mcp.Server) {
		s.AddTool(&mcp.Tool{Name: "fixture", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			return &mcp.CallToolResult{}, nil
		})
	})
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, connection, connection) }()
	pipeline := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fixture","arguments":{}}}` + "\n"
	go func() { _, _ = client.Write([]byte(pipeline)) }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidFrame) {
			t.Fatalf("expected invalid handshake rejection: %v", err)
		}
	case <-time.After(9 * time.Second):
		t.Fatal("invalid handshake did not terminate")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid handshake dispatched a call")
	}
}

func TestSessionPipelinedInitializedThenCall(t *testing.T) {
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), NewServer(nil), connection, connection) }()
	reader := bufio.NewReader(client)
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}` + "\n"
	if _, err := client.Write([]byte(init)); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	pipeline := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
	go func() { _, _ = client.Write([]byte(pipeline)) }()
	data, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"result"`) {
		t.Fatalf("initialized pipeline failed: %s", data)
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

func TestSessionDefaultSDKClientNegotiatesFallback(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, NewServer(nil), serverConn, serverConn) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientConn, Writer: clientConn}, nil)
	if err != nil {
		t.Fatalf("SDK fallback initialization failed: %v", err)
	}
	if _, err := session.ListTools(ctx, nil); err != nil {
		t.Fatalf("SDK tools/list failed: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not stop")
	}
}

func TestSessionCancellationReachesOnlyCallContext(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := NewServer(func(s *mcp.Server) {
		s.AddTool(&mcp.Tool{Name: "wait", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		})
	})
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), server, connection, connection) }()
	reader := bufio.NewReader(client)
	write := func(s string) {
		t.Helper()
		if _, err := client.Write([]byte(s + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	read := func() {
		t.Helper()
		if _, err := reader.ReadBytes('\n'); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`)
	read()
	write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	write(`{"jsonrpc":"2.0","id":42,"method":"tools/call","params":{"name":"wait","arguments":{}}}`)
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not start")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":42}}`)
	select {
	case <-cancelled:
	case <-time.After(10 * time.Second):
		t.Fatal("handler context was not cancelled")
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not close")
	}
}

func TestSessionContextCancellationInterruptsBlockedCall(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	server := NewServer(func(s *mcp.Server) {
		s.AddTool(&mcp.Tool{Name: "wait", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		})
	})
	client, connection := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, server, connection, connection) }()
	reader := bufio.NewReader(client)
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}` + "\n"
	if _, err := client.Write([]byte(init)); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	pipeline := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"wait","arguments":{}}}` + "\n"
	go func() { _, _ = client.Write([]byte(pipeline)) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("handler context was not cancelled")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not exit with unread output")
	}
}

func TestSessionReadinessIsIndependent(t *testing.T) {
	server := NewServer(nil)
	clientA, connectionA := net.Pipe()
	clientB, connectionB := net.Pipe()
	defer clientA.Close()
	defer clientB.Close()
	_ = clientA.SetDeadline(time.Now().Add(10 * time.Second))
	_ = clientB.SetDeadline(time.Now().Add(10 * time.Second))
	done := make(chan error, 2)
	go func() { done <- Serve(context.Background(), server, connectionA, connectionA) }()
	go func() { done <- Serve(context.Background(), server, connectionB, connectionB) }()
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}` + "\n"
	readerA, readerB := bufio.NewReader(clientA), bufio.NewReader(clientB)
	if _, err := clientA.Write([]byte(init)); err != nil {
		t.Fatal(err)
	}
	if _, err := readerA.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := clientB.Write([]byte(init)); err != nil {
		t.Fatal(err)
	}
	if _, err := readerB.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := clientA.Write([]byte("{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := clientB.Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\",\"params\":{}}\n")); err != nil {
		t.Fatal(err)
	}
	_, err := readerB.ReadBytes('\n')
	if err == nil {
		t.Fatal("second session inherited readiness")
	}
	_ = clientA.Close()
	_ = clientB.Close()
	for range 2 {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("server did not close")
		}
	}
}
