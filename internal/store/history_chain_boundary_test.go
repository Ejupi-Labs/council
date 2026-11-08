package store

import (
	"context"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestHistoryReconciliationChecksEveryIntermediateOwner(t *testing.T) {
	for _, closeMiddle := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete_clean_chain", false: "dirty_intermediate_owner"}[closeMiddle], func(t *testing.T) {
			ctx := context.Background()
			s, first, p, native, dir := historyExportFixture(t)
			_, id := historyCreateFixture(t, s, first, p, native, dir)
			closeOwner := func(owner string) {
				t.Helper()
				if r := s.Shutdown(ctx, owner, domain.NewID()); r.Envelope.Error != nil {
					t.Fatal(r)
				}
				clean, err := s.CompleteOwnerEpoch(ctx, owner)
				if err != nil || !clean {
					t.Fatalf("clean owner close: %v %v", clean, err)
				}
			}
			startOwner := func() string {
				t.Helper()
				owner := domain.NewID()
				if err := s.BeginOwnerEpoch(ctx, owner); err != nil {
					t.Fatal(err)
				}
				if err := s.RecoverOwner(ctx, owner); err != nil {
					t.Fatal(err)
				}
				return owner
			}
			closeOwner(first)
			middle := startOwner()
			if closeMiddle {
				closeOwner(middle)
			}
			current := startOwner()
			in := domain.ReconcileHistoryOperationInput{RequestID: domain.NewID(), ExpectedRevision: "1", Action: "verify_no_publication"}
			r := s.ReconcileHistoryExport(ctx, current, id, in, true)
			if r.Status != 200 {
				t.Fatal(r)
			}
			view := r.Envelope.Data.(domain.HistoryReconciliationResult)
			gate, err := s.Gate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if closeMiddle {
				if view.Decision != "no_publication" || gate.State != "open" {
					t.Fatalf("clean chain not verified: %+v gate=%+v", view, gate)
				}
			} else {
				if view.Decision != "unresolved" || len(view.ObservationCodes) != 1 || view.ObservationCodes[0] != "owner_history_unproven" || gate.State != "fenced" || gate.IncidentID == nil {
					t.Fatalf("dirty intermediate owner accepted: %+v gate=%+v", view, gate)
				}
			}
		})
	}
}
