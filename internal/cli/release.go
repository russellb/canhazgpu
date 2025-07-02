package cli

import (
	"context"
	"fmt"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release all manually reserved GPUs held by the current user",
	Long: `Release all manually reserved GPUs held by the current user.

This only releases manual reservations made with the 'reserve' command.
It does not affect active 'run' sessions, which are automatically released
when the command completes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRelease(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}

func runRelease(ctx context.Context) error {
	config := getConfig()
	client := redis_client.NewClient(config)
	defer client.Close()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine
	engine := gpu.NewAllocationEngine(client, config)

	// Release GPUs for current user
	user := getCurrentUser()
	releasedGPUs, err := engine.ReleaseGPUs(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to release GPUs: %v", err)
	}

	if len(releasedGPUs) == 0 {
		fmt.Println("No manually reserved GPUs found for current user")
	} else {
		fmt.Printf("Released %d GPU(s): %v\n", len(releasedGPUs), releasedGPUs)
	}

	return nil
}
