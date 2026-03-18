package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/store"
)

var promptCmd = &cobra.Command{
	Use:   "prompt <run-id> <node-id>",
	Short: "Display full prompt and completion for an LLM node",
	Long: `Display the complete prompt (system prompt, messages, tool definitions)
and optionally the completion for an LLM node within a run.

Examples:
  petaltrace prompt run-abc123 node-xyz
  petaltrace prompt run-abc123 node-xyz --completion
  petaltrace prompt run-abc123 node-xyz --format json
  petaltrace prompt run-abc123 node-xyz --format curl`,
	Args: cobra.ExactArgs(2),
	RunE: runPrompt,
}

var (
	promptCompletion bool
	promptFormat     string
)

func init() {
	rootCmd.AddCommand(promptCmd)

	promptCmd.Flags().BoolVarP(&promptCompletion, "completion", "c", false, "Include the completion/response")
	promptCmd.Flags().StringVarP(&promptFormat, "format", "f", "text", "Output format: text, json, curl, sdk")
}

func runPrompt(cmd *cobra.Command, args []string) error {
	runID := args[0]
	nodeID := args[1]

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	// Find LLM span for this node
	spans, err := traceStore.GetSpansByKind(ctx, runID, store.SpanKindLLM)
	if err != nil {
		return fmt.Errorf("getting LLM spans: %w", err)
	}

	var targetSpan *store.Span
	for _, s := range spans {
		if s.Node != nil && s.Node.NodeID == nodeID {
			targetSpan = s
			break
		}
		// Also check if span name contains the node ID
		if strings.Contains(s.Name, nodeID) {
			targetSpan = s
			break
		}
	}

	// If no node match, try to find by span ID or name
	if targetSpan == nil {
		for _, s := range spans {
			if s.ID == nodeID || s.Name == nodeID {
				targetSpan = s
				break
			}
		}
	}

	if targetSpan == nil {
		return fmt.Errorf("no LLM span found for node %s in run %s", nodeID, runID)
	}

	if targetSpan.LLM == nil {
		return fmt.Errorf("span %s has no LLM data", targetSpan.ID)
	}

	switch promptFormat {
	case "json":
		return outputPromptJSON(targetSpan)
	case "curl":
		return outputPromptCurl(targetSpan)
	case "sdk":
		return outputPromptSDK(targetSpan)
	default:
		return outputPromptText(targetSpan)
	}
}

// ContentBlock represents a content block in messages
type ContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ToolName  string `json:"name,omitempty"`
	ToolInput any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	ID        string `json:"id,omitempty"`
}

func outputPromptText(span *store.Span) error {
	llm := span.LLM

	fmt.Printf("=== Prompt for %s ===\n\n", span.Name)
	fmt.Printf("Provider: %s\n", llm.Provider)
	fmt.Printf("Model: %s\n", llm.Model)
	fmt.Println()

	// System prompt
	if llm.SystemPrompt != "" {
		fmt.Println("--- System Prompt ---")
		fmt.Println(llm.SystemPrompt)
		fmt.Println()
	}

	// Messages
	if len(llm.Messages) > 0 {
		fmt.Println("--- Messages ---")
		for i, msg := range llm.Messages {
			fmt.Printf("\n[%d] %s:\n", i+1, msg.Role)
			printMessageContent(msg.Content)
		}
		fmt.Println()
	}

	// Token counts
	fmt.Println("--- Tokens ---")
	fmt.Printf("Input: %d\n", llm.Tokens.InputTokens)
	if llm.CacheRead != nil && *llm.CacheRead > 0 {
		fmt.Printf("Cache Read: %d\n", *llm.CacheRead)
	}
	if llm.CacheCreation != nil && *llm.CacheCreation > 0 {
		fmt.Printf("Cache Write: %d\n", *llm.CacheCreation)
	}

	// Completion (if requested)
	if promptCompletion {
		fmt.Println()
		fmt.Println("=== Completion ===")
		fmt.Printf("Output Tokens: %d\n", llm.Tokens.OutputTokens)
		if llm.StopReason != "" {
			fmt.Printf("Stop Reason: %s\n", llm.StopReason)
		}
		fmt.Println()

		// Use TextContent if available, otherwise try to parse Content
		if llm.Completion.TextContent != "" {
			fmt.Println(llm.Completion.TextContent)
		} else if len(llm.Completion.Content) > 0 {
			printMessageContent(llm.Completion.Content)
		}
	}

	return nil
}

func printMessageContent(content json.RawMessage) {
	if len(content) == 0 {
		return
	}

	// Try to parse as string first
	var textStr string
	if err := json.Unmarshal(content, &textStr); err == nil {
		fmt.Println(textStr)
		return
	}

	// Try to parse as array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		for _, block := range blocks {
			switch block.Type {
			case "text":
				fmt.Println(block.Text)
			case "tool_use":
				fmt.Printf("  [Tool Use: %s]\n", block.ToolName)
				if block.ToolInput != nil {
					inputJSON, _ := json.MarshalIndent(block.ToolInput, "  ", "  ")
					fmt.Printf("  %s\n", inputJSON)
				}
			case "tool_result":
				fmt.Printf("  [Tool Result: %s]\n", block.ToolUseID)
				if block.Text != "" {
					fmt.Printf("  %s\n", block.Text)
				}
			case "thinking":
				fmt.Println("  [Thinking]")
				if len(block.Text) > 500 {
					fmt.Printf("  %s...\n", block.Text[:500])
				} else {
					fmt.Printf("  %s\n", block.Text)
				}
			}
		}
		return
	}

	// Fallback: print raw JSON
	fmt.Println(string(content))
}

func outputPromptJSON(span *store.Span) error {
	output := map[string]any{
		"span_id":  span.ID,
		"name":     span.Name,
		"provider": span.LLM.Provider,
		"model":    span.LLM.Model,
		"prompt": map[string]any{
			"system":   span.LLM.SystemPrompt,
			"messages": span.LLM.Messages,
		},
		"tokens": span.LLM.Tokens,
	}

	if promptCompletion {
		output["completion"] = span.LLM.Completion
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputPromptCurl(span *store.Span) error {
	llm := span.LLM

	// Build messages array for API format
	var messages []map[string]any

	// Add system message if present
	if llm.SystemPrompt != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": llm.SystemPrompt,
		})
	}

	// Add conversation messages
	for _, msg := range llm.Messages {
		m := map[string]any{
			"role": msg.Role,
		}
		// Try to get text content
		content := extractTextFromRawMessage(msg.Content)
		if content != "" {
			m["content"] = content
		} else {
			// Use raw content
			var rawContent any
			json.Unmarshal(msg.Content, &rawContent)
			m["content"] = rawContent
		}
		messages = append(messages, m)
	}

	// Determine API endpoint based on provider
	var endpoint string
	switch strings.ToLower(llm.Provider) {
	case "anthropic":
		endpoint = "https://api.anthropic.com/v1/messages"
	case "openai":
		endpoint = "https://api.openai.com/v1/chat/completions"
	default:
		endpoint = fmt.Sprintf("https://api.%s.com/v1/chat/completions", llm.Provider)
	}

	body := map[string]any{
		"model":    llm.Model,
		"messages": messages,
	}

	bodyJSON, _ := json.MarshalIndent(body, "", "  ")

	fmt.Printf("curl -X POST %s \\\n", endpoint)
	fmt.Printf("  -H 'Content-Type: application/json' \\\n")

	switch strings.ToLower(llm.Provider) {
	case "anthropic":
		fmt.Printf("  -H 'x-api-key: $ANTHROPIC_API_KEY' \\\n")
		fmt.Printf("  -H 'anthropic-version: 2023-06-01' \\\n")
	case "openai":
		fmt.Printf("  -H 'Authorization: Bearer $OPENAI_API_KEY' \\\n")
	default:
		fmt.Printf("  -H 'Authorization: Bearer $API_KEY' \\\n")
	}

	fmt.Printf("  -d '%s'\n", bodyJSON)

	return nil
}

func outputPromptSDK(span *store.Span) error {
	llm := span.LLM

	switch strings.ToLower(llm.Provider) {
	case "anthropic":
		return outputAnthropicSDK(llm)
	case "openai":
		return outputOpenAISDK(llm)
	default:
		return outputGenericSDK(llm)
	}
}

func outputAnthropicSDK(llm *store.LLMSpanData) error {
	fmt.Println("# Python (anthropic SDK)")
	fmt.Println("import anthropic")
	fmt.Println()
	fmt.Println("client = anthropic.Anthropic()")
	fmt.Println()
	fmt.Println("message = client.messages.create(")
	fmt.Printf("    model=%q,\n", llm.Model)
	fmt.Printf("    max_tokens=1024,\n")

	if llm.SystemPrompt != "" {
		fmt.Printf("    system=%q,\n", escapeString(llm.SystemPrompt))
	}

	fmt.Println("    messages=[")
	for _, msg := range llm.Messages {
		content := extractTextFromRawMessage(msg.Content)
		fmt.Printf("        {\"role\": %q, \"content\": %q},\n", msg.Role, escapeString(content))
	}
	fmt.Println("    ]")
	fmt.Println(")")
	fmt.Println()
	fmt.Println("print(message.content[0].text)")

	return nil
}

func outputOpenAISDK(llm *store.LLMSpanData) error {
	fmt.Println("# Python (openai SDK)")
	fmt.Println("from openai import OpenAI")
	fmt.Println()
	fmt.Println("client = OpenAI()")
	fmt.Println()
	fmt.Println("messages = [")

	if llm.SystemPrompt != "" {
		fmt.Printf("    {\"role\": \"system\", \"content\": %q},\n", escapeString(llm.SystemPrompt))
	}

	for _, msg := range llm.Messages {
		content := extractTextFromRawMessage(msg.Content)
		fmt.Printf("    {\"role\": %q, \"content\": %q},\n", msg.Role, escapeString(content))
	}
	fmt.Println("]")
	fmt.Println()
	fmt.Println("response = client.chat.completions.create(")
	fmt.Printf("    model=%q,\n", llm.Model)
	fmt.Println("    messages=messages")
	fmt.Println(")")
	fmt.Println()
	fmt.Println("print(response.choices[0].message.content)")

	return nil
}

func outputGenericSDK(llm *store.LLMSpanData) error {
	fmt.Printf("# Generic SDK example for %s\n", llm.Provider)
	fmt.Println("# Adapt this to your specific SDK")
	fmt.Println()
	fmt.Printf("model = %q\n", llm.Model)
	if llm.SystemPrompt != "" {
		fmt.Printf("system = %q\n", escapeString(llm.SystemPrompt))
	}
	fmt.Println("messages = [")
	for _, msg := range llm.Messages {
		content := extractTextFromRawMessage(msg.Content)
		fmt.Printf("    {\"role\": %q, \"content\": %q},\n", msg.Role, escapeString(content))
	}
	fmt.Println("]")

	return nil
}

func extractTextFromRawMessage(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}

	// Try string first
	var textStr string
	if err := json.Unmarshal(content, &textStr); err == nil {
		return textStr
	}

	// Try array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		var texts []string
		for _, block := range blocks {
			if block.Type == "text" && block.Text != "" {
				texts = append(texts, block.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	return string(content)
}

func escapeString(s string) string {
	// Escape for Python string literal
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
