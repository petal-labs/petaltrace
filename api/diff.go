package api

import (
	"encoding/json"
	"net/http"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/store"
)

// handleComputeDiff handles POST /api/diff
func (s *Server) handleComputeDiff(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req DiffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.BaseRunID == "" || req.CompareRunID == "" {
		s.writeError(w, http.StatusBadRequest, "base_run_id and compare_run_id are required")
		return
	}

	engine := diff.NewEngine(s.store)

	opts := diff.DiffOptions{
		IncludeContent:    req.IncludeContent,
		IncludeSimilarity: true,
		IncludeInputs:     req.IncludeInputs,
		CacheResult:       !req.NoCache,
	}

	runDiff, err := engine.ComputeDiff(ctx, req.BaseRunID, req.CompareRunID, opts)
	if err != nil {
		s.logger.Error("failed to compute diff", "error", err,
			"base_run_id", req.BaseRunID, "compare_run_id", req.CompareRunID)
		s.writeError(w, http.StatusInternalServerError, "failed to compute diff: "+err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, runDiff)
}

// handleGetDiff handles GET /api/diff/{id}
func (s *Server) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	diffID := r.PathValue("id")

	if diffID == "" {
		s.writeError(w, http.StatusBadRequest, "diff ID is required")
		return
	}

	runDiff, err := s.store.GetDiff(ctx, diffID)
	if err != nil {
		s.logger.Error("failed to get diff", "error", err, "diff_id", diffID)
		s.writeError(w, http.StatusInternalServerError, "failed to get diff")
		return
	}

	if runDiff == nil {
		s.writeError(w, http.StatusNotFound, "diff not found")
		return
	}

	s.writeJSON(w, http.StatusOK, runDiff)
}

// handleGetDiffByRuns handles GET /api/diff/runs
func (s *Server) handleGetDiffByRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	baseRunID := query.Get("base_run_id")
	compareRunID := query.Get("compare_run_id")

	if baseRunID == "" || compareRunID == "" {
		s.writeError(w, http.StatusBadRequest, "base_run_id and compare_run_id query params are required")
		return
	}

	runDiff, err := s.store.GetDiffByRuns(ctx, baseRunID, compareRunID)
	if err != nil {
		s.logger.Error("failed to get diff by runs", "error", err,
			"base_run_id", baseRunID, "compare_run_id", compareRunID)
		s.writeError(w, http.StatusInternalServerError, "failed to get diff")
		return
	}

	if runDiff == nil {
		s.writeError(w, http.StatusNotFound, "diff not found for these runs")
		return
	}

	s.writeJSON(w, http.StatusOK, runDiff)
}

// handleListDiffs handles GET /api/diffs
func (s *Server) handleListDiffs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// Filter by run ID if specified
	runID := query.Get("run_id")

	// Get all diffs - we'll need to add a ListDiffs method to the store
	// For now, return an error indicating this isn't implemented
	if runID != "" {
		// Try to find diffs involving this run
		// This would require a store method we don't have yet
		s.writeError(w, http.StatusNotImplemented, "listing diffs by run not yet implemented")
		return
	}

	// Get stats to show diff count
	stats, err := s.store.GetStats(ctx)
	if err != nil {
		s.logger.Error("failed to get stats", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"total_diffs": stats.DiffCount,
		"message":     "use POST /api/diff to compute a new diff, or GET /api/diff/{id} to retrieve a cached one",
	})
}

// DiffRequest is the request body for POST /api/diff
type DiffRequest struct {
	BaseRunID      string `json:"base_run_id"`
	CompareRunID   string `json:"compare_run_id"`
	IncludeContent bool   `json:"include_content,omitempty"`
	IncludeInputs  bool   `json:"include_inputs,omitempty"`
	NoCache        bool   `json:"no_cache,omitempty"`
}

// DiffSummaryResponse is a lightweight diff summary for listing
type DiffSummaryResponse struct {
	ID           string            `json:"id"`
	BaseRunID    string            `json:"base_run_id"`
	CompareRunID string            `json:"compare_run_id"`
	Summary      store.DiffSummary `json:"summary"`
	CreatedAt    string            `json:"created_at"`
}
