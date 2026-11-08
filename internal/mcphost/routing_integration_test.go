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

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The stdio SDK, native binary, owner and CLI share a disposable project and
// private home. All task execution is explicitly fake; no provider is called.
func TestMCPRoutingCapacityBudgetAndUnknownUsageAcrossCLI(t *testing.T) {
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
	for _, args := range [][]string{{"init", "-b", "fixture"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git setup: %v: %s", err, output)
		}
	}
	// The seed helper builds only this disposable accepted-schema-3 fixture,
	// verifies the migrated owner fence and never-launched provenance under its
	// lock, then records the test-only clean epoch. It never reads user state.
	seedOutput := filepath.Join(home, "schema3-identities.json")
	seedArgs := []string{"test", "-p", "1", "./internal/store", "-run", "^TestRoutingSchema3SeedChild$", "-count=1"}
	seedExecutable := "go"
	if existing := os.Getenv("COUNCIL_ROUTING_STORE_TEST_BINARY"); existing != "" {
		seedExecutable = existing
		seedArgs = []string{"-test.run=^TestRoutingSchema3SeedChild$", "-test.count=1"}
	}
	seed := exec.Command(seedExecutable, seedArgs...)
	seed.Dir = repository
	seed.Env = append(env, "COUNCIL_ROUTING_SCHEMA3_SEED_PATH="+filepath.Join(home, ".council", "v2", "workspace.sqlite3"), "COUNCIL_ROUTING_SCHEMA3_PROJECT="+project, "COUNCIL_ROUTING_SCHEMA3_SEED_OUTPUT="+seedOutput)
	if seedExecutable == "go" {
		// HOME is private fixture state, but a Go module cache contains
		// read-only module trees that Unix TempDir cleanup cannot remove.
		// Reuse the caller's existing external cache without moving it.
		cacheCmd := exec.Command("go", "env", "GOMODCACHE")
		cacheCmd.Dir = repository
		cacheOutput, cacheErr := cacheCmd.Output()
		cache := strings.TrimSpace(string(cacheOutput))
		info, statErr := os.Stat(cache)
		rel, relErr := filepath.Rel(home, cache)
		insideHome := relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
		if cacheErr != nil || !filepath.IsAbs(cache) || statErr != nil || !info.IsDir() || insideHome {
			t.Fatal("Go seed requires an existing module cache outside the fixture home")
		}
		seed.Env = append(seed.Env, "GOMODCACHE="+cache)
	}
	if output, err := seed.CombinedOutput(); err != nil {
		t.Fatalf("isolated legacy routing seed: %v: %s", err, output)
	}
	var legacy struct {
		ProjectID        string `json:"projectId"`
		TaskID           string `json:"taskId"`
		AttemptID        string `json:"attemptId"`
		ExpectedRevision string `json:"expectedRevision"`
		OriginalText     string `json:"originalText"`
	}
	seedBytes, err := os.ReadFile(seedOutput)
	if err != nil || json.Unmarshal(seedBytes, &legacy) != nil || !domain.ValidID(legacy.TaskID) || !domain.ValidID(legacy.AttemptID) {
		t.Fatal("invalid isolated legacy seed", err)
	}
	root := filepath.Join(home, ".council", "v2")
	var ownerID string
	t.Cleanup(func() {
		desc, err := localstate.ReadDescriptor(root)
		if err != nil {
			if ownerID != "" {
				t.Error("fixture owner descriptor disappeared before cleanup")
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.Command(binary, "mcp")
	cmd.Env, cmd.Dir = env, project
	client := mcp.NewClient(&mcp.Implementation{Name: "council-routing-journey", Version: "1"}, nil)
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
			t.Fatalf("%s owner status=%d error=%v", name, out.Status, out.Envelope.Error)
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
			t.Fatalf("CLI %s failed: %v, error=%v", args[0], err, envelope.Error)
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
	var opened domain.ProjectOpen
	call("council_projects_open", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "selectedPath": project, "basePath": project}}, &opened)

	var policy domain.RoutingPolicy
	call("council_routing_policy_get", map[string]any{}, &policy)
	policyID := domain.NewID()
	var updated domain.RoutingPolicy
	result := call("council_routing_policy_update", map[string]any{"input": map[string]any{"requestId": policyID, "expectedRevision": policy.Revision, "unknownAvailability": "allow", "defaultBudget": map[string]any{"admissionLimit": "2", "windowSeconds": 3600}}}, &updated)
	if result.Envelope.Receipt == nil || result.Envelope.Receipt.RequestID != policyID || updated.DefaultBudget.AdmissionLimit != "2" {
		t.Fatal("routing policy update was not retained")
	}
	var cliPolicy domain.RoutingPolicy
	cli(&cliPolicy, "routing", "policy")
	if !reflect.DeepEqual(updated, cliPolicy) {
		t.Fatal("CLI and host disagree on routing policy")
	}

	var contexts domain.Page[domain.ContextView]
	call("council_routing_contexts_list", map[string]any{}, &contexts)
	var codex, claude domain.ContextView
	for _, c := range contexts.Items {
		if c.ID == "codex-native" {
			codex = c
		}
		if c.ID == "claude-native" {
			claude = c
		}
	}
	if codex.ID == "" || claude.ID == "" {
		t.Fatal("native routing contexts missing")
	}
	var changedContext domain.NativeContext
	call("council_routing_context_update", map[string]any{"id": codex.ID, "input": map[string]any{"requestId": domain.NewID(), "expectedRevision": codex.Revision, "label": "Fixture Codex"}}, &changedContext)
	if changedContext.Label != "Fixture Codex" {
		t.Fatal("context update missing")
	}
	var cliContexts domain.Page[domain.ContextView]
	cli(&cliContexts, "routing", "contexts")
	matchedContext := false
	for _, c := range cliContexts.Items {
		if c.ID == codex.ID && c.Label == changedContext.Label && c.Revision == changedContext.Revision {
			matchedContext = true
		}
	}
	if !matchedContext {
		t.Fatal("CLI did not observe updated context")
	}

	var profiles domain.Page[domain.TargetProfile]
	call("council_routing_profiles_list", map[string]any{"contextId": codex.ID}, &profiles)
	var profile domain.TargetProfile
	for _, p := range profiles.Items {
		if p.ID == "codex-native-default" {
			profile = p
		}
	}
	if profile.ID == "" {
		t.Fatal("default Codex profile missing")
	}
	var changedProfile domain.TargetProfile
	call("council_routing_profile_update", map[string]any{"id": profile.ID, "input": map[string]any{"requestId": domain.NewID(), "expectedRevision": profile.Revision, "referencePrice": map[string]any{"currency": "USD", "inputPerMillion": "1.25", "outputPerMillion": "2.5", "sourceLabel": "Fixture reference", "observedAt": "2020-01-01T00:00:00Z"}}}, &changedProfile)
	if changedProfile.ReferencePrice == nil || changedProfile.ReferencePrice.SourceKind != "user_reference" {
		t.Fatal("reference price was not retained as a reference")
	}
	var cliProfiles domain.Page[domain.TargetProfile]
	cli(&cliProfiles, "routing", "profiles", "--context", codex.ID)
	matchedProfile := false
	for _, p := range cliProfiles.Items {
		if p.ID == changedProfile.ID && reflect.DeepEqual(p, changedProfile) {
			matchedProfile = true
		}
	}
	if !matchedProfile {
		t.Fatal("CLI did not observe updated profile")
	}

	var account domain.DeclaredAccount
	call("council_routing_accounts_create", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "label": "Fixture account", "entitlement": "unknown"}}, &account)
	if !domain.ValidID(account.ID) || account.Entitlement != "unknown" {
		t.Fatal("account declaration did not retain unknown entitlement")
	}
	var link domain.AccountLink
	call("council_routing_links_update", map[string]any{"id": claude.ID, "input": map[string]any{"requestId": domain.NewID(), "expectedRevision": claude.AccountLink.Revision, "declaredAccountId": account.ID}}, &link)
	if link.DeclaredAccountID == nil || *link.DeclaredAccountID != account.ID {
		t.Fatal("declared account was not linked")
	}
	var cliAccounts domain.Page[domain.DeclaredAccount]
	cli(&cliAccounts, "routing", "accounts")
	if len(cliAccounts.Items) != 1 || !reflect.DeepEqual(cliAccounts.Items[0], account) {
		t.Fatal("CLI did not observe declared account")
	}
	var before domain.CapacityView
	call("council_routing_capacity_get", map[string]any{"accountId": account.ID}, &before)
	var reported domain.CapacityReportResult
	call("council_routing_reports_create", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "subject": map[string]any{"kind": "account", "id": account.ID}, "expectedRevision": before.Revision, "dimension": "allowance", "state": "exhausted"}}, &reported)
	if reported.Report.Subject.ID != account.ID || reported.Report.State != "exhausted" || reported.Capacity.ReportEligibility.Permitted {
		t.Fatal("capacity report or blocker missing")
	}
	var cliReport domain.CapacityReportDetail
	cli(&cliReport, "routing", "report", "get", reported.Report.ID)
	if !reflect.DeepEqual(cliReport.Report, reported.Report) {
		t.Fatal("CLI did not observe the same immutable report")
	}
	var linkedCapacity domain.CapacityView
	call("council_routing_capacity_get", map[string]any{"contextId": claude.ID}, &linkedCapacity)
	if linkedCapacity.AccountLink == nil || linkedCapacity.AccountLink.DeclaredAccountID == nil || *linkedCapacity.AccountLink.DeclaredAccountID != account.ID || linkedCapacity.ReportEligibility.Permitted {
		t.Fatal("context did not inherit the reported account blocker")
	}
	var cliCapacity domain.CapacityView
	cli(&cliCapacity, "routing", "capacity", "--context", claude.ID)
	if cliCapacity.Revision != linkedCapacity.Revision || cliCapacity.ReportEligibility.Permitted != linkedCapacity.ReportEligibility.Permitted {
		t.Fatal("CLI and host disagree on linked capacity")
	}

	// Adoption acts on the exact historical task and never starts work.
	var prior domain.TaskDetail
	call("council_tasks_get", map[string]any{"taskId": legacy.TaskID}, &prior)
	var priorBudget domain.TaskBudgetView
	call("council_tasks_budget_get", map[string]any{"taskId": legacy.TaskID}, &priorBudget)
	if prior.Task.ProjectID != legacy.ProjectID || prior.Task.OriginalText != legacy.OriginalText || prior.Task.Revision != legacy.ExpectedRevision || prior.LatestAttempt == nil || prior.LatestAttempt.ID != legacy.AttemptID || !priorBudget.AdoptionRequired || priorBudget.Budget != nil {
		t.Fatal("the migrated task did not retain its historical identity and adoption requirement")
	}
	adoptID := domain.NewID()
	var adopted domain.BudgetAmendmentResult
	adoptReceipt := call("council_tasks_budget_amend", map[string]any{"taskId": legacy.TaskID, "input": map[string]any{"requestId": adoptID, "action": "adopt", "expectedRevision": prior.Task.Revision, "additionalAdmissions": "2", "newWindowSeconds": 1800}}, &adopted)
	if adoptReceipt.Envelope.Receipt == nil || adoptReceipt.Envelope.Receipt.RequestID != adoptID || adopted.Version == nil || adopted.Version.Version != "2" || adopted.Amendment.Action != "adopt" || adopted.Budget.HistoricalAdmissions != "1" || adopted.Budget.ConsumedAdmissions != "1" || adopted.Budget.GrantedAdmissions != "3" || adopted.Budget.RemainingAdmissions != "2" {
		t.Fatal("legacy adoption lost historical consumption, version, or exact receipt")
	}
	var legacyAfter domain.TaskDetail
	call("council_tasks_get", map[string]any{"taskId": legacy.TaskID}, &legacyAfter)
	var cliAdopted domain.TaskBudgetView
	cli(&cliAdopted, "tasks", "budget", legacy.TaskID)
	if legacyAfter.LatestAttempt == nil || legacyAfter.LatestAttempt.ID != legacy.AttemptID || legacyAfter.Task.OriginalText != legacy.OriginalText || cliAdopted.AdoptionRequired || cliAdopted.Budget == nil || !reflect.DeepEqual(*cliAdopted.Budget, adopted.Budget) {
		t.Fatal("adoption started work, changed the instructions, or differs across interfaces")
	}

	var admission domain.Admission
	call("council_tasks_start", map[string]any{"input": map[string]any{"requestId": domain.NewID(), "projectId": opened.Project.ID, "originalText": "Routing fake-only fixture", "executionMode": "fake", "fakeScenario": "response", "acceptance": map[string]any{"deliverable": "response_only"}, "workflow": map[string]any{"mode": "author_only"}, "routing": map[string]any{"suitability": "routine", "targetProfileId": profile.ID, "budget": map[string]any{"admissionLimit": "2", "windowSeconds": 3600}}}}, &admission)
	if admission.Attempt.ExecutionMode != "fake" || admission.Budget == nil || admission.Budget.ConsumedAdmissions != "1" {
		t.Fatal("fake task did not receive finite routing authority")
	}
	var attempt domain.Attempt
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		call("council_attempts_get", map[string]any{"id": admission.Attempt.ID}, &attempt)
		if attempt.Phase == "settled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if attempt.Phase != "settled" || attempt.Outcome == nil || *attempt.Outcome != "succeeded" {
		t.Fatal("fake task did not settle successfully")
	}
	var detail domain.TaskDetail
	call("council_tasks_get", map[string]any{"taskId": admission.Task.ID}, &detail)
	var budget domain.TaskBudgetView
	call("council_tasks_budget_get", map[string]any{"taskId": admission.Task.ID}, &budget)
	if budget.Budget == nil {
		t.Fatal("retained budget missing")
	}
	grantID := domain.NewID()
	var grant domain.BudgetAmendmentResult
	grantResult := call("council_tasks_budget_amend", map[string]any{"taskId": admission.Task.ID, "input": map[string]any{"requestId": grantID, "action": "grant", "expectedRevision": detail.Task.Revision, "expectedBudgetRevision": budget.Budget.Revision, "additionalAdmissions": "1"}}, &grant)
	if grantResult.Envelope.Receipt == nil || grantResult.Envelope.Receipt.RequestID != grantID || grant.Budget.ConsumedAdmissions != "1" || grant.Budget.RemainingAdmissions != "2" {
		t.Fatal("budget grant rewrote prior consumption or lost receipt")
	}
	var cliBudget domain.TaskBudgetView
	cli(&cliBudget, "tasks", "budget", admission.Task.ID)
	if cliBudget.Budget == nil || !reflect.DeepEqual(*cliBudget.Budget, grant.Budget) {
		t.Fatal("CLI and host disagree on granted budget")
	}
	var usage domain.UsagePage
	call("council_tasks_usage", map[string]any{"taskId": admission.Task.ID}, &usage)
	if len(usage.Items) != 1 || len(usage.Coverage.ExcludedFakeAttemptIDs) != 1 || usage.Coverage.ExcludedFakeAttemptIDs[0] != admission.Attempt.ID || usage.Charges.Status != "unknown" || usage.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != nil {
		t.Fatal("fake or unmeasured usage was presented as a known charge or token count")
	}
	var cliUsage domain.UsagePage
	cli(&cliUsage, "tasks", "usage", admission.Task.ID)
	if !reflect.DeepEqual(usage, cliUsage) {
		t.Fatal("CLI and host disagree on retained unknown usage")
	}
	after, err := localstate.ReadDescriptor(root)
	if err != nil || after.OwnerInstanceID != ownerID || after.PID != desc.PID {
		t.Fatal("interfaces did not share one continuing owner")
	}
	if err := session.Close(); err != nil {
		t.Fatalf("SDK close: %v", err)
	}
}
