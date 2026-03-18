package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCreateAndGetSpan(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	completedAt := now.Add(2 * time.Second)
	parentID := "parent_span"
	temp := 0.7

	span := &Span{
		RunID:       run.ID,
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
			SystemPrompt: "You are helpful.",
			Messages: []LLMMessage{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
			},
			Temperature: &temp,
			Completion: LLMCompletion{
				TextContent: "Hi there!",
			},
			Tokens: TokenDetail{
				InputTokens:  50,
				OutputTokens: 10,
				TotalTokens:  60,
				CostEstimate: 0.001,
			},
			TotalLatency: 2000,
			StopReason:   "end_turn",
		},
		Attributes: map[string]any{
			"custom": "value",
		},
	}

	if err := store.CreateSpan(ctx, span); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	if span.ID == "" {
		t.Error("span.ID was not set")
	}

	got, err := store.GetSpan(ctx, span.ID)
	if err != nil {
		t.Fatalf("GetSpan() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetSpan() returned nil")
	}

	if got.Kind != SpanKindLLM {
		t.Errorf("Kind = %s, want %s", got.Kind, SpanKindLLM)
	}

	if got.LLM == nil {
		t.Fatal("LLM is nil")
	}

	if got.LLM.Provider != "anthropic" {
		t.Errorf("LLM.Provider = %s, want anthropic", got.LLM.Provider)
	}

	if got.LLM.Tokens.TotalTokens != 60 {
		t.Errorf("LLM.Tokens.TotalTokens = %d, want 60", got.LLM.Tokens.TotalTokens)
	}

	if got.Attributes["custom"] != "value" {
		t.Errorf("Attributes[custom] = %v, want value", got.Attributes["custom"])
	}
}

func TestCreateSpanWithNodeData(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	span := &Span{
		RunID:      run.ID,
		TraceID:    "trace_123",
		Kind:       SpanKindNode,
		Name:       "researcher_node",
		Status:     SpanStatusOK,
		StartedAt:  now,
		DurationMs: 5000,
		Node: &NodeSpanData{
			NodeID:     "node_1",
			NodeType:   "llm_prompt",
			Inputs:     json.RawMessage(`{"topic": "AI"}`),
			Outputs:    json.RawMessage(`{"result": "done"}`),
			RetryCount: 1,
		},
	}

	if err := store.CreateSpan(ctx, span); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	got, err := store.GetSpan(ctx, span.ID)
	if err != nil {
		t.Fatalf("GetSpan() error = %v", err)
	}

	if got.Node == nil {
		t.Fatal("Node is nil")
	}

	if got.Node.NodeID != "node_1" {
		t.Errorf("Node.NodeID = %s, want node_1", got.Node.NodeID)
	}

	if got.Node.RetryCount != 1 {
		t.Errorf("Node.RetryCount = %d, want 1", got.Node.RetryCount)
	}
}

func TestCreateSpanWithError(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	span := &Span{
		RunID:      run.ID,
		TraceID:    "trace_123",
		Kind:       SpanKindTool,
		Name:       "web_search",
		Status:     SpanStatusError,
		StartedAt:  now,
		DurationMs: 1000,
		Tool: &ToolSpanData{
			ToolName:   "web_search",
			ActionName: "search",
			Origin:     "mcp",
		},
		Error: &SpanError{
			Code:    "TIMEOUT",
			Message: "Request timed out",
			Details: "Connection to search API timed out after 30s",
		},
	}

	if err := store.CreateSpan(ctx, span); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	got, err := store.GetSpan(ctx, span.ID)
	if err != nil {
		t.Fatalf("GetSpan() error = %v", err)
	}

	if got.Error == nil {
		t.Fatal("Error is nil")
	}

	if got.Error.Code != "TIMEOUT" {
		t.Errorf("Error.Code = %s, want TIMEOUT", got.Error.Code)
	}

	if got.Error.Message != "Request timed out" {
		t.Errorf("Error.Message = %s, want 'Request timed out'", got.Error.Message)
	}
}

func TestCreateSpanBatch(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	spans := []*Span{
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindNode, Name: "node_1", Status: SpanStatusOK, StartedAt: now},
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindLLM, Name: "llm_1", Status: SpanStatusOK, StartedAt: now.Add(time.Second)},
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindTool, Name: "tool_1", Status: SpanStatusOK, StartedAt: now.Add(2 * time.Second)},
	}

	if err := store.CreateSpanBatch(ctx, spans); err != nil {
		t.Fatalf("CreateSpanBatch() error = %v", err)
	}

	for _, span := range spans {
		if span.ID == "" {
			t.Error("span.ID was not set")
		}
	}

	tree, err := store.GetSpanTree(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetSpanTree() error = %v", err)
	}

	if len(tree) != 3 {
		t.Errorf("len(tree) = %d, want 3", len(tree))
	}
}

func TestGetSpanTree(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	rootSpan := &Span{
		RunID:     run.ID,
		TraceID:   "trace_1",
		Kind:      SpanKindNode,
		Name:      "root",
		Status:    SpanStatusOK,
		StartedAt: now,
	}
	if err := store.CreateSpan(ctx, rootSpan); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	childSpan := &Span{
		RunID:     run.ID,
		ParentID:  &rootSpan.ID,
		TraceID:   "trace_1",
		Kind:      SpanKindLLM,
		Name:      "child_llm",
		Status:    SpanStatusOK,
		StartedAt: now.Add(time.Second),
	}
	if err := store.CreateSpan(ctx, childSpan); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	tree, err := store.GetSpanTree(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetSpanTree() error = %v", err)
	}

	if len(tree) != 2 {
		t.Errorf("len(tree) = %d, want 2", len(tree))
	}

	if tree[0].Name != "root" {
		t.Errorf("tree[0].Name = %s, want root", tree[0].Name)
	}

	if tree[1].ParentID == nil || *tree[1].ParentID != rootSpan.ID {
		t.Error("tree[1].ParentID mismatch")
	}
}

func TestGetSpansByKind(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	spans := []*Span{
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindNode, Name: "node_1", Status: SpanStatusOK, StartedAt: now},
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindLLM, Name: "llm_1", Status: SpanStatusOK, StartedAt: now},
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindLLM, Name: "llm_2", Status: SpanStatusOK, StartedAt: now},
		{RunID: run.ID, TraceID: "trace_1", Kind: SpanKindTool, Name: "tool_1", Status: SpanStatusOK, StartedAt: now},
	}

	for _, span := range spans {
		if err := store.CreateSpan(ctx, span); err != nil {
			t.Fatalf("CreateSpan() error = %v", err)
		}
	}

	llmSpans, err := store.GetSpansByKind(ctx, run.ID, SpanKindLLM)
	if err != nil {
		t.Fatalf("GetSpansByKind() error = %v", err)
	}

	if len(llmSpans) != 2 {
		t.Errorf("len(llmSpans) = %d, want 2", len(llmSpans))
	}

	for _, span := range llmSpans {
		if span.Kind != SpanKindLLM {
			t.Errorf("span.Kind = %s, want %s", span.Kind, SpanKindLLM)
		}
	}
}

func TestUpdateSpan(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusRunning,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	span := &Span{
		RunID:     run.ID,
		TraceID:   "trace_1",
		Kind:      SpanKindLLM,
		Name:      "llm_call",
		Status:    SpanStatusOK,
		StartedAt: now,
	}

	if err := store.CreateSpan(ctx, span); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	completedAt := now.Add(5 * time.Second)
	span.CompletedAt = &completedAt
	span.DurationMs = 5000
	span.LLM = &LLMSpanData{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
		Tokens: TokenDetail{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}

	if err := store.UpdateSpan(ctx, span); err != nil {
		t.Fatalf("UpdateSpan() error = %v", err)
	}

	got, err := store.GetSpan(ctx, span.ID)
	if err != nil {
		t.Fatalf("GetSpan() error = %v", err)
	}

	if got.DurationMs != 5000 {
		t.Errorf("DurationMs = %d, want 5000", got.DurationMs)
	}

	if got.LLM == nil {
		t.Fatal("LLM is nil")
	}

	if got.LLM.Tokens.TotalTokens != 150 {
		t.Errorf("LLM.Tokens.TotalTokens = %d, want 150", got.LLM.Tokens.TotalTokens)
	}
}

func TestIndexAndSearchSpans(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run := &Run{
		WorkflowID:   "wf_001",
		WorkflowName: "test_workflow",
		SourceKind:   "graph",
		Status:       RunStatusCompleted,
		StartedAt:    now,
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	span := &Span{
		RunID:     run.ID,
		TraceID:   "trace_1",
		Kind:      SpanKindLLM,
		Name:      "research_llm",
		Status:    SpanStatusOK,
		StartedAt: now,
	}

	if err := store.CreateSpan(ctx, span); err != nil {
		t.Fatalf("CreateSpan() error = %v", err)
	}

	promptText := "Tell me about quantum computing and machine learning"
	completionText := "Quantum computing offers significant potential for machine learning algorithms"

	if err := store.IndexSpanText(ctx, span.ID, promptText, completionText); err != nil {
		t.Fatalf("IndexSpanText() error = %v", err)
	}

	results, err := store.SearchSpans(ctx, "quantum", 10)
	if err != nil {
		t.Fatalf("SearchSpans() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}

	if len(results) > 0 && results[0].ID != span.ID {
		t.Errorf("results[0].ID = %s, want %s", results[0].ID, span.ID)
	}
}

func TestDiffOperations(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	run1 := &Run{WorkflowID: "wf_001", WorkflowName: "test", SourceKind: "graph", Status: RunStatusCompleted, StartedAt: now}
	run2 := &Run{WorkflowID: "wf_001", WorkflowName: "test", SourceKind: "graph", Status: RunStatusCompleted, StartedAt: now}

	if err := store.CreateRun(ctx, run1); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if err := store.CreateRun(ctx, run2); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	diff := &RunDiff{
		BaseRunID:    run1.ID,
		CompareRunID: run2.ID,
		Summary: DiffSummary{
			StatusMatch:   true,
			DurationDelta: 1000,
			TokenDelta:    100,
		},
		CostDiff: CostDiff{
			BaseCost:    0.05,
			CompareCost: 0.06,
			Delta:       0.01,
		},
	}

	if err := store.CreateDiff(ctx, diff); err != nil {
		t.Fatalf("CreateDiff() error = %v", err)
	}

	got, err := store.GetDiff(ctx, diff.ID)
	if err != nil {
		t.Fatalf("GetDiff() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetDiff() returned nil")
	}

	if got.Summary.TokenDelta != 100 {
		t.Errorf("Summary.TokenDelta = %d, want 100", got.Summary.TokenDelta)
	}

	gotByRuns, err := store.GetDiffByRuns(ctx, run1.ID, run2.ID)
	if err != nil {
		t.Fatalf("GetDiffByRuns() error = %v", err)
	}

	if gotByRuns == nil {
		t.Fatal("GetDiffByRuns() returned nil")
	}

	if gotByRuns.ID != diff.ID {
		t.Errorf("ID = %s, want %s", gotByRuns.ID, diff.ID)
	}
}

func TestPricingOperations(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()

	entry := &PricingEntry{
		Provider:        "anthropic",
		Model:           "claude-sonnet-4-20250514",
		InputPer1M:      3.00,
		OutputPer1M:     15.00,
		CacheReadPer1M:  0.30,
		CacheWritePer1M: 3.75,
		EffectiveFrom:   time.Now().UTC(),
	}

	if err := store.UpsertPricing(ctx, entry); err != nil {
		t.Fatalf("UpsertPricing() error = %v", err)
	}

	got, err := store.GetPricing(ctx, "anthropic", "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("GetPricing() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetPricing() returned nil")
	}

	if got.InputPer1M != 3.00 {
		t.Errorf("InputPer1M = %f, want 3.00", got.InputPer1M)
	}

	if got.OutputPer1M != 15.00 {
		t.Errorf("OutputPer1M = %f, want 15.00", got.OutputPer1M)
	}

	entries, err := store.ListPricing(ctx)
	if err != nil {
		t.Fatalf("ListPricing() error = %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("len(entries) = %d, want 1", len(entries))
	}
}
