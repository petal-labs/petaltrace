package replay

import (
	"context"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func TestNewEngine(t *testing.T) {
	ms := newMockStore()
	cfg := EngineConfig{
		Store:        ms,
		PetalFlowURL: "http://localhost:8080",
		Logger:       nil, // Should default to slog.Default()
	}

	engine := NewEngine(cfg)

	if engine == nil {
		t.Fatal("NewEngine() returned nil")
	}

	if engine.store == nil {
		t.Error("engine.store is nil")
	}

	if engine.validator == nil {
		t.Error("engine.validator is nil")
	}

	if engine.liveReplayer == nil {
		t.Error("engine.liveReplayer is nil")
	}

	if engine.mockedReplayer == nil {
		t.Error("engine.mockedReplayer is nil")
	}

	if engine.hybridReplayer == nil {
		t.Error("engine.hybridReplayer is nil")
	}

	if engine.diffEngine == nil {
		t.Error("engine.diffEngine is nil")
	}

	if engine.replays == nil {
		t.Error("engine.replays is nil")
	}
}

func TestEngine_GetReplay(t *testing.T) {
	ms := newMockStore()
	engine := NewEngine(EngineConfig{Store: ms})

	// Add a replay manually
	replay := &Replay{
		ID:          "replay-1",
		SourceRunID: "run-1",
		Status:      ReplayStatusCompleted,
	}
	engine.replays["replay-1"] = replay

	// Found
	r, ok := engine.GetReplay("replay-1")
	if !ok {
		t.Error("expected to find replay-1")
	}
	if r.ID != "replay-1" {
		t.Errorf("ID = %q, want %q", r.ID, "replay-1")
	}

	// Not found
	_, ok = engine.GetReplay("replay-not-found")
	if ok {
		t.Error("expected not to find replay-not-found")
	}
}

func TestEngine_ListReplays(t *testing.T) {
	ms := newMockStore()
	engine := NewEngine(EngineConfig{Store: ms})

	// Empty list
	replays := engine.ListReplays()
	if len(replays) != 0 {
		t.Errorf("len(replays) = %d, want 0", len(replays))
	}

	// Add replays
	engine.replays["replay-1"] = &Replay{ID: "replay-1"}
	engine.replays["replay-2"] = &Replay{ID: "replay-2"}

	replays = engine.ListReplays()
	if len(replays) != 2 {
		t.Errorf("len(replays) = %d, want 2", len(replays))
	}
}

func TestEngine_GetReplaysBySourceRun(t *testing.T) {
	ms := newMockStore()
	engine := NewEngine(EngineConfig{Store: ms})

	// Add replays with different source runs
	engine.replays["replay-1"] = &Replay{ID: "replay-1", SourceRunID: "run-1"}
	engine.replays["replay-2"] = &Replay{ID: "replay-2", SourceRunID: "run-1"}
	engine.replays["replay-3"] = &Replay{ID: "replay-3", SourceRunID: "run-2"}

	// Get replays for run-1
	replays := engine.GetReplaysBySourceRun("run-1")
	if len(replays) != 2 {
		t.Errorf("len(replays) = %d, want 2", len(replays))
	}

	// Get replays for run-2
	replays = engine.GetReplaysBySourceRun("run-2")
	if len(replays) != 1 {
		t.Errorf("len(replays) = %d, want 1", len(replays))
	}

	// Get replays for non-existent run
	replays = engine.GetReplaysBySourceRun("run-not-found")
	if len(replays) != 0 {
		t.Errorf("len(replays) = %d, want 0", len(replays))
	}
}

func TestEngine_RefreshReplayStatus(t *testing.T) {
	ms := newMockStore()
	engine := NewEngine(EngineConfig{Store: ms})
	ctx := context.Background()

	// Test replay not found
	_, err := engine.RefreshReplayStatus(ctx, "not-found")
	if err == nil {
		t.Error("expected error for not found replay")
	}

	// Test already completed replay
	completedReplay := &Replay{
		ID:       "replay-completed",
		NewRunID: "new-run-1",
		Status:   ReplayStatusCompleted,
	}
	engine.replays["replay-completed"] = completedReplay

	r, err := engine.RefreshReplayStatus(ctx, "replay-completed")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if r.Status != ReplayStatusCompleted {
		t.Errorf("Status = %v, want %v", r.Status, ReplayStatusCompleted)
	}

	// Test replay with no NewRunID
	noRunIDReplay := &Replay{
		ID:       "replay-no-runid",
		NewRunID: "",
		Status:   ReplayStatusRunning,
	}
	engine.replays["replay-no-runid"] = noRunIDReplay

	r, err = engine.RefreshReplayStatus(ctx, "replay-no-runid")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if r.Status != ReplayStatusRunning {
		t.Errorf("Status = %v, want %v", r.Status, ReplayStatusRunning)
	}

	// Test replay where run has completed
	ms.runs["new-run-completed"] = &store.Run{
		ID:     "new-run-completed",
		Status: store.RunStatusCompleted,
	}
	runningReplay := &Replay{
		ID:       "replay-running",
		NewRunID: "new-run-completed",
		Status:   ReplayStatusRunning,
	}
	engine.replays["replay-running"] = runningReplay

	r, err = engine.RefreshReplayStatus(ctx, "replay-running")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if r.Status != ReplayStatusCompleted {
		t.Errorf("Status = %v, want %v", r.Status, ReplayStatusCompleted)
	}
	if r.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	// Test replay where run has failed
	ms.runs["new-run-failed"] = &store.Run{
		ID:     "new-run-failed",
		Status: store.RunStatusFailed,
	}
	failingReplay := &Replay{
		ID:       "replay-failing",
		NewRunID: "new-run-failed",
		Status:   ReplayStatusRunning,
	}
	engine.replays["replay-failing"] = failingReplay

	r, err = engine.RefreshReplayStatus(ctx, "replay-failing")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if r.Status != ReplayStatusFailed {
		t.Errorf("Status = %v, want %v", r.Status, ReplayStatusFailed)
	}
}

func TestReplayStruct(t *testing.T) {
	now := time.Now()
	completedAt := now.Add(time.Second)

	replay := &Replay{
		ID:          "replay-123",
		SourceRunID: "source-run",
		NewRunID:    "new-run",
		DiffID:      "diff-1",
		Mode:        ReplayModeLive,
		Status:      ReplayStatusCompleted,
		Error:       "",
		StartedAt:   now,
		CompletedAt: &completedAt,
	}

	if replay.ID != "replay-123" {
		t.Errorf("ID = %q, want %q", replay.ID, "replay-123")
	}

	if replay.SourceRunID != "source-run" {
		t.Errorf("SourceRunID = %q, want %q", replay.SourceRunID, "source-run")
	}

	if replay.Mode != ReplayModeLive {
		t.Errorf("Mode = %v, want %v", replay.Mode, ReplayModeLive)
	}

	duration := replay.CompletedAt.Sub(replay.StartedAt)
	if duration != time.Second {
		t.Errorf("duration = %v, want %v", duration, time.Second)
	}
}
