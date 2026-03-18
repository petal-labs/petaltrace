package collector

import (
	"context"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func TestBatchWriter_Write(t *testing.T) {
	s := setupTestStore(t)
	writer := NewBatchWriter(BatchWriterConfig{
		Store:     s,
		BatchSize: 10,
	})

	ctx := context.Background()

	// Create a run first
	run := &store.Run{
		ID:          "run-1",
		WorkflowID:  "wf-1",
		Status:      store.RunStatusRunning,
		StartedAt:   time.Now(),
		CreatedAt:   time.Now(),
		TotalTokens: store.TokenSummary{},
		EstimatedCost: store.CostEstimate{
			Currency: "USD",
		},
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error: %v", err)
	}

	spans := make([]*CorrelatedSpan, 5)
	for i := 0; i < 5; i++ {
		spans[i] = &CorrelatedSpan{
			Span: &store.Span{
				ID:        store.NewID(),
				RunID:     "run-1",
				Kind:      store.SpanKindNode,
				Name:      "span",
				Status:    store.SpanStatusOK,
				StartedAt: time.Now(),
				CreatedAt: time.Now(),
			},
			Run: run,
		}
	}

	if err := writer.Write(spans); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Should be in buffer, not flushed yet
	if writer.BufferLen() != 5 {
		t.Errorf("BufferLen = %d, want 5", writer.BufferLen())
	}

	// Flush
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if writer.BufferLen() != 0 {
		t.Errorf("BufferLen after flush = %d, want 0", writer.BufferLen())
	}

	// Check stats
	written, dropped, batches := writer.Stats()
	if written != 5 {
		t.Errorf("written = %d, want 5", written)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
	if batches != 1 {
		t.Errorf("batches = %d, want 1", batches)
	}

	// Verify spans in store
	tree, err := s.GetSpanTree(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetSpanTree error: %v", err)
	}
	if len(tree) != 5 {
		t.Errorf("got %d spans in store, want 5", len(tree))
	}
}

func TestBatchWriter_AutoFlush(t *testing.T) {
	s := setupTestStore(t)
	writer := NewBatchWriter(BatchWriterConfig{
		Store:     s,
		BatchSize: 5, // Small batch size for testing
	})

	ctx := context.Background()

	// Create a run first
	run := &store.Run{
		ID:            "run-2",
		Status:        store.RunStatusRunning,
		StartedAt:     time.Now(),
		CreatedAt:     time.Now(),
		TotalTokens:   store.TokenSummary{},
		EstimatedCost: store.CostEstimate{Currency: "USD"},
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error: %v", err)
	}

	// Write exactly batch size - should auto-flush
	spans := make([]*CorrelatedSpan, 5)
	for i := 0; i < 5; i++ {
		spans[i] = &CorrelatedSpan{
			Span: &store.Span{
				ID:        store.NewID(),
				RunID:     "run-2",
				Kind:      store.SpanKindNode,
				Name:      "span",
				Status:    store.SpanStatusOK,
				StartedAt: time.Now(),
				CreatedAt: time.Now(),
			},
			Run: run,
		}
	}

	if err := writer.Write(spans); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Should have auto-flushed
	if writer.BufferLen() != 0 {
		t.Errorf("BufferLen = %d, want 0 (auto-flushed)", writer.BufferLen())
	}

	written, _, _ := writer.Stats()
	if written != 5 {
		t.Errorf("written = %d, want 5", written)
	}
}

func TestBatchWriter_PeriodicFlush(t *testing.T) {
	s := setupTestStore(t)
	writer := NewBatchWriter(BatchWriterConfig{
		Store:         s,
		BatchSize:     100, // Large batch size
		FlushInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()

	// Create a run first
	run := &store.Run{
		ID:            "run-3",
		Status:        store.RunStatusRunning,
		StartedAt:     time.Now(),
		CreatedAt:     time.Now(),
		TotalTokens:   store.TokenSummary{},
		EstimatedCost: store.CostEstimate{Currency: "USD"},
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error: %v", err)
	}

	writer.Start()
	defer writer.Stop()

	spans := []*CorrelatedSpan{
		{
			Span: &store.Span{
				ID:        store.NewID(),
				RunID:     "run-3",
				Kind:      store.SpanKindNode,
				Name:      "span",
				Status:    store.SpanStatusOK,
				StartedAt: time.Now(),
				CreatedAt: time.Now(),
			},
			Run: run,
		},
	}

	if err := writer.Write(spans); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Buffer should have the span
	if writer.BufferLen() != 1 {
		t.Errorf("BufferLen = %d, want 1", writer.BufferLen())
	}

	// Wait for periodic flush
	time.Sleep(100 * time.Millisecond)

	// Should be flushed now
	if writer.BufferLen() != 0 {
		t.Errorf("BufferLen after periodic flush = %d, want 0", writer.BufferLen())
	}
}

func TestBatchWriter_LLMTextIndexing(t *testing.T) {
	s := setupTestStore(t)
	writer := NewBatchWriter(BatchWriterConfig{
		Store:     s,
		BatchSize: 10,
	})

	ctx := context.Background()

	// Create a run first
	run := &store.Run{
		ID:            "run-4",
		Status:        store.RunStatusRunning,
		StartedAt:     time.Now(),
		CreatedAt:     time.Now(),
		TotalTokens:   store.TokenSummary{},
		EstimatedCost: store.CostEstimate{Currency: "USD"},
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error: %v", err)
	}

	spans := []*CorrelatedSpan{
		{
			Span: &store.Span{
				ID:        store.NewID(),
				RunID:     "run-4",
				Kind:      store.SpanKindLLM,
				Name:      "llm-call",
				Status:    store.SpanStatusOK,
				StartedAt: time.Now(),
				CreatedAt: time.Now(),
				LLM: &store.LLMSpanData{
					Provider:     "anthropic",
					Model:        "claude-sonnet-4",
					SystemPrompt: "You are a helpful coding assistant.",
					Completion: store.LLMCompletion{
						TextContent: "Here is how to write a function in Python...",
					},
				},
			},
			Run: run,
		},
	}

	if err := writer.Write(spans); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	// Search for indexed text
	results, err := s.SearchSpans(ctx, "coding assistant", 10)
	if err != nil {
		t.Fatalf("SearchSpans error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("got %d search results, want 1", len(results))
	}
}

func TestBatchWriter_Stop(t *testing.T) {
	s := setupTestStore(t)
	writer := NewBatchWriter(BatchWriterConfig{
		Store:         s,
		BatchSize:     100,
		FlushInterval: time.Second,
	})

	ctx := context.Background()

	// Create a run first
	run := &store.Run{
		ID:            "run-5",
		Status:        store.RunStatusRunning,
		StartedAt:     time.Now(),
		CreatedAt:     time.Now(),
		TotalTokens:   store.TokenSummary{},
		EstimatedCost: store.CostEstimate{Currency: "USD"},
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error: %v", err)
	}

	writer.Start()

	spans := []*CorrelatedSpan{
		{
			Span: &store.Span{
				ID:        store.NewID(),
				RunID:     "run-5",
				Kind:      store.SpanKindNode,
				Name:      "span",
				Status:    store.SpanStatusOK,
				StartedAt: time.Now(),
				CreatedAt: time.Now(),
			},
			Run: run,
		},
	}

	if err := writer.Write(spans); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Stop should flush remaining
	if err := writer.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	written, _, _ := writer.Stats()
	if written != 1 {
		t.Errorf("written = %d, want 1", written)
	}
}

func TestBatchWriter_RunUpdates(t *testing.T) {
	s := setupTestStore(t)
	writer := NewBatchWriter(BatchWriterConfig{
		Store:     s,
		BatchSize: 10,
	})

	ctx := context.Background()

	// Create a run
	run := &store.Run{
		ID:        "run-6",
		Status:    store.RunStatusRunning,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
		TotalTokens: store.TokenSummary{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
		EstimatedCost: store.CostEstimate{
			Currency: "USD",
			Total:    0.01,
		},
	}
	if err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun error: %v", err)
	}

	// Update run via span write
	run.TotalTokens.InputTokens = 200
	run.TotalTokens.OutputTokens = 100
	run.TotalTokens.TotalTokens = 300
	run.EstimatedCost.Total = 0.02

	spans := []*CorrelatedSpan{
		{
			Span: &store.Span{
				ID:        store.NewID(),
				RunID:     "run-6",
				Kind:      store.SpanKindNode,
				Name:      "span",
				Status:    store.SpanStatusOK,
				StartedAt: time.Now(),
				CreatedAt: time.Now(),
			},
			Run: run,
		},
	}

	if err := writer.Write(spans); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	// Verify run was updated
	savedRun, err := s.GetRun(ctx, "run-6")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}

	if savedRun.TotalTokens.InputTokens != 200 {
		t.Errorf("TotalTokens.InputTokens = %d, want 200", savedRun.TotalTokens.InputTokens)
	}
	if savedRun.EstimatedCost.Total != 0.02 {
		t.Errorf("EstimatedCost.Total = %f, want 0.02", savedRun.EstimatedCost.Total)
	}
}
