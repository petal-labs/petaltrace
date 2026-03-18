package petaltrace

import (
	"github.com/spf13/cobra"
)

// Build-time variables (injected via ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "petaltrace",
	Short: "Agent observability platform for AI workflows",
	Long: `PetalTrace is an agent observability platform that provides deep insight
into AI agent workflows. It captures the full execution lifecycle — from
compiled graph topology through runtime node execution, LLM provider
interactions, tool calls, token consumption, and data flow.

See what your agents are thinking.`,
	Version: Version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./petaltrace.yaml)")

	rootCmd.SetVersionTemplate(`{{printf "petaltrace %s\n" .Version}}`)
}
