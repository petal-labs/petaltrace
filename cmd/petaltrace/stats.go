package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/internal/config"
	"github.com/petal-labs/petaltrace/store"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Display storage and system statistics",
	Long: `Display statistics about the PetalTrace database including
run counts, span counts, storage size, and top workflows.

Examples:
  petaltrace stats
  petaltrace stats --json`,
	RunE: runStats,
}

var statsJSON bool

func init() {
	rootCmd.AddCommand(statsCmd)

	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")
}

func runStats(cmd *cobra.Command, args []string) error {
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

	// Get store stats
	stats, err := traceStore.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	// Get workflow stats
	workflowStats, err := getWorkflowStats(ctx, traceStore)
	if err != nil {
		return fmt.Errorf("getting workflow stats: %w", err)
	}

	if statsJSON {
		output := map[string]any{
			"database":  stats,
			"workflows": workflowStats,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Display stats
	fmt.Println("PetalTrace Statistics")
	fmt.Println("=====================")
	fmt.Println()

	fmt.Println("Database:")
	fmt.Printf("  Path:          %s\n", cfg.Storage.SQLite.Path)
	fmt.Printf("  Size:          %s\n", formatBytes(stats.DatabaseSize))
	fmt.Printf("  Total Runs:    %d\n", stats.RunCount)
	fmt.Printf("  Total Spans:   %d\n", stats.SpanCount)
	fmt.Println()

	if !stats.OldestRun.IsZero() && !stats.NewestRun.IsZero() {
		fmt.Println("Data Range:")
		fmt.Printf("  Oldest Run:    %s\n", stats.OldestRun.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Newest Run:    %s\n", stats.NewestRun.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}

	// Top workflows from stats
	if len(stats.TopWorkflows) > 0 {
		fmt.Println("Top Workflows by Run Count:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  WORKFLOW\tRUNS")
		for _, wf := range stats.TopWorkflows {
			fmt.Fprintf(w, "  %s\t%d\n", truncateString(wf.WorkflowName, 35), wf.RunCount)
		}
		w.Flush()
		fmt.Println()
	}

	// Additional workflow stats with cost
	if len(workflowStats) > 0 {
		fmt.Println("Top Workflows by Cost:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  WORKFLOW\tRUNS\tTOTAL COST")
		limit := 10
		if len(workflowStats) < limit {
			limit = len(workflowStats)
		}
		for i := 0; i < limit; i++ {
			wf := workflowStats[i]
			fmt.Fprintf(w, "  %s\t%d\t$%.4f\n",
				truncateString(wf.Name, 35), wf.RunCount, wf.TotalCost)
		}
		w.Flush()
	}

	return nil
}

type WorkflowStat struct {
	Name      string  `json:"name"`
	RunCount  int     `json:"run_count"`
	TotalCost float64 `json:"total_cost"`
}

func getWorkflowStats(ctx context.Context, s store.TraceStore) ([]WorkflowStat, error) {
	// Get all runs (up to a reasonable limit)
	opts := store.ListRunsOptions{
		Limit: 10000,
	}

	runs, _, err := s.ListRuns(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Aggregate by workflow
	workflowMap := make(map[string]*WorkflowStat)
	for _, run := range runs {
		name := run.WorkflowName
		if name == "" {
			name = "(unknown)"
		}

		stat, ok := workflowMap[name]
		if !ok {
			stat = &WorkflowStat{Name: name}
			workflowMap[name] = stat
		}
		stat.RunCount++
		stat.TotalCost += run.EstimatedCost.Total
	}

	// Convert to slice and sort by cost
	var stats []WorkflowStat
	for _, stat := range workflowMap {
		stats = append(stats, *stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].TotalCost > stats[j].TotalCost
	})

	return stats, nil
}
