package routing

import (
	"fmt"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func graphReservationInput() domain.RoutingSelectionInput {
	snapshot := testSnapshot()
	snapshot.ManagedOccupancy = domain.ManagedOccupancy{ResourceID: "managed-native", Limit: 2, OccupiedSlots: "1", ActiveClaims: []domain.GraphClaimView{{Slot: "1", AttemptID: fmt.Sprintf("%032x", 71), ExecutorKind: "native", ContextID: stringPointer("codex-native")}}}
	return domain.RoutingSelectionInput{RequireManagedReservation: true, Snapshot: snapshot, Assessment: domain.SuitabilityAssessment{Classification: "unknown", Source: "unknown"}, Facts: []domain.NativeContextFact{{ContextID: "codex-native", InstallationStatus: "installed", Source: "local_fixture", ObservedAt: testNow}, {ContextID: "claude-native", InstallationStatus: "installed", Source: "local_fixture", ObservedAt: testNow}}, Now: testNow}
}
func TestGraphReservationSelectsAvailableContextWithoutRetargetingStandalone(t *testing.T) {
	input := graphReservationInput()
	selected, e := Select(input)
	if e != nil || selected.SelectedTarget.ContextID != "claude-native" {
		t.Fatal("graph did not select the currently reservable context", selected, e)
	}
	found := false
	for _, c := range selected.Candidates {
		if c.ContextID == "codex-native" && c.ProfileID != nil && *c.ProfileID == "codex-native-default" {
			found = !c.Eligible && contains(c.ReasonCodes, "managed_capacity_occupied")
		}
	}
	if !found {
		t.Fatal("actual reservation reason is missing", selected.Candidates)
	}
	for _, capacity := range selected.CapacityEvidence {
		if len(capacity.BlockingReportIDs) != 0 {
			t.Fatal("managed reservation fabricated external denial evidence", capacity)
		}
	}
	input.RequireManagedReservation = false
	standalone, e := Select(input)
	if e != nil || standalone.SelectedTarget.ContextID != "codex-native" {
		t.Fatal("standalone admission stopped preserving its captured target/queue behavior", standalone, e)
	}
}
func TestGraphReservationWaitsForPinnedOrCapturedCurrentAccountCapacity(t *testing.T) {
	input := graphReservationInput()
	input.Pins.HarnessID = "codex"
	if _, e := Select(input); e == nil || e.Code != "capacity_wait" {
		t.Fatal("pinned occupied context became an admitted target or eligibility error", e)
	}
	input.Pins = domain.ExecutionPins{}
	captured, current := fmt.Sprintf("%032x", 81), fmt.Sprintf("%032x", 82)
	for _, account := range []string{captured, current} {
		input.Snapshot.Accounts = append(input.Snapshot.Accounts, domain.DeclaredAccount{ID: account})
		input.Snapshot.CapacityStates = append(input.Snapshot.CapacityStates, domain.CapacityState{ScopeKey: "account:" + account, Subject: domain.CapacitySubject{Kind: "account", ID: account}, Revision: "1"})
	}
	input.Snapshot.ManagedOccupancy.ActiveClaims[0].CapturedDeclaredAccountID = &captured
	input.Snapshot.ManagedOccupancy.ActiveClaims[0].CapturedLinkRevision = stringPointer("1")
	for i := range input.Snapshot.Links {
		if input.Snapshot.Links[i].ContextID == "codex-native" {
			input.Snapshot.Links[i].DeclaredAccountID = &current
		}
	}
	for _, account := range []string{captured, current} {
		for i := range input.Snapshot.Links {
			if input.Snapshot.Links[i].ContextID == "claude-native" {
				input.Snapshot.Links[i].DeclaredAccountID = &account
			}
		}
		if _, e := Select(input); e == nil || e.Code != "capacity_wait" {
			t.Fatal("declared account reservation was erased by relinking", account, e)
		}
	}
	for i := range input.Snapshot.Links {
		if input.Snapshot.Links[i].ContextID == "claude-native" {
			input.Snapshot.Links[i].DeclaredAccountID = nil
		}
	}
	if selected, e := Select(input); e != nil || selected.SelectedTarget.ContextID != "claude-native" {
		t.Fatal("unknown unrelated context did not remain eligible", selected, e)
	}
}
