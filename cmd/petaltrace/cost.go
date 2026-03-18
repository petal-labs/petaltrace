package petaltrace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/store"
)

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Cost analysis commands",
	Long:  `Commands for analyzing token costs across runs.`,
}

var costSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Aggregate cost metrics across runs",
	Long: `Display aggregate cost metrics across all runs within a time window.

Examples:
  petaltrace cost summary
  petaltrace cost summary --since 24h
  petaltrace cost summary --since 7d --group-by workflow
  petaltrace cost summary --group-by provider --json`,
	RunE: runCostSummary,
}

var costRunCmd = &cobra.Command{
	Use:   "run <run-id>",
	Short: "Per-run cost breakdown",
	Long: `Display detailed cost breakdown for a specific run.

Examples:
  petaltrace cost run run-abc123
  petaltrace cost run run-abc123 --by-node
  petaltrace cost run run-abc123 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runCostRun,
}

var (
	costSince   string
	costGroupBy string
	costJSON    bool
	costByNode  bool
)

func init() {
	rootCmd.AddCommand(costCmd)
	costCmd.AddCommand(costSummaryCmd)
	costCmd.AddCommand(costRunCmd)

	// Summary flags
	costSummaryCmd.Flags().StringVar(&costSince, "since", "7d", "Time window (e.g., 24h, 7d, 30d)")
	costSummaryCmd.Flags().StringVar(&costGroupBy, "group-by", "", "Group by: workflow, provider, model")
	costSummaryCmd.Flags().BoolVar(&costJSON, "json", false, "Output as JSON")

	// Run flags
	costRunCmd.Flags().BoolVar(&costByNode, "by-node", false, "Show breakdown by node")
	costRunCmd.Flags().BoolVar(&costJSON, "json", false, "Output as JSON")
}

func runCostSummary(cmd *cobra.Command, args []string) error {
	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	// Parse time window
	d, err := parseDuration(costSince)
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}
	since := time.Now().Add(-d)

	// Fetch runs within time window
	opts := store.ListRunsOptions{
		Since: &since,
		Limit: 10000, // Get all runs in window
	}

	runs, _, err := traceStore.ListRuns(ctx, opts)
	if err != nil {
		return fmt.Errorf("listing runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No runs found in the specified time window.")
		return nil
	}

	// Calculate summary
	summary := calculateCostSummary(runs, costGroupBy)

	if costJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	// Display summary
	fmt.Printf("Cost Summary (since %s)\n", since.Format("2006-01-02 15:04"))
	fmt.Printf("=================================\n\n")

	fmt.Printf("Total Runs:    %d\n", summary.TotalRuns)
	fmt.Printf("Total Tokens:  %d\n", summary.TotalTokens)
	fmt.Printf("Total Cost:    $%.4f\n", summary.TotalCost)
	fmt.Println()

	fmt.Println("Token Breakdown:")
	fmt.Printf("  Input:       %d\n", summary.InputTokens)
	fmt.Printf("  Output:      %d\n", summary.OutputTokens)
	if summary.CacheReadTokens > 0 {
		fmt.Printf("  Cache Read:  %d\n", summary.CacheReadTokens)
	}
	if summary.CacheWriteTokens > 0 {
		fmt.Printf("  Cache Write: %d\n", summary.CacheWriteTokens)
	}
	fmt.Println()

	// Display groupings
	if len(summary.ByWorkflow) > 0 && (costGroupBy == "workflow" || costGroupBy == "") {
		fmt.Println("By Workflow:")
		printCostTable(summary.ByWorkflow)
	}

	if len(summary.ByProvider) > 0 && (costGroupBy == "provider" || costGroupBy == "") {
		fmt.Println("By Provider:")
		printCostTable(summary.ByProvider)
	}

	if len(summary.ByModel) > 0 && costGroupBy == "model" {
		fmt.Println("By Model:")
		printCostTable(summary.ByModel)
	}

	return nil
}

type CostSummary struct {
	TotalRuns        int                  `json:"total_runs"`
	TotalTokens      int64                `json:"total_tokens"`
	TotalCost        float64              `json:"total_cost"`
	InputTokens      int64                `json:"input_tokens"`
	OutputTokens     int64                `json:"output_tokens"`
	CacheReadTokens  int64                `json:"cache_read_tokens"`
	CacheWriteTokens int64                `json:"cache_write_tokens"`
	ByWorkflow       map[string]CostGroup `json:"by_workflow,omitempty"`
	ByProvider       map[string]CostGroup `json:"by_provider,omitempty"`
	ByModel          map[string]CostGroup `json:"by_model,omitempty"`
}

type CostGroup struct {
	Runs   int     `json:"runs"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
}

func calculateCostSummary(runs []store.Run, groupBy string) *CostSummary {
	summary := &CostSummary{
		TotalRuns:  len(runs),
		ByWorkflow: make(map[string]CostGroup),
		ByProvider: make(map[string]CostGroup),
		ByModel:    make(map[string]CostGroup),
	}

	for _, run := range runs {
		summary.TotalTokens += int64(run.TotalTokens.TotalTokens)
		summary.TotalCost += run.EstimatedCost.Total
		summary.InputTokens += int64(run.TotalTokens.InputTokens)
		summary.OutputTokens += int64(run.TotalTokens.OutputTokens)
		summary.CacheReadTokens += int64(run.TotalTokens.CacheReadTokens)
		summary.CacheWriteTokens += int64(run.TotalTokens.CacheWriteTokens)

		// By workflow
		wf := run.WorkflowName
		if wf == "" {
			wf = "(unknown)"
		}
		g := summary.ByWorkflow[wf]
		g.Runs++
		g.Tokens += int64(run.TotalTokens.TotalTokens)
		g.Cost += run.EstimatedCost.Total
		summary.ByWorkflow[wf] = g

		// By provider
		for provider, cost := range run.EstimatedCost.ByProvider {
			p := summary.ByProvider[provider]
			p.Runs++
			p.Cost += cost
			summary.ByProvider[provider] = p
		}

		// By model
		for model, cost := range run.EstimatedCost.ByModel {
			m := summary.ByModel[model]
			m.Runs++
			m.Cost += cost
			summary.ByModel[model] = m
		}
	}

	return summary
}

func printCostTable(groups map[string]CostGroup) {
	// Sort by cost descending
	type kv struct {
		Key   string
		Value CostGroup
	}
	var sorted []kv
	for k, v := range groups {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value.Cost > sorted[j].Value.Cost
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tRUNS\tTOKENS\tCOST")
	for _, kv := range sorted {
		fmt.Fprintf(w, "  %s\t%d\t%d\t$%.4f\n",
			truncateString(kv.Key, 30), kv.Value.Runs, kv.Value.Tokens, kv.Value.Cost)
	}
	w.Flush()
	fmt.Println()
}

func runCostRun(cmd *cobra.Command, args []string) error {
	runID := args[0]

	traceStore, err := openStore()
	if err != nil {
		return err
	}
	defer traceStore.Close()

	ctx := context.Background()

	// Get run
	run, err := traceStore.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("getting run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	// Get LLM spans for detailed breakdown
	spans, err := traceStore.GetSpansByKind(ctx, runID, store.SpanKindLLM)
	if err != nil {
		return fmt.Errorf("getting spans: %w", err)
	}

	// Calculate per-node costs
	nodeCosts := calculateNodeCosts(spans)

	if costJSON {
		output := map[string]any{
			"run_id":         run.ID,
			"workflow":       run.WorkflowName,
			"total_tokens":   run.TotalTokens,
			"estimated_cost": run.EstimatedCost,
			"llm_spans":      len(spans),
		}
		if costByNode {
			output["by_node"] = nodeCosts
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Display cost breakdown
	fmt.Printf("Cost Breakdown: %s\n", runID)
	fmt.Printf("Workflow: %s\n", run.WorkflowName)
	fmt.Printf("===================================\n\n")

	fmt.Printf("Total Cost: $%.4f\n\n", run.EstimatedCost.Total)

	fmt.Println("Token Summary:")
	fmt.Printf("  Input:       %d\n", run.TotalTokens.InputTokens)
	fmt.Printf("  Output:      %d\n", run.TotalTokens.OutputTokens)
	if run.TotalTokens.CacheReadTokens > 0 {
		fmt.Printf("  Cache Read:  %d\n", run.TotalTokens.CacheReadTokens)
	}
	if run.TotalTokens.CacheWriteTokens > 0 {
		fmt.Printf("  Cache Write: %d\n", run.TotalTokens.CacheWriteTokens)
	}
	fmt.Printf("  Total:       %d\n", run.TotalTokens.TotalTokens)
	fmt.Println()

	// By provider
	if len(run.EstimatedCost.ByProvider) > 0 {
		fmt.Println("By Provider:")
		for provider, cost := range run.EstimatedCost.ByProvider {
			fmt.Printf("  %s: $%.4f\n", provider, cost)
		}
		fmt.Println()
	}

	// By model
	if len(run.EstimatedCost.ByModel) > 0 {
		fmt.Println("By Model:")
		for model, cost := range run.EstimatedCost.ByModel {
			fmt.Printf("  %s: $%.4f\n", model, cost)
		}
		fmt.Println()
	}

	// By node
	if costByNode && len(nodeCosts) > 0 {
		fmt.Println("By Node:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NODE\tMODEL\tINPUT\tOUTPUT\tCOST")
		for _, nc := range nodeCosts {
			fmt.Fprintf(w, "  %s\t%s\t%d\t%d\t$%.4f\n",
				truncateString(nc.NodeID, 20),
				truncateString(nc.Model, 25),
				nc.InputTokens,
				nc.OutputTokens,
				nc.Cost)
		}
		w.Flush()
		fmt.Println()
	}

	// LLM call count
	fmt.Printf("LLM Calls: %d\n", len(spans))

	return nil
}

type NodeCost struct {
	NodeID       string  `json:"node_id"`
	SpanID       string  `json:"span_id"`
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
}

func calculateNodeCosts(spans []*store.Span) []NodeCost {
	var costs []NodeCost

	for _, span := range spans {
		if span.LLM == nil {
			continue
		}

		nodeID := span.Name
		if span.Node != nil && span.Node.NodeID != "" {
			nodeID = span.Node.NodeID
		}

		costs = append(costs, NodeCost{
			NodeID:       nodeID,
			SpanID:       span.ID,
			Model:        span.LLM.Model,
			Provider:     span.LLM.Provider,
			InputTokens:  span.LLM.Tokens.InputTokens,
			OutputTokens: span.LLM.Tokens.OutputTokens,
			Cost:         span.LLM.Tokens.CostEstimate,
		})
	}

	// Sort by cost descending
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Cost > costs[j].Cost
	})

	return costs
}

func parseDuration(s string) (time.Duration, error) {
	// Try standard duration first
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}

	// Try days format
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var n int
		fmt.Sscanf(days, "%d", &n)
		if n > 0 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf("invalid duration format: %s", s)
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
