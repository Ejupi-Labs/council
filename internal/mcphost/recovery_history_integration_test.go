package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
func TestRecoveryAndHistoryAcrossMCPAndCLI(t *testing.T) {
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
	cliCall := func(stdin []byte, out any, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(callCtx, binary, append(args, "--json")...)
		cmd.Env, cmd.Dir = env, projectDir
		cmd.Stdin = bytes.NewReader(stdin)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var result struct {
			Data  json.RawMessage `json:"data"`
			Error *domain.Error   `json:"error"`
		}
		if json.Unmarshal(stdout.Bytes(), &result) != nil || err != nil || result.Error != nil {
			t.Fatalf("CLI %v failed: %v %s", args, err, stdout.String())
		}
		if bytes.Contains(stdout.Bytes(), []byte(token)) || bytes.Contains(stderr.Bytes(), []byte(token)) {
			t.Fatal("private token exposed")
		}
		if out != nil && json.Unmarshal(result.Data, out) != nil {
			t.Fatalf("CLI %v response shape", args)
		}
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
	cliCall(scope, &resumed, "checkpoints", "resume", paused.Checkpoint.ID, "--checkpoint-revision", paused.Checkpoint.Revision, "--scope-file", "-", "--request-id", domain.NewID())
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
	cliCall(nil, &published, "history", "export", "--preview", preview.Preview.PreviewID, "--revision", preview.Preview.Revision, "--request-id", domain.NewID())
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
	cliCall(nil, &decision, "history", "reconcile", blocked.Operation.OperationID, "--revision", blocked.Operation.Revision, "--request-id", domain.NewID())
	if decision.Decision != "no_publication" || decision.Operation.State != "no_publication" || decision.Operation.Outcome == nil || *decision.Operation.Outcome != "no_publication" {
		t.Fatal("no-publication reconciliation ambiguous")
	}
	retained, err := os.ReadFile(blockedPreview.Preview.DestinationPath)
	if err != nil || !bytes.Equal(retained, existing) {
		t.Fatal("unrelated target was modified")
	}

	// Controlled store fixture: retain a history incident linked to the exact
	// real committed capture receipt, then a separate unresolved incident.
	// Reconciliation below uses the real MCP/CLI/owner, not fabricated proof.
	captureInput := domain.CaptureHistoryInput{RequestID: captureRequestID, ProjectID: project.Project.ID}
	boundID, err := db.FenceHistoryMutation(ctx, ownerID, "history.snapshots.create", captureRequestID, store.Fingerprint("history.snapshots.create", captureInput))
	if err != nil {
		t.Fatal(err)
	}
	independentID := domain.NewID()
	if err := db.Fence(ctx, domain.ExecutionIncident{ID: independentID, Code: "fixture_independent", Message: "Independent fixture incident remains unresolved."}); err != nil {
		t.Fatal(err)
	}
	var pending domain.RecoveryIncidentView
	cliCall(nil, &pending, "incidents", "get", boundID)
	if pending.Incident.ID != boundID || pending.State != "unresolved" {
		t.Fatal("CLI did not inspect the bound incident")
	}
	var reconciliation domain.RecoveryReconciliationResult
	mcpCall("council_incidents_reconcile", map[string]any{"id": boundID, "input": map[string]any{"requestId": domain.NewID(), "expectedIncidentRevision": pending.Revision, "action": "verify_and_reconcile"}}, &reconciliation)
	if reconciliation.Incident.Incident.ID != boundID || reconciliation.Decision.Outcome != "reconciled" || reconciliation.Incident.State != "reconciled" {
		t.Fatal("bound incident did not reconcile conclusively")
	}
	var independent domain.RecoveryIncidentView
	cliCall(nil, &independent, "incidents", "get", independentID)
	if independent.Incident.ID != independentID || independent.State != "unresolved" {
		t.Fatal("independent incident was altered")
	}
	gate, err := db.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != independentID {
		t.Fatal("independent dispatch fence was lost")
	}
}
