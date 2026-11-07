package store

import (
	"context"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// GraphClaims exposes every actual bounded claim. A nil sole-ID projection
// cannot prove that the managed pool is empty.
func (s *Store) GraphClaims(ctx context.Context) ([]domain.GraphClaimView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.graphClaims(ctx, s.db)
}
func (s *Store) graphClaims(ctx context.Context, q queryer) ([]domain.GraphClaimView, error) {
	records, err := s.graphClaimRecords(ctx, q)
	if err != nil {
		return nil, err
	}
	views := make([]domain.GraphClaimView, 0, len(records))
	for _, r := range records {
		views = append(views, r.View)
	}
	return views, nil
}
func claimedAttempt(claims []domain.GraphClaimView, attemptID string) bool {
	for _, claim := range claims {
		if claim.AttemptID == attemptID {
			return true
		}
	}
	return false
}

// CheckAttemptClaim checks the exact owner/attempt without acquiring authority.
func (s *Store) CheckAttemptClaim(ctx context.Context, ownerID, attemptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.gate(ctx, s.db)
	if err != nil {
		return err
	}
	if g.OwnerInstanceID != ownerID {
		return domain.Fail("owner_changed", "This execution belongs to another owner instance.")
	}
	if err = checkGate(g); err != nil {
		return err
	}
	return s.checkAttemptClaim(ctx, s.db, ownerID, attemptID)
}
func (s *Store) checkAttemptClaim(ctx context.Context, q queryer, ownerID, attemptID string) error {
	records, err := s.graphClaimRecords(ctx, q)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.View.AttemptID != attemptID {
			continue
		}
		if r.OwnerInstanceID != ownerID {
			return domain.Fail("owner_changed", "The retained claim belongs to another owner instance.")
		}
		return nil
	}
	return domain.Fail("capacity_conflict", "This exact attempt does not own managed capacity.")
}
