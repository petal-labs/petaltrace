package collector

import (
	"encoding/json"
	"strings"

	"github.com/petal-labs/petaltrace/store"
)

// TextExtractor extracts searchable text from spans for FTS indexing.
type TextExtractor struct {
	maxPromptSize     int
	maxCompletionSize int
}

// TextExtractorConfig configures the text extractor.
type TextExtractorConfig struct {
	MaxPromptSize     int
	MaxCompletionSize int
}

// ExtractedText holds text extracted from a span for FTS indexing.
type ExtractedText struct {
	SpanID     string
	RunID      string
	PromptText string
	Completion string
}

// NewTextExtractor creates a new text extractor.
func NewTextExtractor(cfg TextExtractorConfig) *TextExtractor {
	if cfg.MaxPromptSize <= 0 {
		cfg.MaxPromptSize = 100_000 // 100KB default
	}
	if cfg.MaxCompletionSize <= 0 {
		cfg.MaxCompletionSize = 100_000 // 100KB default
	}

	return &TextExtractor{
		maxPromptSize:     cfg.MaxPromptSize,
		maxCompletionSize: cfg.MaxCompletionSize,
	}
}

// Extract extracts searchable text from an LLM span.
func (e *TextExtractor) Extract(cs *CorrelatedSpan) *ExtractedText {
	if cs.Span.Kind != store.SpanKindLLM || cs.Span.LLM == nil {
		return nil
	}

	llm := cs.Span.LLM
	extracted := &ExtractedText{
		SpanID: cs.Span.ID,
		RunID:  cs.Span.RunID,
	}

	// Extract prompt text
	var promptParts []string

	// System prompt
	if llm.SystemPrompt != "" {
		promptParts = append(promptParts, llm.SystemPrompt)
	}

	// Messages
	for _, msg := range llm.Messages {
		text := e.extractMessageContent(msg.Content)
		if text != "" {
			promptParts = append(promptParts, text)
		}
	}

	extracted.PromptText = e.truncate(strings.Join(promptParts, "\n\n"), e.maxPromptSize)

	// Extract completion text
	if llm.Completion.TextContent != "" {
		extracted.Completion = e.truncate(llm.Completion.TextContent, e.maxCompletionSize)
	} else if len(llm.Completion.Content) > 0 {
		extracted.Completion = e.truncate(e.extractJSONContent(llm.Completion.Content), e.maxCompletionSize)
	}

	return extracted
}

// ExtractBatch processes multiple spans.
func (e *TextExtractor) ExtractBatch(spans []*CorrelatedSpan) []*ExtractedText {
	var results []*ExtractedText
	for _, cs := range spans {
		if extracted := e.Extract(cs); extracted != nil {
			results = append(results, extracted)
		}
	}
	return results
}

// extractMessageContent extracts text from a message content field.
func (e *TextExtractor) extractMessageContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	// Try as string first (simple text message)
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str
	}

	// Try as array of content blocks (Claude/OpenAI style)
	var blocks []map[string]any
	if err := json.Unmarshal(content, &blocks); err == nil {
		var parts []string
		for _, block := range blocks {
			if text := e.extractTextFromBlock(block); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}

	// Try as single object
	var obj map[string]any
	if err := json.Unmarshal(content, &obj); err == nil {
		return e.extractTextFromBlock(obj)
	}

	// Fallback: return raw JSON as string
	return string(content)
}

// extractTextFromBlock extracts text from a content block.
func (e *TextExtractor) extractTextFromBlock(block map[string]any) string {
	// Check block type
	blockType, _ := block["type"].(string)

	switch blockType {
	case "text":
		if text, ok := block["text"].(string); ok {
			return text
		}
	case "tool_use":
		// Extract tool name and input for searchability
		var parts []string
		if name, ok := block["name"].(string); ok {
			parts = append(parts, "Tool: "+name)
		}
		if input, ok := block["input"]; ok {
			if inputJSON, err := json.Marshal(input); err == nil {
				parts = append(parts, string(inputJSON))
			}
		}
		return strings.Join(parts, "\n")
	case "tool_result":
		if content, ok := block["content"].(string); ok {
			return content
		}
		if content, ok := block["content"]; ok {
			if contentJSON, err := json.Marshal(content); err == nil {
				return string(contentJSON)
			}
		}
	case "thinking":
		if thinking, ok := block["thinking"].(string); ok {
			return thinking
		}
	default:
		// Try common text fields
		for _, key := range []string{"text", "content", "value"} {
			if text, ok := block[key].(string); ok {
				return text
			}
		}
	}

	return ""
}

// extractJSONContent extracts text from a JSON content field.
func (e *TextExtractor) extractJSONContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	// Try as string
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str
	}

	// Try as array of content blocks
	var blocks []map[string]any
	if err := json.Unmarshal(content, &blocks); err == nil {
		var parts []string
		for _, block := range blocks {
			if text := e.extractTextFromBlock(block); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}

	// Return raw JSON
	return string(content)
}

// truncate limits text to maxLen characters.
func (e *TextExtractor) truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
