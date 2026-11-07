// Package store persists preparation records. Its caller must hold the native owner lock.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"sync"

	json "encoding/json/v2"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
	// Deterministic persistence-boundary fault injection; nil in production.
	BeforeCommit  func() error
	AfterCommit   func() error
	SQLiteVersion string
}

func Open(path string) (*Store, error) {
	dsn, err := sqliteDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, storageError()
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	fail := func() (*Store, error) { db.Close(); return nil, storageError() }
	for _, q := range []string{"PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err = db.Exec(q); err != nil {
			return fail()
		}
	}
	if err = db.QueryRow("SELECT sqlite_version()").Scan(&s.SQLiteVersion); err != nil {
		return fail()
	}
	// Reject incompatible stores; an unknown file never triggers a reset or migration.
	var version, appID, tables int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail()
	}
	if err = db.QueryRow("PRAGMA application_id").Scan(&appID); err != nil {
		return fail()
	}
	if s.SQLiteVersion != "3.54.1" {
		return fail()
	}
	if version == 0 && appID == 0 {
		if err = db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name NOT GLOB 'sqlite_*'").Scan(&tables); err != nil || tables != 0 {
			return fail()
		}
		err = s.initializeSchema(context.Background(), true)
	} else if version == 1 && appID == applicationID {
		if err = s.validateFoundation(context.Background()); err != nil {
			return fail()
		}
		err = s.initializeSchema(context.Background(), false)
	} else if version == 2 && appID == applicationID {
		if err = s.validateFoundation(context.Background()); err != nil {
			return fail()
		}
		err = s.migrateSharedCapacity(context.Background())
	} else if version == 3 && appID == applicationID {
		if err = s.validateFoundation(context.Background()); err != nil {
			return fail()
		}
		err = s.migrateRouting(context.Background())
	} else if version == 4 && appID == applicationID {
		err = s.migrateWorkflow(context.Background())
	} else if version == 5 && appID == applicationID {
		err = s.migrateGraphs(context.Background())
	} else if version != schemaVersion || appID != applicationID {
		return fail()
	}
	if err != nil {
		db.Close()
		return nil, err // An actual Commit failure remains uncertain.
	}
	// Accepted migrations retain their exact transaction boundaries and historical
	// row encodings. Only known successive schema versions are advanced.
	for {
		var currentVersion int
		if err = db.QueryRow("PRAGMA user_version").Scan(&currentVersion); err != nil {
			return fail()
		}
		if currentVersion == schemaVersion {
			break
		}
		switch currentVersion {
		case 4:
			err = s.migrateWorkflow(context.Background())
		case 5:
			err = s.migrateGraphs(context.Background())
		default:
			return fail()
		}
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	var fk int
	if err = db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		return fail()
	}
	// Probe expected schema and policy now instead of failing during a later mutation.
	if _, err = s.policy(context.Background(), db); err != nil {
		return fail()
	}
	for _, q := range []string{"SELECT sequence,id,physical_identity,resolved_path,data FROM projects LIMIT 0", "SELECT sequence,id,project_id,revision FROM drafts LIMIT 0", "SELECT draft_id,revision,data FROM draft_versions LIMIT 0", "SELECT request_id,operation,fingerprint,status,envelope FROM receipts LIMIT 0"} {
		rows, e := db.Query(q)
		if e != nil {
			return fail()
		}
		rows.Close()
	}
	if err = s.validateGraphSchema(context.Background()); err != nil {
		return fail()
	}
	if err = configureDurableJournal(context.Background(), db); err != nil {
		return fail()
	}
	return s, nil
}
func (s *Store) Close() error { s.mu.Lock(); defer s.mu.Unlock(); return s.db.Close() }
func storageError() *domain.Error {
	return domain.Fail("storage_failed", "Storage could not complete the operation. Retry with the same request identity and input.")
}
func encode(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		panic("internal contract value cannot be encoded")
	}
	return string(b)
}
func decode[T any](raw string) (T, error) {
	var value T
	e := json.Unmarshal([]byte(raw), &value)
	return value, e
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) replay(ctx context.Context, q queryer, id, operation, fingerprint string) (*domain.Result, error) {
	var savedOperation, savedFingerprint, raw string
	var status int
	e := q.QueryRowContext(ctx, "SELECT operation,fingerprint,status,envelope FROM receipts WHERE request_id=?", id).Scan(&savedOperation, &savedFingerprint, &status, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, storageError()
	}
	if savedOperation != operation || savedFingerprint != fingerprint {
		return nil, domain.Fail("request_conflict", "This request identity belongs to different input. Use a new identity for a new intent.")
	}
	envelope, e := decode[domain.Envelope](raw)
	if e != nil {
		return nil, storageError()
	}
	return &domain.Result{Status: status, Envelope: envelope}, nil
}
func (s *Store) Replay(ctx context.Context, id, operation, fingerprint string) (*domain.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replay(ctx, s.db, id, operation, fingerprint)
}
func (s *Store) mutate(ctx context.Context, id, operation, fingerprint string, change func(*sql.Tx) (int, any, error)) domain.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return interruptedMutation(id)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return mutationBeforeCommitFailure(ctx, id, storageError())
	}
	defer tx.Rollback()
	previous, err := s.replay(ctx, tx, id, operation, fingerprint)
	if err != nil {
		return mutationBeforeCommitFailure(ctx, id, err)
	}
	if previous != nil {
		return *previous
	}
	status, data, err := change(tx)
	if err != nil {
		return mutationBeforeCommitFailure(ctx, id, err)
	}
	if err = ctx.Err(); err != nil {
		return interruptedMutation(id)
	}
	receipt := &domain.Receipt{RequestID: id, Operation: operation, CommittedAt: domain.Now()}
	result := domain.Success(status, id, data, receipt)
	if _, err = tx.ExecContext(ctx, "INSERT INTO receipts(request_id,operation,fingerprint,status,envelope) VALUES(?,?,?,?,?)", id, operation, fingerprint, status, encode(result.Envelope)); err != nil {
		return mutationBeforeCommitFailure(ctx, id, storageError())
	}
	if s.BeforeCommit != nil {
		if err = s.BeforeCommit(); err != nil {
			return mutationBeforeCommitFailure(ctx, id, beforeCommitHookError(ctx, err))
		}
	}
	if err = ctx.Err(); err != nil {
		return interruptedMutation(id)
	}
	if err = tx.Commit(); err != nil {
		return domain.Failure(id, domain.Fail("write_uncertain", "Storage acknowledgement is uncertain. Retry with the same request identity and input."))
	}
	if s.AfterCommit != nil {
		if err = s.AfterCommit(); err != nil {
			return domain.Failure(id, domain.Fail("write_uncertain", "Storage acknowledgement is uncertain. Retry with the same request identity and input."))
		}
	}
	return result
}
func interruptedMutation(id string) domain.Result {
	return domain.Failure(id, domain.Fail("operation_interrupted", "The operation was interrupted before commit. Retry with the same request identity and input."))
}
func mutationBeforeCommitFailure(ctx context.Context, id string, err error) domain.Result {
	if durabilityErrorCode(err) == "write_uncertain" {
		return domain.Failure(id, domain.Fail("write_uncertain", "Persistence is uncertain. Retry with the same request identity and input."))
	}
	if hasJoinedDurabilityFault(err) {
		return domain.Failure(id, storageError())
	}
	if ctx.Err() != nil || cancellationOnly(err) {
		return interruptedMutation(id)
	}
	return domain.Failure(id, domain.SafeError(err))
}
func nextSequence(ctx context.Context, q queryer, table string) (int64, error) {
	if table != "projects" && table != "drafts" && table != "tasks" && table != "attempts" && table != "attempt_events" && table != "artifacts" && table != "routing_accounts" && table != "routing_capacity_reports" && table != "routing_decisions" && table != "budget_ledger" && table != "workflow_steps" && table != "workflow_reviews" && table != "workflow_checks" && table != "workflow_resolutions" && table != "workflow_acceptances" && table != "delivery_operations" && table != "task_graphs" && table != "graph_inputs" && table != "graph_dispatches" && table != "graph_admission_observations" {
		return 0, storageError()
	}
	var n int64
	if e := q.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence),0) FROM "+table).Scan(&n); e != nil {
		return 0, storageError()
	}
	if n == math.MaxInt64 {
		return 0, domain.Fail("revision_exhausted", "The stored sequence limit has been reached. Preserve this state and report the limit.")
	}
	return n + 1, nil
}
func increment(revision string) (string, error) {
	n, e := domain.ParseRevision(revision)
	if e != nil {
		return "", storageError()
	}
	if n == math.MaxInt64 {
		return "", domain.Fail("revision_exhausted", "The revision limit has been reached. Preserve the current saved state.")
	}
	return strconv.FormatInt(n+1, 10), nil
}
func validStoredRevision(n int64, s string) bool { return n >= 1 && strconv.FormatInt(n, 10) == s }
func (s *Store) policy(ctx context.Context, q queryer) (domain.Policy, error) {
	var n int64
	var raw string
	e := q.QueryRowContext(ctx, "SELECT revision,data FROM policy WHERE singleton=1").Scan(&n, &raw)
	if e != nil {
		return domain.Policy{}, storageError()
	}
	p, e := decode[domain.Policy](raw)
	if e != nil || !validStoredRevision(n, p.Revision) || !(p.Mode == "autonomous" || p.Mode == "supervised") || !(p.SelectionSource == "default" || p.SelectionSource == "user") {
		return domain.Policy{}, storageError()
	}
	return p, nil
}
func (s *Store) Policy(ctx context.Context) (domain.Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.policy(ctx, s.db)
}
func (s *Store) SetPolicy(ctx context.Context, id, fp, expected, mode string) domain.Result {
	return s.mutate(ctx, id, "policy.set", fp, func(tx *sql.Tx) (int, any, error) {
		p, e := s.policy(ctx, tx)
		if e != nil {
			return 0, nil, e
		}
		if p.Revision != expected {
			return 0, nil, domain.RevisionConflict(p.Revision)
		}
		revision, e := increment(p.Revision)
		if e != nil {
			return 0, nil, e
		}
		p = domain.Policy{Mode: mode, SelectionSource: "user", Revision: revision, UpdatedAt: domain.Now()}
		n, _ := domain.ParseRevision(revision)
		if _, e = tx.ExecContext(ctx, "UPDATE policy SET revision=?,data=? WHERE singleton=1", n, encode(p)); e != nil {
			return 0, nil, storageError()
		}
		return 200, p, nil
	})
}
func Fingerprint(operation string, value any) string {
	sum := sha256.Sum256([]byte(operation + "\x00" + encode(value)))
	return hex.EncodeToString(sum[:])
}
