package routing

import (
	"reflect"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestCatalogHasSixLiteralProfilesAndDetachedDefaults(t *testing.T) {
	profiles := Catalog()
	var selectors []string
	for _, profile := range profiles {
		selectors = append(selectors, profile.ModelSelector)
		if profile.Revision != "1" || !profile.Enabled || profile.Source.Kind != "documented_profile" || profile.Source.ReadDate != "2026-11-06" || profile.ReferencePrice != nil {
			t.Fatal("catalog invented account, price or mutable source evidence", profile)
		}
	}
	if !reflect.DeepEqual(selectors, []string{"", "gpt-6-luna", "gpt-6.1-sol", "", "haiku", "sonnet"}) {
		t.Fatal(selectors)
	}
	if *profiles[4].Source.MinimumNativeVersion != "2.1.293" || *profiles[5].Source.MinimumNativeVersion != "2.1.284" || profiles[1].Source.MinimumNativeVersion != nil {
		t.Fatal("alias compatibility became a fabricated native version")
	}
	*profiles[4].Source.MinimumNativeVersion = "changed"
	if *Catalog()[4].Source.MinimumNativeVersion != "2.1.293" {
		t.Fatal("caller mutated catalog source")
	}
	contexts := Contexts()
	if len(contexts) != 2 || contexts[0].ConnectionID != "codex-native" || contexts[1].Capabilities.MinimumVersion == nil || *contexts[1].Capabilities.MinimumVersion != "2.1.259" || contexts[1].Capabilities.LiveQualification != "unqualified" || contexts[1].Capabilities.Limits.FinalResponseBytesUpperBound != 1<<20 {
		t.Fatal("context profile changed", contexts)
	}
	contexts[0].Capabilities.SupportedPolicyModes[0] = "changed"
	if Contexts()[0].Capabilities.SupportedPolicyModes[0] != "autonomous" {
		t.Fatal("caller mutated capability declaration")
	}
	policy := DefaultPolicy()
	if policy.DefaultBudget != (domain.RoutingBudgetDefaults{AdmissionLimit: "2", WindowSeconds: 1800}) || policy.UnknownAvailability != "allow" || policy.Continuation.MaxAutomaticAdmissions != "0" || len(policy.Continuation.Categories) != 0 || policy.Continuation.AllowReplacement || policy.Continuation.AllowEscalation {
		t.Fatal("default authority widened", policy)
	}
}
