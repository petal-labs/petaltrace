package petaltrace

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/petal-labs/petaltrace/api"
	"github.com/petal-labs/petaltrace/collector"
	"github.com/petal-labs/petaltrace/internal/config"
	"github.com/petal-labs/petaltrace/pricing"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the PetalTrace daemon",
	Long: `Start the PetalTrace daemon which runs the OTLP collector,
HTTP API server, and embedded UI.

The daemon accepts traces via:
  - OTLP/HTTP on port 4318 (configurable)
  - OTLP/gRPC on port 4317 (configurable)

Configuration is loaded from petaltrace.yaml or the path specified
with --config. Environment variables with PETALTRACE_ prefix override
config file values.`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Set up logging
	logLevel := slog.LevelInfo
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("starting petaltrace",
		"version", Version,
		"config", cfgFile,
	)

	if cfg.Storage.Driver == "sqlite" {
		if err := os.MkdirAll(cfg.DataDir(), 0755); err != nil {
			return fmt.Errorf("creating data directory: %w", err)
		}
	}

	traceStore, err := openStoreFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer traceStore.Close()

	logger.Info("database initialized", "driver", cfg.Storage.Driver)

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

	// Determine ports
	httpAddr := fmt.Sprintf("127.0.0.1:%d", cfg.Collector.OTLPHTTPPort)
	grpcAddr := fmt.Sprintf("127.0.0.1:%d", cfg.Collector.OTLPGRPCPort)

	// Create collector
	coll, err := collector.NewCollector(collector.CollectorConfig{
		Store:        traceStore,
		PricingTable: pricingTable,
		Logger:       logger,
		HTTPAddr:     httpAddr,
		GRPCAddr:     grpcAddr,
		BatchSize:    100, // Default batch size
	})
	if err != nil {
		return fmt.Errorf("creating collector: %w", err)
	}

	// Start collector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coll.Start(ctx); err != nil {
		return fmt.Errorf("starting collector: %w", err)
	}

	logger.Info("collector started",
		"otlp_http", coll.HTTPAddr(),
		"otlp_grpc", coll.GRPCAddr(),
	)

	// Start API server
	apiAddr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	apiServer := api.NewServer(api.ServerConfig{
		Store:        traceStore,
		PricingTable: pricingTable,
		Logger:       logger,
		Addr:         apiAddr,
	})

	if err := apiServer.Start(ctx); err != nil {
		return fmt.Errorf("starting API server: %w", err)
	}

	logger.Info("petaltrace ready",
		"api", apiServer.Addr(),
		"otlp_http", coll.HTTPAddr(),
		"otlp_grpc", coll.GRPCAddr(),
	)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("shutting down...")

	// Graceful shutdown
	if err := apiServer.Stop(); err != nil {
		logger.Error("API server stop error", "error", err)
	}

	if err := coll.Stop(); err != nil {
		logger.Error("collector stop error", "error", err)
	}

	logger.Info("petaltrace stopped")
	return nil
}
