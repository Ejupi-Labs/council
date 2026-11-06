package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingBlockedAutomatic(t *testing.T, s *Store) (string, domain.Admission, domain.RoutingFacts) {
	t.Helper()
	ctx := context.Background()
	owner := domain.NewID()
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	p := openFixture(t, s, "automatic")
	in := taskInput(p.ID)
	yes := true
	categories := []string{"pre_launch_ineligible"}
	in.Routing = &domain.RoutingOptions{Suitability: "routine", Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "1", Categories: &categories, AllowReplacement: &yes, AllowEscalation: &yes}}
	facts := wrapperRoutingFacts("fake")
	r := s.CreateTaskWithRouting(ctx, owner, in, facts)
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	if a.Attempt.ResolvedTarget.ContextID != "codex-native" {
		t.Fatal(a)
	}
	no := false
	r = s.UpdateRoutingContext(ctx, "codex-native", domain.ContextUpdateInput{RequestID: domain.NewID(), ExpectedRevision: "1", Enabled: &no})
	if r.Status != 200 {
		t.Fatal(r)
	}
	blocked, e := s.DispatchRouting(ctx, a.Attempt.ID, facts)
	if e != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" || blocked.LaunchIntent {
		t.Fatal(blocked, e)
	}
	return owner, a, facts
}
func TestRoutingContinuationOneLinkedUnitAndImmutablePredecessor(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, first, facts := routingBlockedAutomatic(t, s)
	var terminal, proof string
	s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", first.Attempt.ID).Scan(&terminal)
	s.db.QueryRow("SELECT data FROM routing_continuation_proofs WHERE attempt_id=?", first.Attempt.ID).Scan(&proof)
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || a == nil || a.Attempt.ConnectionID != "claude-native" || a.Attempt.PreviousAttemptID == nil || *a.Attempt.PreviousAttemptID != first.Attempt.ID || a.Budget.ConsumedAdmissions != "2" || a.Budget.AutomaticAdmissions != "1" || a.RoutingDecision.Kind != "automatic" {
		t.Fatal(a, e)
	}
	again, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || again != nil {
		t.Fatal("duplicate continuation", again, e)
	}
	var afterTerminal, afterProof string
	s.db.QueryRow("SELECT data FROM attempt_terminals WHERE attempt_id=?", first.Attempt.ID).Scan(&afterTerminal)
	s.db.QueryRow("SELECT data FROM routing_continuation_proofs WHERE attempt_id=?", first.Attempt.ID).Scan(&afterProof)
	if terminal != afterTerminal || proof != afterProof {
		t.Fatal("continuation rewrote terminal/proof")
	}
	before, _ := s.Task(ctx, first.Task.ID)
	r := s.StopAttempt(ctx, first.Attempt.ID, domain.NewID())
	after, _ := s.Task(ctx, first.Task.ID)
	if r.Status != 200 || before.Task.Revision != after.Task.Revision {
		t.Fatal("Stop on old predecessor changed successor authority", r)
	}
}
func TestRoutingTerminalStopSuppressesOnlyFreshProof(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, first, facts := routingBlockedAutomatic(t, s)
	before, _ := s.Task(ctx, first.Task.ID)
	id := domain.NewID()
	r := s.StopAttempt(ctx, first.Attempt.ID, id)
	if r.Status != 200 || r.Envelope.Data.(map[string]any)["continuationSuppressed"] != true {
		t.Fatal(r)
	}
	after, _ := s.Task(ctx, first.Task.ID)
	if after.Task.Revision == before.Task.Revision {
		t.Fatal("Stop did not invalidate fresh proof")
	}
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || a != nil {
		t.Fatal("Stop permitted replacement", a, e)
	}
	r = s.StopAttempt(ctx, first.Attempt.ID, id)
	if r.Status != 200 {
		t.Fatal(r)
	}
	s.StopAttempt(ctx, first.Attempt.ID, domain.NewID())
	last, _ := s.Task(ctx, first.Task.ID)
	if last.Task.Revision != after.Task.Revision {
		t.Fatal("stale terminal Stop repeatedly changed authority")
	}
}
func TestRoutingContinuationCommitUncertaintyRetainsOneQueuedSuccessor(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, first, facts := routingBlockedAutomatic(t, s)
	s.AfterCommit = func() error { return errors.New("lost automatic ack") }
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	var de *domain.Error
	if a != nil || !errors.As(e, &de) || de.Code != "write_uncertain" {
		t.Fatal(a, e)
	}
	s.AfterCommit = nil
	var count int
	s.db.QueryRow("SELECT count(*) FROM routing_decisions WHERE previous_attempt_id=? AND kind='automatic'", first.Attempt.ID).Scan(&count)
	budget, _ := s.TaskBudget(ctx, first.Task.ID)
	if count != 1 || budget.Budget.ConsumedAdmissions != "2" {
		t.Fatal("uncertainty lost committed authority", count, budget)
	}
}

func TestRoutingContinuationExpiredWindowHasNoNewAuthority(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, first, facts := routingBlockedAutomatic(t, s)
	// Controlled retained metadata moves the whole task window into the past;
	// no sleeping or production clock hook is needed for this admission boundary.
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		b, e := s.taskBudget(ctx, tx, first.Task.ID)
		if e != nil {
			return e
		}
		b.CurrentWindow.FirstAdmissionAt = ptr("2020-01-01T00:00:00.000Z")
		b.CurrentWindow.DeadlineAt = ptr("2020-01-01T00:30:00.000Z")
		return saveBudget(ctx, tx, *b)
	}); e != nil {
		t.Fatal(e)
	}
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || a != nil {
		t.Fatal("expired authority did not return no continuation", a, e)
	}
	b, _ := s.TaskBudget(ctx, first.Task.ID)
	if b.Budget.ConsumedAdmissions != "1" || b.Budget.AutomaticAdmissions != "0" {
		t.Fatal("expired continuation charged work", b)
	}
}
func TestRoutingUnknownInstallationCannotAuthorizeReplacement(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner := domain.NewID()
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	p := openFixture(t, s, "unknown-context")
	in := taskInput(p.ID)
	yes := true
	categories := []string{"pre_launch_ineligible"}
	in.Routing = &domain.RoutingOptions{Suitability: "routine", Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "1", Categories: &categories, AllowReplacement: &yes, AllowEscalation: &yes}}
	facts := wrapperRoutingFacts("fake")
	r := s.CreateTaskWithRouting(ctx, owner, in, facts)
	if r.Status != 201 {
		t.Fatal(r)
	}
	first := r.Envelope.Data.(domain.Admission)
	for i := range facts.Contexts {
		if facts.Contexts[i].ContextID == first.Attempt.ResolvedTarget.ContextID {
			facts.Contexts[i].InstallationStatus = "unknown"
		}
	}
	blocked, e := s.DispatchRouting(ctx, first.Attempt.ID, facts)
	if e != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" {
		t.Fatal(blocked, e)
	}
	var proofs int
	s.db.QueryRow("SELECT count(*) FROM routing_continuation_proofs WHERE attempt_id=?", first.Attempt.ID).Scan(&proofs)
	if proofs != 0 {
		t.Fatal("an observer unknown created positive fallback authority")
	}
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || a != nil {
		t.Fatal(a, e)
	}
}
func TestRoutingContinuationPreCommitFaultDoesNotSpendUnit(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner, first, facts := routingBlockedAutomatic(t, s)
	s.BeforeCommit = func() error { return errors.New("automatic transaction rejected") }
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	var de *domain.Error
	if a != nil || !errors.As(e, &de) || de.Code != "storage_failed" {
		t.Fatal(a, e)
	}
	s.BeforeCommit = nil
	budget, _ := s.TaskBudget(ctx, first.Task.ID)
	var successors int
	s.db.QueryRow("SELECT count(*) FROM routing_decisions WHERE previous_attempt_id=? AND kind=\x27automatic\x27", first.Attempt.ID).Scan(&successors)
	if successors != 0 || budget.Budget.ConsumedAdmissions != "1" {
		t.Fatal("rolled-back auto transaction spent a unit", successors, budget)
	}
}

func TestRoutingUnknownCapacityCoverageCannotAuthorizeReplacement(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	owner := domain.NewID()
	if e := s.RecoverOwner(ctx, owner); e != nil {
		t.Fatal(e)
	}
	r := s.SetRoutingPolicy(ctx, domain.RoutingPolicyInput{RequestID: domain.NewID(), ExpectedRevision: "1", UnknownAvailability: "require_positive"})
	if r.Status != 200 {
		t.Fatal(r)
	}
	for _, id := range []string{"codex-native", "claude-native"} {
		subject := domain.CapacitySubject{Kind: "context", ID: id}
		routingTestReport(t, s, subject, "occupancy", "available", nil)
		routingTestReport(t, s, subject, "allowance", "available", nil)
	}
	p := openFixture(t, s, "unknown-capacity")
	in := taskInput(p.ID)
	yes := true
	categories := []string{"pre_launch_ineligible"}
	in.Routing = &domain.RoutingOptions{Suitability: "routine", Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "1", Categories: &categories, AllowReplacement: &yes, AllowEscalation: &yes}}
	facts := wrapperRoutingFacts("fake")
	r = s.CreateTaskWithRouting(ctx, owner, in, facts)
	if r.Status != 201 {
		t.Fatal(r)
	}
	first := r.Envelope.Data.(domain.Admission)
	routingTestReport(t, s, domain.CapacitySubject{Kind: "context", ID: first.Attempt.ResolvedTarget.ContextID}, "occupancy", "unknown", nil)
	blocked, e := s.DispatchRouting(ctx, first.Attempt.ID, facts)
	if e != nil || blocked.Outcome == nil || *blocked.Outcome != "blocked" {
		t.Fatal(blocked, e)
	}
	var proofs int
	s.db.QueryRow("SELECT count(*) FROM routing_continuation_proofs WHERE attempt_id=?", first.Attempt.ID).Scan(&proofs)
	if proofs != 0 {
		t.Fatal("unknown positive coverage produced negative-report proof")
	}
	a, e := s.ContinueRouting(ctx, owner, first.Attempt.ID, facts)
	if e != nil || a != nil {
		t.Fatal("unknown capacity authorized fallback", a, e)
	}
}
