package gpu

import (
	"context"
	"testing"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestAllocationEngine_Structure(t *testing.T) {
	// Test Redis client setup (can work without actual Redis)
	config := &types.Config{
		RedisHost:       "localhost",
		RedisPort:       6379,
		RedisDB:         15,
		MemoryThreshold: types.MemoryThresholdMB,
	}
	redisClient := redis_client.NewClient(config)

	engine := NewAllocationEngine(redisClient, config)
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.client)
}

func TestAllocationEngine_GetGPUStatus_Structure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Starting integration test - this may take time if Redis is not available")

	// Test Redis client setup
	config := &types.Config{
		RedisHost:       "localhost",
		RedisPort:       6379,
		RedisDB:         15,
		MemoryThreshold: types.MemoryThresholdMB,
	}
	redisClient := redis_client.NewClient(config)

	engine := NewAllocationEngine(redisClient, config)

	t.Log("Attempting to get GPU status (may timeout if Redis unavailable)")
	// This should not panic even if Redis is empty or GPU count not set
	statuses, err := engine.GetGPUStatus(context.Background())

	// Either success with valid data or controlled error
	if err != nil {
		// Expected if GPU pool not initialized
		assert.Empty(t, statuses)
	} else {
		// If successful, should return valid GPU statuses
		for _, status := range statuses {
			assert.GreaterOrEqual(t, status.GPUID, 0)
			assert.NotEmpty(t, status.Status)
		}
	}
}

func TestAllocationEngine_AllocateGPUs_Structure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Starting GPU allocation integration test - may take 10+ seconds")

	config := &types.Config{
		RedisHost:       "localhost",
		RedisPort:       6379,
		RedisDB:         15,
		MemoryThreshold: types.MemoryThresholdMB,
	}
	redisClient := redis_client.NewClient(config)

	engine := NewAllocationEngine(redisClient, config)

	// Test with valid allocation request structure
	request := &types.AllocationRequest{
		GPUCount:        1,
		User:            "testuser",
		ReservationType: types.ReservationTypeRun,
	}

	// Validate the request structure
	err := request.Validate()
	assert.NoError(t, err)

	t.Log("Attempting GPU allocation (requires nvidia-smi validation - may be slow)")
	// Try to allocate (may fail if pool not initialized, but shouldn't panic)
	gpus, err := engine.AllocateGPUs(context.Background(), request)

	if err != nil {
		// Expected if GPU pool not initialized or no GPUs available
		assert.Empty(t, gpus)
	} else {
		// If successful, should return requested number of GPUs
		assert.Len(t, gpus, request.GPUCount)
		for _, gpu := range gpus {
			assert.GreaterOrEqual(t, gpu, 0)
		}
	}
}

func TestAllocationEngine_ReleaseGPUs_Structure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Starting GPU release integration test")

	config := &types.Config{
		RedisHost:       "localhost",
		RedisPort:       6379,
		RedisDB:         15,
		MemoryThreshold: types.MemoryThresholdMB,
	}
	redisClient := redis_client.NewClient(config)

	engine := NewAllocationEngine(redisClient, config)

	t.Log("Attempting to release GPUs for test user")
	// Test releasing GPUs for a user (should not panic even if no reservations exist)
	releasedGPUs, err := engine.ReleaseGPUs(context.Background(), "testuser")

	// Should either succeed or return controlled error
	// Don't assert specific result since it depends on Redis state
	_ = err
	_ = releasedGPUs
}

func TestAllocationRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request *types.AllocationRequest
		wantErr bool
	}{
		{
			name: "Valid run request",
			request: &types.AllocationRequest{
				GPUCount:        2,
				User:            "testuser",
				ReservationType: types.ReservationTypeRun,
			},
			wantErr: false,
		},
		{
			name: "Valid manual request",
			request: &types.AllocationRequest{
				GPUCount:        1,
				User:            "testuser",
				ReservationType: types.ReservationTypeManual,
			},
			wantErr: false,
		},
		{
			name: "Invalid GPU count zero",
			request: &types.AllocationRequest{
				GPUCount:        0,
				User:            "testuser",
				ReservationType: types.ReservationTypeRun,
			},
			wantErr: true,
		},
		{
			name: "Invalid GPU count negative",
			request: &types.AllocationRequest{
				GPUCount:        -1,
				User:            "testuser",
				ReservationType: types.ReservationTypeRun,
			},
			wantErr: true,
		},
		{
			name: "Empty user",
			request: &types.AllocationRequest{
				GPUCount:        1,
				User:            "",
				ReservationType: types.ReservationTypeRun,
			},
			wantErr: true,
		},
		{
			name: "Invalid reservation type",
			request: &types.AllocationRequest{
				GPUCount:        1,
				User:            "testuser",
				ReservationType: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLRUStrategy_Concepts(t *testing.T) {
	// Test the concept of LRU (Least Recently Used) strategy
	// This tests the logic without requiring actual Redis or GPU hardware

	// LRU should prioritize GPUs that were released longest ago
	now := time.Now()

	// Mock GPU states with different release times
	gpuStates := map[int]types.GPUState{
		0: {LastReleased: types.FlexibleTime{Time: now.Add(-3 * time.Hour)}}, // Released 3h ago
		1: {LastReleased: types.FlexibleTime{Time: now.Add(-1 * time.Hour)}}, // Released 1h ago
		2: {LastReleased: types.FlexibleTime{Time: now.Add(-2 * time.Hour)}}, // Released 2h ago
		3: {},                                                                // Never used (zero time)
	}

	// LRU order should be: 3 (never used), 0 (oldest), 2, 1 (newest)
	expectedOrder := []int{3, 0, 2, 1}

	// Simulate LRU sorting logic
	type gpuWithTime struct {
		id   int
		time time.Time
	}

	var gpus []gpuWithTime
	for id, state := range gpuStates {
		gpus = append(gpus, gpuWithTime{id: id, time: state.LastReleased.Time})
	}

	// Sort by time (zero time first, then oldest first)
	for i := 0; i < len(gpus)-1; i++ {
		for j := i + 1; j < len(gpus); j++ {
			// Zero time (never used) comes first
			if gpus[j].time.IsZero() && !gpus[i].time.IsZero() {
				gpus[i], gpus[j] = gpus[j], gpus[i]
			} else if !gpus[i].time.IsZero() && !gpus[j].time.IsZero() && gpus[j].time.Before(gpus[i].time) {
				gpus[i], gpus[j] = gpus[j], gpus[i]
			}
		}
	}

	// Verify LRU order
	for i, expected := range expectedOrder {
		assert.Equal(t, expected, gpus[i].id, "GPU %d should be at position %d in LRU order", expected, i)
	}
}
