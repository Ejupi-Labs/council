package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
)

func TestRoutingConcurrentIndependentAdmissionsShareDeclaredPoolAndOneWorker(t *testing.T) {
	f := newCLIFixture(t)
	var project struct {
		Project domain.Project `json:"project"`
	}
	decodeCLIData(t, f.run("projects", "open", f.repo), &project)
	var account domain.DeclaredAccount
	decodeCLIData(t, f.run("routing", "account", "add", "--label", "Shared declared fixture"), &account)
	// Personal Codex occupancy has no declaration binding to Claude capacity.
	// Both concurrent Claude tasks below share this declared account/context.
	for _, contextID := range []string{"claude-native"} {
		f.run("routing", "link", "--context", contextID, "--declared-account", account.ID, "--expected-revision", "1")
	}
	var accountCapacity domain.CapacityView
	decodeCLIData(t, f.run("routing", "capacity", "--account", account.ID), &accountCapacity)
	var positive domain.CapacityReportResult
	decodeCLIData(t, f.run("routing", "report", "--account", account.ID, "--expected-revision", accountCapacity.Revision, "--dimension", "occupancy", "--state", "available"), &positive)
	decodeCLIData(t, f.run("routing", "report", "--account", account.ID, "--expected-revision", positive.Capacity.Revision, "--dimension", "allowance", "--state", "available"), &positive)
	var codexCapacity domain.CapacityView
	decodeCLIData(t, f.run("routing", "capacity", "--context", "codex-native"), &codexCapacity)
	f.run("routing", "report", "--context", "codex-native", "--expected-revision", codexCapacity.Revision, "--dimension", "occupancy", "--state", "busy")
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	barrier := make(chan struct{})
	results := make(chan concurrentCLIResult, 2)
	requests := []string{domain.NewID(), domain.NewID()}
	for index := range 2 {
		args := []string{"tasks", "start", "--project", project.Project.ID, "--task", "Independent economical fake task", "--suitability", "routine", "--deliverable", "response_only", "--fake", "--fake-scenario", "slow", "--timeout-seconds", "60", "--request-id", requests[index]}
		go func(index int, args []string) { <-barrier; results <- concurrentCLICommand(ctx, f, index, args...) }(index, args)
	}
	close(barrier)
	var admissions [2]domain.Admission
	var processIDs [2]int
	for range 2 {
		select {
		case result := <-results:
			envelope := concurrentCLIEnvelope(t, result)
			decodeCLIData(t, envelope, &admissions[result.index])
			processIDs[result.index] = result.pid
			admission := admissions[result.index]
			if envelope.Receipt == nil || envelope.Receipt.RequestID != requests[result.index] || admission.RoutingDecision == nil || admission.Attempt.ResolvedTarget == nil || admission.Attempt.ResolvedTarget.ContextID != "claude-native" || admission.Attempt.ResolvedTarget.ModelSelector != "haiku" || admission.Attempt.ResolvedTarget.DeclaredAccountAssociation == nil || admission.Attempt.ResolvedTarget.DeclaredAccountAssociation.AccountID != account.ID {
				t.Fatalf("concurrent reported routing lost frozen target %+v", admission)
			}
		case <-ctx.Done():
			t.Fatal("independent routing admissions exceeded fixture bound")
		}
	}
	if processIDs[0] <= 0 || processIDs[0] == processIDs[1] || admissions[0].Attempt.ID == admissions[1].Attempt.ID || admissions[0].Attempt.OwnerInstanceID != admissions[1].Attempt.OwnerInstanceID {
		t.Fatal("distinct clients did not converge on one owner")
	}
	descriptor, err := localstate.ReadDescriptor(filepath.Join(f.home, ".council", "v2"))
	if err != nil || descriptor.OwnerInstanceID != admissions[0].Attempt.OwnerInstanceID {
		t.Fatal("continuing owner identity changed", err)
	}
	active, queued := -1, -1
	for active < 0 && ctx.Err() == nil {
		a := routingReadAttempt(t, ctx, f, admissions[0].Attempt.ID)
		b := routingReadAttempt(t, ctx, f, admissions[1].Attempt.ID)
		if a.LaunchIntent && b.LaunchIntent {
			t.Fatal("two launch intents escaped global managed resource")
		}
		if a.Phase == "running" && b.Phase == "queued" {
			active, queued = 0, 1
		} else if b.Phase == "running" && a.Phase == "queued" {
			active, queued = 1, 0
		}
		if active < 0 {
			select {
			case <-time.After(25 * time.Millisecond):
			case <-ctx.Done():
			}
		}
	}
	if active < 0 {
		t.Fatal("no running/queued pair within bounded fixture")
	}
	before := routingReadAttempt(t, ctx, f, admissions[queued].Attempt.ID)
	if before.LaunchIntent || before.SourceSnapshotID != nil || before.SourceSnapshot != nil {
		t.Fatal("queued attempt prepared source")
	}
	for _, event := range routingAttemptEvents(t, ctx, f, before.ID) {
		if event.Kind == "worker_started" || event.Kind == "preparation_started" {
			t.Fatal("queued attempt escaped worker limit")
		}
	}
	// Preference edits cannot rewrite the already-admitted selector or debit.
	var policy domain.RoutingPolicy
	decodeCLIData(t, f.run("routing", "policy"), &policy)
	f.run("routing", "policy", "set", "--expected-revision", policy.Revision, "--prefer-profile", "codex-sol-general")
	after := routingReadAttempt(t, ctx, f, before.ID)
	if after.ResolvedTarget == nil || after.ResolvedTarget.ContextID != before.ResolvedTarget.ContextID || after.ResolvedTarget.ModelSelector != before.ResolvedTarget.ModelSelector || after.RoutingDecisionID == nil || *after.RoutingDecisionID != *before.RoutingDecisionID {
		t.Fatal("preferences rewrote queued target")
	}
	f.run("attempts", "stop", admissions[active].Attempt.ID)
	first := routingWaitAttempt(t, ctx, f, admissions[active].Attempt.ID, "settled")
	if first.Outcome == nil || *first.Outcome != "cancelled" || first.Cleanup.State != "verified" {
		t.Fatalf("first cleanup unverified %+v", first)
	}
	second := routingWaitAttempt(t, ctx, f, before.ID, "running")
	if first.SettledAt == nil || second.StartedAt == nil {
		t.Fatal("missing retained nonoverlap times")
	}
	end, _ := time.Parse(time.RFC3339Nano, *first.SettledAt)
	start, _ := time.Parse(time.RFC3339Nano, *second.StartedAt)
	if start.Before(end) {
		t.Fatal("shared-pool native workers overlapped")
	}
	f.run("attempts", "stop", second.ID)
	second = routingWaitAttempt(t, ctx, f, second.ID, "settled")
	if second.Outcome == nil || *second.Outcome != "cancelled" || second.Cleanup.State != "verified" {
		t.Fatalf("second cleanup unverified %+v", second)
	}
	for _, admission := range admissions {
		workers := 0
		for _, event := range routingAttemptEvents(t, ctx, f, admission.Attempt.ID) {
			if event.Kind == "worker_started" {
				workers++
			}
		}
		if workers != 1 {
			t.Fatalf("attempt %s launched %d workers", admission.Attempt.ID, workers)
		}
	}
}
