package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/russellb/canhazgpu/internal/utils"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set",
	Long: `Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES automatically set.

The command will:
1. Reserve the requested number of GPUs (or specific GPU IDs)
2. Set CUDA_VISIBLE_DEVICES to the allocated GPU IDs  
3. Run your command
4. Automatically release GPUs when the command finishes
5. Maintain a heartbeat while running to keep the reservation active
6. Forward signals (Ctrl-C/SIGINT) to the child process for graceful shutdown

You can reserve GPUs in two ways:
- By count: --gpus N (allocates N GPUs using LRU strategy)
- By specific IDs: --gpu-ids 1,3,5 (reserves exactly those GPU IDs)

When using --gpu-ids, the --gpus flag is optional if:
- It matches the number of GPU IDs specified, or
- It is 1 (the default value)

If specific GPU IDs are requested and any are not available, the entire
reservation will fail.

Optionally, you can set a timeout to automatically terminate the command and release
GPUs after a specified duration. When the timeout is reached, SIGINT will be sent to
the entire process group (including all child processes) for graceful shutdown, 
followed by a 30-second grace period. If any processes haven't exited after the 
grace period, the entire process group will be force-killed with SIGKILL.
This is useful for preventing runaway processes from holding GPUs indefinitely.

Example usage:
  canhazgpu run --gpus 1 -- python train.py
  canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py
  canhazgpu run --gpu-ids 1,3 -- python train.py
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
		gpuCount, _ := cmd.Flags().GetInt("gpus")
		gpuIDs, _ := cmd.Flags().GetIntSlice("gpu-ids")
		timeoutStr, _ := cmd.Flags().GetString("timeout")

		if len(args) == 0 {
			return fmt.Errorf("no command specified. Use: canhazgpu run --gpus N -- <command>")
		}

		return runRun(cmd.Context(), gpuCount, gpuIDs, timeoutStr, args)
	},
	DisableFlagsInUseLine: true,
}

func init() {
	runCmd.Flags().IntP("gpus", "g", 1, "Number of GPUs to reserve")
	runCmd.Flags().IntSliceP("gpu-ids", "", nil, "Specific GPU IDs to reserve (comma-separated, e.g., 1,3,5)")
	runCmd.Flags().StringP("timeout", "t", "", "Timeout duration for graceful command termination (e.g., 30m, 2h, 1d). Disabled by default.")

	// Allow passing through arbitrary arguments after --
	runCmd.Flags().SetInterspersed(false)

	rootCmd.AddCommand(runCmd)
}

// killProcessGroup kills the entire process group (parent and all children)
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return fmt.Errorf("process not started")
	}

	// Kill the entire process group by using negative PID
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// If we can't get the process group, just kill the process itself
		return cmd.Process.Kill()
	}

	// Send SIGKILL to the entire process group
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		// If group kill fails, try to kill just the process
		return cmd.Process.Kill()
	}

	return nil
}

func runRun(ctx context.Context, gpuCount int, gpuIDs []int, timeoutStr string, command []string) error {
	// If neither is specified, default to 1 GPU
	if gpuCount == 0 && len(gpuIDs) == 0 {
		gpuCount = 1
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
		GPUIDs:          gpuIDs,
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

	// Prepare command (don't use CommandContext to avoid abrupt killing)
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", strings.Join(cudaDevices, ",")))

	// Create a new process group so we can kill all child processes if needed.
	// This ensures that when we send signals (SIGINT or SIGKILL), they reach
	// all child processes spawned by the command, not just the parent.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	// Set up signal handling to forward SIGINT to child process
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		for sig := range sigChan {
			// Forward the signal to the child process
			if cmd.Process != nil {
				// Send signal directly to the process we created
				if err := cmd.Process.Signal(sig.(syscall.Signal)); err != nil {
					// Log error but don't fail - process might have already exited
					fmt.Fprintf(os.Stderr, "Failed to forward signal to child process: %v\n", err)
				}
			}
		}
	}()
	defer func() {
		signal.Stop(sigChan)
		close(sigChan)
	}()

	// Handle timeout with graceful shutdown if specified
	var timeoutKilled int32 // Use atomic operations for thread safety
	if hasTimeout {
		go func() {
			timer := time.NewTimer(timeout)
			defer timer.Stop()

			select {
			case <-timer.C:
				fmt.Printf("Command timeout reached after %s. Attempting graceful shutdown...\n", utils.FormatDuration(timeout))

				// Send SIGINT to the process group for graceful shutdown
				if cmd.Process != nil {
					pgid, err := syscall.Getpgid(cmd.Process.Pid)
					if err == nil {
						// Send SIGINT to the entire process group
						if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil {
							fmt.Printf("Failed to send SIGINT to process group: %v\n", err)
							// If SIGINT fails, kill immediately
							killProcessGroup(cmd)
							atomic.StoreInt32(&timeoutKilled, 1)
							return
						}
					} else {
						// Fallback to sending signal to just the process
						if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
							fmt.Printf("Failed to send SIGINT: %v\n", err)
							// If SIGINT fails, kill immediately
							killProcessGroup(cmd)
							atomic.StoreInt32(&timeoutKilled, 1)
							return
						}
					}
				}

				// Wait 30 seconds for graceful shutdown
				gracePeriod := 30 * time.Second
				fmt.Printf("Waiting %s for graceful shutdown...\n", gracePeriod)

				graceTimer := time.NewTimer(gracePeriod)
				defer graceTimer.Stop()

				select {
				case <-graceTimer.C:
					// Grace period expired, force kill
					fmt.Printf("Grace period expired. Force killing process group...\n")
					if err := killProcessGroup(cmd); err != nil {
						fmt.Printf("Error killing process group: %v\n", err)
					}
					atomic.StoreInt32(&timeoutKilled, 1)
				case <-ctx.Done():
					// Parent context cancelled, don't force kill
					return
				}
			case <-ctx.Done():
				// Parent context cancelled, don't timeout
				return
			}
		}()
	}

	// Wait for command to complete
	err = cmd.Wait()

	// Handle exit code properly - ensure cleanup happens before exiting
	if err != nil {
		if atomic.LoadInt32(&timeoutKilled) == 1 {
			fmt.Printf("Command was terminated due to timeout\n")
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
