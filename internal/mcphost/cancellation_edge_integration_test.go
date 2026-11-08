package mcphost

import (
	"context"
	"encoding/base64"
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

// Each case gets a distinct actual owner and private state. Every launched
// worker is the fixed fake entrypoint of the binary built for this test.
func TestMCPCancellationAndPartialOutcomeEdges(t *testing.T) {
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
		t.Fatalf("build fake fixture: %v %s", err, out)
	}
	for _, scenario := range []string{"slow", "late_success", "partial_failure", "cleanup_unknown"} {
		t.Run(scenario, func(t *testing.T) {
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
			ownerID := domain.NewID()
			e, err := engine.New(db, lock, engine.Config{OwnerInstanceID: ownerID, Prepare: func(ctx context.Context, spec process.NativeSpec) (*process.Owned, error) {
				spec.Executable = binary
				return process.Prepare(ctx, spec)
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				// A retained incident is not an Engine.Close error. Any returned
				// error indicates an actual drain, handle, or epoch-storage failure.
				if err := e.Close(); err != nil {
					t.Errorf("fixture owner close: %v", err)
				}
			})
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
			openBody, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "selectedPath": projectDir, "basePath": projectDir})
			opened := service.Execute(ctx, "projects.open", app.Params{Body: openBody})
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
			token := strings.Repeat("a", 64)
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
				res, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: tool, Arguments: args})
				if err != nil || res == nil {
					t.Fatalf("%s: %v", tool, err)
				}
				raw, _ := json.Marshal(res.StructuredContent)
				var out authorityReplayResponse
				if json.Unmarshal(raw, &out) != nil {
					t.Fatalf("%s malformed result", tool)
				}
				if res.IsError != (out.Envelope.Error != nil) {
					t.Fatal("MCP error indicator mismatch")
				}
				return out
			}
			acceptance := "response_only"
			if scenario == "partial_failure" {
				acceptance = "candidate"
			}
			req := domain.NewID()
			start := map[string]any{"input": map[string]any{"requestId": req, "projectId": project.Project.ID, "originalText": "Fixture task for " + scenario, "executionMode": "fake", "fakeScenario": scenario, "acceptance": map[string]any{"deliverable": acceptance}}}
			result := call("council_tasks_start", start)
			if result.Envelope.Error != nil {
				t.Fatal(result.Envelope.Error)
			}
			var admitted domain.Admission
			if json.Unmarshal(result.Envelope.Data, &admitted) != nil || !domain.ValidID(admitted.Attempt.ID) {
				t.Fatal("missing fake admission")
			}
			get := func() domain.Attempt {
				t.Helper()
				r := call("council_attempts_get", map[string]any{"id": admitted.Attempt.ID})
				if r.Envelope.Error != nil {
					t.Fatal(r.Envelope.Error)
				}
				var a domain.Attempt
				if json.Unmarshal(r.Envelope.Data, &a) != nil {
					t.Fatal("attempt shape")
				}
				return a
			}
			waitFor := func(predicate func(domain.Attempt) bool) domain.Attempt {
				t.Helper()
				deadline := time.NewTimer(10 * time.Second)
				defer deadline.Stop()
				for {
					a := get()
					if predicate(a) {
						return a
					}
					select {
					case <-deadline.C:
						t.Fatalf("attempt did not reach state: phase=%s outcome=%v", a.Phase, a.Outcome)
					case <-callCtx.Done():
						t.Fatal("attempt observation cancelled")
					case <-time.After(25 * time.Millisecond):
					}
				}
			}
			if scenario == "slow" || scenario == "late_success" {
				observed := waitFor(func(a domain.Attempt) bool {
					return a.Phase == "running" || (scenario == "late_success" && a.Phase == "settled")
				})
				if observed.Stop != nil {
					t.Fatal("fixture already had a Stop")
				}
				if observed.Phase == "running" && observed.Outcome != nil {
					t.Fatal("running fixture already has an outcome")
				}
				if scenario == "slow" {
					observed := call("council_attempts_wait", map[string]any{"id": admitted.Attempt.ID, "afterRevision": "9223372036854775807", "timeoutMs": "0"})
					var wait struct {
						Attempt  domain.Attempt `json:"attempt"`
						Changed  bool           `json:"changed"`
						TimedOut bool           `json:"timedOut"`
					}
					if observed.Envelope.Error != nil || json.Unmarshal(observed.Envelope.Data, &wait) != nil || !wait.TimedOut || wait.Changed || wait.Attempt.Outcome != nil || wait.Attempt.Stop != nil {
						t.Fatal("zero wait did not return a nonmutating timeout")
					}
					detail := call("council_tasks_get", map[string]any{"taskId": admitted.Task.ID})
					var current domain.TaskDetail
					if detail.Envelope.Error != nil || json.Unmarshal(detail.Envelope.Data, &current) != nil {
						t.Fatal("missing current task authority")
					}
					pause := call("council_tasks_pause", map[string]any{"taskId": admitted.Task.ID, "input": map[string]any{"requestId": domain.NewID(), "taskVersion": current.Task.CurrentVersion, "expectedTaskRevision": current.Task.Revision}})
					var checkpoint domain.RecoveryPauseResult
					if pause.Envelope.Error != nil || json.Unmarshal(pause.Envelope.Data, &checkpoint) != nil || checkpoint.Checkpoint.State != "pausing" {
						t.Fatal("pause failed to represent in-flight drain")
					}
					found := false
					for _, id := range checkpoint.Checkpoint.AdmittedAttemptIDs {
						if id == admitted.Attempt.ID {
							found = true
						}
					}
					if !found {
						t.Fatal("draining attempt omitted from checkpoint")
					}
				}
				stopID := domain.NewID()
				stop := call("council_attempts_stop", map[string]any{"id": admitted.Attempt.ID, "input": map[string]any{"requestId": stopID}})
				var stopData struct {
					StopAccepted bool   `json:"stopAccepted"`
					Reason       string `json:"reason"`
				}
				if stop.Envelope.Error != nil || stop.Envelope.Receipt == nil || json.Unmarshal(stop.Envelope.Data, &stopData) != nil {
					t.Fatal("explicit Stop returned an invalid result")
				}
				settled := waitFor(func(a domain.Attempt) bool { return a.Phase == "settled" })
				if !stopData.StopAccepted {
					// The bounded fake may finish while CI deschedules this
					// test. In that case no Stop was accepted; require the
					// truthful already-settled result and original success.
					if scenario != "late_success" || stopData.Reason != "already_settled" || settled.Outcome == nil || *settled.Outcome != "succeeded" || settled.Stop != nil || settled.Cleanup.State != "verified" {
						t.Fatalf("unexpected nonaccepted Stop: %+v attempt %+v", stopData, settled)
					}
					return
				}
				if settled.Outcome == nil || *settled.Outcome != "cancelled" || settled.Stop == nil || settled.Stop.RequestID != stopID || settled.Cleanup.State != "verified" {
					t.Fatalf("accepted Stop authority or cleanup lost: %+v", settled)
				}
				return
			}
			settled := waitFor(func(a domain.Attempt) bool { return a.Phase == "settled" })
			if scenario == "partial_failure" {
				if settled.Outcome == nil || *settled.Outcome != "failed" || settled.PrimaryFailure == nil || settled.CandidateID == nil || !settled.AuthoredDelta || settled.Cleanup.State != "verified" {
					t.Fatalf("partial failure was obscured: %+v", settled)
				}
				listed := call("council_attempts_artifacts", map[string]any{"id": settled.ID})
				if listed.Envelope.Error != nil {
					t.Fatal(listed.Envelope.Error)
				}
				var page domain.Page[domain.ArtifactMetadata]
				if json.Unmarshal(listed.Envelope.Data, &page) != nil {
					t.Fatal("artifact list shape")
				}
				var stdout string
				for _, item := range page.Items {
					if item.Kind == "stdout" {
						stdout = item.ID
						break
					}
				}
				if stdout == "" {
					t.Fatal("partial stdout evidence missing")
				}
				bytesResult := call("council_artifacts_bytes", map[string]any{"id": stdout, "offset": "0"})
				var chunk struct {
					BytesBase64 string `json:"bytesBase64"`
				}
				if bytesResult.Envelope.Error != nil || json.Unmarshal(bytesResult.Envelope.Data, &chunk) != nil {
					t.Fatal("partial evidence unreadable")
				}
				raw, err := base64.StdEncoding.DecodeString(chunk.BytesBase64)
				if err != nil || !strings.Contains(string(raw), "turn.failed") {
					t.Fatal("failed terminal evidence missing")
				}
				return
			}
			if settled.Outcome == nil || *settled.Outcome == "succeeded" || settled.Cleanup.State != "unknown" || settled.Cleanup.IncidentID == nil || settled.ResponseArtifactID != nil {
				t.Fatalf("unknown cleanup implied success: %+v", settled)
			}
			gate, err := db.Gate(ctx)
			if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != *settled.Cleanup.IncidentID {
				t.Fatal("unknown cleanup did not retain exact fence")
			}
			next := call("council_tasks_start", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "projectId": project.Project.ID, "originalText": "Do not dispatch", "executionMode": "fake", "fakeScenario": "response"}})
			if next.Envelope.Error == nil || next.Envelope.Error.Code != "dispatch_fenced" {
				t.Fatal("unknown cleanup allowed new work")
			}
		})
	}
}
