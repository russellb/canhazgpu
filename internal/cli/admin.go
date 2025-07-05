package cli

import (
	"context"
	"fmt"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Initialize GPU pool for this machine",
	Long: `Initialize the GPU pool by setting the number of GPUs available on this machine.
This must be run once before using other commands.

Use --force to reinitialize an existing pool (this will clear all reservations).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuCount, _ := cmd.Flags().GetInt("gpus")
		force, _ := cmd.Flags().GetBool("force")

		if gpuCount <= 0 {
			return fmt.Errorf("GPU count must be greater than 0")
		}

		return runAdmin(cmd.Context(), gpuCount, force)
	},
}

func init() {
	adminCmd.Flags().IntP("gpus", "g", 0, "Number of GPUs available on this machine (required)")
	adminCmd.Flags().Bool("force", false, "Force reinitialization even if already initialized")
	if err := adminCmd.MarkFlagRequired("gpus"); err != nil {
		// This should not happen in practice, but handle it
		panic(fmt.Sprintf("Failed to mark gpus flag as required: %v", err))
	}

	rootCmd.AddCommand(adminCmd)
}

func runAdmin(ctx context.Context, gpuCount int, force bool) error {
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

	// Check if already initialized
	existingCount, err := client.GetGPUCount(ctx)
	if err == nil && !force {
		return fmt.Errorf("GPU pool already initialized with %d GPUs. Use --force to reinitialize", existingCount)
	}

	// Clear existing state if force is used
	if force && err == nil {
		fmt.Printf("Releasing all GPUs: admin force reset (clearing %d existing GPUs)\n", existingCount)
		if err := client.ClearAllGPUStates(ctx); err != nil {
			return fmt.Errorf("failed to clear existing GPU states: %v", err)
		}
	}

	// Set GPU count
	if err := client.SetGPUCount(ctx, gpuCount); err != nil {
		return fmt.Errorf("failed to set GPU count: %v", err)
	}

	if force && existingCount > 0 {
		fmt.Printf("Reinitialized %d GPUs (IDs 0 to %d)\n", gpuCount, gpuCount-1)
	} else {
		fmt.Printf("Initialized %d GPUs (IDs 0 to %d)\n", gpuCount, gpuCount-1)
	}

	return nil
}
