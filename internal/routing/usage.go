package routing

import (
	json "encoding/json/v2"
	"math/big"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
)

func emptyCounter() domain.CounterSubtotal {
	return domain.CounterSubtotal{ContributingAttemptIDs: []string{}, MissingAttemptIDs: []string{}}
}
func emptyScope() domain.ScopeSubtotal {
	return domain.ScopeSubtotal{InputTokens: emptyCounter(), OutputTokens: emptyCounter(), CachedInputTokens: emptyCounter(), CacheCreationInputTokens: emptyCounter()}
}
func sumCounter(counter *domain.CounterSubtotal, value *string, id string) bool {
	if value == nil {
		counter.MissingAttemptIDs = append(counter.MissingAttemptIDs, id)
		return false
	}
	if _, err := domain.ParseCount(*value); err != nil {
		counter.MissingAttemptIDs = append(counter.MissingAttemptIDs, id)
		return false
	}
	n, _ := new(big.Int).SetString(*value, 10)
	if counter.KnownValue != nil {
		previous, _ := new(big.Int).SetString(*counter.KnownValue, 10)
		n.Add(n, previous)
	}
	counter.KnownValue = stringPointer(n.String())
	counter.ContributingAttemptIDs = append(counter.ContributingAttemptIDs, id)
	return true
}

// Usage reduces only the bounded returned attempt page. Native per-model previews
// are retained/displayed separately and never added to the selected usage scope.
// Store owns high-water cursor validation and whole-item byte-budget pagination.
func Usage(taskID string, items []domain.UsageItem, references []domain.UsageReferencePrice, nextCursor *string) (domain.UsagePage, *domain.Error) {
	page := domain.UsagePage{TaskID: taskID, Items: []domain.UsageItem{}, NextCursor: copyString(nextCursor), KnownSubtotal: domain.UsageSubtotal{Scope: "returned_attempts", ByUsageScope: domain.UsageScopeSubtotals{TopLevel: emptyScope(), WholeTree: emptyScope()}}, Coverage: domain.UsageCoverage{Scope: "returned_attempts", Status: "unknown", IncludedAttemptIDs: []string{}, UnknownAttemptIDs: []string{}, ExcludedFakeAttemptIDs: []string{}}, Estimates: []domain.EstimateItem{}, ReferencePrices: []domain.UsageReferencePrice{}, Charges: domain.UsageCharges{Status: "unknown", Items: []struct{}{}}}
	if !domain.ValidID(taskID) || len(items) > 100 || len(references) > 100 {
		return page, domain.Invalid("bounded task usage page")
	}
	// Detach all nested observations and preserve their exact string lexemes.
	encoded, err := json.Marshal(items)
	if err != nil {
		return page, domain.Invalid("usage observations")
	}
	if len(items) > 0 {
		if err = json.Unmarshal(encoded, &page.Items); err != nil {
			return page, domain.Invalid("usage observations")
		}
	}
	known, complete := 0, len(items) > 0
	nativeCount := 0
	seen := map[string]bool{}
	modelIDs := map[string]bool{}
	for i := range page.Items {
		item := &page.Items[i]
		if !domain.ValidID(item.AttemptID) || seen[item.AttemptID] {
			return page, domain.Invalid("unique returned attempt identities")
		}
		seen[item.AttemptID] = true
		switch item.ExecutorKind {
		case "local_check", "local_application", "local_external":
			item.SelectedUsageScope = "local_not_applicable"
			page.Coverage.ExcludedLocalAttemptIDs = append(page.Coverage.ExcludedLocalAttemptIDs, item.AttemptID)
			continue
		case "", "native":
			nativeCount++
			modelIDs[item.AttemptID] = true
		default:
			return page, domain.Invalid("known usage executor kind")
		}
		if item.ExecutionMode == "fake" {
			item.SelectedUsageScope = "fake_unmeasured"
			page.Coverage.ExcludedFakeAttemptIDs = append(page.Coverage.ExcludedFakeAttemptIDs, item.AttemptID)
			complete = false
			continue
		}
		usage := item.Usage
		item.SelectedUsageScope = "unknown"
		sourced := usage.Source != nil && *usage.Source != "" && usage.ObservedAt != nil
		if sourced {
			_, e := domain.ParseRoutingTime(*usage.ObservedAt)
			sourced = e == nil
		}
		hasCounter := false
		for _, value := range []*string{usage.InputTokens, usage.OutputTokens, usage.CachedInputTokens, usage.CacheCreationInputTokens} {
			if value != nil {
				if _, e := domain.ParseCount(*value); e == nil {
					hasCounter = true
				}
			}
		}
		if sourced && hasCounter && (usage.Scope == "top_level" || usage.Scope == "whole_tree") {
			item.SelectedUsageScope = usage.Scope
			scope := &page.KnownSubtotal.ByUsageScope.TopLevel
			if usage.Scope == "whole_tree" {
				scope = &page.KnownSubtotal.ByUsageScope.WholeTree
			}
			a := sumCounter(&scope.InputTokens, usage.InputTokens, item.AttemptID)
			b := sumCounter(&scope.OutputTokens, usage.OutputTokens, item.AttemptID)
			c := sumCounter(&scope.CachedInputTokens, usage.CachedInputTokens, item.AttemptID)
			d := sumCounter(&scope.CacheCreationInputTokens, usage.CacheCreationInputTokens, item.AttemptID)
			known++
			page.Coverage.IncludedAttemptIDs = append(page.Coverage.IncludedAttemptIDs, item.AttemptID)
			if !a || !b || !c || !d {
				complete = false
				page.Coverage.UnknownAttemptIDs = append(page.Coverage.UnknownAttemptIDs, item.AttemptID)
			}
		} else {
			complete = false
			page.Coverage.UnknownAttemptIDs = append(page.Coverage.UnknownAttemptIDs, item.AttemptID)
		}
		if models := item.NativeModels; models != nil {
			if len(models.Models) > domain.MaxNativeModels {
				return page, domain.Invalid("bounded native model preview")
			}
			for _, model := range models.Models {
				if model.CostEstimateUSD != nil {
					page.Estimates = append(page.Estimates, domain.EstimateItem{AttemptID: item.AttemptID, Model: model.Model, Currency: "USD", Value: *model.CostEstimateUSD, Scope: "whole_tree_model", Source: models.Source, ObservedAt: models.ObservedAt, EvidenceArtifactIDs: append([]string{}, models.EvidenceArtifactIDs...), PreviewCompleteness: models.Completeness})
				}
			}
		}
	}
	if nativeCount == 0 && len(page.Coverage.ExcludedLocalAttemptIDs) > 0 {
		page.Coverage.Status = "not_applicable"
	} else if complete {
		page.Coverage.Status = "complete"
	} else if known > 0 {
		page.Coverage.Status = "partial"
	}
	for _, reference := range references {
		if modelIDs[reference.AttemptID] {
			reference.ProfileID = copyString(reference.ProfileID)
			reference.ReferencePrice = *copyProfile(domain.TargetProfile{ReferencePrice: &reference.ReferencePrice}).ReferencePrice
			page.ReferencePrices = append(page.ReferencePrices, reference)
		}
	}
	return page, nil
}
