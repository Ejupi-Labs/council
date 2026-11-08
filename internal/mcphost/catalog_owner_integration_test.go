package mcphost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/ownerclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Every advertised tool reaches the real authenticated owner in a fresh
// private home. Mutations use one fixed identity twice so a successful direct
// call is replayed by MCP instead of creating a second effect. Unknown resource
// fixtures exercise the owner's real failure semantics, not fake successes.
func TestCatalogAgainstRealOwner(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home, project := t.TempDir(), t.TempDir()
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	root := filepath.Join(home, ".council", "v2")
	var ownerID string
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil {
			return
		}
		if ownerID == "" {
			ownerID = desc.OwnerInstanceID
		}
		if ownerID != desc.OwnerInstanceID {
			t.Error("fixture owner changed")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "service", "stop", "--owner-instance", ownerID, "--request-id", domain.NewID(), "--json")
		cmd.Dir, cmd.Env = project, env
		_ = cmd.Run()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.Command(binary, "mcp")
	cmd.Dir, cmd.Env = project, env
	client := mcp.NewClient(&mcp.Implementation{Name: "catalog-owner-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	call := func(name string, args any) (domain.Envelope, int) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result == nil || len(result.Content) != 1 {
			t.Fatalf("%s transport: %v", name, err)
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("%s missing text result", name)
		}
		var payload struct {
			Status   int             `json:"status"`
			Envelope domain.Envelope `json:"envelope"`
		}
		if json.Unmarshal([]byte(text.Text), &payload) != nil || result.IsError != (payload.Envelope.Error != nil) {
			t.Fatalf("%s invalid envelope", name)
		}
		return payload.Envelope, payload.Status
	}
	if _, status := call("council_owner_meta", map[string]any{}); status != 200 {
		t.Fatal("owner bootstrap failed")
	}
	desc, err := localstate.ReadDescriptor(root)
	if err != nil || !domain.ValidID(desc.OwnerInstanceID) {
		t.Fatal("fixture owner descriptor unavailable")
	}
	ownerID = desc.OwnerInstanceID
	owner, err := ownerclient.New(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, mapping := range ToolCatalog() {
		if mapping.Operation == "service.stop" {
			continue
		}
		route, ok := ownerclient.RouteFor(mapping.Operation)
		if !ok {
			t.Fatalf("missing route %s", mapping.Operation)
		}
		args := map[string]any{}
		for i, kind := range route.PathKinds {
			value := fmt.Sprintf("%032x", i+1)
			switch kind {
			case "revision":
				value = "2"
			case "graphkey":
				value = "node-one"
			case "context":
				value = "codex-native"
			case "profile":
				value = "codex-native-default"
			}
			args[toolPathArguments[mapping.Name][i]] = value
		}
		switch mapping.Operation {
		case "routing.capacity.get", "routing.reports.list":
			args["contextId"] = "codex-native"
		case "history.snapshots.records":
			args["kind"] = "tasks"
		}
		if route.Method != "GET" {
			body := parityMutationBody(mapping.Operation, domain.NewID(), ownerID, project)
			if body == nil {
				t.Fatalf("missing mutation fixture %s", mapping.Operation)
			}
			args["input"] = body
		}
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		request, invalid := OwnerRequest(mapping, raw)
		if invalid != nil {
			t.Fatalf("%s argument mapping: %v", mapping.Name, invalid)
		}
		if invalid = ValidateArguments(mapping.Name, mapping.Operation, raw, request); invalid != nil {
			t.Fatalf("%s argument validation: %v", mapping.Name, invalid)
		}
		direct := owner.Call(ctx, request)
		envelope, status := call(mapping.Name, args)
		if status != direct.Status || (envelope.Error == nil) != (direct.Envelope.Error == nil) {
			t.Fatalf("%s status mismatch: MCP %d vs owner %d", mapping.Name, status, direct.Status)
		}
		if envelope.Error != nil && envelope.Error.Code != direct.Envelope.Error.Code {
			t.Fatalf("%s error mismatch: %s vs %s", mapping.Name, envelope.Error.Code, direct.Envelope.Error.Code)
		}
		if route.Method != "GET" && (envelope.RequestID != direct.Envelope.RequestID || !reflect.DeepEqual(envelope.Receipt, direct.Envelope.Receipt)) {
			t.Fatalf("%s mutation identity or receipt mismatch", mapping.Name)
		}
		seen++
	}
	// Stop is the last operation because it deliberately closes this owner.
	requestID := domain.NewID()
	stopped, status := call("council_service_stop", map[string]any{"input": map[string]any{"requestId": requestID, "ownerInstanceId": ownerID}})
	if status != 202 || stopped.Error != nil || stopped.Receipt == nil || stopped.Receipt.RequestID != requestID {
		t.Fatalf("explicit owner stop did not retain its receipt: status=%d error=%v receipt=%v", status, stopped.Error, stopped.Receipt)
	}
	seen++
	if seen != len(ToolCatalog()) {
		t.Fatalf("real owner received %d/%d catalog tools", seen, len(ToolCatalog()))
	}
	if err := session.Close(); err != nil {
		t.Fatal("fixture MCP host did not close")
	}
}
