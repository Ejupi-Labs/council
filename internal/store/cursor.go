package store

import (
	"context"
	"encoding/base64"
	"strconv"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

type listCursor struct {
	Kind    string `json:"kind"`
	Filter  string `json:"filter"`
	Maximum string `json:"maximum"`
	After   string `json:"after"`
}

func cursorError() *domain.Error {
	return domain.Fail("invalid_cursor", "This list cursor is invalid. Restart the list with the same project filter.")
}
func decodeCursor(raw, kind, filter string) (int64, int64, error) {
	if len(raw) > 2048 {
		return 0, 0, cursorError()
	}
	for _, c := range raw {
		if c > 127 {
			return 0, 0, cursorError()
		}
	}
	decoded, e := base64.RawURLEncoding.Strict().DecodeString(raw)
	if e != nil {
		return 0, 0, cursorError()
	}
	var cursor listCursor
	if e = json.Unmarshal(decoded, &cursor, json.RejectUnknownMembers(true)); e != nil || cursor.Kind != kind || cursor.Filter != filter {
		return 0, 0, cursorError()
	}
	max, e1 := domain.ParseRevision(cursor.Maximum)
	after, e2 := domain.ParseRevision(cursor.After)
	if e1 != nil || e2 != nil || after > max {
		return 0, 0, cursorError()
	}
	// Accept only the canonical format emitted by this server.
	if base64.RawURLEncoding.EncodeToString([]byte(encode(cursor))) != raw {
		return 0, 0, cursorError()
	}
	return max, after, nil
}
func makeCursor(kind, filter string, max, after int64) *string {
	raw := base64.RawURLEncoding.EncodeToString([]byte(encode(listCursor{kind, filter, strconv.FormatInt(max, 10), strconv.FormatInt(after, 10)})))
	return &raw
}
func pageBounds(ctx context.Context, q queryer, kind, filter, raw string) (int64, int64, error) {
	if raw != "" {
		return decodeCursor(raw, kind, filter)
	}
	var max int64
	if e := q.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0) FROM "+kind).Scan(&max); e != nil {
		return 0, 0, storageError()
	}
	return max, 0, nil
}
