package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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

// Actual CLI, MCP SDK/host, authenticated HTTP controller and native owner
// state participate. The process seam runs only the compiled fake worker.
func TestMCPAuthorityAndHistoryRetryAcrossCLI(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v %s", err, out)
	}
	home := t.TempDir()
	root := filepath.Join(home, ".council", "v2")
	lock, err := localstate.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	db, err := store.Open(filepath.Join(root, "workspace.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ownerID := domain.NewID()
	releaseRoot := make(chan struct{})
	e, err := engine.New(db, lock, engine.Config{OwnerInstanceID: ownerID, Prepare: func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
		select {
		case <-releaseRoot:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		spec.Executable = binary
		return process.Prepare(ctx, spec)
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Errorf("owner close: %v", err)
		}
	})
	service := app.New(db)
	service.Execution = e
	projectDir := filepath.Join(t.TempDir(), "project")
	if err = os.Mkdir(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	gitConfig := filepath.Join(home, "empty-gitconfig")
	if err = os.WriteFile(gitConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
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
		t.Fatal(opened.Envelope.Error)
	}
	project, ok := opened.Envelope.Data.(domain.ProjectOpen)
	if !ok {
		t.Fatal("invalid project result")
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
	mcpCall := func(tool string, args map[string]any, out any) {
		t.Helper()
		result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s: %v", tool, err)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var response authorityReplayResponse
		if json.Unmarshal(raw, &response) != nil || result.IsError || response.Envelope.Error != nil {
			t.Fatalf("%s failed: %s", tool, raw)
		}
		if out != nil && json.Unmarshal(response.Envelope.Data, out) != nil {
			t.Fatalf("%s response shape", tool)
		}
	}
	var lastCLIReceipt *domain.Receipt
	cliCall := func(stdin []byte, out any, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(callCtx, binary, append(args, "--json")...)
		cmd.Env, cmd.Dir = env, projectDir
		cmd.Stdin = bytes.NewReader(stdin)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var result struct {
			Data    json.RawMessage `json:"data"`
			Receipt *domain.Receipt `json:"receipt"`
			Error   *domain.Error   `json:"error"`
		}
		if json.Unmarshal(stdout.Bytes(), &result) != nil || err != nil || result.Error != nil {
			t.Fatalf("CLI %v failed: %v %s", args, err, stdout.String())
		}
		lastCLIReceipt = result.Receipt
		if bytes.Contains(stdout.Bytes(), []byte(token)) || bytes.Contains(stderr.Bytes(), []byte(token)) {
			t.Fatal("private token exposed")
		}
		if out != nil && json.Unmarshal(result.Data, out) != nil {
			t.Fatalf("CLI %v response shape", args)
		}
	}
	mcpResult := func(tool string, args map[string]any) authorityReplayResponse {
		t.Helper()
		result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s transport: %v", tool, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out authorityReplayResponse
		if json.Unmarshal(raw, &out) != nil || result.IsError != (out.Envelope.Error != nil) {
			t.Fatalf("%s malformed result", tool)
		}
		return out
	}
	start := map[string]any{"requestId": domain.NewID(), "projectId": project.Project.ID, "executionMode": "fake", "nodes": []any{map[string]any{"key": "worker", "originalText": "Fixture graph member", "acceptance": map[string]any{"deliverable": "response_only"}}}}
	var graph domain.GraphAdmission
	mcpCall("council_graphs_start", map[string]any{"input": start}, &graph)
	if len(graph.Nodes) != 1 {
		t.Fatal("graph member missing")
	}
	member := graph.Nodes[0]
	var paused domain.RecoveryPauseResult
	mcpCall("council_tasks_pause", map[string]any{"taskId": member.TaskID, "input": map[string]any{"requestId": domain.NewID(), "taskVersion": member.TaskVersion, "expectedTaskRevision": member.TaskRevision}}, &paused)
	if paused.Checkpoint.State != "paused" || paused.Checkpoint.Scope.Kind != "member" || len(paused.Checkpoint.Blockers) != 0 {
		t.Fatalf("member checkpoint not safely paused: %+v", paused.Checkpoint)
	}
	// Finish the real fake root input while the selected member remains paused.
	close(releaseRoot)
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		view, err := db.Graph(callCtx, graph.Graph.ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.Graph.RootInputID != nil {
			rootInput, err := db.GraphInputRecord(callCtx, graph.Graph.ID, *view.Graph.RootInputID)
			if err != nil {
				t.Fatal(err)
			}
			if rootInput.Public.Summary.State == "ready" && rootInput.Public.Summary.Current && rootInput.PrivateRoot != nil {
				break
			}
			if rootInput.Public.Summary.State == "failed" {
				t.Fatalf("fake graph root failed: %v", rootInput.Public.Summary.Failure)
			}
		}
		select {
		case <-deadline.C:
			t.Fatal("fake graph root did not finish")
		case <-callCtx.Done():
			t.Fatal("graph root observation timed out")
		case <-time.After(25 * time.Millisecond):
		}
	}
	var cliCheckpoint domain.RecoveryCheckpoint
	cliCall(nil, &cliCheckpoint, "checkpoints", "get", paused.Checkpoint.ID)
	if cliCheckpoint.ID != paused.Checkpoint.ID || cliCheckpoint.Revision != paused.Checkpoint.Revision {
		t.Fatal("CLI saw a different checkpoint")
	}
	scope, _ := json.Marshal(paused.Checkpoint.Scope)
	var resumed struct {
		Checkpoint         domain.RecoveryCheckpoint `json:"checkpoint"`
		Blockers           []string                  `json:"blockers"`
		AdmittedAttemptIDs []string                  `json:"admittedAttemptIds"`
		NextAction         string                    `json:"nextAction"`
	}
	resumeRequestID := domain.NewID()
	cliCall(scope, &resumed, "checkpoints", "resume", paused.Checkpoint.ID, "--checkpoint-revision", paused.Checkpoint.Revision, "--scope-file", "-", "--request-id", resumeRequestID)
	resumeReceipt := lastCLIReceipt
	if resumeReceipt == nil || resumeReceipt.RequestID != resumeRequestID {
		t.Fatal("CLI resume receipt missing")
	}
	var frozenScope map[string]any
	if json.Unmarshal(scope, &frozenScope) != nil {
		t.Fatal("scope encoding")
	}
	resumeInput := map[string]any{"requestId": resumeRequestID, "expectedCheckpointRevision": paused.Checkpoint.Revision, "scope": frozenScope}
	replayResume := mcpResult("council_checkpoints_resume", map[string]any{"id": paused.Checkpoint.ID, "input": resumeInput})
	var replayData struct {
		Checkpoint         domain.RecoveryCheckpoint `json:"checkpoint"`
		Blockers           []string                  `json:"blockers"`
		AdmittedAttemptIDs []string                  `json:"admittedAttemptIds"`
		NextAction         string                    `json:"nextAction"`
	}
	if replayResume.Envelope.Error != nil || !reflect.DeepEqual(resumeReceipt, replayResume.Envelope.Receipt) || json.Unmarshal(replayResume.Envelope.Data, &replayData) != nil || !reflect.DeepEqual(resumed, replayData) {
		t.Fatal("same resume intent did not replay CLI receipt")
	}
	for _, key := range []string{"nodeId", "graphId", "taskId"} {
		altered := map[string]any{}
		for k, v := range frozenScope {
			altered[k] = v
		}
		altered[key] = domain.NewID()
		result := mcpResult("council_checkpoints_resume", map[string]any{"id": paused.Checkpoint.ID, "input": map[string]any{"requestId": resumeRequestID, "expectedCheckpointRevision": paused.Checkpoint.Revision, "scope": altered}})
		if result.Envelope.Error == nil || result.Envelope.Error.Code != "request_conflict" {
			t.Fatalf("changed %s authority did not conflict: %v", key, result.Envelope.Error)
		}
	}
	standalone := map[string]any{"kind": "task", "taskId": member.TaskID, "taskVersion": member.TaskVersion, "expectedTaskRevision": member.TaskRevision}
	conflictScope := mcpResult("council_checkpoints_resume", map[string]any{"id": paused.Checkpoint.ID, "input": map[string]any{"requestId": resumeRequestID, "expectedCheckpointRevision": paused.Checkpoint.Revision, "scope": standalone}})
	if conflictScope.Envelope.Error == nil || conflictScope.Envelope.Error.Code != "request_conflict" {
		t.Fatal("changed scope kind did not conflict")
	}

	if resumed.Checkpoint.State != "resumed" || resumed.NextAction == "" || resumed.AdmittedAttemptIDs == nil || resumed.Blockers == nil {
		t.Fatal("resume returned ambiguous state")
	}
	var checkpointView domain.RecoveryCheckpoint
	mcpCall("council_checkpoints_get", map[string]any{"id": paused.Checkpoint.ID}, &checkpointView)
	if checkpointView.State != "resumed" || checkpointView.Revision != resumed.Checkpoint.Revision {
		t.Fatal("MCP and CLI disagree on resumed checkpoint")
	}
	var capture struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}
	captureRequestID := domain.NewID()
	cliCall(nil, &capture, "history", "capture", "--project", project.Project.ID, "--request-id", captureRequestID)
	if capture.Snapshot.Completeness != domain.HistoryCompleteness || capture.Snapshot.Notice != domain.HistoryNotice || len(capture.Snapshot.OmittedCategories) == 0 {
		t.Fatal("snapshot did not disclose incomplete history")
	}
	var snapshot struct {
		Snapshot domain.HistorySnapshotSummary `json:"snapshot"`
	}
	mcpCall("council_history_snapshot_get", map[string]any{"snapshotId": capture.Snapshot.SnapshotID}, &snapshot)
	if snapshot.Snapshot.SnapshotID != capture.Snapshot.SnapshotID || snapshot.Snapshot.Completeness != domain.HistoryCompleteness {
		t.Fatal("MCP and CLI disagree on history")
	}
	var preview struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}
	mcpCall("council_history_preview", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "snapshotId": capture.Snapshot.SnapshotID, "snapshotRevision": capture.Snapshot.Revision, "filename": "history-published.json"}}, &preview)
	var published struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	exportRequestID := domain.NewID()
	cliCall(nil, &published, "history", "export", "--preview", preview.Preview.PreviewID, "--revision", preview.Preview.Revision, "--request-id", exportRequestID)
	exportReceipt := lastCLIReceipt
	exportArgs := map[string]any{"requestId": exportRequestID, "previewId": preview.Preview.PreviewID, "previewRevision": preview.Preview.Revision}
	exportReplay := mcpResult("council_history_export", map[string]any{"input": exportArgs})
	var exportData struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	if exportReceipt == nil || exportReplay.Envelope.Error != nil || !reflect.DeepEqual(exportReceipt, exportReplay.Envelope.Receipt) || json.Unmarshal(exportReplay.Envelope.Data, &exportData) != nil || !reflect.DeepEqual(published.Operation, exportData.Operation) {
		t.Fatal("history export replay differed across CLI/MCP")
	}
	changedExport := mcpResult("council_history_export", map[string]any{"input": map[string]any{"requestId": exportRequestID, "previewId": domain.NewID(), "previewRevision": preview.Preview.Revision}})
	if changedExport.Envelope.Error == nil || changedExport.Envelope.Error.Code != "request_conflict" {
		t.Fatal("changed preview authority did not conflict")
	}

	if published.Operation.State != "published" || published.Operation.Outcome == nil || *published.Operation.Outcome != "published" || published.Operation.Completeness != domain.HistoryCompleteness || published.Operation.Durability == nil {
		t.Fatal("publication outcome was not explicit")
	}
	var observed struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	mcpCall("council_history_operation_get", map[string]any{"operationId": published.Operation.OperationID}, &observed)
	if observed.Operation.State != "published" || observed.Operation.OperationID != published.Operation.OperationID {
		t.Fatal("published operation mismatch")
	}
	payloadBytes, err := os.ReadFile(published.Operation.DestinationPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload domain.HistoryPayload
	if json.Unmarshal(payloadBytes, &payload) != nil || payload.Snapshot.Completeness != domain.HistoryCompleteness || payload.Snapshot.Notice != domain.HistoryNotice || bytes.Contains(payloadBytes, []byte(token)) {
		t.Fatal("published payload omitted disclosure or exposed authentication")
	}
	var blockedPreview struct {
		Preview domain.HistoryExportPreview `json:"preview"`
	}
	cliCall(nil, &blockedPreview, "history", "export-preview", "--snapshot", capture.Snapshot.SnapshotID, "--revision", capture.Snapshot.Revision, "--filename", "history-conflict.json", "--request-id", domain.NewID())
	if filepath.Base(blockedPreview.Preview.DestinationPath) != "history-conflict.json" {
		t.Fatal("unexpected fixture output path")
	}
	existing := []byte("fixture preexisting bytes")
	if err = os.WriteFile(blockedPreview.Preview.DestinationPath, existing, 0600); err != nil {
		t.Fatal(err)
	}
	var blocked struct {
		Operation domain.HistoryExportOperation `json:"operation"`
	}
	mcpCall("council_history_export", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "previewId": blockedPreview.Preview.PreviewID, "previewRevision": blockedPreview.Preview.Revision}}, &blocked)
	if blocked.Operation.State != "prepared" || blocked.Operation.NextAction != "reconcile" || blocked.Operation.Outcome != nil {
		t.Fatalf("conflict outcome ambiguous: %+v", blocked.Operation)
	}
	var decision domain.HistoryReconciliationResult
	reconcileRequestID := domain.NewID()
	cliCall(nil, &decision, "history", "reconcile", blocked.Operation.OperationID, "--revision", blocked.Operation.Revision, "--request-id", reconcileRequestID)
	reconcileReceipt := lastCLIReceipt
	reconcileArgs := map[string]any{"id": blocked.Operation.OperationID, "input": map[string]any{"requestId": reconcileRequestID, "expectedRevision": blocked.Operation.Revision, "action": "verify_no_publication"}}
	// Catalog name uses operationId as the outer resource key.
	delete(reconcileArgs, "id")
	reconcileArgs["operationId"] = blocked.Operation.OperationID
	reconcileReplay := mcpResult("council_history_reconcile", reconcileArgs)
	var replayDecision domain.HistoryReconciliationResult
	if reconcileReceipt == nil || reconcileReplay.Envelope.Error != nil || !reflect.DeepEqual(reconcileReceipt, reconcileReplay.Envelope.Receipt) || json.Unmarshal(reconcileReplay.Envelope.Data, &replayDecision) != nil || !reflect.DeepEqual(decision, replayDecision) {
		t.Fatal("history reconciliation replay differed across CLI/MCP")
	}
	changedRevision := "2"
	if blocked.Operation.Revision == "2" {
		changedRevision = "3"
	}
	changedReconcile := mcpResult("council_history_reconcile", map[string]any{"operationId": blocked.Operation.OperationID, "input": map[string]any{"requestId": reconcileRequestID, "expectedRevision": changedRevision, "action": "verify_no_publication"}})
	if changedReconcile.Envelope.Error == nil || changedReconcile.Envelope.Error.Code != "request_conflict" {
		t.Fatal("changed reconciliation revision did not conflict")
	}

	if decision.Decision != "no_publication" || decision.Operation.State != "no_publication" || decision.Operation.Outcome == nil || *decision.Operation.Outcome != "no_publication" {
		t.Fatal("no-publication reconciliation ambiguous")
	}
	retained, err := os.ReadFile(blockedPreview.Preview.DestinationPath)
	if err != nil || !bytes.Equal(retained, existing) {
		t.Fatal("unrelated target was modified")
	}

}
