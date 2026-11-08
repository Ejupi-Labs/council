package store

import (
	"context"
	"sync"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func historyPermitCount(t *testing.T, s *Store, operationID string) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM history_effect_journal WHERE operation_id=? AND kind='permit'", operationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestHistoryReconcileFreezeAndPermitSerialOrder(t *testing.T) {
	for _, permitFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "freeze_first", true: "permit_first"}[permitFirst], func(t *testing.T) {
			ctx := context.Background()
			s, owner, p, native, dir := historyExportFixture(t)
			_, id := historyCreateFixture(t, s, owner, p, native, dir)
			if permitFirst {
				effect, err := s.PermitHistoryExport(ctx, owner, id, "1")
				if err != nil || !domain.ValidID(effect.EffectID) {
					t.Fatal(effect, err)
				}
				in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "2", Action: "verify_no_publication"}
				r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
				if r.Status != 200 {
					t.Fatal(r)
				}
				view := r.Envelope.Data.(domain.HistoryReconciliationResult)
				if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "permit_present_or_uncertain" || historyPermitCount(t, s, id) != 1 {
					t.Fatal(view)
				}
				if _, err = s.PermitHistoryExport(ctx, owner, id, view.Operation.Revision); err == nil {
					t.Fatal("second permit accepted")
				}
				return
			}
			in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
			r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
			if r.Status != 200 {
				t.Fatal(r)
			}
			view := r.Envelope.Data.(domain.HistoryReconciliationResult)
			if view.Decision != "no_publication" || view.Operation.State != "no_publication" {
				t.Fatal(view)
			}
			if _, err := s.PermitHistoryExport(ctx, owner, id, view.Operation.Revision); err == nil || historyPermitCount(t, s, id) != 0 {
				t.Fatalf("frozen operation admitted a permit: %v", err)
			}
		})
	}
}

func TestHistoryReconcileFreezeContendsWithPermit(t *testing.T) {
	// The Engine normally drains publishers first. This deliberately stresses
	// the Store transaction boundary even if a competing caller is already
	// waiting: exactly one authority ordering may commit.
	for iteration := 0; iteration < 8; iteration++ {
		ctx := context.Background()
		s, owner, p, native, dir := historyExportFixture(t)
		_, id := historyCreateFixture(t, s, owner, p, native, dir)
		in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var effect domain.HistoryPublicationEffect
		var permitErr error
		var result domain.Result
		go func() { defer wg.Done(); <-start; effect, permitErr = s.PermitHistoryExport(ctx, owner, id, "1") }()
		go func() { defer wg.Done(); <-start; result = s.ReconcileHistoryExport(ctx, owner, id, in, true) }()
		close(start)
		wg.Wait()
		op, err := s.HistoryExportOperation(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		count := historyPermitCount(t, s, id)
		if permitErr == nil {
			if !domain.ValidID(effect.EffectID) || count != 1 || op.State != "effect_started" || result.Envelope.Error == nil || result.Envelope.Error.Code != "revision_conflict" {
				t.Fatalf("permit won with incompatible result: effect=%+v count=%d op=%+v reconcile=%+v", effect, count, op, result)
			}
		} else {
			if effect.EffectID != "" || count != 0 || op.State != "no_publication" || result.Status != 200 || result.Envelope.Data.(domain.HistoryReconciliationResult).Decision != "no_publication" {
				t.Fatalf("freeze won with incompatible result: error=%v effect=%+v count=%d op=%+v reconcile=%+v", permitErr, effect, count, op, result)
			}
		}
	}
}

func TestHistoryReconcileMissingJournalProofKeepsFence(t *testing.T) {
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
	// Corrupt only the disposable fixture to simulate a missing persisted row.
	// Reinstall the exact immutable trigger before invoking reconciliation.
	if _, err = s.db.Exec("DROP TRIGGER history_effect_journal_no_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DELETE FROM history_effect_journal WHERE operation_id=? AND kind='permit'", id); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(historyJournalNoDelete); err != nil {
		t.Fatal(err)
	}
	in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "2", Action: "verify_no_publication"}
	r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
	if r.Status != 200 {
		t.Fatal(r)
	}
	view := r.Envelope.Data.(domain.HistoryReconciliationResult)
	if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "journal_unproven" || view.Operation.State != "effect_started" {
		t.Fatal(view)
	}
	incidentView, err := s.RecoveryIncident(ctx, incident)
	if err != nil || incidentView.State != "unresolved" {
		t.Fatalf("incident resolved without proof: %+v %v", incidentView, err)
	}
	gate, err := s.Gate(ctx)
	if err != nil || gate.State != "fenced" || gate.IncidentID == nil || *gate.IncidentID != incident {
		t.Fatalf("corrupt journal released fence: %+v %v", gate, err)
	}
}
