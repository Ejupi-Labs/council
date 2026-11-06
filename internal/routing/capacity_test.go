package routing

import (
	"fmt"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

const testNow = "2026-11-06T12:00:00.000Z"

func TestContextPositiveCannotTransferThroughDeclaredAccount(t *testing.T) {
	s := testSnapshot()
	account := fmt.Sprintf("%032x", 1)
	s.Links[0].DeclaredAccountID = &account
	s.Links[1].DeclaredAccountID = &account
	report := testReport(1, domain.CapacitySubject{Kind: "context", ID: "codex-native"}, "occupancy", "available")
	report.Bindings = []string{"context:codex-native", "account:" + account}
	s.CapacityStates[0].Heads.Occupancy = &report
	s.CapacityStates = append(s.CapacityStates, domain.CapacityState{ScopeKey: "account:" + account, Subject: domain.CapacitySubject{Kind: "account", ID: account}, Revision: "1", Heads: domain.CapacityHeads{Occupancy: &report}})
	for _, subject := range []domain.CapacitySubject{{Kind: "account", ID: account}, {Kind: "context", ID: "claude-native"}} {
		view, err := Capacity(s, subject, testNow)
		if err != nil || view.ReportEligibility.PositiveCoverage.Occupancy || len(view.Dimensions.Occupancy.PositiveReportIDs) != 0 {
			t.Fatalf("context-only positive transferred: %+v %v", view, err)
		}
	}
	view, err := Capacity(s, domain.CapacitySubject{Kind: "context", ID: "codex-native"}, testNow)
	if err != nil || !view.ReportEligibility.PositiveCoverage.Occupancy {
		t.Fatalf("original context lost its own positive: %+v %v", view, err)
	}
}

func TestCapacityRetainsAllSixtySixExhaustionOrigins(t *testing.T) {
	s := testSnapshot()
	for i := 0; i < 66; i++ {
		report := testReport(i+1, domain.CapacitySubject{Kind: "account", ID: fmt.Sprintf("%032x", i+1)}, "allowance", "exhausted")
		s.CapacityStates[0].RetainedNegatives = append(s.CapacityStates[0].RetainedNegatives, domain.RetainedCapacityReport{Report: report})
	}
	view, err := Capacity(s, domain.CapacitySubject{Kind: "context", ID: "codex-native"}, testNow)
	if err != nil || len(view.Dimensions.Allowance.BlockingReports) != 66 || view.ReportEligibility.Permitted {
		t.Fatalf("legal denial origins lost: %+v %v", view, err)
	}
}

func testSnapshot() domain.RoutingSnapshot {
	s := domain.RoutingSnapshot{Policy: DefaultPolicy(), Contexts: Contexts(), Profiles: Catalog(), Accounts: []domain.DeclaredAccount{}, ManagedOccupancy: domain.ManagedOccupancy{ResourceID: "managed-native", Limit: 1}}
	for _, context := range s.Contexts {
		s.Links = append(s.Links, domain.AccountLink{ContextID: context.ID, Revision: "1", Source: "user_declaration", UpdatedAt: testNow})
		s.CapacityStates = append(s.CapacityStates, domain.CapacityState{ScopeKey: "context:" + context.ID, Subject: domain.CapacitySubject{Kind: "context", ID: context.ID}, Revision: "1", RetainedNegatives: []domain.RetainedCapacityReport{}})
	}
	return s
}

func testReport(sequence int, subject domain.CapacitySubject, dimension, state string) domain.CapacityReport {
	return domain.CapacityReport{ID: fmt.Sprintf("%032x", sequence), Sequence: fmt.Sprint(sequence), RequestID: fmt.Sprintf("%032x", sequence+100), Subject: subject, Dimension: dimension, State: state, Source: "user_report", ObservedAt: "2026-11-06T11:59:00.000Z", ReceivedAt: "2026-11-06T11:59:01.000Z", ValidForSeconds: 900, ExpiresAt: stringPointer("2026-11-06T12:14:00.000Z"), Bindings: []string{subject.Kind + ":" + subject.ID}}
}

func TestCapacityUnknownAndManagedBusyAreQueueEligible(t *testing.T) {
	s := testSnapshot()
	s.ManagedOccupancy.ActiveAttemptID = stringPointer("0123456789abcdef0123456789abcdef")
	s.ManagedOccupancy.ActiveContextID = stringPointer("codex-native")
	v, err := Capacity(s, domain.CapacitySubject{Kind: "context", ID: "codex-native"}, testNow)
	if err != nil || !v.ReportEligibility.Permitted || !v.ManagedOccupancy.ClaimsSubject || v.Dimensions.Allowance.EffectiveState != "unknown" || v.ReportEligibility.PositiveCoverage.Occupancy {
		t.Fatalf("managed occupancy became external denial/availability: %+v %v", v, err)
	}
}

func TestCapacityRequirePositiveNeedsBothCoveredFreshDimensions(t *testing.T) {
	s := testSnapshot()
	s.Policy.UnknownAvailability = "require_positive"
	context := s.CapacityStates[0].Subject
	occupancy := testReport(1, context, "occupancy", "available")
	s.CapacityStates[0].Heads.Occupancy = &occupancy
	v, err := Capacity(s, context, testNow)
	if err != nil || v.ReportEligibility.Permitted || !v.ReportEligibility.PositiveCoverage.Occupancy || v.ReportEligibility.PositiveCoverage.Allowance {
		t.Fatal("one dimension qualified strict availability", v, err)
	}
	allowance := testReport(2, context, "allowance", "available")
	s.CapacityStates[0].Heads.Allowance = &allowance
	v, err = Capacity(s, context, testNow)
	if err != nil || !v.ReportEligibility.Permitted {
		t.Fatal(v, err)
	}
	allowance.ExpiresAt = stringPointer("2026-11-06T11:59:59.000Z")
	v, err = Capacity(s, context, testNow)
	if err != nil || v.ReportEligibility.Permitted || v.Dimensions.Allowance.EffectiveState != "unknown" || len(v.Dimensions.Allowance.PositiveReportIDs) != 0 {
		t.Fatal("expired positive was renewed", v, err)
	}
}

func TestCapacityAccountPositivesRequireOriginalBindingAndCurrentLink(t *testing.T) {
	s := testSnapshot()
	s.Policy.UnknownAvailability = "require_positive"
	accountID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	s.Accounts = []domain.DeclaredAccount{{ID: accountID}}
	s.Links[0].DeclaredAccountID = stringPointer(accountID)
	subject := domain.CapacitySubject{Kind: "account", ID: accountID}
	occupancy, allowance := testReport(1, subject, "occupancy", "available"), testReport(2, subject, "allowance", "available")
	s.CapacityStates = append(s.CapacityStates, domain.CapacityState{ScopeKey: "account:" + accountID, Subject: subject, Revision: "1", Heads: domain.CapacityHeads{Occupancy: &occupancy, Allowance: &allowance}})
	context := s.CapacityStates[0].Subject
	v, _ := Capacity(s, context, testNow)
	if v.ReportEligibility.Permitted {
		t.Fatal("link copied an old unbound account positive")
	}
	occupancy.Bindings, allowance.Bindings = append(occupancy.Bindings, "context:codex-native"), append(allowance.Bindings, "context:codex-native")
	v, _ = Capacity(s, context, testNow)
	if !v.ReportEligibility.Permitted {
		t.Fatal("current originally covered link was lost", v)
	}
	s.Links[0].DeclaredAccountID = nil
	v, _ = Capacity(s, context, testNow)
	if v.ReportEligibility.Permitted {
		t.Fatal("unlinked account positive survived as context availability")
	}
}

func TestCapacityStickyExhaustionSurvivesAgeRelinkAndNewHead(t *testing.T) {
	s := testSnapshot()
	context := s.CapacityStates[0].Subject
	old := testReport(1, domain.CapacitySubject{Kind: "account", ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, "allowance", "exhausted")
	old.ExpiresAt = stringPointer("2026-11-06T11:00:00.000Z")
	newer := testReport(2, context, "allowance", "available")
	s.CapacityStates[0].Heads.Allowance = &newer
	s.CapacityStates[0].RetainedNegatives = []domain.RetainedCapacityReport{{Report: old, InheritedFrom: &domain.LinkInheritance{AccountID: old.Subject.ID, LinkRevision: "2", LinkedAt: testNow}}}
	v, err := Capacity(s, context, testNow)
	if err != nil || v.ReportEligibility.Permitted || v.Dimensions.Allowance.EffectiveState != "exhausted" || v.Dimensions.Allowance.LatestReport.ReportID != newer.ID || v.Dimensions.Allowance.BlockingReports[0].Freshness != "expired" {
		t.Fatal("old sticky exhaustion disappeared behind a new head", v, err)
	}
	denial := testReport(3, context, "service", "authentication_denied")
	s.CapacityStates[0].Heads.Service = &denial
	// The store resolves only the exact older retained ID; newer denial remains.
	s.CapacityStates[0].RetainedNegatives = nil
	v, _ = Capacity(s, context, testNow)
	if v.ReportEligibility.Permitted || len(v.Dimensions.Allowance.BlockingReports) != 0 || len(v.Dimensions.Service.BlockingReports) != 1 {
		t.Fatal("specific recovery cleared a newer denial", v)
	}
}

func TestElapsedCooldownDoesNotInventAllowance(t *testing.T) {
	s := testSnapshot()
	context := s.CapacityStates[0].Subject
	r := testReport(1, context, "service", "temporary_throttle")
	r.RetryAfter = stringPointer("2026-11-06T11:59:59.000Z")
	s.CapacityStates[0].Heads.Service = &r
	v, err := Capacity(s, context, testNow)
	if err != nil || !v.ReportEligibility.Permitted || v.Dimensions.Service.LatestReport.Freshness != "elapsed_cooldown" || v.Dimensions.Allowance.EffectiveState != "unknown" || v.ReportEligibility.PositiveCoverage.Allowance {
		t.Fatal(v, err)
	}
}
