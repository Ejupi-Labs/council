package routing

import (
	"strings"
	"testing"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func TestUsageLocalExecutorsDoNotBecomeModelCoverageCountersOrPrices(t *testing.T) {
	author := usageItem(strings.Repeat("1", 32), "top_level", stringPointer("7"), stringPointer("3"), stringPointer("1"), stringPointer("0"))
	author.ExecutorKind, author.Role = "native", "author"
	items := []domain.UsageItem{author}
	references := []domain.UsageReferencePrice{}
	for i, kind := range []string{"local_check", "local_application", "local_external", "local_graph"} {
		id := strings.Repeat(string(rune('2'+i)), 32)
		local := usageItem(id, "top_level", stringPointer("999"), stringPointer("999"), stringPointer("999"), stringPointer("999"))
		local.ExecutorKind, local.Role = kind, "check"
		local.ExecutionMode = "fake"
		local.NativeModels = &domain.NativeModelsObservation{Models: []domain.NativeModelUsage{{Model: "untrusted-local-model-label", CostEstimateUSD: stringPointer("999")}}}
		items = append(items, local)
		references = append(references, domain.UsageReferencePrice{AttemptID: id, DecisionID: domain.NewID(), ReferencePrice: domain.ReferencePrice{Currency: "USD", SourceKind: "user_report", SourceLabel: "Controlled metadata fixture", ObservedAt: testNow}})
	}
	page, err := Usage(strings.Repeat("f", 32), items, references, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Coverage.Status != "complete" || len(page.Coverage.IncludedAttemptIDs) != 1 || page.Coverage.IncludedAttemptIDs[0] != author.AttemptID || len(page.Coverage.UnknownAttemptIDs) != 0 || len(page.Coverage.ExcludedFakeAttemptIDs) != 0 || len(page.Coverage.ExcludedLocalAttemptIDs) != 4 {
		t.Fatal("Local executors distorted measured native model coverage", page.Coverage)
	}
	if got := page.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue; got == nil || *got != "7" {
		t.Fatal("Local counters were summed into native observations", got)
	}
	if len(page.Estimates) != 0 || len(page.ReferencePrices) != 0 || page.Charges.Status != "unknown" {
		t.Fatal("Local executor fields became model prices or charges", page)
	}
	for _, item := range page.Items[1:] {
		if item.SelectedUsageScope != "local_not_applicable" {
			t.Fatal("A local process was presented as fake/native model work", item)
		}
	}
}

func TestUsageOnlyLocalIsNotApplicableRatherThanMeasuredZero(t *testing.T) {
	local := domain.UsageItem{AttemptID: domain.NewID(), AttemptSequence: "1", ExecutionMode: "native", ExecutorKind: "local_check", Role: "check"}
	page, err := Usage(domain.NewID(), []domain.UsageItem{local}, nil, nil)
	if err != nil || page.Coverage.Status != "not_applicable" || page.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != nil || len(page.Coverage.UnknownAttemptIDs) != 0 || len(page.Coverage.ExcludedLocalAttemptIDs) != 1 {
		t.Fatal("A no-model page claimed a native measurement or missing model coverage", page, err)
	}
}
