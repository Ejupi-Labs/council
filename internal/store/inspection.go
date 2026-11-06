package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) SourceSnapshot(ctx context.Context, id string) (domain.SourceSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sourceSnapshot(ctx, s.db, id)
}
func (s *Store) sourceSnapshot(ctx context.Context, q queryer, id string) (domain.SourceSnapshot, error) {
	var raw string
	err := q.QueryRowContext(ctx, "SELECT data FROM source_snapshots WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SourceSnapshot{}, notFound()
	}
	if err != nil {
		return domain.SourceSnapshot{}, storageError()
	}
	return decode[domain.SourceSnapshot](raw)
}
