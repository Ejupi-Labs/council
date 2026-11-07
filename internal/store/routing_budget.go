package store

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func BudgetAmendmentFingerprint(taskID string, in domain.BudgetAmendmentInput) string {
	return Fingerprint("tasks.budget.amend", struct {
		TaskID string
		Input  domain.BudgetAmendmentInput
	}{taskID, in})
}
func (s *Store) AmendBudget(ctx context.Context, taskID string, in domain.BudgetAmendmentInput) domain.Result {
	return s.AmendBudgetWithRouting(ctx, taskID, in, wrapperRoutingFacts(""))
}
func (s *Store) AmendBudgetWithRouting(ctx context.Context, taskID string, in domain.BudgetAmendmentInput, facts domain.RoutingFacts) domain.Result {
	if !domain.ValidID(taskID) {
		return domain.Failure(in.RequestID, domain.Invalid("task identity"))
	}
	if e := domain.ValidateBudgetAmendment(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "tasks.budget.amend", BudgetAmendmentFingerprint(taskID, in), func(tx *sql.Tx) (int, any, error) {
		if e := s.requireStandaloneAuthority(ctx, tx, taskID, true); e != nil {
			return 0, nil, e
		}
		if e := s.requireStandaloneAuthority(ctx, tx, taskID, true); e != nil {
			return 0, nil, e
		}
		g, err := s.gate(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		if err = checkGate(g); err != nil {
			return 0, nil, err
		}
		if in.Action == "adopt" || in.NewWindowSeconds != nil {
			for _, claim := range g.ActiveClaims {
				active, e := s.attempt(ctx, tx, claim.AttemptID)
				if e != nil {
					return 0, nil, e
				}
				if active.TaskID == taskID {
					return 0, nil, domain.Fail("attempt_active", "The task still owns the managed claim. Verify cleanup before opening another budget window.")
				}
				if g.OwnerInstanceID != "" && active.OwnerInstanceID != g.OwnerInstanceID {
					return 0, nil, domain.Fail("dispatch_fenced", "The managed claim belongs to an uncertain owner epoch. Inspect retained ownership before adding a window.")
				}
			}
		}
		t, err := s.task(ctx, tx, taskID)
		if err != nil {
			return 0, nil, err
		}
		if t.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(t.Revision)
		}
		v, err := s.version(ctx, tx, taskID, t.CurrentVersion)
		if err != nil {
			return 0, nil, err
		}
		old, err := s.latest(ctx, tx, t)
		if err != nil {
			return 0, nil, err
		}
		b, err := s.taskBudget(ctx, tx, taskID)
		if err != nil {
			return 0, nil, err
		}
		var prior *domain.TaskBudget
		if b != nil {
			copy := *b
			prior = &copy
		}
		var authority *domain.TaskRoutingAuthority
		var newVersion *domain.TaskVersion
		if in.Action == "adopt" {
			if b != nil {
				return 0, nil, domain.Fail("budget_conflict", "This task already has routing authority. Grant additional authority instead of adopting it again.")
			}
			if old != nil && (old.Phase != "settled" || old.Cleanup.State != "verified") {
				return 0, nil, domain.Fail("attempt_active", "Settle prior ownership and verify cleanup before adopting a task budget.")
			}
			var historical int64
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM attempts WHERE task_id=?", taskID).Scan(&historical); err != nil {
				return 0, nil, storageError()
			}
			v.Version, err = increment(v.Version)
			if err != nil {
				return 0, nil, err
			}
			v.CreatedAt = domain.Now()
			v.CreatedBy = "legacy_adoption"
			t.CurrentVersion = v.Version
			if _, err = tx.ExecContext(ctx, "INSERT INTO task_versions VALUES(?,?,?)", taskID, intRevision(v.Version), encode(v)); err != nil {
				return 0, nil, storageError()
			}
			a, e := s.captureRouting(ctx, tx, t, v, in.Routing, facts, nil, "legacy_adoption")
			if e != nil {
				return 0, nil, e
			}
			authority = &a
			newVersion = &v
			legacy := strconv.FormatInt(historical, 10)
			granted, validationErr := domain.AddRoutingCounts(legacy, in.AdditionalAdmissions)
			if validationErr != nil {
				return 0, nil, validationErr
			}
			b = &domain.TaskBudget{TaskID: taskID, Revision: "1", GrantedAdmissions: granted, ConsumedAdmissions: legacy, RemainingAdmissions: in.AdditionalAdmissions, AutomaticAdmissions: "0", HistoricalAdmissions: legacy, CurrentWindow: domain.BudgetWindow{Number: "1", WindowSeconds: *in.NewWindowSeconds}, UpdatedAt: domain.Now()}
			effective, validationErr := effectiveRoutingPolicy(a.PolicySnapshot, a.OptionSnapshot)
			if validationErr != nil {
				return 0, nil, validationErr
			}
			if countInt(effective.Continuation.MaxAutomaticAdmissions) > countInt(b.GrantedAdmissions) {
				return 0, nil, domain.Invalid("automatic admissions no greater than cumulative adopted authority")
			}
			if err = saveBudget(ctx, tx, *b); err != nil {
				return 0, nil, err
			}
			if err = appendBudgetLedger(ctx, tx, *b, "legacy_baseline", in.RequestID, nil, legacy, legacy); err != nil {
				return 0, nil, err
			}
		} else {
			if b == nil {
				return 0, nil, domain.Fail("routing_adoption_required", "Adopt an explicit task budget before granting additional authority.")
			}
			if b.Revision != in.ExpectedBudgetRevision {
				return 0, nil, domain.Fail("budget_conflict", "The task budget changed. Reload its exact revision before amending it.")
			}
			if in.NewWindowSeconds != nil {
				if old != nil && (old.Phase != "settled" || old.Cleanup.State != "verified") {
					return 0, nil, domain.Fail("attempt_active", "A new task window requires settled prior ownership and verified cleanup.")
				}
				b.CurrentWindow.Number, err = increment(b.CurrentWindow.Number)
				if err != nil {
					return 0, nil, err
				}
				b.CurrentWindow.WindowSeconds = *in.NewWindowSeconds
				b.CurrentWindow.FirstAdmissionAt = nil
				b.CurrentWindow.DeadlineAt = nil
			}
			granted, validationErr := domain.AddRoutingCounts(b.GrantedAdmissions, in.AdditionalAdmissions)
			if validationErr != nil {
				return 0, nil, validationErr
			}
			remaining, validationErr := domain.SubtractRoutingCounts(granted, b.ConsumedAdmissions)
			if validationErr != nil {
				return 0, nil, validationErr
			}
			b.GrantedAdmissions = granted
			b.RemainingAdmissions = remaining
			b.Revision, err = increment(b.Revision)
			if err != nil {
				return 0, nil, err
			}
			b.UpdatedAt = domain.Now()
		}
		t.Revision, err = increment(t.Revision)
		if err != nil {
			return 0, nil, err
		}
		t.StateRevision, err = increment(t.StateRevision)
		if err != nil {
			return 0, nil, err
		}
		t.UpdatedAt = domain.Now()
		amendment := domain.BudgetAmendment{ID: domain.NewID(), TaskID: taskID, RequestID: in.RequestID, Action: in.Action, AdditionalAdmissions: in.AdditionalAdmissions, NewWindowSeconds: in.NewWindowSeconds, PriorBudget: prior, Budget: *b, TaskVersion: v.Version, CreatedAt: domain.Now()}
		if authority != nil {
			amendment.AuthorityID = &authority.ID
		}
		b.LatestAmendmentID = &amendment.ID
		amendment.Budget = *b
		if err = saveBudget(ctx, tx, *b); err != nil {
			return 0, nil, err
		}
		if err = saveTask(ctx, tx, t); err != nil {
			return 0, nil, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO budget_amendments VALUES(?,?,?,?)", amendment.ID, taskID, intRevision(b.Revision), encode(amendment)); err != nil {
			return 0, nil, storageError()
		}
		kind := "grant"
		if in.Action == "adopt" {
			kind = "adoption"
		}
		if err = appendBudgetLedger(ctx, tx, *b, kind, in.RequestID, nil, in.AdditionalAdmissions, "0"); err != nil {
			return 0, nil, err
		}
		return 200, domain.BudgetAmendmentResult{Task: taskSummary(t, v, old, false), Version: newVersion, Amendment: amendment, Budget: *b, Authority: authority}, nil
	})
}
