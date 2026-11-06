package routing

import (
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"strings"
	"testing"
)

func usageItem(id, scope string, values ...*string) domain.UsageItem {
	u := domain.UsageObservation{Scope: scope, Source: stringPointer("native.fixture.shape"), ObservedAt: stringPointer(testNow), Completeness: "partial"}
	fields := []**string{&u.InputTokens, &u.OutputTokens, &u.CachedInputTokens, &u.CacheCreationInputTokens}
	for i, value := range values {
		*fields[i] = value
	}
	return domain.UsageItem{AttemptID: id, AttemptSequence: "1", ExecutionMode: "native", Usage: u, EvidenceArtifactIDs: []string{strings.Repeat("a", 32)}}
}
func TestUsageSumsExactScopesOnceAndKeepsMissingFakeAndEstimatesSeparate(t *testing.T) {
	ids := []string{strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)}
	max := stringPointer("9223372036854775807")
	zero := stringPointer("0")
	a := usageItem(ids[0], "top_level", max, zero, nil, nil)
	a.NativeModels = &domain.NativeModelsObservation{Scope: "whole_tree", Source: "claude.result.modelUsage", ObservedAt: testNow, Completeness: "partial", Models: []domain.NativeModelUsage{{Model: "actual-native", InputTokens: max, CostEstimateUSD: stringPointer("1e999")}}, EvidenceArtifactIDs: []string{strings.Repeat("a", 32)}}
	b := usageItem(ids[1], "top_level", max, nil, zero, zero)
	c := usageItem(ids[2], "whole_tree", stringPointer("7"), zero, zero, zero)
	fake := usageItem(ids[3], "top_level", max, max, max, max)
	fake.ExecutionMode = "fake"
	fake.NativeModels = a.NativeModels
	page, err := Usage(strings.Repeat("f", 32), []domain.UsageItem{a, b, c, fake}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sums := page.KnownSubtotal.ByUsageScope
	if sums.TopLevel.InputTokens.KnownValue == nil || *sums.TopLevel.InputTokens.KnownValue != "18446744073709551614" || *sums.WholeTree.InputTokens.KnownValue != "7" || *sums.TopLevel.OutputTokens.KnownValue != "0" {
		t.Fatalf("lost exact/separate totals %+v", sums)
	}
	if len(sums.TopLevel.OutputTokens.MissingAttemptIDs) != 1 || sums.TopLevel.OutputTokens.MissingAttemptIDs[0] != ids[1] || len(page.Coverage.ExcludedFakeAttemptIDs) != 1 || page.Items[3].SelectedUsageScope != "fake_unmeasured" || page.Coverage.Status != "partial" {
		t.Fatalf("coverage %+v", page)
	}
	if len(page.Estimates) != 1 || page.Estimates[0].Value != "1e999" || page.Estimates[0].Scope != "whole_tree_model" || page.Charges.Status != "unknown" || len(page.Charges.Items) != 0 {
		t.Fatalf("estimate became cost %+v", page)
	}
	if a.SelectedUsageScope != "" || a.NativeModels.Models[0].CostEstimateUSD == nil || *a.NativeModels.Models[0].CostEstimateUSD != "1e999" {
		t.Fatal("input mutated")
	}
}
func TestUsageUnknownNeverBecomesZeroAndAllHundredMaximaFit(t *testing.T) {
	task := strings.Repeat("f", 32)
	empty, err := Usage(task, nil, nil, nil)
	if err != nil || empty.Coverage.Status != "unknown" || empty.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != nil || len(empty.Items) != 0 {
		t.Fatalf("empty %+v %v", empty, err)
	}
	items := []domain.UsageItem{}
	for i := 0; i < 100; i++ {
		id := strings.Repeat("0", 30) + string("0123456789abcdef"[i/16]) + string("0123456789abcdef"[i%16])
		v := stringPointer("9223372036854775807")
		items = append(items, usageItem(id, "top_level", v, v, v, v))
	}
	page, err := Usage(task, items, nil, nil)
	if err != nil || *page.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != "922337203685477580700" || page.Coverage.Status != "complete" {
		t.Fatalf("page sum %+v %v", page.KnownSubtotal, err)
	}
	unknown := usageItem(strings.Repeat("a", 32), "", stringPointer("7"))
	unknown.Usage.Source = nil
	page, err = Usage(task, []domain.UsageItem{unknown}, nil, nil)
	if err != nil || page.Items[0].SelectedUsageScope != "unknown" || page.KnownSubtotal.ByUsageScope.TopLevel.InputTokens.KnownValue != nil || page.Coverage.Status != "unknown" {
		t.Fatalf("unknown %+v %v", page, err)
	}
}
