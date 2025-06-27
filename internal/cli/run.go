package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/spf13/cobra"
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

Example usage:
  canhazgpu run --gpus 1 -- python train.py
  canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py

The '--' separator is important - it tells canhazgpu where its options end
and your command begins.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuCount, _ := cmd.Flags().GetInt("gpus")

		if len(args) == 0 {
			return fmt.Errorf("no command specified. Use: canhazgpu run --gpus N -- <command>")
		}

		return runRun(cmd.Context(), gpuCount, args)
	},
	DisableFlagsInUseLine: true,
}

func init() {
	runCmd.Flags().IntP("gpus", "g", 1, "Number of GPUs to reserve")

	// Allow passing through arbitrary arguments after --
	runCmd.Flags().SetInterspersed(false)

	rootCmd.AddCommand(runCmd)
}

func runRun(ctx context.Context, gpuCount int, command []string) error {
	if gpuCount <= 0 {
		return fmt.Errorf("GPU count must be greater than 0")
	}

	config := getConfig()
	client := redis_client.NewClient(config)
	defer client.Close()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine
	engine := gpu.NewAllocationEngine(client)

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

	fmt.Printf("Reserved %d GPU(s): %v for command execution\n",
		len(allocatedGPUs), allocatedGPUs)

	// Start heartbeat manager
	heartbeat := gpu.NewHeartbeatManager(client, allocatedGPUs, user)
	heartbeat.Start()
	defer heartbeat.Stop()

	// Set CUDA_VISIBLE_DEVICES
	cudaDevices := make([]string, len(allocatedGPUs))
	for i, gpuID := range allocatedGPUs {
		cudaDevices[i] = strconv.Itoa(gpuID)
	}

	// Prepare command
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", strings.Join(cudaDevices, ",")))

	// Run command
	err = cmd.Run()

	// Handle exit code
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
		return fmt.Errorf("command failed: %v", err)
	}

	return nil
}
