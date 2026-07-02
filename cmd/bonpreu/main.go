package main

import (
	"fmt"
	"os"

	"github.com/pmontp19/bonpreu-cli/internal/cli"
)

// version is overridden at release time via -ldflags "-X main.version=...".
// It must be a var (not const) for the linker to stamp it.
var version = "0.1.0-dev"

func main() {
	root := cli.NewRoot(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
