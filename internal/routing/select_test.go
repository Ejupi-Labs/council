package routing

import (
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func selectionInput(class string) domain.RoutingSelectionInput {
	return domain.RoutingSelectionInput{Snapshot: testSnapshot(), Assessment: domain.SuitabilityAssessment{Classification: class, Source: "unknown", ReasonCodes: []string{}}, Facts: []domain.NativeContextFact{{ContextID: "codex-native", InstallationStatus: "installed"}, {ContextID: "claude-native", InstallationStatus: "installed"}}, Now: testNow}
}

func TestSelectionRolesPinsAndManagedQueue(t *testing.T) {
	for _, tc := range []struct{ class, profile string }{{"unknown", "codex-native-default"}, {"routine", "codex-luna-economy"}, {"general", "codex-sol-general"}} {
		in := selectionInput(tc.class)
		in.Snapshot.ManagedOccupancy.ActiveAttemptID = stringPointer("0123456789abcdef0123456789abcdef")
		in.Snapshot.ManagedOccupancy.ActiveContextID = stringPointer("claude-native")
		got, err := Select(in)
		if err != nil || got.SelectedTarget.ProfileID == nil || *got.SelectedTarget.ProfileID != tc.profile || len(got.Candidates) != 6 || len(got.CapacityEvidence) != 2 {
			t.Fatal(tc.class, got, err)
		}
	}
	in := selectionInput("unknown")
	in.Pins.Model = "haiku"
	got, err := Select(in)
	if err != nil || got.SelectedTarget.ConnectionID != "claude-native" || got.SelectedTarget.ModelSelector != "haiku" || in.Pins.HarnessID != "" {
		t.Fatal("exact catalog model-only pin did not resolve without rewriting input", got, err)
	}
	in.Pins.Model = "uncatalogued-model"
	if _, err := Select(in); err == nil || err.Code != "pin_unavailable" {
		t.Fatal("ambiguous uncatalogued model inferred a harness", err)
	}
	in.Pins.HarnessID = "claude-code"
	got, err = Select(in)
	if err != nil || got.SelectedTarget.ProfileID != nil || got.SelectedTarget.ModelSelector != in.Pins.Model {
		t.Fatal(got, err)
	}
	in.Options.TargetProfileID = "claude-native-default"
	if _, err := Select(in); err == nil || err.Code != "pin_unavailable" {
		t.Fatal("default profile silently accepted a conflicting literal selector", err)
	}
}

func TestSelectionCapacityAndVersionNegativesPrecedeEconomy(t *testing.T) {
	in := selectionInput("routine")
	r := testReport(1, in.Snapshot.CapacityStates[0].Subject, "occupancy", "busy")
	in.Snapshot.CapacityStates[0].Heads.Occupancy = &r
	got, err := Select(in)
	if err != nil || got.SelectedTarget.ContextID != "claude-native" || got.SelectedTarget.ModelSelector != "haiku" {
		t.Fatal(got, err)
	}
	version, source, at := "2.1.280", "native_invocation", "2026-11-06T11:00:00.000Z"
	in.Facts[1].NativeVersion = &domain.ObservedFact{Status: "observed", Value: &version, Source: &source, ObservedAt: &at}
	got, err = Select(in)
	if err != nil || got.SelectedTarget.ModelSelector != "" {
		t.Fatal("known old version retained incompatible curated alias", got, err)
	}
	in.Pins.HarnessID, in.Pins.Model = "claude-code", "haiku"
	if _, err := Select(in); err == nil {
		t.Fatal("explicit unavailable alias was substituted")
	}
}

func referencePrice(input, output string) *domain.ReferencePrice {
	return &domain.ReferencePrice{Currency: "USD", Unit: stringPointer("per_million_tokens"), Basis: stringPointer("api_list"), InputPerMillion: stringPointer(input), OutputPerMillion: stringPointer(output), SourceKind: "user_reference", SourceLabel: "controlled declaration", ObservedAt: "2026-11-06T11:00:00.000Z"}
}

func TestPriceFrontierHandlesIncomparabilityWithoutComparatorCycles(t *testing.T) {
	in := selectionInput("routine")
	// General profiles are not in the economy frontier. Across the two economy
	// candidates, crossed rates cannot be collapsed into a guessed token mix.
	in.Snapshot.Profiles[1].ReferencePrice = referencePrice("1", "9")
	in.Snapshot.Profiles[4].ReferencePrice = referencePrice("2", "8")
	in.Snapshot.Policy.EconomyOrder = []string{"claude-haiku-economy", "codex-luna-economy"}
	got, err := Select(in)
	if err != nil || *got.SelectedTarget.ProfileID != "claude-haiku-economy" {
		t.Fatal(got, err)
	}
	in.Snapshot.Profiles[1].ReferencePrice = referencePrice("1", "7")
	got, err = Select(in)
	if err != nil || *got.SelectedTarget.ProfileID != "codex-luna-economy" {
		t.Fatal("dominated preference won", got, err)
	}
	in.Snapshot.Profiles[1].ReferencePrice.InputPerMillion = stringPointer("1e999")
	got, err = Select(in)
	if err != nil || *got.SelectedTarget.ProfileID != "claude-haiku-economy" {
		t.Fatal("out-of-domain declaration invented dominance", got, err)
	}
	// Directly test three-candidate nondominated selection to cover the cycle
	// that would arise from Pareto-plus-tie-order used as a sort comparator.
	prices := []*domain.ReferencePrice{referencePrice("1", "1"), nil, referencePrice("2", "2")}
	if got := priceFrontier([]int{2, 1, 0}, prices, testNow); len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Fatal(got)
	}
}
