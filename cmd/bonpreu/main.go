package main

import (
	"fmt"
	"os"

	"github.com/pmontp19/bonpreu-cli/internal/cli"
)

const version = "0.1.0-dev"

func main() {
	root := cli.NewRoot(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
