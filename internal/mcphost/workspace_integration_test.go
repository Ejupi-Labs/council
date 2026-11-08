package mcphost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type workspaceJourneyResponse struct {
	Status   int `json:"status"`
	Envelope struct {
		Data    json.RawMessage `json:"data"`
		Receipt *domain.Receipt `json:"receipt"`
		Error   *domain.Error   `json:"error"`
	} `json:"envelope"`
}

// The actual binary, SDK stdio client, owner, CLI and fake worker participate.
// All home, repository and process state is a disposable fixture.
func TestWorkspaceMCPAndCLISameProjectAndFakeTasks(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	project := filepath.Join(t.TempDir(), "project with spaces 東京")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "council")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/council")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture binary: %v: %s", err, output)
	}
	gitConfig := filepath.Join(home, "empty-gitconfig")
	if err := os.WriteFile(gitConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git setup: %v: %s", err, output)
		}
	}
	root := filepath.Join(home, ".council", "v2")
	var ownedID string
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil {
			if ownedID != "" {
				t.Error("fixture owner descriptor disappeared before cleanup")
			}
			return
		}
		// The fresh private root was created solely for this test, including a
		// partially successful bootstrap that failed before returning to CLI.
		if ownedID == "" {
			ownedID = desc.OwnerInstanceID
		}
		if !domain.ValidID(ownedID) || desc.OwnerInstanceID != ownedID {
			t.Error("fixture owner identity changed before cleanup")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "service", "stop", "--owner-instance", ownedID, "--request-id", domain.NewID(), "--json")
		cmd.Env, cmd.Dir = env, project
		if err := cmd.Run(); err != nil {
			t.Errorf("fixture owner stop failed: %v", err)
			return
		}
		for ctx.Err() == nil {
			lock, err := localstate.Acquire(root)
			if err == nil {
				_ = lock.Close()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Error("fixture owner did not release its lock")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cli := func(args ...string) domain.Envelope {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, append(args, "--json")...)
		cmd.Env, cmd.Dir = env, project
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var envelope domain.Envelope
		if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
			t.Fatalf("CLI returned invalid envelope: %v", decodeErr)
		}
		if err != nil || envelope.Error != nil {
			t.Fatalf("CLI %s failed: %v, error=%v", args[0], err, envelope.Error)
		}
		return envelope
	}
	decode := func(data any, out any) {
		t.Helper()
		encoded, err := json.Marshal(data)
		if err != nil || json.Unmarshal(encoded, out) != nil {
			t.Fatal("result did not match its contract")
		}
	}
	var stderr bytes.Buffer
	cmd := exec.Command(binary, "mcp")
	cmd.Env, cmd.Dir, cmd.Stderr = env, project, &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "council-workspace-journey", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("SDK stdio connection failed: %v", err)
	}
	defer session.Close()
	call := func(tool string, args map[string]any, out any) workspaceJourneyResponse {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s tool transport failed: %v", tool, err)
		}
		payload, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatalf("%s structured result invalid", tool)
		}
		var response workspaceJourneyResponse
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatalf("%s structured response invalid: %v", tool, err)
		}
		if result.IsError || response.Envelope.Error != nil || response.Status < 200 || response.Status >= 300 {
			t.Fatalf("%s owner failed: status=%d error=%v", tool, response.Status, response.Envelope.Error)
		}
		if out != nil && json.Unmarshal(response.Envelope.Data, out) != nil {
			t.Fatalf("%s data contract mismatch", tool)
		}
		return response
	}
	call("council_owner_meta", map[string]any{}, nil)
	desc, err := localstate.ReadDescriptor(root)
	if err != nil || !domain.ValidID(desc.OwnerInstanceID) || desc.Token == "" {
		t.Fatal("fixture owner descriptor missing")
	}
	ownedID = desc.OwnerInstanceID
	openedEnvelope := cli("projects", "open", project)
	var opened domain.ProjectOpen
	decode(openedEnvelope.Data, &opened)
	if !domain.ValidID(opened.Project.ID) {
		t.Fatal("CLI did not open a project")
	}
	var hostProject domain.ProjectGet
	call("council_projects_get", map[string]any{"id": opened.Project.ID}, &hostProject)
	if hostProject.Project.ID != opened.Project.ID {
		t.Fatal("host and CLI observed different projects")
	}
	const hostText = "\ufeffHost → CLI: keep original \"quotes\"\r\n東京 café 🚀\n"
	hostRequestID := domain.NewID()
	var hostAdmission domain.Admission
	hostResponse := call("council_tasks_start", map[string]any{"input": map[string]any{
		"requestId": hostRequestID, "projectId": opened.Project.ID, "originalText": hostText,
		"executionMode": "fake", "fakeScenario": "response", "acceptance": map[string]any{"deliverable": "response_only"},
	}}, &hostAdmission)
	if hostResponse.Envelope.Receipt == nil || hostResponse.Envelope.Receipt.RequestID != hostRequestID || hostAdmission.Attempt.ExecutionMode != "fake" || hostAdmission.Attempt.FakeScenario != "response" {
		t.Fatal("host did not retain the explicit fake admission and receipt")
	}
	var fromCLI domain.TaskDetail
	decode(cli("tasks", "get", hostAdmission.Task.ID).Data, &fromCLI)
	if fromCLI.Task.ID != hostAdmission.Task.ID || fromCLI.Task.ProjectID != opened.Project.ID || fromCLI.Task.OriginalText != hostText || fromCLI.LatestAttempt == nil || fromCLI.LatestAttempt.ID != hostAdmission.Attempt.ID {
		t.Fatal("CLI did not observe the exact host-created task and attempt")
	}
	waitCLI := func(attemptID string) domain.Attempt {
		t.Helper()
		for ctx.Err() == nil {
			var attempt domain.Attempt
			decode(cli("attempts", "get", attemptID).Data, &attempt)
			if attempt.Phase == "settled" {
				return attempt
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("fixture attempt did not settle")
		return domain.Attempt{}
	}
	hostAttempt := waitCLI(hostAdmission.Attempt.ID)
	if hostAttempt.Outcome == nil || *hostAttempt.Outcome != "succeeded" || hostAttempt.Cleanup.State != "verified" {
		t.Fatalf("host fake attempt did not succeed: %s", hostAttempt.Phase)
	}
	const cliText = "CLI → host: preserve UTF-8\r\nZürich 東京 🧪\n"
	cliRequestID := domain.NewID()
	createdEnvelope := cli("tasks", "start", "--project", opened.Project.ID, "--task", cliText, "--deliverable", "response_only", "--fake", "--fake-scenario", "response", "--request-id", cliRequestID)
	var cliAdmission domain.Admission
	decode(createdEnvelope.Data, &cliAdmission)
	if createdEnvelope.Receipt == nil || createdEnvelope.Receipt.RequestID != cliRequestID || cliAdmission.Attempt.ExecutionMode != "fake" || cliAdmission.Attempt.FakeScenario != "response" {
		t.Fatal("CLI did not retain the fake admission and receipt")
	}
	var fromHost domain.TaskDetail
	call("council_tasks_get", map[string]any{"taskId": cliAdmission.Task.ID}, &fromHost)
	if fromHost.Task.ID != cliAdmission.Task.ID || fromHost.Task.ProjectID != opened.Project.ID || fromHost.Task.OriginalText != cliText || fromHost.LatestAttempt == nil || fromHost.LatestAttempt.ID != cliAdmission.Attempt.ID {
		t.Fatal("host did not observe the exact CLI-created task and attempt")
	}
	cliAttempt := waitCLI(cliAdmission.Attempt.ID)
	var observed domain.Attempt
	call("council_attempts_get", map[string]any{"id": cliAdmission.Attempt.ID}, &observed)
	if observed.ID != cliAttempt.ID || observed.TaskID != cliAttempt.TaskID || observed.ExecutionMode != "fake" || observed.Outcome == nil || cliAttempt.Outcome == nil || *observed.Outcome != *cliAttempt.Outcome || observed.Cleanup.State != cliAttempt.Cleanup.State {
		t.Fatal("host and CLI observed different settled attempt facts")
	}
	if *cliAttempt.Outcome != "succeeded" || cliAttempt.Cleanup.State != "verified" {
		t.Fatal("CLI fake attempt did not settle successfully")
	}
	after, err := localstate.ReadDescriptor(root)
	if err != nil || after.OwnerInstanceID != ownedID || after.PID != desc.PID {
		t.Fatal("interfaces did not retain the same owner")
	}
	if err := session.Close(); err != nil {
		t.Fatal(fmt.Errorf("close SDK session: %w", err))
	}
	if bytes.Contains(stderr.Bytes(), []byte(desc.Token)) {
		t.Fatal("owner token leaked into host diagnostics")
	}
}
