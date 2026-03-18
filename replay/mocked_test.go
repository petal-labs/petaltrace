package replay

import (
	"encoding/json"
	"testing"

	"github.com/petal-labs/petaltrace/store"
)

func TestNewMockedReplayer(t *testing.T) {
	ms := newMockStore()
	r := NewMockedReplayer(MockedReplayerConfig{
		Store: ms,
	})

	if r == nil {
		t.Fatal("NewMockedReplayer() returned nil")
	}

	if r.petalflowURL != "http://localhost:8080" {
		t.Errorf("default petalflowURL = %q, want %q", r.petalflowURL, "http://localhost:8080")
	}

	// Test with custom URL
	r2 := NewMockedReplayer(MockedReplayerConfig{
		Store:        ms,
		PetalFlowURL: "http://custom:9000",
	})

	if r2.petalflowURL != "http://custom:9000" {
		t.Errorf("custom petalflowURL = %q, want %q", r2.petalflowURL, "http://custom:9000")
	}
}

func TestBuildMockProvider(t *testing.T) {
	toolID := "tool-use-123"
	captured := &CapturedData{
		Run: &store.Run{ID: "run-1"},
		LLMSpans: []*store.Span{
			{
				ID:   "llm-span-1",
				Name: "agent",
				LLM: &store.LLMSpanData{
					Provider: "openai",
					Model:    "gpt-4",
					Completion: store.LLMCompletion{
						Content:     json.RawMessage(`"Hello!"`),
						TextContent: "Hello!",
					},
					StopReason: "end_turn",
					Tokens: store.TokenDetail{
						InputTokens:  100,
						OutputTokens: 20,
					},
				},
			},
			{
				ID:   "llm-span-2",
				Name: "processor",
				Node: &store.NodeSpanData{NodeID: "processor-node"},
				LLM: &store.LLMSpanData{
					Provider: "anthropic",
					Model:    "claude-3",
					Completion: store.LLMCompletion{
						TextContent: "Processed!",
					},
				},
			},
		},
		ToolSpans: []*store.Span{
			{
				ID:     "tool-span-1",
				Status: store.SpanStatusOK,
				Tool: &store.ToolSpanData{
					ToolName:  "calculator",
					ToolUseID: &toolID,
					Inputs:    json.RawMessage(`{"expr": "2+2"}`),
					Outputs:   json.RawMessage(`{"result": 4}`),
				},
			},
			{
				ID:     "tool-span-2",
				Status: store.SpanStatusError,
				Tool: &store.ToolSpanData{
					ToolName: "file_reader",
					Inputs:   json.RawMessage(`{"path": "/etc/passwd"}`),
				},
				Error: &store.SpanError{
					Message: "access denied",
				},
			},
		},
	}

	mock := BuildMockProvider(captured)

	// Check LLM responses
	if len(mock.Responses) != 2 {
		t.Errorf("len(Responses) = %d, want 2", len(mock.Responses))
	}

	// Check first LLM response (keyed by span name)
	resp, ok := mock.Responses["agent"]
	if !ok {
		t.Error("expected LLM response for 'agent'")
	} else {
		if resp.Provider != "openai" {
			t.Errorf("Provider = %q, want %q", resp.Provider, "openai")
		}
		if resp.Model != "gpt-4" {
			t.Errorf("Model = %q, want %q", resp.Model, "gpt-4")
		}
		if resp.TextContent != "Hello!" {
			t.Errorf("TextContent = %q, want %q", resp.TextContent, "Hello!")
		}
		if resp.InputTokens != 100 {
			t.Errorf("InputTokens = %d, want %d", resp.InputTokens, 100)
		}
	}

	// Check second LLM response (keyed by node ID)
	resp2, ok := mock.Responses["processor-node"]
	if !ok {
		t.Error("expected LLM response for 'processor-node'")
	} else {
		if resp2.Provider != "anthropic" {
			t.Errorf("Provider = %q, want %q", resp2.Provider, "anthropic")
		}
	}

	// Check tool results
	if len(mock.ToolResults) != 2 {
		t.Errorf("len(ToolResults) = %d, want 2", len(mock.ToolResults))
	}

	// Check first tool result (keyed by tool use ID)
	toolResult, ok := mock.ToolResults[toolID]
	if !ok {
		t.Error("expected tool result for tool-use-123")
	} else {
		if toolResult.ToolName != "calculator" {
			t.Errorf("ToolName = %q, want %q", toolResult.ToolName, "calculator")
		}
		if !toolResult.Success {
			t.Error("expected Success = true")
		}
	}

	// Check second tool result (keyed by span ID, no tool use ID)
	toolResult2, ok := mock.ToolResults["tool-span-2"]
	if !ok {
		t.Error("expected tool result for tool-span-2")
	} else {
		if toolResult2.ToolName != "file_reader" {
			t.Errorf("ToolName = %q, want %q", toolResult2.ToolName, "file_reader")
		}
		if toolResult2.Success {
			t.Error("expected Success = false")
		}
		if toolResult2.Error != "access denied" {
			t.Errorf("Error = %q, want %q", toolResult2.Error, "access denied")
		}
	}
}

func TestBuildMockProvider_EmptyCapturedData(t *testing.T) {
	captured := &CapturedData{
		Run:       &store.Run{ID: "run-1"},
		LLMSpans:  nil,
		ToolSpans: nil,
	}

	mock := BuildMockProvider(captured)

	if len(mock.Responses) != 0 {
		t.Errorf("len(Responses) = %d, want 0", len(mock.Responses))
	}

	if len(mock.ToolResults) != 0 {
		t.Errorf("len(ToolResults) = %d, want 0", len(mock.ToolResults))
	}
}

func TestBuildMockProvider_NilLLMData(t *testing.T) {
	captured := &CapturedData{
		Run: &store.Run{ID: "run-1"},
		LLMSpans: []*store.Span{
			{ID: "span-1", Name: "agent", LLM: nil},
		},
		ToolSpans: []*store.Span{
			{ID: "span-2", Tool: nil},
		},
	}

	mock := BuildMockProvider(captured)

	// Should skip spans with nil LLM/Tool data
	if len(mock.Responses) != 0 {
		t.Errorf("len(Responses) = %d, want 0", len(mock.Responses))
	}

	if len(mock.ToolResults) != 0 {
		t.Errorf("len(ToolResults) = %d, want 0", len(mock.ToolResults))
	}
}

func TestMockProvider_GetMockedResponse(t *testing.T) {
	mock := &MockProvider{
		Responses: map[string]*CapturedLLMResponse{
			"node1": {NodeID: "node1", Model: "gpt-4"},
		},
	}

	// Found
	resp, ok := mock.GetMockedResponse("node1")
	if !ok {
		t.Error("expected to find response for node1")
	}
	if resp.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", resp.Model, "gpt-4")
	}

	// Not found
	_, ok = mock.GetMockedResponse("node2")
	if ok {
		t.Error("expected not to find response for node2")
	}
}

func TestMockProvider_GetMockedToolResult(t *testing.T) {
	mock := &MockProvider{
		ToolResults: map[string]*CapturedToolResult{
			"tool1": {ToolName: "calculator", Success: true},
		},
	}

	// Found
	result, ok := mock.GetMockedToolResult("tool1")
	if !ok {
		t.Error("expected to find tool result for tool1")
	}
	if result.ToolName != "calculator" {
		t.Errorf("ToolName = %q, want %q", result.ToolName, "calculator")
	}

	// Not found
	_, ok = mock.GetMockedToolResult("tool2")
	if ok {
		t.Error("expected not to find tool result for tool2")
	}
}
