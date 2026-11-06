package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestRoutingBudgetCountsReplayAndExplicitAdditionalAuthority(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "budget")
	owner := domain.NewID()
	in := taskInput(p.ID)
	r := s.CreateTask(ctx, owner, in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	if a.Budget == nil || a.Budget.GrantedAdmissions != "2" || a.Budget.ConsumedAdmissions != "1" || a.Budget.RemainingAdmissions != "1" || a.RoutingDecision == nil {
		t.Fatal(a)
	}
	firstDeadline := *a.Budget.CurrentWindow.DeadlineAt
	s.StopAttempt(ctx, a.Attempt.ID, domain.NewID())
	detail, e := s.Task(ctx, a.Task.ID)
	if e != nil {
		t.Fatal(e)
	}
	next := domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: detail.Task.Revision, TaskVersion: "1", ExecutionMode: "fake"}
	r = s.StartAttempt(ctx, owner, a.Task.ID, next)
	if r.Status != 201 {
		t.Fatal(r)
	}
	second := r.Envelope.Data.(domain.Admission)
	if second.Budget.ConsumedAdmissions != "2" || second.Budget.RemainingAdmissions != "0" || *second.Budget.CurrentWindow.DeadlineAt != firstDeadline {
		t.Fatal(second)
	}
	s.StopAttempt(ctx, second.Attempt.ID, domain.NewID())
	detail, _ = s.Task(ctx, a.Task.ID)
	third := next
	third.RequestID = domain.NewID()
	third.ExpectedRevision = detail.Task.Revision
	r = s.StartAttempt(ctx, owner, a.Task.ID, third)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "budget_exhausted" || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
	// Exact acknowledged intent survives exhausted authority and accepted Stop.
	r = s.StartAttemptWithRouting(ctx, "another-owner", a.Task.ID, next, domain.RoutingFacts{})
	if r.Status != 201 || r.Envelope.Receipt == nil {
		t.Fatal(r)
	}
	view, _ := s.TaskBudget(ctx, a.Task.ID)
	grant := domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: detail.Task.Revision, ExpectedBudgetRevision: view.Budget.Revision, AdditionalAdmissions: "1"}
	r = s.AmendBudget(ctx, a.Task.ID, grant)
	if r.Status != 200 {
		t.Fatal(r)
	}
	amend := r.Envelope.Data.(domain.BudgetAmendmentResult)
	if amend.Budget.GrantedAdmissions != "3" || amend.Budget.ConsumedAdmissions != "2" || amend.Budget.RemainingAdmissions != "1" || *amend.Budget.CurrentWindow.DeadlineAt != firstDeadline || amend.Task.CurrentVersion != "1" {
		t.Fatal(amend)
	}
}
func TestRoutingBudgetAdmissionLostAcknowledgementIsOneUnit(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "ack")
	in := taskInput(p.ID)
	s.AfterCommit = func() error { return errors.New("lost ack") }
	r := s.CreateTask(ctx, domain.NewID(), in)
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(r)
	}
	s.AfterCommit = nil
	r = s.CreateTask(ctx, domain.NewID(), in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	var attempts, decisions, admissions int
	s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&attempts)
	s.db.QueryRow("SELECT count(*) FROM routing_decisions").Scan(&decisions)
	s.db.QueryRow("SELECT count(*) FROM budget_ledger WHERE kind='admission'").Scan(&admissions)
	if attempts != 1 || decisions != 1 || admissions != 1 {
		t.Fatal(attempts, decisions, admissions)
	}
}
func TestRoutingLegacyAdoptionAppendsVersionAndDoesNotStart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, ids := routingSchema3Database(t, path, "", false)
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r := s.StartAttempt(ctx, domain.NewID(), ids.TaskID, domain.StartAttemptInput{RequestID: domain.NewID(), ExpectedRevision: "1", TaskVersion: "1", ExecutionMode: "fake"})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "routing_adoption_required" {
		t.Fatal(r)
	}
	seconds := 1800
	in := domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "adopt", ExpectedRevision: "1", AdditionalAdmissions: "2", NewWindowSeconds: &seconds}
	r = s.AmendBudget(ctx, ids.TaskID, in)
	if r.Status != 200 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.BudgetAmendmentResult)
	if a.Version == nil || a.Version.Version != "2" || a.Budget.HistoricalAdmissions != "1" || a.Budget.GrantedAdmissions != "3" || a.Budget.ConsumedAdmissions != "1" || a.Budget.RemainingAdmissions != "2" || a.Budget.CurrentWindow.DeadlineAt != nil {
		t.Fatal(a)
	}
	var count int
	s.db.QueryRow("SELECT count(*) FROM attempts").Scan(&count)
	if count != 1 {
		t.Fatal("adoption started work", count)
	}
	r = s.AmendBudgetWithRouting(ctx, ids.TaskID, in, domain.RoutingFacts{})
	if r.Status != 200 {
		t.Fatal("adoption lost replay", r)
	}
}

func TestRoutingNewWindowCannotPassRetainedFence(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "fenced-budget")
	r := s.CreateTask(ctx, domain.NewID(), taskInput(p.ID))
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	s.StopAttempt(ctx, a.Attempt.ID, domain.NewID())
	detail, _ := s.Task(ctx, a.Task.ID)
	budget, _ := s.TaskBudget(ctx, a.Task.ID)
	if e := s.Fence(ctx, domain.ExecutionIncident{ID: domain.NewID(), Code: "test_retained_uncertainty", Message: "Local fake fence", CreatedAt: domain.Now()}); e != nil {
		t.Fatal(e)
	}
	seconds := 1800
	r = s.AmendBudget(ctx, a.Task.ID, domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: detail.Task.Revision, ExpectedBudgetRevision: budget.Budget.Revision, AdditionalAdmissions: "1", NewWindowSeconds: &seconds})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "dispatch_fenced" || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
	after, _ := s.TaskBudget(ctx, a.Task.ID)
	if encode(after) != encode(budget) {
		t.Fatal("fenced grant changed authority", after, budget)
	}
}

func TestRoutingGuidanceContinuationUsesExistingGrantWithoutRefill(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "guidance-authority")
	in := taskInput(p.ID)
	in.Routing = &domain.RoutingOptions{Budget: &domain.RoutingBudgetInput{AdmissionLimit: "4"}}
	r := s.CreateTask(ctx, domain.NewID(), in)
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	before, _ := s.TaskBudget(ctx, a.Task.ID)
	categories := []string{"pre_launch_ineligible"}
	g := domain.GuidanceInput{RequestID: domain.NewID(), ExpectedRevision: "1", GuidanceText: "Preserve the changed guidance.", Routing: &domain.RoutingOptions{Continuation: &domain.ContinuationInput{MaxAutomaticAdmissions: "3", Categories: &categories}}}
	r = s.UpdateGuidanceWithRouting(ctx, a.Task.ID, g, wrapperRoutingFacts("fake"))
	if r.Status != 200 {
		t.Fatal(r)
	}
	after, _ := s.TaskBudget(ctx, a.Task.ID)
	if encode(before) != encode(after) {
		t.Fatal("guidance refilled task budget", before, after)
	}
	view, e := s.TaskRouting(ctx, a.Task.ID)
	if e != nil || view.Authority == nil || view.Authority.PolicySnapshot.DefaultBudget.AdmissionLimit != "2" || view.Authority.OptionSnapshot.Continuation.MaxAutomaticAdmissions != "3" {
		t.Fatal(view, e)
	}
}

func TestRoutingBudgetPersistsLargeCountsAndRejectsCumulativeOverflow(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	p := openFixture(t, s, "large-budget")
	r := s.CreateTask(ctx, domain.NewID(), taskInput(p.ID))
	if r.Status != 201 {
		t.Fatal(r)
	}
	a := r.Envelope.Data.(domain.Admission)
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		b, e := s.taskBudget(ctx, tx, a.Task.ID)
		if e != nil {
			return e
		}
		b.GrantedAdmissions = "9007199254740993"
		b.RemainingAdmissions = "9007199254740992"
		return saveBudget(ctx, tx, *b)
	}); e != nil {
		t.Fatal(e)
	}
	view, _ := s.TaskBudget(ctx, a.Task.ID)
	in := domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: "1", ExpectedBudgetRevision: view.Budget.Revision, AdditionalAdmissions: "2"}
	r = s.AmendBudget(ctx, a.Task.ID, in)
	if r.Status != 200 {
		t.Fatal(r)
	}
	amended := r.Envelope.Data.(domain.BudgetAmendmentResult)
	if amended.Budget.GrantedAdmissions != "9007199254740995" || amended.Budget.RemainingAdmissions != "9007199254740994" {
		t.Fatal("large SQL count rounded", amended.Budget)
	}
	if e := s.executionWrite(ctx, func(tx *sql.Tx) error {
		b, e := s.taskBudget(ctx, tx, a.Task.ID)
		if e != nil {
			return e
		}
		b.GrantedAdmissions = "9223372036854775807"
		b.RemainingAdmissions = "9223372036854775806"
		return saveBudget(ctx, tx, *b)
	}); e != nil {
		t.Fatal(e)
	}
	before, _ := s.TaskBudget(ctx, a.Task.ID)
	detail, _ := s.Task(ctx, a.Task.ID)
	r = s.AmendBudget(ctx, a.Task.ID, domain.BudgetAmendmentInput{RequestID: domain.NewID(), Action: "grant", ExpectedRevision: detail.Task.Revision, ExpectedBudgetRevision: before.Budget.Revision, AdditionalAdmissions: "1"})
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "budget_conflict" || r.Envelope.Receipt != nil {
		t.Fatal("overflow acquired authority", r)
	}
	after, _ := s.TaskBudget(ctx, a.Task.ID)
	if encode(before) != encode(after) {
		t.Fatal("overflow changed retained budget", before, after)
	}
	if replay := s.AmendBudget(ctx, a.Task.ID, in); replay.Status != 200 || replay.Envelope.Receipt == nil {
		t.Fatal("current overflow prevented exact old grant replay", replay)
	}
}
