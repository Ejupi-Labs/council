package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/routing"
)

func routingRows[T any](ctx context.Context, q schemaQueryer, statement string, args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, storageError()
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, storageError()
		}
		item, err := decode[T](raw)
		if err != nil {
			return nil, storageError()
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		return nil, storageError()
	}
	return items, nil
}

func (s *Store) routingPolicy(ctx context.Context, q queryer) (domain.RoutingPolicy, error) {
	var revision int64
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT revision,data FROM routing_policy WHERE singleton=1").Scan(&revision, &raw); err != nil {
		return domain.RoutingPolicy{}, storageError()
	}
	p, err := decode[domain.RoutingPolicy](raw)
	if err != nil || !validStoredRevision(revision, p.Revision) {
		return p, storageError()
	}
	return p, nil
}
func (s *Store) routingSnapshot(ctx context.Context, q schemaQueryer) (domain.RoutingSnapshot, error) {
	var out domain.RoutingSnapshot
	var err error
	if out.Policy, err = s.routingPolicy(ctx, q); err != nil {
		return out, err
	}
	if out.Contexts, err = routingRows[domain.NativeContext](ctx, q, "SELECT data FROM routing_contexts ORDER BY id"); err != nil {
		return out, err
	}
	if out.Profiles, err = routingRows[domain.TargetProfile](ctx, q, "SELECT data FROM routing_profiles ORDER BY id"); err != nil {
		return out, err
	}
	if out.Accounts, err = routingRows[domain.DeclaredAccount](ctx, q, "SELECT data FROM routing_accounts ORDER BY sequence"); err != nil {
		return out, err
	}
	if out.Links, err = routingRows[domain.AccountLink](ctx, q, "SELECT data FROM routing_account_links ORDER BY context_id"); err != nil {
		return out, err
	}
	if out.CapacityStates, err = routingRows[domain.CapacityState](ctx, q, "SELECT data FROM routing_capacity_state ORDER BY scope_key"); err != nil {
		return out, err
	}
	if len(out.Contexts) != 2 || len(out.Profiles) != 6 || len(out.Accounts) > domain.MaxDeclaredAccounts || len(out.Links) != 2 || len(out.CapacityStates) != 2+len(out.Accounts) {
		return out, storageError()
	}
	for _, v := range out.Contexts {
		if !domain.ValidContextID(v.ID) {
			return out, storageError()
		}
	}
	for _, v := range out.Profiles {
		if !domain.ValidProfileID(v.ID) || !domain.ValidContextID(v.ContextID) {
			return out, storageError()
		}
	}
	for _, v := range out.CapacityStates {
		if domain.ValidateCapacitySubject(v.Subject) != nil || v.ScopeKey != v.Subject.ScopeKey() || len(v.RetainedNegatives) > 198 || len(encode(v)) > 1<<20 {
			return out, storageError()
		}
	}
	g, err := s.gate(ctx, q)
	if err != nil {
		return out, err
	}
	out.ManagedOccupancy = domain.ManagedOccupancy{ResourceID: "managed-native", Limit: 1, ActiveAttemptID: g.ActiveAttemptID}
	if g.ActiveAttemptID != nil {
		a, err := s.attempt(ctx, q, *g.ActiveAttemptID)
		if err != nil {
			return out, err
		}
		if a.ResolvedTarget != nil {
			out.ManagedOccupancy.ActiveContextID = ptr(a.ResolvedTarget.ContextID)
		} else if domain.ValidContextID(a.ConnectionID) {
			out.ManagedOccupancy.ActiveContextID = ptr(a.ConnectionID)
		}
	}
	return out, nil
}
func (s *Store) RoutingSnapshot(ctx context.Context) (domain.RoutingSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routingSnapshot(ctx, s.db)
}
func (s *Store) RoutingPolicy(ctx context.Context) (domain.RoutingPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routingPolicy(ctx, s.db)
}
func (s *Store) RoutingContexts(ctx context.Context) (domain.Page[domain.ContextView], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.ContextView]{Items: []domain.ContextView{}}
	snap, err := s.routingSnapshot(ctx, s.db)
	if err != nil {
		return out, err
	}
	for _, c := range snap.Contexts {
		v := domain.ContextView{NativeContext: c}
		for _, l := range snap.Links {
			if l.ContextID == c.ID {
				v.AccountLink = l
			}
		}
		capacity, e := routing.Capacity(snap, domain.CapacitySubject{Kind: "context", ID: c.ID}, domain.Now())
		if e != nil {
			return out, e
		}
		v.Capacity = capacity
		out.Items = append(out.Items, v)
	}
	return out, nil
}
func (s *Store) RoutingProfiles(ctx context.Context, contextID string) (domain.Page[domain.TargetProfile], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.TargetProfile]{Items: []domain.TargetProfile{}}
	if contextID != "" && !domain.ValidContextID(contextID) {
		return out, domain.Invalid("contextId")
	}
	items, err := routingRows[domain.TargetProfile](ctx, s.db, "SELECT data FROM routing_profiles WHERE (?='' OR context_id=?) ORDER BY id", contextID, contextID)
	out.Items = items
	return out, err
}
func (s *Store) Capacity(ctx context.Context, subject domain.CapacitySubject) (domain.CapacityView, error) {
	if e := domain.ValidateCapacitySubject(subject); e != nil {
		return domain.CapacityView{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.routingSnapshot(ctx, s.db)
	if err != nil {
		return domain.CapacityView{}, err
	}
	v, e := routing.Capacity(snap, subject, domain.Now())
	if e != nil {
		return v, e
	}
	return v, nil
}
func (s *Store) routingAuthority(ctx context.Context, q queryer, taskID, version string) (*domain.TaskRoutingAuthority, error) {
	var raw string
	err := q.QueryRowContext(ctx, "SELECT data FROM task_routing_versions WHERE task_id=? AND task_version=?", taskID, intRevision(version)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, storageError()
	}
	a, err := decode[domain.TaskRoutingAuthority](raw)
	if err != nil || a.TaskID != taskID || a.TaskVersion != version {
		return nil, storageError()
	}
	return &a, nil
}
func (s *Store) taskBudget(ctx context.Context, q queryer, taskID string) (*domain.TaskBudget, error) {
	var raw string
	var revision, granted, consumed, automatic, historical, window int64
	var first, deadline sql.NullString
	err := q.QueryRowContext(ctx, "SELECT revision,granted,consumed,automatic,historical,window_number,first_admitted_at,deadline_at,data FROM task_budgets WHERE task_id=?", taskID).Scan(&revision, &granted, &consumed, &automatic, &historical, &window, &first, &deadline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, storageError()
	}
	b, err := decode[domain.TaskBudget](raw)
	remaining, e := domain.SubtractRoutingCounts(b.GrantedAdmissions, b.ConsumedAdmissions)
	if err != nil || e != nil || b.TaskID != taskID || !validStoredRevision(revision, b.Revision) || b.GrantedAdmissions != strconv.FormatInt(granted, 10) || b.ConsumedAdmissions != strconv.FormatInt(consumed, 10) || b.AutomaticAdmissions != strconv.FormatInt(automatic, 10) || b.HistoricalAdmissions != strconv.FormatInt(historical, 10) || b.CurrentWindow.Number != strconv.FormatInt(window, 10) || b.RemainingAdmissions != remaining || first.Valid != (b.CurrentWindow.FirstAdmissionAt != nil) || deadline.Valid != (b.CurrentWindow.DeadlineAt != nil) {
		return nil, storageError()
	}
	if first.Valid && (*b.CurrentWindow.FirstAdmissionAt != first.String || *b.CurrentWindow.DeadlineAt != deadline.String) {
		return nil, storageError()
	}
	return &b, nil
}
func saveBudget(ctx context.Context, tx *sql.Tx, b domain.TaskBudget) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO task_budgets VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(task_id) DO UPDATE SET revision=excluded.revision,granted=excluded.granted,consumed=excluded.consumed,automatic=excluded.automatic,historical=excluded.historical,window_number=excluded.window_number,first_admitted_at=excluded.first_admitted_at,deadline_at=excluded.deadline_at,data=excluded.data", b.TaskID, intRevision(b.Revision), countInt(b.GrantedAdmissions), countInt(b.ConsumedAdmissions), countInt(b.AutomaticAdmissions), countInt(b.HistoricalAdmissions), intRevision(b.CurrentWindow.Number), b.CurrentWindow.FirstAdmissionAt, b.CurrentWindow.DeadlineAt, encode(b))
	if err != nil {
		return storageError()
	}
	return nil
}
func countInt(s string) int64 { n, _ := domain.ParseCount(s); return n }
func (s *Store) TaskBudget(ctx context.Context, taskID string) (domain.TaskBudgetView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.task(ctx, s.db, taskID); err != nil {
		return domain.TaskBudgetView{}, err
	}
	b, err := s.taskBudget(ctx, s.db, taskID)
	return domain.TaskBudgetView{Budget: b, AdoptionRequired: b == nil}, err
}
func (s *Store) TaskRouting(ctx context.Context, taskID string) (domain.TaskRoutingView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out domain.TaskRoutingView
	t, err := s.task(ctx, s.db, taskID)
	if err != nil {
		return out, err
	}
	out.Authority, err = s.routingAuthority(ctx, s.db, taskID, t.CurrentVersion)
	if err != nil {
		return out, err
	}
	out.AdoptionRequired = out.Authority == nil
	var raw string
	err = s.db.QueryRowContext(ctx, "SELECT data FROM routing_decisions WHERE task_id=? ORDER BY sequence DESC LIMIT 1", taskID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, storageError()
	}
	d, err := decode[domain.RoutingDecision](raw)
	if err != nil {
		return out, storageError()
	}
	out.LatestDecision = &d
	return out, nil
}
