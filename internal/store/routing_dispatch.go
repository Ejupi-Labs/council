package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

func (s *Store) routingDecision(ctx context.Context, q queryer, id string) (domain.RoutingDecision, error) {
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT data FROM routing_decisions WHERE id=?", id).Scan(&raw); err != nil {
		return domain.RoutingDecision{}, storageError()
	}
	d, err := decode[domain.RoutingDecision](raw)
	if err != nil || d.ID != id || len(raw) > domain.MaxRoutingDecisionBytes {
		return d, storageError()
	}
	return d, nil
}

// checkRoutingControls checks immutable target eligibility, not which target
// would win a new ranking. A changed preference never edits an admitted target.
func (s *Store) checkRoutingControls(ctx context.Context, tx *sql.Tx, a domain.Attempt, facts *domain.RoutingFacts) (bool, []string, error) {
	if a.RoutingDecisionID == nil || a.ResolvedTarget == nil {
		return false, nil, domain.Fail("routing_adoption_required", "This retained attempt has no004 routing authority. Inspect its owner recovery state.")
	}
	d, err := s.routingDecision(ctx, tx, *a.RoutingDecisionID)
	if err != nil {
		return false, nil, err
	}
	if d.AttemptID != a.ID || d.TaskID != a.TaskID || d.TaskVersion != a.TaskVersion || encode(d.SelectedTarget) != encode(*a.ResolvedTarget) || a.ConnectionID != d.SelectedTarget.ConnectionID {
		return false, nil, storageError()
	}
	b, err := s.taskBudget(ctx, tx, a.TaskID)
	if err != nil {
		return false, nil, err
	}
	if b == nil {
		return false, nil, storageError()
	}
	if b.CurrentWindow.DeadlineAt == nil {
		return false, nil, storageError()
	}
	deadline, e := domain.ParseRoutingTime(*b.CurrentWindow.DeadlineAt)
	if e != nil {
		return false, nil, storageError()
	}
	if !time.Now().UTC().Before(deadline) {
		return false, nil, domain.Fail("budget_expired", "The shared task window expired before launch.")
	}
	authority, err := s.routingAuthority(ctx, tx, a.TaskID, a.TaskVersion)
	if err != nil {
		return false, nil, err
	}
	if authority == nil || authority.ID != d.AuthorityID {
		return false, nil, storageError()
	}
	p, e := effectiveRoutingPolicy(authority.PolicySnapshot, authority.OptionSnapshot)
	if e != nil {
		return false, nil, e
	}
	snap, err := s.routingSnapshot(ctx, tx)
	if err != nil {
		return false, nil, err
	}
	snap.Policy = p
	observations := d.EvidenceSnapshot
	if facts != nil {
		observations = *facts
	}
	opts := authority.OptionSnapshot
	opts.TargetProfileID = ""
	if a.ResolvedTarget.ProfileID != nil {
		opts.TargetProfileID = *a.ResolvedTarget.ProfileID
	}
	pins := domain.ExecutionPins{HarnessID: a.ResolvedTarget.HarnessID, Model: a.ResolvedTarget.ModelSelector}
	selection, selectErr := routing.Select(domain.RoutingSelectionInput{Snapshot: snap, Pins: pins, Options: opts, Assessment: authority.SuitabilityAssessment, Facts: observations.Contexts, Now: domain.Now()})
	if selectErr == nil {
		if selection.SelectedTarget.ContextID != a.ResolvedTarget.ContextID || selection.SelectedTarget.ModelSelector != a.ResolvedTarget.ModelSelector {
			return false, nil, storageError()
		}
		return false, nil, nil
	}
	qualified := false
	reports := []string{}
	for _, candidate := range selection.Candidates {
		if candidate.ContextID != a.ResolvedTarget.ContextID {
			continue
		}
		if a.ResolvedTarget.ProfileID != nil && (candidate.ProfileID == nil || *candidate.ProfileID != *a.ResolvedTarget.ProfileID) {
			continue
		}
		for _, reason := range candidate.ReasonCodes {
			switch reason {
			case "native_context_disabled", "profile_disabled", "known_native_version_incompatible":
				qualified = true
			}
		}
	}
	for _, fact := range observations.Contexts {
		if fact.ContextID == a.ResolvedTarget.ContextID && (fact.InstallationStatus == "missing" || fact.InstallationStatus == "unsupported_launcher") {
			qualified = true
		}
	}
	for _, capacity := range selection.CapacityEvidence {
		if capacity.ContextID == a.ResolvedTarget.ContextID {
			if len(capacity.BlockingReportIDs) > 0 {
				qualified = true
			}
			reports = append(reports, capacity.BlockingReportIDs...)
		}
	}
	return qualified, reports, domain.Fail("routing_unavailable", "The frozen admitted target is currently ineligible. Inspect its routing and capacity evidence; the target has not been substituted.")
}
func (s *Store) DispatchRouting(ctx context.Context, id string, facts domain.RoutingFacts) (domain.Attempt, error) {
	var out domain.Attempt
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		a, e := s.attempt(ctx, tx, id)
		if e != nil {
			return e
		}
		out = a
		if a.Phase == "settled" {
			return nil
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return e
		}
		if e = checkGate(g); e != nil {
			return e
		}
		if g.OwnerInstanceID != "" && g.OwnerInstanceID != a.OwnerInstanceID {
			return domain.Fail("dispatch_fenced", "This attempt belongs to another owner epoch.")
		}
		t, e := s.task(ctx, tx, a.TaskID)
		if e != nil {
			return e
		}
		if t.CurrentVersion != a.TaskVersion {
			return domain.Fail("task_version_conflict", "Guidance changed before routing dispatch.")
		}
		if a.Stop != nil {
			return domain.Fail("attempt_active", "An accepted Stop prevents dispatch.")
		}
		qualified, reports, e := s.checkRoutingControls(ctx, tx, a, &facts)
		if e == nil {
			return nil
		}
		var de *domain.Error
		if !errors.As(e, &de) || de.Code != "routing_unavailable" {
			return e
		}
		// Launch intent precedes every process preparation. Once true, only the
		// caller's owned cleanup path may settle/release native ownership.
		if a.LaunchIntent {
			return e
		}
		out, e = s.settle(ctx, tx, a, domain.Settlement{Outcome: "blocked", Cleanup: domain.Cleanup{State: "verified", Scope: "not_launched"}, EvidenceStatus: "incomplete", Failure: &domain.PrimaryFailure{Code: "routing_unavailable", Message: de.Message}})
		if e != nil {
			return e
		}
		if qualified {
			t, e := s.task(ctx, tx, a.TaskID)
			if e != nil {
				return e
			}
			b, e := s.taskBudget(ctx, tx, a.TaskID)
			if e != nil || b == nil {
				if e != nil {
					return e
				}
				return storageError()
			}
			epoch, epochErr := s.graphAttemptBudgetEpoch(ctx, tx, a)
			if epochErr != nil {
				return epochErr
			}
			graphWindow := ""
			if epoch != "" {
				graphWindow = b.CurrentWindow.Number
			}
			proof := domain.ContinuationProof{GraphBudgetAuthorityEpoch: epoch, GraphWindowNumber: graphWindow, AttemptID: a.ID, OwnerInstanceID: a.OwnerInstanceID, TaskID: a.TaskID, TaskVersion: a.TaskVersion, TaskRevision: t.Revision, BudgetRevision: b.Revision, Category: "pre_launch_ineligible", ExpectedOutputMissing: false, EvidenceReportIDs: reports, ReasonCode: "current_target_ineligible", Eligible: true, CreatedAt: domain.Now()}
			if _, e = tx.ExecContext(ctx, "INSERT INTO routing_continuation_proofs VALUES(?,?,?,?) ON CONFLICT(attempt_id) DO NOTHING", a.ID, a.OwnerInstanceID, proof.Category, encode(proof)); e != nil {
				return storageError()
			}
		}
		return nil
	})
	return observe(out), err
}
