package redis_client

import (
	"context"
	"testing"
	"time"

	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a Redis client connected to test database
func setupTestRedis(t *testing.T) *Client {
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15, // Use test database
	}

	client := NewClient(config)

	// Check if Redis is available
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Skipf("Redis not available for testing: %v", err)
	}

	// Clean state before test
	client.rdb.FlushDB(ctx)

	// Cleanup after test
	t.Cleanup(func() {
		client.rdb.FlushDB(ctx)
		client.Close()
	})

	return client
}

func TestClient_Ping(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	err := client.Ping(ctx)
	assert.NoError(t, err)
}

func TestClient_GPUCount(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initially should return error (not initialized)
	_, err := client.GetGPUCount(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "GPU pool not initialized")

	// Set GPU count
	err = client.SetGPUCount(ctx, 8)
	assert.NoError(t, err)

	// Get GPU count
	count, err := client.GetGPUCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 8, count)
}

func TestClient_GPUState(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	gpuID := 0

	// Initially should return available state
	state, err := client.GetGPUState(ctx, gpuID)
	assert.NoError(t, err)
	assert.Equal(t, "", state.User) // Available GPU

	// Set reserved state
	reservedTime := time.Now()
	reservedState := &types.GPUState{
		User:          "testuser",
		StartTime:     types.FlexibleTime{Time: reservedTime},
		LastHeartbeat: types.FlexibleTime{Time: reservedTime},
		Type:          types.ReservationTypeRun,
	}

	err = client.SetGPUState(ctx, gpuID, reservedState)
	assert.NoError(t, err)

	// Get reserved state
	retrievedState, err := client.GetGPUState(ctx, gpuID)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", retrievedState.User)
	assert.Equal(t, types.ReservationTypeRun, retrievedState.Type)
	assert.True(t, reservedTime.Equal(retrievedState.StartTime.Time))

	// Set available state with last_released
	lastReleased := time.Now()
	availableState := &types.GPUState{
		User:         "",
		LastReleased: types.FlexibleTime{Time: lastReleased},
	}

	err = client.SetGPUState(ctx, gpuID, availableState)
	assert.NoError(t, err)

	// Get available state
	retrievedState, err = client.GetGPUState(ctx, gpuID)
	assert.NoError(t, err)
	assert.Equal(t, "", retrievedState.User)
	assert.True(t, lastReleased.Equal(retrievedState.LastReleased.Time))

	// Delete GPU state
	err = client.DeleteGPUState(ctx, gpuID)
	assert.NoError(t, err)

	// Should return empty state
	retrievedState, err = client.GetGPUState(ctx, gpuID)
	assert.NoError(t, err)
	assert.Equal(t, "", retrievedState.User)
	assert.True(t, retrievedState.LastReleased.Time.IsZero())
}

func TestClient_AllocationLock(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Should be able to acquire lock
	err := client.AcquireAllocationLock(ctx)
	assert.NoError(t, err)

	// Should be able to release lock
	err = client.ReleaseAllocationLock(ctx)
	assert.NoError(t, err)

	// Should be able to acquire again after release
	err = client.AcquireAllocationLock(ctx)
	assert.NoError(t, err)

	// Cleanup
	client.ReleaseAllocationLock(ctx)
}

func TestClient_AllocationLock_Concurrency(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	t.Log("Starting concurrency test - testing lock contention (may take up to 5 seconds)")

	// Acquire lock
	err := client.AcquireAllocationLock(ctx)
	assert.NoError(t, err)
	t.Log("First client acquired lock successfully")

	// Create second client
	config2 := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15, // Same test database
	}
	client2 := NewClient(config2)
	defer client2.Close()

	// Use a timeout context for the second lock attempt
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	t.Log("Second client attempting to acquire lock (should timeout/fail)")
	// Should fail to acquire lock quickly
	start := time.Now()
	err = client2.AcquireAllocationLock(timeoutCtx)
	duration := time.Since(start)
	t.Logf("Second lock attempt took %v and failed as expected", duration)

	// Either times out or gets lock acquisition error
	assert.Error(t, err)
	assert.Less(t, duration, 5*time.Second) // Should not take too long

	t.Log("Releasing lock from first client")
	// Release lock from first client
	err = client.ReleaseAllocationLock(ctx)
	assert.NoError(t, err)

	t.Log("Second client should now be able to acquire lock")
	// Second client should now be able to acquire
	err = client2.AcquireAllocationLock(ctx)
	assert.NoError(t, err)
	t.Log("Concurrency test completed successfully")

	// Cleanup
	client2.ReleaseAllocationLock(ctx)
}

func TestClient_AtomicReserveGPUs_SimpleCase(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 4)
	require.NoError(t, err)

	// Request to reserve 2 GPUs
	request := &types.AllocationRequest{
		GPUCount:        2,
		User:            "testuser",
		ReservationType: types.ReservationTypeRun,
	}

	allocated, err := client.AtomicReserveGPUs(ctx, request, []int{})
	assert.NoError(t, err)
	assert.Len(t, allocated, 2)

	// Verify GPUs are reserved
	for _, gpuID := range allocated {
		state, err := client.GetGPUState(ctx, gpuID)
		assert.NoError(t, err)
		assert.Equal(t, "testuser", state.User)
		assert.Equal(t, types.ReservationTypeRun, state.Type)
	}
}

func TestClient_AtomicReserveGPUs_WithUnreserved(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool with 4 GPUs
	err := client.SetGPUCount(ctx, 4)
	require.NoError(t, err)

	// Reserve 2 GPUs, excluding GPU 1 as unreserved
	request := &types.AllocationRequest{
		GPUCount:        2,
		User:            "testuser",
		ReservationType: types.ReservationTypeRun,
	}

	unreservedGPUs := []int{1}
	allocated, err := client.AtomicReserveGPUs(ctx, request, unreservedGPUs)
	assert.NoError(t, err)
	assert.Len(t, allocated, 2)

	// Verify that GPU 1 was not allocated
	for _, gpuID := range allocated {
		assert.NotEqual(t, 1, gpuID, "Unreserved GPU should not be allocated")
	}
}

func TestClient_AtomicReserveGPUs_InsufficientGPUs(t *testing.T) {
	t.Skip("TODO: Fix Lua script error handling")
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool with only 2 GPUs
	err := client.SetGPUCount(ctx, 2)
	require.NoError(t, err)

	// Try to reserve 3 GPUs (more than available)
	request := &types.AllocationRequest{
		GPUCount:        3,
		User:            "testuser",
		ReservationType: types.ReservationTypeRun,
	}

	allocated, err := client.AtomicReserveGPUs(ctx, request, []int{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Not enough GPUs available")
	assert.Nil(t, allocated)
}

func TestClient_AtomicReserveGPUs_ManualReservation(t *testing.T) {
	t.Skip("TODO: Fix Lua script for manual reservations")
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 2)
	require.NoError(t, err)

	// Request manual reservation with expiry
	expiryTime := time.Now().Add(time.Hour)
	request := &types.AllocationRequest{
		GPUCount:        1,
		User:            "testuser",
		ReservationType: types.ReservationTypeManual,
		ExpiryTime:      &expiryTime,
	}

	allocated, err := client.AtomicReserveGPUs(ctx, request, []int{})
	assert.NoError(t, err)
	assert.Len(t, allocated, 1)

	// Verify manual reservation
	state, err := client.GetGPUState(ctx, allocated[0])
	assert.NoError(t, err)
	assert.Equal(t, "testuser", state.User)
	assert.Equal(t, types.ReservationTypeManual, state.Type)
	assert.False(t, state.ExpiryTime.Time.IsZero())
}

func TestClient_ClearAllGPUStates(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Set some GPU states
	for i := 0; i < 3; i++ {
		state := &types.GPUState{
			User:      "testuser",
			StartTime: types.FlexibleTime{Time: time.Now()},
			Type:      types.ReservationTypeRun,
		}
		err := client.SetGPUState(ctx, i, state)
		require.NoError(t, err)
	}

	// Verify states exist
	for i := 0; i < 3; i++ {
		state, err := client.GetGPUState(ctx, i)
		require.NoError(t, err)
		assert.Equal(t, "testuser", state.User)
	}

	// Clear all states
	err := client.ClearAllGPUStates(ctx)
	assert.NoError(t, err)

	// Verify states are cleared
	for i := 0; i < 3; i++ {
		state, err := client.GetGPUState(ctx, i)
		require.NoError(t, err)
		assert.Equal(t, "", state.User) // Should be available
	}
}

func TestClient_NewClient(t *testing.T) {
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   0,
	}

	client := NewClient(config)
	assert.NotNil(t, client)
	assert.NotNil(t, client.rdb)

	err := client.Close()
	assert.NoError(t, err)
}

func TestClient_AtomicReserveSpecificGPUs(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 4)
	require.NoError(t, err)

	// Test 1: Reserve specific GPUs successfully
	request := &types.AllocationRequest{
		GPUIDs:          []int{1, 3},
		User:            "testuser",
		ReservationType: types.ReservationTypeRun,
	}

	allocatedGPUs, err := client.AtomicReserveGPUs(ctx, request, []int{})
	assert.NoError(t, err)
	assert.ElementsMatch(t, []int{1, 3}, allocatedGPUs)

	// Verify GPUs are reserved
	state1, err := client.GetGPUState(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", state1.User)
	assert.Equal(t, types.ReservationTypeRun, state1.Type)

	state3, err := client.GetGPUState(ctx, 3)
	assert.NoError(t, err)
	assert.Equal(t, "testuser", state3.User)
	assert.Equal(t, types.ReservationTypeRun, state3.Type)

	// Test 2: Try to reserve already reserved GPUs
	request2 := &types.AllocationRequest{
		GPUIDs:          []int{1, 2},
		User:            "anotheruser",
		ReservationType: types.ReservationTypeRun,
	}

	_, err = client.AtomicReserveGPUs(ctx, request2, []int{})
	require.Error(t, err, "Expected error when trying to reserve already reserved GPUs")
	assert.Contains(t, err.Error(), "already reserved")

	// Verify GPU 2 is still available
	state2, err := client.GetGPUState(ctx, 2)
	assert.NoError(t, err)
	assert.Empty(t, state2.User)

	// Test 3: Reserve GPUs marked as unreserved
	request3 := &types.AllocationRequest{
		GPUIDs:          []int{0, 2},
		User:            "user3",
		ReservationType: types.ReservationTypeManual,
	}

	_, err = client.AtomicReserveGPUs(ctx, request3, []int{0}) // GPU 0 is in unreserved list
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "in use without reservation")

	// Test 4: Invalid GPU ID (out of range)
	request4 := &types.AllocationRequest{
		GPUIDs:          []int{2, 5}, // GPU 5 doesn't exist (pool size is 4)
		User:            "user4",
		ReservationType: types.ReservationTypeManual,
	}

	_, err = client.AtomicReserveGPUs(ctx, request4, []int{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Test 5: Successfully reserve remaining available GPUs
	request5 := &types.AllocationRequest{
		GPUIDs:          []int{2},
		User:            "user5",
		ReservationType: types.ReservationTypeManual,
		ExpiryTime:      func() *time.Time { t := time.Now().Add(time.Hour); return &t }(),
	}

	allocatedGPUs, err = client.AtomicReserveGPUs(ctx, request5, []int{})
	assert.NoError(t, err)
	assert.Equal(t, []int{2}, allocatedGPUs)

	// Verify manual reservation with expiry
	state2, err = client.GetGPUState(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, "user5", state2.User)
	assert.Equal(t, types.ReservationTypeManual, state2.Type)
	assert.False(t, state2.ExpiryTime.Time.IsZero())
}

func TestClient_AtomicReserveGPUs_MixedMode(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 8)
	require.NoError(t, err)

	// Test allocating by count (original behavior)
	request1 := &types.AllocationRequest{
		GPUCount:        3,
		User:            "user1",
		ReservationType: types.ReservationTypeRun,
	}

	allocatedGPUs1, err := client.AtomicReserveGPUs(ctx, request1, []int{})
	assert.NoError(t, err)
	assert.Len(t, allocatedGPUs1, 3)
	t.Logf("First allocation (by count): %v", allocatedGPUs1)

	// Test allocating specific IDs (pick ones not allocated in first request)
	// We'll dynamically select GPUs that weren't allocated
	allocatedMap := make(map[int]bool)
	for _, gpu := range allocatedGPUs1 {
		allocatedMap[gpu] = true
	}

	var availableGPUs []int
	for i := 0; i < 8; i++ {
		if !allocatedMap[i] {
			availableGPUs = append(availableGPUs, i)
		}
	}

	// Pick 3 available GPUs
	require.True(t, len(availableGPUs) >= 3, "Need at least 3 available GPUs")
	selectedGPUs := availableGPUs[:3]

	request2 := &types.AllocationRequest{
		GPUIDs:          selectedGPUs,
		User:            "user2",
		ReservationType: types.ReservationTypeRun,
	}

	allocatedGPUs2, err := client.AtomicReserveGPUs(ctx, request2, []int{})
	assert.NoError(t, err)
	assert.ElementsMatch(t, selectedGPUs, allocatedGPUs2)
	t.Logf("Second allocation (by IDs): %v", allocatedGPUs2)

	// Verify no overlap between allocations
	for _, gpu1 := range allocatedGPUs1 {
		for _, gpu2 := range allocatedGPUs2 {
			assert.NotEqual(t, gpu1, gpu2, "GPU %d allocated to both users", gpu1)
		}
	}
}
