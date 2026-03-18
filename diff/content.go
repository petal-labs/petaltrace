package diff

import (
	"strings"

	"github.com/petal-labs/petaltrace/store"
)

// ContentDiffer compares text content between spans
type ContentDiffer struct {
	contextLines int
}

// NewContentDiffer creates a new content differ
func NewContentDiffer() *ContentDiffer {
	return &ContentDiffer{
		contextLines: 3,
	}
}

// SetContextLines sets the number of context lines for diff hunks
func (d *ContentDiffer) SetContextLines(n int) {
	d.contextLines = n
}

// CompareText computes a unified diff between two text strings
func (d *ContentDiffer) CompareText(baseText, compareText string) *store.TextDiff {
	diff := &store.TextDiff{
		BaseText:    baseText,
		CompareText: compareText,
		Hunks:       make([]store.Hunk, 0),
		Similarity:  d.ComputeSimilarity(baseText, compareText),
	}

	if baseText == compareText {
		return diff
	}

	// Generate unified diff hunks
	diff.Hunks = d.generateHunks(baseText, compareText)

	return diff
}

// ComputeSimilarity calculates the similarity ratio between two strings (0.0 to 1.0)
func (d *ContentDiffer) ComputeSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// Use Levenshtein distance ratio for similarity
	distance := levenshteinDistance(a, b)
	maxLen := max(len(a), len(b))
	return 1.0 - float64(distance)/float64(maxLen)
}

// generateHunks creates diff hunks showing differences between texts
func (d *ContentDiffer) generateHunks(baseText, compareText string) []store.Hunk {
	baseLines := strings.Split(baseText, "\n")
	compareLines := strings.Split(compareText, "\n")

	// Simple line-by-line diff using LCS
	lcs := longestCommonSubsequence(baseLines, compareLines)

	var hunks []store.Hunk
	var currentHunk *store.Hunk
	var hunkContent strings.Builder

	baseIdx := 0
	compareIdx := 0
	lcsIdx := 0

	for baseIdx < len(baseLines) || compareIdx < len(compareLines) {
		// Check if current lines match LCS
		baseMatches := baseIdx < len(baseLines) && lcsIdx < len(lcs) && baseLines[baseIdx] == lcs[lcsIdx]
		compareMatches := compareIdx < len(compareLines) && lcsIdx < len(lcs) && compareLines[compareIdx] == lcs[lcsIdx]

		if baseMatches && compareMatches {
			// Lines match - context
			if currentHunk != nil {
				hunkContent.WriteString("  " + baseLines[baseIdx] + "\n")
			}
			baseIdx++
			compareIdx++
			lcsIdx++
		} else if !baseMatches && baseIdx < len(baseLines) {
			// Line removed from base
			if currentHunk == nil {
				currentHunk = &store.Hunk{StartLine: baseIdx + 1}
				hunkContent.Reset()
			}
			hunkContent.WriteString("- " + baseLines[baseIdx] + "\n")
			baseIdx++
		} else if !compareMatches && compareIdx < len(compareLines) {
			// Line added in compare
			if currentHunk == nil {
				currentHunk = &store.Hunk{StartLine: compareIdx + 1}
				hunkContent.Reset()
			}
			hunkContent.WriteString("+ " + compareLines[compareIdx] + "\n")
			compareIdx++
		} else {
			// Edge case - advance both
			baseIdx++
			compareIdx++
		}

		// Close hunk if we've moved past changes
		if currentHunk != nil && baseMatches && compareMatches {
			currentHunk.EndLine = baseIdx
			currentHunk.Content = hunkContent.String()
			hunks = append(hunks, *currentHunk)
			currentHunk = nil
		}
	}

	// Close final hunk
	if currentHunk != nil {
		currentHunk.EndLine = max(baseIdx, compareIdx)
		currentHunk.Content = hunkContent.String()
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

// ComparePrompts compares LLM prompts between two spans
func (d *ContentDiffer) ComparePrompts(baseSpan, compareSpan *store.Span) *store.TextDiff {
	basePrompt := extractPromptText(baseSpan)
	comparePrompt := extractPromptText(compareSpan)
	return d.CompareText(basePrompt, comparePrompt)
}

// CompareOutputs compares LLM outputs between two spans
func (d *ContentDiffer) CompareOutputs(baseSpan, compareSpan *store.Span) *store.TextDiff {
	baseOutput := extractOutputText(baseSpan)
	compareOutput := extractOutputText(compareSpan)
	return d.CompareText(baseOutput, compareOutput)
}

// extractPromptText extracts the prompt text from a span
func extractPromptText(span *store.Span) string {
	if span == nil || span.LLM == nil {
		return ""
	}

	var parts []string

	// System prompt
	if span.LLM.SystemPrompt != "" {
		parts = append(parts, "[System]\n"+span.LLM.SystemPrompt)
	}

	// Messages
	for _, msg := range span.LLM.Messages {
		parts = append(parts, "["+msg.Role+"]\n"+extractMessageContent(msg))
	}

	return strings.Join(parts, "\n\n")
}

// extractOutputText extracts the completion text from a span
func extractOutputText(span *store.Span) string {
	if span == nil || span.LLM == nil {
		return ""
	}

	if span.LLM.Completion.TextContent != "" {
		return span.LLM.Completion.TextContent
	}

	return ""
}

// extractMessageContent extracts text content from a message
func extractMessageContent(msg store.LLMMessage) string {
	// If Content is a string, return it
	if len(msg.Content) > 0 {
		// Try to extract text from JSON
		content := string(msg.Content)
		// Simple case: raw string
		if strings.HasPrefix(content, "\"") {
			return strings.Trim(content, "\"")
		}
		return content
	}
	return ""
}

// levenshteinDistance computes the edit distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use runes for proper unicode handling
	aRunes := []rune(a)
	bRunes := []rune(b)

	// Create distance matrix
	rows := len(aRunes) + 1
	cols := len(bRunes) + 1
	dist := make([][]int, rows)
	for i := range dist {
		dist[i] = make([]int, cols)
	}

	// Initialize first row and column
	for i := 0; i < rows; i++ {
		dist[i][0] = i
	}
	for j := 0; j < cols; j++ {
		dist[0][j] = j
	}

	// Fill in the rest
	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			cost := 0
			if aRunes[i-1] != bRunes[j-1] {
				cost = 1
			}
			dist[i][j] = min(
				dist[i-1][j]+1,      // deletion
				dist[i][j-1]+1,      // insertion
				dist[i-1][j-1]+cost, // substitution
			)
		}
	}

	return dist[rows-1][cols-1]
}

// longestCommonSubsequence finds the LCS of two string slices
func longestCommonSubsequence(a, b []string) []string {
	m := len(a)
	n := len(b)

	// Create LCS length matrix
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// Fill the matrix
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	// Backtrack to find the LCS
	lcs := make([]string, dp[m][n])
	i, j := m, n
	idx := dp[m][n] - 1
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			lcs[idx] = a[i-1]
			i--
			j--
			idx--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return lcs
}
