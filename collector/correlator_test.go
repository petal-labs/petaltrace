package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func setupTestStore(t *testing.T) store.TraceStore {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	t.Cleanup(func() {
		s.Close()
		os.RemoveAll(tmpDir)
	})

	return s
}

func TestCorrelator_NewRun(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			TraceID:   "abc123",
			SpanID:    "span1",
			Name:      "root-span",
			StartTime: now,
			EndTime:   now.Add(100 * time.Millisecond),
			Status:    SpanStatusOK,
			Attributes: map[string]any{
				"petalflow.run.id":        "run-123",
				"petalflow.workflow.name": "test-workflow",
				"petalflow.run.root":      true,
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	if len(correlated) != 1 {
		t.Fatalf("got %d correlated spans, want 1", len(correlated))
	}

	cs := correlated[0]
	if cs.Run == nil {
		t.Fatal("Run is nil")
	}

	if cs.Run.ID != "run-123" {
		t.Errorf("Run.ID = %s, want run-123", cs.Run.ID)
	}
	if cs.Run.WorkflowName != "test-workflow" {
		t.Errorf("Run.WorkflowName = %s, want test-workflow", cs.Run.WorkflowName)
	}
	if cs.Run.Status != store.RunStatusRunning {
		t.Errorf("Run.Status = %s, want running", cs.Run.Status)
	}
	if !cs.IsRoot {
		t.Error("expected IsRoot = true")
	}

	// Verify run was persisted
	savedRun, err := s.GetRun(ctx, "run-123")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}
	if savedRun.WorkflowName != "test-workflow" {
		t.Errorf("saved run WorkflowName = %s", savedRun.WorkflowName)
	}
}

func TestCorrelator_GroupSpansToSameRun(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			TraceID:   "trace1",
			SpanID:    "span1",
			Name:      "root",
			StartTime: now,
			EndTime:   now.Add(100 * time.Millisecond),
			Attributes: map[string]any{
				"petalflow.run.id": "run-456",
			},
		},
		{
			TraceID:      "trace1",
			SpanID:       "span2",
			ParentSpanID: "span1",
			Name:         "child1",
			StartTime:    now.Add(10 * time.Millisecond),
			EndTime:      now.Add(50 * time.Millisecond),
			Attributes: map[string]any{
				"petalflow.run.id": "run-456",
			},
		},
		{
			TraceID:      "trace1",
			SpanID:       "span3",
			ParentSpanID: "span1",
			Name:         "child2",
			StartTime:    now.Add(50 * time.Millisecond),
			EndTime:      now.Add(90 * time.Millisecond),
			Attributes: map[string]any{
				"petalflow.run.id": "run-456",
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	if len(correlated) != 3 {
		t.Fatalf("got %d correlated spans, want 3", len(correlated))
	}

	// All spans should reference the same run
	runID := correlated[0].Run.ID
	for i, cs := range correlated {
		if cs.Run.ID != runID {
			t.Errorf("span %d has Run.ID = %s, want %s", i, cs.Run.ID, runID)
		}
		if cs.Span.RunID != runID {
			t.Errorf("span %d has Span.RunID = %s, want %s", i, cs.Span.RunID, runID)
		}
	}

	// Verify only one run was created
	if c.ActiveRunCount() != 1 {
		t.Errorf("ActiveRunCount = %d, want 1", c.ActiveRunCount())
	}
}

func TestCorrelator_FallbackToTraceID(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			TraceID:    "trace-xyz-789",
			SpanID:     "span1",
			Name:       "operation",
			StartTime:  now,
			EndTime:    now.Add(100 * time.Millisecond),
			Attributes: map[string]any{},
			Resource: map[string]any{
				"service.name": "my-service",
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	if correlated[0].Run.ID != "trace-trace-xyz-789" {
		t.Errorf("Run.ID = %s, want trace-trace-xyz-789", correlated[0].Run.ID)
	}
	if correlated[0].Run.WorkflowName != "my-service" {
		t.Errorf("WorkflowName = %s, want my-service", correlated[0].Run.WorkflowName)
	}
}

func TestCorrelator_CaptureSnapshots(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			TraceID:   "trace1",
			SpanID:    "span1",
			Name:      "workflow-root",
			StartTime: now,
			EndTime:   now.Add(100 * time.Millisecond),
			Attributes: map[string]any{
				"petalflow.run.id":   "run-with-snapshots",
				"petalflow.run.root": true,
				"petalflow.graph": map[string]any{
					"nodes": []any{"node1", "node2"},
					"edges": []any{},
				},
				"petalflow.input": map[string]any{
					"prompt": "Hello, world!",
				},
				"petalflow.config": map[string]any{
					"temperature": 0.7,
				},
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	run := correlated[0].Run
	if len(run.GraphSnapshot) == 0 {
		t.Error("GraphSnapshot is empty")
	}
	if len(run.InputSnapshot) == 0 {
		t.Error("InputSnapshot is empty")
	}
	if len(run.ConfigSnapshot) == 0 {
		t.Error("ConfigSnapshot is empty")
	}
}

func TestCorrelator_ExtractTags(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			TraceID:   "trace1",
			SpanID:    "span1",
			Name:      "tagged-operation",
			StartTime: now,
			EndTime:   now.Add(100 * time.Millisecond),
			Attributes: map[string]any{
				"petalflow.run.id": "run-with-tags",
				"tag.environment":  "production",
				"tag.user_id":      "user-123",
				"tag.experiment":   "ab-test-v2",
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	run := correlated[0].Run
	if run.Tags["environment"] != "production" {
		t.Errorf("Tags[environment] = %s, want production", run.Tags["environment"])
	}
	if run.Tags["user_id"] != "user-123" {
		t.Errorf("Tags[user_id] = %s, want user-123", run.Tags["user_id"])
	}
	if run.Tags["experiment"] != "ab-test-v2" {
		t.Errorf("Tags[experiment] = %s", run.Tags["experiment"])
	}
}

func TestCorrelator_ParentRunID(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			TraceID:   "trace1",
			SpanID:    "span1",
			Name:      "replay-run",
			StartTime: now,
			EndTime:   now.Add(100 * time.Millisecond),
			Attributes: map[string]any{
				"petalflow.run.id":        "run-replay",
				"petalflow.parent_run_id": "run-original",
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	run := correlated[0].Run
	if run.ParentRunID == nil {
		t.Fatal("ParentRunID is nil")
	}
	if *run.ParentRunID != "run-original" {
		t.Errorf("ParentRunID = %s, want run-original", *run.ParentRunID)
	}
}

func TestCorrelator_FlushInactiveRuns(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	// Create first run
	spans1 := []*IngestedSpan{
		{
			SpanID:    "span1",
			Name:      "run1",
			StartTime: now,
			Attributes: map[string]any{
				"petalflow.run.id": "run-1",
			},
		},
	}
	c.ProcessSpans(ctx, spans1)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Create second run
	spans2 := []*IngestedSpan{
		{
			SpanID:    "span2",
			Name:      "run2",
			StartTime: now,
			Attributes: map[string]any{
				"petalflow.run.id": "run-2",
			},
		},
	}
	c.ProcessSpans(ctx, spans2)

	if c.ActiveRunCount() != 2 {
		t.Errorf("ActiveRunCount = %d, want 2", c.ActiveRunCount())
	}

	// Flush with very short max age
	flushed := c.FlushInactiveRuns(5 * time.Millisecond)

	// run-1 should be flushed, run-2 should remain
	if len(flushed) != 1 {
		t.Errorf("flushed %d runs, want 1", len(flushed))
	}
	if c.ActiveRunCount() != 1 {
		t.Errorf("ActiveRunCount after flush = %d, want 1", c.ActiveRunCount())
	}
}

func TestCorrelator_IsRootSpan(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			SpanID:     "span1",
			Name:       "no-parent",
			StartTime:  now,
			Attributes: map[string]any{"petalflow.run.id": "run-1"},
		},
		{
			SpanID:       "span2",
			ParentSpanID: "span1",
			Name:         "has-parent",
			StartTime:    now,
			Attributes:   map[string]any{"petalflow.run.id": "run-2"},
		},
		{
			SpanID:       "span3",
			ParentSpanID: "span1",
			Name:         "explicit-root",
			StartTime:    now,
			Attributes: map[string]any{
				"petalflow.run.id":   "run-3",
				"petalflow.run.root": true,
			},
		},
	}

	correlated, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	if !correlated[0].IsRoot {
		t.Error("span1 should be root (no parent)")
	}
	if correlated[1].IsRoot {
		t.Error("span2 should not be root (has parent)")
	}
	if !correlated[2].IsRoot {
		t.Error("span3 should be root (explicit marker)")
	}
}

func TestCorrelator_CompleteRun(t *testing.T) {
	s := setupTestStore(t)
	c := NewCorrelator(CorrelatorConfig{Store: s})

	ctx := context.Background()
	now := time.Now()

	spans := []*IngestedSpan{
		{
			SpanID:    "span1",
			Name:      "operation",
			StartTime: now,
			Attributes: map[string]any{
				"petalflow.run.id": "run-to-complete",
			},
		},
	}

	_, err := c.ProcessSpans(ctx, spans)
	if err != nil {
		t.Fatalf("ProcessSpans error: %v", err)
	}

	completedAt := now.Add(500 * time.Millisecond)
	err = c.CompleteRun(ctx, "run-to-complete", store.RunStatusCompleted, completedAt)
	if err != nil {
		t.Fatalf("CompleteRun error: %v", err)
	}

	run, err := s.GetRun(ctx, "run-to-complete")
	if err != nil {
		t.Fatalf("GetRun error: %v", err)
	}

	if run.Status != store.RunStatusCompleted {
		t.Errorf("Status = %s, want completed", run.Status)
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt is nil")
	}
	if run.DurationMs != 500 {
		t.Errorf("DurationMs = %d, want 500", run.DurationMs)
	}
}
