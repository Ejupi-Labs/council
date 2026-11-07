package routing

import (
	json "encoding/json/v2"
	"sort"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func scopeKey(subject domain.CapacitySubject) string { return subject.Kind + ":" + subject.ID }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	return stringPointer(*value)
}

func reportFreshness(report domain.CapacityReport, now time.Time) string {
	observed, err := time.Parse(time.RFC3339Nano, report.ObservedAt)
	if err != nil || observed.After(now) {
		return "unknown"
	}
	if (report.State == "temporary_throttle" || report.State == "temporary_overload") && report.RetryAfter != nil {
		until, err := time.Parse(time.RFC3339Nano, *report.RetryAfter)
		if err != nil {
			return "unknown"
		}
		if !now.Before(until) {
			return "elapsed_cooldown"
		}
	}
	if report.ExpiresAt == nil {
		return "unknown"
	}
	expires, err := time.Parse(time.RFC3339Nano, *report.ExpiresAt)
	if err != nil {
		return "unknown"
	}
	if !now.Before(expires) {
		return "expired"
	}
	return "fresh"
}

func reportReference(report domain.CapacityReport, now time.Time, retained bool, inherited *domain.LinkInheritance) domain.ReportRef {
	var copyInheritance *domain.LinkInheritance
	if inherited != nil {
		value := *inherited
		copyInheritance = &value
	}
	return domain.ReportRef{ReportID: report.ID, Subject: report.Subject, Dimension: report.Dimension, State: report.State, Source: report.Source, ObservedAt: report.ObservedAt, ReceivedAt: report.ReceivedAt, ExpiresAt: copyString(report.ExpiresAt), RetryAfter: copyString(report.RetryAfter), Freshness: reportFreshness(report, now), Retained: retained, InheritedFrom: copyInheritance}
}

func denying(state string) bool {
	switch state {
	case "busy", "exhausted", "temporary_throttle", "temporary_overload", "authentication_denied", "model_denied", "permission_denied":
		return true
	}
	return false
}

// Capacity computes report policy only. A busy managed worker queues admission;
// it is never transformed into an external report denial or category-A reason.
func Capacity(snapshot domain.RoutingSnapshot, subject domain.CapacitySubject, stamp string) (domain.CapacityView, *domain.Error) {
	now, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || len(snapshot.CapacityStates) > 66 || len(snapshot.Accounts) > 64 || len(snapshot.Contexts) > 2 || len(snapshot.Links) > 2 {
		return domain.CapacityView{}, domain.Fail("routing_unavailable", "Capacity evidence is invalid or exceeds its supported bound.")
	}
	key := scopeKey(subject)
	var state *domain.CapacityState
	for index := range snapshot.CapacityStates {
		if snapshot.CapacityStates[index].ScopeKey == key {
			state = &snapshot.CapacityStates[index]
			break
		}
	}
	if state == nil {
		return domain.CapacityView{}, domain.Fail("routing_unavailable", "The selected capacity scope is unavailable.")
	}
	unknown := func() domain.CapacityDimension {
		return domain.CapacityDimension{EffectiveState: "unknown", BlockingReports: []domain.ReportRef{}, PositiveReportIDs: []string{}}
	}
	view := domain.CapacityView{Subject: subject, Revision: state.Revision, AsOf: stamp,
		Dimensions:        domain.CapacityDimensions{Occupancy: unknown(), Allowance: unknown(), Service: unknown()},
		ReportEligibility: domain.ReportEligibility{Policy: snapshot.Policy.UnknownAvailability, Permitted: true, ReasonCodes: []string{}}, ManagedOccupancy: snapshot.ManagedOccupancy}
	view.ManagedOccupancy.ActiveAttemptID, view.ManagedOccupancy.ActiveContextID = copyString(snapshot.ManagedOccupancy.ActiveAttemptID), copyString(snapshot.ManagedOccupancy.ActiveContextID)
	view.ManagedOccupancy.ClaimsSubject = false
	if snapshot.ManagedOccupancy.ActiveClaims != nil {
		if domain.ValidateGraphClaims(snapshot.ManagedOccupancy.ActiveClaims) != nil {
			return domain.CapacityView{}, domain.Fail("routing_unavailable", "Managed ownership observations are inconsistent.")
		}
		view.ManagedOccupancy.ActiveClaims = make([]domain.GraphClaimView, 0, len(snapshot.ManagedOccupancy.ActiveClaims))
		for _, claim := range snapshot.ManagedOccupancy.ActiveClaims {
			claim.ContextID, claim.CapturedDeclaredAccountID = copyString(claim.ContextID), copyString(claim.CapturedDeclaredAccountID)
			claim.CapturedLinkRevision = copyString(claim.CapturedLinkRevision)
			claim.GraphID, claim.NodeID = copyString(claim.GraphID), copyString(claim.NodeID)
			view.ManagedOccupancy.ActiveClaims = append(view.ManagedOccupancy.ActiveClaims, claim)
		}
	}
	var accountState *domain.CapacityState
	if subject.Kind == "context" {
		for _, link := range snapshot.Links {
			if link.ContextID == subject.ID {
				copy := link
				copy.DeclaredAccountID = copyString(link.DeclaredAccountID)
				view.AccountLink = &copy
				if link.DeclaredAccountID != nil {
					for index := range snapshot.CapacityStates {
						if snapshot.CapacityStates[index].ScopeKey == "account:"+*link.DeclaredAccountID {
							accountState = &snapshot.CapacityStates[index]
							break
						}
					}
				}
				break
			}
		}
		if view.AccountLink == nil {
			return domain.CapacityView{}, domain.Fail("routing_unavailable", "The native context has no current declared-link revision.")
		}
		view.ManagedOccupancy.ClaimsSubject = view.ManagedOccupancy.ActiveContextID != nil && *view.ManagedOccupancy.ActiveContextID == subject.ID
	} else if subject.Kind == "account" {
		for _, link := range snapshot.Links {
			if view.ManagedOccupancy.ActiveContextID != nil && link.ContextID == *view.ManagedOccupancy.ActiveContextID && link.DeclaredAccountID != nil && *link.DeclaredAccountID == subject.ID {
				view.ManagedOccupancy.ClaimsSubject = true
			}
		}
	} else {
		return domain.CapacityView{}, domain.Invalid("capacity subject")
	}
	if snapshot.ManagedOccupancy.ActiveClaims != nil {
		view.ManagedOccupancy.ClaimsSubject = false
		for _, claim := range snapshot.ManagedOccupancy.ActiveClaims {
			if subject.Kind == "context" {
				if claim.ContextID != nil && *claim.ContextID == subject.ID {
					view.ManagedOccupancy.ClaimsSubject = true
				}
				continue
			}
			if claim.CapturedDeclaredAccountID != nil && *claim.CapturedDeclaredAccountID == subject.ID {
				view.ManagedOccupancy.ClaimsSubject = true
			}
			for _, link := range snapshot.Links {
				if claim.ContextID != nil && link.ContextID == *claim.ContextID && link.DeclaredAccountID != nil && *link.DeclaredAccountID == subject.ID {
					view.ManagedOccupancy.ClaimsSubject = true
				}
			}
		}
	}
	dimensions := map[string]*domain.CapacityDimension{"occupancy": &view.Dimensions.Occupancy, "allowance": &view.Dimensions.Allowance, "service": &view.Dimensions.Service}
	latestSequence := map[string]int64{}
	apply := func(report *domain.CapacityReport, retained bool, inheritance *domain.LinkInheritance, positiveApplies bool) {
		if report == nil {
			return
		}
		dimension := dimensions[report.Dimension]
		if dimension == nil {
			return
		}
		ref := reportReference(*report, now, retained, inheritance)
		if !retained {
			sequence, _ := domain.ParseCount(report.Sequence)
			if dimension.LatestReport == nil || sequence >= latestSequence[report.Dimension] {
				copy := ref
				dimension.LatestReport = &copy
				latestSequence[report.Dimension] = sequence
			}
		}
		if denying(report.State) && (report.State == "exhausted" || ref.Freshness == "fresh") {
			for index := range dimension.BlockingReports {
				if dimension.BlockingReports[index].ReportID == ref.ReportID {
					if retained {
						// The same immutable report can arrive through its original
						// account and a later context link. Preserve that context's
						// concrete inherited provenance when the account copy is nil.
						if ref.InheritedFrom == nil {
							ref.InheritedFrom = dimension.BlockingReports[index].InheritedFrom
						}
						dimension.BlockingReports[index] = ref
					}
					return
				}
			}
			dimension.BlockingReports = append(dimension.BlockingReports, ref)
		} else if !retained && positiveApplies && report.State == "available" && ref.Freshness == "fresh" && !contains(dimension.PositiveReportIDs, report.ID) {
			dimension.PositiveReportIDs = append(dimension.PositiveReportIDs, report.ID)
		}
	}
	applyState := func(current *domain.CapacityState) *domain.Error {
		if len(current.RetainedNegatives) > 198 {
			return domain.Fail("routing_unavailable", "Retained capacity evidence exceeds its supported bound.")
		}
		for _, report := range []*domain.CapacityReport{current.Heads.Occupancy, current.Heads.Allowance, current.Heads.Service} {
			positive := report != nil && contains(report.Bindings, key)
			if positive {
				if report.Subject.Kind == "context" {
					positive = subject.Kind == "context" && report.Subject.ID == subject.ID
				} else if report.Subject.Kind == "account" && subject.Kind == "context" {
					positive = view.AccountLink != nil && view.AccountLink.DeclaredAccountID != nil && *view.AccountLink.DeclaredAccountID == report.Subject.ID
				} else {
					positive = report.Subject == subject
				}
			}
			apply(report, false, nil, positive)
		}
		for _, negative := range current.RetainedNegatives {
			apply(&negative.Report, true, negative.InheritedFrom, false)
		}
		return nil
	}
	if err := applyState(state); err != nil {
		return domain.CapacityView{}, err
	}
	if accountState != nil {
		if err := applyState(accountState); err != nil {
			return domain.CapacityView{}, err
		}
	}
	for name, dimension := range dimensions {
		if len(dimension.BlockingReports) > 66 {
			return domain.CapacityView{}, domain.Fail("routing_unavailable", "Blocking capacity evidence exceeds its supported bound.")
		}
		sort.Strings(dimension.PositiveReportIDs)
		sort.Slice(dimension.BlockingReports, func(i, j int) bool {
			return dimension.BlockingReports[i].ReportID < dimension.BlockingReports[j].ReportID
		})
		if len(dimension.BlockingReports) != 0 {
			dimension.EffectiveState = dimension.BlockingReports[0].State
			view.ReportEligibility.Permitted = false
			view.ReportEligibility.ReasonCodes = append(view.ReportEligibility.ReasonCodes, name+"_denied")
		} else if len(dimension.PositiveReportIDs) != 0 {
			dimension.EffectiveState = "available"
		}
	}
	view.ReportEligibility.PositiveCoverage = domain.PositiveCoverage{Occupancy: len(view.Dimensions.Occupancy.PositiveReportIDs) != 0, Allowance: len(view.Dimensions.Allowance.PositiveReportIDs) != 0}
	if view.ReportEligibility.Policy == "require_positive" && (!view.ReportEligibility.PositiveCoverage.Occupancy || !view.ReportEligibility.PositiveCoverage.Allowance) {
		view.ReportEligibility.Permitted = false
		view.ReportEligibility.ReasonCodes = append(view.ReportEligibility.ReasonCodes, "fresh_positive_occupancy_and_allowance_required")
	} else if view.ReportEligibility.Policy != "allow" && view.ReportEligibility.Policy != "require_positive" {
		return domain.CapacityView{}, domain.Invalid("unknown-availability policy")
	}
	sort.Strings(view.ReportEligibility.ReasonCodes)
	encoded, marshalErr := json.Marshal(view)
	if marshalErr != nil || len(encoded) > 128<<10 {
		return domain.CapacityView{}, domain.Fail("routing_unavailable", "The bounded capacity view cannot represent this evidence.")
	}
	return view, nil
}
