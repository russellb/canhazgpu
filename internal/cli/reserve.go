package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/russellb/canhazgpu/internal/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var reserveCmd = &cobra.Command{
	Use:   "reserve",
	Short: "Reserve GPUs manually for a specified duration",
	Long: `Reserve GPUs manually for a specified duration without running a command.
This is useful for interactive development sessions or planning work.

You can reserve GPUs in two ways:
- By count: --gpus N (allocates N GPUs using MRU-per-user strategy)
- By specific IDs: --gpu-ids 1,3,5 (reserves exactly those GPU IDs)

When using --gpu-ids, the --gpus flag is optional if:
- It matches the number of GPU IDs specified, or
- It is 1 (the default value)

If specific GPU IDs are requested and any are not available, the entire
reservation will fail.

Use --force to reserve GPUs that are currently in unreserved use. This is
useful when you've started a job without using canhazgpu and want to create
a reservation retroactively.

Duration formats supported:
- 30m (30 minutes)
- 2h (2 hours)
- 1d (1 day)
- 0.5h (30 minutes with decimal)

IMPORTANT: Unlike 'canhazgpu run', this command does NOT automatically set
CUDA_VISIBLE_DEVICES. After reserving, you must manually set the environment
variable based on the GPU IDs shown in the output:
  export CUDA_VISIBLE_DEVICES=1,3

Example usage:
  canhazgpu reserve --gpus 2 --duration 4h
  canhazgpu reserve --gpu-ids 1,3 --duration 2h
  canhazgpu reserve --gpu-ids 0,1,2 --duration 8h --force

The reserved GPUs must be manually released with 'canhazgpu release' or will
automatically expire after the specified duration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuCount := viper.GetInt("reserve.gpus")
		gpuIDs := viper.GetIntSlice("reserve.gpu-ids")
		durationStr := viper.GetString("reserve.duration")
		force := viper.GetBool("reserve.force")
		note := viper.GetString("reserve.note")
		customUser := viper.GetString("reserve.user")

		return runReserve(cmd.Context(), gpuCount, gpuIDs, durationStr, force, note, customUser)
	},
}

func init() {
	reserveCmd.Flags().IntP("gpus", "g", 1, "Number of GPUs to reserve")
	reserveCmd.Flags().IntSliceP("gpu-ids", "G", nil, "Specific GPU IDs to reserve (comma-separated, e.g., 1,3,5)")
	reserveCmd.Flags().StringP("duration", "d", "30m", "Duration to reserve GPUs (e.g., 30m, 2h, 1d)")
	reserveCmd.Flags().BoolP("force", "f", false, "Force reservation even if GPU is in unreserved use")
	reserveCmd.Flags().StringP("note", "n", "", "Optional note describing the reservation purpose")
	reserveCmd.Flags().StringP("user", "u", "", "Custom user identifier (e.g., your name when using a shared account)")

	rootCmd.AddCommand(reserveCmd)
}

func runReserve(ctx context.Context, gpuCount int, gpuIDs []int, durationStr string, force bool, note string, customUser string) error {
	// If neither is specified, default to 1 GPU
	if gpuCount == 0 && len(gpuIDs) == 0 {
		gpuCount = 1
	}

	// Parse duration
	duration, err := utils.ParseDuration(durationStr)
	if err != nil {
		return err
	}

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

	// Get actual OS user and determine display user
	actualUser := getCurrentUser()
	displayUser := actualUser
	if customUser != "" {
		displayUser = customUser
	}

	// Create allocation request
	expiryTime := time.Now().Add(duration)
	request := &types.AllocationRequest{
		GPUCount:        gpuCount,
		GPUIDs:          gpuIDs,
		User:            displayUser,
		ActualUser:      actualUser,
		ReservationType: types.ReservationTypeManual,
		ExpiryTime:      &expiryTime,
		Force:           force,
		Note:            note,
	}

	// Allocate GPUs
	allocatedGPUs, err := engine.AllocateGPUs(ctx, request)
	if err != nil {
		return err
	}

	fmt.Printf("Reserved %d GPU(s): %v for %s\n",
		len(allocatedGPUs), allocatedGPUs, utils.FormatDuration(duration))

	// Build list for CUDA_VISIBLE_DEVICES
	ids := make([]string, len(allocatedGPUs))
	for i, id := range allocatedGPUs {
		ids[i] = strconv.Itoa(id)
	}

	fmt.Printf(
		"\nRun the following command to run only on these GPUs:\nexport CUDA_VISIBLE_DEVICES=%s\n",
		strings.Join(ids, ","),
	)

	return nil
}
