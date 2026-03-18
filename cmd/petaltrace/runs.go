package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/internal/config"
	"github.com/petal-labs/petaltrace/store"
)

var runsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Manage workflow runs",
	Long:  `Commands for listing, viewing, and managing workflow runs.`,
}

var runsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent runs",
	Long: `List recent workflow runs with optional filtering.

Examples:
  petaltrace runs list
  petaltrace runs list --workflow email-processor
  petaltrace runs list --status completed --since 24h
  petaltrace runs list --limit 20 --json`,
	RunE: runRunsList,
}

var runsShowCmd = &cobra.Command{
	Use:   "show <run-id>",
	Short: "Show run details",
	Long: `Display detailed information about a specific run.

Examples:
  petaltrace runs show run-abc123
  petaltrace runs show run-abc123 --spans
  petaltrace runs show run-abc123 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runRunsShow,
}

var runsDeleteCmd = &cobra.Command{
	Use:   "delete <run-id>",
	Short: "Delete a run",
	Long: `Delete a run and all its associated spans.

Examples:
  petaltrace runs delete run-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runRunsDelete,
}

// Flags
var (
	listWorkflow    string
	listStatus      string
	listSince       string
	listLimit       int
	listJSON        bool
	showSpans       bool
	showNode        string
	showJSON        bool
	deleteConfirm   bool
)

func init() {
	rootCmd.AddCommand(runsCmd)
	runsCmd.AddCommand(runsListCmd)
	runsCmd.AddCommand(runsShowCmd)
	runsCmd.AddCommand(runsDeleteCmd)

	// List flags
	runsListCmd.Flags().StringVar(&listWorkflow, "workflow", "", "Filter by workflow name")
	runsListCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (running, completed, failed)")
	runsListCmd.Flags().StringVar(&listSince, "since", "", "Show runs since duration (e.g., 24h, 7d)")
	runsListCmd.Flags().IntVar(&listLimit, "limit", 50, "Maximum number of runs to show")
	runsListCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	// Show flags
	runsShowCmd.Flags().BoolVar(&showSpans, "spans", false, "Show span tree")
	runsShowCmd.Flags().StringVar(&showNode, "node", "", "Filter spans by node ID")
	runsShowCmd.Flags().BoolVar(&showJSON, "json", false, "Output as JSON")

	// Delete flags
	runsDeleteCmd.Flags().BoolVarP(&deleteConfirm, "yes", "y", false, "Skip confirmation")
}

func runRunsList(cmd *cobra.Command, args []string) error {
	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	opts := store.ListRunsOptions{
		Limit: listLimit,
	}

	if listWorkflow != "" {
		opts.WorkflowName = listWorkflow
	}

	if listStatus != "" {
		opts.Status = store.RunStatus(listStatus)
	}

	if listSince != "" {
		d, err := time.ParseDuration(listSince)
		if err != nil {
			// Try parsing as days
			if strings.HasSuffix(listSince, "d") {
				days := strings.TrimSuffix(listSince, "d")
				var n int
				fmt.Sscanf(days, "%d", &n)
				d = time.Duration(n) * 24 * time.Hour
			} else {
				return fmt.Errorf("invalid duration: %s", listSince)
			}
		}
		since := time.Now().Add(-d)
		opts.Since = &since
	}

	runs, _, err := traceStore.ListRuns(ctx, opts)
	if err != nil {
		return fmt.Errorf("listing runs: %w", err)
	}

	if listJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(runs)
	}

	if len(runs) == 0 {
		fmt.Println("No runs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tWORKFLOW\tRUN ID\tDURATION\tTOKENS\tCOST\tSTARTED")

	for _, run := range runs {
		status := statusIcon(run.Status)
		duration := formatDuration(run.DurationMs)
		tokens := fmt.Sprintf("%d", run.TotalTokens.TotalTokens)
		cost := fmt.Sprintf("$%.4f", run.EstimatedCost.Total)
		started := run.StartedAt.Format("2006-01-02 15:04:05")

		// Truncate workflow name if too long
		workflow := run.WorkflowName
		if len(workflow) > 25 {
			workflow = workflow[:22] + "..."
		}

		// Truncate run ID for display
		runID := run.ID
		if len(runID) > 20 {
			runID = runID[:17] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			status, workflow, runID, duration, tokens, cost, started)
	}

	w.Flush()
	fmt.Printf("\nShowing %d runs\n", len(runs))

	return nil
}

func runRunsShow(cmd *cobra.Command, args []string) error {
	runID := args[0]

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	run, err := traceStore.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("getting run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	if showJSON {
		output := map[string]any{
			"run": run,
		}

		if showSpans {
			spans, err := traceStore.GetSpanTree(ctx, runID)
			if err != nil {
				return fmt.Errorf("getting spans: %w", err)
			}
			output["spans"] = spans
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Display run header
	fmt.Printf("Run: %s\n", run.ID)
	fmt.Printf("Status: %s %s\n", statusIcon(run.Status), run.Status)
	fmt.Printf("Workflow: %s", run.WorkflowName)
	if run.WorkflowVersion != "" {
		fmt.Printf(" (v%s)", run.WorkflowVersion)
	}
	fmt.Println()
	fmt.Printf("Started: %s\n", run.StartedAt.Format("2006-01-02 15:04:05 MST"))
	if run.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", run.CompletedAt.Format("2006-01-02 15:04:05 MST"))
	}
	fmt.Printf("Duration: %s\n", formatDuration(run.DurationMs))
	fmt.Println()

	// Token summary
	fmt.Println("Tokens:")
	fmt.Printf("  Input:  %d\n", run.TotalTokens.InputTokens)
	fmt.Printf("  Output: %d\n", run.TotalTokens.OutputTokens)
	if run.TotalTokens.CacheReadTokens > 0 {
		fmt.Printf("  Cache Read: %d\n", run.TotalTokens.CacheReadTokens)
	}
	if run.TotalTokens.CacheWriteTokens > 0 {
		fmt.Printf("  Cache Write: %d\n", run.TotalTokens.CacheWriteTokens)
	}
	fmt.Printf("  Total:  %d\n", run.TotalTokens.TotalTokens)
	fmt.Println()

	// Cost summary
	fmt.Printf("Estimated Cost: $%.4f\n", run.EstimatedCost.Total)
	if len(run.EstimatedCost.ByProvider) > 0 {
		fmt.Println("  By Provider:")
		for provider, cost := range run.EstimatedCost.ByProvider {
			fmt.Printf("    %s: $%.4f\n", provider, cost)
		}
	}
	fmt.Println()

	// Tags
	if len(run.Tags) > 0 {
		fmt.Println("Tags:")
		for k, v := range run.Tags {
			fmt.Printf("  %s: %s\n", k, v)
		}
		fmt.Println()
	}

	// Parent run (for replays)
	if run.ParentRunID != nil {
		fmt.Printf("Replay of: %s\n\n", *run.ParentRunID)
	}

	// Span tree
	if showSpans {
		spans, err := traceStore.GetSpanTree(ctx, runID)
		if err != nil {
			return fmt.Errorf("getting spans: %w", err)
		}

		// Filter by node if specified
		if showNode != "" {
			var filtered []*store.Span
			for _, s := range spans {
				if s.Node != nil && s.Node.NodeID == showNode {
					filtered = append(filtered, s)
				}
			}
			spans = filtered
		}

		fmt.Printf("Spans (%d):\n", len(spans))
		printSpanTree(spans, "")
	}

	return nil
}

func runRunsDelete(cmd *cobra.Command, args []string) error {
	runID := args[0]

	if !deleteConfirm {
		fmt.Printf("Delete run %s? [y/N] ", runID)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	if err := traceStore.DeleteRun(ctx, runID); err != nil {
		return fmt.Errorf("deleting run: %w", err)
	}

	fmt.Printf("Deleted run %s\n", runID)
	return nil
}

func openStore() (store.TraceStore, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	dbPath := cfg.Storage.SQLite.Path
	if dbPath == "" {
		dbPath = "./data/petaltrace.db"
	}

	return store.NewSQLiteStore(store.SQLiteOptions{
		Path:    dbPath,
		WALMode: cfg.Storage.SQLite.WALMode,
	})
}

func statusIcon(status store.RunStatus) string {
	switch status {
	case store.RunStatusRunning:
		return "⏳"
	case store.RunStatusCompleted:
		return "✓"
	case store.RunStatusFailed:
		return "✗"
	case store.RunStatusCancelled:
		return "⊘"
	default:
		return "?"
	}
}

func formatDuration(ms int64) string {
	if ms == 0 {
		return "-"
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Second {
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func printSpanTree(spans []*store.Span, indent string) {
	// Build parent-child map
	children := make(map[string][]*store.Span)
	var roots []*store.Span

	for _, s := range spans {
		if s.ParentID == nil {
			roots = append(roots, s)
		} else {
			children[*s.ParentID] = append(children[*s.ParentID], s)
		}
	}

	// Print tree recursively
	var printNode func(s *store.Span, prefix string, isLast bool)
	printNode = func(s *store.Span, prefix string, isLast bool) {
		connector := "├─"
		if isLast {
			connector = "└─"
		}

		kindIcon := spanKindIcon(s.Kind)
		statusIcon := spanStatusIcon(s.Status)
		duration := formatDuration(s.DurationMs)

		fmt.Printf("%s%s %s %s %s (%s)\n", prefix, connector, statusIcon, kindIcon, s.Name, duration)

		childPrefix := prefix
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}

		kids := children[s.ID]
		for i, child := range kids {
			printNode(child, childPrefix, i == len(kids)-1)
		}
	}

	for i, root := range roots {
		printNode(root, "", i == len(roots)-1)
	}
}

func spanKindIcon(kind store.SpanKind) string {
	switch kind {
	case store.SpanKindLLM:
		return "🤖"
	case store.SpanKindNode:
		return "📦"
	case store.SpanKindTool:
		return "🔧"
	case store.SpanKindEdge:
		return "→"
	default:
		return "○"
	}
}

func spanStatusIcon(status store.SpanStatus) string {
	switch status {
	case store.SpanStatusOK:
		return "✓"
	case store.SpanStatusError:
		return "✗"
	default:
		return "○"
	}
}
