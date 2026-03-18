package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRunMarshalJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	completedAt := now.Add(5 * time.Second)
	parentID := "parent_123"

	run := Run{
		ID:              "run_abc123",
		WorkflowID:      "wf_001",
		WorkflowName:    "research_pipeline",
		WorkflowVersion: "1.0.0",
		SourceKind:      "agent_workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		CompletedAt:     &completedAt,
		DurationMs:      5000,
		TotalTokens: TokenSummary{
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
		},
		EstimatedCost: CostEstimate{
			Currency: "USD",
			Total:    0.05,
			ByProvider: map[string]float64{
				"anthropic": 0.05,
			},
		},
		NodeCount:     3,
		ErrorCount:    0,
		Tags:          map[string]string{"env": "test"},
		TriggerSource: "cli",
		ParentRunID:   &parentID,
		Starred:       true,
		CreatedAt:     now,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Run
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != run.ID {
		t.Errorf("ID = %s, want %s", decoded.ID, run.ID)
	}

	if decoded.Status != RunStatusCompleted {
		t.Errorf("Status = %s, want %s", decoded.Status, RunStatusCompleted)
	}

	if decoded.TotalTokens.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500", decoded.TotalTokens.TotalTokens)
	}

	if decoded.EstimatedCost.Total != 0.05 {
		t.Errorf("EstimatedCost.Total = %f, want 0.05", decoded.EstimatedCost.Total)
	}

	if decoded.ParentRunID == nil || *decoded.ParentRunID != parentID {
		t.Errorf("ParentRunID mismatch")
	}

	if !decoded.Starred {
		t.Error("Starred = false, want true")
	}
}

func TestSpanMarshalJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	completedAt := now.Add(2 * time.Second)
	parentID := "span_parent"

	span := Span{
		ID:          "span_xyz",
		RunID:       "run_abc",
		ParentID:    &parentID,
		TraceID:     "trace_123",
		Kind:        SpanKindLLM,
		Name:        "llm_call",
		Status:      SpanStatusOK,
		StartedAt:   now,
		CompletedAt: &completedAt,
		DurationMs:  2000,
		LLM: &LLMSpanData{
			Provider:     "anthropic",
			Model:        "claude-sonnet-4-20250514",
			SystemPrompt: "You are a helpful assistant.",
			Messages: []LLMMessage{
				{
					Role:    "user",
					Content: json.RawMessage(`"Hello, world!"`),
				},
			},
			Completion: LLMCompletion{
				TextContent: "Hello! How can I help you today?",
			},
			Tokens: TokenDetail{
				InputTokens:  50,
				OutputTokens: 20,
				TotalTokens:  70,
				CostEstimate: 0.001,
			},
			TotalLatency: 2000,
			StopReason:   "end_turn",
		},
		Attributes: map[string]any{
			"custom_key": "custom_value",
		},
		CreatedAt: now,
	}

	data, err := json.Marshal(span)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Span
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Kind != SpanKindLLM {
		t.Errorf("Kind = %s, want %s", decoded.Kind, SpanKindLLM)
	}

	if decoded.LLM == nil {
		t.Fatal("LLM is nil")
	}

	if decoded.LLM.Provider != "anthropic" {
		t.Errorf("LLM.Provider = %s, want anthropic", decoded.LLM.Provider)
	}

	if decoded.LLM.Tokens.TotalTokens != 70 {
		t.Errorf("LLM.Tokens.TotalTokens = %d, want 70", decoded.LLM.Tokens.TotalTokens)
	}
}

func TestNodeSpanDataMarshalJSON(t *testing.T) {
	data := NodeSpanData{
		NodeID:     "node_1",
		NodeType:   "llm_prompt",
		Inputs:     json.RawMessage(`{"prompt": "hello"}`),
		Outputs:    json.RawMessage(`{"response": "world"}`),
		Config:     json.RawMessage(`{"model": "claude-3"}`),
		RetryCount: 2,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded NodeSpanData
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.NodeID != "node_1" {
		t.Errorf("NodeID = %s, want node_1", decoded.NodeID)
	}

	if decoded.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", decoded.RetryCount)
	}
}

func TestToolSpanDataMarshalJSON(t *testing.T) {
	invokedBy := "llm_span_1"
	toolUseID := "toolu_123"

	data := ToolSpanData{
		ToolName:   "web_search",
		ActionName: "search",
		Origin:     "mcp",
		Inputs:     json.RawMessage(`{"query": "golang testing"}`),
		Outputs:    json.RawMessage(`{"results": []}`),
		DurationMs: 500,
		InvokedBy:  &invokedBy,
		ToolUseID:  &toolUseID,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ToolSpanData
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ToolName != "web_search" {
		t.Errorf("ToolName = %s, want web_search", decoded.ToolName)
	}

	if decoded.InvokedBy == nil || *decoded.InvokedBy != invokedBy {
		t.Error("InvokedBy mismatch")
	}
}

func TestEdgeSpanDataMarshalJSON(t *testing.T) {
	data := EdgeSpanData{
		SourceNode:  "node_a",
		SourcePort:  "output",
		TargetNode:  "node_b",
		TargetPort:  "input",
		DataSize:    1024,
		DataPreview: "Preview text...",
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded EdgeSpanData
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SourceNode != "node_a" {
		t.Errorf("SourceNode = %s, want node_a", decoded.SourceNode)
	}

	if decoded.DataSize != 1024 {
		t.Errorf("DataSize = %d, want 1024", decoded.DataSize)
	}
}

func TestRunStatusConstants(t *testing.T) {
	statuses := []RunStatus{
		RunStatusRunning,
		RunStatusCompleted,
		RunStatusFailed,
		RunStatusCancelled,
	}

	expected := []string{"running", "completed", "failed", "cancelled"}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("RunStatus = %s, want %s", status, expected[i])
		}
	}
}

func TestSpanKindConstants(t *testing.T) {
	kinds := []SpanKind{
		SpanKindNode,
		SpanKindLLM,
		SpanKindTool,
		SpanKindEdge,
		SpanKindCustom,
	}

	expected := []string{"node", "llm", "tool", "edge", "custom"}

	for i, kind := range kinds {
		if string(kind) != expected[i] {
			t.Errorf("SpanKind = %s, want %s", kind, expected[i])
		}
	}
}

func TestCostEstimateMarshalJSON(t *testing.T) {
	cost := CostEstimate{
		Currency: "USD",
		Total:    1.25,
		ByProvider: map[string]float64{
			"anthropic": 1.00,
			"openai":    0.25,
		},
		ByModel: map[string]float64{
			"claude-sonnet-4-20250514": 1.00,
			"gpt-4o":             0.25,
		},
		ByNode: map[string]float64{
			"researcher": 0.80,
			"writer":     0.45,
		},
	}

	bytes, err := json.Marshal(cost)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CostEstimate
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Total != 1.25 {
		t.Errorf("Total = %f, want 1.25", decoded.Total)
	}

	if decoded.ByProvider["anthropic"] != 1.00 {
		t.Errorf("ByProvider[anthropic] = %f, want 1.00", decoded.ByProvider["anthropic"])
	}

	if decoded.ByModel["claude-sonnet-4-20250514"] != 1.00 {
		t.Errorf("ByModel[claude-sonnet-4-20250514] = %f, want 1.00", decoded.ByModel["claude-sonnet-4-20250514"])
	}
}

func TestRunDiffMarshalJSON(t *testing.T) {
	diff := RunDiff{
		ID:           "diff_001",
		BaseRunID:    "run_a",
		CompareRunID: "run_b",
		Summary: DiffSummary{
			StatusMatch:    true,
			DurationDelta:  1000,
			TokenDelta:     200,
			CostDelta:      0.02,
			NodeDiffCount:  2,
			PathDivergence: false,
		},
		NodeDiffs: []NodeDiff{
			{
				NodeID:          "node_1",
				NodeType:        "llm_prompt",
				Status:          "content_diff",
				DurationBase:    2000,
				DurationCompare: 2500,
			},
		},
		CostDiff: CostDiff{
			BaseCost:    0.10,
			CompareCost: 0.12,
			Delta:       0.02,
		},
		CreatedAt: time.Now().UTC(),
	}

	bytes, err := json.Marshal(diff)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded RunDiff
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Summary.NodeDiffCount != 2 {
		t.Errorf("Summary.NodeDiffCount = %d, want 2", decoded.Summary.NodeDiffCount)
	}

	if len(decoded.NodeDiffs) != 1 {
		t.Fatalf("NodeDiffs length = %d, want 1", len(decoded.NodeDiffs))
	}

	if decoded.NodeDiffs[0].NodeID != "node_1" {
		t.Errorf("NodeDiffs[0].NodeID = %s, want node_1", decoded.NodeDiffs[0].NodeID)
	}
}
