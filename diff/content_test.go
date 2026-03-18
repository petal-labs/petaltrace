package diff

import (
	"encoding/json"
	"testing"

	"github.com/petal-labs/petaltrace/store"
)

func TestContentDiffer_CompareText(t *testing.T) {
	d := NewContentDiffer()

	tests := []struct {
		name           string
		baseText       string
		compareText    string
		wantSimilarity float64
		wantHunks      bool
	}{
		{
			name:           "identical text",
			baseText:       "hello world",
			compareText:    "hello world",
			wantSimilarity: 1.0,
			wantHunks:      false,
		},
		{
			name:           "empty strings",
			baseText:       "",
			compareText:    "",
			wantSimilarity: 1.0,
			wantHunks:      false,
		},
		{
			name:           "completely different",
			baseText:       "abc",
			compareText:    "xyz",
			wantSimilarity: 0.0,
			wantHunks:      true,
		},
		{
			name:           "one empty",
			baseText:       "hello",
			compareText:    "",
			wantSimilarity: 0.0,
			wantHunks:      true,
		},
		{
			name:           "slight difference",
			baseText:       "hello world",
			compareText:    "hello worlds",
			wantSimilarity: 0.9, // Should be > 0.9
			wantHunks:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.CompareText(tt.baseText, tt.compareText)

			if result.BaseText != tt.baseText {
				t.Errorf("BaseText = %q, want %q", result.BaseText, tt.baseText)
			}

			if result.CompareText != tt.compareText {
				t.Errorf("CompareText = %q, want %q", result.CompareText, tt.compareText)
			}

			// Check similarity is in expected range
			if tt.wantSimilarity == 1.0 && result.Similarity != 1.0 {
				t.Errorf("Similarity = %f, want exactly 1.0", result.Similarity)
			} else if tt.wantSimilarity == 0.0 && result.Similarity != 0.0 {
				t.Errorf("Similarity = %f, want exactly 0.0", result.Similarity)
			} else if tt.wantSimilarity > 0 && tt.wantSimilarity < 1 {
				if result.Similarity < tt.wantSimilarity {
					t.Errorf("Similarity = %f, want >= %f", result.Similarity, tt.wantSimilarity)
				}
			}

			// Check hunks presence
			hasHunks := len(result.Hunks) > 0
			if hasHunks != tt.wantHunks {
				t.Errorf("has hunks = %v, want %v", hasHunks, tt.wantHunks)
			}
		})
	}
}

func TestContentDiffer_SetContextLines(t *testing.T) {
	d := NewContentDiffer()

	if d.contextLines != 3 {
		t.Errorf("default contextLines = %d, want 3", d.contextLines)
	}

	d.SetContextLines(5)

	if d.contextLines != 5 {
		t.Errorf("contextLines after set = %d, want 5", d.contextLines)
	}
}

func TestComputeSimilarity(t *testing.T) {
	d := NewContentDiffer()

	tests := []struct {
		name string
		a    string
		b    string
		want float64
	}{
		{"identical", "hello", "hello", 1.0},
		{"both empty", "", "", 1.0},
		{"one empty", "hello", "", 0.0},
		{"other empty", "", "hello", 0.0},
		{"completely different", "abc", "xyz", 0.0},
		{"one char diff", "hello", "hallo", 0.8}, // 1 edit in 5 chars = 0.8
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.ComputeSimilarity(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ComputeSimilarity(%q, %q) = %f, want %f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"", "", 0},
		{"hello", "", 5},
		{"", "world", 5},
		{"hello", "hello", 0},
		{"hello", "hallo", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLongestCommonSubsequence(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want []string
	}{
		{
			name: "empty slices",
			a:    []string{},
			b:    []string{},
			want: []string{},
		},
		{
			name: "identical",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "no common",
			a:    []string{"a", "b"},
			b:    []string{"c", "d"},
			want: []string{},
		},
		{
			name: "partial overlap",
			a:    []string{"a", "b", "c", "d"},
			b:    []string{"a", "c", "d"},
			want: []string{"a", "c", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := longestCommonSubsequence(tt.a, tt.b)
			if len(got) != len(tt.want) {
				t.Errorf("LCS length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("LCS[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestComparePrompts(t *testing.T) {
	d := NewContentDiffer()

	// Test with nil spans
	result := d.ComparePrompts(nil, nil)
	if result.Similarity != 1.0 {
		t.Errorf("nil spans should have similarity 1.0, got %f", result.Similarity)
	}

	// Test with spans that have LLM data
	baseSpan := &store.Span{
		LLM: &store.LLMSpanData{
			SystemPrompt: "You are a helpful assistant",
			Messages: []store.LLMMessage{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
			},
		},
	}

	compareSpan := &store.Span{
		LLM: &store.LLMSpanData{
			SystemPrompt: "You are a helpful assistant",
			Messages: []store.LLMMessage{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
			},
		},
	}

	result = d.ComparePrompts(baseSpan, compareSpan)
	if result.Similarity != 1.0 {
		t.Errorf("identical prompts should have similarity 1.0, got %f", result.Similarity)
	}
}

func TestCompareOutputs(t *testing.T) {
	d := NewContentDiffer()

	// Test with nil spans
	result := d.CompareOutputs(nil, nil)
	if result.Similarity != 1.0 {
		t.Errorf("nil spans should have similarity 1.0, got %f", result.Similarity)
	}

	// Test with spans that have completion data
	baseSpan := &store.Span{
		LLM: &store.LLMSpanData{
			Completion: store.LLMCompletion{
				TextContent: "Hello, how can I help you?",
			},
		},
	}

	compareSpan := &store.Span{
		LLM: &store.LLMSpanData{
			Completion: store.LLMCompletion{
				TextContent: "Hello, how can I assist you?",
			},
		},
	}

	result = d.CompareOutputs(baseSpan, compareSpan)
	if result.Similarity == 1.0 {
		t.Error("different outputs should not have similarity 1.0")
	}
	if result.Similarity < 0.7 {
		t.Errorf("similar outputs should have similarity > 0.7, got %f", result.Similarity)
	}
}

func TestExtractPromptText(t *testing.T) {
	tests := []struct {
		name string
		span *store.Span
		want string
	}{
		{
			name: "nil span",
			span: nil,
			want: "",
		},
		{
			name: "span without LLM",
			span: &store.Span{},
			want: "",
		},
		{
			name: "span with system prompt only",
			span: &store.Span{
				LLM: &store.LLMSpanData{
					SystemPrompt: "You are a helper",
				},
			},
			want: "[System]\nYou are a helper",
		},
		{
			name: "span with messages",
			span: &store.Span{
				LLM: &store.LLMSpanData{
					Messages: []store.LLMMessage{
						{Role: "user", Content: json.RawMessage(`"Hello"`)},
					},
				},
			},
			want: "[user]\nHello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPromptText(tt.span)
			if got != tt.want {
				t.Errorf("extractPromptText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractOutputText(t *testing.T) {
	tests := []struct {
		name string
		span *store.Span
		want string
	}{
		{
			name: "nil span",
			span: nil,
			want: "",
		},
		{
			name: "span without LLM",
			span: &store.Span{},
			want: "",
		},
		{
			name: "span with completion",
			span: &store.Span{
				LLM: &store.LLMSpanData{
					Completion: store.LLMCompletion{
						TextContent: "Here is my response",
					},
				},
			},
			want: "Here is my response",
		},
		{
			name: "span with empty completion",
			span: &store.Span{
				LLM: &store.LLMSpanData{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOutputText(tt.span)
			if got != tt.want {
				t.Errorf("extractOutputText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateHunks(t *testing.T) {
	d := NewContentDiffer()

	tests := []struct {
		name        string
		baseText    string
		compareText string
		wantHunks   int
	}{
		{
			name:        "identical",
			baseText:    "line1\nline2",
			compareText: "line1\nline2",
			wantHunks:   0,
		},
		{
			name:        "line added",
			baseText:    "line1\nline2",
			compareText: "line1\nline2\nline3",
			wantHunks:   1,
		},
		{
			name:        "line removed",
			baseText:    "line1\nline2\nline3",
			compareText: "line1\nline2",
			wantHunks:   1,
		},
		{
			name:        "line changed",
			baseText:    "line1\noriginal\nline3",
			compareText: "line1\nmodified\nline3",
			wantHunks:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunks := d.generateHunks(tt.baseText, tt.compareText)
			if len(hunks) != tt.wantHunks {
				t.Errorf("generateHunks() returned %d hunks, want %d", len(hunks), tt.wantHunks)
			}
		})
	}
}
