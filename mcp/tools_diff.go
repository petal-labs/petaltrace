package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/replay"
)

// diffCompareTool returns the petaltrace.diff.compare tool definition
func (s *Server) diffCompareTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"base_run_id": {
				"type": "string",
				"description": "The base run ID for comparison"
			},
			"compare_run_id": {
				"type": "string",
				"description": "The run ID to compare against the base"
			},
			"include_content": {
				"type": "boolean",
				"description": "Include full text diffs for node outputs",
				"default": false
			},
			"include_similarity": {
				"type": "boolean",
				"description": "Include semantic similarity scores",
				"default": false
			}
		},
		"required": ["base_run_id", "compare_run_id"]
	}`)

	return &Tool{
		Name:        "petaltrace.diff.compare",
		Description: "Compare two workflow runs and identify differences in execution paths, outputs, tokens, and costs. Useful for regression testing and A/B comparison.",
		InputSchema: schema,
		Handler:     s.handleDiffCompare,
	}
}

// DiffCompareArgs contains arguments for diff.compare
type DiffCompareArgs struct {
	BaseRunID         string `json:"base_run_id"`
	CompareRunID      string `json:"compare_run_id"`
	IncludeContent    bool   `json:"include_content"`
	IncludeSimilarity bool   `json:"include_similarity"`
}

// DiffCompareResult contains the comparison result
type DiffCompareResult struct {
	DiffID       string          `json:"diff_id"`
	BaseRunID    string          `json:"base_run_id"`
	CompareRunID string          `json:"compare_run_id"`
	Summary      DiffSummaryInfo `json:"summary"`
	NodeDiffs    []NodeDiffInfo  `json:"node_diffs"`
	CostDiff     CostDiffInfo    `json:"cost_diff"`
}

// DiffSummaryInfo contains summary of differences
type DiffSummaryInfo struct {
	StatusMatch       bool    `json:"status_match"`
	BaseStatus        string  `json:"base_status"`
	CompareStatus     string  `json:"compare_status"`
	DurationDeltaMs   int64   `json:"duration_delta_ms"`
	TokenDelta        int     `json:"token_delta"`
	CostDeltaUSD      float64 `json:"cost_delta_usd"`
	NodesAdded        int     `json:"nodes_added"`
	NodesRemoved      int     `json:"nodes_removed"`
	NodesChanged      int     `json:"nodes_changed"`
	PathDiverged      bool    `json:"path_diverged"`
}

// NodeDiffInfo contains diff info for a single node
type NodeDiffInfo struct {
	NodeID           string   `json:"node_id"`
	Status           string   `json:"status"` // "added", "removed", "changed", "unchanged"
	BaseTokens       int      `json:"base_tokens,omitempty"`
	CompareTokens    int      `json:"compare_tokens,omitempty"`
	TokenDelta       int      `json:"token_delta,omitempty"`
	BaseCostUSD      float64  `json:"base_cost_usd,omitempty"`
	CompareCostUSD   float64  `json:"compare_cost_usd,omitempty"`
	CostDeltaUSD     float64  `json:"cost_delta_usd,omitempty"`
	Similarity       float64  `json:"similarity,omitempty"`
	ContentDiff      string   `json:"content_diff,omitempty"`
}

// CostDiffInfo contains cost comparison info
type CostDiffInfo struct {
	BaseTotalUSD    float64            `json:"base_total_usd"`
	CompareTotalUSD float64            `json:"compare_total_usd"`
	DeltaUSD        float64            `json:"delta_usd"`
	DeltaPercent    float64            `json:"delta_percent"`
	ByProvider      map[string]float64 `json:"by_provider,omitempty"`
	ByModel         map[string]float64 `json:"by_model,omitempty"`
}

// handleDiffCompare handles the petaltrace.diff.compare tool
func (s *Server) handleDiffCompare(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params DiffCompareArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if params.BaseRunID == "" {
		return nil, fmt.Errorf("base_run_id is required")
	}
	if params.CompareRunID == "" {
		return nil, fmt.Errorf("compare_run_id is required")
	}

	if s.diffEngine == nil {
		return nil, fmt.Errorf("diff engine not configured")
	}

	// Build diff options
	opts := diff.DiffOptions{
		IncludeContent:    params.IncludeContent,
		IncludeSimilarity: params.IncludeSimilarity,
	}

	// Compute diff
	runDiff, err := s.diffEngine.ComputeDiff(ctx, params.BaseRunID, params.CompareRunID, opts)
	if err != nil {
		return nil, fmt.Errorf("computing diff: %w", err)
	}

	// Get run statuses
	baseRun, _ := s.store.GetRun(ctx, params.BaseRunID)
	compareRun, _ := s.store.GetRun(ctx, params.CompareRunID)

	baseStatus := ""
	compareStatus := ""
	if baseRun != nil {
		baseStatus = string(baseRun.Status)
	}
	if compareRun != nil {
		compareStatus = string(compareRun.Status)
	}

	// Build summary
	summary := DiffSummaryInfo{
		StatusMatch:     runDiff.Summary.StatusMatch,
		BaseStatus:      baseStatus,
		CompareStatus:   compareStatus,
		DurationDeltaMs: runDiff.Summary.DurationDelta,
		TokenDelta:      runDiff.Summary.TokenDelta,
		CostDeltaUSD:    runDiff.Summary.CostDelta,
		PathDiverged:    runDiff.Summary.PathDivergence,
	}

	// Count node changes
	for _, nd := range runDiff.NodeDiffs {
		switch nd.Status {
		case "added":
			summary.NodesAdded++
		case "removed":
			summary.NodesRemoved++
		case "changed":
			summary.NodesChanged++
		}
	}

	// Build node diffs
	nodeDiffs := make([]NodeDiffInfo, len(runDiff.NodeDiffs))
	for i, nd := range runDiff.NodeDiffs {
		info := NodeDiffInfo{
			NodeID: nd.NodeID,
			Status: nd.Status,
		}

		// Extract token info from TokenDiff if present
		if nd.TokenDiff != nil {
			info.BaseTokens = nd.TokenDiff.BaseTokens.TotalTokens
			info.CompareTokens = nd.TokenDiff.CompareTokens.TotalTokens
			info.TokenDelta = nd.TokenDiff.Delta
			info.BaseCostUSD = nd.TokenDiff.BaseTokens.CostEstimate
			info.CompareCostUSD = nd.TokenDiff.CompareTokens.CostEstimate
			info.CostDeltaUSD = nd.TokenDiff.CompareTokens.CostEstimate - nd.TokenDiff.BaseTokens.CostEstimate
		}

		if params.IncludeSimilarity && nd.OutputDiff != nil {
			info.Similarity = nd.OutputDiff.Similarity
		}

		if params.IncludeContent && nd.OutputDiff != nil {
			// Build unified diff from hunks
			var diffContent string
			for _, h := range nd.OutputDiff.Hunks {
				diffContent += h.Content + "\n"
			}
			info.ContentDiff = diffContent
		}

		nodeDiffs[i] = info
	}

	// Build cost diff
	costDiff := CostDiffInfo{
		BaseTotalUSD:    runDiff.CostDiff.BaseCost,
		CompareTotalUSD: runDiff.CostDiff.CompareCost,
		DeltaUSD:        runDiff.CostDiff.Delta,
		ByProvider:      runDiff.CostDiff.ByProvider,
		ByModel:         runDiff.CostDiff.ByModel,
	}
	if runDiff.CostDiff.BaseCost > 0 {
		costDiff.DeltaPercent = (runDiff.CostDiff.Delta / runDiff.CostDiff.BaseCost) * 100
	}

	result := DiffCompareResult{
		DiffID:       runDiff.ID,
		BaseRunID:    runDiff.BaseRunID,
		CompareRunID: runDiff.CompareRunID,
		Summary:      summary,
		NodeDiffs:    nodeDiffs,
		CostDiff:     costDiff,
	}

	return json.Marshal(result)
}

// runReplayTool returns the petaltrace.run.replay tool definition
func (s *Server) runReplayTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"run_id": {
				"type": "string",
				"description": "The run ID to replay"
			},
			"mode": {
				"type": "string",
				"enum": ["live", "mocked", "hybrid"],
				"description": "Replay mode: live (real calls), mocked (captured responses), hybrid (live LLM, mocked tools)",
				"default": "live"
			},
			"model": {
				"type": "string",
				"description": "Override the LLM model for the replay"
			},
			"temperature": {
				"type": "number",
				"description": "Override the temperature for the replay"
			},
			"auto_diff": {
				"type": "boolean",
				"description": "Automatically diff against the source run after completion",
				"default": false
			},
			"tags": {
				"type": "object",
				"additionalProperties": { "type": "string" },
				"description": "Tags to add to the replay run"
			}
		},
		"required": ["run_id"]
	}`)

	return &Tool{
		Name:        "petaltrace.run.replay",
		Description: "Replay a prior workflow run. Useful for regression testing, A/B testing prompt changes, or reproducing issues.",
		InputSchema: schema,
		Handler:     s.handleRunReplay,
	}
}

// RunReplayArgs contains arguments for run.replay
type RunReplayArgs struct {
	RunID       string            `json:"run_id"`
	Mode        string            `json:"mode,omitempty"`
	Model       string            `json:"model,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	AutoDiff    bool              `json:"auto_diff,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// RunReplayResult contains the replay result
type RunReplayResult struct {
	ReplayID     string `json:"replay_id"`
	SourceRunID  string `json:"source_run_id"`
	NewRunID     string `json:"new_run_id,omitempty"`
	DiffID       string `json:"diff_id,omitempty"`
	Mode         string `json:"mode"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

// handleRunReplay handles the petaltrace.run.replay tool
func (s *Server) handleRunReplay(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params RunReplayArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	if s.replayEngine == nil {
		return nil, fmt.Errorf("replay engine not configured")
	}

	// Build replay request
	mode := replay.ReplayModeLive
	if params.Mode != "" {
		mode = replay.ReplayMode(params.Mode)
	}

	req := &replay.ReplayRequest{
		SourceRunID: params.RunID,
		Mode:        mode,
		Tags:        params.Tags,
		AutoDiff:    params.AutoDiff,
	}

	// Apply overrides
	if params.Model != "" {
		req.Overrides.Model = params.Model
	}
	if params.Temperature != nil {
		req.Overrides.Temperature = params.Temperature
	}

	// Execute replay synchronously
	result, err := s.replayEngine.ExecuteSync(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("executing replay: %w", err)
	}

	response := RunReplayResult{
		ReplayID:    result.ID,
		SourceRunID: result.SourceRunID,
		NewRunID:    result.NewRunID,
		DiffID:      result.DiffID,
		Mode:        string(result.Mode),
		Status:      string(result.Status),
		Error:       result.Error,
	}

	return json.Marshal(response)
}
