package engine

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingContinuationInput(project string, enabled bool) domain.StartTaskInput {
	in := domain.StartTaskInput{RequestID: domain.NewID(), ProjectID: project, OriginalText: "CONTROLLED LOCAL TEST: perform the explicitly routine fixture task.", ExecutionMode: "fake", FakeScenario: "response", Acceptance: &domain.AcceptanceInput{Deliverable: "response_only"}, Routing: &domain.RoutingOptions{Suitability: "routine"}}
	if enabled {
		yes := true
		categories := []string{"pre_launch_ineligible"}
		in.Routing.Continuation = &domain.ContinuationInput{MaxAutomaticAdmissions: "1", Categories: &categories, AllowReplacement: &yes, AllowEscalation: &yes}
	}
	return in
}

func disableRoutingCodex(t *testing.T, e *Engine) {
	t.Helper()
	no := false
	r := e.db.UpdateRoutingContext(context.Background(), "codex-native", domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
	if r.Envelope.Error != nil {
		t.Fatal(r.Envelope.Error)
	}
}

func TestRoutingQualifiedFreshBlockCreatesOneOwnedSuccessorOnlyWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "default_off", true: "explicit_opt_in"}[enabled], func(t *testing.T) {
			e, project, _ := testEngine(t)
			stopFixtureScheduler(t, e)
			ctx := context.Background()
			first := decodeAdmission(t, e.StartTask(ctx, routingContinuationInput(project, enabled)))
			if first.Attempt.ConnectionID != "codex-native" {
				t.Fatal("fixture did not capture its initial economy target", first)
			}
			disableRoutingCodex(t, e)
			restartFixtureScheduler(e)
			old := settled(t, e, first.Attempt.ID)
			if old.Outcome == nil || *old.Outcome != "blocked" || old.LaunchIntent || old.StartedAt != nil || old.Cleanup.Scope != "not_launched" || old.PrimaryFailure == nil || old.PrimaryFailure.Code != "routing_unavailable" {
				t.Fatal("changed eligibility was not a proven pre-launch block", old)
			}
			if enabled {
				awaitFixtureCondition(t, "fresh qualified block did not produce its authorized successor", func() bool {
					detail, err := e.db.Task(ctx, first.Task.ID)
					return err == nil && detail.LatestAttempt != nil && detail.LatestAttempt.ID != old.ID
				})
				detail, err := e.db.Task(ctx, first.Task.ID)
				if err != nil {
					t.Fatal(err)
				}
				next := settled(t, e, detail.LatestAttempt.ID)
				if next.Outcome == nil || *next.Outcome != "succeeded" || next.ConnectionID != "claude-native" || next.PreviousAttemptID == nil || *next.PreviousAttemptID != old.ID || next.Cleanup.State != "verified" {
					t.Fatal("successor did not run once on its captured owned target", next)
				}
			}
			// Calling the fresh-event path twice cannot consume a predecessor twice;
			// with default OFF it also cannot manufacture permission.
			e.continueRouting(old.ID)
			attempts, err := e.db.Attempts(ctx, first.Task.ID, 100, "")
			want := 1
			if enabled {
				want = 2
			}
			if err != nil || len(attempts.Items) != want {
				t.Fatal("automatic work was duplicated or invented", attempts, err)
			}
			budget, err := e.db.TaskBudget(ctx, first.Task.ID)
			if err != nil || budget.Budget == nil || budget.Budget.ConsumedAdmissions != map[bool]string{false: "1", true: "2"}[enabled] || budget.Budget.AutomaticAdmissions != map[bool]string{false: "0", true: "1"}[enabled] {
				t.Fatal("the linked successor did not consume one cumulative unit", budget, err)
			}
			if budget.Budget.CurrentWindow.DeadlineAt == nil || first.Budget.CurrentWindow.DeadlineAt == nil || *budget.Budget.CurrentWindow.DeadlineAt != *first.Budget.CurrentWindow.DeadlineAt {
				t.Fatal("continuation reset the common task clock", budget)
			}
		})
	}
}

func TestRoutingStopBetweenSettlementAndContinuationPreservesTerminalAndPreventsSuccessor(t *testing.T) {
	e, project, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	first := decodeAdmission(t, e.StartTask(ctx, routingContinuationInput(project, true)))
	claimed, err := e.db.ClaimAttempt(ctx, first.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	disableRoutingCodex(t, e)
	e.run(ctx, claimed)
	old, err := e.db.Attempt(ctx, claimed.ID)
	if err != nil || old.Outcome == nil || *old.Outcome != "blocked" {
		t.Fatal(old, err)
	}
	requestID := domain.NewID()
	stop := e.Stop(ctx, claimed.ID, requestID)
	if stop.Envelope.Error != nil {
		t.Fatal(stop)
	}
	restartFixtureScheduler(e)
	e.continueRouting(claimed.ID)
	attempts, err := e.db.Attempts(ctx, first.Task.ID, 100, "")
	if err != nil || len(attempts.Items) != 1 {
		t.Fatal("an ordered user Stop was bypassed by automatic replacement", attempts, err)
	}
	current, err := e.db.Attempt(ctx, claimed.ID)
	if err != nil || current.Outcome == nil || *current.Outcome != *old.Outcome || current.PrimaryFailure.Code != old.PrimaryFailure.Code || current.Stop != nil {
		t.Fatal("terminal Stop suppression rewrote the prior outcome", current, err)
	}
	replayed := e.Stop(ctx, claimed.ID, requestID)
	if replayed.Envelope.Receipt == nil || *replayed.Envelope.Receipt != *stop.Envelope.Receipt {
		t.Fatal("the suppression control was not replayable", replayed)
	}
}

func TestRoutingNewOwnerCannotRearmRetainedPrelaunchProof(t *testing.T) {
	e, projectID, _ := testEngine(t)
	stopFixtureScheduler(t, e)
	ctx := context.Background()
	first := decodeAdmission(t, e.StartTask(ctx, routingContinuationInput(projectID, true)))
	claimed, err := e.db.ClaimAttempt(ctx, first.Attempt.ID, e.config.OwnerInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	disableRoutingCodex(t, e)
	e.run(ctx, claimed)
	old, err := e.db.Attempt(ctx, claimed.ID)
	if err != nil || old.Outcome == nil || *old.Outcome != "blocked" || old.LaunchIntent {
		t.Fatal("fixture did not retain its fresh no-process proof", old, err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	replacement, err := New(e.db, e.lock, Config{OwnerInstanceID: domain.NewID()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	// Even a direct invocation of the fresh-event path cannot re-arm old-epoch
	// proof. Production recovery never scans these settled historical rows.
	replacement.continueRouting(old.ID)
	attempts, err := e.db.Attempts(ctx, first.Task.ID, 100, "")
	if err != nil || len(attempts.Items) != 1 {
		t.Fatal("owner recovery replayed a retained failure", attempts, err)
	}
	budget, err := e.db.TaskBudget(ctx, first.Task.ID)
	if err != nil || budget.Budget == nil || budget.Budget.ConsumedAdmissions != "1" || budget.Budget.AutomaticAdmissions != "0" {
		t.Fatal("recovery created new work authority", budget, err)
	}
}
