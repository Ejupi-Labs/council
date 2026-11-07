package routing

import (
	json "encoding/json/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func knownVersionLess(actual, minimum string) (bool, bool) {
	parse := func(value string) ([3]int64, bool) {
		var out [3]int64
		parts := strings.Split(value, ".")
		if len(parts) != 3 {
			return out, false
		}
		for i, part := range parts {
			n, err := strconv.ParseInt(part, 10, 32)
			if err != nil || n < 0 || strconv.FormatInt(n, 10) != part {
				return out, false
			}
			out[i] = n
		}
		return out, true
	}
	a, aOK := parse(actual)
	b, bOK := parse(minimum)
	if !aOK || !bOK {
		return false, false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i], true
		}
	}
	return false, true
}

func compactCapacity(view domain.CapacityView) domain.CandidateCapacityEvidence {
	e := domain.CandidateCapacityEvidence{ContextID: view.Subject.ID, Revision: view.Revision, AsOf: view.AsOf,
		EffectiveStates:  domain.CapacityEffectiveStates{Occupancy: view.Dimensions.Occupancy.EffectiveState, Allowance: view.Dimensions.Allowance.EffectiveState, Service: view.Dimensions.Service.EffectiveState},
		PositiveCoverage: view.ReportEligibility.PositiveCoverage, Permitted: view.ReportEligibility.Permitted, BlockingReportIDs: []string{}, SupportingReportIDs: []string{}}
	if view.AccountLink != nil {
		e.DeclaredAccountID, e.AccountLinkRevision = copyString(view.AccountLink.DeclaredAccountID), stringPointer(view.AccountLink.Revision)
	}
	for _, dimension := range []domain.CapacityDimension{view.Dimensions.Occupancy, view.Dimensions.Allowance, view.Dimensions.Service} {
		if dimension.LatestReport != nil && !contains(e.SupportingReportIDs, dimension.LatestReport.ReportID) {
			e.SupportingReportIDs = append(e.SupportingReportIDs, dimension.LatestReport.ReportID)
		}
		for _, ref := range dimension.BlockingReports {
			if !contains(e.BlockingReportIDs, ref.ReportID) {
				e.BlockingReportIDs = append(e.BlockingReportIDs, ref.ReportID)
			}
			if !contains(e.SupportingReportIDs, ref.ReportID) {
				e.SupportingReportIDs = append(e.SupportingReportIDs, ref.ReportID)
			}
		}
		for _, id := range dimension.PositiveReportIDs {
			if !contains(e.SupportingReportIDs, id) {
				e.SupportingReportIDs = append(e.SupportingReportIDs, id)
			}
		}
	}
	sort.Strings(e.BlockingReportIDs)
	sort.Strings(e.SupportingReportIDs)
	return e
}

func copyProfile(profile domain.TargetProfile) domain.TargetProfile {
	profile.Source.MinimumNativeVersion = copyString(profile.Source.MinimumNativeVersion)
	if profile.ReferencePrice != nil {
		price := *profile.ReferencePrice
		price.Unit, price.Basis = copyString(price.Unit), copyString(price.Basis)
		price.InputPerMillion, price.OutputPerMillion = copyString(price.InputPerMillion), copyString(price.OutputPerMillion)
		price.SourceURL, price.ValidUntil = copyString(price.SourceURL), copyString(price.ValidUntil)
		profile.ReferencePrice = &price
	}
	return profile
}

// priceFrontier preserves captured order after removing only actual comparable
// dominance. Pareto plus tie order is not used as a non-transitive comparator.
func priceFrontier(indices []int, prices []*domain.ReferencePrice, now string) []int {
	out := make([]int, 0, len(indices))
	for _, candidate := range indices {
		dominated := false
		for _, other := range indices {
			if other != candidate {
				relation, comparable := ComparePrice(prices[other], prices[candidate], now)
				if comparable && relation < 0 {
					dominated = true
					break
				}
			}
		}
		if !dominated {
			out = append(out, candidate)
		}
	}
	return out
}

func Select(input domain.RoutingSelectionInput) (domain.RoutingSelectionResult, *domain.Error) {
	result := domain.RoutingSelectionResult{Candidates: []domain.CandidateAssessment{}, CapacityEvidence: []domain.CandidateCapacityEvidence{}, Uncertainties: []string{}}
	if _, err := time.Parse(time.RFC3339Nano, input.Now); err != nil || len(input.Snapshot.Contexts) != 2 || len(input.Snapshot.Profiles) != 6 || len(input.Facts) > 2 {
		return result, domain.Fail("routing_unavailable", "Bounded native routing evidence is unavailable.")
	}
	if input.RequireManagedReservation && (input.Snapshot.ManagedOccupancy.Limit != domain.MaxManagedSlots || input.Snapshot.ManagedOccupancy.ActiveClaims == nil || domain.ValidateGraphClaims(input.Snapshot.ManagedOccupancy.ActiveClaims) != nil) {
		return result, domain.Fail("routing_unavailable", "Complete managed ownership observations are required before graph admission.")
	}
	capacityWaiting := false
	if err := domain.ValidatePins(&input.Pins); err != nil {
		return result, err
	}
	if err := domain.ValidateModelArgument(input.Pins.Model); err != nil {
		return result, err
	}
	if input.Pins.AccountID != "" || input.Pins.PaymentPath == "paid_api" {
		return result, domain.Fail("pin_unavailable", "The native connection cannot enforce the requested actual account or paid API path.")
	}
	contexts := map[string]domain.NativeContext{}
	facts := map[string]domain.NativeContextFact{}
	for _, context := range input.Snapshot.Contexts {
		contexts[context.ID] = context
	}
	for _, fact := range input.Facts {
		if _, duplicate := facts[fact.ContextID]; duplicate {
			return result, domain.Fail("routing_unavailable", "Native context evidence is ambiguous.")
		}
		facts[fact.ContextID] = fact
	}
	profiles := make([]domain.TargetProfile, 0, 7)
	profileIDs := map[string]bool{}
	for _, profile := range input.Snapshot.Profiles {
		if profileIDs[profile.ID] {
			return result, domain.Fail("routing_unavailable", "Native profile declarations are ambiguous.")
		}
		profileIDs[profile.ID] = true
		profiles = append(profiles, copyProfile(profile))
	}
	forcedContext := ""
	if input.Pins.HarnessID != "" {
		for _, context := range input.Snapshot.Contexts {
			if context.HarnessID == input.Pins.HarnessID {
				forcedContext = context.ID
			}
		}
		if forcedContext == "" {
			return result, domain.Fail("pin_unavailable", "The requested harness has no supported native context.")
		}
	}
	if input.Options.TargetProfileID != "" {
		var selected *domain.TargetProfile
		for i := range profiles {
			if profiles[i].ID == input.Options.TargetProfileID {
				selected = &profiles[i]
				break
			}
		}
		if selected == nil || forcedContext != "" && forcedContext != selected.ContextID || input.Pins.Model != "" && input.Pins.Model != selected.ModelSelector {
			return result, domain.Fail("pin_unavailable", "The explicit profile and literal harness/model choices conflict.")
		}
		forcedContext = selected.ContextID
	} else if input.Pins.Model != "" && forcedContext == "" {
		for _, profile := range profiles {
			if profile.ModelSelector == input.Pins.Model {
				if forcedContext != "" && forcedContext != profile.ContextID {
					return result, domain.Fail("pin_unavailable", "The model-only choice has no unambiguous native context.")
				}
				forcedContext = profile.ContextID
			}
		}
		if forcedContext == "" {
			return result, domain.Fail("pin_unavailable", "Supply an explicit compatible harness for this uncatalogued native selector.")
		}
	}
	if input.Pins.Model != "" && input.Options.TargetProfileID == "" {
		matched := false
		for _, profile := range profiles {
			if profile.ContextID == forcedContext && profile.ModelSelector == input.Pins.Model {
				matched = true
			}
		}
		if !matched {
			profiles = append(profiles, domain.TargetProfile{ContextID: forcedContext, ModelSelector: input.Pins.Model, Role: "explicit", Enabled: true, Revision: "1"})
		}
	}
	class := input.Assessment.Classification
	if class == "" {
		class = "unknown"
	}
	if class != "unknown" && class != "routine" && class != "general" {
		return result, domain.Invalid("suitability assessment")
	}
	capacities := map[string]domain.CapacityView{}
	for _, context := range input.Snapshot.Contexts {
		view, err := Capacity(input.Snapshot, domain.CapacitySubject{Kind: "context", ID: context.ID}, input.Now)
		if err != nil {
			return result, err
		}
		capacities[context.ID] = view
		result.CapacityEvidence = append(result.CapacityEvidence, compactCapacity(view))
	}
	roleRanks, availabilityRanks := []int{}, []int{}
	prices := []*domain.ReferencePrice{}
	for _, profile := range profiles {
		candidate := domain.CandidateAssessment{ContextID: profile.ContextID, Eligible: true, ReasonCodes: []string{}, Uncertainties: []string{"authenticated_account_unknown", "model_access_unknown", "actual_model_unknown", "native_billing_unknown"}}
		if profile.ID != "" {
			copy := profile
			candidate.ProfileID, candidate.ProfileSnapshot = stringPointer(profile.ID), &copy
		}
		exclude := func(reason string) {
			candidate.Eligible = false
			candidate.ReasonCodes = append(candidate.ReasonCodes, reason)
		}
		context, exists := contexts[profile.ContextID]
		if !exists || !context.Enabled {
			exclude("native_context_disabled")
		}
		if !profile.Enabled {
			exclude("profile_disabled")
		}
		if forcedContext != "" && forcedContext != profile.ContextID {
			exclude("literal_harness_constraint")
		}
		if input.Options.TargetProfileID != "" && input.Options.TargetProfileID != profile.ID {
			exclude("explicit_profile_constraint")
		}
		if input.Pins.Model != "" && input.Pins.Model != profile.ModelSelector {
			exclude("literal_model_constraint")
		}
		fact, hasFact := facts[profile.ContextID]
		if !hasFact || fact.InstallationStatus != "installed" {
			exclude("native_installation_unavailable")
		}
		minimum := profile.Source.MinimumNativeVersion
		if minimum == nil {
			minimum = context.Capabilities.MinimumVersion
		}
		observedVersion := false
		if fact.NativeVersion != nil && fact.NativeVersion.Status == "observed" && fact.NativeVersion.Value != nil && fact.NativeVersion.Source != nil && fact.NativeVersion.ObservedAt != nil {
			at, err := time.Parse(time.RFC3339Nano, *fact.NativeVersion.ObservedAt)
			if err == nil && !at.After(mustTime(input.Now)) {
				if minimum != nil {
					less, known := knownVersionLess(*fact.NativeVersion.Value, *minimum)
					observedVersion = known
					if known && less {
						exclude("known_native_version_incompatible")
					}
				} else {
					observedVersion = true
				}
			}
		}
		if !observedVersion {
			candidate.Uncertainties = append(candidate.Uncertainties, "native_version_unknown")
		}
		capacity := capacities[profile.ContextID]
		if !capacity.ReportEligibility.Permitted {
			exclude("capacity_reports_deny")
			candidate.ReasonCodes = append(candidate.ReasonCodes, capacity.ReportEligibility.ReasonCodes...)
		}
		if capacity.Dimensions.Occupancy.EffectiveState == "unknown" {
			candidate.Uncertainties = append(candidate.Uncertainties, "external_occupancy_unknown")
		}
		if capacity.Dimensions.Allowance.EffectiveState == "unknown" {
			candidate.Uncertainties = append(candidate.Uncertainties, "allowance_unknown")
		}
		explicit := input.Options.TargetProfileID != "" || input.Pins.Model != ""
		rank := 0
		if !explicit {
			switch class {
			case "unknown":
				if profile.Role != "native_default" {
					exclude("unknown_suitability_requires_native_default")
				}
			case "routine":
				switch profile.Role {
				case "economy":
					rank = 0
				case "general":
					rank = 1
				default:
					rank = 2
				}
			case "general":
				if profile.Role == "economy" {
					exclude("general_suitability_required")
				}
				if profile.Role == "native_default" {
					rank = 1
				}
			}
		}
		if class == "unknown" || profile.Role == "explicit" || profile.Role == "native_default" {
			candidate.Uncertainties = append(candidate.Uncertainties, "suitability_not_verified")
		}
		availability := 2
		if capacity.ReportEligibility.PositiveCoverage.Occupancy && capacity.ReportEligibility.PositiveCoverage.Allowance {
			availability = 0
		} else if capacity.ReportEligibility.PositiveCoverage.Occupancy || capacity.ReportEligibility.PositiveCoverage.Allowance {
			availability = 1
		}
		if candidate.Eligible && input.RequireManagedReservation && managedReservationOccupied(input.Snapshot, profile.ContextID) {
			candidate.Eligible = false
			candidate.ReasonCodes = append(candidate.ReasonCodes, "managed_capacity_occupied")
			capacityWaiting = true
		}
		if candidate.Eligible {
			candidate.ReasonCodes = append(candidate.ReasonCodes, "captured_policy_eligible")
		}
		sort.Strings(candidate.ReasonCodes)
		sort.Strings(candidate.Uncertainties)
		result.Candidates = append(result.Candidates, candidate)
		roleRanks = append(roleRanks, rank)
		availabilityRanks = append(availabilityRanks, availability)
		prices = append(prices, profile.ReferencePrice)
	}
	indices := []int{}
	bestAvailability, bestRole := 3, 3
	for i, candidate := range result.Candidates {
		if candidate.Eligible {
			if availabilityRanks[i] < bestAvailability || availabilityRanks[i] == bestAvailability && roleRanks[i] < bestRole {
				bestAvailability, bestRole = availabilityRanks[i], roleRanks[i]
				indices = []int{i}
			} else if availabilityRanks[i] == bestAvailability && roleRanks[i] == bestRole {
				indices = append(indices, i)
			}
		}
	}
	if len(indices) == 0 {
		if capacityWaiting {
			return result, domain.Fail("capacity_wait", "Wait for an available managed slot, native context or declared account reservation.")
		}
		code := "routing_unavailable"
		if input.Pins.HarnessID != "" || input.Pins.Model != "" || input.Options.TargetProfileID != "" {
			code = "pin_unavailable"
		}
		return result, domain.Fail(code, "No native target meets the captured choices and current capability/capacity controls. Inspect the reports and explicit choices.")
	}
	order := append([]string{}, input.Snapshot.Policy.EconomyOrder...)
	for _, profile := range Catalog() {
		if !contains(order, profile.ID) {
			order = append(order, profile.ID)
		}
	}
	rankOrder := func(index int) int {
		for rank, id := range order {
			if profiles[index].ID == id {
				return rank
			}
		}
		return len(order)
	}
	sort.SliceStable(indices, func(i, j int) bool { return rankOrder(indices[i]) < rankOrder(indices[j]) })
	indices = priceFrontier(indices, prices, input.Now)
	selectedIndex := indices[0]
	profile := profiles[selectedIndex]
	context := contexts[profile.ContextID]
	result.SelectedTarget = domain.ResolvedTarget{ContextID: context.ID, ConnectionID: context.ConnectionID, HarnessID: context.HarnessID, ModelSelector: profile.ModelSelector, ProfileID: copyString(result.Candidates[selectedIndex].ProfileID)}
	if link := capacities[context.ID].AccountLink; link != nil && link.DeclaredAccountID != nil {
		for _, account := range input.Snapshot.Accounts {
			if account.ID == *link.DeclaredAccountID {
				result.SelectedTarget.DeclaredAccountAssociation = &domain.DeclaredAccountAssociation{AccountID: account.ID, Label: account.Label, LinkRevision: link.Revision, Source: "user_declaration"}
				break
			}
		}
	}
	if class == "routine" && profile.Role != "economy" && input.Options.TargetProfileID == "" && input.Pins.Model == "" {
		result.Candidates[selectedIndex].ReasonCodes = append(result.Candidates[selectedIndex].ReasonCodes, "routine_economy_unavailable_fallback")
	}
	result.Uncertainties = append(result.Uncertainties, result.Candidates[selectedIndex].Uncertainties...)
	_, comparablePrice := ComparePrice(profile.ReferencePrice, profile.ReferencePrice, input.Now)
	if !comparablePrice {
		result.Uncertainties = append(result.Uncertainties, "price_comparison_unknown")
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 60<<10 {
		return domain.RoutingSelectionResult{}, domain.Fail("routing_unavailable", "The bounded routing decision cannot represent this evidence; inspect retained report history.")
	}
	return result, nil
}

func mustTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func RequiresReplacement(previous, next domain.ResolvedTarget) bool {
	return previous.ContextID != next.ContextID || previous.ModelSelector != next.ModelSelector
}

func RequiresEscalation(previous, next domain.ResolvedTarget, snapshot domain.RoutingSnapshot, now string) bool {
	if !RequiresReplacement(previous, next) {
		return false
	}
	var before, after *domain.TargetProfile
	for i := range snapshot.Profiles {
		p := &snapshot.Profiles[i]
		if previous.ProfileID != nil && p.ID == *previous.ProfileID {
			before = p
		}
		if next.ProfileID != nil && p.ID == *next.ProfileID {
			after = p
		}
	}
	if before == nil || after == nil {
		return true
	}
	a, b := Tier(before.Role), Tier(after.Role)
	if a == nil || b == nil || *b > *a {
		return true
	}
	price, comparable := ComparePrice(after.ReferencePrice, before.ReferencePrice, now)
	return !comparable || price > 0
}
