package petaltrace

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/store"
)

var diffCmd = &cobra.Command{
	Use:   "diff <base_run_id> <compare_run_id>",
	Short: "Compare two runs",
	Long:  `Compare two workflow runs and display differences in structure, content, tokens, and costs.`,
	Args:  cobra.ExactArgs(2),
	RunE:  runDiff,
}

var (
	diffIncludeContent bool
	diffIncludeInputs  bool
	diffFormat         string
	diffOutput         string
	diffNoCache        bool
)

func init() {
	rootCmd.AddCommand(diffCmd)

	diffCmd.Flags().BoolVar(&diffIncludeContent, "include-content", false, "Include full text diffs for prompts and outputs")
	diffCmd.Flags().BoolVar(&diffIncludeInputs, "include-inputs", false, "Include input/output data diffs")
	diffCmd.Flags().StringVar(&diffFormat, "format", "table", "Output format: table, json, or summary")
	diffCmd.Flags().StringVarP(&diffOutput, "output", "o", "", "Write output to file")
	diffCmd.Flags().BoolVar(&diffNoCache, "no-cache", false, "Don't use or store cached diff results")
}

func runDiff(cmd *cobra.Command, args []string) error {
	baseRunID := args[0]
	compareRunID := args[1]

	traceStore, err := openStore()
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer traceStore.Close()

	engine := diff.NewEngine(traceStore)

	opts := diff.DiffOptions{
		IncludeContent:    diffIncludeContent,
		IncludeSimilarity: true,
		IncludeInputs:     diffIncludeInputs,
		CacheResult:       !diffNoCache,
	}

	runDiff, err := engine.ComputeDiff(cmd.Context(), baseRunID, compareRunID, opts)
	if err != nil {
		return fmt.Errorf("computing diff: %w", err)
	}

	// Format output
	var output string
	switch diffFormat {
	case "json":
		output, err = formatDiffJSON(runDiff)
	case "summary":
		output = formatDiffSummary(runDiff)
	default:
		output = formatDiffTable(runDiff, diffIncludeContent)
	}

	if err != nil {
		return fmt.Errorf("formatting output: %w", err)
	}

	// Write output
	if diffOutput != "" {
		if err := os.WriteFile(diffOutput, []byte(output), 0644); err != nil {
			return fmt.Errorf("writing output file: %w", err)
		}
		fmt.Printf("Diff written to %s\n", diffOutput)
	} else {
		fmt.Print(output)
	}

	return nil
}

func formatDiffJSON(d *store.RunDiff) (string, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func formatDiffSummary(d *store.RunDiff) string {
	var sb strings.Builder

	sb.WriteString("=== Diff Summary ===\n\n")
	sb.WriteString(fmt.Sprintf("Base Run:    %s\n", d.BaseRunID))
	sb.WriteString(fmt.Sprintf("Compare Run: %s\n\n", d.CompareRunID))

	// Status
	statusMatch := "Yes"
	if !d.Summary.StatusMatch {
		statusMatch = "No"
	}
	sb.WriteString(fmt.Sprintf("Status Match:     %s\n", statusMatch))

	// Path divergence
	pathDiv := "No"
	if d.Summary.PathDivergence {
		pathDiv = "Yes"
	}
	sb.WriteString(fmt.Sprintf("Path Divergence:  %s\n", pathDiv))

	// Deltas
	sb.WriteString(fmt.Sprintf("Duration Delta:   %+dms\n", d.Summary.DurationDelta))
	sb.WriteString(fmt.Sprintf("Token Delta:      %+d\n", d.Summary.TokenDelta))
	sb.WriteString(fmt.Sprintf("Cost Delta:       %+.6f\n", d.Summary.CostDelta))
	sb.WriteString(fmt.Sprintf("Nodes Changed:    %d\n", d.Summary.NodeDiffCount))

	// Graph changes
	if d.GraphDiff != nil {
		if len(d.GraphDiff.NodesAdded) > 0 {
			sb.WriteString(fmt.Sprintf("\nNodes Added:   %s\n", strings.Join(d.GraphDiff.NodesAdded, ", ")))
		}
		if len(d.GraphDiff.NodesRemoved) > 0 {
			sb.WriteString(fmt.Sprintf("Nodes Removed: %s\n", strings.Join(d.GraphDiff.NodesRemoved, ", ")))
		}
		if len(d.GraphDiff.EdgesChanged) > 0 {
			sb.WriteString(fmt.Sprintf("Edges Changed: %s\n", strings.Join(d.GraphDiff.EdgesChanged, ", ")))
		}
	}

	// Cost breakdown
	sb.WriteString("\n=== Cost Breakdown ===\n\n")
	sb.WriteString(fmt.Sprintf("Base Cost:    $%.6f\n", d.CostDiff.BaseCost))
	sb.WriteString(fmt.Sprintf("Compare Cost: $%.6f\n", d.CostDiff.CompareCost))
	sb.WriteString(fmt.Sprintf("Delta:        $%+.6f\n", d.CostDiff.Delta))

	if len(d.CostDiff.ByProvider) > 0 {
		sb.WriteString("\nBy Provider:\n")
		for provider, delta := range d.CostDiff.ByProvider {
			sb.WriteString(fmt.Sprintf("  %s: $%+.6f\n", provider, delta))
		}
	}

	if len(d.CostDiff.ByModel) > 0 {
		sb.WriteString("\nBy Model:\n")
		for model, delta := range d.CostDiff.ByModel {
			sb.WriteString(fmt.Sprintf("  %s: $%+.6f\n", model, delta))
		}
	}

	return sb.String()
}

func formatDiffTable(d *store.RunDiff, includeContent bool) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("Comparing runs: %s vs %s\n", d.BaseRunID, d.CompareRunID))
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Summary section
	sb.WriteString("SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Status Match:\t%v\n", d.Summary.StatusMatch)
	fmt.Fprintf(w, "Path Divergence:\t%v\n", d.Summary.PathDivergence)
	fmt.Fprintf(w, "Duration Delta:\t%+dms\n", d.Summary.DurationDelta)
	fmt.Fprintf(w, "Token Delta:\t%+d\n", d.Summary.TokenDelta)
	fmt.Fprintf(w, "Cost Delta:\t$%+.6f\n", d.Summary.CostDelta)
	fmt.Fprintf(w, "Nodes Changed:\t%d\n", d.Summary.NodeDiffCount)
	w.Flush()

	// Node diffs
	if len(d.NodeDiffs) > 0 {
		sb.WriteString("\nNODE DIFFERENCES\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n")

		w = tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "NODE\tSTATUS\tTYPE\tDURATION (ms)\tTOKENS\n")

		for _, node := range d.NodeDiffs {
			duration := fmt.Sprintf("%d -> %d", node.DurationBase, node.DurationCompare)
			tokens := "-"
			if node.TokenDiff != nil {
				tokens = fmt.Sprintf("%d -> %d (%+d)",
					node.TokenDiff.BaseTokens.TotalTokens,
					node.TokenDiff.CompareTokens.TotalTokens,
					node.TokenDiff.Delta)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				node.NodeID, node.Status, node.NodeType, duration, tokens)
		}
		w.Flush()

		// Content diffs if requested
		if includeContent {
			for _, node := range d.NodeDiffs {
				if node.OutputDiff != nil && node.OutputDiff.Similarity < 1.0 {
					sb.WriteString(fmt.Sprintf("\n--- Output Diff: %s (similarity: %.2f%%) ---\n",
						node.NodeID, node.OutputDiff.Similarity*100))

					if len(node.OutputDiff.Hunks) > 0 {
						for _, hunk := range node.OutputDiff.Hunks {
							sb.WriteString(fmt.Sprintf("@@ lines %d-%d @@\n", hunk.StartLine, hunk.EndLine))
							sb.WriteString(hunk.Content)
						}
					} else {
						// Show simple comparison
						sb.WriteString("Base:\n")
						sb.WriteString(truncateText(node.OutputDiff.BaseText, 500))
						sb.WriteString("\n\nCompare:\n")
						sb.WriteString(truncateText(node.OutputDiff.CompareText, 500))
						sb.WriteString("\n")
					}
				}
			}
		}
	}

	// Graph diff
	if d.GraphDiff != nil && (len(d.GraphDiff.NodesAdded) > 0 || len(d.GraphDiff.NodesRemoved) > 0) {
		sb.WriteString("\nGRAPH CHANGES\n")
		sb.WriteString(strings.Repeat("-", 40) + "\n")

		if len(d.GraphDiff.NodesAdded) > 0 {
			sb.WriteString("Added nodes:   " + strings.Join(d.GraphDiff.NodesAdded, ", ") + "\n")
		}
		if len(d.GraphDiff.NodesRemoved) > 0 {
			sb.WriteString("Removed nodes: " + strings.Join(d.GraphDiff.NodesRemoved, ", ") + "\n")
		}
	}

	return sb.String()
}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
