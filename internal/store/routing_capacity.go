package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

func capacityNegative(state string) bool {
	switch state {
	case "busy", "exhausted", "temporary_throttle", "temporary_overload", "authentication_denied", "model_denied", "permission_denied":
		return true
	}
	return false
}
func capacityHead(s *domain.CapacityState, dimension string) **domain.CapacityReport {
	switch dimension {
	case "occupancy":
		return &s.Heads.Occupancy
	case "allowance":
		return &s.Heads.Allowance
	default:
		return &s.Heads.Service
	}
}
func activeNegative(r domain.CapacityReport, now string) bool {
	if !capacityNegative(r.State) {
		return false
	}
	if r.State == "exhausted" {
		return true
	}
	at, e := domain.ParseRoutingTime(now)
	if e != nil {
		return false
	}
	if r.ExpiresAt == nil {
		return false
	}
	expiry, e := domain.ParseRoutingTime(*r.ExpiresAt)
	if e != nil || !at.Before(expiry) {
		return false
	}
	if r.RetryAfter != nil {
		until, e := domain.ParseRoutingTime(*r.RetryAfter)
		if e != nil || !at.Before(until) {
			return false
		}
	}
	return true
}
func currentNegatives(s domain.CapacityState, now string) []domain.RetainedCapacityReport {
	out := []domain.RetainedCapacityReport{}
	seen := map[string]bool{}
	for _, n := range s.RetainedNegatives {
		if !seen[n.Report.ID] && activeNegative(n.Report, now) {
			out = append(out, n)
			seen[n.Report.ID] = true
		}
	}
	for _, r := range []*domain.CapacityReport{s.Heads.Occupancy, s.Heads.Allowance, s.Heads.Service} {
		if r != nil && !seen[r.ID] && activeNegative(*r, now) {
			out = append(out, domain.RetainedCapacityReport{Report: *r})
			seen[r.ID] = true
		}
	}
	return out
}
func retainCapacityReport(s *domain.CapacityState, r domain.CapacityReport, inherited *domain.LinkInheritance) {
	for _, n := range s.RetainedNegatives {
		if n.Report.ID == r.ID {
			return
		}
	}
	s.RetainedNegatives = append(s.RetainedNegatives, domain.RetainedCapacityReport{Report: r, InheritedFrom: inherited})
}
func saveCapacityState(ctx context.Context, tx *sql.Tx, s domain.CapacityState) error {
	if len(s.RetainedNegatives) > 198 || len(encode(s)) > 1<<20 {
		return domain.Fail("report_conflict", "The bounded retained report set is full. Correct an exact retained report before adding another denial.")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE routing_capacity_state SET revision=?,data=? WHERE scope_key=?", intRevision(s.Revision), encode(s), s.ScopeKey); err != nil {
		return storageError()
	}
	return nil
}
func validateCapacityViews(s domain.RoutingSnapshot, now string) error {
	for _, state := range s.CapacityStates {
		if _, e := routing.Capacity(s, state.Subject, now); e != nil {
			return e
		}
	}
	return nil
}
func capacityBindings(s domain.RoutingSnapshot, subject domain.CapacitySubject) []string {
	set := map[string]bool{subject.ScopeKey(): true}
	accountID := ""
	if subject.Kind == "account" {
		accountID = subject.ID
	} else {
		for _, l := range s.Links {
			if l.ContextID == subject.ID && l.DeclaredAccountID != nil {
				accountID = *l.DeclaredAccountID
			}
		}
	}
	if accountID != "" {
		set["account:"+accountID] = true
		for _, l := range s.Links {
			if l.DeclaredAccountID != nil && *l.DeclaredAccountID == accountID {
				set["context:"+l.ContextID] = true
			}
		}
	}
	out := []string{}
	for key := range set {
		out = append(out, key)
	}
	sortBindings(out)
	return out
}
func (s *Store) capacityReport(ctx context.Context, q queryer, id string) (domain.CapacityReport, error) {
	var raw string
	err := q.QueryRowContext(ctx, "SELECT data FROM routing_capacity_reports WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CapacityReport{}, notFound()
	}
	if err != nil {
		return domain.CapacityReport{}, storageError()
	}
	r, err := decode[domain.CapacityReport](raw)
	if err != nil || r.ID != id {
		return r, storageError()
	}
	return r, nil
}
func removeCapacityReport(state *domain.CapacityState, id string) bool {
	changed := false
	out := []domain.RetainedCapacityReport{}
	for _, n := range state.RetainedNegatives {
		if n.Report.ID == id {
			changed = true
		} else {
			out = append(out, n)
		}
	}
	state.RetainedNegatives = out
	for _, head := range []**domain.CapacityReport{&state.Heads.Occupancy, &state.Heads.Allowance, &state.Heads.Service} {
		if *head != nil && (*head).ID == id {
			*head = nil
			changed = true
		}
	}
	return changed
}
func (s *Store) CreateCapacityReport(ctx context.Context, in domain.CapacityReportInput) domain.Result {
	if e := domain.ValidateCapacityReport(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.reports.create", Fingerprint("routing.reports.create", in), func(tx *sql.Tx) (int, any, error) {
		snap, err := s.routingSnapshot(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		scope := in.Subject.ScopeKey()
		index := -1
		for i, state := range snap.CapacityStates {
			if state.ScopeKey == scope {
				index = i
			}
		}
		if index < 0 {
			return 0, nil, notFound()
		}
		if snap.CapacityStates[index].Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(snap.CapacityStates[index].Revision)
		}
		now := domain.Now()
		at, _ := domain.ParseRoutingTime(now)
		observed := in.ObservedAt
		if observed == "" {
			observed = now
		}
		observedAt, e := domain.ParseRoutingTime(observed)
		if e != nil || observedAt.After(at) {
			return 0, nil, domain.Invalid("observedAt no later than owner time")
		}
		if in.RetryAfter != "" {
			until, e := domain.ParseRoutingTime(in.RetryAfter)
			if e != nil || !until.After(at) {
				return 0, nil, domain.Invalid("future retryAfter")
			}
		}
		changed := map[int]bool{}
		if in.CorrectionOf != "" {
			old, e := s.capacityReport(ctx, tx, in.CorrectionOf)
			if e != nil {
				return 0, nil, e
			}
			if old.Subject != in.Subject || old.Dimension != in.Dimension || !capacityNegative(old.State) {
				return 0, nil, domain.Fail("report_conflict", "Correct an exact negative report of the same original subject and dimension.")
			}
			for i := range snap.CapacityStates {
				if removeCapacityReport(&snap.CapacityStates[i], old.ID) {
					changed[i] = true
				}
			}
			if len(changed) == 0 {
				return 0, nil, domain.Fail("report_conflict", "That report is no longer retained. Reload the current exact report bindings before correcting it.")
			}
		}
		validFor := 900
		if in.ValidForSeconds != nil {
			validFor = *in.ValidForSeconds
		}
		expires := observedAt.Add(time.Duration(validFor) * time.Second).Format("2006-01-02T15:04:05.000Z")
		seq, err := nextSequence(ctx, tx, "routing_capacity_reports")
		if err != nil {
			return 0, nil, err
		}
		report := domain.CapacityReport{ID: domain.NewID(), Sequence: strconv.FormatInt(seq, 10), RequestID: in.RequestID, Subject: in.Subject, Dimension: in.Dimension, State: in.State, Source: "user_report", ObservedAt: observed, ReceivedAt: now, ValidForSeconds: validFor, ExpiresAt: &expires, Bindings: capacityBindings(snap, in.Subject), Note: in.Note}
		if in.RetryAfter != "" {
			report.RetryAfter = ptr(in.RetryAfter)
		}
		if in.CorrectionOf != "" {
			report.CorrectionOf = ptr(in.CorrectionOf)
			report.CorrectionAction = ptr(in.CorrectionAction)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO routing_capacity_reports VALUES(?,?,?,?,?)", seq, report.ID, scope, report.Dimension, encode(report)); err != nil {
			return 0, nil, storageError()
		}
		for _, key := range report.Bindings {
			if _, err = tx.ExecContext(ctx, "INSERT INTO routing_report_bindings VALUES(?,?)", report.ID, key); err != nil {
				return 0, nil, storageError()
			}
			for i := range snap.CapacityStates {
				state := &snap.CapacityStates[i]
				if state.ScopeKey != key {
					continue
				}
				state.RetainedNegatives = currentNegatives(*state, now)
				head := capacityHead(state, report.Dimension)
				replace := *head == nil
				if *head != nil {
					oldAt, e := domain.ParseRoutingTime((*head).ObservedAt)
					replace = e != nil || !observedAt.Before(oldAt)
				}
				if replace {
					copy := report
					*head = &copy
				}
				if activeNegative(report, now) {
					retainCapacityReport(state, report, nil)
				}
				changed[i] = true
			}
		}
		for i := range changed {
			state := &snap.CapacityStates[i]
			state.Revision, err = increment(state.Revision)
			if err != nil {
				return 0, nil, err
			}
			if err = saveCapacityState(ctx, tx, *state); err != nil {
				return 0, nil, err
			}
		}
		if err = validateCapacityViews(snap, now); err != nil {
			return 0, nil, err
		}
		capacity, e := routing.Capacity(snap, in.Subject, now)
		if e != nil {
			return 0, nil, e
		}
		return 201, domain.CapacityReportResult{Report: report, Capacity: capacity}, nil
	})
}
func (s *Store) CapacityReport(ctx context.Context, id string) (domain.CapacityReportDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.capacityReport(ctx, s.db, id)
	out := domain.CapacityReportDetail{Report: r, UnresolvedScopeKeys: []string{}}
	if err != nil {
		return out, err
	}
	states, err := routingRows[domain.CapacityState](ctx, s.db, "SELECT data FROM routing_capacity_state ORDER BY scope_key")
	if err != nil {
		return out, err
	}
	for _, state := range states {
		found := false
		for _, n := range state.RetainedNegatives {
			if n.Report.ID == id {
				found = true
			}
		}
		for _, head := range []*domain.CapacityReport{state.Heads.Occupancy, state.Heads.Allowance, state.Heads.Service} {
			if head != nil && head.ID == id && capacityNegative(head.State) {
				found = true
			}
		}
		if found {
			out.UnresolvedScopeKeys = append(out.UnresolvedScopeKeys, state.ScopeKey)
		}
	}
	return out, nil
}
