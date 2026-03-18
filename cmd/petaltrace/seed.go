package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/internal/config"
	"github.com/petal-labs/petaltrace/store"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Generate test data for development",
	Long: `Seed the database with realistic test data for development and demonstration.

This creates sample workflow runs with LLM calls, tool invocations, and
various execution patterns to showcase PetalTrace capabilities.`,
	RunE: runSeed,
}

var (
	seedCount int
	seedClear bool
)

func init() {
	rootCmd.AddCommand(seedCmd)
	seedCmd.Flags().IntVarP(&seedCount, "count", "n", 10, "Number of workflow runs to generate")
	seedCmd.Flags().BoolVar(&seedClear, "clear", false, "Clear existing data before seeding")
}

func runSeed(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dbPath := cfg.Storage.SQLite.Path
	if dbPath == "" {
		dbPath = "./data/petaltrace.db"
	}

	if err := os.MkdirAll(cfg.DataDir(), 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{
		Path:    dbPath,
		WALMode: cfg.Storage.SQLite.WALMode,
	})
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer traceStore.Close()

	ctx := context.Background()

	fmt.Printf("Seeding database with %d workflow runs...\n", seedCount)

	generator := &testDataGenerator{
		store: traceStore,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	for i := 0; i < seedCount; i++ {
		run, spans := generator.generateRun(i)

		if err := traceStore.CreateRun(ctx, run); err != nil {
			return fmt.Errorf("creating run %d: %w", i, err)
		}

		for _, span := range spans {
			if err := traceStore.CreateSpan(ctx, span); err != nil {
				return fmt.Errorf("creating span: %w", err)
			}
		}

		if err := traceStore.UpdateRunAggregates(ctx, run.ID); err != nil {
			return fmt.Errorf("updating aggregates: %w", err)
		}

		fmt.Printf("  Created run: %s (%s)\n", run.WorkflowName, run.ID)
	}

	fmt.Printf("\nSeeding complete. Created %d runs.\n", seedCount)
	return nil
}

type testDataGenerator struct {
	store store.TraceStore
	rng   *rand.Rand
}

var workflowNames = []string{
	"customer-support-agent",
	"code-review-assistant",
	"document-summarizer",
	"research-analyst",
	"email-responder",
	"data-extraction-pipeline",
	"content-moderator",
	"translation-workflow",
	"qa-test-generator",
	"meeting-notes-processor",
}

var llmModels = []struct {
	provider string
	model    string
}{
	{"anthropic", "claude-sonnet-4-20250514"},
	{"anthropic", "claude-3-5-haiku-20241022"},
	{"anthropic", "claude-opus-4-20250514"},
	{"openai", "gpt-4o"},
	{"openai", "gpt-4o-mini"},
	{"openai", "o1"},
}

var toolNames = []string{
	"web_search",
	"read_file",
	"write_file",
	"execute_code",
	"send_email",
	"query_database",
	"fetch_url",
	"create_ticket",
	"slack_message",
	"calendar_create",
}

var systemPrompts = []string{
	"You are a helpful customer support agent. Answer questions accurately and professionally.",
	"You are a code review assistant. Analyze code for bugs, security issues, and best practices.",
	"You are a research analyst. Synthesize information from multiple sources to answer questions.",
	"You are a document summarizer. Create concise summaries while preserving key information.",
	"You are an email assistant. Draft professional and clear email responses.",
}

var userMessages = []string{
	"Can you help me understand how to reset my password?",
	"Please review this Python function for potential issues.",
	"What are the latest developments in quantum computing?",
	"Summarize this 50-page research paper on climate change.",
	"Draft a response to this customer complaint email.",
	"How do I configure the API authentication settings?",
	"Can you analyze this code for security vulnerabilities?",
	"What's the best approach for implementing caching here?",
	"Help me debug this error message I'm seeing.",
	"Can you explain this technical concept in simpler terms?",
}

var assistantResponses = []string{
	"I'd be happy to help you with that. Here's a step-by-step guide...",
	"After reviewing the code, I found several areas for improvement...",
	"Based on my analysis of the available information...",
	"Here's a concise summary of the key points...",
	"I've drafted the following response for your review...",
	"The configuration process involves the following steps...",
	"I identified several potential security concerns...",
	"For optimal caching, I recommend the following approach...",
	"The error appears to be caused by...",
	"Let me break this down into simpler terms...",
}

func (g *testDataGenerator) generateRun(index int) (*store.Run, []*store.Span) {
	now := time.Now().UTC()
	startTime := now.Add(-time.Duration(g.rng.Intn(168)) * time.Hour) // Random time in last week
	durationMs := int64(g.rng.Intn(30000) + 1000)                     // 1-31 seconds
	completedAt := startTime.Add(time.Duration(durationMs) * time.Millisecond)

	status := store.RunStatusCompleted
	if g.rng.Float32() < 0.1 {
		status = store.RunStatusFailed
	} else if g.rng.Float32() < 0.05 {
		status = store.RunStatusRunning
		completedAt = time.Time{}
	}

	workflowName := workflowNames[g.rng.Intn(len(workflowNames))]
	workflowID := fmt.Sprintf("wf_%s", store.NewID()[:8])

	run := &store.Run{
		ID:              store.NewID(),
		WorkflowID:      workflowID,
		WorkflowName:    workflowName,
		WorkflowVersion: fmt.Sprintf("v1.%d.%d", g.rng.Intn(5), g.rng.Intn(10)),
		SourceKind:      "petalflow",
		Status:          status,
		StartedAt:       startTime,
		DurationMs:      durationMs,
		Tags: map[string]string{
			"environment": []string{"production", "staging", "development"}[g.rng.Intn(3)],
			"team":        []string{"platform", "growth", "support"}[g.rng.Intn(3)],
		},
		TriggerSource: []string{"api", "cron", "webhook", "manual"}[g.rng.Intn(4)],
		Starred:       g.rng.Float32() < 0.2,
		CreatedAt:     startTime,
	}

	if status != store.RunStatusRunning {
		run.CompletedAt = &completedAt
	}

	// Generate graph snapshot
	run.GraphSnapshot = g.generateGraphSnapshot(workflowName)

	// Generate spans
	spans := g.generateSpans(run)

	return run, spans
}

func (g *testDataGenerator) generateGraphSnapshot(workflowName string) json.RawMessage {
	graph := map[string]any{
		"name": workflowName,
		"nodes": []map[string]any{
			{"id": "input", "type": "input", "label": "User Input"},
			{"id": "router", "type": "router", "label": "Intent Router"},
			{"id": "llm_main", "type": "llm", "label": "Main LLM"},
			{"id": "tool_executor", "type": "tool", "label": "Tool Executor"},
			{"id": "output", "type": "output", "label": "Response"},
		},
		"edges": []map[string]any{
			{"source": "input", "target": "router"},
			{"source": "router", "target": "llm_main"},
			{"source": "llm_main", "target": "tool_executor"},
			{"source": "tool_executor", "target": "llm_main"},
			{"source": "llm_main", "target": "output"},
		},
	}
	data, _ := json.Marshal(graph)
	return data
}

func (g *testDataGenerator) generateSpans(run *store.Run) []*store.Span {
	var spans []*store.Span
	traceID := store.NewID()

	// Root node span
	rootSpan := g.generateNodeSpan(run, traceID, nil, "workflow_root", "workflow", run.StartedAt, run.DurationMs)
	spans = append(spans, rootSpan)

	currentTime := run.StartedAt.Add(10 * time.Millisecond)
	parentID := rootSpan.ID

	// Generate 2-5 LLM calls
	llmCount := g.rng.Intn(4) + 2
	for i := 0; i < llmCount; i++ {
		duration := int64(g.rng.Intn(5000) + 500)
		llmSpan := g.generateLLMSpan(run, traceID, &parentID, currentTime, duration)
		spans = append(spans, llmSpan)

		// Maybe add a tool call after LLM
		if g.rng.Float32() < 0.6 {
			toolStart := currentTime.Add(time.Duration(duration) * time.Millisecond)
			toolDuration := int64(g.rng.Intn(2000) + 100)
			toolSpan := g.generateToolSpan(run, traceID, &llmSpan.ID, toolStart, toolDuration)
			spans = append(spans, toolSpan)
			currentTime = toolStart.Add(time.Duration(toolDuration) * time.Millisecond)
		} else {
			currentTime = currentTime.Add(time.Duration(duration) * time.Millisecond)
		}

		currentTime = currentTime.Add(50 * time.Millisecond)
	}

	// Add edge spans
	edgeSpan := g.generateEdgeSpan(run, traceID, &parentID, "llm_main", "output", currentTime)
	spans = append(spans, edgeSpan)

	return spans
}

func (g *testDataGenerator) generateNodeSpan(run *store.Run, traceID string, parentID *string, nodeID, nodeType string, startTime time.Time, durationMs int64) *store.Span {
	completedAt := startTime.Add(time.Duration(durationMs) * time.Millisecond)

	return &store.Span{
		ID:          store.NewID(),
		RunID:       run.ID,
		ParentID:    parentID,
		TraceID:     traceID,
		Kind:        store.SpanKindNode,
		Name:        fmt.Sprintf("node.%s", nodeID),
		Status:      store.SpanStatusOK,
		StartedAt:   startTime,
		CompletedAt: &completedAt,
		DurationMs:  durationMs,
		Node: &store.NodeSpanData{
			NodeID:   nodeID,
			NodeType: nodeType,
			Inputs:   json.RawMessage(`{"query": "user input"}`),
			Outputs:  json.RawMessage(`{"result": "processed output"}`),
		},
		CreatedAt: startTime,
	}
}

func (g *testDataGenerator) generateLLMSpan(run *store.Run, traceID string, parentID *string, startTime time.Time, durationMs int64) *store.Span {
	completedAt := startTime.Add(time.Duration(durationMs) * time.Millisecond)
	modelInfo := llmModels[g.rng.Intn(len(llmModels))]

	inputTokens := g.rng.Intn(2000) + 100
	outputTokens := g.rng.Intn(1000) + 50
	cacheRead := 0
	if g.rng.Float32() < 0.3 {
		cacheRead = g.rng.Intn(inputTokens / 2)
	}

	// Calculate cost estimate based on model
	var costEstimate float64
	switch modelInfo.model {
	case "claude-sonnet-4-20250514":
		costEstimate = float64(inputTokens)*3.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
	case "claude-3-5-haiku-20241022":
		costEstimate = float64(inputTokens)*0.8/1_000_000 + float64(outputTokens)*4.0/1_000_000
	case "claude-opus-4-20250514":
		costEstimate = float64(inputTokens)*15.0/1_000_000 + float64(outputTokens)*75.0/1_000_000
	case "gpt-4o":
		costEstimate = float64(inputTokens)*2.5/1_000_000 + float64(outputTokens)*10.0/1_000_000
	case "gpt-4o-mini":
		costEstimate = float64(inputTokens)*0.15/1_000_000 + float64(outputTokens)*0.6/1_000_000
	default:
		costEstimate = float64(inputTokens)*5.0/1_000_000 + float64(outputTokens)*15.0/1_000_000
	}

	systemPrompt := systemPrompts[g.rng.Intn(len(systemPrompts))]
	userMsg := userMessages[g.rng.Intn(len(userMessages))]
	assistantResp := assistantResponses[g.rng.Intn(len(assistantResponses))]

	userContent, _ := json.Marshal(userMsg)
	messages := []store.LLMMessage{
		{Role: "user", Content: userContent},
	}

	completionContent, _ := json.Marshal(assistantResp)
	ttft := int64(g.rng.Intn(500) + 50)

	status := store.SpanStatusOK
	if run.Status == store.RunStatusFailed && g.rng.Float32() < 0.5 {
		status = store.SpanStatusError
	}

	span := &store.Span{
		ID:          store.NewID(),
		RunID:       run.ID,
		ParentID:    parentID,
		TraceID:     traceID,
		Kind:        store.SpanKindLLM,
		Name:        fmt.Sprintf("llm.%s.%s", modelInfo.provider, modelInfo.model),
		Status:      status,
		StartedAt:   startTime,
		CompletedAt: &completedAt,
		DurationMs:  durationMs,
		LLM: &store.LLMSpanData{
			Provider:     modelInfo.provider,
			Model:        modelInfo.model,
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Completion: store.LLMCompletion{
				Content:     completionContent,
				TextContent: assistantResp,
			},
			Tokens: store.TokenDetail{
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				TotalTokens:  inputTokens + outputTokens,
				CostEstimate: costEstimate,
			},
			TimeToFirstToken: &ttft,
			TotalLatency:     durationMs,
			StopReason:       "end_turn",
			CacheRead:        &cacheRead,
		},
		CreatedAt: startTime,
	}

	if status == store.SpanStatusError {
		span.Error = &store.SpanError{
			Code:    "rate_limit_exceeded",
			Message: "Rate limit exceeded. Please retry after 60 seconds.",
		}
	}

	return span
}

func (g *testDataGenerator) generateToolSpan(run *store.Run, traceID string, parentID *string, startTime time.Time, durationMs int64) *store.Span {
	completedAt := startTime.Add(time.Duration(durationMs) * time.Millisecond)
	toolName := toolNames[g.rng.Intn(len(toolNames))]

	inputs := map[string]any{
		"query": "sample query",
	}
	outputs := map[string]any{
		"result": "tool execution result",
		"status": "success",
	}

	inputsJSON, _ := json.Marshal(inputs)
	outputsJSON, _ := json.Marshal(outputs)

	return &store.Span{
		ID:          store.NewID(),
		RunID:       run.ID,
		ParentID:    parentID,
		TraceID:     traceID,
		Kind:        store.SpanKindTool,
		Name:        fmt.Sprintf("tool.%s", toolName),
		Status:      store.SpanStatusOK,
		StartedAt:   startTime,
		CompletedAt: &completedAt,
		DurationMs:  durationMs,
		Tool: &store.ToolSpanData{
			ToolName:   toolName,
			ActionName: "execute",
			Origin:     "llm_request",
			Inputs:     inputsJSON,
			Outputs:    outputsJSON,
			DurationMs: durationMs,
		},
		CreatedAt: startTime,
	}
}

func (g *testDataGenerator) generateEdgeSpan(run *store.Run, traceID string, parentID *string, sourceNode, targetNode string, startTime time.Time) *store.Span {
	durationMs := int64(g.rng.Intn(10) + 1)
	completedAt := startTime.Add(time.Duration(durationMs) * time.Millisecond)

	return &store.Span{
		ID:          store.NewID(),
		RunID:       run.ID,
		ParentID:    parentID,
		TraceID:     traceID,
		Kind:        store.SpanKindEdge,
		Name:        fmt.Sprintf("edge.%s->%s", sourceNode, targetNode),
		Status:      store.SpanStatusOK,
		StartedAt:   startTime,
		CompletedAt: &completedAt,
		DurationMs:  durationMs,
		Edge: &store.EdgeSpanData{
			SourceNode:  sourceNode,
			SourcePort:  "output",
			TargetNode:  targetNode,
			TargetPort:  "input",
			DataSize:    int64(g.rng.Intn(10000) + 100),
			DataPreview: "data transferred between nodes...",
		},
		CreatedAt: startTime,
	}
}
