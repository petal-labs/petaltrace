package diff

import (
	"github.com/petal-labs/petaltrace/store"
)

// CostDiffer compares token usage and costs between runs
type CostDiffer struct{}

// NewCostDiffer creates a new cost differ
func NewCostDiffer() *CostDiffer {
	return &CostDiffer{}
}

// CompareRuns computes the cost difference between two runs
func (d *CostDiffer) CompareRuns(baseRun, compareRun *store.Run) store.CostDiff {
	diff := store.CostDiff{
		BaseCost:    baseRun.EstimatedCost.Total,
		CompareCost: compareRun.EstimatedCost.Total,
		Delta:       compareRun.EstimatedCost.Total - baseRun.EstimatedCost.Total,
		ByProvider:  make(map[string]float64),
		ByModel:     make(map[string]float64),
	}

	// Calculate provider deltas
	allProviders := make(map[string]bool)
	for provider := range baseRun.EstimatedCost.ByProvider {
		allProviders[provider] = true
	}
	for provider := range compareRun.EstimatedCost.ByProvider {
		allProviders[provider] = true
	}

	for provider := range allProviders {
		baseCost := baseRun.EstimatedCost.ByProvider[provider]
		compareCost := compareRun.EstimatedCost.ByProvider[provider]
		diff.ByProvider[provider] = compareCost - baseCost
	}

	// Calculate model deltas
	allModels := make(map[string]bool)
	for model := range baseRun.EstimatedCost.ByModel {
		allModels[model] = true
	}
	for model := range compareRun.EstimatedCost.ByModel {
		allModels[model] = true
	}

	for model := range allModels {
		baseCost := baseRun.EstimatedCost.ByModel[model]
		compareCost := compareRun.EstimatedCost.ByModel[model]
		diff.ByModel[model] = compareCost - baseCost
	}

	return diff
}

// CompareTokens computes the token difference between two spans
func (d *CostDiffer) CompareTokens(baseSpan, compareSpan *store.Span) *store.TokenDiff {
	baseTokens := store.TokenDetail{}
	compareTokens := store.TokenDetail{}

	if baseSpan != nil && baseSpan.LLM != nil {
		baseTokens = store.TokenDetail{
			InputTokens:  baseSpan.LLM.Tokens.InputTokens,
			OutputTokens: baseSpan.LLM.Tokens.OutputTokens,
			TotalTokens:  baseSpan.LLM.Tokens.TotalTokens,
			CostEstimate: baseSpan.LLM.Tokens.CostEstimate,
		}
	}

	if compareSpan != nil && compareSpan.LLM != nil {
		compareTokens = store.TokenDetail{
			InputTokens:  compareSpan.LLM.Tokens.InputTokens,
			OutputTokens: compareSpan.LLM.Tokens.OutputTokens,
			TotalTokens:  compareSpan.LLM.Tokens.TotalTokens,
			CostEstimate: compareSpan.LLM.Tokens.CostEstimate,
		}
	}

	return &store.TokenDiff{
		BaseTokens:    baseTokens,
		CompareTokens: compareTokens,
		Delta:         compareTokens.TotalTokens - baseTokens.TotalTokens,
	}
}

// AggregateTokenDiffs aggregates multiple token diffs
func (d *CostDiffer) AggregateTokenDiffs(diffs []*store.TokenDiff) TokenAggregates {
	agg := TokenAggregates{}

	for _, diff := range diffs {
		if diff == nil {
			continue
		}

		agg.TotalBaseTokens += diff.BaseTokens.TotalTokens
		agg.TotalCompareTokens += diff.CompareTokens.TotalTokens
		agg.TotalDelta += diff.Delta

		agg.BaseInputTokens += diff.BaseTokens.InputTokens
		agg.CompareInputTokens += diff.CompareTokens.InputTokens
		agg.BaseOutputTokens += diff.BaseTokens.OutputTokens
		agg.CompareOutputTokens += diff.CompareTokens.OutputTokens

		agg.TotalBaseCost += diff.BaseTokens.CostEstimate
		agg.TotalCompareCost += diff.CompareTokens.CostEstimate
	}

	agg.CostDelta = agg.TotalCompareCost - agg.TotalBaseCost

	return agg
}

// TokenAggregates holds aggregated token statistics
type TokenAggregates struct {
	TotalBaseTokens     int
	TotalCompareTokens  int
	TotalDelta          int
	BaseInputTokens     int
	CompareInputTokens  int
	BaseOutputTokens    int
	CompareOutputTokens int
	TotalBaseCost       float64
	TotalCompareCost    float64
	CostDelta           float64
}

// PercentChange calculates the percentage change in tokens
func (a TokenAggregates) PercentChange() float64 {
	if a.TotalBaseTokens == 0 {
		if a.TotalCompareTokens == 0 {
			return 0
		}
		return 100.0 // Infinite increase approximated as 100%
	}
	return float64(a.TotalDelta) / float64(a.TotalBaseTokens) * 100
}

// CostPercentChange calculates the percentage change in cost
func (a TokenAggregates) CostPercentChange() float64 {
	if a.TotalBaseCost == 0 {
		if a.TotalCompareCost == 0 {
			return 0
		}
		return 100.0
	}
	return a.CostDelta / a.TotalBaseCost * 100
}
