package store

import (
	"context"
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryReconcileDrainedNoPermitRetainsIndependentFence(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	incident := domain.ExecutionIncident{ID: domain.NewID(), Code: "other_blocker", Message: "independent test fence", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, incident); err != nil {
		t.Fatal(err)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, owner, id, in, false)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view := r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "actor_not_drained" {
		t.Fatal(view)
	}
	if _, err := s.PermitHistoryExport(ctx, owner, id, view.Operation.Revision); err == nil {
		t.Fatal("frozen operation obtained permit")
	}
	in = domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: view.Operation.Revision, Action: "verify_no_publication"}
	r = s.ReconcileHistoryExport(ctx, owner, id, in, true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view = r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "no_publication" || view.Operation.State != "no_publication" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "no_permit_proven" {
		t.Fatal(view)
	}
	replay := s.ReconcileHistoryExport(ctx, owner, id, in, false)
	if replay.Status != 200 || replay.Envelope.Receipt.CommittedAt != r.Envelope.Receipt.CommittedAt {
		t.Fatal(replay)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != incident.ID {
		t.Fatalf("independent fence changed: %+v %v", gate, err)
	}
}

func TestHistoryBoundOperationIncidentReleasesOnlyProvenCause(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	other := domain.ExecutionIncident{ID: domain.NewID(), Code: "independent_test", Message: "independent", CreatedAt: domain.Now()}
	if err := s.Fence(ctx, other); err != nil {
		t.Fatal(err)
	}
	incident, err := s.FenceHistoryOperation(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
	if r.Status != 200 || r.Envelope.Data.(domain.HistoryReconciliationResult).Decision != "no_publication" {
		t.Fatal(r)
	}
	view, err := s.RecoveryIncident(ctx, incident)
	if err != nil || view.State != "reconciled" {
		t.Fatalf("linked incident: %+v %v", view, err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != other.ID {
		t.Fatalf("independent incident lost: %+v %v", gate, err)
	}
	otherView, err := s.RecoveryIncident(ctx, other.ID)
	if err != nil || otherView.State != "unresolved" {
		t.Fatalf("independent incident resolved: %+v %v", otherView, err)
	}
}

func TestHistoryBoundOperationPermitCannotResolveIncident(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	if _, err := s.PermitHistoryExport(ctx, owner, id, "1"); err != nil {
		t.Fatal(err)
	}
	incident, err := s.FenceHistoryOperation(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "2", Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
	if r.Status != 200 || r.Envelope.Data.(domain.HistoryReconciliationResult).Decision != "unresolved" {
		t.Fatal(r)
	}
	view, err := s.RecoveryIncident(ctx, incident)
	if err != nil || view.State != "unresolved" {
		t.Fatalf("permit incident: %+v %v", view, err)
	}
}

func TestHistoryReceiptIncidentResolutionAndMismatch(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "mismatch"}[mismatch], func(t *testing.T) {
			ctx := context.Background()
			s, owner, _, _, _ := historyExportFixture(t)
			var req, fp string
			if err := s.db.QueryRow("SELECT request_id,fingerprint FROM receipts WHERE operation=?", historyCaptureOperation).Scan(&req, &fp); err != nil {
				t.Fatal(err)
			}
			if mismatch {
				fp = strings.Repeat("a", 64)
			}
			incident, err := s.FenceHistoryMutation(ctx, owner, historyCaptureOperation, req, fp)
			if err != nil {
				t.Fatal(err)
			}
			in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "1", Action: "verify_and_reconcile"}
			if r, handled := s.TryReconcileHistoryIncident(ctx, owner, incident, in, false); !handled || r.Envelope.Error == nil {
				t.Fatal(r, handled)
			}
			r, handled := s.TryReconcileHistoryIncident(ctx, owner, incident, in, true)
			if !handled {
				t.Fatal("binding ignored")
			}
			gate, err := s.Gate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if mismatch {
				if r.Envelope.Error == nil || gate.State != "fenced" {
					t.Fatal(r, gate)
				}
				return
			}
			if r.Status != 200 || gate.State != "open" {
				t.Fatal(r, gate)
			}
			replay, handled := s.TryReconcileHistoryIncident(ctx, owner, incident, in, true)
			if !handled || replay.Status != 200 || replay.Envelope.Receipt.CommittedAt != r.Envelope.Receipt.CommittedAt {
				t.Fatal(replay, handled)
			}
		})
	}
}

func TestHistoryCreateReceiptIncidentFreezesUnstartedOperation(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	create, id := historyCreateFixture(t, s, owner, p, native, dir)
	incident, err := s.FenceHistoryMutation(ctx, owner, HistoryCreateOperation, create.RequestID, HistoryCreateFingerprint(create))
	if err != nil {
		t.Fatal(err)
	}
	in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "1", Action: "verify_and_reconcile"}
	r, handled := s.TryReconcileHistoryIncident(ctx, owner, incident, in, true)
	if !handled || r.Status != 200 {
		t.Fatal(r, handled)
	}
	op, err := s.HistoryExportOperation(ctx, id)
	if err != nil || op.State != "prepared" || op.NextAction != "reconcile" {
		t.Fatalf("operation: %+v %v", op, err)
	}
	if _, err = s.PermitHistoryExport(ctx, owner, id, op.Revision); err == nil {
		t.Fatal("frozen operation authorized")
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "open" {
		t.Fatalf("receipt fence retained: %+v %v", gate, err)
	}
}

func TestHistoryReceiptIncidentDirtyOwnerRemainsFenced(t *testing.T) {
	ctx := context.Background()
	s, owner, _, _, _ := historyExportFixture(t)
	id, err := s.FenceHistoryMutation(ctx, owner, historyCaptureOperation, domain.NewID(), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	newOwner := domain.NewID()
	if err = s.BeginOwnerEpoch(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOwner(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "1", Action: "verify_and_reconcile"}
	r, handled := s.TryReconcileHistoryIncident(ctx, newOwner, id, in, true)
	if !handled || r.Envelope.Error == nil {
		t.Fatal(r, handled)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" {
		t.Fatalf("dirty fence lost: %+v %v", gate, err)
	}
}

func TestHistoryReconcileRejectsDirtyOwnerHandoff(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	newOwner := domain.NewID()
	if err := s.BeginOwnerEpoch(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverOwner(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, newOwner, id, in, true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view := r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "owner_history_unproven" {
		t.Fatal(view)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" {
		t.Fatalf("dirty handoff unfenced: %+v %v", gate, err)
	}
}

func TestHistoryExportFreezeStrandedPreparedAfterDirtyRestart(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	newOwner := domain.NewID()
	if err := s.BeginOwnerEpoch(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverOwner(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkHistoryExportPrePermitFailure(ctx, newOwner, id, "request_interrupted"); err != nil {
		t.Fatal(err)
	}
	op, err := s.HistoryExportOperation(ctx, id)
	if err != nil || op.State != "prepared" || op.NextAction != "reconcile" || len(op.Blockers) != 1 || op.Blockers[0] != "request_interrupted" {
		t.Fatalf("stranded operation: %+v %v", op, err)
	}
	if _, err = s.PermitHistoryExport(ctx, newOwner, id, op.Revision); err == nil {
		t.Fatal("new owner obtained old publication permit")
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: op.Revision, Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, newOwner, id, in, true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view := r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "owner_history_unproven" {
		t.Fatal(view)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" {
		t.Fatalf("dirty owner fence lost: %+v %v", gate, err)
	}
}

func TestHistoryReconcileCleanOwnerHandoffAndStaleRevision(t *testing.T) {
	ctx := context.Background()
	s, owner, p, native, dir := historyExportFixture(t)
	_, id := historyCreateFixture(t, s, owner, p, native, dir)
	if r := s.Shutdown(ctx, owner, domain.NewID()); r.Envelope.Error != nil {
		t.Fatal(r)
	}
	clean, err := s.CompleteOwnerEpoch(ctx, owner)
	if err != nil || !clean {
		t.Fatalf("clean shutdown: %v %v", clean, err)
	}
	newOwner := domain.NewID()
	if err = s.BeginOwnerEpoch(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverOwner(ctx, newOwner); err != nil {
		t.Fatal(err)
	}
	stale := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "2", Action: "verify_no_publication"}
	if r := s.ReconcileHistoryExport(ctx, newOwner, id, stale, true); r.Envelope.Error == nil || r.Envelope.Error.Code != "revision_conflict" {
		t.Fatal(r)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, newOwner, id, in, true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view := r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "no_publication" {
		t.Fatal(view)
	}
}
