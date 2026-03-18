package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCreateAndGetRun(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	completedAt := now.Add(5 * time.Second)
	parentID := "parent_123"

	run := &Run{
		WorkflowID:      "wf_001",
		WorkflowName:    "research_pipeline",
		WorkflowVersion: "1.0.0",
		SourceKind:      "agent_workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		CompletedAt:     &completedAt,
		DurationMs:      5000,
		GraphSnapshot:   json.RawMessage(`{"nodes": []}`),
		InputSnapshot:   json.RawMessage(`{"input": "test"}`),
		ConfigSnapshot:  json.RawMessage(`{"model": "claude"}`),
		TotalTokens: TokenSummary{
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
		},
		EstimatedCost: CostEstimate{
			Currency: "USD",
			Total:    0.05,
		},
		NodeCount:     3,
		ErrorCount:    0,
		Tags:          map[string]string{"env": "test"},
		TriggerSource: "cli",
		ParentRunID:   &parentID,
		Starred:       true,
	}

	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	if run.ID == "" {
		t.Error("run.ID was not set")
	}

	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got == nil {
		t.Fatal("GetRun() returned nil")
	}

	if got.ID != run.ID {
		t.Errorf("ID = %s, want %s", got.ID, run.ID)
	}

	if got.WorkflowName != "research_pipeline" {
		t.Errorf("WorkflowName = %s, want research_pipeline", got.WorkflowName)
	}

	if got.Status != RunStatusCompleted {
		t.Errorf("Status = %s, want %s", got.Status, RunStatusCompleted)
	}

	if got.TotalTokens.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500", got.TotalTokens.TotalTokens)
	}

	if !got.Starred {
		t.Error("Starred = false, want true")
	}

	if got.ParentRunID == nil || *got.ParentRunID != parentID {
		t.Error("ParentRunID mismatch")
	}
}

func TestGetRunNotFound(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()

	got, err := store.GetRun(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got != nil {
		t.Errorf("GetRun() = %v, want nil", got)
	}
}

func TestUpdateRun(t *testing.T) {
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

	completedAt := now.Add(10 * time.Second)
	run.Status = RunStatusCompleted
	run.CompletedAt = &completedAt
	run.DurationMs = 10000
	run.TotalTokens.TotalTokens = 2000

	if err := store.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun() error = %v", err)
	}

	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got.Status != RunStatusCompleted {
		t.Errorf("Status = %s, want %s", got.Status, RunStatusCompleted)
	}

	if got.DurationMs != 10000 {
		t.Errorf("DurationMs = %d, want 10000", got.DurationMs)
	}

	if got.TotalTokens.TotalTokens != 2000 {
		t.Errorf("TotalTokens = %d, want 2000", got.TotalTokens.TotalTokens)
	}
}

func TestDeleteRun(t *testing.T) {
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

	if err := store.DeleteRun(ctx, run.ID); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}

	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}

	if got != nil {
		t.Error("run was not deleted")
	}
}

func TestDeleteRunNotFound(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()

	err := store.DeleteRun(ctx, "nonexistent")
	if err == nil {
		t.Error("DeleteRun() expected error for nonexistent run")
	}
}

func TestListRuns(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	runs := []*Run{
		{WorkflowID: "wf_001", WorkflowName: "pipeline_a", SourceKind: "agent_workflow", Status: RunStatusCompleted, StartedAt: now.Add(-3 * time.Hour)},
		{WorkflowID: "wf_002", WorkflowName: "pipeline_b", SourceKind: "graph", Status: RunStatusFailed, StartedAt: now.Add(-2 * time.Hour)},
		{WorkflowID: "wf_001", WorkflowName: "pipeline_a", SourceKind: "agent_workflow", Status: RunStatusCompleted, StartedAt: now.Add(-1 * time.Hour)},
	}

	for _, run := range runs {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
	}

	t.Run("list all", func(t *testing.T) {
		results, cursor, err := store.ListRuns(ctx, ListRunsOptions{})
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}

		if len(results) != 3 {
			t.Errorf("len(results) = %d, want 3", len(results))
		}

		if cursor != "" {
			t.Errorf("cursor = %s, want empty", cursor)
		}
	})

	t.Run("filter by workflow", func(t *testing.T) {
		results, _, err := store.ListRuns(ctx, ListRunsOptions{
			WorkflowID: "wf_001",
		})
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("len(results) = %d, want 2", len(results))
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		results, _, err := store.ListRuns(ctx, ListRunsOptions{
			Status: RunStatusFailed,
		})
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}

		if len(results) != 1 {
			t.Errorf("len(results) = %d, want 1", len(results))
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		since := now.Add(-2*time.Hour - 30*time.Minute)
		results, _, err := store.ListRuns(ctx, ListRunsOptions{
			Since: &since,
		})
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("len(results) = %d, want 2", len(results))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		results, cursor, err := store.ListRuns(ctx, ListRunsOptions{
			Limit: 2,
		})
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("len(results) = %d, want 2", len(results))
		}

		if cursor == "" {
			t.Error("cursor is empty, expected non-empty")
		}
	})

	t.Run("sort by duration", func(t *testing.T) {
		results, _, err := store.ListRuns(ctx, ListRunsOptions{
			SortBy:    "started_at",
			SortOrder: "asc",
		})
		if err != nil {
			t.Fatalf("ListRuns() error = %v", err)
		}

		if len(results) != 3 {
			t.Errorf("len(results) = %d, want 3", len(results))
		}

		if results[0].StartedAt.After(results[1].StartedAt) {
			t.Error("results not sorted by started_at asc")
		}
	})
}

func TestListRunsWithWorkflowName(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	runs := []*Run{
		{WorkflowID: "wf_001", WorkflowName: "research_pipeline", SourceKind: "graph", Status: RunStatusCompleted, StartedAt: now},
		{WorkflowID: "wf_002", WorkflowName: "writer_pipeline", SourceKind: "graph", Status: RunStatusCompleted, StartedAt: now},
	}

	for _, run := range runs {
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun() error = %v", err)
		}
	}

	results, _, err := store.ListRuns(ctx, ListRunsOptions{
		WorkflowName: "research",
	})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}

	if results[0].WorkflowName != "research_pipeline" {
		t.Errorf("WorkflowName = %s, want research_pipeline", results[0].WorkflowName)
	}
}
