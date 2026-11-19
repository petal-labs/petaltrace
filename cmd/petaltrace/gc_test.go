package petaltrace

import (
	"context"
	"fmt"
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

// TestRunGCTraversesAllPages verifies that GC follows ListRuns cursors across
// every result page, so eligible runs on any page are deleted (or reported in
// dry-run) exactly once, while starred runs are preserved unless
// --include-starred is passed.
func TestRunGCTraversesAllPages(t *testing.T) {
	const runCount = 25

	// Eligible runs are ids gc-run-02..gc-run-25 (older than 24h); gc-run-01
	// is recent and must survive every variant. gc-run-03, gc-run-14 and
	// gc-run-20 are starred; the first two are eligible so the starred path
	// is exercised on multiple pages.
	starred := map[string]bool{
		"gc-run-03": true,
		"gc-run-14": true,
		"gc-run-20": true,
	}

	for _, tc := range []struct {
		name           string
		dryRun         bool
		includeStarred bool
		wantSurvivors  []string
	}{
		{
			name:          "default preserves starred",
			wantSurvivors: []string{"gc-run-01", "gc-run-03", "gc-run-14", "gc-run-20"},
		},
		{
			name:           "include-starred deletes starred",
			includeStarred: true,
			wantSurvivors:  []string{"gc-run-01"},
		},
		{
			name:          "dry-run deletes nothing",
			dryRun:        true,
			wantSurvivors: allRunIDs(runCount),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			dbPath := filepath.Join(tempDir, "petaltrace.db")
			configPath := filepath.Join(tempDir, "petaltrace.yaml")
			configData := []byte("retention:\n  default: 24h\nstorage:\n  sqlite:\n    path: " + strconv.Quote(dbPath) + "\n")
			if err := os.WriteFile(configPath, configData, 0600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
			if err != nil {
				t.Fatalf("open test store: %v", err)
			}
			now := time.Now()
			for i := 1; i <= runCount; i++ {
				startedAt := now.Add(-time.Hour) // gc-run-01: recent
				if i >= 2 {
					// Extra hour of margin so GC's own now() cannot pull a
					// run back inside the retention window.
					startedAt = now.Add(-time.Duration(i)*24*time.Hour - time.Hour)
				}
				run := &store.Run{
					ID:           fmt.Sprintf("gc-run-%02d", i),
					WorkflowID:   "workflow",
					WorkflowName: "gc-page-traversal",
					SourceKind:   "test",
					Status:       store.RunStatusCompleted,
					StartedAt:    startedAt,
					Starred:      starred[fmt.Sprintf("gc-run-%02d", i)],
				}
				if err := traceStore.CreateRun(context.Background(), run); err != nil {
					_ = traceStore.Close()
					t.Fatalf("create run %s: %v", run.ID, err)
				}
			}
			if err := traceStore.Close(); err != nil {
				t.Fatalf("close test store: %v", err)
			}

			previousConfig, previousRetain := cfgFile, gcRetain
			previousDryRun, previousIncludeStarred, previousVerbose := gcDryRun, gcIncludeStarred, gcVerbose
			previousPageSize := gcPageSize
			t.Cleanup(func() {
				cfgFile = previousConfig
				gcRetain = previousRetain
				gcDryRun = previousDryRun
				gcIncludeStarred = previousIncludeStarred
				gcVerbose = previousVerbose
				gcPageSize = previousPageSize
			})
			cfgFile = configPath
			gcRetain = ""
			gcDryRun = tc.dryRun
			gcIncludeStarred = tc.includeStarred
			gcVerbose = false
			// 25 runs over a page size of 10 forces three ListRuns pages.
			gcPageSize = 10

			if err := runGC(gcCmd, nil); err != nil {
				t.Fatalf("runGC() error = %v", err)
			}

			traceStore, err = store.NewSQLiteStore(store.SQLiteOptions{Path: dbPath})
			if err != nil {
				t.Fatalf("reopen test store: %v", err)
			}
			defer traceStore.Close()

			remaining, _, err := traceStore.ListRuns(context.Background(), store.ListRunsOptions{Limit: 1000})
			if err != nil {
				t.Fatalf("list runs after gc: %v", err)
			}

			got := make(map[string]bool, len(remaining))
			for _, run := range remaining {
				got[run.ID] = true
			}
			want := make(map[string]bool, len(tc.wantSurvivors))
			for _, id := range tc.wantSurvivors {
				want[id] = true
			}

			if len(remaining) != len(want) {
				t.Errorf("len(remaining) = %d, want %d", len(remaining), len(want))
			}
			for id := range want {
				if !got[id] {
					t.Errorf("run %s was deleted, expected it to survive", id)
				}
			}
			for id := range got {
				if !want[id] {
					t.Errorf("run %s survived, expected it to be deleted", id)
				}
			}
		})
	}
}

func allRunIDs(n int) []string {
	ids := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		ids = append(ids, fmt.Sprintf("gc-run-%02d", i))
	}
	return ids
}
