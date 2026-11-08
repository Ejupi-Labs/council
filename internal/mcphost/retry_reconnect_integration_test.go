package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
)

// Capture only correlation IDs, not request bodies or owner credentials.
type reconnectTrace struct {
	io.WriteCloser
	mu      sync.Mutex
	pending []byte
	ids     []string
}

func (w *reconnectTrace) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.WriteCloser.Write(p)
	w.pending = append(w.pending, p[:n]...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		var msg struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
		}
		if json.Unmarshal(w.pending[:end], &msg) == nil && msg.Method == "tools/call" {
			w.ids = append(w.ids, string(msg.ID))
		}
		w.pending = append(w.pending[:0], w.pending[end+1:]...)
	}
	return n, err
}

func (w *reconnectTrace) callIDs() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.ids...)
}

type reconnectHost struct {
	session *mcp.ClientSession
	cmd     *exec.Cmd
	trace   *reconnectTrace
	once    sync.Once
	err     error
}

func (h *reconnectHost) close() error {
	h.once.Do(func() {
		_ = h.trace.Close()
		finished := make(chan error, 1)
		go func() { _ = h.session.Close(); finished <- h.cmd.Wait() }()
		select {
		case h.err = <-finished:
		case <-time.After(8 * time.Second):
			_ = h.cmd.Process.Kill()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
			}
			h.err = errors.New("MCP fixture did not exit after EOF")
		}
	})
	return h.err
}

type reconnectPayload struct {
	Status   int `json:"status"`
	Envelope struct {
		RequestID string          `json:"requestId"`
		Data      json.RawMessage `json:"data"`
		Receipt   *domain.Receipt `json:"receipt"`
		Error     *domain.Error   `json:"error"`
	} `json:"envelope"`
}

// Exercise real subprocess hosts, SDK framing, HTTP owner/controller and SQLite.
// The test seam drops or withholds only the first acknowledgement after commit;
// fixture Prepare waits for cancellation and never launches a provider process.
func TestMCPReconnectAfterCommittedTaskAckLossAndCancellation(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture build: %v %s", err, out)
	}
	for _, mode := range []string{"lost_ack", "cancel_after_commit"} {
		t.Run(mode, func(t *testing.T) {
			home, projectDir := t.TempDir(), t.TempDir()
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
			controller, err := engine.New(db, lock, engine.Config{OwnerInstanceID: ownerID, Prepare: func(ctx context.Context, _ process.NativeSpec) (*process.Owned, error) {
				<-ctx.Done()
				return nil, errors.New("fixture prevents native launch")
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := controller.Close(); err != nil {
					t.Errorf("fixture controller close: %v", err)
				}
			})
			service := app.New(db)
			service.Execution = controller
			config := filepath.Join(t.TempDir(), "empty-gitconfig")
			if err := os.WriteFile(config, nil, 0600); err != nil {
				t.Fatal(err)
			}
			env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "GIT_CONFIG_GLOBAL="+config, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
			for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
				cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...)
				cmd.Env = env
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("fixture Git: %v %s", err, out)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
			defer cancel()
			openBody, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "selectedPath": projectDir, "basePath": projectDir})
			opened := service.Execute(ctx, "projects.open", app.Params{Body: openBody})
			project, ok := opened.Envelope.Data.(domain.ProjectOpen)
			if !ok || opened.Envelope.Error != nil {
				t.Fatal("fixture project failed")
			}
			policy, err := db.Policy(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if policy.Mode != "autonomous" {
				body, _ := json.Marshal(map[string]any{"requestId": domain.NewID(), "expectedRevision": policy.Revision, "mode": "autonomous"})
				if result := service.Execute(ctx, "policy.set", app.Params{Body: body}); result.Envelope.Error != nil {
					t.Fatal("fixture policy failed")
				}
			}
			token := strings.Repeat("r", 64)
			var calls atomic.Int32
			var retainedMu sync.Mutex
			var retained []byte
			committed, abandoned := make(chan struct{}), make(chan struct{})
			server := httptest.NewUnstartedServer(nil)
			actual := httpapi.New(service, httpapi.Config{Host: server.Listener.Addr().String(), Token: token, OwnerInstanceID: ownerID})
			server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && r.URL.Path == "/api/v1/tasks" && calls.Add(1) == 1 {
					recorder := httptest.NewRecorder()
					actual.ServeHTTP(recorder, r)
					if recorder.Code != 201 {
						t.Errorf("fixture task commit returned %d", recorder.Code)
					}
					retainedMu.Lock()
					retained = append([]byte(nil), recorder.Body.Bytes()...)
					retainedMu.Unlock()
					close(committed)
					if mode == "cancel_after_commit" {
						select {
						case <-r.Context().Done():
						case <-time.After(15 * time.Second):
							t.Error("cancel did not reach HTTP request")
						}
						close(abandoned)
						return
					}
					hijacker, ok := w.(http.Hijacker)
					if !ok {
						t.Error("cannot drop fixture acknowledgement")
						close(abandoned)
						return
					}
					conn, _, err := hijacker.Hijack()
					if err != nil {
						t.Error("cannot hijack fixture connection")
						close(abandoned)
						return
					}
					_ = conn.Close()
					close(abandoned)
					return
				}
				actual.ServeHTTP(w, r)
			})
			server.Start()
			t.Cleanup(server.Close)
			if err := lock.WriteDescriptor(localstate.Descriptor{OwnerInstanceID: ownerID, PID: os.Getpid(), StartedAt: domain.Now(), Origin: server.URL, APIVersion: 1, Token: token}); err != nil {
				t.Fatal(err)
			}
			connect := func() *reconnectHost {
				t.Helper()
				cmd := exec.Command(binary, "mcp")
				cmd.Env, cmd.Dir, cmd.Stderr = env, projectDir, io.Discard
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				trace := &reconnectTrace{WriteCloser: stdin}
				client := mcp.NewClient(&mcp.Implementation{Name: "retry-fixture", Version: "1"}, nil)
				session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdout, Writer: trace}, nil)
				if err != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					t.Fatalf("MCP connect: %v", err)
				}
				h := &reconnectHost{session: session, cmd: cmd, trace: trace}
				t.Cleanup(func() {
					if err := h.close(); err != nil {
						t.Errorf("fixture host close: %v", err)
					}
				})
				return h
			}
			requestID := domain.NewID()
			input := map[string]any{"requestId": requestID, "projectId": project.Project.ID, "originalText": "Retain the exact fake task on retry", "executionMode": "fake", "fakeScenario": "response", "workflow": map[string]any{"mode": "author_only"}, "acceptance": map[string]any{"deliverable": "response_only"}}
			args := map[string]any{"input": input}
			first := connect()
			type callResult struct {
				result *mcp.CallToolResult
				err    error
			}
			firstCtx, cancelFirst := context.WithCancel(ctx)
			firstResult := make(chan callResult, 1)
			go func() {
				result, err := first.session.CallTool(firstCtx, &mcp.CallToolParams{Name: "council_tasks_start", Arguments: args})
				firstResult <- callResult{result, err}
			}()
			select {
			case <-committed:
			case <-ctx.Done():
				t.Fatal("owner did not commit")
			}
			if mode == "cancel_after_commit" {
				cancelFirst()
			}
			select {
			case result := <-firstResult:
				if mode == "lost_ack" {
					if result.err != nil || result.result == nil || !result.result.IsError {
						t.Fatal("lost ack not reported as uncertain")
					}
					raw, _ := json.Marshal(result.result)
					if !bytes.Contains(raw, []byte("write_uncertain")) || !bytes.Contains(raw, []byte(requestID)) {
						t.Fatal("uncertain result lost request identity")
					}
				} else if !errors.Is(result.err, context.Canceled) {
					t.Fatalf("mutation cancellation: %v", result.err)
				}
			case <-ctx.Done():
				t.Fatal("first call did not finish")
			}
			cancelFirst()
			select {
			case <-abandoned:
			case <-ctx.Done():
				t.Fatal("owner request was not abandoned")
			}
			if err := first.close(); err != nil {
				t.Fatalf("close first host: %v", err)
			}
			firstIDs := first.trace.callIDs()
			if len(firstIDs) != 1 {
				t.Fatalf("first host calls: %d", len(firstIDs))
			}
			retainedMu.Lock()
			original := append([]byte(nil), retained...)
			retainedMu.Unlock()
			var originalEnvelope domain.Envelope
			if json.Unmarshal(original, &originalEnvelope) != nil || originalEnvelope.Receipt == nil || originalEnvelope.RequestID != requestID {
				t.Fatal("owner did not retain original receipt")
			}
			var admission domain.Admission
			rawData, _ := json.Marshal(originalEnvelope.Data)
			if json.Unmarshal(rawData, &admission) != nil || !domain.ValidID(admission.Task.ID) || !domain.ValidID(admission.Attempt.ID) {
				t.Fatal("owner admission missing identities")
			}
			second := connect()
			if _, err := second.session.CallTool(ctx, &mcp.CallToolParams{Name: "council_owner_meta", Arguments: map[string]any{}}); err != nil {
				t.Fatal("reconnected host metadata failed")
			}
			start := make(chan struct{})
			outcomes := make(chan callResult, 2)
			for range 2 {
				go func() {
					<-start
					result, err := second.session.CallTool(ctx, &mcp.CallToolParams{Name: "council_tasks_start", Arguments: args})
					outcomes <- callResult{result, err}
				}()
			}
			close(start)
			for range 2 {
				result := <-outcomes
				if result.err != nil || result.result == nil || result.result.IsError {
					t.Fatalf("concurrent replay failed: %v", result.err)
				}
				raw, err := json.Marshal(result.result.StructuredContent)
				if err != nil {
					t.Fatal(err)
				}
				var payload reconnectPayload
				if json.Unmarshal(raw, &payload) != nil || payload.Envelope.Error != nil || payload.Envelope.RequestID != requestID || !reflect.DeepEqual(payload.Envelope.Receipt, originalEnvelope.Receipt) {
					t.Fatal("replay receipt changed")
				}
				var actualAdmission domain.Admission
				if json.Unmarshal(payload.Envelope.Data, &actualAdmission) != nil || actualAdmission.Task.ID != admission.Task.ID || actualAdmission.Attempt.ID != admission.Attempt.ID {
					t.Fatal("replay admission identity changed")
				}
			}
			ids := second.trace.callIDs()
			if len(ids) != 3 || ids[1] == ids[2] || ids[1] == firstIDs[0] || ids[2] == firstIDs[0] {
				t.Fatal("correlation IDs were not distinct across retries")
			}
			changed := map[string]any{"input": map[string]any{"requestId": requestID, "projectId": project.Project.ID, "originalText": "Changed task", "executionMode": "fake", "fakeScenario": "response", "workflow": map[string]any{"mode": "author_only"}, "acceptance": map[string]any{"deliverable": "response_only"}}}
			conflict, err := second.session.CallTool(ctx, &mcp.CallToolParams{Name: "council_tasks_start", Arguments: changed})
			encoded, _ := json.Marshal(conflict)
			if err != nil || conflict == nil || !conflict.IsError || !bytes.Contains(encoded, []byte("request_conflict")) {
				t.Fatal("changed intent did not conflict")
			}
			tasks, err := db.Tasks(ctx, project.Project.ID, 10, "")
			if err != nil || len(tasks.Items) != 1 || tasks.Items[0].ID != admission.Task.ID {
				t.Fatal("retry created extra tasks")
			}
			attempts, err := db.Attempts(ctx, admission.Task.ID, 10, "")
			if err != nil || len(attempts.Items) != 1 || attempts.Items[0].ID != admission.Attempt.ID {
				t.Fatal("retry created extra attempts")
			}
			if calls.Load() != 4 {
				t.Fatalf("unexpected HTTP task mutation count %d", calls.Load())
			}
			cmd := exec.CommandContext(ctx, binary, "tasks", "start", "--project", project.Project.ID, "--task", "Retain the exact fake task on retry", "--fake", "--fake-scenario", "response", "--workflow", "author_only", "--deliverable", "response_only", "--request-id", requestID, "--json")
			cmd.Env, cmd.Dir = env, projectDir
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("CLI exact retry failed: %v", err)
			}
			var cliEnvelope domain.Envelope
			if json.Unmarshal(out, &cliEnvelope) != nil || !reflect.DeepEqual(cliEnvelope.Receipt, originalEnvelope.Receipt) {
				t.Fatal("CLI did not retain original owner receipt")
			}
			cliData, _ := json.Marshal(cliEnvelope.Data)
			var cliAdmission domain.Admission
			if json.Unmarshal(cliData, &cliAdmission) != nil || cliAdmission.Task.ID != admission.Task.ID || cliAdmission.Attempt.ID != admission.Attempt.ID {
				t.Fatal("CLI returned different admission")
			}
			if err := second.close(); err != nil {
				t.Fatalf("close second host: %v", err)
			}
		})
	}
}
