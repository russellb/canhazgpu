package cli

import (
	"context"
	"fmt"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Release manually reserved GPUs held by the current user",
	Long: `Release manually reserved GPUs held by the current user.

By default, releases all manually reserved GPUs. You can optionally specify
which GPU(s) to release using the --gpu-ids flag.

This command can release:
- Manual reservations made with the 'reserve' command
- Run-type reservations made with the 'run' command (useful for cleaning up
  after known failures faster than waiting for heartbeat timeout)

Examples:
  canhazgpu release                # Release all manually reserved GPUs
  canhazgpu release --gpu-ids 1,3  # Release specific GPUs`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuIDs := viper.GetIntSlice("release.gpu-ids")
		return runRelease(cmd.Context(), gpuIDs)
	},
}

func init() {
	releaseCmd.Flags().IntSliceP("gpu-ids", "G", nil, "Specific GPU IDs to release (comma-separated, e.g., 1,3,5)")

	rootCmd.AddCommand(releaseCmd)
}

func runRelease(ctx context.Context, gpuIDs []int) error {
	config := getConfig()
	client := redis_client.NewClient(config)
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Printf("Warning: failed to close Redis client: %v\n", err)
		}
	}()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine
	engine := gpu.NewAllocationEngine(client, config)

	// Release GPUs for current user
	user := getCurrentUser()
	var releasedGPUs []int
	var err error

	if len(gpuIDs) > 0 {
		// Release specific GPUs
		releasedGPUs, err = engine.ReleaseSpecificGPUs(ctx, user, gpuIDs)
	} else {
		// Release all manually reserved GPUs
		releasedGPUs, err = engine.ReleaseGPUs(ctx, user)
	}

	if err != nil {
		return fmt.Errorf("failed to release GPUs: %v", err)
	}

	if len(releasedGPUs) == 0 {
		if len(gpuIDs) > 0 {
			fmt.Printf("No reservations found for current user on GPU(s): %v\n", gpuIDs)
		} else {
			fmt.Println("No manually reserved GPUs found for current user")
		}
	} else {
		fmt.Printf("Released %d GPU(s): %v\n", len(releasedGPUs), releasedGPUs)
	}

	return nil
}
