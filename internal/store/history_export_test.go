package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/historyout"
)

func historyExportFixture(t *testing.T) (*Store, string, domain.PreviewHistoryExportInput, historyout.Preview, string) {
	t.Helper()
	ctx := context.Background()
	s := testStore(t)
	owner := domain.NewID()
	if err := s.BeginOwnerEpoch(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverOwner(ctx, owner); err != nil {
		t.Fatal(err)
	}
	project := openFixture(t, s, "history-export")
	in := domain.CaptureHistoryInput{RequestID: domain.NewID(), ProjectID: project.ID}
	if r := s.CaptureHistory(ctx, in); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	var snapshot string
	if err := s.db.QueryRow("SELECT id FROM history_snapshots WHERE project_id=?", project.ID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	p := domain.PreviewHistoryExportInput{RequestID: domain.NewID(), SnapshotID: snapshot, SnapshotRevision: "1", Filename: "history-work.json"}
	return s, owner, p, historyout.Preview{Name: p.Filename, Root: historyout.RootIdentity{StateRoot: "fake-state-root", ExportRoot: "fake-export-root"}}, filepath.Clean(t.TempDir())
}

func historyCreateFixture(t *testing.T, s *Store, owner string, p domain.PreviewHistoryExportInput, native historyout.Preview, dir string) (domain.ExecuteHistoryExportInput, string) {
	t.Helper()
	ctx := context.Background()
	r := s.PreviewHistoryExport(ctx, owner, p, dir, native)
	if r.Envelope.Error != nil {
		t.Fatal(r)
	}
	var preview string
	if err := s.db.QueryRow("SELECT id FROM history_previews WHERE snapshot_id=?", p.SnapshotID).Scan(&preview); err != nil {
		t.Fatal(err)
	}
	in := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview, PreviewRevision: "1"}
	r = s.CreateHistoryExport(ctx, owner, in)
	if r.Envelope.Error != nil {
		t.Fatal(r)
	}
	var operation string
	if err := s.db.QueryRow("SELECT id FROM history_export_operations WHERE request_id=?", in.RequestID).Scan(&operation); err != nil {
		t.Fatal(err)
	}
	return in, operation
}

func TestHistoryExportPreviewAndCreateReplayAreBound(t *testing.T) {
	ctx := context.Background()
	s, owner, in, native, dir := historyExportFixture(t)
	s.AfterCommit = func() error { return errors.New("lost acknowledgement") }
	first := s.PreviewHistoryExport(ctx, owner, in, dir, native)
	if first.Envelope.Error == nil || first.Envelope.Error.Code != "write_uncertain" {
		t.Fatal(first)
	}
	s.AfterCommit = nil
	// The request replay precedes validation of the stale native observation.
	replay := s.PreviewHistoryExport(ctx, owner, in, "", historyout.Preview{})
	if replay.Status != 201 || replay.Envelope.Receipt == nil {
		t.Fatal(replay)
	}
	var preview string
	if err := s.db.QueryRow("SELECT id FROM history_previews WHERE snapshot_id=?", in.SnapshotID).Scan(&preview); err != nil {
		t.Fatal(err)
	}
	public, private, err := s.HistoryExportPreview(ctx, preview)
	if err != nil || private != native || public.Filename != in.Filename {
		t.Fatalf("preview mismatch %+v %+v %v", public, private, err)
	}
	changed := in
	changed.Filename = "history-other.json"
	if r := s.PreviewHistoryExport(ctx, owner, changed, dir, native); r.Envelope.Error == nil || r.Envelope.Error.Code != "request_conflict" {
		t.Fatal(r)
	}
	create := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: preview, PreviewRevision: "1"}
	first = s.CreateHistoryExport(ctx, owner, create)
	replay = s.CreateHistoryExport(ctx, owner, create)
	if first.Status != 202 || replay.Status != 202 || first.Envelope.Receipt.CommittedAt != replay.Envelope.Receipt.CommittedAt {
		t.Fatalf("create replay %+v %+v", first, replay)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM history_export_operations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("operation count %d: %v", count, err)
	}
}

func TestHistoryExportPermitLostAckNeverReauthorizes(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	s.AfterCommit = func() error { return errors.New("lost permit acknowledgement") }
	effect, err := s.PermitHistoryExport(ctx, owner, id, "1")
	if err == nil || effect.EffectID != "" {
		t.Fatalf("ambiguous permit was usable: %+v %v", effect, err)
	}
	s.AfterCommit = nil
	if again, err := s.PermitHistoryExport(ctx, owner, id, "1"); err == nil || again.EffectID != "" {
		t.Fatalf("permit replayed: %+v %v", again, err)
	}
	op, err := s.HistoryExportOperation(ctx, id)
	if err != nil || op.State != "effect_started" {
		t.Fatalf("lost permit not retained: %+v %v", op, err)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: op.Revision, Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view := r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "permit_present_or_uncertain" {
		t.Fatal(view)
	}
}

func TestHistoryExportVerifiedPublicationAndUncertainObservation(t *testing.T) {
	for _, ok := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified", false: "incomplete"}[ok], func(t *testing.T) {
			ctx := context.Background()
			s, owner, p, native, dir := historyExportFixture(t)
			_, id := historyCreateFixture(t, s, owner, p, native, dir)
			effect, err := s.PermitHistoryExport(ctx, owner, id, "1")
			if err != nil {
				t.Fatal(err)
			}
			nativeResult := historyout.Result{PermitAccepted: true, CreateAttempted: true, Created: true, BytesWritten: 10, FileFlushed: true, NameVerified: ok, DirectorySync: "unavailable", FileIdentity: "fake-file"}
			if err = s.FinishHistoryExport(ctx, owner, effect, nativeResult, 10, nil); err != nil {
				t.Fatal(err)
			}
			op, err := s.HistoryExportOperation(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if ok && (op.State != "published" || op.Durability == nil || op.Durability.DirectorySync != "unavailable") || !ok && (op.State != "uncertain" || op.Durability != nil) {
				t.Fatalf("unexpected observation: %+v", op)
			}
			if err = s.FinishHistoryExport(ctx, owner, effect, nativeResult, 10, nil); err == nil {
				t.Fatal("duplicate finish accepted")
			}
		})
	}
}

func TestHistoryExportPrePermitFailureRequiresReconciliation(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	if err := s.MarkHistoryExportPrePermitFailure(ctx, owner, id, "raw native error"); err == nil {
		t.Fatal("unbounded pre-permit failure reason accepted")
	}
	before, err := s.HistoryExportOperation(ctx, id)
	if err != nil || before.Revision != "1" || before.NextAction != "wait" {
		t.Fatalf("invalid reason changed operation: %+v %v", before, err)
	}
	if err := s.MarkHistoryExportPrePermitFailure(ctx, owner, id, "target_conflict"); err != nil {
		t.Fatal(err)
	}
	op, err := s.HistoryExportOperation(ctx, id)
	if err != nil || op.State != "prepared" || op.NextAction != "reconcile" || len(op.Blockers) != 1 || op.Blockers[0] != "target_conflict" {
		t.Fatalf("pre-permit failure not retained: %+v %v", op, err)
	}
	if _, err = s.PermitHistoryExport(ctx, owner, id, op.Revision); err == nil {
		t.Fatal("pre-permit failure was reauthorized")
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: op.Revision, Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
	if r.Status != 200 || r.Envelope.Data.(domain.HistoryReconciliationResult).Decision != "no_publication" {
		t.Fatal(r)
	}
}

func TestHistoryExportDestinationFenceAtPreviewCreateAndPermit(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	// Two previews may precede either creation; the second creation must then
	// recheck the same durable destination and leave the first able to proceed.
	for i := 0; i < 2; i++ {
		p.RequestID = domain.NewID()
		if r := s.PreviewHistoryExport(ctx, owner, p, dir, native); r.Envelope.Error != nil {
			t.Fatal(r)
		}
	}
	rows, err := s.db.Query("SELECT id FROM history_previews ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatal(ids)
	}
	first := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: ids[0], PreviewRevision: "1"}
	second := domain.ExecuteHistoryExportInput{RequestID: domain.NewID(), PreviewID: ids[1], PreviewRevision: "1"}
	if r := s.CreateHistoryExport(ctx, owner, first); r.Status != 202 {
		t.Fatal(r)
	}
	if r := s.CreateHistoryExport(ctx, owner, second); r.Envelope.Error == nil || r.Envelope.Error.Code != "effect_uncertain" {
		t.Fatal(r)
	}
	var id string
	if err = s.db.QueryRow("SELECT id FROM history_export_operations WHERE request_id=?", first.RequestID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// Case aliases of a reserved ASCII filename must also be fenced.
	alias := p
	alias.RequestID = domain.NewID()
	alias.Filename = "history-WORK.json"
	aliasNative := native
	aliasNative.Name = alias.Filename
	if r := s.PreviewHistoryExport(ctx, owner, alias, dir, aliasNative); r.Envelope.Error == nil || r.Envelope.Error.Code != "effect_uncertain" {
		t.Fatal(r)
	}
	other := p
	other.RequestID = domain.NewID()
	other.Filename = "history-other.json"
	otherNative := native
	otherNative.Name = other.Filename
	if r := s.PreviewHistoryExport(ctx, owner, other, dir, otherNative); r.Status != 201 {
		t.Fatal(r)
	}
	effect, err := s.PermitHistoryExport(ctx, owner, id, "1")
	if err != nil {
		t.Fatal(err)
	}
	// A permit with no create is still ambiguous and must keep the name fenced.
	if err = s.FinishHistoryExport(ctx, owner, effect, historyout.Result{PermitAccepted: true, DirectorySync: "unknown"}, 10, errors.New("interrupted")); err != nil {
		t.Fatal(err)
	}
	alias.RequestID = domain.NewID()
	if r := s.PreviewHistoryExport(ctx, owner, alias, dir, aliasNative); r.Envelope.Error == nil || r.Envelope.Error.Code != "effect_uncertain" {
		t.Fatal(r)
	}
}
