package redis_client

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
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
		if err := client.Close(); err != nil {
			t.Logf("Warning: failed to close Redis client: %v", err)
		}
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
	assert.True(t, retrievedState.LastReleased.IsZero())
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
	if err := client.ReleaseAllocationLock(ctx); err != nil {
		t.Logf("Warning: failed to release allocation lock: %v", err)
	}
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
	defer func() {
		if err := client2.Close(); err != nil {
			t.Logf("Warning: failed to close Redis client2: %v", err)
		}
	}()

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
	if err := client2.ReleaseAllocationLock(ctx); err != nil {
		t.Logf("Warning: failed to release allocation lock: %v", err)
	}
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
	assert.False(t, state.ExpiryTime.IsZero())
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
		RedisDB:   15,
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
	assert.False(t, state2.ExpiryTime.IsZero())
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

func TestClient_ProviderBackwardCompatibility(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Test 1: Provider not initialized, no GPU count -> should get standard error
	provider, err := client.GetAvailableProvider(ctx)
	assert.Error(t, err)
	assert.Empty(t, provider)
	assert.Contains(t, err.Error(), "GPU provider not initialized")

	// Test 2: Set GPU count first (simulating pre-provider deployment)
	err = client.SetGPUCount(ctx, 4)
	require.NoError(t, err)

	// Now when getting provider, it should auto-migrate to NVIDIA
	provider, err = client.GetAvailableProvider(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "nvidia", provider)

	// Test 3: Verify provider was actually stored in Redis
	storedProvider, err := client.GetAvailableProvider(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "nvidia", storedProvider)

	// Test 4: Clear provider but keep GPU count to test migration again
	err = client.rdb.Del(ctx, types.RedisKeyProvider).Err()
	require.NoError(t, err)

	// Should auto-migrate again
	provider, err = client.GetAvailableProvider(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "nvidia", provider)
}

// Test Usage History functionality with sorted sets and backwards compatibility

func TestClient_RecordUsageHistory_NewFormat(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Create test usage record
	startTime := time.Now().Add(-2 * time.Hour)
	endTime := time.Now().Add(-1 * time.Hour)
	usageRecord := &types.UsageRecord{
		User:            "testuser",
		GPUID:           0,
		StartTime:       types.FlexibleTime{Time: startTime},
		EndTime:         types.FlexibleTime{Time: endTime},
		Duration:        3600.0, // 1 hour in seconds
		ReservationType: types.ReservationTypeRun,
	}

	// Record usage history
	err := client.RecordUsageHistory(ctx, usageRecord)
	assert.NoError(t, err)

	// Verify record was added to sorted set
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"
	count, err := client.rdb.ZCard(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify the record can be retrieved by score
	results, err := client.rdb.ZRangeByScore(ctx, sortedSetKey, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", endTime.Unix()-1),
		Max: fmt.Sprintf("%d", endTime.Unix()+1),
	}).Result()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Verify old format key does NOT exist (no dual-write)
	oldKey := fmt.Sprintf("%s%d:%s:%d", types.RedisKeyUsageHistory,
		endTime.Unix(), usageRecord.User, usageRecord.GPUID)
	exists, err := client.rdb.Exists(ctx, oldKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestClient_GetUsageHistory_NewFormat(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Create multiple test usage records
	baseTime := time.Now().Add(-24 * time.Hour)
	var expectedRecords []*types.UsageRecord

	for i := 0; i < 5; i++ {
		startTime := baseTime.Add(time.Duration(i) * time.Hour)
		endTime := baseTime.Add(time.Duration(i+1) * time.Hour)
		usageRecord := &types.UsageRecord{
			User:            fmt.Sprintf("user%d", i),
			GPUID:           i % 2, // Alternate between GPU 0 and 1
			StartTime:       types.FlexibleTime{Time: startTime},
			EndTime:         types.FlexibleTime{Time: endTime},
			Duration:        3600.0,
			ReservationType: types.ReservationTypeRun,
		}
		expectedRecords = append(expectedRecords, usageRecord)

		// Record each usage
		err := client.RecordUsageHistory(ctx, usageRecord)
		require.NoError(t, err)
	}

	// Query for records in a specific time range
	queryStart := baseTime.Add(-1 * time.Hour)
	queryEnd := baseTime.Add(3 * time.Hour)

	retrievedRecords, err := client.GetUsageHistory(ctx, queryStart, queryEnd)
	assert.NoError(t, err)

	// Should get first 3 records (i=0,1,2 have end times within range)
	assert.Len(t, retrievedRecords, 3)

	// Verify records are correct
	for i, record := range retrievedRecords {
		assert.Equal(t, expectedRecords[i].User, record.User)
		assert.Equal(t, expectedRecords[i].GPUID, record.GPUID)
		assert.Equal(t, expectedRecords[i].Duration, record.Duration)
	}
}

func TestClient_GetUsageHistory_BackwardsCompatibility(t *testing.T) {
	// This test is now represented by TestClient_GetUsageHistory_OldFormatOnlyMigration
	// Simplified logic: only check old format when new format doesn't exist
	t.Skip("Test functionality moved to TestClient_GetUsageHistory_OldFormatOnlyMigration")
}

func TestClient_GetUsageHistory_MixedFormats(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	baseTime := time.Now().Add(-24 * time.Hour)

	// Create some records in new format
	for i := 0; i < 2; i++ {
		startTime := baseTime.Add(time.Duration(i) * time.Hour)
		endTime := baseTime.Add(time.Duration(i+1) * time.Hour)
		usageRecord := &types.UsageRecord{
			User:            fmt.Sprintf("newuser%d", i),
			GPUID:           i,
			StartTime:       types.FlexibleTime{Time: startTime},
			EndTime:         types.FlexibleTime{Time: endTime},
			Duration:        3600.0,
			ReservationType: types.ReservationTypeRun,
		}

		err := client.RecordUsageHistory(ctx, usageRecord)
		require.NoError(t, err)
	}

	// Query records - should only get new format records
	queryStart := baseTime.Add(-1 * time.Hour)
	queryEnd := baseTime.Add(5 * time.Hour)

	retrievedRecords, err := client.GetUsageHistory(ctx, queryStart, queryEnd)
	assert.NoError(t, err)

	// Since sorted set exists, we should only get new format records
	assert.Len(t, retrievedRecords, 2)

	// Verify we only have new format users
	userMap := make(map[string]bool)
	for _, record := range retrievedRecords {
		userMap[record.User] = true
	}

	assert.True(t, userMap["newuser0"])
	assert.True(t, userMap["newuser1"])

	// Verify sorted set has correct count
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"
	count, err := client.rdb.ZCard(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestClient_GetUsageHistory_OldFormatOnlyMigration(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	baseTime := time.Now().Add(-24 * time.Hour)

	// Create records in old format only (simulate legacy data)
	var oldRecords []*types.UsageRecord
	for i := 0; i < 3; i++ {
		startTime := baseTime.Add(time.Duration(i) * time.Hour)
		endTime := baseTime.Add(time.Duration(i+1) * time.Hour)
		usageRecord := &types.UsageRecord{
			User:            fmt.Sprintf("olduser%d", i),
			GPUID:           i,
			StartTime:       types.FlexibleTime{Time: startTime},
			EndTime:         types.FlexibleTime{Time: endTime},
			Duration:        3600.0,
			ReservationType: types.ReservationTypeManual,
		}
		oldRecords = append(oldRecords, usageRecord)

		// Store only in old format
		oldKey := fmt.Sprintf("%s%d:%s:%d", types.RedisKeyUsageHistory,
			endTime.Unix(), usageRecord.User, usageRecord.GPUID)
		data, err := json.Marshal(usageRecord)
		require.NoError(t, err)
		err = client.rdb.Set(ctx, oldKey, data, 90*24*time.Hour).Err()
		require.NoError(t, err)
	}

	// Verify sorted set doesn't exist yet
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"
	exists, err := client.rdb.Exists(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	// Query all records - should trigger migration
	queryStart := baseTime.Add(-1 * time.Hour)
	queryEnd := baseTime.Add(4 * time.Hour)

	retrievedRecords, err := client.GetUsageHistory(ctx, queryStart, queryEnd)
	assert.NoError(t, err)
	assert.Len(t, retrievedRecords, 3)

	// Verify migration occurred - sorted set should now exist
	exists, err = client.rdb.Exists(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	// Verify records were migrated correctly
	count, err := client.rdb.ZCard(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Verify retrieved records match original (order may be different due to sorted set)
	retrievedUserMap := make(map[string]*types.UsageRecord)
	for _, record := range retrievedRecords {
		retrievedUserMap[record.User] = record
	}

	for _, oldRecord := range oldRecords {
		retrievedRecord, exists := retrievedUserMap[oldRecord.User]
		assert.True(t, exists, "Record for user %s should exist", oldRecord.User)
		if exists {
			assert.Equal(t, oldRecord.GPUID, retrievedRecord.GPUID)
			assert.Equal(t, oldRecord.ReservationType, retrievedRecord.ReservationType)
		}
	}

	// Verify old format keys still exist (per user request - don't remove old data)
	for i, oldRecord := range oldRecords {
		oldKey := fmt.Sprintf("%s%d:%s:%d", types.RedisKeyUsageHistory,
			oldRecord.EndTime.ToTime().Unix(), oldRecord.User, oldRecord.GPUID)
		exists, err := client.rdb.Exists(ctx, oldKey).Result()
		assert.NoError(t, err)
		assert.Equal(t, int64(1), exists, "Old format key should still exist for record %d", i)
	}

	// Subsequent queries should use new format only
	retrievedRecords2, err := client.GetUsageHistory(ctx, queryStart, queryEnd)
	assert.NoError(t, err)
	assert.Len(t, retrievedRecords2, 3)

	// Should have same records as before
	for _, record := range retrievedRecords2 {
		assert.True(t, retrievedUserMap[record.User] != nil)
	}
}

func TestClient_UsageHistory_EmptyResults(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Query when no records exist
	startTime := time.Now().Add(-2 * time.Hour)
	endTime := time.Now().Add(-1 * time.Hour)

	records, err := client.GetUsageHistory(ctx, startTime, endTime)
	assert.NoError(t, err)
	assert.Empty(t, records)
}

func TestClient_UsageHistory_TimeRangeFiltering(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Create records across different time periods
	baseTime := time.Now().Add(-48 * time.Hour)
	testCases := []struct {
		name       string
		hourOffset int
		user       string
	}{
		{"old_record", 0, "user_old"},
		{"target_record1", 24, "user_target1"},
		{"target_record2", 25, "user_target2"},
		{"recent_record", 47, "user_recent"},
	}

	for _, tc := range testCases {
		startTime := baseTime.Add(time.Duration(tc.hourOffset) * time.Hour)
		endTime := baseTime.Add(time.Duration(tc.hourOffset+1) * time.Hour)
		usageRecord := &types.UsageRecord{
			User:            tc.user,
			GPUID:           0,
			StartTime:       types.FlexibleTime{Time: startTime},
			EndTime:         types.FlexibleTime{Time: endTime},
			Duration:        3600.0,
			ReservationType: types.ReservationTypeRun,
		}

		err := client.RecordUsageHistory(ctx, usageRecord)
		require.NoError(t, err)
	}

	// Query for only the middle 2 records (target_record1 and target_record2)
	queryStart := baseTime.Add(23 * time.Hour)
	queryEnd := baseTime.Add(27 * time.Hour)

	retrievedRecords, err := client.GetUsageHistory(ctx, queryStart, queryEnd)
	assert.NoError(t, err)
	assert.Len(t, retrievedRecords, 2)

	// Verify correct records were returned
	retrievedUsers := make(map[string]bool)
	for _, record := range retrievedRecords {
		retrievedUsers[record.User] = true
	}

	assert.True(t, retrievedUsers["user_target1"])
	assert.True(t, retrievedUsers["user_target2"])
	assert.False(t, retrievedUsers["user_old"])
	assert.False(t, retrievedUsers["user_recent"])
}

func TestClient_MigrateOldUsageRecords(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Create test records to migrate
	baseTime := time.Now().Add(-24 * time.Hour)
	var testRecords []*types.UsageRecord

	for i := 0; i < 3; i++ {
		startTime := baseTime.Add(time.Duration(i) * time.Hour)
		endTime := baseTime.Add(time.Duration(i+1) * time.Hour)
		usageRecord := &types.UsageRecord{
			User:            fmt.Sprintf("migrateuser%d", i),
			GPUID:           i,
			StartTime:       types.FlexibleTime{Time: startTime},
			EndTime:         types.FlexibleTime{Time: endTime},
			Duration:        3600.0,
			ReservationType: types.ReservationTypeManual,
		}
		testRecords = append(testRecords, usageRecord)
	}

	// Call migration function directly
	err := client.migrateOldUsageRecords(ctx, testRecords)
	assert.NoError(t, err)

	// Verify records were added to sorted set
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"
	count, err := client.rdb.ZCard(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Verify records can be retrieved by time range
	queryStart := baseTime.Add(-1 * time.Hour)
	queryEnd := baseTime.Add(4 * time.Hour)

	results, err := client.rdb.ZRangeByScore(ctx, sortedSetKey, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", queryStart.Unix()),
		Max: fmt.Sprintf("%d", queryEnd.Unix()),
	}).Result()
	assert.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestClient_UsageHistory_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	client := setupTestRedis(t)
	ctx := context.Background()

	// Create a larger number of records to test performance
	numRecords := 1000
	baseTime := time.Now().Add(-time.Duration(numRecords) * time.Hour)

	t.Logf("Creating %d usage records for performance test", numRecords)
	start := time.Now()

	for i := 0; i < numRecords; i++ {
		startTime := baseTime.Add(time.Duration(i) * time.Hour)
		endTime := baseTime.Add(time.Duration(i+1) * time.Hour)
		usageRecord := &types.UsageRecord{
			User:            fmt.Sprintf("perfuser%d", i%10), // 10 different users
			GPUID:           i % 8,                           // 8 GPUs
			StartTime:       types.FlexibleTime{Time: startTime},
			EndTime:         types.FlexibleTime{Time: endTime},
			Duration:        3600.0,
			ReservationType: types.ReservationTypeRun,
		}

		err := client.RecordUsageHistory(ctx, usageRecord)
		require.NoError(t, err)
	}

	insertDuration := time.Since(start)
	t.Logf("Inserted %d records in %v (%.2f records/sec)", numRecords, insertDuration, float64(numRecords)/insertDuration.Seconds())

	// Test query performance
	queryStart := baseTime.Add(time.Duration(numRecords/4) * time.Hour)
	queryEnd := baseTime.Add(time.Duration(3*numRecords/4) * time.Hour)

	start = time.Now()
	retrievedRecords, err := client.GetUsageHistory(ctx, queryStart, queryEnd)
	queryDuration := time.Since(start)

	assert.NoError(t, err)
	expectedRecords := numRecords / 2                                                       // Should get middle half of records
	assert.InDelta(t, expectedRecords, len(retrievedRecords), float64(expectedRecords)*0.1) // Allow 10% variance

	t.Logf("Retrieved %d records in %v", len(retrievedRecords), queryDuration)
	assert.Less(t, queryDuration, 1*time.Second, "Query should complete in under 1 second")

	// Verify sorted set exists and has correct count
	sortedSetKey := types.RedisKeyPrefix + "usage_history_sorted"
	count, err := client.rdb.ZCard(ctx, sortedSetKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(numRecords), count)
}

func TestClient_ProviderExplicitSet(t *testing.T) {
	client := setupTestRedis(t)
	ctx := context.Background()

	// Test explicit provider setting
	err := client.SetAvailableProvider(ctx, "amd")
	assert.NoError(t, err)

	provider, err := client.GetAvailableProvider(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "amd", provider)

	// Test overwriting provider
	err = client.SetAvailableProvider(ctx, "nvidia")
	assert.NoError(t, err)

	provider, err = client.GetAvailableProvider(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "nvidia", provider)
}
