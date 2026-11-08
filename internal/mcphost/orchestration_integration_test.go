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
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/fixture"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Real SDK and CLI share one real owner; all workers and fixtures are local.
func TestMCPWorkflowAndDependentGraphAcrossCLI(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home, project := t.TempDir(), t.TempDir()
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
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "GIT_CONFIG_GLOBAL="+gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	if err := os.MkdirAll(filepath.Join(project, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("CONTROLLED LOCAL WORKFLOW TEST DATA\r\nold\r\n東京\r\n")
	if err := os.WriteFile(filepath.Join(project, "docs", "note.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"add", "docs/note.md"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git setup: %v: %s", err, output)
		}
	}
	root := filepath.Join(home, ".council", "v2")
	var releaseTemps []string
	// Registered before owner cleanup, so failed fixtures stop their owner
	// and children before attempting to remove any still-linked temporary.
	t.Cleanup(func() {
		for _, temporary := range releaseTemps {
			if err := os.Remove(temporary); err != nil && !os.IsNotExist(err) {
				t.Errorf("controlled release temporary cleanup: %v", err)
			}
		}
	})
	var ownerID string
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil {
			if ownerID != "" {
				t.Error("fixture owner descriptor disappeared")
			}
			return
		}
		if ownerID == "" {
			ownerID = desc.OwnerInstanceID
		}
		if !domain.ValidID(ownerID) || desc.OwnerInstanceID != ownerID {
			t.Error("fixture owner changed before cleanup")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "service", "stop", "--owner-instance", ownerID, "--request-id", domain.NewID(), "--json")
		cmd.Env, cmd.Dir = env, project
		if err := cmd.Run(); err != nil {
			t.Errorf("fixture owner stop: %v", err)
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
		t.Error("fixture owner did not release lock")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.Command(binary, "mcp")
	cmd.Env, cmd.Dir = env, project
	client := mcp.NewClient(&mcp.Implementation{Name: "council-orchestration-journey", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("SDK stdio connection: %v", err)
	}
	defer session.Close()
	type response struct {
		Status   int `json:"status"`
		Envelope struct {
			Data    json.RawMessage `json:"data"`
			Receipt *domain.Receipt `json:"receipt"`
			Error   *domain.Error   `json:"error"`
		} `json:"envelope"`
	}
	call := func(name string, args map[string]any, target any) response {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result == nil {
			t.Fatalf("%s transport: %v", name, err)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal("invalid structured result")
		}
		var out response
		if json.Unmarshal(raw, &out) != nil || result.IsError || out.Envelope.Error != nil || out.Status < 200 || out.Status >= 300 {
			t.Fatalf("%s status=%d error=%v", name, out.Status, out.Envelope.Error)
		}
		if target != nil && json.Unmarshal(out.Envelope.Data, target) != nil {
			t.Fatalf("%s data shape", name)
		}
		return out
	}
	cli := func(target any, args ...string) domain.Envelope {
		t.Helper()
		command := exec.CommandContext(ctx, binary, append(args, "--json")...)
		command.Env, command.Dir = env, project
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		var envelope domain.Envelope
		if json.Unmarshal(stdout.Bytes(), &envelope) != nil || err != nil || envelope.Error != nil {
			t.Fatalf("CLI %s failed: %v error=%v", args[0], err, envelope.Error)
		}
		if target != nil {
			raw, err := json.Marshal(envelope.Data)
			if err != nil || json.Unmarshal(raw, target) != nil {
				t.Fatalf("CLI %s data shape", args[0])
			}
		}
		return envelope
	}
	call("council_owner_meta", map[string]any{}, nil)
	desc, err := localstate.ReadDescriptor(root)
	if err != nil || !domain.ValidID(desc.OwnerInstanceID) {
		t.Fatal("fixture owner missing")
	}
	ownerID = desc.OwnerInstanceID
	var projectOpen domain.ProjectOpen
	cli(&projectOpen, "projects", "open", project)

	request := domain.NewID()
	var admission domain.Admission
	result := call("council_tasks_start", map[string]any{"input": map[string]any{
		"requestId": request, "projectId": projectOpen.Project.ID,
		"originalText":  "Make " + fixture.WorkflowFile + " contain exactly Council controlled workflow validation: VALID followed by CRLF. Inspect and correct the actual bytes.",
		"executionMode": "fake", "fakeScenario": "workflow-defect",
		"workflow": map[string]any{"review": "required", "checks": []any{map[string]any{"key": "controlled-validation", "executable": binary, "arguments": []string{"__fixture", "check", "--case", "workflow-validation"}}}},
		"routing":  map[string]any{"budget": map[string]any{"admissionLimit": "2"}},
	}}, &admission)
	if result.Envelope.Receipt == nil || result.Envelope.Receipt.RequestID != request || admission.Workflow == nil || admission.Workflow.Authority.ExecutionMode != "fake" {
		t.Fatal("workflow authority or fake admission missing")
	}
	workflowID := admission.Workflow.ID
	checkID := ""
	for _, check := range admission.Workflow.Authority.Checks {
		if check.Key == "controlled-validation" && check.Required && check.Kind == "command" {
			checkID = check.ID
		}
	}
	if checkID == "" {
		t.Fatal("required controlled check was not captured")
	}
	var workflow domain.WorkflowDetail
	var checks domain.Page[domain.CheckReceipt]
	var detail domain.TaskDetail
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		call("council_tasks_workflow_get", map[string]any{"taskId": admission.Task.ID}, &workflow)
		if workflow.Workflow != nil && workflow.Workflow.State == "blocked" && workflow.Workflow.NextAction == "budget_exhausted" {
			call("council_tasks_workflow_checks", map[string]any{"taskId": admission.Task.ID}, &checks)
			call("council_tasks_get", map[string]any{"taskId": admission.Task.ID}, &detail)
			if len(checks.Items) == 1 && detail.LatestAttempt != nil && detail.LatestAttempt.ID == checks.Items[0].AttemptID && detail.LatestAttempt.Phase == "settled" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if workflow.Workflow == nil || workflow.Workflow.State != "blocked" || workflow.Workflow.NextAction != "budget_exhausted" {
		t.Fatal("workflow did not retain its budget blocker")
	}
	call("council_tasks_workflow_checks", map[string]any{"taskId": admission.Task.ID}, &checks)
	if len(checks.Items) != 1 || checks.Items[0].DefinitionID != checkID || checks.Items[0].Outcome != "failed" || checks.Items[0].ExitCode == nil || *checks.Items[0].ExitCode != 1 || !checks.Items[0].RootTerminal || checks.Items[0].Cleanup.State != "verified" {
		t.Fatal("actual failed check evidence missing")
	}
	var reviews domain.Page[domain.ReviewRecord]
	call("council_tasks_workflow_reviews", map[string]any{"taskId": admission.Task.ID}, &reviews)
	if len(reviews.Items) != 1 || reviews.Items[0].Outcome != "changes_required" || len(reviews.Items[0].Findings) == 0 || len(reviews.Items[0].Report.InspectedEvidence) == 0 {
		t.Fatal("blocking review evidence missing")
	}
	firstReview, err := json.Marshal(reviews.Items[0])
	if err != nil {
		t.Fatal("could not capture original review")
	}
	var cliReviews domain.Page[domain.ReviewRecord]
	cli(&cliReviews, "tasks", "reviews", admission.Task.ID)
	if !reflect.DeepEqual(cliReviews.Items, reviews.Items) {
		t.Fatal("CLI and host observed different review records")
	}
	call("council_tasks_get", map[string]any{"taskId": admission.Task.ID}, &detail)
	var budget domain.TaskBudgetView
	call("council_tasks_budget_get", map[string]any{"taskId": admission.Task.ID}, &budget)
	if budget.Budget == nil {
		t.Fatal("workflow budget missing")
	}
	grantID := domain.NewID()
	cli(nil, "tasks", "budget", "grant", admission.Task.ID, "--expected-revision", detail.Task.Revision, "--expected-budget-revision", budget.Budget.Revision, "--additional-admissions", "2", "--request-id", grantID)
	call("council_tasks_get", map[string]any{"taskId": admission.Task.ID}, &detail)
	call("council_tasks_workflow_get", map[string]any{"taskId": admission.Task.ID}, &workflow)
	if workflow.Workflow == nil || workflow.Workflow.ID != workflowID || workflow.Workflow.State != "blocked" {
		t.Fatal("grant unexpectedly changed workflow authority")
	}
	advanceID := domain.NewID()
	advanced := call("council_tasks_workflow_advance", map[string]any{"taskId": admission.Task.ID, "input": map[string]any{"requestId": advanceID, "expectedRevision": detail.Task.Revision, "workflowRevision": workflow.Workflow.Revision, "taskVersion": detail.Task.CurrentVersion}}, nil)
	if advanced.Envelope.Receipt == nil || advanced.Envelope.Receipt.RequestID != advanceID {
		t.Fatal("explicit workflow advance receipt missing")
	}
	deadline = time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		call("council_tasks_workflow_get", map[string]any{"taskId": admission.Task.ID}, &workflow)
		if workflow.Workflow != nil && workflow.Workflow.State == "completed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if workflow.Workflow == nil || workflow.Workflow.State != "completed" || workflow.Workflow.ID != workflowID {
		t.Fatal("workflow did not complete under its captured identity")
	}
	call("council_tasks_workflow_reviews", map[string]any{"taskId": admission.Task.ID}, &reviews)
	var retainedReview []byte
	if len(reviews.Items) > 0 {
		retainedReview, err = json.Marshal(reviews.Items[0])
	}
	if err != nil || len(reviews.Items) != 2 || !bytes.Equal(retainedReview, firstReview) || reviews.Items[1].Outcome != "approved" || reviews.Items[1].CandidateBinding == nil {
		t.Fatal("workflow review lineage changed or final review absent")
	}
	var cliWorkflow domain.WorkflowDetail
	cli(&cliWorkflow, "tasks", "workflow", admission.Task.ID)
	if cliWorkflow.Workflow == nil || cliWorkflow.Workflow.ID != workflowID || cliWorkflow.Workflow.State != "completed" {
		t.Fatal("CLI did not observe completed workflow")
	}
	if workflow.Acceptance == nil || workflow.Workflow.Authority.Delivery.LocalMode != "review_only" || workflow.Acceptance.DeliveryResult != "review_only" || workflow.Acceptance.DeliveryOperationID != nil || workflow.Workflow.SelectedCandidateID == nil || !reflect.DeepEqual(cliWorkflow.Acceptance, workflow.Acceptance) {
		t.Fatal("completed workflow did not retain the same review-only acceptance across interfaces")
	}
	var candidates, cliCandidates domain.Page[domain.Candidate]
	call("council_tasks_workflow_candidates", map[string]any{"taskId": admission.Task.ID}, &candidates)
	cli(&cliCandidates, "tasks", "candidates", admission.Task.ID)
	if len(candidates.Items) < 2 || candidates.NextCursor != nil || !reflect.DeepEqual(candidates.Items, cliCandidates.Items) || reviews.Items[0].CandidateBinding == nil {
		t.Fatalf("candidate records differ or lack review bindings: MCP count=%d CLI count=%d", len(candidates.Items), len(cliCandidates.Items))
	}
	var selected, predecessor *domain.Candidate
	for i := range candidates.Items {
		candidate := &candidates.Items[i]
		if candidate.ID == *workflow.Workflow.SelectedCandidateID {
			selected = candidate
		} else if candidate.ID == reviews.Items[0].CandidateBinding.CandidateID {
			predecessor = candidate
		}
	}
	if selected == nil || predecessor == nil || selected.WorkflowID != workflowID || selected.PredecessorCandidateID == nil || *selected.PredecessorCandidateID != predecessor.ID || selected.ID != reviews.Items[1].CandidateBinding.CandidateID || selected.ID != workflow.Acceptance.CandidateBinding.CandidateID || !selected.HasDelta {
		t.Fatal("selected candidate lost its actual correction, review, or acceptance lineage")
	}
	var deliveries, cliDeliveries domain.Page[domain.WorkflowDeliveryView]
	call("council_tasks_workflow_deliveries", map[string]any{"taskId": admission.Task.ID}, &deliveries)
	cli(&cliDeliveries, "tasks", "deliveries", admission.Task.ID)
	if len(deliveries.Items) != 1 || deliveries.NextCursor != nil || !reflect.DeepEqual(deliveries.Items, cliDeliveries.Items) {
		t.Fatal("MCP and CLI did not observe the same review-only delivery record")
	}
	delivery := deliveries.Items[0]
	if delivery.Kind != "review_only" || delivery.Acceptance == nil || delivery.ID != workflow.Acceptance.ID || !reflect.DeepEqual(delivery.Acceptance, workflow.Acceptance) || delivery.Operation != nil || delivery.External != nil || delivery.StepID != nil || delivery.AttemptID != nil {
		t.Fatal("review-only acceptance was presented as a local application or external delivery")
	}
	unchanged, err := os.ReadFile(filepath.Join(project, "docs", "note.md"))
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("review-only workflow changed the original project")
	}
	if _, err := os.Stat(filepath.Join(project, fixture.WorkflowFile)); !os.IsNotExist(err) {
		t.Fatal("review-only candidate was unexpectedly applied to the project", err)
	}

	graphRequest := domain.NewID()
	var graph domain.GraphAdmission
	graphResult := call("council_graphs_start", map[string]any{"input": map[string]any{
		"requestId": graphRequest, "projectId": projectOpen.Project.ID, "executionMode": "fake",
		"routing":      map[string]any{"budget": map[string]any{"admissionLimit": "2"}},
		"nodes":        []any{map[string]any{"key": "producer", "originalText": "Return the controlled predecessor response.", "fakeScenario": "graph-response", "acceptance": map[string]any{"deliverable": "response_only"}}, map[string]any{"key": "receiver", "originalText": "Use the explicitly accepted predecessor as context.", "fakeScenario": "graph-response", "acceptance": map[string]any{"deliverable": "response_only"}}},
		"dependencies": []any{map[string]any{"from": "producer", "to": "receiver", "kind": "context"}},
	}}, &graph)
	if graphResult.Envelope.Receipt == nil || graphResult.Envelope.Receipt.RequestID != graphRequest || !domain.ValidID(graph.Graph.ID) {
		t.Fatal("graph admission receipt missing")
	}
	var graphView domain.GraphDetail
	var producer, receiver domain.GraphNodeSummary
	deadline = time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		call("council_graphs_get", map[string]any{"graphId": graph.Graph.ID}, &graphView)
		for _, n := range graphView.Nodes {
			if n.Key == "producer" {
				producer = n
			}
			if n.Key == "receiver" {
				receiver = n
			}
		}
		if producer.LatestAttemptID != nil {
			var attempt domain.Attempt
			call("council_attempts_get", map[string]any{"id": *producer.LatestAttemptID}, &attempt)
			if attempt.Phase == "settled" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if producer.LatestAttemptID == nil || receiver.LatestAttemptID != nil || receiver.InputID != nil {
		t.Fatal("dependent node ran before predecessor acceptance")
	}
	var cliGraph domain.GraphDetail
	cli(&cliGraph, "graphs", "get", graph.Graph.ID)
	if cliGraph.Graph.ID != graph.Graph.ID || len(cliGraph.Nodes) != 2 {
		t.Fatal("CLI did not observe same dependent graph")
	}
	accept := func(node domain.GraphNodeSummary) domain.ResponseAcceptance {
		t.Helper()
		if node.LatestAttemptID == nil {
			t.Fatal("node attempt missing")
		}
		var task domain.TaskDetail
		call("council_tasks_get", map[string]any{"taskId": node.TaskID}, &task)
		var attempt domain.Attempt
		call("council_attempts_get", map[string]any{"id": *node.LatestAttemptID}, &attempt)
		if attempt.Phase != "settled" || attempt.Outcome == nil || *attempt.Outcome != "succeeded" || attempt.ExecutionMode != "fake" || attempt.ResponseArtifactID == nil {
			t.Fatal("fake graph response missing")
		}
		var artifact domain.ArtifactMetadata
		call("council_artifacts_get", map[string]any{"id": *attempt.ResponseArtifactID}, &artifact)
		if artifact.SHA256 == nil || !artifact.Closed {
			t.Fatal("response evidence is not closed and hashed")
		}
		var result struct {
			Task       domain.TaskSummary        `json:"task"`
			Acceptance domain.ResponseAcceptance `json:"acceptance"`
		}
		requestID := domain.NewID()
		envelope := cli(&result, "tasks", "accept-response", node.TaskID, "--expected-revision", task.Task.Revision, "--task-version", task.Task.CurrentVersion, "--attempt", attempt.ID, "--response", artifact.ID, "--response-sha256", *artifact.SHA256, "--reviewed", "--request-id", requestID)
		accepted := result.Acceptance
		if envelope.Receipt == nil || envelope.Receipt.RequestID != requestID || result.Task.ID != node.TaskID || accepted.RequestID != requestID || accepted.TaskID != node.TaskID || accepted.AttemptID != attempt.ID || accepted.ResponseArtifactID != artifact.ID || accepted.ResponseSHA256 != *artifact.SHA256 {
			t.Fatal("CLI accepted a different response")
		}
		return accepted
	}
	acceptedProducer := accept(producer)
	deadline = time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		call("council_graphs_get", map[string]any{"graphId": graph.Graph.ID}, &graphView)
		for _, n := range graphView.Nodes {
			if n.Key == "receiver" {
				receiver = n
			}
		}
		if receiver.InputID != nil && receiver.LatestAttemptID != nil {
			var attempt domain.Attempt
			call("council_attempts_get", map[string]any{"id": *receiver.LatestAttemptID}, &attempt)
			if attempt.Phase == "settled" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if receiver.InputID == nil || receiver.LatestAttemptID == nil {
		t.Fatal("accepted predecessor did not prepare dependent node")
	}
	var graphInput domain.GraphInputDetail
	call("council_graphs_inputs_get", map[string]any{"graphId": graph.Graph.ID, "inputId": *receiver.InputID}, &graphInput)
	if graphInput.InputBinding == nil || len(graphInput.InputBinding.Contributions) != 1 || graphInput.InputBinding.Contributions[0].AcceptanceID != acceptedProducer.ID || graphInput.InputBinding.Contributions[0].ProducerAttemptID != acceptedProducer.AttemptID {
		t.Fatal("dependent input did not bind the exact accepted predecessor")
	}
	accept(receiver)
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		call("council_graphs_get", map[string]any{"graphId": graph.Graph.ID}, &graphView)
		if graphView.Graph.State == "completed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if graphView.Graph.State != "completed" {
		t.Fatal("accepted dependent graph did not complete")
	}
	cli(&cliGraph, "graphs", "get", graph.Graph.ID)
	if cliGraph.Graph.ID != graph.Graph.ID || cliGraph.Graph.State != "completed" {
		t.Fatal("CLI did not observe completed dependent graph")
	}
	// Two different native contexts share one declared account and contend for
	// its single managed reservation. Workers are real, controlled fake children.
	var contexts domain.Page[domain.ContextView]
	call("council_routing_contexts_list", map[string]any{}, &contexts)
	var account domain.DeclaredAccount
	call("council_routing_accounts_create", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "label": "Shared account", "entitlement": "unknown"}}, &account)
	linked := 0
	for _, native := range contexts.Items {
		if native.ID != "codex-native" && native.ID != "claude-native" {
			continue
		}
		var link domain.AccountLink
		call("council_routing_links_update", map[string]any{"id": native.ID, "input": map[string]any{"requestId": domain.NewID(), "expectedRevision": native.AccountLink.Revision, "declaredAccountId": account.ID}}, &link)
		if link.DeclaredAccountID == nil || *link.DeclaredAccountID != account.ID {
			t.Fatal("shared account link missing")
		}
		linked++
	}
	if linked != 2 {
		t.Fatal("expected both controlled native contexts")
	}
	var siblings domain.GraphAdmission
	call("council_graphs_start", map[string]any{"input": map[string]any{
		"requestId": domain.NewID(), "projectId": projectOpen.Project.ID, "executionMode": "fake", "maxParallelism": "2",
		"routing": map[string]any{"budget": map[string]any{"admissionLimit": "4"}},
		"nodes": []any{
			map[string]any{"key": "a", "originalText": "Hold the controlled sibling barrier.", "pins": map[string]any{"harnessId": "codex"}, "fakeScenario": "graph-wait", "workflow": map[string]any{"review": "required"}},
			map[string]any{"key": "b", "originalText": "Hold the other controlled sibling barrier.", "pins": map[string]any{"harnessId": "claude-code"}, "fakeScenario": "graph-wait", "workflow": map[string]any{"review": "required"}},
		},
	}}, &siblings)
	if siblings.Graph.MaxParallelism != "2" {
		t.Fatal("fixture must allow both graph slots")
	}
	var siblingView domain.GraphDetail
	var first, second domain.Attempt
	var firstNode, waitingNode domain.GraphNodeSummary
	var workspaceIDs []string
	released := map[string]bool{}
	t.Cleanup(func() {
		for _, id := range workspaceIDs {
			if domain.ValidID(id) && !released[id] {
				_ = os.WriteFile(filepath.Join(root, "workspaces", id, fixture.GraphReleaseFile), []byte("release\n"), 0600)
			}
		}
	})
	release := func(id string) {
		t.Helper()
		if !domain.ValidID(id) || released[id] {
			t.Fatal("invalid or repeated controlled release")
		}
		workspace := filepath.Join(root, "workspaces", id)
		temporary := filepath.Join(root, "workspaces", "."+fixture.GraphReleaseFile+"-"+domain.NewID()+".tmp")
		file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("controlled release", err)
		}
		// Keep the closed fixture-owned hardlink outside the snapshot until
		// both children have settled; Windows may deny deletion while a child
		// has the final name open.
		releaseTemps = append(releaseTemps, temporary)
		_, writeErr := file.WriteString("release\n")
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			t.Fatal("controlled release write", writeErr, syncErr, closeErr)
		}
		if err := os.Link(temporary, filepath.Join(workspace, fixture.GraphReleaseFile)); err != nil {
			t.Fatal("controlled release publication", err)
		}
		released[id] = true
	}
	ready := func(a domain.Attempt) bool {
		if a.Phase != "running" || a.SourceSnapshotID == nil || !domain.ValidID(*a.SourceSnapshotID) {
			return false
		}
		body, err := os.ReadFile(filepath.Join(root, "workspaces", *a.SourceSnapshotID, fixture.GraphReadyFile))
		return err == nil && string(body) == "ready\n"
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		call("council_graphs_get", map[string]any{"graphId": siblings.Graph.ID}, &siblingView)
		if len(siblingView.ActiveClaims) == 1 && siblingView.Budget.Budget.ConsumedAdmissions == "1" {
			claim := siblingView.ActiveClaims[0]
			for _, node := range siblingView.Nodes {
				if claim.NodeID != nil && node.ID == *claim.NodeID {
					firstNode = node
				} else {
					waitingNode = node
				}
			}
			if firstNode.LatestAttemptID != nil && waitingNode.ID != "" && waitingNode.LatestAttemptID == nil && claim.CapturedDeclaredAccountID != nil && *claim.CapturedDeclaredAccountID == account.ID {
				call("council_attempts_get", map[string]any{"id": *firstNode.LatestAttemptID}, &first)
				if ready(first) {
					workspaceIDs = append(workspaceIDs, *first.SourceSnapshotID)
					break
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(workspaceIDs) != 1 {
		t.Fatal("first worker never held the shared account")
	}
	waitObserved := false
	until := time.Now().Add(350 * time.Millisecond)
	for time.Now().Before(until) {
		call("council_graphs_get", map[string]any{"graphId": siblings.Graph.ID}, &siblingView)
		if len(siblingView.ActiveClaims) != 1 || siblingView.ActiveClaims[0].AttemptID != first.ID || siblingView.Budget.Budget.ConsumedAdmissions != "1" {
			t.Fatal("sibling was admitted while account was held")
		}
		for _, blocker := range siblingView.Blockers {
			if blocker.NodeID != nil && *blocker.NodeID == waitingNode.ID && (blocker.Code == "capacity_wait" || blocker.Code == "attempt_active") {
				waitObserved = true
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !waitObserved {
		t.Fatal("waiting sibling has no actual capacity blocker")
	}
	release(workspaceIDs[0])
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		call("council_graphs_get", map[string]any{"graphId": siblings.Graph.ID}, &siblingView)
		for _, node := range siblingView.Nodes {
			if node.ID == waitingNode.ID && node.LatestAttemptID != nil {
				call("council_attempts_get", map[string]any{"id": *node.LatestAttemptID}, &second)
				if ready(second) {
					workspaceIDs = append(workspaceIDs, *second.SourceSnapshotID)
				}
			}
		}
		if len(workspaceIDs) == 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(workspaceIDs) != 2 || second.ID == first.ID || second.ConnectionID == first.ConnectionID || second.ResolvedTarget == nil || second.ResolvedTarget.DeclaredAccountAssociation == nil || second.ResolvedTarget.DeclaredAccountAssociation.AccountID != account.ID {
		t.Fatal("other captured context did not wake after account release")
	}
	var settledFirst domain.Attempt
	call("council_attempts_get", map[string]any{"id": first.ID}, &settledFirst)
	if settledFirst.Phase != "settled" || settledFirst.SettledAt == nil || second.StartedAt == nil {
		t.Fatal("first sibling did not settle before second started")
	}
	ended, endErr := time.Parse(time.RFC3339Nano, *settledFirst.SettledAt)
	started, startErr := time.Parse(time.RFC3339Nano, *second.StartedAt)
	if endErr != nil || startErr != nil || started.Before(ended) {
		t.Fatal("sibling execution intervals overlap")
	}
	release(workspaceIDs[1])
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		call("council_attempts_get", map[string]any{"id": second.ID}, &second)
		if second.Phase == "settled" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if second.Phase != "settled" {
		t.Fatal("second worker did not settle after release")
	}
	for _, temporary := range releaseTemps {
		if err := os.Remove(temporary); err != nil {
			t.Fatal("controlled release temporary cleanup", err)
		}
	}
	var cliSiblings domain.GraphDetail
	cli(&cliSiblings, "graphs", "get", siblings.Graph.ID)
	if cliSiblings.Graph.ID != siblings.Graph.ID || (cliSiblings.Budget.Budget.ConsumedAdmissions == "0" || cliSiblings.Budget.Budget.ConsumedAdmissions == "1") {
		t.Fatal("CLI did not observe serialized sibling admissions")
	}

	after, err := localstate.ReadDescriptor(root)
	if err != nil || after.OwnerInstanceID != ownerID || after.PID != desc.PID {
		t.Fatal("interfaces did not share the same owner")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("SDK close: %v", err)
	}
}
