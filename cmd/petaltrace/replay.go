package petaltrace

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/replay"
	"github.com/spf13/cobra"
)

var replayCmd = &cobra.Command{
	Use:   "replay <run_id>",
	Short: "Replay a workflow run",
	Long: `Replay a workflow run using different modes:
  - live: Re-execute against real LLM providers
  - mocked: Use captured responses for deterministic replay
  - hybrid: Mock tools but make live LLM calls`,
	Args: cobra.ExactArgs(1),
	RunE: runReplay,
}

var (
	replayMode        string
	replayModel       string
	replayProvider    string
	replayTemperature float64
	replayMaxTokens   int
	replayDiff        bool
	replaySync        bool
	replayTags        []string
	replayJSON        bool
	replayPetalFlowURL string
)

func init() {
	rootCmd.AddCommand(replayCmd)

	replayCmd.Flags().StringVar(&replayMode, "mode", "live", "Replay mode: live, mocked, or hybrid")
	replayCmd.Flags().StringVar(&replayModel, "model", "", "Override LLM model")
	replayCmd.Flags().StringVar(&replayProvider, "provider", "", "Override LLM provider")
	replayCmd.Flags().Float64Var(&replayTemperature, "temperature", -1, "Override sampling temperature")
	replayCmd.Flags().IntVar(&replayMaxTokens, "max-tokens", 0, "Override max tokens")
	replayCmd.Flags().BoolVar(&replayDiff, "diff", false, "Auto-diff after completion")
	replayCmd.Flags().BoolVar(&replaySync, "sync", true, "Wait for replay to complete")
	replayCmd.Flags().StringSliceVar(&replayTags, "tag", nil, "Add tags (format: key=value)")
	replayCmd.Flags().BoolVar(&replayJSON, "json", false, "Output as JSON")
	replayCmd.Flags().StringVar(&replayPetalFlowURL, "petalflow-url", "http://localhost:8080", "PetalFlow daemon URL")
}

func runReplay(cmd *cobra.Command, args []string) error {
	sourceRunID := args[0]

	traceStore, err := openStore()
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer traceStore.Close()

	// Parse tags
	tags := make(map[string]string)
	for _, tag := range replayTags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) == 2 {
			tags[parts[0]] = parts[1]
		}
	}

	// Build request
	req := &replay.ReplayRequest{
		SourceRunID: sourceRunID,
		Mode:        replay.ReplayMode(replayMode),
		Tags:        tags,
		AutoDiff:    replayDiff,
	}

	// Apply overrides
	if replayModel != "" {
		req.Overrides.Model = replayModel
	}
	if replayProvider != "" {
		req.Overrides.Provider = replayProvider
	}
	if replayTemperature >= 0 {
		req.Overrides.Temperature = &replayTemperature
	}
	if replayMaxTokens > 0 {
		req.Overrides.MaxTokens = &replayMaxTokens
	}

	// Create replay engine
	engine := replay.NewEngine(replay.EngineConfig{
		Store:        traceStore,
		PetalFlowURL: replayPetalFlowURL,
	})

	// Execute replay
	var result *replay.Replay
	if replaySync {
		result, err = engine.ExecuteSync(cmd.Context(), req)
	} else {
		result, err = engine.Execute(cmd.Context(), req)
	}

	if err != nil {
		return fmt.Errorf("replay failed: %w", err)
	}

	// Output result
	if replayJSON {
		return outputReplayJSON(result)
	}

	return outputReplayTable(result, traceStore, cmd)
}

func outputReplayJSON(r *replay.Replay) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputReplayTable(r *replay.Replay, traceStore interface{ Close() error }, cmd *cobra.Command) error {
	fmt.Println("=== Replay Result ===")
	fmt.Println()
	fmt.Printf("Replay ID:    %s\n", r.ID)
	fmt.Printf("Source Run:   %s\n", r.SourceRunID)
	fmt.Printf("Mode:         %s\n", r.Mode)
	fmt.Printf("Status:       %s\n", r.Status)

	if r.NewRunID != "" {
		fmt.Printf("New Run ID:   %s\n", r.NewRunID)
	}

	fmt.Printf("Started At:   %s\n", r.StartedAt.Format(time.RFC3339))

	if r.CompletedAt != nil {
		fmt.Printf("Completed At: %s\n", r.CompletedAt.Format(time.RFC3339))
		fmt.Printf("Duration:     %s\n", r.CompletedAt.Sub(r.StartedAt).Round(time.Millisecond))
	}

	if r.Error != "" {
		fmt.Printf("\nError: %s\n", r.Error)
	}

	if r.DiffID != "" {
		fmt.Printf("\nDiff ID: %s\n", r.DiffID)
		fmt.Println("\nTo view the diff:")
		fmt.Printf("  petaltrace diff %s %s\n", r.SourceRunID, r.NewRunID)
	}

	// If auto-diff was requested and completed, show summary
	if r.DiffID != "" && r.Status == replay.ReplayStatusCompleted {
		// Try to load and display diff summary
		if store, ok := traceStore.(interface {
			GetDiff(ctx interface{}, id string) (*interface{}, error)
		}); ok {
			_ = store // Would load diff here
		}
	}

	return nil
}

// replayStatusCmd shows status of a replay
var replayStatusCmd = &cobra.Command{
	Use:   "status <replay_id>",
	Short: "Check replay status",
	Args:  cobra.ExactArgs(1),
	RunE:  runReplayStatus,
}

func init() {
	replayCmd.AddCommand(replayStatusCmd)
}

func runReplayStatus(cmd *cobra.Command, args []string) error {
	replayID := args[0]

	traceStore, err := openStore()
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer traceStore.Close()

	engine := replay.NewEngine(replay.EngineConfig{
		Store:        traceStore,
		PetalFlowURL: replayPetalFlowURL,
	})

	result, err := engine.RefreshReplayStatus(cmd.Context(), replayID)
	if err != nil {
		return fmt.Errorf("getting replay status: %w", err)
	}

	if replayJSON {
		return outputReplayJSON(result)
	}

	return outputReplayTable(result, traceStore, cmd)
}

// replayDiffCmd computes diff for a completed replay
var replayDiffCmd = &cobra.Command{
	Use:   "diff <replay_id>",
	Short: "Compute diff for a completed replay",
	Args:  cobra.ExactArgs(1),
	RunE:  runReplayDiffCmd,
}

func init() {
	replayCmd.AddCommand(replayDiffCmd)
	replayDiffCmd.Flags().BoolVar(&diffIncludeContent, "include-content", false, "Include full text diffs")
}

func runReplayDiffCmd(cmd *cobra.Command, args []string) error {
	replayID := args[0]

	traceStore, err := openStore()
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer traceStore.Close()

	replayEngine := replay.NewEngine(replay.EngineConfig{
		Store:        traceStore,
		PetalFlowURL: replayPetalFlowURL,
	})

	r, ok := replayEngine.GetReplay(replayID)
	if !ok {
		return fmt.Errorf("replay not found: %s", replayID)
	}

	if r.Status != replay.ReplayStatusCompleted {
		return fmt.Errorf("replay not completed (status: %s)", r.Status)
	}

	if r.NewRunID == "" {
		return fmt.Errorf("replay has no new run ID")
	}

	// Compute diff
	diffEngine := diff.NewEngine(traceStore)
	opts := diff.DiffOptions{
		IncludeContent:    diffIncludeContent,
		IncludeSimilarity: true,
		CacheResult:       true,
	}

	runDiff, err := diffEngine.ComputeDiff(cmd.Context(), r.SourceRunID, r.NewRunID, opts)
	if err != nil {
		return fmt.Errorf("computing diff: %w", err)
	}

	// Output diff summary
	fmt.Println(formatDiffSummary(runDiff))

	return nil
}
