package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// This tests actual concurrent SQLite-backed claims using fake admission
// metadata. No native process or provider is started by this store fixture.
func TestConcurrentClaimsShareDeclaredAccount(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner := domain.NewID()
	project := openFixture(t, s, "concurrent-shared-account")
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	account := routingTestAccount(t, s)
	routingTestLink(t, s, "codex-native", account.ID, false)
	routingTestLink(t, s, "claude-native", account.ID, false)

	admit := func(harness string) domain.Attempt {
		t.Helper()
		input := taskInput(project.ID)
		input.Pins = &domain.ExecutionPins{HarnessID: harness}
		result := s.CreateTask(ctx, owner, input)
		if result.Status != 201 || result.Envelope.Error != nil {
			t.Fatal("controlled task admission failed", result)
		}
		attempt := result.Envelope.Data.(domain.Admission).Attempt
		if attempt.ExecutionMode != "fake" || attempt.ResolvedTarget == nil || attempt.ResolvedTarget.DeclaredAccountAssociation == nil || attempt.ResolvedTarget.DeclaredAccountAssociation.AccountID != account.ID {
			t.Fatal("admission did not capture the same declared fake account", attempt)
		}
		return attempt
	}
	first, second := admit("codex"), admit("claude-code")
	if first.ConnectionID == second.ConnectionID {
		t.Fatal("fixture did not use two distinct native contexts")
	}

	type claimResult struct {
		id      string
		attempt domain.Attempt
		err     error
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for _, attempt := range []domain.Attempt{first, second} {
		go func(id string) {
			ready <- struct{}{}
			<-start
			claimed, err := s.ClaimAttempt(ctx, id, owner)
			results <- claimResult{id: id, attempt: claimed, err: err}
		}(attempt.ID)
	}
	<-ready
	<-ready
	close(start)
	collect := func() claimResult {
		t.Helper()
		select {
		case result := <-results:
			return result
		case <-ctx.Done():
			t.Fatal("concurrent claim did not finish before its watchdog", ctx.Err())
			return claimResult{}
		}
	}
	a, b := collect(), collect()
	var winner, waiting claimResult
	switch {
	case a.err == nil && b.err != nil:
		winner, waiting = a, b
	case b.err == nil && a.err != nil:
		winner, waiting = b, a
	default:
		t.Fatalf("concurrent same-account claims did not yield exactly one winner: first=%+v second=%+v", a, b)
	}
	var blocked *domain.Error
	if !errors.As(waiting.err, &blocked) || blocked.Code != "attempt_active" {
		t.Fatalf("other claim was not blocked by occupied capacity: %v", waiting.err)
	}
	if winner.attempt.ID != winner.id || winner.attempt.Phase != "preparing" || winner.attempt.SourceSnapshotID != nil || winner.attempt.LaunchIntent {
		t.Fatal("winner did not own an unlaunched preparation claim", winner.attempt)
	}
	assertOnlyClaim := func(id string) {
		t.Helper()
		claims, err := s.GraphClaims(ctx)
		if err != nil || len(claims) != 1 || claims[0].AttemptID != id || claims[0].CapturedDeclaredAccountID == nil || *claims[0].CapturedDeclaredAccountID != account.ID {
			t.Fatal("shared account has missing or overlapping managed claims", claims, err)
		}
	}
	assertOnlyClaim(winner.id)
	queued, err := s.Attempt(ctx, waiting.id)
	if err != nil || queued.Phase != "queued" || queued.LaunchIntent || queued.SourceSnapshotID != nil {
		t.Fatal("blocked attempt acquired preparation or launch state", queued, err)
	}

	// This is an unlaunched synthetic attempt. Verified not-launched cleanup
	// proves only store release and does not stand in for process cleanup.
	settled, err := s.SettleAttempt(ctx, winner.id, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"})
	if err != nil || settled.Phase != "settled" || settled.Cleanup.State != "verified" {
		t.Fatal("known unlaunched winner did not settle", settled, err)
	}
	claims, err := s.GraphClaims(ctx)
	if err != nil || len(claims) != 0 {
		t.Fatal("known cleanup did not release the exact claim", claims, err)
	}
	claimed, err := s.ClaimAttempt(ctx, waiting.id, owner)
	if err != nil || claimed.ID != waiting.id || claimed.Phase != "preparing" {
		t.Fatal("other account claimant did not proceed after release", claimed, err)
	}
	assertOnlyClaim(waiting.id)
}
