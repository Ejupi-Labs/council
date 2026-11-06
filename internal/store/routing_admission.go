package store

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

// Legacy wrappers cannot attest installation. Fake mode is a local, explicitly
// labelled harness fact; production callers provide metadata-only observations.
func wrapperRoutingFacts(mode string) domain.RoutingFacts {
	f := domain.RoutingFacts{Assessment: domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown", ReasonCodes: []string{}}, Contexts: []domain.NativeContextFact{}}
	if mode == "fake" {
		for _, id := range []string{"codex-native", "claude-native"} {
			f.Contexts = append(f.Contexts, domain.NativeContextFact{ContextID: id, InstallationStatus: "installed", Source: "local_fixture", ObservedAt: domain.Now()})
		}
	}
	return f
}
func checkResolvedScenario(a domain.Attempt, mode, scenario string) *domain.Error {
	if a.ResolvedTarget == nil {
		return domain.Invalid("resolved native target")
	}
	return domain.CheckExecutionScenario(domain.ExecutionPins{HarnessID: a.ResolvedTarget.HarnessID, Model: a.ResolvedTarget.ModelSelector}, mode, scenario)
}
func effectiveRoutingPolicy(base domain.RoutingPolicy, options domain.RoutingOptions) (domain.RoutingPolicy, *domain.Error) {
	var e *domain.Error
	base.Continuation, e = domain.ResolveContinuation(base.Continuation, options.Continuation)
	if e != nil {
		return base, e
	}
	if e = domain.CheckContinuationCapability(base.Continuation); e != nil {
		return base, e
	}
	if options.Budget != nil {
		if options.Budget.AdmissionLimit != "" {
			base.DefaultBudget.AdmissionLimit = options.Budget.AdmissionLimit
		}
		if options.Budget.WindowSeconds != nil {
			base.DefaultBudget.WindowSeconds = *options.Budget.WindowSeconds
		}
	}
	return base, nil
}
func (s *Store) captureRouting(ctx context.Context, tx *sql.Tx, t domain.Task, v domain.TaskVersion, options *domain.RoutingOptions, facts domain.RoutingFacts, prior *domain.TaskRoutingAuthority, createdBy string) (domain.TaskRoutingAuthority, error) {
	policy, err := s.routingPolicy(ctx, tx)
	if err != nil {
		return domain.TaskRoutingAuthority{}, err
	}
	opts := domain.RoutingOptions{}
	if prior != nil {
		policy = prior.PolicySnapshot
		opts = prior.OptionSnapshot
	}
	if options != nil {
		opts = *options
		if prior == nil || options.UseCurrentPolicy != nil && *options.UseCurrentPolicy {
			policy, err = s.routingPolicy(ctx, tx)
			if err != nil {
				return domain.TaskRoutingAuthority{}, err
			}
		}
		if options.ExpectedPolicyRevision != "" && options.ExpectedPolicyRevision != policy.Revision {
			return domain.TaskRoutingAuthority{}, domain.RevisionConflict(policy.Revision)
		}
	}
	effective, validationErr := effectiveRoutingPolicy(policy, opts)
	if validationErr != nil {
		return domain.TaskRoutingAuthority{}, validationErr
	}
	if prior != nil {
		budget, readErr := s.taskBudget(ctx, tx, t.ID)
		if readErr != nil {
			return domain.TaskRoutingAuthority{}, readErr
		}
		if budget == nil {
			return domain.TaskRoutingAuthority{}, storageError()
		}
		if countInt(effective.Continuation.MaxAutomaticAdmissions) > countInt(budget.GrantedAdmissions) {
			return domain.TaskRoutingAuthority{}, domain.Invalid("automatic admissions no greater than cumulative task admission authority")
		}
	}
	assessment := facts.Assessment
	if assessment.Classification == "" {
		assessment = domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown", ReasonCodes: []string{}}
	}
	if opts.Suitability != "" && opts.Suitability != "unknown" {
		assessment = domain.SuitabilityAssessment{Classification: opts.Suitability, Source: "user_declared", ReasonCodes: []string{}}
	}
	a := domain.TaskRoutingAuthority{ID: domain.NewID(), TaskID: t.ID, TaskVersion: v.Version, CapturedAt: domain.Now(), PolicySnapshot: policy, OptionSnapshot: opts, SuitabilityAssessment: assessment, CreatedBy: createdBy}
	if len(encode(a)) > domain.MaxRoutingDecisionBytes {
		return a, domain.Invalid("bounded routing authority")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO task_routing_versions VALUES(?,?,?,?)", a.ID, t.ID, intRevision(v.Version), encode(a)); err != nil {
		return a, storageError()
	}
	return a, nil
}
func (s *Store) initialBudget(ctx context.Context, tx *sql.Tx, a domain.TaskRoutingAuthority, requestID string) (domain.TaskBudget, error) {
	p, e := effectiveRoutingPolicy(a.PolicySnapshot, a.OptionSnapshot)
	if e != nil {
		return domain.TaskBudget{}, e
	}
	if countInt(p.Continuation.MaxAutomaticAdmissions) > countInt(p.DefaultBudget.AdmissionLimit) {
		return domain.TaskBudget{}, domain.Invalid("automatic admissions no greater than initial admission authority")
	}
	b := domain.TaskBudget{TaskID: a.TaskID, Revision: "1", GrantedAdmissions: p.DefaultBudget.AdmissionLimit, ConsumedAdmissions: "0", RemainingAdmissions: p.DefaultBudget.AdmissionLimit, AutomaticAdmissions: "0", HistoricalAdmissions: "0", CurrentWindow: domain.BudgetWindow{Number: "1", WindowSeconds: p.DefaultBudget.WindowSeconds}, UpdatedAt: domain.Now()}
	if err := saveBudget(ctx, tx, b); err != nil {
		return b, err
	}
	if err := appendBudgetLedger(ctx, tx, b, "grant", requestID, nil, b.GrantedAdmissions, "0"); err != nil {
		return b, err
	}
	return b, nil
}
func appendBudgetLedger(ctx context.Context, tx *sql.Tx, b domain.TaskBudget, kind, requestID string, attemptID *string, granted, consumed string) error {
	seq, e := nextSequence(ctx, tx, "budget_ledger")
	if e != nil {
		return e
	}
	l := domain.BudgetLedgerEntry{ID: domain.NewID(), Sequence: strconv.FormatInt(seq, 10), TaskID: b.TaskID, BudgetRevision: b.Revision, Kind: kind, RequestID: requestID, AttemptID: attemptID, GrantedDelta: granted, ConsumedDelta: consumed, Window: b.CurrentWindow, CreatedAt: domain.Now()}
	if _, err := tx.ExecContext(ctx, "INSERT INTO budget_ledger VALUES(?,?,?,?,?,?,?)", seq, l.ID, b.TaskID, intRevision(b.Revision), kind, attemptID, encode(l)); err != nil {
		return storageError()
	}
	return nil
}
func (s *Store) admitRouting(ctx context.Context, tx *sql.Tx, t domain.Task, v domain.TaskVersion, a *domain.Attempt, authority domain.TaskRoutingAuthority, b *domain.TaskBudget, facts domain.RoutingFacts, kind string) (domain.RoutingDecision, error) {
	if b.RemainingAdmissions == "0" {
		return domain.RoutingDecision{}, domain.Fail("budget_exhausted", "This task has no remaining managed admissions. Grant additional authority explicitly before starting another attempt.")
	}
	now, e := domain.ParseRoutingTime(a.AdmittedAt)
	if e != nil {
		return domain.RoutingDecision{}, storageError()
	}
	if b.CurrentWindow.DeadlineAt != nil {
		deadline, e := domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
		if e != nil {
			return domain.RoutingDecision{}, storageError()
		}
		if !now.Before(deadline) {
			return domain.RoutingDecision{}, domain.Fail("budget_expired", "The shared task window expired. Grant an explicit new window before new work.")
		}
	}
	p, e := effectiveRoutingPolicy(authority.PolicySnapshot, authority.OptionSnapshot)
	if e != nil {
		return domain.RoutingDecision{}, e
	}
	snapshot, err := s.routingSnapshot(ctx, tx)
	if err != nil {
		return domain.RoutingDecision{}, err
	}
	snapshot.Policy = p
	selection, e := routing.Select(domain.RoutingSelectionInput{Snapshot: snapshot, Pins: v.Pins, Options: authority.OptionSnapshot, Assessment: authority.SuitabilityAssessment, Facts: facts.Contexts, Now: a.AdmittedAt})
	if e != nil {
		return domain.RoutingDecision{}, e
	}
	if b.CurrentWindow.DeadlineAt == nil {
		first := a.AdmittedAt
		deadline := now.Add(time.Duration(b.CurrentWindow.WindowSeconds) * time.Second).Format("2006-01-02T15:04:05.000Z")
		b.CurrentWindow.FirstAdmissionAt = &first
		b.CurrentWindow.DeadlineAt = &deadline
		if err = appendBudgetLedger(ctx, tx, *b, "window_open", a.StartRequestID, nil, "0", "0"); err != nil {
			return domain.RoutingDecision{}, err
		}
	}
	deadline, _ := domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
	attemptDeadline, _ := domain.ParseRoutingTime(a.DeadlineAt)
	if deadline.Before(attemptDeadline) {
		a.DeadlineAt = *b.CurrentWindow.DeadlineAt
	}
	b.ConsumedAdmissions, e = domain.AddRoutingCounts(b.ConsumedAdmissions, "1")
	if e != nil {
		return domain.RoutingDecision{}, e
	}
	b.RemainingAdmissions, e = domain.SubtractRoutingCounts(b.GrantedAdmissions, b.ConsumedAdmissions)
	if e != nil {
		return domain.RoutingDecision{}, e
	}
	if kind == "automatic" {
		b.AutomaticAdmissions, e = domain.AddRoutingCounts(b.AutomaticAdmissions, "1")
		if e != nil {
			return domain.RoutingDecision{}, e
		}
	}
	b.Revision, err = increment(b.Revision)
	if err != nil {
		return domain.RoutingDecision{}, err
	}
	b.UpdatedAt = a.AdmittedAt
	a.ConnectionID = selection.SelectedTarget.ConnectionID
	a.ResolvedTarget = &selection.SelectedTarget
	d := domain.RoutingDecision{ID: domain.NewID(), TaskID: t.ID, TaskVersion: v.Version, AttemptID: a.ID, RequestID: a.StartRequestID, AuthorityID: authority.ID, Kind: kind, PreviousAttemptID: a.PreviousAttemptID, CreatedAt: a.AdmittedAt, CandidateAssessments: selection.Candidates, SelectedTarget: selection.SelectedTarget, BudgetEffect: domain.BudgetEffect{AdmissionUnits: "1", RemainingAdmissions: b.RemainingAdmissions, BudgetRevision: b.Revision, Window: b.CurrentWindow}, EvidenceSnapshot: domain.RoutingFacts{Assessment: authority.SuitabilityAssessment, Contexts: facts.Contexts}, CapacityEvidence: selection.CapacityEvidence, Uncertainties: selection.Uncertainties}
	if len(encode(d)) > domain.MaxRoutingDecisionBytes {
		return d, domain.Fail("routing_unavailable", "The complete routing decision exceeds its bounded evidence envelope. Inspect capacity reports before new work.")
	}
	a.RoutingDecisionID = &d.ID
	// The caller inserts the Attempt before the decision to satisfy immutable FK.
	if err = saveBudget(ctx, tx, *b); err != nil {
		return d, err
	}
	return d, nil
}
func storeRoutingDecision(ctx context.Context, tx *sql.Tx, d domain.RoutingDecision) error {
	seq, err := nextSequence(ctx, tx, "routing_decisions")
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO routing_decisions VALUES(?,?,?,?,?,?,?,?)", seq, d.ID, d.TaskID, intRevision(d.TaskVersion), d.AttemptID, d.PreviousAttemptID, d.Kind, encode(d)); err != nil {
		return storageError()
	}
	return appendBudgetLedger(ctx, tx, domain.TaskBudget{TaskID: d.TaskID, Revision: d.BudgetEffect.BudgetRevision, CurrentWindow: d.BudgetEffect.Window}, "admission", d.RequestID, &d.AttemptID, "0", "1")
}
