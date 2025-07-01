package gpu

import (
	"context"
	"testing"

	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
)

// TestParseNvidiaSmiOutput would test parsing nvidia-smi output
// This requires implementing parseNvidiaSmiOutput function if it's internal
// The parsing logic is currently part of queryGPUMemory and queryGPUProcesses
// Skipping detailed parsing tests and testing the public interface instead

func TestGetProcessOwner(t *testing.T) {
	tests := []struct {
		name      string
		pid       int
		wantError bool
	}{
		{
			name:      "Current process",
			pid:       1, // init process should exist
			wantError: false,
		},
		{
			name:      "Invalid PID",
			pid:       -1,
			wantError: true,
		},
		{
			name:      "Non-existent PID",
			pid:       999999,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := getProcessOwner(tt.pid)
			
			if tt.wantError {
				assert.Error(t, err)
				assert.Empty(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, user)
			}
		})
	}
}

// TestFilterGPUUsage would test GPU usage filtering
// This requires implementing filterGPUUsage function if it's internal
// The filtering logic is currently part of the main DetectGPUUsage function
// Skipping internal function tests

func TestDetectGPUUsage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Starting nvidia-smi integration test - may take 5-10 seconds or timeout")
	t.Log("This test requires nvidia-smi command to be available on the system")
	
	// This test requires nvidia-smi to be available
	usage, err := DetectGPUUsage(context.Background())
	
	// If nvidia-smi is not available, the function should handle it gracefully
	if err != nil {
		t.Logf("nvidia-smi not available or failed: %v (this is expected on non-GPU systems)", err)
		// Should return empty usage, not crash
		assert.Empty(t, usage)
		return
	}
	
	t.Log("nvidia-smi detection completed successfully")

	// If successful, usage should be a valid map
	assert.NotNil(t, usage)
	
	// Each GPU usage should have valid data
	for gpuID, gpuUsage := range usage {
		assert.GreaterOrEqual(t, gpuID, 0)
		assert.Equal(t, gpuID, gpuUsage.GPUID)
		assert.GreaterOrEqual(t, gpuUsage.MemoryMB, 0) // Memory can be 0 or more
		
		t.Logf("GPU %d: %dMB memory usage, %d processes", gpuID, gpuUsage.MemoryMB, len(gpuUsage.Processes))
		
		// Each process should have valid data
		for _, proc := range gpuUsage.Processes {
			assert.Greater(t, proc.PID, 0)
			assert.NotEmpty(t, proc.ProcessName)
			assert.GreaterOrEqual(t, proc.MemoryMB, 0)
		}
	}
}

func TestDetectGPUUsage_Structure(t *testing.T) {
	// Test that DetectGPUUsage function exists and can be called
	usage, err := DetectGPUUsage(context.Background())
	
	// Don't check for success since nvidia-smi might not be available in test environment
	// Just verify it doesn't panic and returns proper types
	assert.NotNil(t, usage) // Should return empty map, not nil
	_ = err // Error is acceptable if nvidia-smi not available
}

func TestGetUnreservedGPUs(t *testing.T) {
	// Test the threshold logic that determines unreserved usage
	usage := map[int]*types.GPUUsage{
		0: {GPUID: 0, MemoryMB: 512},  // Below threshold - authorized
		1: {GPUID: 1, MemoryMB: 1536}, // Above threshold - unreserved  
		2: {GPUID: 2, MemoryMB: 1024}, // At threshold - authorized (uses > not >=)
		3: {GPUID: 3, MemoryMB: 0},    // No usage - authorized
	}
	
	unreserved := GetUnreservedGPUs(context.Background(), usage)
	
	// Should find only GPU 1 (>1024MB threshold, not >=
	expected := []int{1}
	assert.ElementsMatch(t, expected, unreserved)
}

func TestIsGPUInUnreservedUse(t *testing.T) {
	tests := []struct {
		name     string
		usage    *types.GPUUsage
		expected bool
	}{
		{
			name:     "Nil usage",
			usage:    nil,
			expected: false,
		},
		{
			name:     "Below threshold",
			usage:    &types.GPUUsage{MemoryMB: 512},
			expected: false,
		},
		{
			name:     "At threshold", 
			usage:    &types.GPUUsage{MemoryMB: types.MemoryThresholdMB},
			expected: false, // Uses > not >=, so exactly 1024MB is authorized
		},
		{
			name:     "Above threshold",
			usage:    &types.GPUUsage{MemoryMB: 1536},
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGPUInUnreservedUse(tt.usage)
			assert.Equal(t, tt.expected, result)
		})
	}
}