package mcphost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

// A test-only settlement is delivered after a real accepted MCP Stop while
// preparation is held before process creation. It is not a native success.
func TestMCPAcceptedStopWinsInjectedLateSettlement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
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
	prepared := make(chan struct{})
	release := make(chan struct{})
	var preparedOnce, releaseOnce sync.Once
	releaseBarrier := func() { releaseOnce.Do(func() { close(release) }) }
	ownerID := domain.NewID()
	e, err := engine.New(db, lock, engine.Config{OwnerInstanceID: ownerID, Prepare: func(context.Context, process.NativeSpec) (*process.Owned, error) {
		preparedOnce.Do(func() { close(prepared) })
		<-release
		return nil, errors.New("fixture deliberately created no process")
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Errorf("owner cleanup: %v", err)
		}
	})
	t.Cleanup(releaseBarrier)
	service := app.New(db)
	service.Execution = e
	projectDir := filepath.Join(t.TempDir(), "project")
	if err = os.Mkdir(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err = os.WriteFile(gitConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git: %v %s", err, out)
		}
	}
	ctx := context.Background()
	body, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "selectedPath": projectDir, "basePath": projectDir})
	opened := service.Execute(ctx, "projects.open", app.Params{Body: body})
	if opened.Envelope.Error != nil {
		t.Fatal(opened.Envelope.Error)
	}
	project, ok := opened.Envelope.Data.(domain.ProjectOpen)
	if !ok {
		t.Fatal("invalid project fixture")
	}
	policy, err := db.Policy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != "autonomous" {
		body, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "expectedRevision": policy.Revision, "mode": "autonomous"})
		r := service.Execute(ctx, "policy.set", app.Params{Body: body})
		if r.Envelope.Error != nil {
			t.Fatal(r.Envelope.Error)
		}
	}
	token := strings.Repeat("b", 64)
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = httpapi.New(service, httpapi.Config{Host: srv.Listener.Addr().String(), Token: token, OwnerInstanceID: ownerID})
	srv.Start()
	t.Cleanup(srv.Close)
	if err = lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: srv.URL, APIVersion: 1, Token: token}); err != nil {
		t.Fatal(err)
	}
	session, callCtx := compatibilitySession(t, root)
	call := func(tool string, args map[string]any) authorityReplayResponse {
		t.Helper()
		result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s: %v", tool, err)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var response authorityReplayResponse
		if json.Unmarshal(raw, &response) != nil || result.IsError != (response.Envelope.Error != nil) {
			t.Fatal("invalid MCP response")
		}
		return response
	}
	start := call("council_tasks_start", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "projectId": project.Project.ID, "originalText": "Fixture late settlement", "executionMode": "fake", "fakeScenario": "late_success", "workflow": map[string]any{"mode": "author_only"}, "acceptance": map[string]any{"deliverable": "response_only"}}})
	if start.Envelope.Error != nil {
		t.Fatal(start.Envelope.Error)
	}
	var admitted domain.Admission
	if json.Unmarshal(start.Envelope.Data, &admitted) != nil || !domain.ValidID(admitted.Attempt.ID) {
		t.Fatal("invalid admission")
	}
	select {
	case <-prepared:
	case <-time.After(8 * time.Second):
		t.Fatal("fixture did not reach pre-process barrier")
	}
	before, err := db.Attempt(ctx, admitted.Attempt.ID)
	if err != nil || before.Phase == "settled" {
		t.Fatalf("attempt was not held before process creation: %+v %v", before, err)
	}
	gate, err := db.Gate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimed := false
	for _, claim := range gate.ActiveClaims {
		if claim.AttemptID == admitted.Attempt.ID {
			claimed = true
		}
	}
	if !claimed {
		t.Fatal("fixture did not retain the attempt's capacity claim")
	}
	select {
	case <-release:
		t.Fatal("fixture released the process-preparation barrier early")
	default:
	}
	stopID := domain.NewID()
	stopArgs := map[string]any{"id": admitted.Attempt.ID, "input": map[string]any{"requestId": stopID}}
	accepted := call("council_attempts_stop", stopArgs)
	var response struct {
		StopAccepted bool `json:"stopAccepted"`
	}
	if accepted.Envelope.Error != nil || accepted.Envelope.Receipt == nil || json.Unmarshal(accepted.Envelope.Data, &response) != nil || !response.StopAccepted {
		t.Fatal("Stop was not durably accepted")
	}
	afterStop, err := db.Attempt(ctx, admitted.Attempt.ID)
	if err != nil || afterStop.Stop == nil || afterStop.Stop.RequestID != stopID || afterStop.Phase == "settled" {
		t.Fatal("Stop did not retain the claimed pre-launch attempt")
	}
	injected, err := db.SettleAttempt(ctx, admitted.Attempt.ID, domain.Settlement{Outcome: "succeeded", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"})
	if err != nil || injected.Outcome == nil || *injected.Outcome != "cancelled" || injected.Stop == nil || injected.Stop.RequestID != stopID {
		t.Fatalf("late fixture settlement overrode Stop: %+v %v", injected, err)
	}
	observed := call("council_attempts_get", map[string]any{"id": admitted.Attempt.ID})
	var final domain.Attempt
	if observed.Envelope.Error != nil || json.Unmarshal(observed.Envelope.Data, &final) != nil || final.Outcome == nil || *final.Outcome != "cancelled" || final.Stop == nil || final.Stop.RequestID != stopID || final.Cleanup.State != "verified" || final.EvidenceStatus != "incomplete" {
		t.Fatal("MCP did not preserve accepted Stop and controlled settlement")
	}
	replay := call("council_attempts_stop", stopArgs)
	if replay.Envelope.Error != nil || !reflect.DeepEqual(accepted.Envelope.Receipt, replay.Envelope.Receipt) {
		t.Fatal("Stop receipt changed after late settlement")
	}
	again, err := db.SettleAttempt(ctx, admitted.Attempt.ID, domain.Settlement{Outcome: "succeeded", NoProcessCreated: true, EffectsKnown: true, Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"})
	if err != nil || again.Outcome == nil || *again.Outcome != "cancelled" || again.Revision != injected.Revision {
		t.Fatal("duplicate late settlement changed result")
	}
	releaseBarrier()
}
