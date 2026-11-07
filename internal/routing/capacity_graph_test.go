package routing

import (
	"fmt"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

// Typed capacity facts exercise projections, not real account identity/usage.
func TestGraphCapacityProjectsEveryClaimAndCapturedCurrentAccounts(t *testing.T) {
	snapshot := testSnapshot()
	captured, current := fmt.Sprintf("%032x", 31), fmt.Sprintf("%032x", 32)
	for _, id := range []string{captured, current} {
		snapshot.Accounts = append(snapshot.Accounts, domain.DeclaredAccount{ID: id})
		snapshot.CapacityStates = append(snapshot.CapacityStates, domain.CapacityState{ScopeKey: "account:" + id, Subject: domain.CapacitySubject{Kind: "account", ID: id}, Revision: "1"})
	}
	snapshot.Links[0].DeclaredAccountID = &current
	snapshot.ManagedOccupancy = domain.ManagedOccupancy{ResourceID: "managed-native", Limit: 2, OccupiedSlots: "2", ActiveClaims: []domain.GraphClaimView{
		{Slot: "1", AttemptID: fmt.Sprintf("%032x", 41), ExecutorKind: "native", ContextID: stringPointer("codex-native"), CapturedDeclaredAccountID: &captured, CapturedLinkRevision: stringPointer("1")},
		{Slot: "2", AttemptID: fmt.Sprintf("%032x", 42), ExecutorKind: "native", ContextID: stringPointer("claude-native")},
	}}
	for _, subject := range []domain.CapacitySubject{{Kind: "context", ID: "codex-native"}, {Kind: "context", ID: "claude-native"}, {Kind: "account", ID: captured}, {Kind: "account", ID: current}} {
		view, e := Capacity(snapshot, subject, testNow)
		if e != nil || !view.ManagedOccupancy.ClaimsSubject || len(view.ManagedOccupancy.ActiveClaims) != 2 || view.ManagedOccupancy.ActiveAttemptID != nil || view.ManagedOccupancy.ActiveContextID != nil {
			t.Fatalf("sole marker lost actual claim scope: subject=%+v view=%+v error=%v", subject, view, e)
		}
		if !view.ReportEligibility.Permitted || len(view.Dimensions.Occupancy.BlockingReports) != 0 {
			t.Fatal("managed occupancy became a report denial", view)
		}
	}
	view, e := Capacity(snapshot, domain.CapacitySubject{Kind: "context", ID: "codex-native"}, testNow)
	if e != nil {
		t.Fatal(e)
	}
	*view.ManagedOccupancy.ActiveClaims[0].ContextID = "mutated"
	if *snapshot.ManagedOccupancy.ActiveClaims[0].ContextID != "codex-native" {
		t.Fatal("returned view aliases source claim evidence")
	}
}
