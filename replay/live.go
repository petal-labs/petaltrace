package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/petal-labs/petaltrace/store"
)

// LiveReplayer executes replay against real LLM providers
type LiveReplayer struct {
	store          store.TraceStore
	petalflowURL   string
	httpClient     *http.Client
	requestTimeout time.Duration
}

// LiveReplayerConfig configures the live replayer
type LiveReplayerConfig struct {
	Store          store.TraceStore
	PetalFlowURL   string        // URL to PetalFlow daemon API
	RequestTimeout time.Duration // HTTP request timeout
}

// NewLiveReplayer creates a new live replayer
func NewLiveReplayer(cfg LiveReplayerConfig) *LiveReplayer {
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 5 * time.Minute
	}
	if cfg.PetalFlowURL == "" {
		cfg.PetalFlowURL = "http://localhost:8080"
	}

	return &LiveReplayer{
		store:          cfg.Store,
		petalflowURL:   cfg.PetalFlowURL,
		requestTimeout: cfg.RequestTimeout,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// ExecuteRequest holds the request to PetalFlow for execution
type ExecuteRequest struct {
	WorkflowID    string            `json:"workflow_id"`
	WorkflowName  string            `json:"workflow_name"`
	Graph         json.RawMessage   `json:"graph"`
	Input         json.RawMessage   `json:"input"`
	Config        json.RawMessage   `json:"config,omitempty"`
	ParentRunID   string            `json:"parent_run_id,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	TriggerSource string            `json:"trigger_source"`

	// Model overrides
	ModelOverride       string   `json:"model_override,omitempty"`
	ProviderOverride    string   `json:"provider_override,omitempty"`
	TemperatureOverride *float64 `json:"temperature_override,omitempty"`
	MaxTokensOverride   *int     `json:"max_tokens_override,omitempty"`
}

// ExecuteResponse holds the response from PetalFlow
type ExecuteResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// Replay executes a live replay of a source run
func (r *LiveReplayer) Replay(ctx context.Context, req *ReplayRequest, captured *CapturedData) (*Replay, error) {
	replay := &Replay{
		ID:          uuid.New().String(),
		SourceRunID: req.SourceRunID,
		Mode:        ReplayModeLive,
		Status:      ReplayStatusRunning,
		StartedAt:   time.Now(),
	}

	// Build execution request
	execReq := ExecuteRequest{
		WorkflowID:    captured.Run.WorkflowID,
		WorkflowName:  captured.Run.WorkflowName,
		Graph:         captured.Run.GraphSnapshot,
		Input:         captured.Run.InputSnapshot,
		Config:        captured.Run.ConfigSnapshot,
		ParentRunID:   req.SourceRunID,
		TriggerSource: "petaltrace-replay",
	}

	// Apply tags
	execReq.Tags = make(map[string]string)
	execReq.Tags["replay_source"] = req.SourceRunID
	execReq.Tags["replay_mode"] = string(req.Mode)
	for k, v := range req.Tags {
		execReq.Tags[k] = v
	}

	// Apply overrides
	if req.Overrides.Model != "" {
		execReq.ModelOverride = req.Overrides.Model
	}
	if req.Overrides.Provider != "" {
		execReq.ProviderOverride = req.Overrides.Provider
	}
	if req.Overrides.Temperature != nil {
		execReq.TemperatureOverride = req.Overrides.Temperature
	}
	if req.Overrides.MaxTokens != nil {
		execReq.MaxTokensOverride = req.Overrides.MaxTokens
	}

	// Execute via PetalFlow API
	execResp, err := r.executeViaAPI(ctx, execReq)
	if err != nil {
		replay.Status = ReplayStatusFailed
		replay.Error = err.Error()
		now := time.Now()
		replay.CompletedAt = &now
		return replay, err
	}

	replay.NewRunID = execResp.RunID

	// If synchronous execution, mark as completed
	if execResp.Status == "completed" || execResp.Status == "failed" {
		now := time.Now()
		replay.CompletedAt = &now
		if execResp.Status == "completed" {
			replay.Status = ReplayStatusCompleted
		} else {
			replay.Status = ReplayStatusFailed
			replay.Error = execResp.Error
		}
	}

	return replay, nil
}

// executeViaAPI sends execution request to PetalFlow daemon
func (r *LiveReplayer) executeViaAPI(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", r.petalflowURL+"/api/execute", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PetalFlow API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var execResp ExecuteResponse
	if err := json.Unmarshal(respBody, &execResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &execResp, nil
}

// WaitForCompletion polls for replay completion
func (r *LiveReplayer) WaitForCompletion(ctx context.Context, replay *Replay, pollInterval time.Duration) error {
	if replay.NewRunID == "" {
		return fmt.Errorf("replay has no new run ID")
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			run, err := r.store.GetRun(ctx, replay.NewRunID)
			if err != nil {
				return fmt.Errorf("checking run status: %w", err)
			}
			if run == nil {
				continue // Run not yet visible
			}

			switch run.Status {
			case store.RunStatusCompleted:
				replay.Status = ReplayStatusCompleted
				now := time.Now()
				replay.CompletedAt = &now
				return nil
			case store.RunStatusFailed:
				replay.Status = ReplayStatusFailed
				now := time.Now()
				replay.CompletedAt = &now
				return nil
			}
			// Still running, continue polling
		}
	}
}
