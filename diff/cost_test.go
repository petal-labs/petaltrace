package diff

import (
	"testing"

	"github.com/petal-labs/petaltrace/store"
)

func TestCostDiffer_CompareRuns(t *testing.T) {
	d := NewCostDiffer()

	baseRun := &store.Run{
		EstimatedCost: store.CostEstimate{
			Total: 0.05,
			ByProvider: map[string]float64{
				"openai":    0.03,
				"anthropic": 0.02,
			},
			ByModel: map[string]float64{
				"gpt-4":      0.03,
				"claude-3.5": 0.02,
			},
		},
	}

	compareRun := &store.Run{
		EstimatedCost: store.CostEstimate{
			Total: 0.08,
			ByProvider: map[string]float64{
				"openai":    0.05,
				"anthropic": 0.02,
				"google":    0.01,
			},
			ByModel: map[string]float64{
				"gpt-4":      0.05,
				"claude-3.5": 0.02,
				"gemini-pro": 0.01,
			},
		},
	}

	result := d.CompareRuns(baseRun, compareRun)

	// Check total costs
	if result.BaseCost != 0.05 {
		t.Errorf("BaseCost = %f, want 0.05", result.BaseCost)
	}
	if result.CompareCost != 0.08 {
		t.Errorf("CompareCost = %f, want 0.08", result.CompareCost)
	}
	if result.Delta != 0.03 {
		t.Errorf("Delta = %f, want 0.03", result.Delta)
	}

	// Check provider deltas (use tolerance for float comparison)
	const tolerance = 0.0001
	if delta := result.ByProvider["openai"]; abs(delta-0.02) > tolerance {
		t.Errorf("ByProvider[openai] = %f, want 0.02", delta)
	}
	if delta := result.ByProvider["anthropic"]; abs(delta-0.0) > tolerance {
		t.Errorf("ByProvider[anthropic] = %f, want 0.0", delta)
	}
	if delta := result.ByProvider["google"]; abs(delta-0.01) > tolerance {
		t.Errorf("ByProvider[google] = %f, want 0.01", delta)
	}

	// Check model deltas
	if delta := result.ByModel["gpt-4"]; abs(delta-0.02) > tolerance {
		t.Errorf("ByModel[gpt-4] = %f, want 0.02", delta)
	}
	if delta := result.ByModel["gemini-pro"]; abs(delta-0.01) > tolerance {
		t.Errorf("ByModel[gemini-pro] = %f, want 0.01", delta)
	}
}

func TestCostDiffer_CompareTokens(t *testing.T) {
	d := NewCostDiffer()

	tests := []struct {
		name          string
		baseSpan      *store.Span
		compareSpan   *store.Span
		wantDelta     int
		wantBaseTotal int
	}{
		{
			name:          "both nil",
			baseSpan:      nil,
			compareSpan:   nil,
			wantDelta:     0,
			wantBaseTotal: 0,
		},
		{
			name:     "base nil",
			baseSpan: nil,
			compareSpan: &store.Span{
				LLM: &store.LLMSpanData{
					Tokens: store.TokenDetail{
						InputTokens:  100,
						OutputTokens: 50,
						TotalTokens:  150,
					},
				},
			},
			wantDelta:     150,
			wantBaseTotal: 0,
		},
		{
			name: "compare nil",
			baseSpan: &store.Span{
				LLM: &store.LLMSpanData{
					Tokens: store.TokenDetail{
						InputTokens:  100,
						OutputTokens: 50,
						TotalTokens:  150,
					},
				},
			},
			compareSpan:   nil,
			wantDelta:     -150,
			wantBaseTotal: 150,
		},
		{
			name: "both have tokens",
			baseSpan: &store.Span{
				LLM: &store.LLMSpanData{
					Tokens: store.TokenDetail{
						InputTokens:  100,
						OutputTokens: 50,
						TotalTokens:  150,
					},
				},
			},
			compareSpan: &store.Span{
				LLM: &store.LLMSpanData{
					Tokens: store.TokenDetail{
						InputTokens:  120,
						OutputTokens: 60,
						TotalTokens:  180,
					},
				},
			},
			wantDelta:     30,
			wantBaseTotal: 150,
		},
		{
			name:     "span without LLM data",
			baseSpan: &store.Span{
				// No LLM field
			},
			compareSpan: &store.Span{
				LLM: &store.LLMSpanData{
					Tokens: store.TokenDetail{
						TotalTokens: 100,
					},
				},
			},
			wantDelta:     100,
			wantBaseTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.CompareTokens(tt.baseSpan, tt.compareSpan)

			if result.Delta != tt.wantDelta {
				t.Errorf("Delta = %d, want %d", result.Delta, tt.wantDelta)
			}

			if result.BaseTokens.TotalTokens != tt.wantBaseTotal {
				t.Errorf("BaseTokens.TotalTokens = %d, want %d", result.BaseTokens.TotalTokens, tt.wantBaseTotal)
			}
		})
	}
}

func TestCostDiffer_AggregateTokenDiffs(t *testing.T) {
	d := NewCostDiffer()

	diffs := []*store.TokenDiff{
		{
			BaseTokens: store.TokenDetail{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				CostEstimate: 0.01,
			},
			CompareTokens: store.TokenDetail{
				InputTokens:  120,
				OutputTokens: 60,
				TotalTokens:  180,
				CostEstimate: 0.015,
			},
			Delta: 30,
		},
		{
			BaseTokens: store.TokenDetail{
				InputTokens:  200,
				OutputTokens: 100,
				TotalTokens:  300,
				CostEstimate: 0.02,
			},
			CompareTokens: store.TokenDetail{
				InputTokens:  180,
				OutputTokens: 80,
				TotalTokens:  260,
				CostEstimate: 0.018,
			},
			Delta: -40,
		},
		nil, // Should be skipped
	}

	agg := d.AggregateTokenDiffs(diffs)

	if agg.TotalBaseTokens != 450 {
		t.Errorf("TotalBaseTokens = %d, want 450", agg.TotalBaseTokens)
	}

	if agg.TotalCompareTokens != 440 {
		t.Errorf("TotalCompareTokens = %d, want 440", agg.TotalCompareTokens)
	}

	if agg.TotalDelta != -10 {
		t.Errorf("TotalDelta = %d, want -10", agg.TotalDelta)
	}

	if agg.BaseInputTokens != 300 {
		t.Errorf("BaseInputTokens = %d, want 300", agg.BaseInputTokens)
	}

	if agg.CompareInputTokens != 300 {
		t.Errorf("CompareInputTokens = %d, want 300", agg.CompareInputTokens)
	}

	if agg.TotalBaseCost != 0.03 {
		t.Errorf("TotalBaseCost = %f, want 0.03", agg.TotalBaseCost)
	}

	if agg.TotalCompareCost != 0.033 {
		t.Errorf("TotalCompareCost = %f, want 0.033", agg.TotalCompareCost)
	}
}

func TestTokenAggregates_PercentChange(t *testing.T) {
	tests := []struct {
		name string
		agg  TokenAggregates
		want float64
	}{
		{
			name: "both zero",
			agg: TokenAggregates{
				TotalBaseTokens:    0,
				TotalCompareTokens: 0,
				TotalDelta:         0,
			},
			want: 0,
		},
		{
			name: "base zero compare non-zero",
			agg: TokenAggregates{
				TotalBaseTokens:    0,
				TotalCompareTokens: 100,
				TotalDelta:         100,
			},
			want: 100.0,
		},
		{
			name: "50% increase",
			agg: TokenAggregates{
				TotalBaseTokens:    100,
				TotalCompareTokens: 150,
				TotalDelta:         50,
			},
			want: 50.0,
		},
		{
			name: "25% decrease",
			agg: TokenAggregates{
				TotalBaseTokens:    100,
				TotalCompareTokens: 75,
				TotalDelta:         -25,
			},
			want: -25.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agg.PercentChange()
			if got != tt.want {
				t.Errorf("PercentChange() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestTokenAggregates_CostPercentChange(t *testing.T) {
	tests := []struct {
		name string
		agg  TokenAggregates
		want float64
	}{
		{
			name: "both zero",
			agg: TokenAggregates{
				TotalBaseCost:    0,
				TotalCompareCost: 0,
				CostDelta:        0,
			},
			want: 0,
		},
		{
			name: "base zero compare non-zero",
			agg: TokenAggregates{
				TotalBaseCost:    0,
				TotalCompareCost: 0.05,
				CostDelta:        0.05,
			},
			want: 100.0,
		},
		{
			name: "100% increase",
			agg: TokenAggregates{
				TotalBaseCost:    0.01,
				TotalCompareCost: 0.02,
				CostDelta:        0.01,
			},
			want: 100.0,
		},
		{
			name: "50% decrease",
			agg: TokenAggregates{
				TotalBaseCost:    0.10,
				TotalCompareCost: 0.05,
				CostDelta:        -0.05,
			},
			want: -50.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.agg.CostPercentChange()
			if got != tt.want {
				t.Errorf("CostPercentChange() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestNewCostDiffer(t *testing.T) {
	d := NewCostDiffer()
	if d == nil {
		t.Error("NewCostDiffer() returned nil")
	}
}

// abs returns the absolute value of x
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
