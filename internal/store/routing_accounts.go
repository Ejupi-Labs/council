package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) CreateRoutingAccount(ctx context.Context, in domain.AccountCreateInput) domain.Result {
	if e := domain.ValidateAccountCreate(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.accounts.create", Fingerprint("routing.accounts.create", in), func(tx *sql.Tx) (int, any, error) {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM routing_accounts").Scan(&count); err != nil {
			return 0, nil, storageError()
		}
		if count >= domain.MaxDeclaredAccounts {
			return 0, nil, domain.Fail("routing_unavailable", "The 64 declared-account limit is reached. Preserve capacity history and use an existing declaration.")
		}
		now := domain.Now()
		a := domain.DeclaredAccount{ID: domain.NewID(), Label: in.Label, Entitlement: in.Entitlement, Revision: "1", CreatedAt: now, UpdatedAt: now}
		if a.Entitlement == "" {
			a.Entitlement = "unknown"
		}
		if in.ProviderLabel != "" {
			a.ProviderLabel = ptr(in.ProviderLabel)
		}
		seq, err := nextSequence(ctx, tx, "routing_accounts")
		if err != nil {
			return 0, nil, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO routing_accounts VALUES(?,?,1,?)", seq, a.ID, encode(a)); err != nil {
			return 0, nil, storageError()
		}
		state := domain.CapacityState{ScopeKey: "account:" + a.ID, Subject: domain.CapacitySubject{Kind: "account", ID: a.ID}, Revision: "1", RetainedNegatives: []domain.RetainedCapacityReport{}}
		if _, err = tx.ExecContext(ctx, "INSERT INTO routing_capacity_state VALUES(?,NULL,?,1,?)", state.ScopeKey, a.ID, encode(state)); err != nil {
			return 0, nil, storageError()
		}
		return 201, a, nil
	})
}
func (s *Store) UpdateRoutingAccount(ctx context.Context, id string, in domain.AccountUpdateInput) domain.Result {
	if e := domain.ValidateAccountUpdate(id, in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.accounts.update", routingTargetFingerprint("routing.accounts.update", id, in), func(tx *sql.Tx) (int, any, error) {
		var raw string
		err := tx.QueryRowContext(ctx, "SELECT data FROM routing_accounts WHERE id=?", id).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, notFound()
		}
		if err != nil {
			return 0, nil, storageError()
		}
		a, err := decode[domain.DeclaredAccount](raw)
		if err != nil || a.ID != id {
			return 0, nil, storageError()
		}
		if a.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(a.Revision)
		}
		if in.Label != nil {
			a.Label = *in.Label
		}
		if in.ProviderLabel != nil {
			a.ProviderLabel = nil
			if *in.ProviderLabel != "" {
				a.ProviderLabel = ptr(*in.ProviderLabel)
			}
		}
		if in.Entitlement != "" {
			a.Entitlement = in.Entitlement
		}
		a.Revision, err = increment(a.Revision)
		if err != nil {
			return 0, nil, err
		}
		a.UpdatedAt = domain.Now()
		if _, err = tx.ExecContext(ctx, "UPDATE routing_accounts SET revision=?,data=? WHERE id=?", intRevision(a.Revision), encode(a), id); err != nil {
			return 0, nil, storageError()
		}
		return 200, a, nil
	})
}
func (s *Store) UpdateAccountLink(ctx context.Context, id string, in domain.AccountLinkInput) domain.Result {
	if e := domain.ValidateAccountLink(id, in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.links.update", routingTargetFingerprint("routing.links.update", id, in), func(tx *sql.Tx) (int, any, error) {
		snap, err := s.routingSnapshot(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		var link domain.AccountLink
		for _, l := range snap.Links {
			if l.ContextID == id {
				link = l
			}
		}
		if link.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(link.Revision)
		}
		if in.DeclaredAccountID != "" {
			found := false
			for _, a := range snap.Accounts {
				if a.ID == in.DeclaredAccountID {
					found = true
				}
			}
			if !found {
				return 0, nil, notFound()
			}
		}
		oldID := link.DeclaredAccountID
		link.DeclaredAccountID = nil
		if in.DeclaredAccountID != "" {
			link.DeclaredAccountID = ptr(in.DeclaredAccountID)
		}
		link.Revision, err = increment(link.Revision)
		if err != nil {
			return 0, nil, err
		}
		link.UpdatedAt = domain.Now()
		// Inherit actual retained denial IDs, never positive availability or report
		// bindings. The immutable report body still records its original context.
		for i := range snap.CapacityStates {
			state := &snap.CapacityStates[i]
			if state.ScopeKey != "context:"+id {
				continue
			}
			for _, accountID := range []*string{oldID, link.DeclaredAccountID} {
				if accountID == nil {
					continue
				}
				for _, source := range snap.CapacityStates {
					if source.ScopeKey != "account:"+*accountID {
						continue
					}
					for _, negative := range currentNegatives(source, domain.Now()) {
						inheritance := &domain.LinkInheritance{AccountID: *accountID, LinkRevision: link.Revision, LinkedAt: link.UpdatedAt}
						retainCapacityReport(state, negative.Report, inheritance)
					}
				}
			}
			state.Revision, err = increment(state.Revision)
			if err != nil {
				return 0, nil, err
			}
			if err = saveCapacityState(ctx, tx, *state); err != nil {
				return 0, nil, err
			}
		}
		for i := range snap.Links {
			if snap.Links[i].ContextID == id {
				snap.Links[i] = link
			}
		}
		if err = validateCapacityViews(snap, domain.Now()); err != nil {
			return 0, nil, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE routing_account_links SET account_id=?,revision=?,data=? WHERE context_id=?", link.DeclaredAccountID, intRevision(link.Revision), encode(link), id); err != nil {
			return 0, nil, storageError()
		}
		return 200, link, nil
	})
}
func (s *Store) RoutingAccounts(ctx context.Context, limit int, cursor string) (domain.Page[domain.DeclaredAccount], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.DeclaredAccount]{Items: []domain.DeclaredAccount{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("limit")
	}
	max, after, err := pageBounds(ctx, s.db, "routing_accounts", "", cursor)
	if err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT sequence,data FROM routing_accounts WHERE sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", after, max, limit+1)
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
			out.NextCursor = makeCursor("routing_accounts", "", max, last)
			break
		}
		a, e := decode[domain.DeclaredAccount](raw)
		if e != nil {
			return out, storageError()
		}
		out.Items = append(out.Items, a)
		last = seq
	}
	if rows.Err() != nil {
		return out, storageError()
	}
	return out, nil
}

func sortBindings(bindings []string) { sort.Strings(bindings) }
