package main

import (
	"os"

	"github.com/petal-labs/petaltrace/cmd/petaltrace"
)

func main() {
	if err := petaltrace.Execute(); err != nil {
		os.Exit(1)
	}
}
