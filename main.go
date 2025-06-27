package main

import (
	"context"
	"fmt"
	"os"

	"github.com/russellb/canhazgpu/internal/cli"
)

func main() {
	ctx := context.Background()
	if err := cli.Execute(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
