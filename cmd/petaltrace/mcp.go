package petaltrace

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/diff"
	"github.com/petal-labs/petaltrace/internal/config"
	"github.com/petal-labs/petaltrace/mcp"
	"github.com/petal-labs/petaltrace/pricing"
	"github.com/petal-labs/petaltrace/replay"
	"github.com/petal-labs/petaltrace/store"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the PetalTrace MCP server",
	Long: `Start the PetalTrace MCP server which exposes trace query, comparison,
and replay capabilities to agents via the MCP protocol.

The MCP server uses stdio transport, reading JSON-RPC requests from stdin
and writing responses to stdout. This allows it to be invoked by MCP clients
like Claude Code.

Available tools:
  petaltrace.trace.list      List recent runs with filters
  petaltrace.trace.get       Get run detail with span tree
  petaltrace.trace.search    Search runs by prompt/completion content
  petaltrace.prompt.get      Get full LLM prompt and completion
  petaltrace.cost.summary    Aggregate cost metrics
  petaltrace.cost.run        Per-run cost breakdown
  petaltrace.diff.compare    Compare two runs
  petaltrace.run.replay      Trigger workflow replay`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Set up logging to stderr (stdout is used for MCP protocol)
	logLevel := slog.LevelInfo
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting petaltrace MCP server",
		"version", version,
		"transport", "stdio",
	)

	// Initialize SQLite store
	dbPath := cfg.Storage.SQLite.Path
	if dbPath == "" {
		dbPath = "./data/petaltrace.db"
	}

	traceStore, err := store.NewSQLiteStore(store.SQLiteOptions{
		Path:    dbPath,
		WALMode: cfg.Storage.SQLite.WALMode,
	})
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer traceStore.Close()

	logger.Info("database initialized", "path", dbPath)

	// Load pricing table
	var pricingTable *pricing.PricingTable
	if cfg.Pricing.Overrides != "" {
		pricingTable, err = pricing.LoadWithOverrides(cfg.Pricing.Overrides)
	} else {
		pricingTable, err = pricing.LoadBuiltin()
	}
	if err != nil {
		return fmt.Errorf("loading pricing: %w", err)
	}

	// Create engines
	diffEngine := diff.NewEngine(traceStore)
	replayEngine := replay.NewEngine(replay.EngineConfig{
		Store:        traceStore,
		PetalFlowURL: "http://localhost:8080",
		Logger:       logger,
	})

	// Create MCP server
	mcpServer := mcp.NewServer(mcp.ServerConfig{
		Store:        traceStore,
		PricingTable: pricingTable,
		DiffEngine:   diffEngine,
		ReplayEngine: replayEngine,
		Logger:       logger,
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
	})

	// Set up cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down MCP server...")
		cancel()
		mcpServer.Close()
	}()

	// Run MCP server (blocks until stdin closes or context is cancelled)
	if err := mcpServer.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("MCP server error: %w", err)
	}

	logger.Info("petaltrace MCP server stopped")
	return nil
}
