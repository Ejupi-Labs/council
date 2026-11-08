package mcphost

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"strings"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/mcphost/schemas"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OwnerCaller is the continuing owner's fixed-route transport. It is injected
// for offline tests; it must not start a second engine or accept arbitrary URLs.
type OwnerCaller interface {
	Call(context.Context, ownerclient.Request) domain.Result
}

// NewHost binds the independently reviewed catalog to one owner transport.
// A missing or changed schema aborts registration rather than exposing a
// partial, permissive set of tools.
func NewHost(owner OwnerCaller) (*Server, error) {
	if owner == nil {
		return nil, errors.New("Council owner transport is unavailable")
	}
	mappings := ToolCatalog()
	if len(mappings) == 0 || len(mappings) != len(schemas.Names()) {
		return nil, errors.New("Council MCP catalog is incomplete")
	}
	for _, mapping := range mappings {
		if _, ok := schemas.Lookup(mapping.Name); !ok {
			return nil, errors.New("Council MCP schema is unavailable")
		}
		if _, ok := ownerclient.RouteFor(mapping.Operation); !ok {
			return nil, errors.New("Council MCP owner route is unavailable")
		}
	}
	return NewServer(func(server *mcp.Server) {
		for _, mapping := range mappings {
			mapping := mapping
			rawSchema, _ := schemas.Lookup(mapping.Name)
			route, _ := ownerclient.RouteFor(mapping.Operation)
			server.AddTool(&mcp.Tool{
				Name:        mapping.Name,
				Description: toolDescription(mapping.Operation, route.Method),
				InputSchema: json.RawMessage(rawSchema),
				Annotations: &mcp.ToolAnnotations{ReadOnlyHint: route.Method == "GET"},
			}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				if req == nil || req.Params == nil {
					return toolFailure(domain.Invalid("documented tool arguments"), ""), nil
				}
				if req.Params.Name != mapping.Name || req.Params.RequestState != "" || len(req.Params.InputResponses) != 0 {
					return toolFailure(domain.Invalid("supported tool call parameters"), ""), nil
				}
				arguments := []byte(req.Params.Arguments)
				request, invalid := OwnerRequest(mapping, arguments)
				if invalid != nil {
					return toolFailure(invalid, request.RequestID), nil
				}
				if invalid = ValidateArguments(mapping.Name, mapping.Operation, arguments, request); invalid != nil {
					return toolFailure(invalid, request.RequestID), nil
				}
				return toolResult(owner.Call(ctx, request)), nil
			})
		}
	}), nil
}

func toolDescription(operation, method string) string {
	switch operation {
	case "artifacts.bytes":
		return "Read an artifact byte page. Reuse the returned through boundary and concatenate bytesBase64."
	case "tasks.start":
		return "Start a task with the complete original instructions and a durable requestId. Admission is not completion."
	case "service.stop":
		return "Stop the explicitly identified Council owner with a durable requestId."
	}
	if strings.HasPrefix(operation, "history.") {
		return "Inspect or export incomplete metadata history; this is not an evidence backup or restore."
	}
	description := "Council " + strings.ReplaceAll(operation, ".", " ") + "."
	if method != "GET" {
		description += " Supply the durable requestId and exact captured authority."
	}
	return description
}

func toolFailure(err *domain.Error, requestID string) *mcp.CallToolResult {
	if !domain.ValidID(requestID) {
		requestID = domain.NewID()
	}
	return toolResult(domain.Failure(requestID, err))
}

func toolResult(result domain.Result) *mcp.CallToolResult {
	type response struct {
		Status   int             `json:"status"`
		Envelope domain.Envelope `json:"envelope"`
	}
	payload, err := jsonv2.Marshal(response{Status: result.Status, Envelope: result.Envelope})
	if err != nil {
		fallback := domain.Failure(domain.NewID(), domain.Fail("internal_error", "The result could not be encoded."))
		payload, _ = jsonv2.Marshal(response{Status: fallback.Status, Envelope: fallback.Envelope})
		result = fallback
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(payload)}},
		StructuredContent: json.RawMessage(payload),
		IsError:           result.Envelope.Error != nil,
	}
}
