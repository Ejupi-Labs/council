package mcphost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type parityObservation struct {
	operation string
	params    app.Params
}
type parityExecutor struct {
	mu           sync.Mutex
	observations []parityObservation
	ownerID      string
	fail         bool
}

func (p *parityExecutor) Execute(_ context.Context, operation string, params app.Params) domain.Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.observations = append(p.observations, parityObservation{operation, params})
	id := domain.NewID()
	if len(params.Body) > 0 {
		var request struct {
			RequestID string `json:"requestId"`
		}
		_ = json.Unmarshal(params.Body, &request)
		id = request.RequestID
	}
	if p.fail {
		return domain.Failure(id, domain.Fail("revision_conflict", "Fixture conflict."))
	}
	var receipt *domain.Receipt
	if len(params.Body) > 0 {
		receipt = &domain.Receipt{RequestID: id, Operation: operation, CommittedAt: "2026-01-02T03:04:05.000Z"}
	}
	return domain.Success(200, id, map[string]any{"observed": true}, receipt)
}
func (p *parityExecutor) Metadata(context.Context) (map[string]any, error) {
	return map[string]any{
		"scope": "managed_execution", "managedExecution": "native_codex_claude",
		"executionContract": domain.ExecutionContract, "workflowContract": domain.WorkflowContract, "graphContract": domain.GraphContract,
		"recoveryContract": "007-v1", "historyContract": "007a-v1", "maintenanceContract": "owner-stop/1",
		"capabilities": []string{"workspace_foundation", "managed_execution", "economical_routing", "verified_delivery", "task_graphs", "safe_recovery", "metadata_history"},
		"admission":    map[string]any{"state": "open", "incidentId": nil},
		"capacity":     map[string]any{"resourceId": domain.ManagedNativeResourceID, "limit": 2, "activeAttemptId": nil, "occupiedSlots": "0", "activeClaims": []any{}},
	}, nil
}

func parityMutationBody(op, requestID, ownerID, base string) map[string]any {
	const id = "0123456789abcdef0123456789abcdef"
	in := map[string]any{"requestId": requestID}
	set := func(pairs ...any) {
		for i := 0; i < len(pairs); i += 2 {
			in[pairs[i].(string)] = pairs[i+1]
		}
	}
	authority := []any{map[string]any{"nodeId": id, "taskVersion": "1", "nodeControlRevision": "1"}}
	switch op {
	case "projects.open":
		set("selectedPath", ".", "basePath", base)
	case "policy.set":
		set("expectedRevision", "1", "mode", "autonomous")
	case "drafts.create":
		set("projectId", id, "originalText", "fixture")
	case "drafts.update":
		set("expectedRevision", "1", "originalText", "fixture")
	case "service.stop":
		set("ownerInstanceId", ownerID)
	case "tasks.start":
		set("projectId", id, "originalText", "fixture")
	case "tasks.guidance":
		set("expectedRevision", "1", "guidanceText", "")
	case "tasks.attempts.start":
		set("expectedRevision", "1", "taskVersion", "1")
	case "tasks.response.accept":
		set("expectedRevision", "1", "taskVersion", "1", "attemptId", id, "responseArtifactId", id, "responseSha256", strings.Repeat("a", 64), "confirmation", "reviewed_response_and_conditions")
	case "attempts.stop":
	case "routing.policy.update", "routing.context.update", "routing.profile.update", "routing.accounts.update":
		set("expectedRevision", "1")
	case "routing.accounts.create":
		set("label", "fixture")
	case "routing.links.update":
		set("expectedRevision", "1", "unlink", true)
	case "routing.reports.create":
		set("expectedRevision", "1", "subject", map[string]any{"kind": "context", "id": "codex-native"}, "dimension", "occupancy", "state", "busy")
	case "tasks.budget.amend":
		set("expectedRevision", "1", "action", "grant", "expectedBudgetRevision", "1", "additionalAdmissions", "1")
	case "workflow.preferences.update":
		set("expectedRevision", "1", "localMode", "review_only")
	case "tasks.workflow.adopt":
		set("expectedRevision", "1", "taskVersion", "1", "workflow", map[string]any{"mode": "auto"})
	case "tasks.workflow.advance":
		set("expectedRevision", "1", "workflowRevision", "1", "taskVersion", "1")
	case "tasks.workflow.check":
		set("expectedRevision", "1", "workflowRevision", "1", "taskVersion", "1", "checkId", id)
	case "graphs.start":
		set("projectId", id, "nodes", []any{map[string]any{"key": "node-one", "originalText": "fixture"}})
	case "graphs.advance", "graphs.pause":
		set("graphVersion", "1", "expectedGraphControlRevision", "1", "expectedBudgetAuthorityEpoch", "1", "expectedWindowNumber", "1", "nodeAuthorities", authority)
	case "graphs.stop":
		set("expectedGraphControlRevision", "1")
	case "graphs.nodes.stop":
		set("expectedGraphControlRevision", "1", "nodeId", id, "expectedNodeControlRevision", "1")
	case "graphs.nodes.guidance":
		set("expectedGraphControlRevision", "1", "nodeId", id, "expectedNodeControlRevision", "1", "taskVersion", "1", "expectedRevision", "1", "guidanceText", "")
	case "graphs.budget.amend":
		set("expectedGraphControlRevision", "1", "expectedBudgetAuthorityEpoch", "1", "expectedWindowNumber", "1", "additionalAdmissions", "1")
	case "tasks.pause":
		set("taskVersion", "1", "expectedTaskRevision", "1")
	case "checkpoints.resume":
		set("expectedCheckpointRevision", "1", "scope", map[string]any{"kind": "task", "taskId": id, "taskVersion": "1", "expectedTaskRevision": "1"})
	case "incidents.reconcile":
		set("expectedIncidentRevision", "1", "action", "verify_and_reconcile")
	case "history.snapshots.create":
		set("projectId", id)
	case "history.exports.preview":
		set("snapshotId", id, "snapshotRevision", "1", "filename", "history-fixture.json")
	case "history.exports.create":
		set("previewId", id, "previewRevision", "1")
	case "history.operations.reconcile":
		set("expectedRevision", "1", "action", "verify_no_publication")
	default:
		return nil
	}
	return in
}

// This is an authenticated routing fixture, not the real owner engine or a
// transactional replay test. Real engine/CLI/UI journeys remain separate.
func TestCatalogAllToolsReachAuthenticatedHTTPRouter(t *testing.T) {
	const token = "council-fixture-private-token-00000000000000000000000"
	ownerID := domain.NewID()
	spy := &parityExecutor{ownerID: ownerID}
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = httpapi.New(spy, httpapi.Config{Host: server.Listener.Addr().String(), Token: token, OwnerInstanceID: ownerID})
	server.Start()
	defer server.Close()
	root := filepath.Join(t.TempDir(), "owner")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: 1, StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	owner, err := ownerclient.New(root)
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(owner)
	if err != nil {
		t.Fatal(err)
	}
	clientConn, hostConn := net.Pipe()
	defer clientConn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, host, hostConn, hostConn) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientConn, Writer: clientConn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	items := ToolCatalog()
	if len(items) != 91 {
		t.Fatalf("unexpected catalog size: %d", len(items))
	}
	for _, mapping := range items {
		t.Run(mapping.Name, func(t *testing.T) {
			route, ok := ownerclient.RouteFor(mapping.Operation)
			if !ok {
				t.Fatal("missing route")
			}
			args := map[string]any{}
			ids := []string{}
			for i, kind := range route.PathKinds {
				var value string
				switch kind {
				case "id":
					value = fmt.Sprintf("%032x", i+1)
				case "revision":
					value = "2"
				case "graphkey":
					value = "node-one"
				case "context":
					value = "codex-native"
				case "profile":
					value = "codex-native-default"
				default:
					t.Fatal("unknown path kind")
				}
				args[toolPathArguments[mapping.Name][i]] = value
				ids = append(ids, value)
			}
			query := map[string]string{}
			for _, key := range route.QueryKeys {
				var value string
				switch key {
				case "projectId", "inputId":
					value = "0123456789abcdef0123456789abcdef"
				case "contextId":
					value = "codex-native"
				case "accountId":
					if _, ok := query["contextId"]; ok {
						continue
					}
					value = "0123456789abcdef0123456789abcdef"
				case "workflowId":
					value = "0123456789abcdef0123456789abcdef"
				case "taskVersion":
					if _, ok := query["workflowId"]; ok {
						continue
					}
					value = "1"
				case "revision", "afterRevision", "offset":
					value = "1"
				case "through", "limit", "timeoutMs":
					value = "2"
				case "nodeKey":
					value = "node-one"
				case "kind":
					value = "tasks"
				case "cursor":
					value = base64.RawURLEncoding.EncodeToString([]byte(`{}`))
					if mapping.Operation == "history.snapshots.records" {
						value += "." + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
					}
				default:
					t.Fatalf("unreviewed query key %s", key)
				}
				args[key] = value
				query[key] = value
			}
			requestID := domain.NewID()
			var body []byte
			if route.Method != "GET" {
				input := parityMutationBody(mapping.Operation, requestID, ownerID, root)
				if input == nil {
					t.Fatalf("missing valid mutation fixture for %s", mapping.Operation)
				}
				args["input"] = input
				body, err = json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
			}
			rawArgs, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			call := func() (*mcp.CallToolResult, domain.Envelope, int) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: mapping.Name, Arguments: json.RawMessage(rawArgs)})
				if err != nil || result == nil || len(result.Content) != 1 {
					t.Fatalf("MCP call failed: %v", err)
				}
				text, ok := result.Content[0].(*mcp.TextContent)
				if !ok {
					t.Fatal("missing text envelope")
				}
				var payload struct {
					Status   int             `json:"status"`
					Envelope domain.Envelope `json:"envelope"`
				}
				if json.Unmarshal([]byte(text.Text), &payload) != nil {
					t.Fatal("invalid MCP envelope")
				}
				return result, payload.Envelope, payload.Status
			}
			spy.mu.Lock()
			before := len(spy.observations)
			spy.fail = false
			spy.mu.Unlock()
			result, envelope, status := call()
			if result.IsError || status != 200 || envelope.Error != nil {
				t.Fatalf("valid call rejected: %+v", envelope.Error)
			}
			if mapping.Operation == "meta" {
				return
			}
			spy.mu.Lock()
			if len(spy.observations) != before+1 {
				spy.mu.Unlock()
				t.Fatal("router did not receive exactly one call")
			}
			observation := spy.observations[before]
			spy.mu.Unlock()
			if observation.operation != mapping.Operation || observation.params.ID != strings.Join(ids, "/") || !reflect.DeepEqual(observation.params.Query, query) || !bytes.Equal(observation.params.Body, body) {
				t.Fatal("HTTP router changed operation, identity order, query or body")
			}
			if route.Method == "GET" {
				if envelope.Receipt != nil {
					t.Fatal("read acquired receipt")
				}
				spy.mu.Lock()
				spy.fail = true
				spy.mu.Unlock()
				failure, failedEnvelope, failedStatus := call()
				if !failure.IsError || failedStatus != 409 || failedEnvelope.Error == nil || failedEnvelope.Error.Code != "revision_conflict" || failedEnvelope.Receipt != nil {
					t.Fatal("owner read error status, envelope or receipt changed")
				}
				return
			}
			if envelope.Receipt == nil || envelope.Receipt.RequestID != requestID || envelope.Receipt.Operation != mapping.Operation || envelope.Receipt.CommittedAt != "2026-01-02T03:04:05.000Z" {
				t.Fatal("mutation receipt changed")
			}
			_, replay, replayStatus := call()
			if replayStatus != status || !reflect.DeepEqual(replay.Receipt, envelope.Receipt) || replay.RequestID != envelope.RequestID {
				t.Fatal("fixture acknowledgement replay changed")
			}
			spy.mu.Lock()
			spy.fail = true
			spy.mu.Unlock()
			failure, failedEnvelope, failedStatus := call()
			if !failure.IsError || failedStatus != 409 || failedEnvelope.Error == nil || failedEnvelope.Error.Code != "revision_conflict" || failedEnvelope.Receipt != nil {
				t.Fatal("owner error status, envelope or receipt changed")
			}
		})
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
		t.Fatal("host did not stop")
	}
}
