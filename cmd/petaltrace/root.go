package petaltrace

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	version = "0.1.0-dev"
)

var rootCmd = &cobra.Command{
	Use:   "petaltrace",
	Short: "Agent observability platform for AI workflows",
	Long: `PetalTrace is an agent observability platform that provides deep insight
into AI agent workflows. It captures the full execution lifecycle — from
compiled graph topology through runtime node execution, LLM provider
interactions, tool calls, token consumption, and data flow.

See what your agents are thinking.`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./petaltrace.yaml)")

	rootCmd.SetVersionTemplate(`{{printf "petaltrace %s\n" .Version}}`)
}

func exitWithError(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	}
	os.Exit(1)
}
