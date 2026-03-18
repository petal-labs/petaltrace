package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

// handleListRuns handles GET /api/runs
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	opts := store.ListRunsOptions{}

	// Parse filters
	if v := query.Get("workflow"); v != "" {
		opts.WorkflowName = v
	}
	if v := query.Get("workflow_id"); v != "" {
		opts.WorkflowID = v
	}
	if v := query.Get("status"); v != "" {
		opts.Status = store.RunStatus(v)
	}
	if v := query.Get("since"); v != "" {
		since, err := parseTimeOrDuration(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'since' parameter: "+err.Error())
			return
		}
		opts.Since = &since
	}
	if v := query.Get("until"); v != "" {
		until, err := parseTimeOrDuration(v)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'until' parameter: "+err.Error())
			return
		}
		opts.Until = &until
	}
	if v := query.Get("min_cost"); v != "" {
		minCost, err := strconv.ParseFloat(v, 64)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid 'min_cost' parameter")
			return
		}
		opts.MinCost = &minCost
	}
	if v := query.Get("starred"); v != "" {
		starred := v == "true"
		opts.Starred = &starred
	}
	if v := query.Get("cursor"); v != "" {
		opts.Cursor = v
	}
	if v := query.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 {
			s.writeError(w, http.StatusBadRequest, "invalid 'limit' parameter")
			return
		}
		opts.Limit = limit
	} else {
		opts.Limit = 50
	}
	if v := query.Get("sort_by"); v != "" {
		opts.SortBy = v
	}
	if v := query.Get("sort_order"); v != "" {
		opts.SortOrder = v
	}

	runs, cursor, err := s.store.ListRuns(ctx, opts)
	if err != nil {
		s.logger.Error("failed to list runs", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}

	s.writeJSON(w, http.StatusOK, PaginatedResponse{
		Data:    runs,
		Cursor:  cursor,
		HasMore: cursor != "",
	})
}

// handleGetRun handles GET /api/runs/{id}
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
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

	// Include spans if requested
	includeSpans := r.URL.Query().Get("include_spans") == "true"
	if includeSpans {
		spans, err := s.store.GetSpanTree(ctx, runID)
		if err != nil {
			s.logger.Error("failed to get spans", "error", err, "run_id", runID)
			s.writeError(w, http.StatusInternalServerError, "failed to get spans")
			return
		}

		s.writeJSON(w, http.StatusOK, map[string]any{
			"run":   run,
			"spans": spans,
		})
		return
	}

	s.writeJSON(w, http.StatusOK, run)
}

// handleDeleteRun handles DELETE /api/runs/{id}
func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := r.PathValue("id")

	if runID == "" {
		s.writeError(w, http.StatusBadRequest, "run ID is required")
		return
	}

	// Check if run exists
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

	if err := s.store.DeleteRun(ctx, runID); err != nil {
		s.logger.Error("failed to delete run", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to delete run")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"run_id": runID,
	})
}

// handleGetSpans handles GET /api/runs/{id}/spans
func (s *Server) handleGetSpans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := r.PathValue("id")

	if runID == "" {
		s.writeError(w, http.StatusBadRequest, "run ID is required")
		return
	}

	// Check if run exists
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

	// Filter by kind if specified
	kind := r.URL.Query().Get("kind")
	var spans []*store.Span
	if kind != "" {
		spans, err = s.store.GetSpansByKind(ctx, runID, store.SpanKind(kind))
	} else {
		spans, err = s.store.GetSpanTree(ctx, runID)
	}

	if err != nil {
		s.logger.Error("failed to get spans", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to get spans")
		return
	}

	// Filter by node if specified
	nodeID := r.URL.Query().Get("node")
	if nodeID != "" {
		var filtered []*store.Span
		for _, span := range spans {
			if span.Node != nil && span.Node.NodeID == nodeID {
				filtered = append(filtered, span)
			}
		}
		spans = filtered
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"spans": spans})
}

// handleGetSpan handles GET /api/runs/{id}/spans/{spanId}
func (s *Server) handleGetSpan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := r.PathValue("id")
	spanID := r.PathValue("spanId")

	if runID == "" || spanID == "" {
		s.writeError(w, http.StatusBadRequest, "run ID and span ID are required")
		return
	}

	span, err := s.store.GetSpan(ctx, spanID)
	if err != nil {
		s.logger.Error("failed to get span", "error", err, "span_id", spanID)
		s.writeError(w, http.StatusInternalServerError, "failed to get span")
		return
	}
	if span == nil {
		s.writeError(w, http.StatusNotFound, "span not found")
		return
	}

	// Verify span belongs to the specified run
	if span.RunID != runID {
		s.writeError(w, http.StatusNotFound, "span not found in run")
		return
	}

	s.writeJSON(w, http.StatusOK, span)
}

// handleGetGraph handles GET /api/runs/{id}/graph
func (s *Server) handleGetGraph(w http.ResponseWriter, r *http.Request) {
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

	// Build graph overlay with execution data
	spans, err := s.store.GetSpanTree(ctx, runID)
	if err != nil {
		s.logger.Error("failed to get spans", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to get spans")
		return
	}

	// Create node status overlay
	nodeStatuses := make(map[string]NodeStatus)
	for _, span := range spans {
		if span.Node != nil {
			nodeStatuses[span.Node.NodeID] = NodeStatus{
				NodeID:     span.Node.NodeID,
				Status:     string(span.Status),
				DurationMs: span.DurationMs,
				SpanID:     span.ID,
			}

			// Add token/cost info for LLM nodes
			if span.LLM != nil {
				nodeStatuses[span.Node.NodeID] = NodeStatus{
					NodeID:       span.Node.NodeID,
					Status:       string(span.Status),
					DurationMs:   span.DurationMs,
					SpanID:       span.ID,
					TokenCount:   span.LLM.Tokens.TotalTokens,
					CostEstimate: span.LLM.Tokens.CostEstimate,
				}
			}
		}
	}

	response := GraphResponse{
		RunID:         runID,
		GraphSnapshot: run.GraphSnapshot,
		NodeStatuses:  nodeStatuses,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// NodeStatus represents the execution status of a node
type NodeStatus struct {
	NodeID       string  `json:"node_id"`
	Status       string  `json:"status"`
	DurationMs   int64   `json:"duration_ms"`
	SpanID       string  `json:"span_id"`
	TokenCount   int     `json:"token_count,omitempty"`
	CostEstimate float64 `json:"cost_estimate,omitempty"`
}

// GraphResponse is the response for GET /api/runs/{id}/graph
type GraphResponse struct {
	RunID         string                `json:"run_id"`
	GraphSnapshot json.RawMessage       `json:"graph_snapshot"`
	NodeStatuses  map[string]NodeStatus `json:"node_statuses"`
}

// Helper functions

func parseTimeOrDuration(s string) (time.Time, error) {
	// Try parsing as RFC3339 first
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}

	// Try parsing as duration
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var n int
		if _, err := strconv.Atoi(days); err == nil {
			n, _ = strconv.Atoi(days)
			return time.Now().Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, err
	}

	return time.Now().Add(-d), nil
}
