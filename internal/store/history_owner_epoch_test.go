package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryOwnerEpochRetainsUnsettledPublication(t *testing.T) {
	for _, state := range []string{"effect_started", "uncertain"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			owner, project, snapshot, preview, operation, request := domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID(), domain.NewID()
			if err := s.BeginOwnerEpoch(ctx, owner); err != nil {
				t.Fatal(err)
			}
			if err := s.RecoverOwner(ctx, owner); err != nil {
				t.Fatal(err)
			}
			for _, q := range []struct {
				query string
				args  []any
			}{
				{"INSERT INTO projects(sequence,id,physical_identity,resolved_path,data) VALUES(1,?,?,?,?)", []any{project, "fake-identity", "fake-project", "{}"}},
				{"INSERT INTO history_snapshots(id,project_id,revision,data,cursor_key) VALUES(?,?,1,?,?)", []any{snapshot, project, "{}", make([]byte, 32)}},
				{"INSERT INTO history_previews(id,snapshot_id,revision,data) VALUES(?,?,1,?)", []any{preview, snapshot, "{}"}},
				{"INSERT INTO history_export_operations(id,request_id,preview_id,snapshot_id,owner_instance_id,owner_generation,revision,state,frozen,journal_revision,data) VALUES(?,?,?,?,?,1,1,?,0,0,?)", []any{operation, request, preview, snapshot, owner, state, "{}"}},
			} {
				if _, err := s.db.ExecContext(ctx, q.query, q.args...); err != nil {
					t.Fatal(err)
				}
			}
			if r := s.Shutdown(ctx, owner, domain.NewID()); r.Envelope.Error != nil {
				t.Fatal(r)
			}
			clean, err := s.CompleteOwnerEpoch(ctx, owner)
			if err != nil || clean {
				t.Fatalf("unresolved history effect %s allowed a clean epoch: clean=%v err=%v", state, clean, err)
			}
			epoch, err := readRecoveryOwnerEpoch(ctx, s.db)
			if err != nil || epoch.State != "dirty" || epoch.OwnerID != owner {
				t.Fatalf("uncertain effect did not retain its dirty owner: %+v err=%v", epoch, err)
			}
		})
	}
}
