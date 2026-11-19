package store

import (
	"context"
	"testing"
	"time"
)

func TestReplayRecordLifecycle(t *testing.T) {
	traceStore := testStore(t)
	defer traceStore.Close()

	ctx := context.Background()
	sourceRun := &Run{
		ID:           "replay-source",
		WorkflowID:   "workflow",
		WorkflowName: "replay test",
		SourceKind:   "test",
		Status:       RunStatusCompleted,
		StartedAt:    time.Now().UTC(),
	}
	if err := traceStore.CreateRun(ctx, sourceRun); err != nil {
		t.Fatalf("CreateRun(): %v", err)
	}

	startedAt := time.Now().UTC().Truncate(time.Microsecond)
	replay := &ReplayRecord{
		ID:          "replay-1",
		SourceRunID: sourceRun.ID,
		Mode:        "mocked",
		Status:      "running",
		StartedAt:   startedAt,
	}
	if err := traceStore.CreateReplay(ctx, replay); err != nil {
		t.Fatalf("CreateReplay(): %v", err)
	}

	got, err := traceStore.GetReplay(ctx, replay.ID)
	if err != nil {
		t.Fatalf("GetReplay(): %v", err)
	}
	if got == nil || got.SourceRunID != sourceRun.ID || got.Mode != replay.Mode || got.Status != replay.Status {
		t.Fatalf("GetReplay() = %#v, want source %q, mode %q, status %q", got, sourceRun.ID, replay.Mode, replay.Status)
	}
	if !got.StartedAt.Equal(startedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, startedAt)
	}

	completedAt := startedAt.Add(time.Second)
	replay.NewRunID = "replay-run"
	replay.DiffID = "replay-diff"
	replay.Status = "failed"
	replay.Error = "execution failed"
	replay.CompletedAt = &completedAt
	if err := traceStore.UpdateReplay(ctx, replay); err != nil {
		t.Fatalf("UpdateReplay(): %v", err)
	}

	got, err = traceStore.GetReplay(ctx, replay.ID)
	if err != nil {
		t.Fatalf("GetReplay() after update: %v", err)
	}
	if got == nil || got.NewRunID != replay.NewRunID || got.DiffID != replay.DiffID ||
		got.Status != replay.Status || got.Error != replay.Error || got.CompletedAt == nil ||
		!got.CompletedAt.Equal(completedAt) {
		t.Fatalf("updated replay = %#v, want %#v", got, replay)
	}

	sourceReplays, err := traceStore.ListReplays(ctx, sourceRun.ID)
	if err != nil {
		t.Fatalf("ListReplays(source): %v", err)
	}
	if len(sourceReplays) != 1 || sourceReplays[0].ID != replay.ID {
		t.Fatalf("ListReplays(source) = %#v, want replay %q", sourceReplays, replay.ID)
	}
	allReplays, err := traceStore.ListReplays(ctx, "")
	if err != nil {
		t.Fatalf("ListReplays(all): %v", err)
	}
	if len(allReplays) != 1 {
		t.Fatalf("ListReplays(all) returned %d replays, want 1", len(allReplays))
	}

	var foreignKeys int
	if err := traceStore.DB().QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("check foreign key enforcement: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign key enforcement = %d, want 1", foreignKeys)
	}

	if err := traceStore.DeleteRun(ctx, sourceRun.ID); err != nil {
		t.Fatalf("DeleteRun(source): %v", err)
	}
	allReplays, err = traceStore.ListReplays(ctx, "")
	if err != nil {
		t.Fatalf("ListReplays() after source deletion: %v", err)
	}
	if len(allReplays) != 0 {
		t.Fatalf("ListReplays() after source deletion returned %d replays, want 0", len(allReplays))
	}
}
