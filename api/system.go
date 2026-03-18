package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/store"
)

// handleHealth handles GET /api/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   Version,
	}

	// Check store connectivity
	ctx := r.Context()
	_, err := s.store.GetStats(ctx)
	if err != nil {
		health.Status = "degraded"
		health.Details = map[string]string{
			"store": "unavailable: " + err.Error(),
		}
	} else {
		health.Details = map[string]string{
			"store": "connected",
		}
	}

	status := http.StatusOK
	if health.Status != "healthy" {
		status = http.StatusServiceUnavailable
	}

	s.writeJSON(w, status, health)
}

// handleStats handles GET /api/stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := s.store.GetStats(ctx)
	if err != nil {
		s.logger.Error("failed to get stats", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to get storage stats")
		return
	}

	response := StatsResponse{
		Database: DatabaseStats{
			SizeBytes: stats.DatabaseSize,
			RunCount:  stats.RunCount,
			SpanCount: stats.SpanCount,
			DiffCount: stats.DiffCount,
		},
		TopWorkflows: stats.TopWorkflows,
		Runtime: RuntimeStats{
			GoVersion:    runtime.Version(),
			NumGoroutine: runtime.NumGoroutine(),
			NumCPU:       runtime.NumCPU(),
		},
	}

	if !stats.OldestRun.IsZero() {
		response.Database.OldestRun = &stats.OldestRun
	}
	if !stats.NewestRun.IsZero() {
		response.Database.NewestRun = &stats.NewestRun
	}

	// Calculate memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	response.Runtime.AllocBytes = memStats.Alloc
	response.Runtime.TotalAllocBytes = memStats.TotalAlloc
	response.Runtime.SysBytes = memStats.Sys

	s.writeJSON(w, http.StatusOK, response)
}

// handleGetPricing handles GET /api/pricing
func (s *Server) handleGetPricing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	// If provider and model specified, get specific entry
	provider := query.Get("provider")
	model := query.Get("model")

	if provider != "" && model != "" {
		entry, err := s.store.GetPricing(ctx, provider, model)
		if err != nil {
			s.logger.Error("failed to get pricing", "error", err, "provider", provider, "model", model)
			s.writeError(w, http.StatusInternalServerError, "failed to get pricing")
			return
		}
		if entry == nil {
			s.writeError(w, http.StatusNotFound, "pricing not found for provider/model")
			return
		}
		s.writeJSON(w, http.StatusOK, entry)
		return
	}

	// Otherwise list all pricing entries
	entries, err := s.store.ListPricing(ctx)
	if err != nil {
		s.logger.Error("failed to list pricing", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to list pricing")
		return
	}

	// Group by provider for easier consumption
	byProvider := make(map[string][]store.PricingEntry)
	for _, entry := range entries {
		byProvider[entry.Provider] = append(byProvider[entry.Provider], entry)
	}

	response := PricingResponse{
		Entries:    entries,
		ByProvider: byProvider,
		UpdatedAt:  time.Now(),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleUpdatePricing handles PUT /api/pricing
func (s *Server) handleUpdatePricing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req PricingUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	// Validate required fields
	if req.Provider == "" || req.Model == "" {
		s.writeError(w, http.StatusBadRequest, "provider and model are required")
		return
	}

	// Create pricing entry
	entry := &store.PricingEntry{
		Provider:        req.Provider,
		Model:           req.Model,
		InputPer1M:      req.InputPer1M,
		OutputPer1M:     req.OutputPer1M,
		CacheReadPer1M:  req.CacheReadPer1M,
		CacheWritePer1M: req.CacheWritePer1M,
		EffectiveFrom:   time.Now(),
	}

	if err := s.store.UpsertPricing(ctx, entry); err != nil {
		s.logger.Error("failed to update pricing", "error", err, "provider", req.Provider, "model", req.Model)
		s.writeError(w, http.StatusInternalServerError, "failed to update pricing")
		return
	}

	// Also update the in-memory pricing table if available
	if s.pricingTable != nil {
		s.pricingTable.Set(pricing.ModelPricing{
			Provider:        req.Provider,
			Model:           req.Model,
			InputPer1M:      req.InputPer1M,
			OutputPer1M:     req.OutputPer1M,
			CacheReadPer1M:  req.CacheReadPer1M,
			CacheWritePer1M: req.CacheWritePer1M,
		})
	}

	s.logger.Info("pricing updated", "provider", req.Provider, "model", req.Model)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "updated",
		"entry":   entry,
	})
}

// Version is the API version (set at build time or default)
var Version = "0.1.0-dev"

// HealthResponse is the response for GET /api/health
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Version   string            `json:"version"`
	Details   map[string]string `json:"details,omitempty"`
}

// StatsResponse is the response for GET /api/stats
type StatsResponse struct {
	Database     DatabaseStats         `json:"database"`
	TopWorkflows []store.WorkflowStats `json:"top_workflows"`
	Runtime      RuntimeStats          `json:"runtime"`
}

// DatabaseStats contains database statistics
type DatabaseStats struct {
	SizeBytes int64      `json:"size_bytes"`
	RunCount  int64      `json:"run_count"`
	SpanCount int64      `json:"span_count"`
	DiffCount int64      `json:"diff_count"`
	OldestRun *time.Time `json:"oldest_run,omitempty"`
	NewestRun *time.Time `json:"newest_run,omitempty"`
}

// RuntimeStats contains Go runtime statistics
type RuntimeStats struct {
	GoVersion       string `json:"go_version"`
	NumGoroutine    int    `json:"num_goroutine"`
	NumCPU          int    `json:"num_cpu"`
	AllocBytes      uint64 `json:"alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
}

// PricingResponse is the response for GET /api/pricing
type PricingResponse struct {
	Entries    []store.PricingEntry            `json:"entries"`
	ByProvider map[string][]store.PricingEntry `json:"by_provider"`
	UpdatedAt  time.Time                       `json:"updated_at"`
}

// PricingUpdateRequest is the request body for PUT /api/pricing
type PricingUpdateRequest struct {
	Provider        string  `json:"provider"`
	Model           string  `json:"model"`
	InputPer1M      float64 `json:"input_per_1m"`
	OutputPer1M     float64 `json:"output_per_1m"`
	CacheReadPer1M  float64 `json:"cache_read_per_1m,omitempty"`
	CacheWritePer1M float64 `json:"cache_write_per_1m,omitempty"`
}
