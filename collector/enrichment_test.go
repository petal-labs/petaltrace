package collector

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/store"
)

func TestEnricher_LLMCostCalculation(t *testing.T) {
	pt, err := pricing.LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin error: %v", err)
	}

	enricher := NewEnricher(EnricherConfig{
		PricingTable: pt,
	})

	cacheRead := 200
	run := &store.Run{
		ID: "run-1",
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency:   "USD",
			ByProvider: make(map[string]float64),
			ByModel:    make(map[string]float64),
			ByNode:     make(map[string]float64),
		},
	}

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-1",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-20250514",
				Tokens: store.TokenDetail{
					InputTokens:  1000,
					OutputTokens: 500,
					TotalTokens:  1500,
				},
				CacheRead: &cacheRead,
			},
		},
		Run: run,
	}

	ctx := context.Background()
	enricher.EnrichSpan(ctx, cs)

	// Check cost was calculated
	// Claude Sonnet: $3/1M input, $15/1M output, $0.30/1M cache read
	expectedCost := (1000*3.00 + 500*15.00 + 200*0.30) / 1_000_000
	if math.Abs(cs.Span.LLM.Tokens.CostEstimate-expectedCost) > 0.0001 {
		t.Errorf("CostEstimate = %f, want %f", cs.Span.LLM.Tokens.CostEstimate, expectedCost)
	}

	// Check run aggregates
	if run.TotalTokens.InputTokens != 1000 {
		t.Errorf("TotalTokens.InputTokens = %d, want 1000", run.TotalTokens.InputTokens)
	}
	if run.TotalTokens.OutputTokens != 500 {
		t.Errorf("TotalTokens.OutputTokens = %d, want 500", run.TotalTokens.OutputTokens)
	}
	if run.TotalTokens.CacheReadTokens != 200 {
		t.Errorf("TotalTokens.CacheReadTokens = %d, want 200", run.TotalTokens.CacheReadTokens)
	}

	if math.Abs(run.EstimatedCost.Total-expectedCost) > 0.0001 {
		t.Errorf("EstimatedCost.Total = %f, want %f", run.EstimatedCost.Total, expectedCost)
	}

	if run.EstimatedCost.ByProvider["anthropic"] == 0 {
		t.Error("ByProvider[anthropic] not set")
	}
	if run.EstimatedCost.ByModel["anthropic/claude-sonnet-4-20250514"] == 0 {
		t.Error("ByModel not set")
	}
}

func TestEnricher_OpenAICost(t *testing.T) {
	pt, err := pricing.LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin error: %v", err)
	}

	enricher := NewEnricher(EnricherConfig{
		PricingTable: pt,
	})

	run := &store.Run{
		ID: "run-2",
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency:   "USD",
			ByProvider: make(map[string]float64),
			ByModel:    make(map[string]float64),
		},
	}

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-2",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Provider: "OpenAI", // Test case normalization
				Model:    "gpt-4o",
				Tokens: store.TokenDetail{
					InputTokens:  10000,
					OutputTokens: 1000,
					TotalTokens:  11000,
				},
			},
		},
		Run: run,
	}

	ctx := context.Background()
	enricher.EnrichSpan(ctx, cs)

	// GPT-4o: $2.50/1M input, $10/1M output
	expectedCost := (10000*2.50 + 1000*10.00) / 1_000_000
	if math.Abs(cs.Span.LLM.Tokens.CostEstimate-expectedCost) > 0.0001 {
		t.Errorf("CostEstimate = %f, want %f", cs.Span.LLM.Tokens.CostEstimate, expectedCost)
	}

	if run.EstimatedCost.ByProvider["openai"] == 0 {
		t.Error("ByProvider[openai] not set")
	}
}

func TestEnricher_MultipleLLMSpans(t *testing.T) {
	pt, err := pricing.LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin error: %v", err)
	}

	enricher := NewEnricher(EnricherConfig{
		PricingTable: pt,
	})

	run := &store.Run{
		ID: "run-3",
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency:   "USD",
			ByProvider: make(map[string]float64),
			ByModel:    make(map[string]float64),
		},
	}

	spans := []*CorrelatedSpan{
		{
			Span: &store.Span{
				ID:        "span-1",
				Kind:      store.SpanKindLLM,
				StartedAt: time.Now(),
				LLM: &store.LLMSpanData{
					Provider: "anthropic",
					Model:    "claude-sonnet-4-20250514",
					Tokens: store.TokenDetail{
						InputTokens:  1000,
						OutputTokens: 500,
						TotalTokens:  1500,
					},
				},
			},
			Run: run,
		},
		{
			Span: &store.Span{
				ID:        "span-2",
				Kind:      store.SpanKindLLM,
				StartedAt: time.Now(),
				LLM: &store.LLMSpanData{
					Provider: "anthropic",
					Model:    "claude-sonnet-4-20250514",
					Tokens: store.TokenDetail{
						InputTokens:  2000,
						OutputTokens: 1000,
						TotalTokens:  3000,
					},
				},
			},
			Run: run,
		},
	}

	ctx := context.Background()
	enricher.EnrichSpans(ctx, spans)

	// Check aggregated totals
	if run.TotalTokens.InputTokens != 3000 {
		t.Errorf("TotalTokens.InputTokens = %d, want 3000", run.TotalTokens.InputTokens)
	}
	if run.TotalTokens.OutputTokens != 1500 {
		t.Errorf("TotalTokens.OutputTokens = %d, want 1500", run.TotalTokens.OutputTokens)
	}
	if run.TotalTokens.TotalTokens != 4500 {
		t.Errorf("TotalTokens.TotalTokens = %d, want 4500", run.TotalTokens.TotalTokens)
	}

	// Both spans should have costs
	expectedCost1 := (1000*3.00 + 500*15.00) / 1_000_000
	expectedCost2 := (2000*3.00 + 1000*15.00) / 1_000_000
	expectedTotal := expectedCost1 + expectedCost2

	if math.Abs(run.EstimatedCost.Total-expectedTotal) > 0.0001 {
		t.Errorf("EstimatedCost.Total = %f, want %f", run.EstimatedCost.Total, expectedTotal)
	}
}

func TestEnricher_NonLLMSpanIgnored(t *testing.T) {
	pt, err := pricing.LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin error: %v", err)
	}

	enricher := NewEnricher(EnricherConfig{
		PricingTable: pt,
	})

	run := &store.Run{
		ID: "run-4",
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency: "USD",
		},
	}

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-1",
			Kind:      store.SpanKindNode, // Not LLM
			StartedAt: time.Now(),
		},
		Run: run,
	}

	ctx := context.Background()
	enricher.EnrichSpan(ctx, cs)

	// Run aggregates should be unchanged
	if run.TotalTokens.TotalTokens != 0 {
		t.Errorf("TotalTokens.TotalTokens = %d, want 0", run.TotalTokens.TotalTokens)
	}
	if run.EstimatedCost.Total != 0 {
		t.Errorf("EstimatedCost.Total = %f, want 0", run.EstimatedCost.Total)
	}
}

func TestEnricher_ByNodeTracking(t *testing.T) {
	pt, err := pricing.LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin error: %v", err)
	}

	enricher := NewEnricher(EnricherConfig{
		PricingTable: pt,
	})

	run := &store.Run{
		ID: "run-5",
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency:   "USD",
			ByProvider: make(map[string]float64),
			ByModel:    make(map[string]float64),
			ByNode:     make(map[string]float64),
		},
	}

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-1",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			Attributes: map[string]any{
				"petalflow.node.id": "node-abc",
			},
			LLM: &store.LLMSpanData{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-20250514",
				Tokens: store.TokenDetail{
					InputTokens:  1000,
					OutputTokens: 500,
					TotalTokens:  1500,
				},
			},
		},
		Run: run,
	}

	ctx := context.Background()
	enricher.EnrichSpan(ctx, cs)

	if run.EstimatedCost.ByNode["node-abc"] == 0 {
		t.Error("ByNode[node-abc] not set")
	}
}

func TestEnricher_UnknownModel(t *testing.T) {
	pt, err := pricing.LoadBuiltin()
	if err != nil {
		t.Fatalf("LoadBuiltin error: %v", err)
	}

	enricher := NewEnricher(EnricherConfig{
		PricingTable: pt,
	})

	run := &store.Run{
		ID: "run-6",
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency:   "USD",
			ByProvider: make(map[string]float64),
			ByModel:    make(map[string]float64),
		},
	}

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-1",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Provider: "unknown-provider",
				Model:    "unknown-model",
				Tokens: store.TokenDetail{
					InputTokens:  1000,
					OutputTokens: 500,
					TotalTokens:  1500,
				},
			},
		},
		Run: run,
	}

	ctx := context.Background()
	enricher.EnrichSpan(ctx, cs)

	// Cost should be 0 for unknown model
	if cs.Span.LLM.Tokens.CostEstimate != 0 {
		t.Errorf("CostEstimate = %f, want 0 for unknown model", cs.Span.LLM.Tokens.CostEstimate)
	}

	// But tokens should still be aggregated
	if run.TotalTokens.TotalTokens != 1500 {
		t.Errorf("TotalTokens.TotalTokens = %d, want 1500", run.TotalTokens.TotalTokens)
	}
}

func TestNormalizeProvider(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"OpenAI", "openai"},
		{"OPENAI", "openai"},
		{"open_ai", "openai"},
		{"Anthropic", "anthropic"},
		{"ANTHROPIC", "anthropic"},
		{"claude", "anthropic"},
		{"Google", "google"},
		{"google-ai", "google"},
		{"vertex", "google"},
		{"Mistral", "mistral"},
		{"mistral-ai", "mistral"},
		{"DeepSeek", "deepseek"},
		{"deep_seek", "deepseek"},
		{"custom-provider", "custom-provider"},
		{"  openai  ", "openai"}, // Whitespace trimming
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeProvider(tt.input)
			if got != tt.want {
				t.Errorf("normalizeProvider(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
