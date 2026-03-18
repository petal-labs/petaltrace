package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// handleCostSummary handles GET /api/cost/summary
func (s *Server) handleCostSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// Parse time window
	var since time.Time
	if v := query.Get("since"); v != "" {
		var err error
		since, err = parseTimeOrDuration(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'since' parameter: "+err.Error())
			return
		}
	} else {
		// Default to 7 days
		since = time.Now().Add(-7 * 24 * time.Hour)
	}

	var until time.Time
	if v := query.Get("until"); v != "" {
		var err error
		until, err = parseTimeOrDuration(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'until' parameter: "+err.Error())
			return
		}
	}

	groupBy := query.Get("group_by")

	// Get runs in time window
	opts := store.ListRunsOptions{
		Since: &since,
		Limit: 10000,
	}
	if !until.IsZero() {
		opts.Until = &until
	}

	runs, _, err := s.store.ListRuns(ctx, opts)
	if err != nil {
		s.logger.Error("failed to list runs", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	// Calculate summary using internal maps
	byWorkflowMap := make(map[string]*costGroupMap)
	byProviderMap := make(map[string]*costGroupMap)
	byModelMap := make(map[string]*costGroupMap)

	summary := CostSummaryResponse{
		Since:     since,
		Until:     until,
		TotalRuns: len(runs),
	}

	for _, run := range runs {
		summary.TotalTokens += int64(run.TotalTokens.TotalTokens)
		summary.TotalCost += run.EstimatedCost.Total
		summary.InputTokens += int64(run.TotalTokens.InputTokens)
		summary.OutputTokens += int64(run.TotalTokens.OutputTokens)
		summary.CacheReadTokens += int64(run.TotalTokens.CacheReadTokens)
		summary.CacheWriteTokens += int64(run.TotalTokens.CacheWriteTokens)

		// By workflow
		wf := run.WorkflowName
		if wf == "" {
			wf = "(unknown)"
		}
		if byWorkflowMap[wf] == nil {
			byWorkflowMap[wf] = &costGroupMap{}
		}
		byWorkflowMap[wf].Runs++
		byWorkflowMap[wf].Tokens += int64(run.TotalTokens.TotalTokens)
		byWorkflowMap[wf].Cost += run.EstimatedCost.Total

		// Get LLM spans for provider/model breakdown
		spans, err := s.store.GetSpansByKind(ctx, run.ID, store.SpanKindLLM)
		if err != nil {
			s.logger.Warn("failed to get LLM spans for run", "run_id", run.ID, "error", err)
			continue
		}

		for _, span := range spans {
			if span.LLM == nil {
				continue
			}

			provider := span.LLM.Provider
			if provider == "" {
				provider = "(unknown)"
			}
			if byProviderMap[provider] == nil {
				byProviderMap[provider] = &costGroupMap{}
			}
			byProviderMap[provider].Runs++
			byProviderMap[provider].Tokens += int64(span.LLM.Tokens.TotalTokens)
			byProviderMap[provider].Cost += span.LLM.Tokens.CostEstimate

			model := span.LLM.Model
			if model == "" {
				model = "(unknown)"
			}
			if byModelMap[model] == nil {
				byModelMap[model] = &costGroupMap{}
			}
			byModelMap[model].Runs++
			byModelMap[model].Tokens += int64(span.LLM.Tokens.TotalTokens)
			byModelMap[model].Cost += span.LLM.Tokens.CostEstimate
		}
	}

	// Convert maps to sorted arrays
	for name, g := range byWorkflowMap {
		summary.ByWorkflow = append(summary.ByWorkflow, CostBreakdown{
			Name:        name,
			RunCount:    g.Runs,
			TotalTokens: g.Tokens,
			TotalCost:   g.Cost,
		})
	}
	sort.Slice(summary.ByWorkflow, func(i, j int) bool {
		return summary.ByWorkflow[i].TotalCost > summary.ByWorkflow[j].TotalCost
	})

	for name, g := range byProviderMap {
		summary.ByProvider = append(summary.ByProvider, CostBreakdown{
			Name:        name,
			RunCount:    g.Runs,
			TotalTokens: g.Tokens,
			TotalCost:   g.Cost,
		})
	}
	sort.Slice(summary.ByProvider, func(i, j int) bool {
		return summary.ByProvider[i].TotalCost > summary.ByProvider[j].TotalCost
	})

	for name, g := range byModelMap {
		summary.ByModel = append(summary.ByModel, CostBreakdown{
			Name:        name,
			RunCount:    g.Runs,
			TotalTokens: g.Tokens,
			TotalCost:   g.Cost,
		})
	}
	sort.Slice(summary.ByModel, func(i, j int) bool {
		return summary.ByModel[i].TotalCost > summary.ByModel[j].TotalCost
	})

	// Filter by group_by if specified
	if groupBy != "" {
		switch groupBy {
		case "workflow":
			summary.ByProvider = nil
			summary.ByModel = nil
		case "provider":
			summary.ByWorkflow = nil
			summary.ByModel = nil
		case "model":
			summary.ByWorkflow = nil
			summary.ByProvider = nil
		}
	}

	s.writeJSON(w, http.StatusOK, summary)
}

// handleCostRun handles GET /api/cost/runs/{id}
func (s *Server) handleCostRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := r.PathValue("id")

	if runID == "" {
		s.writeError(w, http.StatusBadRequest, "run ID is required")
		return
	}

	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		s.logger.Error("failed to get run", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run == nil {
		s.writeError(w, http.StatusNotFound, "run not found")
		return
	}

	// Get LLM spans for detailed breakdown
	spans, err := s.store.GetSpansByKind(ctx, runID, store.SpanKindLLM)
	if err != nil {
		s.logger.Error("failed to get spans", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to get spans")
		return
	}

	// Calculate per-node costs
	var nodeCosts []NodeCostBreakdown
	for _, span := range spans {
		if span.LLM == nil {
			continue
		}

		nodeID := span.Name
		if span.Node != nil && span.Node.NodeID != "" {
			nodeID = span.Node.NodeID
		}

		nodeCosts = append(nodeCosts, NodeCostBreakdown{
			NodeID:       nodeID,
			SpanID:       span.ID,
			Model:        span.LLM.Model,
			Provider:     span.LLM.Provider,
			InputTokens:  span.LLM.Tokens.InputTokens,
			OutputTokens: span.LLM.Tokens.OutputTokens,
			TotalTokens:  span.LLM.Tokens.TotalTokens,
			Cost:         span.LLM.Tokens.CostEstimate,
			DurationMs:   span.DurationMs,
		})
	}

	// Sort by cost descending
	sort.Slice(nodeCosts, func(i, j int) bool {
		return nodeCosts[i].Cost > nodeCosts[j].Cost
	})

	response := CostRunResponse{
		RunID:         run.ID,
		WorkflowName:  run.WorkflowName,
		TotalTokens:   run.TotalTokens,
		EstimatedCost: run.EstimatedCost,
		LLMCalls:      len(spans),
		ByNode:        nodeCosts,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleCostTimeseries handles GET /api/cost/timeseries
func (s *Server) handleCostTimeseries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// Parse time window
	var since time.Time
	if v := query.Get("since"); v != "" {
		var err error
		since, err = parseTimeOrDuration(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'since' parameter: "+err.Error())
			return
		}
	} else {
		since = time.Now().Add(-7 * 24 * time.Hour)
	}

	var until time.Time
	if v := query.Get("until"); v != "" {
		var err error
		until, err = parseTimeOrDuration(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'until' parameter: "+err.Error())
			return
		}
	} else {
		until = time.Now()
	}

	// Parse bucket size
	bucketSize := query.Get("bucket")
	if bucketSize == "" {
		bucketSize = "1h"
	}
	bucketDuration, err := time.ParseDuration(bucketSize)
	if err != nil {
		// Try days
		if len(bucketSize) > 1 && bucketSize[len(bucketSize)-1] == 'd' {
			days, err := strconv.Atoi(bucketSize[:len(bucketSize)-1])
			if err == nil {
				bucketDuration = time.Duration(days) * 24 * time.Hour
			}
		}
		if bucketDuration == 0 {
			s.writeError(w, http.StatusBadRequest, "invalid 'bucket' parameter")
			return
		}
	}

	// Get runs in time window
	opts := store.ListRunsOptions{
		Since: &since,
		Until: &until,
		Limit: 10000,
	}

	runs, _, err := s.store.ListRuns(ctx, opts)
	if err != nil {
		s.logger.Error("failed to list runs", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	// Create time buckets
	buckets := make(map[int64]*TimeBucket)
	for t := since.Truncate(bucketDuration); t.Before(until); t = t.Add(bucketDuration) {
		buckets[t.Unix()] = &TimeBucket{
			Timestamp: t,
		}
	}

	// Aggregate runs into buckets
	for _, run := range runs {
		bucketTime := run.StartedAt.Truncate(bucketDuration).Unix()
		if bucket, ok := buckets[bucketTime]; ok {
			bucket.Runs++
			bucket.Tokens += int64(run.TotalTokens.TotalTokens)
			bucket.Cost += run.EstimatedCost.Total
		}
	}

	// Convert to sorted slice
	var dataPoints []TimeBucket
	for _, bucket := range buckets {
		dataPoints = append(dataPoints, *bucket)
	}
	sort.Slice(dataPoints, func(i, j int) bool {
		return dataPoints[i].Timestamp.Before(dataPoints[j].Timestamp)
	})

	response := CostTimeseriesResponse{
		Since:      since,
		Until:      until,
		BucketSize: bucketSize,
		Data:       dataPoints,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// CostSummaryResponse is the response for GET /api/cost/summary
type CostSummaryResponse struct {
	Since            time.Time       `json:"since"`
	Until            time.Time       `json:"until,omitempty"`
	TotalRuns        int             `json:"total_runs"`
	TotalTokens      int64           `json:"total_tokens"`
	TotalCost        float64         `json:"total_cost"`
	InputTokens      int64           `json:"input_tokens"`
	OutputTokens     int64           `json:"output_tokens"`
	CacheReadTokens  int64           `json:"cache_read_tokens"`
	CacheWriteTokens int64           `json:"cache_write_tokens"`
	ByWorkflow       []CostBreakdown `json:"by_workflow,omitempty"`
	ByProvider       []CostBreakdown `json:"by_provider,omitempty"`
	ByModel          []CostBreakdown `json:"by_model,omitempty"`
}

// CostBreakdown represents aggregated cost data with a name
type CostBreakdown struct {
	Name        string  `json:"name"`
	RunCount    int     `json:"run_count"`
	TotalTokens int64   `json:"total_tokens"`
	TotalCost   float64 `json:"total_cost"`
}

// costGroupMap is used internally for aggregation
type costGroupMap struct {
	Runs   int
	Tokens int64
	Cost   float64
}

// CostRunResponse is the response for GET /api/cost/runs/{id}
type CostRunResponse struct {
	RunID         string              `json:"run_id"`
	WorkflowName  string              `json:"workflow_name"`
	TotalTokens   store.TokenSummary  `json:"total_tokens"`
	EstimatedCost store.CostEstimate  `json:"estimated_cost"`
	LLMCalls      int                 `json:"llm_calls"`
	ByNode        []NodeCostBreakdown `json:"by_node"`
}

// NodeCostBreakdown represents cost data for a single node
type NodeCostBreakdown struct {
	NodeID       string  `json:"node_id"`
	SpanID       string  `json:"span_id"`
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost"`
	DurationMs   int64   `json:"duration_ms"`
}

// CostTimeseriesResponse is the response for GET /api/cost/timeseries
type CostTimeseriesResponse struct {
	Since      time.Time    `json:"since"`
	Until      time.Time    `json:"until"`
	BucketSize string       `json:"bucket_size"`
	Data       []TimeBucket `json:"data"`
}

// TimeBucket represents a time bucket in timeseries data
type TimeBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Runs      int       `json:"runs"`
	Tokens    int64     `json:"tokens"`
	Cost      float64   `json:"cost"`
}
