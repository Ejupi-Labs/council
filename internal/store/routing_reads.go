package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

func routingHistoryPage[T any](ctx context.Context, q schemaQueryer, table, filter, column string, limit int, cursor string) (domain.Page[T], error) {
	out := domain.Page[T]{Items: []T{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("limit")
	}
	// This helper is private; identifiers below are authored constants, never
	// request fields. Cursor kind/filter binds the exact same ordered query.
	if !((table == "routing_decisions" || table == "budget_ledger") && column == "task_id" || table == "routing_capacity_reports" && column == "scope_key") {
		return out, storageError()
	}
	max, after, err := pageBounds(ctx, q, table, filter, cursor)
	if err != nil {
		return out, err
	}
	rows, err := q.QueryContext(ctx, "SELECT sequence,data FROM "+table+" WHERE "+column+"=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", filter, after, max, limit+1)
	if err != nil {
		return out, storageError()
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq int64
		var raw string
		if rows.Scan(&seq, &raw) != nil {
			return out, storageError()
		}
		if len(out.Items) == limit {
			out.NextCursor = makeCursor(table, filter, max, last)
			break
		}
		item, e := decode[T](raw)
		if e != nil {
			return out, storageError()
		}
		out.Items = append(out.Items, item)
		last = seq
	}
	if rows.Err() != nil {
		return out, storageError()
	}
	return out, nil
}
func (s *Store) CapacityReports(ctx context.Context, subject domain.CapacitySubject, limit int, cursor string) (domain.Page[domain.CapacityReport], error) {
	if e := domain.ValidateCapacitySubject(subject); e != nil {
		return domain.Page[domain.CapacityReport]{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var exists int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM routing_capacity_state WHERE scope_key=?", subject.ScopeKey()).Scan(&exists); err != nil {
		return domain.Page[domain.CapacityReport]{}, storageError()
	}
	if exists != 1 {
		return domain.Page[domain.CapacityReport]{}, notFound()
	}
	return routingHistoryPage[domain.CapacityReport](ctx, s.db, "routing_capacity_reports", subject.ScopeKey(), "scope_key", limit, cursor)
}
func (s *Store) RoutingDecisions(ctx context.Context, taskID string, limit int, cursor string) (domain.Page[domain.RoutingDecision], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.task(ctx, s.db, taskID); err != nil {
		return domain.Page[domain.RoutingDecision]{}, err
	}
	return routingHistoryPage[domain.RoutingDecision](ctx, s.db, "routing_decisions", taskID, "task_id", limit, cursor)
}
func (s *Store) BudgetLedger(ctx context.Context, taskID string, limit int, cursor string) (domain.Page[domain.BudgetLedgerEntry], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.task(ctx, s.db, taskID); err != nil {
		return domain.Page[domain.BudgetLedgerEntry]{}, err
	}
	return routingHistoryPage[domain.BudgetLedgerEntry](ctx, s.db, "budget_ledger", taskID, "task_id", limit, cursor)
}

func (s *Store) Usage(ctx context.Context, taskID string, limit int, cursor string) (domain.UsagePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var empty domain.UsagePage
	if limit < 1 || limit > 100 {
		return empty, domain.Invalid("limit")
	}
	if _, err := s.task(ctx, s.db, taskID); err != nil {
		return empty, err
	}
	kind := "attempts"
	filter := "usage:" + taskID
	max, after, err := pageBounds(ctx, s.db, kind, filter, cursor)
	if err != nil {
		return empty, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT sequence,data FROM attempts WHERE task_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", taskID, after, max, limit+1)
	if err != nil {
		return empty, storageError()
	}
	items := []domain.UsageItem{}
	attempts := []domain.Attempt{}
	sequences := []int64{}
	more := false
	for rows.Next() {
		var seq int64
		var raw string
		if rows.Scan(&seq, &raw) != nil {
			rows.Close()
			return empty, storageError()
		}
		if len(items) == limit {
			more = true
			break
		}
		a, e := decode[domain.Attempt](raw)
		if e != nil || a.TaskID != taskID {
			rows.Close()
			return empty, storageError()
		}
		item := domain.UsageItem{AttemptID: a.ID, AttemptSequence: strconv.FormatInt(seq, 10), ExecutionMode: a.ExecutionMode, Outcome: a.Outcome, Usage: a.TargetObservations.Usage, NativeModels: a.TargetObservations.NativeModels, EvidenceArtifactIDs: []string{}}

		items = append(items, item)
		attempts = append(attempts, a)
		sequences = append(sequences, seq)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, storageError()
	}
	references := []domain.UsageReferencePrice{}
	for i, a := range attempts {
		var stdoutID string
		if e := s.db.QueryRowContext(ctx, "SELECT id FROM artifacts WHERE attempt_id=? AND json_extract(data,'$.kind')='stdout' ORDER BY sequence LIMIT 1", a.ID).Scan(&stdoutID); e == nil {
			items[i].EvidenceArtifactIDs = append(items[i].EvidenceArtifactIDs, stdoutID)
		} else if !errors.Is(e, sql.ErrNoRows) {
			return empty, storageError()
		}
		if a.RoutingDecisionID == nil {
			continue
		}
		d, e := s.routingDecision(ctx, s.db, *a.RoutingDecisionID)
		if e != nil {
			return empty, e
		}
		for _, candidate := range d.CandidateAssessments {
			if candidate.ProfileSnapshot == nil || candidate.ProfileSnapshot.ReferencePrice == nil || candidate.ContextID != d.SelectedTarget.ContextID || d.SelectedTarget.ProfileID == nil || candidate.ProfileID == nil || *candidate.ProfileID != *d.SelectedTarget.ProfileID {
				continue
			}
			references = append(references, domain.UsageReferencePrice{AttemptID: a.ID, DecisionID: d.ID, ProfileID: d.SelectedTarget.ProfileID, ReferencePrice: *candidate.ProfileSnapshot.ReferencePrice})
			break
		}
	}
	for {
		var next *string
		if (more || len(items) < len(attempts)) && len(items) > 0 {
			next = makeCursor(kind, filter, max, sequences[len(items)-1])
		}
		page, e := routing.Usage(taskID, items, references, next)
		if e != nil {
			return empty, e
		}
		if len(encode(domain.Envelope{APIVersion: domain.APIVersion, RequestID: "00000000000000000000000000000000", Data: page}))+1 <= domain.MaxUsagePageBytes {
			return page, nil
		}
		if len(items) <= 1 {
			return empty, domain.Fail("routing_unavailable", "A retained usage item exceeds the bounded response envelope. Inspect its exact attempt evidence.")
		}
		items = items[:len(items)-1]
	}
}
