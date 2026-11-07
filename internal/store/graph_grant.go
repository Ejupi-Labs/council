package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) amendGraphBudget(ctx context.Context, ownerID, graphID string, in domain.GraphBudgetAmendmentInput) domain.Result {
	if !domain.ValidID(graphID) || !domain.ValidID(ownerID) {
		return domain.Failure(in.RequestID, domain.Invalid("exact graph and owner identity"))
	}
	return s.mutate(ctx, in.RequestID, "graphs.budget.amend", GraphBudgetAmendmentFingerprint(graphID, in), func(tx *sql.Tx) (int, any, error) {
		h, e := s.graphHeader(ctx, tx, graphID)
		if e != nil {
			return 0, nil, e
		}
		g, e := s.gate(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if e = checkGate(g); e != nil {
			return 0, nil, e
		}
		if g.OwnerInstanceID != ownerID {
			return 0, nil, domain.Fail("owner_unavailable", "The graph grant belongs to a different current mutation owner.")
		}
		if h.Summary.ControlRevision != in.ExpectedGraphControlRevision {
			return 0, nil, graphControlError()
		}
		b, e := s.rawTaskBudget(ctx, tx, h.Summary.RootTaskID)
		if e != nil {
			return 0, nil, e
		}
		if b == nil || h.Summary.BudgetAuthorityEpoch != in.ExpectedBudgetAuthorityEpoch || b.CurrentWindow.Number != in.ExpectedWindowNumber {
			return 0, nil, domain.Fail("budget_conflict", "The graph budget authority or window changed. Capture its current epoch before granting units.")
		}
		prior := *b
		at := time.Now().UTC()
		if in.NewWindowSeconds != nil {
			for _, claim := range g.ActiveClaims {
				active, e := s.attempt(ctx, tx, claim.AttemptID)
				if e != nil {
					return 0, nil, e
				}
				if claim.LegacyBarrier || active.OwnerInstanceID != ownerID {
					return 0, nil, domain.Fail("dispatch_fenced", "Uncertain retained ownership prevents opening another graph budget window.")
				}
				if claim.GraphID != nil && *claim.GraphID == graphID {
					return 0, nil, domain.Fail("attempt_active", "The graph still owns a managed claim. Settle its exact ownership before granting another window.")
				}
			}
			var unsettled int
			e = tx.QueryRowContext(ctx, "SELECT count(*) FROM attempts WHERE (task_id=? OR task_id IN (SELECT task_id FROM graph_nodes WHERE graph_id=?)) AND (phase!='settled' OR json_extract(data,'$.cleanup.state')!='verified')", h.Summary.RootTaskID, graphID).Scan(&unsettled)
			if e != nil {
				return 0, nil, storageError()
			}
			if unsettled != 0 {
				return 0, nil, domain.Fail("attempt_active", "A new graph window requires settled root and node operations with verified cleanup.")
			}
			b.CurrentWindow.Number, e = increment(b.CurrentWindow.Number)
			if e != nil {
				return 0, nil, e
			}
			b.CurrentWindow.WindowSeconds = *in.NewWindowSeconds
			b.CurrentWindow.FirstAdmissionAt, b.CurrentWindow.DeadlineAt = nil, nil
			seconds := min(*in.NewWindowSeconds, domain.GraphPreparationSeconds)
			h.PreparationDeadlineAt = at.Add(time.Duration(seconds) * time.Second).Format("2006-01-02T15:04:05.000Z")
		}
		granted, invalid := domain.AddRoutingCounts(b.GrantedAdmissions, in.AdditionalAdmissions)
		if invalid != nil {
			return 0, nil, invalid
		}
		remaining, invalid := domain.SubtractRoutingCounts(granted, b.ConsumedAdmissions)
		if invalid != nil {
			return 0, nil, invalid
		}
		b.GrantedAdmissions, b.RemainingAdmissions = granted, remaining
		b.Revision, e = increment(b.Revision)
		if e != nil {
			return 0, nil, e
		}
		now := at.Format("2006-01-02T15:04:05.000Z")
		b.UpdatedAt = now
		t, e := s.task(ctx, tx, h.Summary.RootTaskID)
		if e != nil {
			return 0, nil, e
		}
		t.Revision, e = increment(t.Revision)
		if e != nil {
			return 0, nil, e
		}
		t.StateRevision, e = increment(t.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		t.UpdatedAt = now
		amendment := domain.BudgetAmendment{ID: domain.NewID(), TaskID: t.ID, RequestID: in.RequestID, Action: "grant", AdditionalAdmissions: in.AdditionalAdmissions, NewWindowSeconds: in.NewWindowSeconds, PriorBudget: &prior, TaskVersion: t.CurrentVersion, CreatedAt: now}
		b.LatestAmendmentID = &amendment.ID
		amendment.Budget = *b
		h.Summary.BudgetAuthorityEpoch, e = increment(h.Summary.BudgetAuthorityEpoch)
		if e != nil {
			return 0, nil, e
		}
		h.Summary.StateRevision, e = increment(h.Summary.StateRevision)
		if e != nil {
			return 0, nil, e
		}
		h.Summary.UpdatedAt = now
		if h.DispatchAuthority != nil {
			h.DispatchAuthority.State = "suspended"
		}
		if h.Summary.Stop == nil && h.Summary.State != "completed" {
			h.Summary.NextOwner = "user"
			h.Summary.NextAction = "Advance the graph explicitly with its current budget authority when ready."
			if h.Summary.State != "running" && h.Summary.State != "preparing" && h.Summary.State != "cancelling" {
				h.Summary.State = "blocked"
			}
		}
		if e = saveBudget(ctx, tx, *b); e != nil {
			return 0, nil, e
		}
		if e = saveTask(ctx, tx, t); e != nil {
			return 0, nil, e
		}
		if e = saveGraphHeader(ctx, tx, h); e != nil {
			return 0, nil, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO budget_amendments VALUES(?,?,?,?)", amendment.ID, t.ID, intRevision(b.Revision), encode(amendment)); e != nil {
			return 0, nil, storageError()
		}
		if e = appendBudgetLedger(ctx, tx, *b, "grant", in.RequestID, nil, in.AdditionalAdmissions, "0"); e != nil {
			return 0, nil, e
		}
		view, e := s.graphBudgetView(ctx, tx, h)
		if e != nil {
			return 0, nil, e
		}
		return 200, domain.GraphBudgetAmendmentResult{Graph: h.Summary, Budget: view, Amendment: amendment}, nil
	})
}
