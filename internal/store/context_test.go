package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func assertConnectionSettings(t *testing.T, s *Store) {
	t.Helper()
	for pragma, want := range map[string]int{"foreign_keys": 1, "synchronous": 2, "busy_timeout": 5000} {
		var got int
		if err := s.db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil || got != want {
			t.Fatalf("Connection %s=%d, want %d: %v", pragma, got, want, err)
		}
	}
	assertDurableJournal(t, s)
}
func updateContextPolicy(ctx context.Context, tx *sql.Tx) error {
	p := domain.Policy{Mode: "supervised", SelectionSource: "user", Revision: "2", UpdatedAt: domain.Now()}
	_, err := tx.ExecContext(ctx, "UPDATE policy SET revision=2,data=? WHERE singleton=1", encode(p))
	return err
}
func assertPolicyRevision(t *testing.T, s *Store, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := s.Policy(ctx)
	if err != nil || p.Revision != want {
		t.Fatal(p, err)
	}
}
func TestContextCanceledBeforeTransactionDoesNotBecomeStorageFailure(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := s.executionWrite(ctx, func(*sql.Tx) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal("Canceled Begin was not distinguished", called, err)
	}
	deadline, release := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer release()
	err = s.executionWrite(deadline, func(*sql.Tx) error { t.Fatal("Expired transaction callback ran"); return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	r := s.SetPolicy(ctx, domain.NewID(), "canceled-before-begin", "1", "supervised")
	if r.Envelope.Error == nil || r.Envelope.Error.Code != "operation_interrupted" || r.Envelope.Receipt != nil {
		t.Fatal(r)
	}
	assertPolicyRevision(t, s, "1")
}
func TestContextCanceledInsideCallbackRollsBackWithoutCommit(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	after := false
	s.AfterCommit = func() error { after = true; return nil }
	defer func() { s.AfterCommit = nil }()
	err := s.executionWrite(ctx, func(tx *sql.Tx) error {
		if err := updateContextPolicy(ctx, tx); err != nil {
			return err
		}
		cancel()
		return errors.New("callback did not reach commit")
	})
	if !errors.Is(err, context.Canceled) || after {
		t.Fatal("A known callback cancellation became persistence uncertainty", err, after)
	}
	assertPolicyRevision(t, s, "1")
	assertConnectionSettings(t, s)
}
func TestContextCanceledAtPreCommitRemainsInterrupted(t *testing.T) {
	for _, public := range []bool{false, true} {
		name := "internal"
		if public {
			name = "public"
		}
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			committed := false
			s.BeforeCommit = func() error { cancel(); return nil }
			s.AfterCommit = func() error { committed = true; return nil }
			defer func() { s.BeforeCommit = nil; s.AfterCommit = nil }()
			if public {
				request := domain.NewID()
				r := s.SetPolicy(ctx, request, "cancel-before-commit", "1", "supervised")
				if r.Envelope.Error == nil || r.Envelope.Error.Code != "operation_interrupted" || r.Envelope.RequestID != request || r.Envelope.Receipt != nil {
					t.Fatal(r)
				}
			} else {
				err := s.executionWrite(ctx, func(tx *sql.Tx) error { return updateContextPolicy(ctx, tx) })
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
			if committed {
				t.Fatal("Commit was attempted after known cancellation")
			}
			assertPolicyRevision(t, s, "1")
		})
	}
}
func TestContextCommitAndAfterCommitErrorsRemainUncertain(t *testing.T) {
	for _, public := range []bool{false, true} {
		name := "internal"
		if public {
			name = "public"
		}
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			var transaction *sql.Tx
			s.BeforeCommit = func() error { return transaction.Rollback() }
			defer func() { s.BeforeCommit = nil }()
			if public {
				request := domain.NewID()
				r := s.mutate(context.Background(), request, "context-test-commit", "commit-attempt", func(tx *sql.Tx) (int, any, error) {
					transaction = tx
					return 200, map[string]string{"result": "never acknowledged"}, nil
				})
				if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" || r.Envelope.Receipt != nil {
					t.Fatal("An actual Commit error was weakened", r)
				}
			} else {
				err := s.executionWrite(context.Background(), func(tx *sql.Tx) error { transaction = tx; return nil })
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "write_uncertain" {
					t.Fatal("An actual Commit error was weakened", err)
				}
			}
		})
	}
	for _, public := range []bool{false, true} {
		name := "internal_after_commit"
		if public {
			name = "public_after_commit"
		}
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.AfterCommit = func() error { cancel(); return context.Canceled }
			defer func() { s.AfterCommit = nil }()
			request := domain.NewID()
			if public {
				r := s.SetPolicy(ctx, request, "commit-before-context-cancel", "1", "supervised")
				if r.Envelope.Error == nil || r.Envelope.Error.Code != "write_uncertain" || r.Envelope.RequestID != request || r.Envelope.Receipt != nil {
					t.Fatal("Postcommit uncertainty became interruption", r)
				}
			} else {
				err := s.executionWrite(ctx, func(tx *sql.Tx) error { return updateContextPolicy(ctx, tx) })
				var de *domain.Error
				if !errors.As(err, &de) || de.Code != "write_uncertain" {
					t.Fatal("Postcommit uncertainty became interruption", err)
				}
			}
			assertPolicyRevision(t, s, "2")
			s.AfterCommit = nil
			if public {
				r := s.SetPolicy(context.Background(), request, "commit-before-context-cancel", "1", "supervised")
				if r.Status != 200 || r.Envelope.Receipt == nil {
					t.Fatal("Committed intent did not reconcile by identity", r)
				}
			}
		})
	}
}
func TestSQLiteConnectionReplacementRetainsFKFullAndBusyTimeout(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.BeforeCommit = func() error { cancel(); return nil }
	err := s.executionWrite(ctx, func(tx *sql.Tx) error { return updateContextPolicy(ctx, tx) })
	s.BeforeCommit = nil
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertPolicyRevision(t, s, "1")
	assertConnectionSettings(t, s)
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"PRAGMA foreign_keys=OFF", "PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=1"} {
		if _, err = conn.ExecContext(context.Background(), q); err != nil {
			conn.Close()
			t.Fatal(err)
		}
	}
	err = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
	if !errors.Is(err, driver.ErrBadConn) {
		t.Fatal("Physical connection was not discarded", err)
	}
	assertConnectionSettings(t, s)
	_, err = s.db.Exec("INSERT INTO drafts(sequence,id,project_id,revision) VALUES(1,?,?,1)", domain.NewID(), domain.NewID())
	if err == nil {
		t.Fatal("Replacement connection accepted an orphan foreign key")
	}
}
func TestSQLiteLiteralSpecialPathsCannotInjectDSNOptions(t *testing.T) {
	names := []string{"state café # % &_pragma=foreign_keys(0).sqlite3"}
	if runtime.GOOS != "windows" {
		names = append(names, "state?mode=memory&_pragma=foreign_keys(0)#literal.sqlite3")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			dsn, err := sqliteDSN(path)
			if err != nil {
				t.Fatal(err)
			}
			uri, err := url.Parse(dsn)
			if err != nil || uri.Scheme != "file" || uri.Fragment != "" || uri.Query().Get("mode") != "" || len(uri.Query()["_pragma"]) != 3 {
				t.Fatal("Literal filename became DSN authority", dsn, err)
			}
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			assertConnectionSettings(t, s)
			var opened string
			if err = s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&opened); err != nil {
				t.Fatal(err)
			}
			expected, err := os.Stat(path)
			if err != nil {
				t.Fatal("Intended literal database was not created", err)
			}
			actual, err := os.Stat(opened)
			if err != nil || !os.SameFile(expected, actual) {
				t.Fatal("URI redirected the database path", err)
			}
		})
	}
}

func TestContextExplicitDurabilityFaultWinsOverCancellation(t *testing.T) {
	for _, public := range []bool{false, true} {
		for _, kind := range []string{"callback_uncertain", "callback_joined_storage", "hook_joined_fault", "hook_pure_cancellation"} {
			name := kind
			if public {
				name = "public_" + name
			}
			t.Run(name, func(t *testing.T) {
				s := testStore(t)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				defer func() { s.BeforeCommit = nil }()
				want := "storage_failed"
				if kind == "callback_uncertain" {
					want = "write_uncertain"
				}
				if kind == "hook_pure_cancellation" {
					want = "operation_interrupted"
				}
				callback := func(tx *sql.Tx) error {
					if err := updateContextPolicy(ctx, tx); err != nil {
						return err
					}
					switch kind {
					case "callback_uncertain":
						cancel()
						return errors.Join(context.Canceled, domain.Fail("write_uncertain", "An explicit earlier durability acknowledgement is unknown."))
					case "callback_joined_storage":
						cancel()
						return errors.Join(context.Canceled, storageError())
					}
					return nil
				}
				if kind == "hook_joined_fault" {
					s.BeforeCommit = func() error {
						cancel()
						return errors.Join(context.Canceled, errors.New("explicit injected persistence fault"))
					}
				}
				if kind == "hook_pure_cancellation" {
					s.BeforeCommit = func() error { cancel(); return errors.Join(context.Canceled) }
				}
				if public {
					r := s.mutate(ctx, domain.NewID(), "context-explicit-fault", name, func(tx *sql.Tx) (int, any, error) {
						err := callback(tx)
						return 200, map[string]bool{"acknowledged": true}, err
					})
					if r.Envelope.Error == nil || r.Envelope.Error.Code != want || r.Envelope.Receipt != nil {
						t.Fatal("Explicit fault classification changed", want, r)
					}
				} else {
					err := s.executionWrite(ctx, callback)
					if want == "operation_interrupted" {
						if err != context.Canceled {
							t.Fatal("Pure hook cancellation lost exact sentinel", err)
						}
					} else if durabilityErrorCode(err) != want || !errors.Is(err, context.Canceled) {
						t.Fatal("Explicit durability marker was masked", want, err)
					}
				}
				assertPolicyRevision(t, s, "1")
			})
		}
	}
}
