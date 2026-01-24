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

// ExitCodeError is an error that carries an exit code
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string {
	return e.Message
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set",
	Long: `Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES automatically set.

The command will:
1. Reserve the requested number of GPUs (or specific GPU IDs)
2. Set CUDA_VISIBLE_DEVICES to the allocated GPU IDs
3. Run your command with full interactive terminal support
4. Automatically release GPUs when the command finishes
5. Maintain a heartbeat while running to keep the reservation active

Interactive programs (like Python REPL, codex, vim, etc.) are fully supported.
Signals like Ctrl-C go directly to your command.

By default, if GPUs are not available, the command will wait in a queue until
resources become available (FCFS - First Come First Served). Use --nonblock to
fail immediately instead.

You can reserve GPUs in two ways:
- By count: --gpus N (allocates N GPUs using MRU-per-user strategy)
- By specific IDs: --gpu-ids 1,3,5 (reserves exactly those GPU IDs)

When using --gpu-ids, the --gpus flag is optional if:
- It matches the number of GPU IDs specified, or
- It is 1 (the default value)

If specific GPU IDs are requested and any are not available, the entire
reservation will wait in the queue until those specific IDs become available.

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
  canhazgpu run --nonblock --gpus 4 -- python train.py  # Fail if unavailable
  canhazgpu run --wait 30m --gpus 4 -- python train.py  # Wait up to 30 minutes

Timeout formats supported:
- 30s (30 seconds)
- 30m (30 minutes)
- 2h (2 hours)
- 1d (1 day)
- 0.5h (30 minutes with decimal)

The '--' separator is required - it tells canhazgpu where its options end
and your command begins.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		gpuCount := viper.GetInt("run.gpus")
		gpuIDs := viper.GetIntSlice("run.gpu-ids")
		timeoutStr := viper.GetString("run.timeout")
		note := viper.GetString("run.note")
		customUser := viper.GetString("run.user")
		nonblock := viper.GetBool("run.nonblock")
		waitStr := viper.GetString("run.wait")

		// Check if "--" separator was used
		dashIndex := cmd.ArgsLenAtDash()

		// Validate command arguments (requires "--" separator)
		if err := validateRunCommand(args, dashIndex); err != nil {
			return err
		}

		err := runRun(cmd.Context(), gpuCount, gpuIDs, timeoutStr, note, customUser, nonblock, waitStr, args)

		// Handle exit code errors
		if exitErr, ok := err.(*ExitCodeError); ok {
			os.Exit(exitErr.Code)
		}

		return err
	},
	DisableFlagsInUseLine: true,
}

func init() {
	runCmd.Flags().IntP("gpus", "g", 1, "Number of GPUs to reserve")
	runCmd.Flags().IntSliceP("gpu-ids", "G", nil, "Specific GPU IDs to reserve (comma-separated, e.g., 1,3,5)")
	runCmd.Flags().StringP("timeout", "t", "", "Timeout duration for graceful command termination (e.g., 30m, 2h, 1d). Disabled by default.")
	runCmd.Flags().StringP("note", "n", "", "Optional note describing the reservation purpose")
	runCmd.Flags().StringP("user", "u", "", "Custom user identifier (e.g., your name when using a shared account)")
	runCmd.Flags().Bool("nonblock", false, "Fail immediately if GPUs are unavailable instead of waiting in queue")
	runCmd.Flags().StringP("wait", "w", "", "Maximum time to wait for GPUs (e.g., 30m, 2h). Default: wait forever.")

	// Require explicit -- separator: only parse flags before --, everything after is treated as opaque args
	runCmd.Flags().SetInterspersed(false)

	rootCmd.AddCommand(runCmd)
}

// validateRunCommand validates that a command was provided with required "--" separator
func validateRunCommand(args []string, dashIndex int) error {
	// Case 1: No arguments at all
	if len(args) == 0 {
		return fmt.Errorf("no command specified. You must provide a command to run.\n\nUsage: canhazgpu run [flags] -- <command>\n\nExamples:\n  canhazgpu run --gpus 1 -- python train.py\n  canhazgpu run --gpu-ids 0,2 -- python -m torch.distributed.launch train.py\n\nNote: The '--' separator is required to separate canhazgpu flags from your command")
	}

	// Case 2: Check if "--" separator was used
	// dashIndex == -1 means no "--" was found
	if dashIndex == -1 {
		return fmt.Errorf("missing '--' separator. You must use '--' to separate canhazgpu flags from your command.\n\nYou provided: canhazgpu run [flags] %s\nCorrect usage: canhazgpu run [flags] -- %s\n\nExamples:\n  canhazgpu run --gpus 1 -- python train.py\n  canhazgpu run --gpu-ids 0,1 -- %s",
			strings.Join(args, " "), strings.Join(args, " "), args[0])
	}

	return nil
}

func runRun(ctx context.Context, gpuCount int, gpuIDs []int, timeoutStr string, note string, customUser string, nonblock bool, waitStr string, command []string) error {
	// Cobra has already processed the "--" separator and given us just the command args

	// If neither is specified, default to 1 GPU
	if gpuCount == 0 && len(gpuIDs) == 0 {
		gpuCount = 1
	}

	config := getConfig()

	// Validate timeout format early (before allocating GPUs)
	if timeoutStr != "" {
		if _, err := utils.ParseDuration(timeoutStr); err != nil {
			return fmt.Errorf("invalid timeout format: %v", err)
		}
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

	client := redis_client.NewClient(config)
	// Note: We don't defer close here because we'll exec() and the process will be replaced

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
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
	request := &gpu.QueuedAllocationRequest{
		AllocationRequest: &types.AllocationRequest{
			GPUCount:        gpuCount,
			GPUIDs:          gpuIDs,
			User:            displayUser,
			ActualUser:      actualUser,
			ReservationType: types.ReservationTypeRun,
			ExpiryTime:      nil, // No expiry for run-type reservations
			Note:            note,
		},
		Blocking:    !nonblock,
		WaitTimeout: waitTimeout,
	}

	// Allocate GPUs (with queue support)
	result, err := engine.AllocateGPUsWithQueue(ctx, request)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("%v", err)
	}
	allocatedGPUs := result.AllocatedGPUs

	// Verify we got the requested number of GPUs
	expectedCount := gpuCount
	if len(gpuIDs) > 0 {
		expectedCount = len(gpuIDs)
	}
	if len(allocatedGPUs) != expectedCount {
		_ = client.Close()
		return fmt.Errorf("failed to allocate requested GPUs: requested %d, got %d", expectedCount, len(allocatedGPUs))
	}

	// Build GPU list string for supervisor
	gpuListParts := make([]string, len(allocatedGPUs))
	for i, gpuID := range allocatedGPUs {
		gpuListParts[i] = strconv.Itoa(gpuID)
	}
	gpuListStr := strings.Join(gpuListParts, ",")

	// Print reservation info
	if timeoutStr != "" {
		timeout, _ := utils.ParseDuration(timeoutStr)
		fmt.Printf("Reserved %d GPU(s): %v for command execution (timeout: %s)\n",
			len(allocatedGPUs), allocatedGPUs, utils.FormatDuration(timeout))
	} else {
		fmt.Printf("Reserved %d GPU(s): %v for command execution\n",
			len(allocatedGPUs), allocatedGPUs)
	}

	// Close Redis client before spawning supervisor (supervisor will create its own)
	_ = client.Close()

	// Get our own executable path for spawning supervisor
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	// Build supervisor command arguments
	supervisorArgs := []string{
		executable,
		"supervisor",
		"--gpus", gpuListStr,
		"--user", displayUser,
		"--pid", strconv.Itoa(os.Getpid()),
	}
	if timeoutStr != "" {
		supervisorArgs = append(supervisorArgs, "--timeout", timeoutStr)
	}

	// Start supervisor process (detached, will monitor us)
	supervisorCmd := exec.Command(supervisorArgs[0], supervisorArgs[1:]...)
	supervisorCmd.Stdout = nil       // Detach stdout
	supervisorCmd.Stderr = os.Stderr // Keep stderr for error messages
	supervisorCmd.Stdin = nil        // No stdin needed

	// Detach the supervisor from our process group so it survives our exec()
	supervisorCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := supervisorCmd.Start(); err != nil {
		return fmt.Errorf("failed to start supervisor: %v", err)
	}

	// Give supervisor a moment to initialize
	time.Sleep(50 * time.Millisecond)

	// Find the binary to exec
	binary, err := exec.LookPath(command[0])
	if err != nil {
		// Kill supervisor since we can't exec
		if supervisorCmd.Process != nil {
			_ = supervisorCmd.Process.Kill()
		}
		return fmt.Errorf("command not found: %s", command[0])
	}

	// Set up environment with CUDA_VISIBLE_DEVICES
	env := os.Environ()
	env = append(env, fmt.Sprintf("CUDA_VISIBLE_DEVICES=%s", gpuListStr))

	// Exec the user's command - this replaces the current process
	// The supervisor will continue running and monitor our PID
	// When we exit, the supervisor will detect it and release GPUs
	err = syscall.Exec(binary, command, env)

	// If we get here, exec failed
	// Kill supervisor since we couldn't exec
	if supervisorCmd.Process != nil {
		_ = supervisorCmd.Process.Kill()
	}
	return fmt.Errorf("failed to exec command: %v", err)
}
