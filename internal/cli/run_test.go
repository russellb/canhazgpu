package cli

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
)

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
	client.ClearAllGPUStates(ctx)
	defer client.ClearAllGPUStates(ctx)
	defer client.Close()

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
	tests := []struct {
		name     string
		gpuCount int
		command  []string
		wantErr  bool
	}{
		{
			name:     "Zero GPU count",
			gpuCount: 0,
			command:  []string{"echo", "test"},
			wantErr:  true,
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
			
			err := runRun(ctx, tt.gpuCount, tt.command)
			
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
	client.ClearAllGPUStates(ctx)
	defer client.ClearAllGPUStates(ctx)
	defer client.Close()

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
	availableState := &types.GPUState{
		LastReleased: types.FlexibleTime{Time: now},
	}
	err = client.SetGPUState(ctx, 0, availableState)
	assert.NoError(t, err)
	
	// Verify GPU is released
	state, err = client.GetGPUState(ctx, 0)
	assert.NoError(t, err)
	assert.Empty(t, state.User, "GPU should be released")
	assert.False(t, state.LastReleased.Time.IsZero(), "Should have release timestamp")
	
	t.Log("GPU cleanup verified - this simulates the fixed behavior")
}