package store

import (
	"context"
	"database/sql"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func routingTargetFingerprint(op, id string, in any) string {
	return Fingerprint(op, struct {
		ID    string
		Input any
	}{id, in})
}
func (s *Store) SetRoutingPolicy(ctx context.Context, in domain.RoutingPolicyInput) domain.Result {
	if e := domain.ValidateRoutingPolicyInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.policy.update", Fingerprint("routing.policy.update", in), func(tx *sql.Tx) (int, any, error) {
		p, err := s.routingPolicy(ctx, tx)
		if err != nil {
			return 0, nil, err
		}
		if p.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(p.Revision)
		}
		if in.EconomyOrder != nil {
			p.EconomyOrder = append([]string{}, (*in.EconomyOrder)...)
		}
		if in.UnknownAvailability != "" {
			p.UnknownAvailability = in.UnknownAvailability
		}
		if in.DefaultBudget != nil {
			if in.DefaultBudget.AdmissionLimit != "" {
				p.DefaultBudget.AdmissionLimit = in.DefaultBudget.AdmissionLimit
			}
			if in.DefaultBudget.WindowSeconds != nil {
				p.DefaultBudget.WindowSeconds = *in.DefaultBudget.WindowSeconds
			}
		}
		continuation, e := domain.ResolveContinuation(p.Continuation, in.Continuation)
		if e != nil {
			return 0, nil, e
		}
		if e = domain.CheckContinuationCapability(continuation); e != nil {
			return 0, nil, e
		}
		if countInt(continuation.MaxAutomaticAdmissions) > countInt(p.DefaultBudget.AdmissionLimit) {
			return 0, nil, domain.Invalid("automatic admissions no greater than total default admissions")
		}
		p.Continuation = continuation
		p.Revision, err = increment(p.Revision)
		if err != nil {
			return 0, nil, err
		}
		p.UpdatedAt = domain.Now()
		if _, err = tx.ExecContext(ctx, "UPDATE routing_policy SET revision=?,data=? WHERE singleton=1", intRevision(p.Revision), encode(p)); err != nil {
			return 0, nil, storageError()
		}
		return 200, p, nil
	})
}
func (s *Store) UpdateRoutingContext(ctx context.Context, id string, in domain.ContextUpdateInput) domain.Result {
	if e := domain.ValidateContextUpdate(id, in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.context.update", routingTargetFingerprint("routing.context.update", id, in), func(tx *sql.Tx) (int, any, error) {
		var raw string
		if err := tx.QueryRowContext(ctx, "SELECT data FROM routing_contexts WHERE id=?", id).Scan(&raw); err != nil {
			return 0, nil, storageError()
		}
		c, err := decode[domain.NativeContext](raw)
		if err != nil || c.ID != id {
			return 0, nil, storageError()
		}
		if c.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(c.Revision)
		}
		if in.Label != nil {
			c.Label = *in.Label
		}
		if in.Enabled != nil {
			c.Enabled = *in.Enabled
		}
		c.Revision, err = increment(c.Revision)
		if err != nil {
			return 0, nil, err
		}
		c.UpdatedAt = domain.Now()
		if _, err = tx.ExecContext(ctx, "UPDATE routing_contexts SET revision=?,enabled=?,data=? WHERE id=?", intRevision(c.Revision), c.Enabled, encode(c), id); err != nil {
			return 0, nil, storageError()
		}
		return 200, c, nil
	})
}
func (s *Store) UpdateRoutingProfile(ctx context.Context, id string, in domain.ProfileUpdateInput) domain.Result {
	if e := domain.ValidateProfileUpdate(id, in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "routing.profile.update", routingTargetFingerprint("routing.profile.update", id, in), func(tx *sql.Tx) (int, any, error) {
		var raw string
		if err := tx.QueryRowContext(ctx, "SELECT data FROM routing_profiles WHERE id=?", id).Scan(&raw); err != nil {
			return 0, nil, storageError()
		}
		p, err := decode[domain.TargetProfile](raw)
		if err != nil || p.ID != id {
			return 0, nil, storageError()
		}
		if p.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(p.Revision)
		}
		if in.Enabled != nil {
			p.Enabled = *in.Enabled
		}
		if in.ClearReferencePrice != nil {
			p.ReferencePrice = nil
		}
		if in.ReferencePrice != nil {
			r := in.ReferencePrice
			p.ReferencePrice = &domain.ReferencePrice{Currency: r.Currency, InputPerMillion: r.InputPerMillion, OutputPerMillion: r.OutputPerMillion, SourceKind: "user_reference", SourceLabel: r.SourceLabel, ObservedAt: r.ObservedAt}
			if r.Unit != "" {
				p.ReferencePrice.Unit = ptr(r.Unit)
			}
			if r.Basis != "" {
				p.ReferencePrice.Basis = ptr(r.Basis)
			}
			if r.SourceURL != "" {
				p.ReferencePrice.SourceURL = ptr(r.SourceURL)
			}
			if r.ValidUntil != "" {
				p.ReferencePrice.ValidUntil = ptr(r.ValidUntil)
			}
			now, _ := domain.ParseRoutingTime(domain.Now())
			observed, e := domain.ParseRoutingTime(r.ObservedAt)
			if e != nil || observed.After(now) {
				return 0, nil, domain.Invalid("reference observedAt no later than owner time")
			}
		}
		p.Revision, err = increment(p.Revision)
		if err != nil {
			return 0, nil, err
		}
		p.UpdatedAt = domain.Now()
		if _, err = tx.ExecContext(ctx, "UPDATE routing_profiles SET revision=?,enabled=?,data=? WHERE id=?", intRevision(p.Revision), p.Enabled, encode(p), id); err != nil {
			return 0, nil, storageError()
		}
		return 200, p, nil
	})
}
