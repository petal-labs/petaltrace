package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// traceListTool returns the petaltrace.trace.list tool definition
func (s *Server) traceListTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"workflow": {
				"type": "string",
				"description": "Filter by workflow name"
			},
			"status": {
				"type": "string",
				"enum": ["running", "completed", "failed", "cancelled"],
				"description": "Filter by run status"
			},
			"since": {
				"type": "string",
				"description": "Time window (e.g., '24h', '7d')"
			},
			"limit": {
				"type": "integer",
				"description": "Maximum number of runs to return",
				"default": 50
			},
			"cursor": {
				"type": "string",
				"description": "Pagination cursor"
			}
		}
	}`)

	return &Tool{
		Name:        "petaltrace.trace.list",
		Description: "List recent workflow runs with optional filters. Returns run IDs, status, duration, tokens, and cost estimates.",
		InputSchema: schema,
		Handler:     s.handleTraceList,
	}
}

// TraceListArgs contains arguments for trace.list
type TraceListArgs struct {
	Workflow string `json:"workflow,omitempty"`
	Status   string `json:"status,omitempty"`
	Since    string `json:"since,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// TraceListResult contains the result of trace.list
type TraceListResult struct {
	Runs   []RunSummary `json:"runs"`
	Cursor string       `json:"cursor,omitempty"`
	Count  int          `json:"count"`
}

// RunSummary contains summary information about a run
type RunSummary struct {
	ID            string   `json:"id"`
	WorkflowName  string   `json:"workflow_name"`
	Status        string   `json:"status"`
	DurationMs    int64    `json:"duration_ms"`
	TotalTokens   int      `json:"total_tokens"`
	EstimatedCost float64  `json:"estimated_cost"`
	StartedAt     string   `json:"started_at"`
	NodeCount     int      `json:"node_count"`
	ErrorCount    int      `json:"error_count"`
}

// handleTraceList handles the petaltrace.trace.list tool
func (s *Server) handleTraceList(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params TraceListArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}

	// Set defaults
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// Build query options
	opts := store.ListRunsOptions{
		Limit:  limit,
		Cursor: params.Cursor,
	}

	if params.Workflow != "" {
		opts.WorkflowName = params.Workflow
	}
	if params.Status != "" {
		opts.Status = store.RunStatus(params.Status)
	}
	if params.Since != "" {
		duration, err := time.ParseDuration(params.Since)
		if err == nil {
			since := time.Now().Add(-duration)
			opts.Since = &since
		}
	}

	// Query runs
	runs, cursor, err := s.store.ListRuns(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}

	// Build result
	summaries := make([]RunSummary, len(runs))
	for i, run := range runs {
		summaries[i] = RunSummary{
			ID:            run.ID,
			WorkflowName:  run.WorkflowName,
			Status:        string(run.Status),
			DurationMs:    run.DurationMs,
			TotalTokens:   run.TotalTokens.TotalTokens,
			EstimatedCost: run.EstimatedCost.Total,
			StartedAt:     run.StartedAt.Format(time.RFC3339),
			NodeCount:     run.NodeCount,
			ErrorCount:    run.ErrorCount,
		}
	}

	result := TraceListResult{
		Runs:   summaries,
		Cursor: cursor,
		Count:  len(summaries),
	}

	return json.Marshal(result)
}

// traceGetTool returns the petaltrace.trace.get tool definition
func (s *Server) traceGetTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"run_id": {
				"type": "string",
				"description": "The run ID to retrieve"
			},
			"include_spans": {
				"type": "boolean",
				"description": "Include the span tree in the response",
				"default": true
			}
		},
		"required": ["run_id"]
	}`)

	return &Tool{
		Name:        "petaltrace.trace.get",
		Description: "Get detailed information about a workflow run, including its span tree showing node execution, LLM calls, and tool invocations.",
		InputSchema: schema,
		Handler:     s.handleTraceGet,
	}
}

// TraceGetArgs contains arguments for trace.get
type TraceGetArgs struct {
	RunID        string `json:"run_id"`
	IncludeSpans bool   `json:"include_spans"`
}

// TraceGetResult contains the result of trace.get
type TraceGetResult struct {
	Run   RunDetail    `json:"run"`
	Spans []SpanDetail `json:"spans,omitempty"`
}

// RunDetail contains detailed run information
type RunDetail struct {
	ID              string         `json:"id"`
	WorkflowID      string         `json:"workflow_id"`
	WorkflowName    string         `json:"workflow_name"`
	WorkflowVersion string         `json:"workflow_version"`
	Status          string         `json:"status"`
	StartedAt       string         `json:"started_at"`
	CompletedAt     string         `json:"completed_at,omitempty"`
	DurationMs      int64          `json:"duration_ms"`
	TotalTokens     TokenSummary   `json:"total_tokens"`
	EstimatedCost   CostSummary    `json:"estimated_cost"`
	NodeCount       int            `json:"node_count"`
	ErrorCount      int            `json:"error_count"`
	Tags            map[string]string `json:"tags,omitempty"`
	TriggerSource   string         `json:"trigger_source"`
	ParentRunID     string         `json:"parent_run_id,omitempty"`
}

// TokenSummary contains token counts
type TokenSummary struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CostSummary contains cost information
type CostSummary struct {
	Currency string  `json:"currency"`
	Total    float64 `json:"total"`
}

// SpanDetail contains span information
type SpanDetail struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	DurationMs int64  `json:"duration_ms"`

	// LLM-specific fields
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`

	// Tool-specific fields
	ToolName string `json:"tool_name,omitempty"`

	// Node-specific fields
	NodeID   string `json:"node_id,omitempty"`
	NodeType string `json:"node_type,omitempty"`

	// Error information
	Error string `json:"error,omitempty"`
}

// handleTraceGet handles the petaltrace.trace.get tool
func (s *Server) handleTraceGet(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params TraceGetArgs
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

	// Build run detail
	runDetail := RunDetail{
		ID:              run.ID,
		WorkflowID:      run.WorkflowID,
		WorkflowName:    run.WorkflowName,
		WorkflowVersion: run.WorkflowVersion,
		Status:          string(run.Status),
		StartedAt:       run.StartedAt.Format(time.RFC3339),
		DurationMs:      run.DurationMs,
		TotalTokens: TokenSummary{
			InputTokens:      run.TotalTokens.InputTokens,
			OutputTokens:     run.TotalTokens.OutputTokens,
			CacheReadTokens:  run.TotalTokens.CacheReadTokens,
			CacheWriteTokens: run.TotalTokens.CacheWriteTokens,
			TotalTokens:      run.TotalTokens.TotalTokens,
		},
		EstimatedCost: CostSummary{
			Currency: run.EstimatedCost.Currency,
			Total:    run.EstimatedCost.Total,
		},
		NodeCount:     run.NodeCount,
		ErrorCount:    run.ErrorCount,
		Tags:          run.Tags,
		TriggerSource: run.TriggerSource,
	}
	if run.CompletedAt != nil {
		runDetail.CompletedAt = run.CompletedAt.Format(time.RFC3339)
	}
	if run.ParentRunID != nil {
		runDetail.ParentRunID = *run.ParentRunID
	}

	result := TraceGetResult{
		Run: runDetail,
	}

	// Get spans if requested
	if params.IncludeSpans {
		spans, err := s.store.GetSpanTree(ctx, params.RunID)
		if err != nil {
			return nil, fmt.Errorf("getting spans: %w", err)
		}

		spanDetails := make([]SpanDetail, len(spans))
		for i, span := range spans {
			detail := SpanDetail{
				ID:         span.ID,
				Kind:       string(span.Kind),
				Name:       span.Name,
				Status:     string(span.Status),
				StartedAt:  span.StartedAt.Format(time.RFC3339),
				DurationMs: span.DurationMs,
			}
			if span.ParentID != nil {
				detail.ParentID = *span.ParentID
			}

			// Add LLM-specific fields
			if span.LLM != nil {
				detail.Provider = span.LLM.Provider
				detail.Model = span.LLM.Model
				detail.InputTokens = span.LLM.Tokens.InputTokens
				detail.OutputTokens = span.LLM.Tokens.OutputTokens
			}

			// Add tool-specific fields
			if span.Tool != nil {
				detail.ToolName = span.Tool.ToolName
			}

			// Add node-specific fields
			if span.Node != nil {
				detail.NodeID = span.Node.NodeID
				detail.NodeType = span.Node.NodeType
			}

			// Add error
			if span.Error != nil {
				detail.Error = span.Error.Message
			}

			spanDetails[i] = detail
		}

		result.Spans = spanDetails
	}

	return json.Marshal(result)
}

// traceSearchTool returns the petaltrace.trace.search tool definition
func (s *Server) traceSearchTool() *Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search query text to find in prompts or completions"
			},
			"workflow": {
				"type": "string",
				"description": "Filter by workflow name"
			},
			"limit": {
				"type": "integer",
				"description": "Maximum number of results",
				"default": 20
			}
		},
		"required": ["query"]
	}`)

	return &Tool{
		Name:        "petaltrace.trace.search",
		Description: "Search workflow runs by content in prompts or LLM completions. Useful for finding runs that processed specific topics or produced certain outputs.",
		InputSchema: schema,
		Handler:     s.handleTraceSearch,
	}
}

// TraceSearchArgs contains arguments for trace.search
type TraceSearchArgs struct {
	Query    string `json:"query"`
	Workflow string `json:"workflow,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// TraceSearchResult contains search results
type TraceSearchResult struct {
	Runs  []SearchHit `json:"runs"`
	Count int         `json:"count"`
}

// SearchHit represents a search result
type SearchHit struct {
	RunID        string `json:"run_id"`
	WorkflowName string `json:"workflow_name"`
	Status       string `json:"status"`
	SpanID       string `json:"span_id"`
	SpanName     string `json:"span_name"`
	Snippet      string `json:"snippet"`
	StartedAt    string `json:"started_at"`
}

// handleTraceSearch handles the petaltrace.trace.search tool
func (s *Server) handleTraceSearch(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var params TraceSearchArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	if params.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Search spans using FTS
	spans, err := s.store.SearchSpans(ctx, params.Query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching spans: %w", err)
	}

	// Build result by fetching run info for each span
	results := make([]SearchHit, 0, len(spans))
	runCache := make(map[string]*store.Run)

	for _, span := range spans {
		// Get run info (with caching)
		run, ok := runCache[span.RunID]
		if !ok {
			run, err = s.store.GetRun(ctx, span.RunID)
			if err != nil {
				continue
			}
			if run == nil {
				continue
			}
			runCache[span.RunID] = run
		}

		// Apply workflow filter if specified
		if params.Workflow != "" && run.WorkflowName != params.Workflow {
			continue
		}

		// Extract snippet from LLM data
		snippet := ""
		if span.LLM != nil {
			snippet = truncate(span.LLM.Completion.TextContent, 200)
		}

		results = append(results, SearchHit{
			RunID:        span.RunID,
			WorkflowName: run.WorkflowName,
			Status:       string(run.Status),
			SpanID:       span.ID,
			SpanName:     span.Name,
			Snippet:      snippet,
			StartedAt:    span.StartedAt.Format(time.RFC3339),
		})
	}

	result := TraceSearchResult{
		Runs:  results,
		Count: len(results),
	}

	return json.Marshal(result)
}

// truncate truncates a string to the given length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
