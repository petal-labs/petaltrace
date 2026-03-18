package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/petal-labs/petaltrace/replay"
)

// replayEngine is the shared replay engine instance
var (
	replayEngine     *replay.Engine
	replayEngineOnce sync.Once
)

// getReplayEngine returns the replay engine, creating it if necessary
func (s *Server) getReplayEngine() *replay.Engine {
	replayEngineOnce.Do(func() {
		replayEngine = replay.NewEngine(replay.EngineConfig{
			Store:        s.store,
			PetalFlowURL: "http://localhost:8080", // TODO: make configurable
			Logger:       s.logger,
		})
	})
	return replayEngine
}

// handleTriggerReplay handles POST /api/replay
func (s *Server) handleTriggerReplay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ReplayAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if req.SourceRunID == "" {
		s.writeError(w, http.StatusBadRequest, "source_run_id is required")
		return
	}

	// Build replay request
	replayReq := &replay.ReplayRequest{
		SourceRunID: req.SourceRunID,
		Mode:        replay.ReplayMode(req.Mode),
		Tags:        req.Tags,
		AutoDiff:    req.AutoDiff,
	}

	// Set default mode
	if replayReq.Mode == "" {
		replayReq.Mode = replay.ReplayModeLive
	}

	// Apply overrides
	if req.Model != "" {
		replayReq.Overrides.Model = req.Model
	}
	if req.Provider != "" {
		replayReq.Overrides.Provider = req.Provider
	}
	if req.Temperature != nil {
		replayReq.Overrides.Temperature = req.Temperature
	}
	if req.MaxTokens != nil {
		replayReq.Overrides.MaxTokens = req.MaxTokens
	}
	if len(req.GraphOverrides) > 0 {
		replayReq.Overrides.GraphOverrides = req.GraphOverrides
	}
	if len(req.InputOverrides) > 0 {
		replayReq.Overrides.InputOverrides = req.InputOverrides
	}

	// Execute replay
	engine := s.getReplayEngine()

	var result *replay.Replay
	var err error

	if req.Sync {
		result, err = engine.ExecuteSync(ctx, replayReq)
	} else {
		result, err = engine.Execute(ctx, replayReq)
	}

	if err != nil {
		s.logger.Error("replay failed",
			"source_run_id", req.SourceRunID,
			"mode", req.Mode,
			"error", err,
		)
		s.writeError(w, http.StatusInternalServerError, "replay failed: "+err.Error())
		return
	}

	s.writeJSON(w, http.StatusAccepted, ReplayAPIResponse{
		ReplayID:    result.ID,
		SourceRunID: result.SourceRunID,
		NewRunID:    result.NewRunID,
		DiffID:      result.DiffID,
		Mode:        string(result.Mode),
		Status:      string(result.Status),
		Error:       result.Error,
		StartedAt:   result.StartedAt.Unix(),
		CompletedAt: timeToUnixPtr(result.CompletedAt),
	})
}

// handleGetReplayStatus handles GET /api/replay/{id}
func (s *Server) handleGetReplayStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	replayID := r.PathValue("id")

	if replayID == "" {
		s.writeError(w, http.StatusBadRequest, "replay ID is required")
		return
	}

	engine := s.getReplayEngine()

	// Refresh status from store
	result, err := engine.RefreshReplayStatus(ctx, replayID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "replay not found: "+err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, ReplayAPIResponse{
		ReplayID:    result.ID,
		SourceRunID: result.SourceRunID,
		NewRunID:    result.NewRunID,
		DiffID:      result.DiffID,
		Mode:        string(result.Mode),
		Status:      string(result.Status),
		Error:       result.Error,
		StartedAt:   result.StartedAt.Unix(),
		CompletedAt: timeToUnixPtr(result.CompletedAt),
	})
}

// handleListReplays handles GET /api/replays
func (s *Server) handleListReplays(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	sourceRunID := query.Get("source_run_id")

	engine := s.getReplayEngine()

	var replays []*replay.Replay
	if sourceRunID != "" {
		replays = engine.GetReplaysBySourceRun(sourceRunID)
	} else {
		replays = engine.ListReplays()
	}

	// Convert to API response format
	responses := make([]ReplayAPIResponse, len(replays))
	for i, r := range replays {
		responses[i] = ReplayAPIResponse{
			ReplayID:    r.ID,
			SourceRunID: r.SourceRunID,
			NewRunID:    r.NewRunID,
			DiffID:      r.DiffID,
			Mode:        string(r.Mode),
			Status:      string(r.Status),
			Error:       r.Error,
			StartedAt:   r.StartedAt.Unix(),
			CompletedAt: timeToUnixPtr(r.CompletedAt),
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"replays": responses,
		"count":   len(responses),
	})
}

// timeToUnixPtr converts a time pointer to unix timestamp, returning 0 if nil
func timeToUnixPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

// ReplayAPIRequest is the request body for POST /api/replay
type ReplayAPIRequest struct {
	SourceRunID string `json:"source_run_id"`
	Mode        string `json:"mode,omitempty"` // live, mocked, hybrid

	// Overrides
	Model       string   `json:"model,omitempty"`
	Provider    string   `json:"provider,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`

	// Advanced overrides
	GraphOverrides json.RawMessage `json:"graph_overrides,omitempty"`
	InputOverrides json.RawMessage `json:"input_overrides,omitempty"`

	// Options
	Tags     map[string]string `json:"tags,omitempty"`
	AutoDiff bool              `json:"auto_diff,omitempty"`
	Sync     bool              `json:"sync,omitempty"` // Wait for completion
}

// ReplayAPIResponse is the response for replay endpoints
type ReplayAPIResponse struct {
	ReplayID    string `json:"replay_id"`
	SourceRunID string `json:"source_run_id"`
	NewRunID    string `json:"new_run_id,omitempty"`
	DiffID      string `json:"diff_id,omitempty"`
	Mode        string `json:"mode"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	StartedAt   int64  `json:"started_at"`
	CompletedAt int64  `json:"completed_at,omitempty"`
}
