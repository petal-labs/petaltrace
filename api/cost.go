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

	// Calculate summary
	summary := CostSummaryResponse{
		Since:      since,
		Until:      until,
		TotalRuns:  len(runs),
		ByWorkflow: make(map[string]CostGroup),
		ByProvider: make(map[string]CostGroup),
		ByModel:    make(map[string]CostGroup),
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
		g := summary.ByWorkflow[wf]
		g.Runs++
		g.Tokens += int64(run.TotalTokens.TotalTokens)
		g.Cost += run.EstimatedCost.Total
		summary.ByWorkflow[wf] = g

		// By provider
		for provider, cost := range run.EstimatedCost.ByProvider {
			p := summary.ByProvider[provider]
			p.Runs++
			p.Cost += cost
			summary.ByProvider[provider] = p
		}

		// By model
		for model, cost := range run.EstimatedCost.ByModel {
			m := summary.ByModel[model]
			m.Runs++
			m.Cost += cost
			summary.ByModel[model] = m
		}
	}

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
		DataPoints: dataPoints,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// CostSummaryResponse is the response for GET /api/cost/summary
type CostSummaryResponse struct {
	Since            time.Time            `json:"since"`
	Until            time.Time            `json:"until,omitempty"`
	TotalRuns        int                  `json:"total_runs"`
	TotalTokens      int64                `json:"total_tokens"`
	TotalCost        float64              `json:"total_cost"`
	InputTokens      int64                `json:"input_tokens"`
	OutputTokens     int64                `json:"output_tokens"`
	CacheReadTokens  int64                `json:"cache_read_tokens"`
	CacheWriteTokens int64                `json:"cache_write_tokens"`
	ByWorkflow       map[string]CostGroup `json:"by_workflow,omitempty"`
	ByProvider       map[string]CostGroup `json:"by_provider,omitempty"`
	ByModel          map[string]CostGroup `json:"by_model,omitempty"`
}

// CostGroup represents aggregated cost data
type CostGroup struct {
	Runs   int     `json:"runs"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
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
	DataPoints []TimeBucket `json:"data_points"`
}

// TimeBucket represents a time bucket in timeseries data
type TimeBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Runs      int       `json:"runs"`
	Tokens    int64     `json:"tokens"`
	Cost      float64   `json:"cost"`
}
