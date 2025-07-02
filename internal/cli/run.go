package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/russellb/canhazgpu/internal/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set",
	Long: `Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES automatically set.

The command will:
1. Reserve the requested number of GPUs
2. Set CUDA_VISIBLE_DEVICES to the allocated GPU IDs  
3. Run your command
4. Automatically release GPUs when the command finishes
5. Maintain a heartbeat while running to keep the reservation active

Optionally, you can set a timeout to automatically kill the command and release
GPUs after a specified duration. This is useful for preventing runaway processes
from holding GPUs indefinitely.

Example usage:
  canhazgpu run --gpus 1 -- python train.py
  canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py
  canhazgpu run --gpus 1 --timeout 2h -- python long_training.py

Timeout formats supported:
- 30s (30 seconds)
- 30m (30 minutes)
- 2h (2 hours)  
- 1d (1 day)
- 0.5h (30 minutes with decimal)

The '--' separator is important - it tells canhazgpu where its options end
and your command begins.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuCount := viper.GetInt("run.gpus")
		timeoutStr := viper.GetString("run.timeout")

		if len(args) == 0 {
			return fmt.Errorf("no command specified. Use: canhazgpu run --gpus N -- <command>")
		}

		return runRun(cmd.Context(), gpuCount, timeoutStr, args)
	},
	DisableFlagsInUseLine: true,
}

func init() {
	runCmd.Flags().IntP("gpus", "g", 1, "Number of GPUs to reserve")
	runCmd.Flags().StringP("timeout", "t", "", "Timeout duration to kill command (e.g., 30m, 2h, 1d). Disabled by default.")

	// Allow passing through arbitrary arguments after --
	runCmd.Flags().SetInterspersed(false)

	rootCmd.AddCommand(runCmd)
}

func runRun(ctx context.Context, gpuCount int, timeoutStr string, command []string) error {
	if gpuCount <= 0 {
		return fmt.Errorf("GPU count must be greater than 0")
	}

	config := getConfig()

	// Parse timeout if provided
	var timeout time.Duration
	var hasTimeout bool

	if timeoutStr != "" {
		var err error
		timeout, err = utils.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid timeout format: %v", err)
		}
		hasTimeout = true
	}
	client := redis_client.NewClient(config)
	defer client.Close()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine
	engine := gpu.NewAllocationEngine(client, config)

	// Create allocation request
	user := getCurrentUser()
	request := &types.AllocationRequest{
		GPUCount:        gpuCount,
		User:            user,
		ReservationType: types.ReservationTypeRun,
		ExpiryTime:      nil, // No expiry for run-type reservations
	}

	// Allocate GPUs
	allocatedGPUs, err := engine.AllocateGPUs(ctx, request)
	if err != nil {
		return err
	}

	if hasTimeout {
		fmt.Printf("Reserved %d GPU(s): %v for command execution (timeout: %s)\n",
			len(allocatedGPUs), allocatedGPUs, utils.FormatDuration(timeout))
	} else {
		fmt.Printf("Reserved %d GPU(s): %v for command execution\n",
			len(allocatedGPUs), allocatedGPUs)
	}

	// Start heartbeat manager
	heartbeat := gpu.NewHeartbeatManager(client, allocatedGPUs, user)
	heartbeat.Start()
	defer heartbeat.Stop()

	// Set CUDA_VISIBLE_DEVICES
	cudaDevices := make([]string, len(allocatedGPUs))
	for i, gpuID := range allocatedGPUs {
		cudaDevices[i] = strconv.Itoa(gpuID)
	}

	// Create context with timeout if specified
	var cmdCtx context.Context
	var cancelFunc context.CancelFunc
	if hasTimeout {
		cmdCtx, cancelFunc = context.WithTimeout(ctx, timeout)
		defer cancelFunc()
	} else {
		cmdCtx = ctx
	}

	// Prepare command
	cmd := exec.CommandContext(cmdCtx, command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", strings.Join(cudaDevices, ",")))

	// Run command
	err = cmd.Run()

	// Handle exit code properly - ensure cleanup happens before exiting
	if err != nil {
		// Check if error was due to timeout
		if hasTimeout && cmdCtx.Err() == context.DeadlineExceeded {
			fmt.Printf("Command timed out after %s and was killed\n", utils.FormatDuration(timeout))
			// Stop heartbeat and clean up GPUs before exiting
			heartbeat.Stop()
			os.Exit(124) // Standard timeout exit code
		}

		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				// Stop heartbeat and clean up GPUs before exiting
				heartbeat.Stop()

				// Exit with the same code as the failed command
				os.Exit(status.ExitStatus())
			}
		}
		// For other types of errors, the defer will handle cleanup
		return fmt.Errorf("command failed: %v", err)
	}

	return nil
}
