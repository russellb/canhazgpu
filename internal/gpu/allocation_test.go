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

	if !isAnyGPUProviderAvailable() {
		t.Skip("Skipping test: no GPU providers available (nvidia-smi, amd-smi not found)")
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

	if !isAnyGPUProviderAvailable() {
		t.Skip("Skipping test: no GPU providers available (nvidia-smi, amd-smi not found)")
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

	t.Log("Attempting GPU allocation (requires GPU provider validation - may be slow)")
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

func TestReleaseSpecificGPUs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test Redis client setup
	config := &types.Config{
		RedisHost:       "localhost",
		RedisPort:       6379,
		RedisDB:         15,
		MemoryThreshold: types.MemoryThresholdMB,
	}
	redisClient := redis_client.NewClient(config)
	defer func() {
		if err := redisClient.Close(); err != nil {
			t.Logf("Warning: failed to close Redis client: %v", err)
		}
	}()

	ctx := context.Background()

	// Check Redis connectivity
	if err := redisClient.Ping(ctx); err != nil {
		t.Skip("Skipping test: Redis not available")
	}

	// Initialize test environment
	if err := redisClient.SetGPUCount(ctx, 4); err != nil {
		t.Fatal(err)
	}

	engine := NewAllocationEngine(redisClient, config)

	// Test 1: Release specific manually reserved GPUs
	t.Run("ReleaseSpecificManualGPUs", func(t *testing.T) {
		// Clean up state
		for i := 0; i < 4; i++ {
			if err := redisClient.SetGPUState(ctx, i, &types.GPUState{}); err != nil {
				t.Fatalf("Failed to reset GPU %d state: %v", i, err)
			}
		}

		// Reserve GPUs 0, 1, 2 manually
		now := time.Now()
		expiryTime := now.Add(1 * time.Hour)

		for i := 0; i < 3; i++ {
			state := &types.GPUState{
				User:       "testuser",
				StartTime:  types.FlexibleTime{Time: now},
				Type:       types.ReservationTypeManual,
				ExpiryTime: types.FlexibleTime{Time: expiryTime},
			}
			if err := redisClient.SetGPUState(ctx, i, state); err != nil {
				t.Fatal(err)
			}
		}

		// Release specific GPUs 0 and 2
		released, err := engine.ReleaseSpecificGPUs(ctx, "testuser", []int{0, 2})
		assert.NoError(t, err)
		assert.ElementsMatch(t, []int{0, 2}, released)

		// Verify GPUs 0 and 2 are released
		for _, gpuID := range []int{0, 2} {
			state, err := redisClient.GetGPUState(ctx, gpuID)
			assert.NoError(t, err)
			assert.Empty(t, state.User)
			assert.NotZero(t, state.LastReleased.Time)
		}

		// Verify GPU 1 is still reserved
		state, err := redisClient.GetGPUState(ctx, 1)
		assert.NoError(t, err)
		assert.Equal(t, "testuser", state.User)
		assert.Equal(t, types.ReservationTypeManual, state.Type)
	})

	// Test 2: Release specific run-type GPUs
	t.Run("ReleaseSpecificRunGPUs", func(t *testing.T) {
		// Clean up state
		for i := 0; i < 4; i++ {
			if err := redisClient.SetGPUState(ctx, i, &types.GPUState{}); err != nil {
				t.Fatalf("Failed to reset GPU %d state: %v", i, err)
			}
		}

		// Reserve GPU 1 as run-type
		now := time.Now()
		state := &types.GPUState{
			User:          "testuser",
			StartTime:     types.FlexibleTime{Time: now},
			LastHeartbeat: types.FlexibleTime{Time: now},
			Type:          types.ReservationTypeRun,
		}
		if err := redisClient.SetGPUState(ctx, 1, state); err != nil {
			t.Fatal(err)
		}

		// Release the run-type GPU
		released, err := engine.ReleaseSpecificGPUs(ctx, "testuser", []int{1})
		assert.NoError(t, err)
		assert.Equal(t, []int{1}, released)

		// Verify GPU 1 is released
		state, err = redisClient.GetGPUState(ctx, 1)
		assert.NoError(t, err)
		assert.Empty(t, state.User)
		assert.NotZero(t, state.LastReleased.Time)
	})

	// Test 3: No GPUs released if not owned by user
	t.Run("NoReleaseIfNotOwned", func(t *testing.T) {
		// Clean up state
		for i := 0; i < 4; i++ {
			if err := redisClient.SetGPUState(ctx, i, &types.GPUState{}); err != nil {
				t.Fatalf("Failed to reset GPU %d state: %v", i, err)
			}
		}

		// Reserve GPU 0 by different user
		state := &types.GPUState{
			User:      "otheruser",
			StartTime: types.FlexibleTime{Time: time.Now()},
			Type:      types.ReservationTypeManual,
		}
		if err := redisClient.SetGPUState(ctx, 0, state); err != nil {
			t.Fatal(err)
		}

		// Try to release as testuser
		released, err := engine.ReleaseSpecificGPUs(ctx, "testuser", []int{0})
		assert.NoError(t, err)
		assert.Empty(t, released)

		// Verify GPU 0 is still owned by otheruser
		state, err = redisClient.GetGPUState(ctx, 0)
		assert.NoError(t, err)
		assert.Equal(t, "otheruser", state.User)
	})

	// Test 4: Handle non-existent GPU IDs gracefully
	t.Run("HandleNonExistentGPUs", func(t *testing.T) {
		// Try to release GPUs that don't exist
		released, err := engine.ReleaseSpecificGPUs(ctx, "testuser", []int{10, 20})
		assert.NoError(t, err)
		assert.Empty(t, released)
	})
}
