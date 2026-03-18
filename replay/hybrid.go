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

// HybridReplayer executes replay with mocked tools and live LLM calls
type HybridReplayer struct {
	store          store.TraceStore
	petalflowURL   string
	httpClient     *http.Client
	requestTimeout time.Duration
}

// HybridReplayerConfig configures the hybrid replayer
type HybridReplayerConfig struct {
	Store          store.TraceStore
	PetalFlowURL   string
	RequestTimeout time.Duration
}

// NewHybridReplayer creates a new hybrid replayer
func NewHybridReplayer(cfg HybridReplayerConfig) *HybridReplayer {
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 5 * time.Minute
	}
	if cfg.PetalFlowURL == "" {
		cfg.PetalFlowURL = "http://localhost:8080"
	}

	return &HybridReplayer{
		store:          cfg.Store,
		petalflowURL:   cfg.PetalFlowURL,
		requestTimeout: cfg.RequestTimeout,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

// HybridExecuteRequest extends ExecuteRequest with mocked tool data
type HybridExecuteRequest struct {
	ExecuteRequest

	// MockedTools contains tool results to use instead of executing
	MockedTools map[string]json.RawMessage `json:"mocked_tools,omitempty"`

	// ToolMockMode controls how tools are mocked
	ToolMockMode ToolMockMode `json:"tool_mock_mode,omitempty"`
}

// ToolMockMode defines how tools are mocked
type ToolMockMode string

const (
	// ToolMockModeAll mocks all tool calls
	ToolMockModeAll ToolMockMode = "all"
	// ToolMockModeExternal only mocks external tool calls
	ToolMockModeExternal ToolMockMode = "external"
	// ToolMockModeNone doesn't mock any tools (live execution)
	ToolMockModeNone ToolMockMode = "none"
)

// Replay executes a hybrid replay with mocked tools and live LLM
func (r *HybridReplayer) Replay(ctx context.Context, req *ReplayRequest, captured *CapturedData) (*Replay, error) {
	replay := &Replay{
		ID:          uuid.New().String(),
		SourceRunID: req.SourceRunID,
		Mode:        ReplayModeHybrid,
		Status:      ReplayStatusRunning,
		StartedAt:   time.Now(),
	}

	// Build mock provider for tools only
	mockProvider := BuildMockProvider(captured)

	// Build hybrid execution request
	execReq := HybridExecuteRequest{
		ExecuteRequest: ExecuteRequest{
			WorkflowID:    captured.Run.WorkflowID,
			WorkflowName:  captured.Run.WorkflowName,
			Graph:         captured.Run.GraphSnapshot,
			Input:         captured.Run.InputSnapshot,
			Config:        captured.Run.ConfigSnapshot,
			ParentRunID:   req.SourceRunID,
			TriggerSource: "petaltrace-hybrid-replay",
		},
		MockedTools:  make(map[string]json.RawMessage),
		ToolMockMode: ToolMockModeAll,
	}

	// Add tool mocks
	for toolID, result := range mockProvider.ToolResults {
		execReq.MockedTools[toolID] = result.Output
	}

	// Apply tags
	execReq.Tags = make(map[string]string)
	execReq.Tags["replay_source"] = req.SourceRunID
	execReq.Tags["replay_mode"] = string(req.Mode)
	for k, v := range req.Tags {
		execReq.Tags[k] = v
	}

	// Apply model overrides (LLM calls are live)
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

	// Execute via PetalFlow API with hybrid mode
	execResp, err := r.executeHybrid(ctx, execReq)
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

// executeHybrid sends hybrid execution request to PetalFlow
func (r *HybridReplayer) executeHybrid(ctx context.Context, req HybridExecuteRequest) (*ExecuteResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", r.petalflowURL+"/api/execute/hybrid", bytes.NewReader(body))
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

// WaitForCompletion polls for hybrid replay completion
func (r *HybridReplayer) WaitForCompletion(ctx context.Context, replay *Replay, pollInterval time.Duration) error {
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
				continue
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
		}
	}
}

// ExtractToolMocks extracts tool mock data from captured spans
func ExtractToolMocks(toolSpans []*store.Span) map[string]json.RawMessage {
	mocks := make(map[string]json.RawMessage)

	for _, span := range toolSpans {
		if span.Tool == nil {
			continue
		}

		// Use tool use ID or span ID as key
		key := span.ID
		if span.Tool.ToolUseID != nil && *span.Tool.ToolUseID != "" {
			key = *span.Tool.ToolUseID
		}

		mocks[key] = span.Tool.Outputs
	}

	return mocks
}
