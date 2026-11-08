package mcphost

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/app"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/engine"
	"github.com/Ejupi-Labs/council/v2/internal/httpapi"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/process"
	"github.com/Ejupi-Labs/council/v2/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http/httptest"
)

type authorityReplayResponse struct {
	Status   int `json:"status"`
	Envelope struct {
		Data    json.RawMessage `json:"data"`
		Receipt *domain.Receipt `json:"receipt"`
		Error   *domain.Error   `json:"error"`
	} `json:"envelope"`
}

// This runs the actual controller/store behind authenticated HTTP and MCP.
// A fixture Prepare waits for cancellation, preventing every native launch.
func TestMCPGraphRecoveryHistoryAuthorityReplay(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	dbPath := filepath.Join(root, "workspace.sqlite3")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ownerID := domain.NewID()
	e, err := engine.New(db, lock, engine.Config{OwnerInstanceID: ownerID, Prepare: func(ctx context.Context, _ process.NativeSpec) (*process.Owned, error) {
		<-ctx.Done()
		return nil, errors.New("fixture launch cancelled")
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Errorf("fixture engine cleanup: %v", err)
		}
	})
	service := app.New(db)
	service.Execution = e
	projectDir := filepath.Join(t.TempDir(), "project")
	if err = os.Mkdir(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err = os.WriteFile(config, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+config, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git: %v %s", err, out)
		}
	}
	ctx := context.Background()
	openBody, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "selectedPath": projectDir, "basePath": projectDir})
	opened := service.Execute(ctx, "projects.open", app.Params{Body: openBody})
	if opened.Envelope.Error != nil {
		t.Fatalf("project: %v", opened.Envelope.Error)
	}
	project, ok := opened.Envelope.Data.(domain.ProjectOpen)
	if !ok {
		t.Fatal("unexpected project result")
	}
	policy, err := db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != "autonomous" {
		body, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "expectedRevision": policy.Revision, "mode": "autonomous"})
		result := service.Execute(ctx, "policy.set", app.Params{Body: body})
		if result.Envelope.Error != nil {
			t.Fatal(result.Envelope.Error)
		}
	}
	token := strings.Repeat("e", 64)
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = httpapi.New(service, httpapi.Config{Host: srv.Listener.Addr().String(), Token: token, OwnerInstanceID: ownerID})
	srv.Start()
	t.Cleanup(srv.Close)
	if err = lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: srv.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	session, callCtx := compatibilitySession(t, root)
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	count := func(table string) int {
		t.Helper()
		var query string
		switch table {
		case "graphs":
			query = "SELECT count(*) FROM task_graphs"
		case "checkpoints":
			query = "SELECT count(*) FROM recovery_checkpoints"
		case "snapshots":
			query = "SELECT count(*) FROM history_snapshots"
		case "previews":
			query = "SELECT count(*) FROM history_previews"
		default:
			t.Fatal("unsupported fixture table")
		}
		var n int
		if err := sqlDB.QueryRowContext(callCtx, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	call := func(tool string, args map[string]any) authorityReplayResponse {
		t.Helper()
		result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s: %v", tool, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out authorityReplayResponse
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("invalid result")
		}
		if result.IsError != (out.Envelope.Error != nil) {
			t.Fatal("error indicator differs")
		}
		return out
	}
	replay := func(tool, table string, args, changed map[string]any) authorityReplayResponse {
		t.Helper()
		first := call(tool, args)
		if first.Envelope.Error != nil || first.Envelope.Receipt == nil {
			t.Fatalf("%s first: %v", tool, first.Envelope.Error)
		}
		n := count(table)
		second := call(tool, args)
		var a, b any
		if second.Envelope.Error != nil || second.Envelope.Receipt == nil || !reflect.DeepEqual(first.Envelope.Receipt, second.Envelope.Receipt) || json.Unmarshal(first.Envelope.Data, &a) != nil || json.Unmarshal(second.Envelope.Data, &b) != nil || !reflect.DeepEqual(a, b) {
			t.Fatalf("%s replay changed receipt or data", tool)
		}
		conflict := call(tool, changed)
		if conflict.Envelope.Error == nil || conflict.Envelope.Error.Code != "request_conflict" {
			t.Fatalf("%s changed authority: %v", tool, conflict.Envelope.Error)
		}
		if count(table) != n {
			t.Fatalf("%s changed durable effects on replay/conflict", tool)
		}
		return first
	}
	startID := domain.NewID()
	node := map[string]any{"key": "worker", "originalText": "Fixture graph task", "acceptance": map[string]any{"deliverable": "response_only"}}
	start := map[string]any{"requestId": startID, "projectId": project.Project.ID, "executionMode": "fake", "nodes": []any{node}}
	changedStart := map[string]any{"requestId": startID, "projectId": project.Project.ID, "executionMode": "fake", "nodes": []any{map[string]any{"key": "worker", "originalText": "Changed task", "acceptance": map[string]any{"deliverable": "response_only"}}}}
	graphResult := replay("council_graphs_start", "graphs", map[string]any{"input": start}, map[string]any{"input": changedStart})
	if count("graphs") != 1 {
		t.Fatal("graph creation did not persist exactly one graph")
	}
	var graph domain.GraphAdmission
	if json.Unmarshal(graphResult.Envelope.Data, &graph) != nil || !domain.ValidID(graph.Graph.ID) || len(graph.Nodes) != 1 {
		t.Fatal("invalid graph admission")
	}
	pauseID := domain.NewID()
	authority := graph.DispatchAuthority
	pause := map[string]any{"requestId": pauseID, "graphVersion": graph.Graph.GraphVersion, "expectedGraphControlRevision": graph.Graph.ControlRevision, "expectedBudgetAuthorityEpoch": graph.Graph.BudgetAuthorityEpoch, "expectedWindowNumber": authority.WindowNumber, "nodeAuthorities": authority.NodeAuthorities}
	changedPause := map[string]any{}
	for k, v := range pause {
		changedPause[k] = v
	}
	changedPause["expectedWindowNumber"] = "2"
	if authority.WindowNumber == "2" {
		changedPause["expectedWindowNumber"] = "3"
	}
	pauseResult := replay("council_graphs_pause", "checkpoints", map[string]any{"graphId": graph.Graph.ID, "input": pause}, map[string]any{"graphId": graph.Graph.ID, "input": changedPause})
	if count("checkpoints") != 1 {
		t.Fatal("pause did not persist exactly one checkpoint")
	}
	var paused domain.RecoveryPauseResult
	if json.Unmarshal(pauseResult.Envelope.Data, &paused) != nil || !domain.ValidID(paused.Checkpoint.ID) {
		t.Fatal("invalid pause checkpoint")
	}
	current, err := db.Graph(ctx, graph.Graph.ID)
	if err != nil {
		t.Fatal(err)
	}
	stopID := domain.NewID()
	stop := map[string]any{"requestId": stopID, "expectedGraphControlRevision": current.Graph.ControlRevision}
	changedStop := map[string]any{"requestId": stopID, "expectedGraphControlRevision": "999"}
	stopResult := replay("council_graphs_stop", "graphs", map[string]any{"graphId": graph.Graph.ID, "input": stop}, map[string]any{"graphId": graph.Graph.ID, "input": changedStop})
	var stopped domain.GraphStopResult
	if json.Unmarshal(stopResult.Envelope.Data, &stopped) != nil {
		t.Fatal("invalid graph stop result")
	}
	afterStop, err := db.Graph(ctx, graph.Graph.ID)
	if err != nil || afterStop.Graph.ControlRevision != stopped.Graph.ControlRevision {
		t.Fatal("stop replay changed graph control authority")
	}
	captureID := domain.NewID()
	capture := map[string]any{"requestId": captureID, "projectId": project.Project.ID}
	changedCapture := map[string]any{"requestId": captureID, "projectId": domain.NewID()}
	captureResult := replay("council_history_capture", "snapshots", map[string]any{"input": capture}, map[string]any{"input": changedCapture})
	if count("snapshots") != 1 {
		t.Fatal("capture did not persist exactly one snapshot")
	}
	var snapshot struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}
	if json.Unmarshal(captureResult.Envelope.Data, &snapshot) != nil || !domain.ValidID(snapshot.Snapshot.SnapshotID) {
		t.Fatal("invalid history snapshot")
	}
	previewID := domain.NewID()
	preview := map[string]any{"requestId": previewID, "snapshotId": snapshot.Snapshot.SnapshotID, "snapshotRevision": snapshot.Snapshot.Revision, "filename": "history-fixture.json"}
	changedPreview := map[string]any{"requestId": previewID, "snapshotId": snapshot.Snapshot.SnapshotID, "snapshotRevision": snapshot.Snapshot.Revision, "filename": "history-other.json"}
	replay("council_history_preview", "previews", map[string]any{"input": preview}, map[string]any{"input": changedPreview})
	if count("previews") != 1 {
		t.Fatal("preview did not persist exactly one preview")
	}
}
