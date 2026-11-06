package store

import (
	"context"
	"database/sql"

	"encoding/json/jsontext"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func (s *Store) appendEvent(ctx context.Context, tx *sql.Tx, attemptID, kind string, metadata jsontext.Value) (string, error) {
	if len(kind) == 0 || len(kind) > 64 {
		return "", domain.Invalid("bounded event kind")
	}
	for _, c := range kind {
		if !(c >= 'a' && c <= 'z' || c == '_' || c >= '0' && c <= '9') {
			return "", domain.Invalid("event kind")
		}
	}
	if len(metadata) == 0 {
		metadata = jsontext.Value(`{}`)
	}
	if len(metadata) > domain.MaxEventBytes || !metadata.IsValid() || metadata.Kind() != '{' {
		return "", domain.Invalid("event metadata up to 16 KiB")
	}
	seq, e := nextSequence(ctx, tx, "attempt_events")
	if e != nil {
		return "", e
	}
	event := domain.AttemptEvent{Sequence: domain.Decimal(seq), AttemptID: attemptID, Kind: kind, ObservedAt: domain.Now(), Metadata: metadata}
	if _, e = tx.ExecContext(ctx, "INSERT INTO attempt_events(sequence,attempt_id,data) VALUES(?,?,?)", seq, attemptID, encode(event)); e != nil {
		return "", storageError()
	}
	return event.Sequence, nil
}
func (s *Store) Events(ctx context.Context, attemptID string, limit int, cursor string) (domain.Page[domain.AttemptEvent], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := domain.Page[domain.AttemptEvent]{Items: []domain.AttemptEvent{}}
	if limit < 1 || limit > 100 {
		return out, domain.Invalid("limit")
	}
	if _, e := s.attempt(ctx, s.db, attemptID); e != nil {
		return out, e
	}
	max, after, e := pageBounds(ctx, s.db, "attempt_events", attemptID, cursor)
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, "SELECT sequence,data FROM attempt_events WHERE attempt_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?", attemptID, after, max, limit+1)
	if e != nil {
		return out, storageError()
	}
	defer rows.Close()
	var last int64
	for rows.Next() {
		var seq int64
		var raw string
		if e = rows.Scan(&seq, &raw); e != nil {
			return out, storageError()
		}
		if len(out.Items) == limit {
			out.NextCursor = makeCursor("attempt_events", attemptID, max, last)
			break
		}
		v, e := decode[domain.AttemptEvent](raw)
		if e != nil || v.Sequence != domain.Decimal(seq) {
			return out, storageError()
		}
		out.Items = append(out.Items, v)
		last = seq
	}
	if rows.Err() != nil {
		return out, storageError()
	}
	return out, nil
}
