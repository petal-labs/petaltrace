package api

import (
	"net/http"
	"strings"

	"github.com/petal-labs/petaltrace/store"
)

// handleGetPrompt handles GET /api/runs/{id}/prompts/{nodeId}
func (s *Server) handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := r.PathValue("id")
	nodeID := r.PathValue("nodeId")

	if runID == "" || nodeID == "" {
		s.writeError(w, http.StatusBadRequest, "run ID and node ID are required")
		return
	}

	// Get LLM spans for the run
	spans, err := s.store.GetSpansByKind(ctx, runID, store.SpanKindLLM)
	if err != nil {
		s.logger.Error("failed to get LLM spans", "error", err, "run_id", runID)
		s.writeError(w, http.StatusInternalServerError, "failed to get spans")
		return
	}

	// Find the span matching the node ID
	var targetSpan *store.Span
	for _, span := range spans {
		// Match by node ID
		if span.Node != nil && span.Node.NodeID == nodeID {
			targetSpan = span
			break
		}
		// Also check if span name contains the node ID
		if strings.Contains(span.Name, nodeID) {
			targetSpan = span
			break
		}
	}

	// If no node match, try by span ID or name
	if targetSpan == nil {
		for _, span := range spans {
			if span.ID == nodeID || span.Name == nodeID {
				targetSpan = span
				break
			}
		}
	}

	if targetSpan == nil {
		s.writeError(w, http.StatusNotFound, "LLM span not found for node")
		return
	}

	if targetSpan.LLM == nil {
		s.writeError(w, http.StatusNotFound, "span has no LLM data")
		return
	}

	// Build response
	response := PromptResponse{
		SpanID:   targetSpan.ID,
		RunID:    targetSpan.RunID,
		Name:     targetSpan.Name,
		Provider: targetSpan.LLM.Provider,
		Model:    targetSpan.LLM.Model,
		Prompt: PromptData{
			SystemPrompt:    targetSpan.LLM.SystemPrompt,
			Messages:        targetSpan.LLM.Messages,
			ToolDefinitions: targetSpan.LLM.ToolDefinitions,
			Temperature:     targetSpan.LLM.Temperature,
			MaxTokens:       targetSpan.LLM.MaxTokens,
		},
		Completion: CompletionData{
			Content:     targetSpan.LLM.Completion.Content,
			TextContent: targetSpan.LLM.Completion.TextContent,
			StopReason:  targetSpan.LLM.StopReason,
		},
		Tokens: TokenData{
			InputTokens:  targetSpan.LLM.Tokens.InputTokens,
			OutputTokens: targetSpan.LLM.Tokens.OutputTokens,
			TotalTokens:  targetSpan.LLM.Tokens.TotalTokens,
			CostEstimate: targetSpan.LLM.Tokens.CostEstimate,
		},
		Timing: TimingData{
			StartedAt:        targetSpan.StartedAt,
			CompletedAt:      targetSpan.CompletedAt,
			DurationMs:       targetSpan.DurationMs,
			TimeToFirstToken: targetSpan.LLM.TimeToFirstToken,
			TotalLatency:     targetSpan.LLM.TotalLatency,
		},
	}

	// Add cache info if available
	if targetSpan.LLM.CacheRead != nil {
		response.Tokens.CacheReadTokens = targetSpan.LLM.CacheRead
	}
	if targetSpan.LLM.CacheCreation != nil {
		response.Tokens.CacheWriteTokens = targetSpan.LLM.CacheCreation
	}

	// Add node info if available
	if targetSpan.Node != nil {
		response.NodeID = targetSpan.Node.NodeID
		response.NodeType = targetSpan.Node.NodeType
	}

	s.writeJSON(w, http.StatusOK, response)
}

// PromptResponse is the response for GET /api/runs/{id}/prompts/{nodeId}
type PromptResponse struct {
	SpanID     string         `json:"span_id"`
	RunID      string         `json:"run_id"`
	NodeID     string         `json:"node_id,omitempty"`
	NodeType   string         `json:"node_type,omitempty"`
	Name       string         `json:"name"`
	Provider   string         `json:"provider"`
	Model      string         `json:"model"`
	Prompt     PromptData     `json:"prompt"`
	Completion CompletionData `json:"completion"`
	Tokens     TokenData      `json:"tokens"`
	Timing     TimingData     `json:"timing"`
}

// PromptData contains prompt information
type PromptData struct {
	SystemPrompt    string                 `json:"system_prompt,omitempty"`
	Messages        []store.LLMMessage     `json:"messages,omitempty"`
	ToolDefinitions []store.ToolDefinition `json:"tool_definitions,omitempty"`
	Temperature     *float64               `json:"temperature,omitempty"`
	MaxTokens       *int                   `json:"max_tokens,omitempty"`
}

// CompletionData contains completion information
type CompletionData struct {
	Content     any    `json:"content,omitempty"`
	TextContent string `json:"text_content,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// TokenData contains token usage information
type TokenData struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostEstimate     float64 `json:"cost_estimate"`
	CacheReadTokens  *int    `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int    `json:"cache_write_tokens,omitempty"`
}

// TimingData contains timing information
type TimingData struct {
	StartedAt        any    `json:"started_at"`
	CompletedAt      any    `json:"completed_at,omitempty"`
	DurationMs       int64  `json:"duration_ms"`
	TimeToFirstToken *int64 `json:"time_to_first_token_ms,omitempty"`
	TotalLatency     int64  `json:"total_latency_ms"`
}
