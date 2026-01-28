package cli

import (
	"context"
	"fmt"
	"sort"
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

By default, if GPUs are not available, the command will wait in a queue until
resources become available (FCFS - First Come First Served). Use --nonblock to
fail immediately instead.

You can reserve GPUs in two ways:
- By count: --gpus N (allocates N GPUs using MRU-per-user strategy)
- By specific IDs: --gpu-ids 1,3,5 (reserves exactly those GPU IDs)

When using --gpu-ids, the --gpus flag is optional if:
- It matches the number of GPU IDs specified, or
- It is 1 (the default value)

If specific GPU IDs are requested and any are not available, the reservation
will wait in the queue until those specific IDs become available.

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
  canhazgpu reserve --nonblock --gpus 4 --duration 2h  # Fail if unavailable
  canhazgpu reserve --wait 30m --gpus 4 --duration 2h  # Wait up to 30 minutes
  export CUDA_VISIBLE_DEVICES=$(canhazgpu reserve --gpus 2 --short)  # For scripting

The reserved GPUs must be manually released with 'canhazgpu release' or will
automatically expire after the specified duration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuCount := viper.GetInt("reserve.gpus")
		gpuIDs := viper.GetIntSlice("reserve.gpu-ids")
		durationStr := viper.GetString("reserve.duration")
		force := viper.GetBool("reserve.force")
		note := viper.GetString("reserve.note")
		customUser := viper.GetString("reserve.user")
		nonblock := viper.GetBool("reserve.nonblock")
		waitStr := viper.GetString("reserve.wait")
		short := viper.GetBool("reserve.short")

		return runReserve(cmd.Context(), gpuCount, gpuIDs, durationStr, force, note, customUser, nonblock, waitStr, short)
	},
}

func init() {
	reserveCmd.Flags().IntP("gpus", "g", 1, "Number of GPUs to reserve")
	reserveCmd.Flags().IntSliceP("gpu-ids", "G", nil, "Specific GPU IDs to reserve (comma-separated, e.g., 1,3,5)")
	reserveCmd.Flags().StringP("duration", "d", "30m", "Duration to reserve GPUs (e.g., 30m, 2h, 1d)")
	reserveCmd.Flags().BoolP("force", "f", false, "Force reservation even if GPU is in unreserved use")
	reserveCmd.Flags().StringP("note", "n", "", "Optional note describing the reservation purpose")
	reserveCmd.Flags().StringP("user", "u", "", "Custom user identifier (e.g., your name when using a shared account)")
	reserveCmd.Flags().Bool("nonblock", false, "Fail immediately if GPUs are unavailable instead of waiting in queue")
	reserveCmd.Flags().StringP("wait", "w", "", "Maximum time to wait for GPUs (e.g., 30m, 2h). Default: wait forever.")
	reserveCmd.Flags().BoolP("short", "s", false, "Output only the GPU IDs (for use with command substitution)")

	rootCmd.AddCommand(reserveCmd)
}

func runReserve(ctx context.Context, gpuCount int, gpuIDs []int, durationStr string, force bool, note string, customUser string, nonblock bool, waitStr string, short bool) error {
	// If neither is specified, default to 1 GPU
	if gpuCount == 0 && len(gpuIDs) == 0 {
		gpuCount = 1
	}

	// Parse duration
	duration, err := utils.ParseDuration(durationStr)
	if err != nil {
		return err
	}

	// Parse wait timeout if provided
	var waitTimeout *time.Duration
	if waitStr != "" {
		wt, err := utils.ParseDuration(waitStr)
		if err != nil {
			return fmt.Errorf("invalid wait timeout format: %v", err)
		}
		waitTimeout = &wt
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
	request := &gpu.QueuedAllocationRequest{
		AllocationRequest: &types.AllocationRequest{
			GPUCount:        gpuCount,
			GPUIDs:          gpuIDs,
			User:            displayUser,
			ActualUser:      actualUser,
			ReservationType: types.ReservationTypeManual,
			ExpiryTime:      &expiryTime,
			Force:           force,
			Note:            note,
		},
		Blocking:    !nonblock,
		WaitTimeout: waitTimeout,
	}

	// Allocate GPUs (with queue support)
	result, err := engine.AllocateGPUsWithQueue(ctx, request)
	if err != nil {
		return err
	}
	allocatedGPUs := result.AllocatedGPUs

	// Sort GPU IDs for consistent ordering in output and environment variable
	sort.Ints(allocatedGPUs)

	// Build list for CUDA_VISIBLE_DEVICES
	ids := make([]string, len(allocatedGPUs))
	for i, id := range allocatedGPUs {
		ids[i] = strconv.Itoa(id)
	}

	if short {
		// Short output: just the GPU IDs for command substitution
		fmt.Print(strings.Join(ids, ","))
		return nil
	}

	fmt.Printf("Reserved %d GPU(s): %v for %s\n",
		len(allocatedGPUs), allocatedGPUs, utils.FormatDuration(duration))

	fmt.Printf(
		"\nRun the following command to run only on these GPUs:\nexport CUDA_VISIBLE_DEVICES=%s\n",
		strings.Join(ids, ","),
	)

	return nil
}
