package diff

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/petal-labs/petaltrace/store"
)

// Engine orchestrates diff computation between runs
type Engine struct {
	store      store.TraceStore
	structural *StructuralDiffer
	content    *ContentDiffer
	cost       *CostDiffer
}

// NewEngine creates a new diff engine
func NewEngine(s store.TraceStore) *Engine {
	return &Engine{
		store:      s,
		structural: NewStructuralDiffer(),
		content:    NewContentDiffer(),
		cost:       NewCostDiffer(),
	}
}

// DiffOptions configures diff computation
type DiffOptions struct {
	IncludeContent    bool // Include full text diffs
	IncludeSimilarity bool // Compute similarity scores
	IncludeInputs     bool // Include input/output data diffs
	CacheResult       bool // Store result in database
}

// DefaultOptions returns default diff options
func DefaultOptions() DiffOptions {
	return DiffOptions{
		IncludeContent:    true,
		IncludeSimilarity: true,
		IncludeInputs:     false,
		CacheResult:       true,
	}
}

// ComputeDiff computes the difference between two runs
func (e *Engine) ComputeDiff(ctx context.Context, baseRunID, compareRunID string, opts DiffOptions) (*store.RunDiff, error) {
	// Check cache first
	if opts.CacheResult {
		cached, err := e.store.GetDiffByRuns(ctx, baseRunID, compareRunID)
		if err == nil && cached != nil {
			return cached, nil
		}
	}

	// Load runs
	baseRun, err := e.store.GetRun(ctx, baseRunID)
	if err != nil {
		return nil, fmt.Errorf("loading base run: %w", err)
	}
	if baseRun == nil {
		return nil, fmt.Errorf("base run not found: %s", baseRunID)
	}

	compareRun, err := e.store.GetRun(ctx, compareRunID)
	if err != nil {
		return nil, fmt.Errorf("loading compare run: %w", err)
	}
	if compareRun == nil {
		return nil, fmt.Errorf("compare run not found: %s", compareRunID)
	}

	// Load span trees
	baseSpans, err := e.store.GetSpanTree(ctx, baseRunID)
	if err != nil {
		return nil, fmt.Errorf("loading base spans: %w", err)
	}

	compareSpans, err := e.store.GetSpanTree(ctx, compareRunID)
	if err != nil {
		return nil, fmt.Errorf("loading compare spans: %w", err)
	}

	// Compute diff components
	diff := &store.RunDiff{
		ID:           uuid.New().String(),
		BaseRunID:    baseRunID,
		CompareRunID: compareRunID,
		CreatedAt:    time.Now(),
	}

	// Graph/structural diff
	diff.GraphDiff = e.structural.CompareGraphs(baseSpans, compareSpans)

	// Cost diff
	diff.CostDiff = e.cost.CompareRuns(baseRun, compareRun)

	// Node-level diffs
	matches := e.structural.MatchNodes(baseSpans, compareSpans)
	diff.NodeDiffs = e.computeNodeDiffs(matches, opts)

	// Compute summary
	diff.Summary = e.computeSummary(baseRun, compareRun, diff)

	// Cache result if requested
	if opts.CacheResult {
		if err := e.store.CreateDiff(ctx, diff); err != nil {
			// Log but don't fail - caching is optional
			// Just continue with the computed diff
		}
	}

	return diff, nil
}

// computeNodeDiffs computes diffs for each matched node pair
func (e *Engine) computeNodeDiffs(matches []NodeMatch, opts DiffOptions) []store.NodeDiff {
	var diffs []store.NodeDiff

	for _, match := range matches {
		nodeDiff := store.NodeDiff{
			NodeID:   match.NodeID,
			NodeType: match.NodeType,
		}

		switch match.Status {
		case MatchStatusBoth:
			nodeDiff.Status = "modified"

			// Duration comparison
			if match.BaseSpan != nil {
				nodeDiff.DurationBase = match.BaseSpan.DurationMs
			}
			if match.CompareSpan != nil {
				nodeDiff.DurationCompare = match.CompareSpan.DurationMs
			}

			// Token diff for LLM spans
			if isLLMSpan(match.BaseSpan) || isLLMSpan(match.CompareSpan) {
				nodeDiff.TokenDiff = e.cost.CompareTokens(match.BaseSpan, match.CompareSpan)
			}

			// Content diffs if requested
			if opts.IncludeContent && (isLLMSpan(match.BaseSpan) || isLLMSpan(match.CompareSpan)) {
				nodeDiff.PromptDiff = e.content.ComparePrompts(match.BaseSpan, match.CompareSpan)
				nodeDiff.OutputDiff = e.content.CompareOutputs(match.BaseSpan, match.CompareSpan)
			}

			// Input/output data diffs if requested
			if opts.IncludeInputs {
				nodeDiff.InputDiff = compareNodeData(match.BaseSpan, match.CompareSpan, true)
				nodeDiff.ResultDiff = compareNodeData(match.BaseSpan, match.CompareSpan, false)
			}

		case MatchStatusBaseOnly:
			nodeDiff.Status = "removed"
			if match.BaseSpan != nil {
				nodeDiff.DurationBase = match.BaseSpan.DurationMs
			}

		case MatchStatusCompareOnly:
			nodeDiff.Status = "added"
			if match.CompareSpan != nil {
				nodeDiff.DurationCompare = match.CompareSpan.DurationMs
			}
		}

		diffs = append(diffs, nodeDiff)
	}

	return diffs
}

// computeSummary computes the diff summary
func (e *Engine) computeSummary(baseRun, compareRun *store.Run, diff *store.RunDiff) store.DiffSummary {
	summary := store.DiffSummary{
		StatusMatch:    baseRun.Status == compareRun.Status,
		DurationDelta:  compareRun.DurationMs - baseRun.DurationMs,
		TokenDelta:     compareRun.TotalTokens.TotalTokens - baseRun.TotalTokens.TotalTokens,
		CostDelta:      diff.CostDiff.Delta,
		NodeDiffCount:  len(diff.NodeDiffs),
		PathDivergence: e.structural.HasPathDivergence(diff.GraphDiff),
	}

	return summary
}

// GetDiff retrieves a cached diff by ID
func (e *Engine) GetDiff(ctx context.Context, diffID string) (*store.RunDiff, error) {
	return e.store.GetDiff(ctx, diffID)
}

// GetDiffByRuns retrieves a cached diff by run IDs
func (e *Engine) GetDiffByRuns(ctx context.Context, baseRunID, compareRunID string) (*store.RunDiff, error) {
	return e.store.GetDiffByRuns(ctx, baseRunID, compareRunID)
}

// isLLMSpan checks if a span is an LLM span
func isLLMSpan(span *store.Span) bool {
	return span != nil && span.LLM != nil
}

// compareNodeData compares node inputs or outputs
func compareNodeData(baseSpan, compareSpan *store.Span, isInput bool) *store.DataDiff {
	var baseData, compareData []byte

	if baseSpan != nil && baseSpan.Node != nil {
		if isInput {
			baseData = baseSpan.Node.Inputs
		} else {
			baseData = baseSpan.Node.Outputs
		}
	}

	if compareSpan != nil && compareSpan.Node != nil {
		if isInput {
			compareData = compareSpan.Node.Inputs
		} else {
			compareData = compareSpan.Node.Outputs
		}
	}

	return &store.DataDiff{
		BaseData:    baseData,
		CompareData: compareData,
		Changed:     string(baseData) != string(compareData),
	}
}
