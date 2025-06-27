package main

import (
	"context"
	"fmt"
	"os"

	"github.com/russellb/canhazgpu/internal/cli"
)

// Version information set by build-time ldflags
var version = "dev"

func main() {
	cli.SetVersion(version)
	ctx := context.Background()
	if err := cli.Execute(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
