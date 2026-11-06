package store

import (
	"context"
	"database/sql"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) deliveryPreference(ctx context.Context, q queryer) (domain.DeliveryPreference, error) {
	var raw string
	var revision int64
	if e := q.QueryRowContext(ctx, "SELECT revision,data FROM delivery_preferences WHERE singleton=1").Scan(&revision, &raw); e != nil {
		return domain.DeliveryPreference{}, storageError()
	}
	p, e := decode[domain.DeliveryPreference](raw)
	if e != nil || !validStoredRevision(revision, p.Revision) || (p.LocalMode != "review_only" && p.LocalMode != "apply_verified_local") || (p.SelectionSource != "default" && p.SelectionSource != "user") {
		return p, storageError()
	}
	return p, nil
}
func (s *Store) DeliveryPreference(ctx context.Context) (domain.DeliveryPreference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliveryPreference(ctx, s.db)
}
func DeliveryPreferenceFingerprint(in domain.DeliveryPreferenceInput) string {
	return Fingerprint("workflow.preferences.update", in)
}
func (s *Store) UpdateDeliveryPreference(ctx context.Context, in domain.DeliveryPreferenceInput) domain.Result {
	if e := domain.ValidateDeliveryPreferenceInput(in); e != nil {
		return domain.Failure(in.RequestID, e)
	}
	return s.mutate(ctx, in.RequestID, "workflow.preferences.update", DeliveryPreferenceFingerprint(in), func(tx *sql.Tx) (int, any, error) {
		p, e := s.deliveryPreference(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if p.Revision != in.ExpectedRevision {
			return 0, nil, domain.RevisionConflict(p.Revision)
		}
		p.Revision, e = increment(p.Revision)
		if e != nil {
			return 0, nil, e
		}
		p.LocalMode = in.LocalMode
		p.SelectionSource = "user"
		p.UpdatedAt = domain.Now()
		if _, e = tx.ExecContext(ctx, "UPDATE delivery_preferences SET revision=?,data=? WHERE singleton=1", intRevision(p.Revision), encode(p)); e != nil {
			return 0, nil, storageError()
		}
		return 200, p, nil
	})
}
