package cli

import (
	"context"
	"fmt"

	"github.com/russellb/canhazgpu/internal/gpu"
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
		provider, _ := cmd.Flags().GetString("provider")

		if gpuCount <= 0 {
			return fmt.Errorf("GPU count must be greater than 0")
		}

		return runAdmin(cmd.Context(), gpuCount, force, provider)
	},
}

func init() {
	adminCmd.Flags().IntP("gpus", "g", 0, "Number of GPUs available on this machine (required)")
	adminCmd.Flags().Bool("force", false, "Force reinitialization even if already initialized")
	adminCmd.Flags().StringP("provider", "p", "", "GPU provider to use (nvidia or amd). If not specified, auto-detect available provider")
	if err := adminCmd.MarkFlagRequired("gpus"); err != nil {
		// This should not happen in practice, but handle it
		panic(fmt.Sprintf("Failed to mark gpus flag as required: %v", err))
	}

	rootCmd.AddCommand(adminCmd)
}

func runAdmin(ctx context.Context, gpuCount int, force bool, explicitProvider string) error {
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

	// Determine which provider to use
	var providerName string
	if explicitProvider != "" {
		// Use explicitly specified provider
		fmt.Printf("Using explicitly specified GPU provider: %s\n", explicitProvider)

		// Validate provider name
		if explicitProvider != "nvidia" && explicitProvider != "amd" {
			return fmt.Errorf("invalid provider '%s'. Valid providers are: nvidia, amd", explicitProvider)
		}

		// Validate that the specified provider is available
		pm := gpu.NewProviderManager()
		availableProviders := pm.GetAvailableProviders()

		available := false
		for _, provider := range availableProviders {
			if provider.Name() == explicitProvider {
				available = true
				break
			}
		}

		if !available {
			return fmt.Errorf("provider '%s' is not available on this system", explicitProvider)
		}

		providerName = explicitProvider
	} else {
		// Auto-detect available provider
		fmt.Print("Detecting available GPU provider... ")
		pm := gpu.NewProviderManager()
		availableProviders := pm.GetAvailableProviders()

		if len(availableProviders) == 0 {
			return fmt.Errorf("no GPU providers available (nvidia-smi, amd-smi not found)")
		}

		if len(availableProviders) > 1 {
			var names []string
			for _, provider := range availableProviders {
				names = append(names, provider.Name())
			}
			return fmt.Errorf("multiple GPU providers detected: %v. Please specify one with --provider", names)
		}

		providerName = availableProviders[0].Name()
		fmt.Printf("found %s\n", providerName)
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

	// Store available provider
	if err := client.SetAvailableProvider(ctx, providerName); err != nil {
		return fmt.Errorf("failed to store provider information: %v", err)
	}

	if force && existingCount > 0 {
		fmt.Printf("Reinitialized %d GPUs (IDs 0 to %d)\n", gpuCount, gpuCount-1)
	} else {
		fmt.Printf("Initialized %d GPUs (IDs 0 to %d)\n", gpuCount, gpuCount-1)
	}

	return nil
}
