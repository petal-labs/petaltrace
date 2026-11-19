package petaltrace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/replay"
	"github.com/petal-labs/petaltrace/store"
)

func TestReplayStatusAndDiffCommandsUsePersistedReplay(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "petaltrace.db")
	configPath := filepath.Join(dir, "petaltrace.yaml")
	configContents := "storage:\n  driver: sqlite\n  sqlite:\n    path: " + dbPath + "\n"
	if err := os.WriteFile(configPath, []byte(configContents), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for _, runID := range []string{"source-run", "new-run"} {
		if err := traceStore.CreateRun(ctx, &store.Run{
			ID:           runID,
			WorkflowID:   "workflow",
			WorkflowName: "Replay persistence test",
			SourceKind:   "test",
			Status:       store.RunStatusCompleted,
			StartedAt:    now,
		}); err != nil {
			_ = traceStore.Close()
			t.Fatalf("create run %q: %v", runID, err)
		}
	}
	completedAt := now.Add(time.Second)
	if err := traceStore.CreateReplay(ctx, &store.ReplayRecord{
		ID:          "replay-1",
		SourceRunID: "source-run",
		NewRunID:    "new-run",
		Mode:        string(replay.ReplayModeMocked),
		Status:      string(replay.ReplayStatusCompleted),
		StartedAt:   now,
		CompletedAt: &completedAt,
	}); err != nil {
		_ = traceStore.Close()
		t.Fatalf("create replay: %v", err)
	}
	if err := traceStore.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	oldConfigFile, oldReplayJSON := cfgFile, replayJSON
	cfgFile, replayJSON = configPath, true
	defer func() {
		cfgFile, replayJSON = oldConfigFile, oldReplayJSON
	}()

	statusCmd := &cobra.Command{}
	statusCmd.SetContext(ctx)
	if err := runReplayStatus(statusCmd, []string{"replay-1"}); err != nil {
		t.Fatalf("runReplayStatus(): %v", err)
	}
	diffCmd := &cobra.Command{}
	diffCmd.SetContext(ctx)
	if err := runReplayDiffCmd(diffCmd, []string{"replay-1"}); err != nil {
		t.Fatalf("runReplayDiffCmd(): %v", err)
	}

	reopenedStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopenedStore.Close()
	persistedReplay, err := reopenedStore.GetReplay(ctx, "replay-1")
	if err != nil {
		t.Fatalf("GetReplay() after CLI diff: %v", err)
	}
	if persistedReplay == nil || persistedReplay.DiffID == "" {
		t.Fatalf("persisted replay after CLI diff = %#v, want a diff ID", persistedReplay)
	}
}
