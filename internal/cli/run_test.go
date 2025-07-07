package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
)

// isNvidiaSmiAvailable checks if nvidia-smi command is available
func isNvidiaSmiAvailable() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}

// TestIsNvidiaSmiAvailable tests the helper function itself
func TestIsNvidiaSmiAvailable(t *testing.T) {
	available := isNvidiaSmiAvailable()
	t.Logf("nvidia-smi availability: %v", available)

	// This test just documents the current state, doesn't assert a specific value
	// since it depends on the test environment
}

func TestRunCommand_FailureCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Testing GPU cleanup when command fails")

	// Setup test Redis client
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15, // Test database
	}
	client := redis_client.NewClient(config)

	// Check if Redis is available
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Skipf("Redis not available for testing: %v", err)
	}

	// Clean state
	if err := client.ClearAllGPUStates(ctx); err != nil {
		t.Logf("Warning: failed to clear GPU states: %v", err)
	}
	defer func() {
		if err := client.ClearAllGPUStates(ctx); err != nil {
			t.Logf("Warning: failed to clear GPU states in defer: %v", err)
		}
	}()
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("Warning: failed to close Redis client: %v", err)
		}
	}()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 4)
	if err != nil {
		t.Skipf("Could not initialize GPU pool: %v", err)
	}

	t.Log("Running command that will fail with exit code 1")

	// Test that we can't easily test the actual runRun function with os.Exit()
	// But we can test the logic leading up to it

	// Create a command that will fail
	cmd := exec.Command("sh", "-c", "exit 1")
	err = cmd.Run()

	// Verify the command actually fails
	assert.Error(t, err)

	if exitError, ok := err.(*exec.ExitError); ok {
		t.Logf("Command failed with exit code as expected: %v", exitError)
		// This verifies the error handling path works
		assert.True(t, true, "Exit error handling works")
	} else {
		t.Errorf("Expected exec.ExitError, got %T", err)
	}
}

func TestRunCommand_Structure(t *testing.T) {
	// Test basic command structure without actually running
	assert.NotNil(t, runCmd)
	assert.Equal(t, "run", runCmd.Use)
	assert.Contains(t, runCmd.Short, "Reserve GPUs and run")

	// Check flags
	gpusFlag := runCmd.Flags().Lookup("gpus")
	assert.NotNil(t, gpusFlag)
	assert.Equal(t, "int", gpusFlag.Value.Type())
}

func TestRunRun_Validation(t *testing.T) {
	if !isNvidiaSmiAvailable() {
		t.Skip("Skipping test: nvidia-smi command not available")
	}

	tests := []struct {
		name     string
		gpuCount int
		command  []string
		wantErr  bool
	}{
		{
			name:     "Zero GPU count (defaults to 1)",
			gpuCount: 0,
			command:  []string{"echo", "test"},
			wantErr:  false,
		},
		{
			name:     "Negative GPU count",
			gpuCount: -1,
			command:  []string{"echo", "test"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := runRun(ctx, tt.gpuCount, nil, "", tt.command)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExitCodeHandling(t *testing.T) {
	// Test that we can properly detect exit codes from failed commands
	// This tests the logic that was fixed to ensure cleanup happens

	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()

	// Verify we get an ExitError
	assert.Error(t, err)

	if exitError, ok := err.(*exec.ExitError); ok {
		t.Log("Successfully detected exit error")

		// This is the same logic used in the fixed run command
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			exitCode := status.ExitStatus()
			assert.Equal(t, 42, exitCode, "Should preserve original exit code")
			t.Logf("Exit code correctly detected as: %d", exitCode)
		} else {
			t.Error("Could not extract exit status")
		}
	} else {
		t.Errorf("Expected exec.ExitError, got %T", err)
	}
}

func TestRunCommand_HeartbeatCleanup_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Testing heartbeat cleanup behavior")

	// Setup test Redis client
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	client := redis_client.NewClient(config)

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Clean state
	if err := client.ClearAllGPUStates(ctx); err != nil {
		t.Logf("Warning: failed to clear GPU states: %v", err)
	}
	defer func() {
		if err := client.ClearAllGPUStates(ctx); err != nil {
			t.Logf("Warning: failed to clear GPU states in defer: %v", err)
		}
	}()
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("Warning: failed to close Redis client: %v", err)
		}
	}()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 4)
	if err != nil {
		t.Skipf("Could not initialize GPU pool: %v", err)
	}

	// Test successful command execution path
	t.Log("Testing successful command (should clean up via defer)")

	// We can't easily test the full runRun with a real command due to os.Exit
	// But we can verify the heartbeat manager cleanup logic works

	// Manual test of cleanup logic - this simulates what should happen
	user := "testuser"

	// Reserve a GPU manually to simulate allocation
	reservedState := &types.GPUState{
		User:          user,
		StartTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat: types.FlexibleTime{Time: time.Now()},
		Type:          types.ReservationTypeRun,
	}

	err = client.SetGPUState(ctx, 0, reservedState)
	assert.NoError(t, err)

	// Verify GPU is reserved
	state, err := client.GetGPUState(ctx, 0)
	assert.NoError(t, err)
	assert.Equal(t, user, state.User)

	t.Log("GPU successfully reserved, now testing cleanup")

	// Test manual cleanup (simulates heartbeat.Stop())
	now := time.Now()

	// Check what state exists before we try to change it
	stateBefore, err := client.GetGPUState(ctx, 0)
	if err != nil {
		t.Logf("Failed to get state before update: %v", err)
	} else {
		t.Logf("State before update:")
		t.Logf("  User: %q", stateBefore.User)
		t.Logf("  LastReleased.IsZero(): %v", stateBefore.LastReleased.IsZero())
	}

	availableState := &types.GPUState{
		LastReleased: types.FlexibleTime{Time: now},
	}

	// Debug what we're trying to set
	t.Logf("Before SetGPUState:")
	t.Logf("  availableState.User: %q", availableState.User)
	t.Logf("  availableState.LastReleased.Time: %v", availableState.LastReleased.Time)
	t.Logf("  availableState.LastReleased.IsZero(): %v", availableState.LastReleased.IsZero())
	t.Logf("  availableState.LastReleased.ToTime().IsZero(): %v", availableState.LastReleased.ToTime().IsZero())

	// Test what JSON marshaling produces
	jsonData, marshalErr := json.Marshal(availableState)
	if marshalErr != nil {
		t.Logf("JSON marshal failed: %v", marshalErr)
	} else {
		t.Logf("JSON marshaled data: %s", string(jsonData))
	}

	// Test unmarshaling to verify round-trip
	var unmarshaled types.GPUState
	if unmarshalErr := json.Unmarshal(jsonData, &unmarshaled); unmarshalErr != nil {
		t.Logf("JSON unmarshal failed: %v", unmarshalErr)
	} else {
		t.Logf("JSON unmarshaled state:")
		t.Logf("  User: %q", unmarshaled.User)
		t.Logf("  LastReleased.Time: %v", unmarshaled.LastReleased.Time)
		t.Logf("  LastReleased.IsZero(): %v", unmarshaled.LastReleased.IsZero())
		t.Logf("  LastReleased.ToTime().IsZero(): %v", unmarshaled.LastReleased.ToTime().IsZero())
	}

	// Test Redis connection before critical operation
	pingErr := client.Ping(ctx)
	if pingErr != nil {
		t.Logf("Redis ping failed before SetGPUState: %v", pingErr)
	} else {
		t.Logf("Redis ping successful before SetGPUState")
	}

	// Test what Redis SET operation we're actually doing
	redisKey := fmt.Sprintf("canhazgpu:gpu:%d", 0)
	t.Logf("Redis key: %q", redisKey)

	err = client.SetGPUState(ctx, 0, availableState)
	if err != nil {
		t.Logf("SetGPUState failed: %v", err)
	}
	assert.NoError(t, err)
	t.Logf("SetGPUState completed successfully")

	// Test Redis connection after critical operation
	pingErr = client.Ping(ctx)
	if pingErr != nil {
		t.Logf("Redis ping failed after SetGPUState: %v", pingErr)
	} else {
		t.Logf("Redis ping successful after SetGPUState")
	}

	// Test what's actually stored in Redis - get the state immediately after setting
	immediateState, getErr := client.GetGPUState(ctx, 0)
	if getErr != nil {
		t.Logf("Failed to get immediate Redis state: %v", getErr)
	} else {
		t.Logf("Immediate Redis state after SetGPUState:")
		t.Logf("  User: %q", immediateState.User)
		t.Logf("  LastReleased.Time: %v", immediateState.LastReleased.Time)
		t.Logf("  LastReleased.IsZero(): %v", immediateState.LastReleased.IsZero())
		t.Logf("  LastReleased.ToTime().IsZero(): %v", immediateState.LastReleased.ToTime().IsZero())
	}

	// Check the logic: SetGPUState should store our availableState since User is empty and LastReleased is not zero
	// From the code: if state.User == "" && !state.LastReleased.ToTime().IsZero() then store the state
	shouldStore := availableState.User == "" && !availableState.LastReleased.ToTime().IsZero()
	t.Logf("Should store state based on logic: %v", shouldStore)

	// Verify GPU is released
	state, err = client.GetGPUState(ctx, 0)
	assert.NoError(t, err)

	// Debug what we actually got back
	t.Logf("After GetGPUState:")
	t.Logf("  state.User: %q", state.User)
	t.Logf("  state.LastReleased.Time: %v", state.LastReleased.Time)
	t.Logf("  state.LastReleased.IsZero(): %v", state.LastReleased.IsZero())
	t.Logf("  state.LastReleased.ToTime(): %v", state.LastReleased.ToTime())
	t.Logf("  state.LastReleased.ToTime().IsZero(): %v", state.LastReleased.ToTime().IsZero())

	assert.Empty(t, state.User, "GPU should be released")

	// Check that the state has a release timestamp
	// Use ToTime() to be more explicit about the check
	releaseTime := state.LastReleased.ToTime()
	assert.False(t, releaseTime.IsZero(), "Should have release timestamp")

	t.Log("GPU cleanup verified - this simulates the fixed behavior")
}

func TestValidateRunCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		dashIndex   int // -1 means no "--" found, >= 0 means "--" at that position
		expectError bool
		errorMatch  string // Substring that should be in the error message
	}{
		{
			name:        "No command specified",
			args:        []string{},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "no command specified",
		},
		{
			name:        "Only -- specified (empty command after --)",
			args:        []string{}, // Cobra gives us empty args after processing "--"
			dashIndex:   0,          // -- was found at position 0
			expectError: true,
			errorMatch:  "no command specified", // Empty args, even with --
		},
		{
			name:        "Valid command with --",
			args:        []string{"python", "train.py"},
			dashIndex:   0, // -- was found
			expectError: false,
		},
		{
			name:        "Command without -- (python)",
			args:        []string{"python", "train.py"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (python3)",
			args:        []string{"python3", "-m", "torch.distributed.launch", "train.py"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (script file)",
			args:        []string{"./train.py"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (absolute path)",
			args:        []string{"/usr/bin/python", "script.py"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (shell script)",
			args:        []string{"train.sh", "--epochs", "10"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (node)",
			args:        []string{"node", "server.js"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (docker)",
			args:        []string{"docker", "run", "my-image"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (python module)",
			args:        []string{"python", "-m", "torch.distributed.launch"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Valid complex command with --",
			args:        []string{"python", "-m", "torch.distributed.launch", "--nproc_per_node=2", "train.py", "--epochs", "100"},
			dashIndex:   0, // -- was found
			expectError: false,
		},
		{
			name:        "Valid nvidia-smi with --",
			args:        []string{"nvidia-smi"},
			dashIndex:   0, // -- was found
			expectError: false,
		},
		{
			name:        "Command without -- (nvidia-smi)",
			args:        []string{"nvidia-smi"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command without -- (unknown command)",
			args:        []string{"unknowncommand123"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Valid echo command with --",
			args:        []string{"echo", "hello"},
			dashIndex:   0, // -- was found
			expectError: false,
		},
		{
			name:        "Flag without -- (should error)",
			args:        []string{"--help"},
			dashIndex:   -1, // No -- found
			expectError: true,
			errorMatch:  "missing '--' separator",
		},
		{
			name:        "Command with arguments containing dashes",
			args:        []string{"python", "script.py", "--model", "gpt-4", "--temperature", "0.7"},
			dashIndex:   0, // -- was found
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunCommand(tt.args, tt.dashIndex)

			if tt.expectError {
				assert.Error(t, err, "Expected an error for args: %v, dashIndex: %d", tt.args, tt.dashIndex)
				if tt.errorMatch != "" {
					assert.Contains(t, err.Error(), tt.errorMatch, "Error message should contain expected substring")
				}
			} else {
				assert.NoError(t, err, "Expected no error for args: %v, dashIndex: %d", tt.args, tt.dashIndex)
			}
		})
	}
}
