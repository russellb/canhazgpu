package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/utils"
	"github.com/spf13/cobra"
)

var supervisorCmd = &cobra.Command{
	Use:    "supervisor",
	Short:  "Internal supervisor mode for monitoring GPU reservations",
	Hidden: true, // Hidden from help - internal use only
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuStr, _ := cmd.Flags().GetString("gpus")
		user, _ := cmd.Flags().GetString("user")
		pidStr, _ := cmd.Flags().GetString("pid")
		timeoutStr, _ := cmd.Flags().GetString("timeout")

		// Parse GPU IDs
		gpuIDs, err := parseGPUList(gpuStr)
		if err != nil {
			return fmt.Errorf("invalid GPU list: %v", err)
		}

		// Parse PID
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			return fmt.Errorf("invalid PID: %v", err)
		}

		// Parse timeout if provided
		var timeout time.Duration
		var hasTimeout bool
		if timeoutStr != "" {
			timeout, err = utils.ParseDuration(timeoutStr)
			if err != nil {
				return fmt.Errorf("invalid timeout: %v", err)
			}
			hasTimeout = true
		}

		return runSupervisor(cmd.Context(), gpuIDs, user, pid, timeout, hasTimeout)
	},
}

func init() {
	supervisorCmd.Flags().String("gpus", "", "Comma-separated GPU IDs to manage")
	supervisorCmd.Flags().String("user", "", "User who owns the reservation")
	supervisorCmd.Flags().String("pid", "", "PID of the process to monitor")
	supervisorCmd.Flags().String("timeout", "", "Timeout duration for the command")

	rootCmd.AddCommand(supervisorCmd)
}

// parseGPUList parses a comma-separated list of GPU IDs
func parseGPUList(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty GPU list")
	}

	parts := strings.Split(s, ",")
	gpuIDs := make([]int, len(parts))
	for i, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid GPU ID %q: %v", part, err)
		}
		gpuIDs[i] = id
	}
	return gpuIDs, nil
}

// runSupervisor runs the supervisor loop that monitors a process and maintains GPU heartbeats
func runSupervisor(ctx context.Context, gpuIDs []int, user string, pid int, timeout time.Duration, hasTimeout bool) error {
	config := getConfig()
	client := redis_client.NewClient(config)
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "supervisor: warning: failed to close Redis client: %v\n", err)
		}
	}()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("supervisor: failed to connect to Redis: %v", err)
	}

	// Start heartbeat manager
	heartbeat := gpu.NewHeartbeatManager(client, gpuIDs, user)
	if err := heartbeat.Start(); err != nil {
		return fmt.Errorf("supervisor: failed to start heartbeat: %v", err)
	}
	defer heartbeat.Stop()

	// Set up timeout handling if configured
	var timeoutChan <-chan time.Time
	if hasTimeout {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutChan = timer.C
	}

	// Monitor the process
	pollInterval := 500 * time.Millisecond
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, exit gracefully
			return nil

		case <-timeoutChan:
			// Timeout reached - attempt graceful shutdown
			fmt.Fprintf(os.Stderr, "supervisor: timeout reached after %s, sending SIGINT to process %d\n",
				utils.FormatDuration(timeout), pid)

			// Send SIGINT for graceful shutdown
			if err := syscall.Kill(pid, syscall.SIGINT); err != nil {
				// Process may have already exited
				if !isProcessRunning(pid) {
					return nil
				}
				fmt.Fprintf(os.Stderr, "supervisor: failed to send SIGINT: %v\n", err)
			}

			// Wait grace period
			gracePeriod := 30 * time.Second
			fmt.Fprintf(os.Stderr, "supervisor: waiting %s for graceful shutdown...\n", gracePeriod)
			time.Sleep(gracePeriod)

			// Check if process is still running
			if isProcessRunning(pid) {
				fmt.Fprintf(os.Stderr, "supervisor: grace period expired, sending SIGKILL to process %d\n", pid)
				if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
					fmt.Fprintf(os.Stderr, "supervisor: failed to send SIGKILL: %v\n", err)
				}
			}

			// Process should be dead now, exit
			return nil

		case <-ticker.C:
			// Check if process is still running
			if !isProcessRunning(pid) {
				// Process has exited, we're done
				// The deferred heartbeat.Stop() will release GPUs
				return nil
			}
		}
	}
}

// isProcessRunning checks if a process with the given PID is still running
func isProcessRunning(pid int) bool {
	// Sending signal 0 checks if process exists without actually sending a signal
	err := syscall.Kill(pid, 0)
	return err == nil
}
