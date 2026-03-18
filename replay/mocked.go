package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/petal-labs/petaltrace/store"
)

// MockedReplayer executes replay using captured responses
type MockedReplayer struct {
	store        store.TraceStore
	petalflowURL string
}

// MockedReplayerConfig configures the mocked replayer
type MockedReplayerConfig struct {
	Store        store.TraceStore
	PetalFlowURL string
}

// NewMockedReplayer creates a new mocked replayer
func NewMockedReplayer(cfg MockedReplayerConfig) *MockedReplayer {
	if cfg.PetalFlowURL == "" {
		cfg.PetalFlowURL = "http://localhost:8080"
	}
	return &MockedReplayer{
		store:        cfg.Store,
		petalflowURL: cfg.PetalFlowURL,
	}
}

// MockProvider holds captured LLM responses for mocking
type MockProvider struct {
	// Responses maps node ID to captured LLM response
	Responses map[string]*CapturedLLMResponse `json:"responses"`

	// ToolResults maps tool call ID to captured result
	ToolResults map[string]*CapturedToolResult `json:"tool_results"`
}

// CapturedLLMResponse holds a captured LLM response
type CapturedLLMResponse struct {
	NodeID       string          `json:"node_id"`
	SpanID       string          `json:"span_id"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Completion   json.RawMessage `json:"completion"`
	TextContent  string          `json:"text_content"`
	StopReason   string          `json:"stop_reason"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
}

// CapturedToolResult holds a captured tool execution result
type CapturedToolResult struct {
	ToolName string          `json:"tool_name"`
	ToolID   string          `json:"tool_id"`
	SpanID   string          `json:"span_id"`
	Input    json.RawMessage `json:"input"`
	Output   json.RawMessage `json:"output"`
	Success  bool            `json:"success"`
	Error    string          `json:"error,omitempty"`
}

// BuildMockProvider creates a mock provider from captured data
func BuildMockProvider(captured *CapturedData) *MockProvider {
	mock := &MockProvider{
		Responses:   make(map[string]*CapturedLLMResponse),
		ToolResults: make(map[string]*CapturedToolResult),
	}

	// Extract LLM responses
	for _, span := range captured.LLMSpans {
		if span.LLM == nil {
			continue
		}

		nodeID := span.Name
		if span.Node != nil && span.Node.NodeID != "" {
			nodeID = span.Node.NodeID
		}

		mock.Responses[nodeID] = &CapturedLLMResponse{
			NodeID:       nodeID,
			SpanID:       span.ID,
			Provider:     span.LLM.Provider,
			Model:        span.LLM.Model,
			Completion:   span.LLM.Completion.Content,
			TextContent:  span.LLM.Completion.TextContent,
			StopReason:   span.LLM.StopReason,
			InputTokens:  span.LLM.Tokens.InputTokens,
			OutputTokens: span.LLM.Tokens.OutputTokens,
		}
	}

	// Extract tool results
	for _, span := range captured.ToolSpans {
		if span.Tool == nil {
			continue
		}

		toolID := span.ID
		if span.Tool.ToolUseID != nil && *span.Tool.ToolUseID != "" {
			toolID = *span.Tool.ToolUseID
		}

		success := span.Status == store.SpanStatusOK
		var errMsg string
		if span.Error != nil {
			errMsg = span.Error.Message
		}

		mock.ToolResults[toolID] = &CapturedToolResult{
			ToolName: span.Tool.ToolName,
			ToolID:   toolID,
			SpanID:   span.ID,
			Input:    span.Tool.Inputs,
			Output:   span.Tool.Outputs,
			Success:  success,
			Error:    errMsg,
		}
	}

	return mock
}

// Replay executes a mocked replay using captured responses
func (r *MockedReplayer) Replay(ctx context.Context, req *ReplayRequest, captured *CapturedData) (*Replay, error) {
	replay := &Replay{
		ID:          uuid.New().String(),
		SourceRunID: req.SourceRunID,
		Mode:        ReplayModeMocked,
		Status:      ReplayStatusRunning,
		StartedAt:   time.Now(),
	}

	// Build mock provider
	mockProvider := BuildMockProvider(captured)

	// For mocked replay, we create a deterministic execution
	// that returns the same outputs as the original run
	newRunID, err := r.executeMocked(ctx, req, captured, mockProvider)
	if err != nil {
		replay.Status = ReplayStatusFailed
		replay.Error = err.Error()
		now := time.Now()
		replay.CompletedAt = &now
		return replay, err
	}

	replay.NewRunID = newRunID
	replay.Status = ReplayStatusCompleted
	now := time.Now()
	replay.CompletedAt = &now

	return replay, nil
}

// executeMocked creates a new run with mocked execution data
func (r *MockedReplayer) executeMocked(ctx context.Context, req *ReplayRequest, captured *CapturedData, mock *MockProvider) (string, error) {
	// Generate new run ID
	newRunID := "replay-" + uuid.New().String()[:8]

	// Create new run based on source
	newRun := &store.Run{
		ID:              newRunID,
		WorkflowID:      captured.Run.WorkflowID,
		WorkflowName:    captured.Run.WorkflowName,
		WorkflowVersion: captured.Run.WorkflowVersion,
		SourceKind:      "mocked-replay",
		Status:          store.RunStatusRunning,
		StartedAt:       time.Now(),
		GraphSnapshot:   captured.Run.GraphSnapshot,
		InputSnapshot:   captured.Run.InputSnapshot,
		ConfigSnapshot:  captured.Run.ConfigSnapshot,
		ParentRunID:     &req.SourceRunID,
		Tags:            make(map[string]string),
		TriggerSource:   "petaltrace-replay",
		CreatedAt:       time.Now(),
	}

	// Apply tags
	newRun.Tags["replay_source"] = req.SourceRunID
	newRun.Tags["replay_mode"] = string(req.Mode)
	for k, v := range req.Tags {
		newRun.Tags[k] = v
	}

	// Create the run
	if err := r.store.CreateRun(ctx, newRun); err != nil {
		return "", fmt.Errorf("creating run: %w", err)
	}

	// Copy spans with new IDs but same data
	spanIDMap := make(map[string]string) // old ID -> new ID
	for _, span := range captured.Spans {
		newSpanID := uuid.New().String()
		spanIDMap[span.ID] = newSpanID

		newSpan := &store.Span{
			ID:          newSpanID,
			RunID:       newRunID,
			TraceID:     newRunID,
			Name:        span.Name,
			Kind:        span.Kind,
			Status:      span.Status,
			StartedAt:   time.Now(),
			DurationMs:  span.DurationMs,
			Attributes:  span.Attributes,
			Node:        span.Node,
			LLM:         span.LLM,
			Tool:        span.Tool,
			Edge:        span.Edge,
			Error:       span.Error,
		}

		// Update completed time
		completedAt := time.Now().Add(time.Duration(span.DurationMs) * time.Millisecond)
		newSpan.CompletedAt = &completedAt

		// Update parent ID if exists
		if span.ParentID != nil {
			if newParentID, ok := spanIDMap[*span.ParentID]; ok {
				newSpan.ParentID = &newParentID
			}
		}

		if err := r.store.CreateSpan(ctx, newSpan); err != nil {
			return "", fmt.Errorf("creating span: %w", err)
		}
	}

	// Update run aggregates
	if err := r.store.UpdateRunAggregates(ctx, newRunID); err != nil {
		return "", fmt.Errorf("updating aggregates: %w", err)
	}

	// Mark run as completed
	completedAt := time.Now()
	newRun.Status = store.RunStatusCompleted
	newRun.CompletedAt = &completedAt
	newRun.DurationMs = completedAt.Sub(newRun.StartedAt).Milliseconds()

	if err := r.store.UpdateRun(ctx, newRun); err != nil {
		return "", fmt.Errorf("completing run: %w", err)
	}

	return newRunID, nil
}

// GetMockedResponse returns the mocked response for a node
func (m *MockProvider) GetMockedResponse(nodeID string) (*CapturedLLMResponse, bool) {
	resp, ok := m.Responses[nodeID]
	return resp, ok
}

// GetMockedToolResult returns the mocked tool result
func (m *MockProvider) GetMockedToolResult(toolID string) (*CapturedToolResult, bool) {
	result, ok := m.ToolResults[toolID]
	return result, ok
}
