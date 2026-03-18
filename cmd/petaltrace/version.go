package petaltrace

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var jsonOutput bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print detailed version information including build metadata.`,
	Run: func(cmd *cobra.Command, args []string) {
		if jsonOutput {
			printVersionJSON()
		} else {
			printVersionText()
		}
	},
}

func init() {
	versionCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output version information as JSON")
	rootCmd.AddCommand(versionCmd)
}

func printVersionText() {
	fmt.Printf("petaltrace %s\n", Version)
	fmt.Printf("  Commit:     %s\n", Commit)
	fmt.Printf("  Built:      %s\n", BuildDate)
	fmt.Printf("  Go version: %s\n", runtime.Version())
	fmt.Printf("  Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func printVersionJSON() {
	info := map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_date": BuildDate,
		"go_version": runtime.Version(),
		"goos":       runtime.GOOS,
		"goarch":     runtime.GOARCH,
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(data))
}
