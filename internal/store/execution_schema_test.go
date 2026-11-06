package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func foundationDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema); e != nil {
		t.Fatal(e)
	}
	p := domain.Policy{Mode: "autonomous", SelectionSource: "default", Revision: "1", UpdatedAt: domain.Now()}
	if _, e = db.Exec("INSERT INTO policy VALUES(1,1,?)", encode(p)); e != nil {
		t.Fatal(e)
	}
	return db
}
func TestExecutionMigrationRetainsFoundationReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db := foundationDatabase(t, path)
	id := domain.NewID()
	original := domain.Success(200, id, domain.Policy{Mode: "supervised", Revision: "9007199254740993"}, &domain.Receipt{RequestID: id, Operation: "policy.set", CommittedAt: domain.Now()})
	if _, e := db.Exec("INSERT INTO receipts VALUES(?,?,?,?,?)", id, "policy.set", "original-input", 200, encode(original.Envelope)); e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var retained string
	if e = s.db.QueryRow("SELECT envelope FROM receipts WHERE request_id=?", id).Scan(&retained); e != nil || retained != encode(original.Envelope) {
		t.Fatal("foundation receipt bytes changed", e)
	}
	var version int
	if e = s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != 5 {
		t.Fatal(version, e)
	}
	r, e := s.Replay(context.Background(), id, "policy.set", "original-input")
	if e != nil || r == nil || r.Envelope.Data.(map[string]any)["revision"] != "9007199254740993" {
		t.Fatal(r, e)
	}
}
func TestFailedExecutionMigrationRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db := foundationDatabase(t, path)
	if _, e := db.Exec("CREATE TABLE attempts(sentinel TEXT); INSERT INTO attempts VALUES('retained')"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	if _, e := Open(path); e == nil {
		t.Fatal("migration collision accepted")
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var version, count int
	var value string
	db.QueryRow("PRAGMA user_version").Scan(&version)
	db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='tasks'").Scan(&count)
	db.QueryRow("SELECT sentinel FROM attempts").Scan(&value)
	if version != 1 || count != 0 || value != "retained" {
		t.Fatal(version, count, value)
	}
}
