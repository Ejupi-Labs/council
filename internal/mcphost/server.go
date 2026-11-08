package mcphost

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server owns only MCP sessions; it does not own or stop the Council owner.
type Server struct {
	sdk   *mcp.Server
	mu    sync.Mutex
	ready map[*mcp.ServerSession]bool
	gates map[*mcp.ServerSession]*initializationGate
}

// NewServer registers the approved tools before any session is accepted.
func NewServer(register func(*mcp.Server)) *Server {
	s := &Server{ready: make(map[*mcp.ServerSession]bool), gates: make(map[*mcp.ServerSession]*initializationGate)}
	s.sdk = mcp.NewServer(&mcp.Implementation{Name: "Council", Version: domain.AppVersion}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{"2025-06-18", "2025-11-25"},
		Capabilities:              &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		Logger:                    slog.New(slog.NewTextHandler(io.Discard, nil)),
		InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
			s.mu.Lock()
			s.ready[req.Session] = true
			if gate := s.gates[req.Session]; gate != nil {
				gate.acknowledge()
			}
			s.mu.Unlock()
		},
	})
	s.sdk.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "initialize", "notifications/initialized", "notifications/cancelled", "ping":
				return next(ctx, method, req)
			}
			session, ok := req.GetSession().(*mcp.ServerSession)
			s.mu.Lock()
			ready := ok && s.ready[session]
			s.mu.Unlock()
			if !ready {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidRequest, Message: "session not initialized"}
			}
			return next(ctx, method, req)
		}
	})
	if register != nil {
		register(s.sdk)
	}
	return s
}

// Serve handles one connection. EOF and cancellation do not stop Council work.
// Returned framing and output errors contain no request data.
func Serve(ctx context.Context, server *Server, input io.ReadCloser, output io.WriteCloser) error {
	reader := newFrameReader(input, MaxFrameBytes)
	gate := newInitializationGate()
	reader.initialization = gate
	writer := &protocolWriter{destination: output}
	server.mu.Lock()
	session, err := server.sdk.Connect(ctx, &mcp.IOTransport{Reader: reader, Writer: writer, MaxLineLength: -1}, nil)
	if err != nil {
		server.mu.Unlock()
		return errors.New("MCP connection failed")
	}
	server.gates[session] = gate
	server.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	var waitErr error
	select {
	case <-ctx.Done():
		// Closing the transport first interrupts pending reads and writes so the
		// SDK can cancel call contexts before its graceful session drain.
		_ = reader.Close()
		_ = writer.Close()
		_ = session.Close()
		waitErr = <-done
	case waitErr = <-done:
	}
	server.mu.Lock()
	delete(server.ready, session)
	delete(server.gates, session)
	server.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := reader.err(); err != nil {
		return err
	}
	if err := writer.err(); err != nil {
		return err
	}
	if waitErr != nil {
		return errors.New("MCP connection failed")
	}
	return nil
}
