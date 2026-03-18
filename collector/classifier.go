package collector

import (
	"encoding/json"
	"strings"

	"github.com/petal-labs/petaltrace/store"
)

// Classifier determines span kind and extracts kind-specific payloads.
type Classifier struct{}

// NewClassifier creates a new span classifier.
func NewClassifier() *Classifier {
	return &Classifier{}
}

// ClassifySpan determines the kind and extracts typed payload from an ingested span.
func (c *Classifier) ClassifySpan(in *IngestedSpan) *store.Span {
	span := &store.Span{
		TraceID:    in.TraceID,
		Name:       in.Name,
		StartedAt:  in.StartTime,
		Attributes: in.Attributes,
	}

	if !in.EndTime.IsZero() {
		span.CompletedAt = &in.EndTime
		span.DurationMs = in.EndTime.Sub(in.StartTime).Milliseconds()
	}

	span.Status = c.convertStatus(in.Status, in.StatusMsg)
	if in.Status == SpanStatusError {
		span.Error = c.extractError(in)
	}

	kind := c.determineKind(in)
	span.Kind = kind

	switch kind {
	case store.SpanKindLLM:
		span.LLM = c.extractLLMData(in)
	case store.SpanKindNode:
		span.Node = c.extractNodeData(in)
	case store.SpanKindTool:
		span.Tool = c.extractToolData(in)
	case store.SpanKindEdge:
		span.Edge = c.extractEdgeData(in)
	}

	return span
}

// determineKind inspects attributes to classify the span.
func (c *Classifier) determineKind(in *IngestedSpan) store.SpanKind {
	attrs := in.Attributes

	// Check for GenAI semantic conventions (LLM spans)
	if hasAttr(attrs, "gen_ai.system") || hasAttr(attrs, "gen_ai.request.model") {
		return store.SpanKindLLM
	}

	// Check for PetalFlow node spans
	if hasAttr(attrs, "petalflow.node.id") || hasAttr(attrs, "petalflow.node.type") {
		// If it's a node that also has LLM attributes, check for nested structure
		if hasAttr(attrs, "petalflow.edge.source") {
			return store.SpanKindEdge
		}
		return store.SpanKindNode
	}

	// Check for tool invocations
	if hasAttr(attrs, "tool.name") || hasAttr(attrs, "petalflow.tool.name") {
		return store.SpanKindTool
	}

	// Check for edge spans
	if hasAttr(attrs, "petalflow.edge.source") || hasAttr(attrs, "petalflow.edge.target") {
		return store.SpanKindEdge
	}

	// Fallback: check span name patterns
	name := strings.ToLower(in.Name)
	if strings.Contains(name, "llm") || strings.Contains(name, "completion") ||
		strings.Contains(name, "chat") || strings.Contains(name, "generate") {
		return store.SpanKindLLM
	}

	if strings.Contains(name, "tool") || strings.Contains(name, "function_call") {
		return store.SpanKindTool
	}

	return store.SpanKindCustom
}

func (c *Classifier) convertStatus(status SpanStatusHint, msg string) store.SpanStatus {
	switch status {
	case SpanStatusError:
		return store.SpanStatusError
	default:
		return store.SpanStatusOK
	}
}

func (c *Classifier) extractError(in *IngestedSpan) *store.SpanError {
	err := &store.SpanError{
		Message: in.StatusMsg,
	}

	if code, ok := getStringAttr(in.Attributes, "error.code"); ok {
		err.Code = code
	}
	if details, ok := getStringAttr(in.Attributes, "error.details"); ok {
		err.Details = details
	}
	if err.Message == "" {
		if msg, ok := getStringAttr(in.Attributes, "error.message"); ok {
			err.Message = msg
		}
	}

	return err
}

func (c *Classifier) extractLLMData(in *IngestedSpan) *store.LLMSpanData {
	attrs := in.Attributes
	llm := &store.LLMSpanData{}

	// Provider and model
	if v, ok := getStringAttr(attrs, "gen_ai.system"); ok {
		llm.Provider = v
	}
	if v, ok := getStringAttr(attrs, "gen_ai.request.model"); ok {
		llm.Model = v
	}

	// Request parameters
	if v, ok := getFloat64Attr(attrs, "gen_ai.request.temperature"); ok {
		llm.Temperature = &v
	}
	if v, ok := getIntAttr(attrs, "gen_ai.request.max_tokens"); ok {
		llm.MaxTokens = &v
	}

	// Prompts and messages
	if v, ok := getStringAttr(attrs, "gen_ai.prompt"); ok {
		llm.SystemPrompt = v
	}
	if v, ok := getStringAttr(attrs, "gen_ai.system_prompt"); ok {
		llm.SystemPrompt = v
	}

	// Messages array
	if msgs, ok := attrs["gen_ai.messages"]; ok {
		llm.Messages = c.parseMessages(msgs)
	}

	// Completion
	if v, ok := getStringAttr(attrs, "gen_ai.completion"); ok {
		llm.Completion.TextContent = v
		llm.Completion.Content, _ = json.Marshal(v)
	}

	// Token usage
	if v, ok := getIntAttr(attrs, "gen_ai.usage.input_tokens"); ok {
		llm.Tokens.InputTokens = v
	}
	if v, ok := getIntAttr(attrs, "gen_ai.usage.output_tokens"); ok {
		llm.Tokens.OutputTokens = v
	}
	if v, ok := getIntAttr(attrs, "gen_ai.usage.total_tokens"); ok {
		llm.Tokens.TotalTokens = v
	} else {
		llm.Tokens.TotalTokens = llm.Tokens.InputTokens + llm.Tokens.OutputTokens
	}

	// Cache tokens
	if v, ok := getIntAttr(attrs, "gen_ai.usage.cache_read_input_tokens"); ok {
		llm.CacheRead = &v
	}
	if v, ok := getIntAttr(attrs, "gen_ai.usage.cache_creation_input_tokens"); ok {
		llm.CacheCreation = &v
	}

	// Response metadata
	if v, ok := getStringAttr(attrs, "gen_ai.response.id"); ok {
		llm.RequestID = v
	}
	if v, ok := getStringAttr(attrs, "gen_ai.response.finish_reasons"); ok {
		llm.StopReason = v
	}
	if v, ok := getStringAttr(attrs, "gen_ai.stop_reason"); ok {
		llm.StopReason = v
	}

	// Latency
	if v, ok := getIntAttr(attrs, "gen_ai.latency.time_to_first_token_ms"); ok {
		v64 := int64(v)
		llm.TimeToFirstToken = &v64
	}
	llm.TotalLatency = in.EndTime.Sub(in.StartTime).Milliseconds()

	// Tool definitions
	if tools, ok := attrs["gen_ai.request.tools"]; ok {
		llm.ToolDefinitions = c.parseToolDefinitions(tools)
	}

	return llm
}

func (c *Classifier) parseMessages(v any) []store.LLMMessage {
	var messages []store.LLMMessage

	switch arr := v.(type) {
	case []any:
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				msg := store.LLMMessage{}
				if role, ok := m["role"].(string); ok {
					msg.Role = role
				}
				if content, ok := m["content"]; ok {
					msg.Content, _ = json.Marshal(content)
				}
				messages = append(messages, msg)
			}
		}
	case string:
		// Single message as string
		messages = append(messages, store.LLMMessage{
			Role:    "user",
			Content: json.RawMessage(`"` + arr + `"`),
		})
	}

	return messages
}

func (c *Classifier) parseToolDefinitions(v any) []store.ToolDefinition {
	var tools []store.ToolDefinition

	arr, ok := v.([]any)
	if !ok {
		return tools
	}

	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			tool := store.ToolDefinition{}
			if name, ok := m["name"].(string); ok {
				tool.Name = name
			}
			if desc, ok := m["description"].(string); ok {
				tool.Description = desc
			}
			if schema, ok := m["input_schema"]; ok {
				tool.InputSchema, _ = json.Marshal(schema)
			}
			tools = append(tools, tool)
		}
	}

	return tools
}

func (c *Classifier) extractNodeData(in *IngestedSpan) *store.NodeSpanData {
	attrs := in.Attributes
	node := &store.NodeSpanData{}

	if v, ok := getStringAttr(attrs, "petalflow.node.id"); ok {
		node.NodeID = v
	}
	if v, ok := getStringAttr(attrs, "petalflow.node.type"); ok {
		node.NodeType = v
	}
	if v, ok := getIntAttr(attrs, "petalflow.node.retry_count"); ok {
		node.RetryCount = v
	}

	// Inputs/outputs/config as JSON
	if v, ok := attrs["petalflow.node.inputs"]; ok {
		node.Inputs, _ = json.Marshal(v)
	}
	if v, ok := attrs["petalflow.node.outputs"]; ok {
		node.Outputs, _ = json.Marshal(v)
	}
	if v, ok := attrs["petalflow.node.config"]; ok {
		node.Config, _ = json.Marshal(v)
	}

	return node
}

func (c *Classifier) extractToolData(in *IngestedSpan) *store.ToolSpanData {
	attrs := in.Attributes
	tool := &store.ToolSpanData{
		DurationMs: in.EndTime.Sub(in.StartTime).Milliseconds(),
	}

	// Try both naming conventions
	if v, ok := getStringAttr(attrs, "tool.name"); ok {
		tool.ToolName = v
	} else if v, ok := getStringAttr(attrs, "petalflow.tool.name"); ok {
		tool.ToolName = v
	}

	if v, ok := getStringAttr(attrs, "tool.action"); ok {
		tool.ActionName = v
	} else if v, ok := getStringAttr(attrs, "petalflow.tool.action"); ok {
		tool.ActionName = v
	}

	if v, ok := getStringAttr(attrs, "tool.origin"); ok {
		tool.Origin = v
	} else if v, ok := getStringAttr(attrs, "petalflow.tool.origin"); ok {
		tool.Origin = v
	}

	// Inputs/outputs
	if v, ok := attrs["tool.inputs"]; ok {
		tool.Inputs, _ = json.Marshal(v)
	} else if v, ok := attrs["petalflow.tool.inputs"]; ok {
		tool.Inputs, _ = json.Marshal(v)
	}

	if v, ok := attrs["tool.outputs"]; ok {
		tool.Outputs, _ = json.Marshal(v)
	} else if v, ok := attrs["petalflow.tool.outputs"]; ok {
		tool.Outputs, _ = json.Marshal(v)
	}

	// Invocation context
	if v, ok := getStringAttr(attrs, "tool.invoked_by"); ok {
		tool.InvokedBy = &v
	}
	if v, ok := getStringAttr(attrs, "gen_ai.tool_use.id"); ok {
		tool.ToolUseID = &v
	}

	return tool
}

func (c *Classifier) extractEdgeData(in *IngestedSpan) *store.EdgeSpanData {
	attrs := in.Attributes
	edge := &store.EdgeSpanData{}

	if v, ok := getStringAttr(attrs, "petalflow.edge.source"); ok {
		edge.SourceNode = v
	}
	if v, ok := getStringAttr(attrs, "petalflow.edge.source_port"); ok {
		edge.SourcePort = v
	}
	if v, ok := getStringAttr(attrs, "petalflow.edge.target"); ok {
		edge.TargetNode = v
	}
	if v, ok := getStringAttr(attrs, "petalflow.edge.target_port"); ok {
		edge.TargetPort = v
	}
	if v, ok := getIntAttr(attrs, "petalflow.edge.data_size"); ok {
		edge.DataSize = int64(v)
	}
	if v, ok := getStringAttr(attrs, "petalflow.edge.data_preview"); ok {
		edge.DataPreview = v
	}

	return edge
}

// Helper functions

func hasAttr(attrs map[string]any, key string) bool {
	_, ok := attrs[key]
	return ok
}

func getStringAttr(attrs map[string]any, key string) (string, bool) {
	v, ok := attrs[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func getIntAttr(attrs map[string]any, key string) (int, bool) {
	v, ok := attrs[key]
	if !ok {
		return 0, false
	}

	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func getFloat64Attr(attrs map[string]any, key string) (float64, bool) {
	v, ok := attrs[key]
	if !ok {
		return 0, false
	}

	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
