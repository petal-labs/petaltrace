package collector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func TestClassifier_LLMSpan(t *testing.T) {
	c := NewClassifier()

	now := time.Now()
	in := &IngestedSpan{
		TraceID:   "trace123",
		SpanID:    "span123",
		Name:      "llm-completion",
		StartTime: now,
		EndTime:   now.Add(500 * time.Millisecond),
		Status:    SpanStatusOK,
		Attributes: map[string]any{
			"gen_ai.system":                          "anthropic",
			"gen_ai.request.model":                   "claude-sonnet-4-20250514",
			"gen_ai.request.temperature":             0.7,
			"gen_ai.request.max_tokens":              int64(4096),
			"gen_ai.system_prompt":                   "You are a helpful assistant.",
			"gen_ai.usage.input_tokens":              int64(100),
			"gen_ai.usage.output_tokens":             int64(50),
			"gen_ai.usage.cache_read_input_tokens":   int64(20),
			"gen_ai.completion":                      "Hello! I'm happy to help.",
			"gen_ai.response.id":                     "msg_123",
			"gen_ai.stop_reason":                     "end_turn",
			"gen_ai.latency.time_to_first_token_ms":  int64(150),
		},
	}

	span := c.ClassifySpan(in)

	if span.Kind != store.SpanKindLLM {
		t.Errorf("Kind = %s, want %s", span.Kind, store.SpanKindLLM)
	}

	if span.LLM == nil {
		t.Fatal("LLM data is nil")
	}

	llm := span.LLM
	if llm.Provider != "anthropic" {
		t.Errorf("Provider = %s, want anthropic", llm.Provider)
	}
	if llm.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %s, want claude-sonnet-4-20250514", llm.Model)
	}
	if llm.Temperature == nil || *llm.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", llm.Temperature)
	}
	if llm.MaxTokens == nil || *llm.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %v, want 4096", llm.MaxTokens)
	}
	if llm.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("SystemPrompt = %s, want 'You are a helpful assistant.'", llm.SystemPrompt)
	}
	if llm.Tokens.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", llm.Tokens.InputTokens)
	}
	if llm.Tokens.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", llm.Tokens.OutputTokens)
	}
	if llm.Tokens.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", llm.Tokens.TotalTokens)
	}
	if llm.CacheRead == nil || *llm.CacheRead != 20 {
		t.Errorf("CacheRead = %v, want 20", llm.CacheRead)
	}
	if llm.Completion.TextContent != "Hello! I'm happy to help." {
		t.Errorf("Completion.TextContent = %s", llm.Completion.TextContent)
	}
	if llm.RequestID != "msg_123" {
		t.Errorf("RequestID = %s, want msg_123", llm.RequestID)
	}
	if llm.StopReason != "end_turn" {
		t.Errorf("StopReason = %s, want end_turn", llm.StopReason)
	}
	if llm.TimeToFirstToken == nil || *llm.TimeToFirstToken != 150 {
		t.Errorf("TimeToFirstToken = %v, want 150", llm.TimeToFirstToken)
	}
	if llm.TotalLatency != 500 {
		t.Errorf("TotalLatency = %d, want 500", llm.TotalLatency)
	}
}

func TestClassifier_NodeSpan(t *testing.T) {
	c := NewClassifier()

	now := time.Now()
	in := &IngestedSpan{
		TraceID:   "trace123",
		SpanID:    "span456",
		Name:      "process_data",
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Status:    SpanStatusOK,
		Attributes: map[string]any{
			"petalflow.node.id":          "node_abc",
			"petalflow.node.type":        "transformer",
			"petalflow.node.retry_count": int64(2),
			"petalflow.node.inputs": map[string]any{
				"data": "input value",
			},
			"petalflow.node.outputs": map[string]any{
				"result": "output value",
			},
		},
	}

	span := c.ClassifySpan(in)

	if span.Kind != store.SpanKindNode {
		t.Errorf("Kind = %s, want %s", span.Kind, store.SpanKindNode)
	}

	if span.Node == nil {
		t.Fatal("Node data is nil")
	}

	node := span.Node
	if node.NodeID != "node_abc" {
		t.Errorf("NodeID = %s, want node_abc", node.NodeID)
	}
	if node.NodeType != "transformer" {
		t.Errorf("NodeType = %s, want transformer", node.NodeType)
	}
	if node.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", node.RetryCount)
	}

	var inputs map[string]any
	if err := json.Unmarshal(node.Inputs, &inputs); err != nil {
		t.Fatalf("unmarshal inputs: %v", err)
	}
	if inputs["data"] != "input value" {
		t.Errorf("inputs[data] = %v, want 'input value'", inputs["data"])
	}
}

func TestClassifier_ToolSpan(t *testing.T) {
	c := NewClassifier()

	now := time.Now()
	in := &IngestedSpan{
		TraceID:   "trace123",
		SpanID:    "span789",
		Name:      "execute_tool",
		StartTime: now,
		EndTime:   now.Add(200 * time.Millisecond),
		Status:    SpanStatusOK,
		Attributes: map[string]any{
			"tool.name":       "web_search",
			"tool.action":     "search",
			"tool.origin":     "mcp",
			"tool.invoked_by": "node_abc",
			"tool.inputs": map[string]any{
				"query": "latest news",
			},
			"tool.outputs": map[string]any{
				"results": []any{"result1", "result2"},
			},
			"gen_ai.tool_use.id": "tool_use_123",
		},
	}

	span := c.ClassifySpan(in)

	if span.Kind != store.SpanKindTool {
		t.Errorf("Kind = %s, want %s", span.Kind, store.SpanKindTool)
	}

	if span.Tool == nil {
		t.Fatal("Tool data is nil")
	}

	tool := span.Tool
	if tool.ToolName != "web_search" {
		t.Errorf("ToolName = %s, want web_search", tool.ToolName)
	}
	if tool.ActionName != "search" {
		t.Errorf("ActionName = %s, want search", tool.ActionName)
	}
	if tool.Origin != "mcp" {
		t.Errorf("Origin = %s, want mcp", tool.Origin)
	}
	if tool.InvokedBy == nil || *tool.InvokedBy != "node_abc" {
		t.Errorf("InvokedBy = %v, want node_abc", tool.InvokedBy)
	}
	if tool.ToolUseID == nil || *tool.ToolUseID != "tool_use_123" {
		t.Errorf("ToolUseID = %v, want tool_use_123", tool.ToolUseID)
	}
	if tool.DurationMs != 200 {
		t.Errorf("DurationMs = %d, want 200", tool.DurationMs)
	}
}

func TestClassifier_EdgeSpan(t *testing.T) {
	c := NewClassifier()

	now := time.Now()
	in := &IngestedSpan{
		TraceID:   "trace123",
		SpanID:    "span_edge",
		Name:      "data_transfer",
		StartTime: now,
		EndTime:   now.Add(10 * time.Millisecond),
		Status:    SpanStatusOK,
		Attributes: map[string]any{
			"petalflow.edge.source":       "node_a",
			"petalflow.edge.source_port":  "output",
			"petalflow.edge.target":       "node_b",
			"petalflow.edge.target_port":  "input",
			"petalflow.edge.data_size":    int64(1024),
			"petalflow.edge.data_preview": "preview of data...",
		},
	}

	span := c.ClassifySpan(in)

	if span.Kind != store.SpanKindEdge {
		t.Errorf("Kind = %s, want %s", span.Kind, store.SpanKindEdge)
	}

	if span.Edge == nil {
		t.Fatal("Edge data is nil")
	}

	edge := span.Edge
	if edge.SourceNode != "node_a" {
		t.Errorf("SourceNode = %s, want node_a", edge.SourceNode)
	}
	if edge.SourcePort != "output" {
		t.Errorf("SourcePort = %s, want output", edge.SourcePort)
	}
	if edge.TargetNode != "node_b" {
		t.Errorf("TargetNode = %s, want node_b", edge.TargetNode)
	}
	if edge.TargetPort != "input" {
		t.Errorf("TargetPort = %s, want input", edge.TargetPort)
	}
	if edge.DataSize != 1024 {
		t.Errorf("DataSize = %d, want 1024", edge.DataSize)
	}
	if edge.DataPreview != "preview of data..." {
		t.Errorf("DataPreview = %s", edge.DataPreview)
	}
}

func TestClassifier_CustomSpan(t *testing.T) {
	c := NewClassifier()

	now := time.Now()
	in := &IngestedSpan{
		TraceID:   "trace123",
		SpanID:    "span_custom",
		Name:      "custom_operation",
		StartTime: now,
		EndTime:   now.Add(50 * time.Millisecond),
		Status:    SpanStatusOK,
		Attributes: map[string]any{
			"custom.attr": "value",
		},
	}

	span := c.ClassifySpan(in)

	if span.Kind != store.SpanKindCustom {
		t.Errorf("Kind = %s, want %s", span.Kind, store.SpanKindCustom)
	}

	if span.Attributes["custom.attr"] != "value" {
		t.Errorf("custom.attr = %v, want value", span.Attributes["custom.attr"])
	}
}

func TestClassifier_ErrorSpan(t *testing.T) {
	c := NewClassifier()

	now := time.Now()
	in := &IngestedSpan{
		TraceID:   "trace123",
		SpanID:    "span_err",
		Name:      "failing_operation",
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Status:    SpanStatusError,
		StatusMsg: "Connection refused",
		Attributes: map[string]any{
			"error.code":    "CONN_REFUSED",
			"error.details": "Could not connect to upstream service",
		},
	}

	span := c.ClassifySpan(in)

	if span.Status != store.SpanStatusError {
		t.Errorf("Status = %s, want %s", span.Status, store.SpanStatusError)
	}

	if span.Error == nil {
		t.Fatal("Error is nil")
	}

	if span.Error.Message != "Connection refused" {
		t.Errorf("Error.Message = %s, want 'Connection refused'", span.Error.Message)
	}
	if span.Error.Code != "CONN_REFUSED" {
		t.Errorf("Error.Code = %s, want CONN_REFUSED", span.Error.Code)
	}
	if span.Error.Details != "Could not connect to upstream service" {
		t.Errorf("Error.Details = %s", span.Error.Details)
	}
}

func TestClassifier_KindDetectionByName(t *testing.T) {
	c := NewClassifier()

	tests := []struct {
		name     string
		spanName string
		wantKind store.SpanKind
	}{
		{"llm keyword", "chat completion", store.SpanKindLLM},
		{"llm keyword uppercase", "LLM call", store.SpanKindLLM},
		{"generate keyword", "generate response", store.SpanKindLLM},
		{"tool keyword", "tool execution", store.SpanKindTool},
		{"function call", "function_call handler", store.SpanKindTool},
		{"unrelated", "process data", store.SpanKindCustom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &IngestedSpan{
				Name:       tt.spanName,
				StartTime:  time.Now(),
				EndTime:    time.Now(),
				Attributes: map[string]any{},
			}

			span := c.ClassifySpan(in)
			if span.Kind != tt.wantKind {
				t.Errorf("Kind = %s, want %s", span.Kind, tt.wantKind)
			}
		})
	}
}

func TestClassifier_LLMMessages(t *testing.T) {
	c := NewClassifier()

	in := &IngestedSpan{
		Name:      "chat",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Attributes: map[string]any{
			"gen_ai.system":        "openai",
			"gen_ai.request.model": "gpt-4o",
			"gen_ai.messages": []any{
				map[string]any{
					"role":    "system",
					"content": "You are helpful.",
				},
				map[string]any{
					"role":    "user",
					"content": "Hello!",
				},
			},
		},
	}

	span := c.ClassifySpan(in)
	if span.LLM == nil {
		t.Fatal("LLM is nil")
	}

	if len(span.LLM.Messages) != 2 {
		t.Fatalf("Messages count = %d, want 2", len(span.LLM.Messages))
	}

	if span.LLM.Messages[0].Role != "system" {
		t.Errorf("Messages[0].Role = %s, want system", span.LLM.Messages[0].Role)
	}
	if span.LLM.Messages[1].Role != "user" {
		t.Errorf("Messages[1].Role = %s, want user", span.LLM.Messages[1].Role)
	}
}

func TestClassifier_ToolDefinitions(t *testing.T) {
	c := NewClassifier()

	in := &IngestedSpan{
		Name:      "chat_with_tools",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Attributes: map[string]any{
			"gen_ai.system":        "anthropic",
			"gen_ai.request.model": "claude-sonnet-4",
			"gen_ai.request.tools": []any{
				map[string]any{
					"name":        "calculator",
					"description": "Perform calculations",
					"input_schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"expression": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}

	span := c.ClassifySpan(in)
	if span.LLM == nil {
		t.Fatal("LLM is nil")
	}

	if len(span.LLM.ToolDefinitions) != 1 {
		t.Fatalf("ToolDefinitions count = %d, want 1", len(span.LLM.ToolDefinitions))
	}

	tool := span.LLM.ToolDefinitions[0]
	if tool.Name != "calculator" {
		t.Errorf("Name = %s, want calculator", tool.Name)
	}
	if tool.Description != "Perform calculations" {
		t.Errorf("Description = %s", tool.Description)
	}
	if len(tool.InputSchema) == 0 {
		t.Error("InputSchema is empty")
	}
}

func TestClassifier_PetalFlowToolNaming(t *testing.T) {
	c := NewClassifier()

	in := &IngestedSpan{
		Name:      "tool_exec",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Attributes: map[string]any{
			"petalflow.tool.name":   "database_query",
			"petalflow.tool.action": "select",
			"petalflow.tool.origin": "builtin",
			"petalflow.tool.inputs": map[string]any{
				"sql": "SELECT * FROM users",
			},
		},
	}

	span := c.ClassifySpan(in)

	if span.Kind != store.SpanKindTool {
		t.Errorf("Kind = %s, want %s", span.Kind, store.SpanKindTool)
	}

	if span.Tool.ToolName != "database_query" {
		t.Errorf("ToolName = %s, want database_query", span.Tool.ToolName)
	}
	if span.Tool.ActionName != "select" {
		t.Errorf("ActionName = %s, want select", span.Tool.ActionName)
	}
	if span.Tool.Origin != "builtin" {
		t.Errorf("Origin = %s, want builtin", span.Tool.Origin)
	}
}
