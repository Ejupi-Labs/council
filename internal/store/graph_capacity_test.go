package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestGraphCapacityAdmissionAccountSurvivesUnlinkBeforeClaim(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := domain.NewID()
	p := openFixture(t, s, "admitted-account-reservation")
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	account := routingTestAccount(t, s)
	routingTestLink(t, s, "codex-native", account.ID, false)
	routingTestLink(t, s, "claude-native", account.ID, false)
	admit := func(harness string) domain.Admission {
		t.Helper()
		in := taskInput(p.ID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		return r.Envelope.Data.(domain.Admission)
	}
	first, second := admit("codex"), admit("claude-code")
	if first.Attempt.ResolvedTarget == nil || first.Attempt.ResolvedTarget.DeclaredAccountAssociation == nil {
		t.Fatal("fixture admission did not capture a declared association")
	}
	capturedRevision := first.Attempt.ResolvedTarget.DeclaredAccountAssociation.LinkRevision
	routingTestLink(t, s, "codex-native", "", true)
	if _, e := s.ClaimAttempt(ctx, first.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	claims, e := s.GraphClaims(ctx)
	if e != nil || len(claims) != 1 || claims[0].CapturedDeclaredAccountID == nil || *claims[0].CapturedDeclaredAccountID != account.ID || claims[0].CapturedLinkRevision == nil || *claims[0].CapturedLinkRevision != capturedRevision {
		t.Fatal("claim silently dropped the admitted account/link evidence", claims, e)
	}
	if _, e := s.ClaimAttempt(ctx, second.Attempt.ID, owner); e == nil {
		t.Fatal("unlink before claim invented capacity for the captured shared account")
	}
}

// These are real SQLite/control transitions with fixed fake admission metadata,
// not evidence that a provider or native worker executed.
func TestGraphCapacityTwoActualContextClaimsAndSpecificGuard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := openFixture(t, s, "two-context-claims")
	owner := domain.NewID()
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	admit := func(harness string) domain.Admission {
		t.Helper()
		in := taskInput(p.ID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		return r.Envelope.Data.(domain.Admission)
	}
	first, second, waiting := admit("codex"), admit("claude-code"), admit("codex")
	if _, e := s.ClaimAttempt(ctx, first.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ClaimAttempt(ctx, second.Attempt.ID, owner); e != nil {
		t.Fatal("independent supported second context must own the second slot", e)
	}
	claims, e := s.GraphClaims(ctx)
	if e != nil || len(claims) != 2 || claims[0].Slot != "1" || claims[1].Slot != "2" {
		t.Fatal("actual bounded claims missing", claims, e)
	}
	if gate, e := s.Gate(ctx); e != nil || gate.ActiveAttemptID != nil {
		t.Fatal("two claims cannot become a sole-ID/empty-pool proof", gate, e)
	}
	for _, id := range []string{first.Attempt.ID, second.Attempt.ID} {
		if e := s.CheckAttemptClaim(ctx, owner, id); e != nil {
			t.Fatal("own claim guard used the sole marker", id, e)
		}
	}
	if _, e := s.ClaimAttempt(ctx, waiting.Attempt.ID, owner); e == nil {
		t.Fatal("third/same-context dispatch exceeded actual capacity")
	}
	if e := s.CheckAttemptClaim(ctx, domain.NewID(), second.Attempt.ID); e == nil {
		t.Fatal("another owner borrowed an occupied claim")
	}
	if _, e := s.SettleAttempt(ctx, first.Attempt.ID, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete"}); e != nil {
		t.Fatal(e)
	}
	if e := s.CheckAttemptClaim(ctx, owner, second.Attempt.ID); e != nil {
		t.Fatal("settling first claim released another worker", e)
	}
	if e := s.CheckAttemptClaim(ctx, owner, first.Attempt.ID); e == nil {
		t.Fatal("settled claim still owned")
	}
	if _, e := s.ClaimAttempt(ctx, waiting.Attempt.ID, owner); e != nil {
		t.Fatal("known release did not free its exact slot/context", e)
	}
}

func TestGraphCapacityCapturedAccountSurvivesUnlink(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := domain.NewID()
	p := openFixture(t, s, "captured-account-claims")
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	account := routingTestAccount(t, s)
	routingTestLink(t, s, "codex-native", account.ID, false)
	routingTestLink(t, s, "claude-native", account.ID, false)
	admit := func(harness string) domain.Admission {
		t.Helper()
		in := taskInput(p.ID)
		in.Pins = &domain.ExecutionPins{HarnessID: harness}
		r := s.CreateTask(ctx, owner, in)
		if r.Status != 201 {
			t.Fatal(r)
		}
		return r.Envelope.Data.(domain.Admission)
	}
	first, second := admit("codex"), admit("claude-code")
	if _, e := s.ClaimAttempt(ctx, first.Attempt.ID, owner); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ClaimAttempt(ctx, second.Attempt.ID, owner); e == nil {
		t.Fatal("declared aliases invented a second account reservation")
	}
	routingTestLink(t, s, "codex-native", "", true)
	claims, e := s.GraphClaims(ctx)
	if e != nil || len(claims) != 1 || claims[0].CapturedDeclaredAccountID == nil || *claims[0].CapturedDeclaredAccountID != account.ID {
		t.Fatal("unlink erased captured reservation", claims, e)
	}
	if _, e := s.ClaimAttempt(ctx, second.Attempt.ID, owner); e == nil {
		t.Fatal("unlink refilled occupied captured account")
	}
}
