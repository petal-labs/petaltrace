package collector

import (
	"context"
	"log/slog"
	"strings"

	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/store"
)

// Enricher adds cost estimates and other derived data to spans.
type Enricher struct {
	pricingTable *pricing.PricingTable
	logger       *slog.Logger
}

// EnricherConfig configures the enricher.
type EnricherConfig struct {
	PricingTable *pricing.PricingTable
	Logger       *slog.Logger
}

// NewEnricher creates a new span enricher.
func NewEnricher(cfg EnricherConfig) *Enricher {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Enricher{
		pricingTable: cfg.PricingTable,
		logger:       cfg.Logger,
	}
}

// EnrichSpan adds cost estimates and other derived data to a correlated span.
func (e *Enricher) EnrichSpan(ctx context.Context, cs *CorrelatedSpan) {
	if cs.Span.Kind != store.SpanKindLLM {
		return
	}

	if cs.Span.LLM == nil {
		return
	}

	e.enrichLLMCost(cs)
	e.updateRunAggregates(cs)
}

// EnrichSpans processes multiple spans.
func (e *Enricher) EnrichSpans(ctx context.Context, spans []*CorrelatedSpan) {
	for _, cs := range spans {
		e.EnrichSpan(ctx, cs)
	}
}

func (e *Enricher) enrichLLMCost(cs *CorrelatedSpan) {
	llm := cs.Span.LLM

	if e.pricingTable == nil {
		return
	}

	provider := normalizeProvider(llm.Provider)
	model := llm.Model

	// Calculate cache tokens if present
	cacheRead := 0
	cacheWrite := 0
	if llm.CacheRead != nil {
		cacheRead = *llm.CacheRead
	}
	if llm.CacheCreation != nil {
		cacheWrite = *llm.CacheCreation
	}

	// Compute cost
	cost := e.pricingTable.ComputeCost(
		provider,
		model,
		llm.Tokens.InputTokens,
		llm.Tokens.OutputTokens,
		cacheRead,
		cacheWrite,
	)

	llm.Tokens.CostEstimate = cost

	e.logger.Debug("enriched LLM span with cost",
		"provider", provider,
		"model", model,
		"input_tokens", llm.Tokens.InputTokens,
		"output_tokens", llm.Tokens.OutputTokens,
		"cache_read", cacheRead,
		"cache_write", cacheWrite,
		"cost", cost,
	)
}

func (e *Enricher) updateRunAggregates(cs *CorrelatedSpan) {
	if cs.Run == nil || cs.Span.LLM == nil {
		return
	}

	llm := cs.Span.LLM
	run := cs.Run

	// Update token totals
	run.TotalTokens.InputTokens += llm.Tokens.InputTokens
	run.TotalTokens.OutputTokens += llm.Tokens.OutputTokens
	run.TotalTokens.TotalTokens += llm.Tokens.TotalTokens

	if llm.CacheRead != nil {
		run.TotalTokens.CacheReadTokens += *llm.CacheRead
	}
	if llm.CacheCreation != nil {
		run.TotalTokens.CacheWriteTokens += *llm.CacheCreation
	}

	// Update cost totals
	run.EstimatedCost.Total += llm.Tokens.CostEstimate

	// Track by provider
	provider := normalizeProvider(llm.Provider)
	if run.EstimatedCost.ByProvider == nil {
		run.EstimatedCost.ByProvider = make(map[string]float64)
	}
	run.EstimatedCost.ByProvider[provider] += llm.Tokens.CostEstimate

	// Track by model
	if run.EstimatedCost.ByModel == nil {
		run.EstimatedCost.ByModel = make(map[string]float64)
	}
	modelKey := provider + "/" + llm.Model
	run.EstimatedCost.ByModel[modelKey] += llm.Tokens.CostEstimate

	// Track by node if we have node context
	if nodeID, ok := getStringAttr(cs.Span.Attributes, "petalflow.node.id"); ok {
		if run.EstimatedCost.ByNode == nil {
			run.EstimatedCost.ByNode = make(map[string]float64)
		}
		run.EstimatedCost.ByNode[nodeID] += llm.Tokens.CostEstimate
	}
}

// normalizeProvider normalizes provider names to match pricing table keys.
func normalizeProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))

	// Map common variations
	switch p {
	case "openai", "open_ai", "open-ai":
		return "openai"
	case "anthropic", "claude":
		return "anthropic"
	case "google", "google-ai", "google_ai", "vertex", "vertex-ai":
		return "google"
	case "mistral", "mistral-ai", "mistral_ai":
		return "mistral"
	case "deepseek", "deep_seek", "deep-seek":
		return "deepseek"
	default:
		return p
	}
}
