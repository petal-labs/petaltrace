package petaltrace

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/internal/config"
	"github.com/petal-labs/petaltrace/store"
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Run garbage collection on old runs",
	Long: `Delete runs older than the retention period.

By default, uses the retention period from the configuration file.
Starred runs are preserved regardless of age.

Examples:
  petaltrace gc
  petaltrace gc --retain 30d
  petaltrace gc --dry-run
  petaltrace gc --include-starred`,
	RunE: runGC,
}

var (
	gcRetain         string
	gcDryRun         bool
	gcIncludeStarred bool
	gcVerbose        bool
)

func init() {
	rootCmd.AddCommand(gcCmd)

	gcCmd.Flags().StringVar(&gcRetain, "retain", "", "Override retention period (e.g., 30d, 7d)")
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "Preview deletions without executing")
	gcCmd.Flags().BoolVar(&gcIncludeStarred, "include-starred", false, "Also delete starred runs")
	gcCmd.Flags().BoolVar(&gcVerbose, "verbose", false, "Show each run being deleted")
}

func runGC(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	// Determine retention period
	var retentionDuration time.Duration
	if gcRetain != "" {
		retentionDuration, err = parseDuration(gcRetain)
		if err != nil {
			return fmt.Errorf("invalid retention period: %w", err)
		}
	} else {
		retentionDuration = cfg.Retention.Default.Duration
		if retentionDuration == 0 {
			retentionDuration = 30 * 24 * time.Hour // 30 days default
		}
	}

	cutoffTime := time.Now().Add(-retentionDuration)

	fmt.Printf("Garbage Collection\n")
	fmt.Printf("==================\n")
	fmt.Printf("Retention period: %s\n", formatRetention(retentionDuration))
	fmt.Printf("Deleting runs older than: %s\n", cutoffTime.Format("2006-01-02 15:04:05"))
	if gcIncludeStarred {
		fmt.Printf("Including starred runs: yes\n")
	} else {
		fmt.Printf("Preserving starred runs: yes\n")
	}
	if gcDryRun {
		fmt.Printf("Mode: dry-run (no deletions)\n")
	}
	fmt.Println()

	// Find runs to delete
	opts := store.ListRunsOptions{
		Limit: 10000, // Get all runs
	}

	runs, _, err := traceStore.ListRuns(ctx, opts)
	if err != nil {
		return fmt.Errorf("listing runs: %w", err)
	}

	var toDelete []store.Run
	for _, run := range runs {
		// Check if run is old enough
		if !run.StartedAt.Before(cutoffTime) {
			continue
		}

		// Check starred status
		if run.Starred && !gcIncludeStarred {
			if gcVerbose {
				fmt.Printf("  Skipping starred run: %s\n", run.ID)
			}
			continue
		}

		toDelete = append(toDelete, run)
	}

	if len(toDelete) == 0 {
		fmt.Println("No runs to delete.")
		return nil
	}

	fmt.Printf("Found %d runs to delete.\n\n", len(toDelete))

	if gcDryRun {
		fmt.Println("Runs that would be deleted:")
		for _, run := range toDelete {
			fmt.Printf("  %s  %s  %s  started %s\n",
				statusIcon(run.Status),
				run.ID,
				truncateString(run.WorkflowName, 30),
				run.StartedAt.Format("2006-01-02"))
		}
		fmt.Printf("\nDry run complete. Use without --dry-run to delete.\n")
		return nil
	}

	// Perform deletion
	var deleted, failed int
	for _, run := range toDelete {
		if gcVerbose {
			fmt.Printf("  Deleting: %s (%s)\n", run.ID, run.WorkflowName)
		}

		if err := traceStore.DeleteRun(ctx, run.ID); err != nil {
			fmt.Printf("  Error deleting %s: %v\n", run.ID, err)
			failed++
		} else {
			deleted++
		}
	}

	fmt.Printf("\nGarbage collection complete.\n")
	fmt.Printf("  Deleted: %d runs\n", deleted)
	if failed > 0 {
		fmt.Printf("  Failed:  %d runs\n", failed)
	}

	// Run VACUUM to reclaim space
	stats, err := traceStore.GetStats(ctx)
	if err == nil && stats != nil {
		fmt.Printf("  Database size: %s\n", formatBytes(stats.DatabaseSize))
	}

	return nil
}

func formatRetention(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%d days", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	return d.String()
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
