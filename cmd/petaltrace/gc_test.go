package petaltrace

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/petal-labs/petaltrace/store"
)

func TestRunGCNeverRetentionPreservesOldRuns(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		name := "normal"
		if dryRun {
			name = "dry-run"
		}
		t.Run(name, func(t *testing.T) {
			tempDir := t.TempDir()
			dbPath := filepath.Join(tempDir, "petaltrace.db")
			configPath := filepath.Join(tempDir, "petaltrace.yaml")
			configData := []byte("retention:\n  default: never\nstorage:\n  sqlite:\n    path: " + strconv.Quote(dbPath) + "\n")
			if err := os.WriteFile(configPath, configData, 0600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
			if err != nil {
				t.Fatalf("open test store: %v", err)
			}
			oldRun := &store.Run{
				ID:           "old-run",
				WorkflowID:   "workflow",
				WorkflowName: "old-workflow",
				SourceKind:   "test",
				Status:       store.RunStatusCompleted,
				StartedAt:    time.Now().Add(-60 * 24 * time.Hour),
			}
			if err := traceStore.CreateRun(context.Background(), oldRun); err != nil {
				_ = traceStore.Close()
				t.Fatalf("create old run: %v", err)
			}
			if err := traceStore.Close(); err != nil {
				t.Fatalf("close test store: %v", err)
			}

			previousConfig, previousRetain := cfgFile, gcRetain
			previousDryRun, previousIncludeStarred, previousVerbose := gcDryRun, gcIncludeStarred, gcVerbose
			t.Cleanup(func() {
				cfgFile = previousConfig
				gcRetain = previousRetain
				gcDryRun = previousDryRun
				gcIncludeStarred = previousIncludeStarred
				gcVerbose = previousVerbose
			})
			cfgFile = configPath
			gcRetain = ""
			gcDryRun = dryRun
			gcIncludeStarred = false
			gcVerbose = false

			if err := runGC(gcCmd, nil); err != nil {
				t.Fatalf("runGC() error = %v", err)
			}

			traceStore, err = store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
			if err != nil {
				t.Fatalf("reopen test store: %v", err)
			}
			defer traceStore.Close()

			got, err := traceStore.GetRun(context.Background(), oldRun.ID)
			if err != nil {
				t.Fatalf("get old run: %v", err)
			}
			if got == nil {
				t.Fatal("runGC deleted a run with retention.default: never")
			}
		})
	}
}
