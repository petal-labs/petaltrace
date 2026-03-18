package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// promptGetTool returns the petaltrace.prompt.get tool definition
func (s *Server) promptGetTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"run_id": {
				"type": "string",
				"description": "The run ID containing the LLM node"
			},
			"node_id": {
				"type": "string",
				"description": "The node ID of the LLM node to retrieve prompt for"
			},
			"include_completion": {
				"type": "boolean",
				"description": "Include the LLM completion in the response",
				"default": true
			}
		},
		"required": ["run_id", "node_id"]
	}`)

	return &Tool{
		Name:        "petaltrace.prompt.get",
		Description: "Get the full prompt and completion for an LLM node in a run. Useful for understanding exactly what the model saw and how it responded.",
		InputSchema: schema,
		Handler:     s.handlePromptGet,
	}
}

// PromptGetArgs contains arguments for prompt.get
type PromptGetArgs struct {
	RunID             string `json:"run_id"`
	NodeID            string `json:"node_id"`
	IncludeCompletion bool   `json:"include_completion"`
}

// PromptGetResult contains the result of prompt.get
type PromptGetResult struct {
	RunID        string          `json:"run_id"`
	NodeID       string          `json:"node_id"`
	SpanID       string          `json:"span_id"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	SystemPrompt string          `json:"system_prompt"`
	Messages     json.RawMessage `json:"messages"`
	Completion   *CompletionInfo `json:"completion,omitempty"`
	Tokens       TokenInfo       `json:"tokens"`
	Latency      LatencyInfo     `json:"latency"`
	StopReason   string          `json:"stop_reason"`
}

// CompletionInfo contains LLM completion details
type CompletionInfo struct {
	Content     json.RawMessage `json:"content"`
	TextContent string          `json:"text_content"`
}

// TokenInfo contains token usage
type TokenInfo struct {
	Input       int     `json:"input"`
	Output      int     `json:"output"`
	Total       int     `json:"total"`
	CacheRead   int     `json:"cache_read,omitempty"`
	CacheCreate int     `json:"cache_create,omitempty"`
	Cost        float64 `json:"cost_usd"`
}

// LatencyInfo contains latency details
type LatencyInfo struct {
	TotalMs       int64 `json:"total_ms"`
	TimeToFirstMs int64 `json:"time_to_first_ms,omitempty"`
}

// handlePromptGet handles the petaltrace.prompt.get tool
func (s *Server) handlePromptGet(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params PromptGetArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if params.NodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}

	// Get all LLM spans for the run
	spans, err := s.store.GetSpansByKind(ctx, params.RunID, store.SpanKindLLM)
	if err != nil {
		return nil, fmt.Errorf("getting LLM spans: %w", err)
	}

	// Find the span for the specified node
	var targetSpan *store.Span
	for _, span := range spans {
		if span.Node != nil && span.Node.NodeID == params.NodeID {
			targetSpan = span
			break
		}
		// Also check span name as fallback
		if span.Name == params.NodeID {
			targetSpan = span
			break
		}
	}

	if targetSpan == nil {
		return nil, fmt.Errorf("LLM node not found: %s", params.NodeID)
	}

	if targetSpan.LLM == nil {
		return nil, fmt.Errorf("span has no LLM data")
	}

	llm := targetSpan.LLM

	// Build messages JSON
	messagesJSON, err := json.Marshal(llm.Messages)
	if err != nil {
		messagesJSON = []byte("[]")
	}

	result := PromptGetResult{
		RunID:        params.RunID,
		NodeID:       params.NodeID,
		SpanID:       targetSpan.ID,
		Provider:     llm.Provider,
		Model:        llm.Model,
		SystemPrompt: llm.SystemPrompt,
		Messages:     messagesJSON,
		Tokens: TokenInfo{
			Input:  llm.Tokens.InputTokens,
			Output: llm.Tokens.OutputTokens,
			Total:  llm.Tokens.TotalTokens,
			Cost:   llm.Tokens.CostEstimate,
		},
		Latency: LatencyInfo{
			TotalMs: llm.TotalLatency,
		},
		StopReason: llm.StopReason,
	}

	// Add cache tokens if present
	if llm.CacheRead != nil {
		result.Tokens.CacheRead = *llm.CacheRead
	}
	if llm.CacheCreation != nil {
		result.Tokens.CacheCreate = *llm.CacheCreation
	}

	// Add TTFT if present
	if llm.TimeToFirstToken != nil {
		result.Latency.TimeToFirstMs = *llm.TimeToFirstToken
	}

	// Add completion if requested
	if params.IncludeCompletion {
		result.Completion = &CompletionInfo{
			Content:     llm.Completion.Content,
			TextContent: llm.Completion.TextContent,
		}
	}

	return json.Marshal(result)
}

// costSummaryTool returns the petaltrace.cost.summary tool definition
func (s *Server) costSummaryTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"workflow": {
				"type": "string",
				"description": "Filter by workflow name"
			},
			"since": {
				"type": "string",
				"description": "Time window (e.g., '24h', '7d', '30d')"
			},
			"group_by": {
				"type": "string",
				"enum": ["workflow", "provider", "model"],
				"description": "Group costs by this dimension",
				"default": "workflow"
			}
		}
	}`)

	return &Tool{
		Name:        "petaltrace.cost.summary",
		Description: "Get aggregate cost metrics for workflow runs. Useful for understanding overall token usage and costs over time.",
		InputSchema: schema,
		Handler:     s.handleCostSummary,
	}
}

// CostSummaryArgs contains arguments for cost.summary
type CostSummaryArgs struct {
	Workflow string `json:"workflow,omitempty"`
	Since    string `json:"since,omitempty"`
	GroupBy  string `json:"group_by,omitempty"`
}

// CostSummaryResult contains cost summary data
type CostSummaryResult struct {
	TotalRuns    int             `json:"total_runs"`
	TotalTokens  int             `json:"total_tokens"`
	TotalCostUSD float64         `json:"total_cost_usd"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	AverageCost  float64         `json:"average_cost_per_run"`
	Breakdown    []CostBreakdown `json:"breakdown"`
	TimeRange    TimeRange       `json:"time_range"`
}

// CostBreakdown contains cost grouped by dimension
type CostBreakdown struct {
	Name         string  `json:"name"`
	RunCount     int     `json:"run_count"`
	TotalTokens  int     `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// TimeRange describes the time range of the data
type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// handleCostSummary handles the petaltrace.cost.summary tool
func (s *Server) handleCostSummary(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params CostSummaryArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	// Set default group by
	if params.GroupBy == "" {
		params.GroupBy = "workflow"
	}

	// Build query options
	opts := store.ListRunsOptions{
		Limit: 1000, // Get a reasonable number for aggregation
	}

	if params.Workflow != "" {
		opts.WorkflowName = params.Workflow
	}
	if params.Since != "" {
		duration, err := time.ParseDuration(params.Since)
		if err == nil {
			since := time.Now().Add(-duration)
			opts.Since = &since
		}
	}

	// Query runs
	runs, _, err := s.store.ListRuns(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}

	// Aggregate data
	var totalTokens, inputTokens, outputTokens int
	var totalCost float64
	breakdownMap := make(map[string]*CostBreakdown)

	var minTime, maxTime time.Time
	for i, run := range runs {
		totalTokens += run.TotalTokens.TotalTokens
		inputTokens += run.TotalTokens.InputTokens
		outputTokens += run.TotalTokens.OutputTokens
		totalCost += run.EstimatedCost.Total

		// Update time range
		if i == 0 || run.StartedAt.Before(minTime) {
			minTime = run.StartedAt
		}
		if i == 0 || run.StartedAt.After(maxTime) {
			maxTime = run.StartedAt
		}

		// Group by dimension
		var key string
		switch params.GroupBy {
		case "provider":
			// For provider grouping, we need to aggregate from cost breakdown
			for provider, cost := range run.EstimatedCost.ByProvider {
				if _, ok := breakdownMap[provider]; !ok {
					breakdownMap[provider] = &CostBreakdown{Name: provider}
				}
				breakdownMap[provider].TotalCostUSD += cost
				breakdownMap[provider].RunCount++
			}
			continue
		case "model":
			for model, cost := range run.EstimatedCost.ByModel {
				if _, ok := breakdownMap[model]; !ok {
					breakdownMap[model] = &CostBreakdown{Name: model}
				}
				breakdownMap[model].TotalCostUSD += cost
				breakdownMap[model].RunCount++
			}
			continue
		default: // workflow
			key = run.WorkflowName
		}

		if _, ok := breakdownMap[key]; !ok {
			breakdownMap[key] = &CostBreakdown{Name: key}
		}
		breakdownMap[key].RunCount++
		breakdownMap[key].TotalTokens += run.TotalTokens.TotalTokens
		breakdownMap[key].TotalCostUSD += run.EstimatedCost.Total
	}

	// Convert breakdown map to slice
	breakdowns := make([]CostBreakdown, 0, len(breakdownMap))
	for _, b := range breakdownMap {
		breakdowns = append(breakdowns, *b)
	}

	// Calculate average cost
	var avgCost float64
	if len(runs) > 0 {
		avgCost = totalCost / float64(len(runs))
	}

	result := CostSummaryResult{
		TotalRuns:    len(runs),
		TotalTokens:  totalTokens,
		TotalCostUSD: totalCost,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		AverageCost:  avgCost,
		Breakdown:    breakdowns,
		TimeRange: TimeRange{
			From: minTime.Format(time.RFC3339),
			To:   maxTime.Format(time.RFC3339),
		},
	}

	return json.Marshal(result)
}

// costRunTool returns the petaltrace.cost.run tool definition
func (s *Server) costRunTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"run_id": {
				"type": "string",
				"description": "The run ID to get cost breakdown for"
			}
		},
		"required": ["run_id"]
	}`)

	return &Tool{
		Name:        "petaltrace.cost.run",
		Description: "Get detailed cost breakdown for a specific workflow run, including per-node token usage and costs.",
		InputSchema: schema,
		Handler:     s.handleCostRun,
	}
}

// CostRunArgs contains arguments for cost.run
type CostRunArgs struct {
	RunID string `json:"run_id"`
}

// CostRunResult contains per-run cost breakdown
type CostRunResult struct {
	RunID        string             `json:"run_id"`
	WorkflowName string             `json:"workflow_name"`
	Status       string             `json:"status"`
	TotalTokens  int                `json:"total_tokens"`
	TotalCostUSD float64            `json:"total_cost_usd"`
	InputTokens  int                `json:"input_tokens"`
	OutputTokens int                `json:"output_tokens"`
	ByNode       []NodeCost         `json:"by_node"`
	ByProvider   map[string]float64 `json:"by_provider"`
	ByModel      map[string]float64 `json:"by_model"`
}

// NodeCost contains cost info for a single node
type NodeCost struct {
	NodeID       string  `json:"node_id"`
	NodeName     string  `json:"node_name"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// handleCostRun handles the petaltrace.cost.run tool
func (s *Server) handleCostRun(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params CostRunArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if params.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	// Get run
	run, err := s.store.GetRun(ctx, params.RunID)
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("run not found: %s", params.RunID)
	}

	// Get LLM spans
	spans, err := s.store.GetSpansByKind(ctx, params.RunID, store.SpanKindLLM)
	if err != nil {
		return nil, fmt.Errorf("getting LLM spans: %w", err)
	}

	// Build per-node cost breakdown
	nodeCosts := make([]NodeCost, 0, len(spans))
	for _, span := range spans {
		if span.LLM == nil {
			continue
		}

		nodeID := span.Name
		if span.Node != nil && span.Node.NodeID != "" {
			nodeID = span.Node.NodeID
		}

		nodeCosts = append(nodeCosts, NodeCost{
			NodeID:       nodeID,
			NodeName:     span.Name,
			Provider:     span.LLM.Provider,
			Model:        span.LLM.Model,
			InputTokens:  span.LLM.Tokens.InputTokens,
			OutputTokens: span.LLM.Tokens.OutputTokens,
			TotalTokens:  span.LLM.Tokens.TotalTokens,
			CostUSD:      span.LLM.Tokens.CostEstimate,
		})
	}

	result := CostRunResult{
		RunID:        run.ID,
		WorkflowName: run.WorkflowName,
		Status:       string(run.Status),
		TotalTokens:  run.TotalTokens.TotalTokens,
		TotalCostUSD: run.EstimatedCost.Total,
		InputTokens:  run.TotalTokens.InputTokens,
		OutputTokens: run.TotalTokens.OutputTokens,
		ByNode:       nodeCosts,
		ByProvider:   run.EstimatedCost.ByProvider,
		ByModel:      run.EstimatedCost.ByModel,
	}

	return json.Marshal(result)
}
