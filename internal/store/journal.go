package store

import (
	"context"
	"database/sql"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// configureDurableJournal runs only after application/schema validation. The
// lifetime owner has already checked the database and retained companion files.
// WAL/FULL keeps a synchronized commit acknowledgement while avoiding rollback
// journal rewrites. Refuse unsupported settings rather than weakening durability.
func configureDurableJournal(ctx context.Context, db *sql.DB) error {
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil || mode != "wal" {
		return storageError()
	}
	if _, err := db.ExecContext(ctx, "PRAGMA synchronous=FULL"); err != nil {
		return storageError()
	}
	var synchronous int
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil || synchronous != 2 {
		return storageError()
	}
	// Read the mode back after all setters; PRAGMAs can silently decline settings.
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		return storageError()
	}
	return nil
}

// sqliteDSN treats its input as a literal file path, never as caller-supplied DSN
// syntax. Every replacement physical connection receives the same local flags.
// Journal mode is deliberately absent: it is changed only after schema checks.
func sqliteDSN(path string) (string, error) {
	if path == "" || !utf8.ValidString(path) || strings.ContainsRune(path, 0) {
		return "", storageError()
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", storageError()
	}
	uriPath := filepath.ToSlash(absolute)
	if runtime.GOOS == "windows" && len(uriPath) >= 2 && uriPath[1] == ':' {
		uriPath = "/" + uriPath
	}
	query := url.Values{}
	// Acquire the writer lock before reading admission authority. Concurrent
	// clients then wait before capturing a snapshot instead of failing its upgrade.
	query.Set("_txlock", "immediate")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous(2)")
	return (&url.URL{Scheme: "file", Path: uriPath, RawQuery: query.Encode()}).String(), nil
}
