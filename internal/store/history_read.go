package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// historySnapshot reads only the Council-generated snapshot, never source data.
// Caller holds s.mu; the cursor key is private and must not enter responses.
func (s *Store) historySnapshot(ctx context.Context, id string) (domain.HistorySnapshot, []byte, error) {
	if !domain.ValidID(id) {
		return domain.HistorySnapshot{}, nil, domain.Invalid("history snapshot identity")
	}
	var projectID string
	var revision int64
	var size int64
	err := s.db.QueryRowContext(ctx, "SELECT project_id,revision,length(CAST(data AS BLOB)) FROM history_snapshots WHERE id=?", id).Scan(&projectID, &revision, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.HistorySnapshot{}, nil, notFound()
	}
	if err != nil || size < 1 || size > domain.HistoryMaxPayloadBytes {
		return domain.HistorySnapshot{}, nil, storageError()
	}
	var raw string
	var key []byte
	if err = s.db.QueryRowContext(ctx, "SELECT data,cursor_key FROM history_snapshots WHERE id=?", id).Scan(&raw, &key); err != nil || int64(len(raw)) != size || len(key) != 32 {
		return domain.HistorySnapshot{}, nil, storageError()
	}
	var snapshot domain.HistorySnapshot
	if err = json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true)); err != nil || revision != 1 || snapshot.Summary.SnapshotID != id || snapshot.Summary.ProjectID != projectID || domain.ValidateHistorySnapshot(snapshot) != nil {
		return domain.HistorySnapshot{}, nil, storageError()
	}
	return snapshot, key, nil
}

func (s *Store) HistorySnapshot(ctx context.Context, id string) (domain.HistorySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, _, err := s.historySnapshot(ctx, id)
	return snapshot, err
}

func historyCursor(key []byte, id, kind string, position int) string {
	data := base64.RawURLEncoding.EncodeToString([]byte("1." + id + "." + kind + "." + strconv.Itoa(position)))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return data + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func parseHistoryCursor(key []byte, raw, id, kind string, total int) (int, error) {
	if len(raw) > domain.HistoryMaxCursorBytes {
		return 0, cursorError()
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return 0, cursorError()
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(parts[0]))
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) || base64.RawURLEncoding.EncodeToString(sig) != parts[1] {
		return 0, cursorError()
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || base64.RawURLEncoding.EncodeToString(payload) != parts[0] {
		return 0, cursorError()
	}
	fields := strings.Split(string(payload), ".")
	if len(fields) != 4 || fields[0] != "1" || fields[1] != id || fields[2] != kind {
		return 0, cursorError()
	}
	position, bad := domain.ParseRevision(fields[3])
	if bad != nil || position >= int64(total) {
		return 0, cursorError()
	}
	return int(position), nil
}

// HistoryRecords uses stable captured rows and an authenticated cursor bound to
// the exact snapshot, revision, section and next position.
func (s *Store) HistoryRecords(ctx context.Context, id string, q domain.HistoryRecordsQuery) (domain.HistorySnapshotPage, error) {
	if err := domain.ValidateHistoryRecordsQuery(q); err != nil {
		return domain.HistorySnapshotPage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, key, err := s.historySnapshot(ctx, id)
	if err != nil {
		return domain.HistorySnapshotPage{}, err
	}
	limit := domain.HistoryDefaultPageSize
	if q.Limit != "" {
		n, _ := domain.ParseRevision(q.Limit)
		limit = int(n)
	}
	total := len(snapshot.Tasks)
	if q.Kind == "attempts" {
		total = len(snapshot.Attempts)
	}
	start := 0
	if q.Cursor != "" {
		start, err = parseHistoryCursor(key, q.Cursor, id, q.Kind, total)
		if err != nil {
			return domain.HistorySnapshotPage{}, err
		}
	}
	end := start + limit
	if end > total {
		end = total
	}
	p := domain.HistorySnapshotPage{SnapshotID: id, Revision: "1", Completeness: domain.HistoryCompleteness, Notice: domain.HistoryNotice, Kind: q.Kind}
	if q.Kind == "tasks" {
		p.Items = snapshot.Tasks[start:end]
	} else {
		p.Items = snapshot.Attempts[start:end]
	}
	if end < total {
		cursor := historyCursor(key, id, q.Kind, end)
		p.NextCursor = &cursor
	}
	if domain.ValidateHistorySnapshotPage(p) != nil {
		return domain.HistorySnapshotPage{}, storageError()
	}
	return p, nil
}
