package store

import (
	"context"
	"errors"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryReconcileReceiptProvesCommitOrUnchangedRollback(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost_ack"}[commit], func(t *testing.T) {
			ctx := context.Background()
			s, owner, p, native, dir := historyExportFixture(t)
			_, id := historyCreateFixture(t, s, owner, p, native, dir)
			incident, err := s.FenceHistoryOperation(ctx, owner, id)
			if err != nil {
				t.Fatal(err)
			}
			in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
			if commit {
				s.AfterCommit = func() error { return errors.New("lost acknowledgement") }
			} else {
				s.BeforeCommit = func() error { return errors.New("before commit") }
			}
			r := s.ReconcileHistoryExport(ctx, owner, id, in, true)
			if r.Envelope.Error == nil {
				t.Fatal("injected fault was ignored", r)
			}
			s.AfterCommit = nil
			s.BeforeCommit = nil
			saved, err := s.VerifyHistoryReconciliationReceipt(ctx, owner, id, in)
			if err != nil {
				t.Fatal(err)
			}
			view, err := s.RecoveryIncident(ctx, incident)
			if err != nil {
				t.Fatal(err)
			}
			if commit {
				if saved == nil || saved.Status != 200 || view.State != "reconciled" {
					t.Fatal(saved, view)
				}
			} else {
				if saved != nil || view.State != "unresolved" {
					t.Fatal(saved, view)
				}
				gate, err := s.Gate(ctx)
				if err != nil || gate.State != "fenced" {
					t.Fatal(gate, err)
				}
			}
			changed := in
			changed.ExpectedRevision = "2"
			if _, err = s.VerifyHistoryReconciliationReceipt(ctx, owner, id, changed); err == nil {
				t.Fatal("changed request or original revision accepted")
			}
		})
	}
}

func TestHistoryIncidentReconcileReceiptProvesCommitOrUnchangedRollback(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost_ack"}[commit], func(t *testing.T) {
			ctx := context.Background()
			s, owner, _, _, _ := historyExportFixture(t)
			var req, fp string
			if err := s.db.QueryRow("SELECT request_id,fingerprint FROM receipts WHERE operation=?", historyCaptureOperation).Scan(&req, &fp); err != nil {
				t.Fatal(err)
			}
			incident, err := s.FenceHistoryMutation(ctx, owner, historyCaptureOperation, req, fp)
			if err != nil {
				t.Fatal(err)
			}
			in := domain.ReconciliationInput{RequestID: domain.NewID(), ExpectedIncidentRevision: "1", Action: "verify_and_reconcile"}
			if commit {
				s.AfterCommit = func() error { return errors.New("lost acknowledgement") }
			} else {
				s.BeforeCommit = func() error { return errors.New("before commit") }
			}
			r, handled := s.TryReconcileHistoryIncident(ctx, owner, incident, in, true)
			if !handled || r.Envelope.Error == nil {
				t.Fatal(r, handled)
			}
			s.AfterCommit = nil
			s.BeforeCommit = nil
			saved, err := s.VerifyHistoryIncidentReconciliationReceipt(ctx, owner, incident, in)
			if err != nil {
				t.Fatal(err)
			}
			view, err := s.RecoveryIncident(ctx, incident)
			if err != nil {
				t.Fatal(err)
			}
			if commit {
				if saved == nil || saved.Status != 200 || view.State != "reconciled" {
					t.Fatal(saved, view)
				}
			} else {
				if saved != nil || view.State != "unresolved" {
					t.Fatal(saved, view)
				}
				gate, err := s.Gate(ctx)
				if err != nil || gate.State != "fenced" {
					t.Fatal(gate, err)
				}
			}
			changed := in
			changed.ExpectedIncidentRevision = "2"
			if _, err = s.VerifyHistoryIncidentReconciliationReceipt(ctx, owner, incident, changed); err == nil {
				t.Fatal("changed request or original revision accepted")
			}
		})
	}
}
