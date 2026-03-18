package collector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func TestTextExtractor_SimpleText(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-1",
			RunID:     "run-1",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				SystemPrompt: "You are a helpful assistant.",
				Messages: []store.LLMMessage{
					{
						Role:    "user",
						Content: json.RawMessage(`"Hello, how are you?"`),
					},
				},
				Completion: store.LLMCompletion{
					TextContent: "I'm doing well, thank you for asking!",
				},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	if extracted.SpanID != "span-1" {
		t.Errorf("SpanID = %s, want span-1", extracted.SpanID)
	}
	if extracted.RunID != "run-1" {
		t.Errorf("RunID = %s, want run-1", extracted.RunID)
	}

	if !strings.Contains(extracted.PromptText, "You are a helpful assistant.") {
		t.Errorf("PromptText missing system prompt: %s", extracted.PromptText)
	}
	if !strings.Contains(extracted.PromptText, "Hello, how are you?") {
		t.Errorf("PromptText missing user message: %s", extracted.PromptText)
	}

	if extracted.Completion != "I'm doing well, thank you for asking!" {
		t.Errorf("Completion = %s", extracted.Completion)
	}
}

func TestTextExtractor_ContentBlocks(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-2",
			RunID:     "run-2",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Messages: []store.LLMMessage{
					{
						Role: "user",
						Content: json.RawMessage(`[
							{"type": "text", "text": "What's in this image?"},
							{"type": "image", "url": "https://example.com/image.png"}
						]`),
					},
				},
				Completion: store.LLMCompletion{
					Content: json.RawMessage(`[
						{"type": "text", "text": "I can see a cat in the image."}
					]`),
				},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	if !strings.Contains(extracted.PromptText, "What's in this image?") {
		t.Errorf("PromptText missing text block: %s", extracted.PromptText)
	}

	if !strings.Contains(extracted.Completion, "I can see a cat in the image.") {
		t.Errorf("Completion missing text block: %s", extracted.Completion)
	}
}

func TestTextExtractor_ToolUse(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-3",
			RunID:     "run-3",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Messages: []store.LLMMessage{
					{
						Role:    "user",
						Content: json.RawMessage(`"Search for weather in London"`),
					},
				},
				Completion: store.LLMCompletion{
					Content: json.RawMessage(`[
						{"type": "tool_use", "name": "web_search", "input": {"query": "weather London"}}
					]`),
				},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	if !strings.Contains(extracted.Completion, "web_search") {
		t.Errorf("Completion missing tool name: %s", extracted.Completion)
	}
	if !strings.Contains(extracted.Completion, "weather London") {
		t.Errorf("Completion missing tool input: %s", extracted.Completion)
	}
}

func TestTextExtractor_ToolResult(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-4",
			RunID:     "run-4",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Messages: []store.LLMMessage{
					{
						Role: "user",
						Content: json.RawMessage(`[
							{"type": "tool_result", "tool_use_id": "tool_123", "content": "The weather in London is 15°C and cloudy."}
						]`),
					},
				},
				Completion: store.LLMCompletion{
					TextContent: "Based on the search results, London is currently experiencing mild weather at 15°C with clouds.",
				},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	if !strings.Contains(extracted.PromptText, "15°C and cloudy") {
		t.Errorf("PromptText missing tool result: %s", extracted.PromptText)
	}
}

func TestTextExtractor_ThinkingBlock(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-5",
			RunID:     "run-5",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				Messages: []store.LLMMessage{
					{
						Role:    "user",
						Content: json.RawMessage(`"Solve 2+2"`),
					},
				},
				Completion: store.LLMCompletion{
					Content: json.RawMessage(`[
						{"type": "thinking", "thinking": "Let me calculate this simple arithmetic..."},
						{"type": "text", "text": "The answer is 4."}
					]`),
				},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	if !strings.Contains(extracted.Completion, "simple arithmetic") {
		t.Errorf("Completion missing thinking block: %s", extracted.Completion)
	}
	if !strings.Contains(extracted.Completion, "answer is 4") {
		t.Errorf("Completion missing text block: %s", extracted.Completion)
	}
}

func TestTextExtractor_Truncation(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{
		MaxPromptSize:     100,
		MaxCompletionSize: 50,
	})

	longText := strings.Repeat("a", 200)

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-6",
			RunID:     "run-6",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				SystemPrompt: longText,
				Completion: store.LLMCompletion{
					TextContent: longText,
				},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	if len(extracted.PromptText) > 100 {
		t.Errorf("PromptText not truncated: len=%d", len(extracted.PromptText))
	}
	if len(extracted.Completion) > 50 {
		t.Errorf("Completion not truncated: len=%d", len(extracted.Completion))
	}
}

func TestTextExtractor_NonLLMSpan(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-7",
			RunID:     "run-7",
			Kind:      store.SpanKindNode,
			StartedAt: time.Now(),
		},
	}

	extracted := extractor.Extract(cs)
	if extracted != nil {
		t.Error("expected nil for non-LLM span")
	}
}

func TestTextExtractor_EmptyLLMData(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-8",
			RunID:     "run-8",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM:       nil,
		},
	}

	extracted := extractor.Extract(cs)
	if extracted != nil {
		t.Error("expected nil for nil LLM data")
	}
}

func TestTextExtractor_Batch(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	spans := []*CorrelatedSpan{
		{
			Span: &store.Span{
				ID:        "span-1",
				RunID:     "run-1",
				Kind:      store.SpanKindLLM,
				StartedAt: time.Now(),
				LLM: &store.LLMSpanData{
					SystemPrompt: "First prompt",
					Completion:   store.LLMCompletion{TextContent: "First completion"},
				},
			},
		},
		{
			Span: &store.Span{
				ID:        "span-2",
				RunID:     "run-1",
				Kind:      store.SpanKindNode, // Not LLM
				StartedAt: time.Now(),
			},
		},
		{
			Span: &store.Span{
				ID:        "span-3",
				RunID:     "run-1",
				Kind:      store.SpanKindLLM,
				StartedAt: time.Now(),
				LLM: &store.LLMSpanData{
					SystemPrompt: "Second prompt",
					Completion:   store.LLMCompletion{TextContent: "Second completion"},
				},
			},
		},
	}

	results := extractor.ExtractBatch(spans)
	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}

	if results[0].SpanID != "span-1" {
		t.Errorf("results[0].SpanID = %s, want span-1", results[0].SpanID)
	}
	if results[1].SpanID != "span-3" {
		t.Errorf("results[1].SpanID = %s, want span-3", results[1].SpanID)
	}
}

func TestTextExtractor_MultipleMessages(t *testing.T) {
	extractor := NewTextExtractor(TextExtractorConfig{})

	cs := &CorrelatedSpan{
		Span: &store.Span{
			ID:        "span-10",
			RunID:     "run-10",
			Kind:      store.SpanKindLLM,
			StartedAt: time.Now(),
			LLM: &store.LLMSpanData{
				SystemPrompt: "You are a math tutor.",
				Messages: []store.LLMMessage{
					{Role: "user", Content: json.RawMessage(`"What is 2+2?"`)},
					{Role: "assistant", Content: json.RawMessage(`"2+2 equals 4."`)},
					{Role: "user", Content: json.RawMessage(`"What about 3+3?"`)},
				},
				Completion: store.LLMCompletion{TextContent: "3+3 equals 6."},
			},
		},
	}

	extracted := extractor.Extract(cs)
	if extracted == nil {
		t.Fatal("extracted is nil")
	}

	expectedPhrases := []string{
		"You are a math tutor.",
		"What is 2+2?",
		"2+2 equals 4.",
		"What about 3+3?",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(extracted.PromptText, phrase) {
			t.Errorf("PromptText missing: %s\nGot: %s", phrase, extracted.PromptText)
		}
	}
}
