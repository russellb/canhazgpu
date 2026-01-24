package gpu

import (
	"context"
	"testing"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupQueueTestRedis creates a Redis client connected to test database
func setupQueueTestRedis(t *testing.T) *redis_client.Client {
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15, // Use test database
	}

	client := redis_client.NewClient(config)

	// Check if Redis is available
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Skipf("Redis not available for testing: %v", err)
	}

	// Clean entire test database before test
	if err := client.FlushTestDB(ctx); err != nil {
		t.Logf("Warning: failed to flush test DB: %v", err)
	}

	// Cleanup after test
	t.Cleanup(func() {
		if err := client.FlushTestDB(ctx); err != nil {
			t.Logf("Warning: failed to flush test DB in cleanup: %v", err)
		}
		if err := client.Close(); err != nil {
			t.Logf("Warning: failed to close Redis client: %v", err)
		}
	})

	return client
}

func TestQueueEntry_AddAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	// Create a queue entry
	entry := &types.QueueEntry{
		ID:              "test-entry-1",
		User:            "testuser",
		ActualUser:      "testuser",
		RequestedCount:  2,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	// Add to queue
	err := client.AddToQueue(ctx, entry)
	require.NoError(t, err)

	// Get the entry
	retrieved, err := client.GetQueueEntry(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, entry.ID, retrieved.ID)
	assert.Equal(t, entry.User, retrieved.User)
	assert.Equal(t, entry.RequestedCount, retrieved.RequestedCount)
	assert.Equal(t, entry.ReservationType, retrieved.ReservationType)
}

func TestQueueEntry_Remove(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	entry := &types.QueueEntry{
		ID:              "test-entry-remove",
		User:            "testuser",
		RequestedCount:  1,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	// Add to queue
	err := client.AddToQueue(ctx, entry)
	require.NoError(t, err)

	// Remove from queue
	err = client.RemoveFromQueue(ctx, entry.ID)
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err := client.GetQueueEntry(ctx, entry.ID)
	assert.NoError(t, err) // No error, just returns nil
	assert.Nil(t, retrieved)
}

func TestQueueOrdering_FCFS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	// Add entries in order with slight time differences
	entries := []struct {
		id   string
		user string
	}{
		{"entry-1", "alice"},
		{"entry-2", "bob"},
		{"entry-3", "charlie"},
	}

	for i, e := range entries {
		entry := &types.QueueEntry{
			ID:              e.id,
			User:            e.user,
			RequestedCount:  1,
			AllocatedGPUs:   []int{},
			ReservationType: types.ReservationTypeRun,
			EnqueueTime:     types.FlexibleTime{Time: time.Now().Add(time.Duration(i) * time.Second)},
			LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
		}
		err := client.AddToQueue(ctx, entry)
		require.NoError(t, err)
	}

	// Get all entries and verify order
	allEntries, err := client.GetAllQueueEntries(ctx)
	require.NoError(t, err)
	assert.Len(t, allEntries, 3)

	// Verify FCFS order
	assert.Equal(t, "entry-1", allEntries[0].ID)
	assert.Equal(t, "entry-2", allEntries[1].ID)
	assert.Equal(t, "entry-3", allEntries[2].ID)

	// Verify positions
	pos, err := client.GetQueuePosition(ctx, "entry-1")
	require.NoError(t, err)
	assert.Equal(t, 0, pos)

	pos, err = client.GetQueuePosition(ctx, "entry-2")
	require.NoError(t, err)
	assert.Equal(t, 1, pos)

	pos, err = client.GetQueuePosition(ctx, "entry-3")
	require.NoError(t, err)
	assert.Equal(t, 2, pos)
}

func TestQueuePosition_IsFirstInQueue(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	// Add two entries
	entry1 := &types.QueueEntry{
		ID:              "first-entry",
		User:            "alice",
		RequestedCount:  1,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	entry2 := &types.QueueEntry{
		ID:              "second-entry",
		User:            "bob",
		RequestedCount:  1,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now().Add(time.Second)},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	err := client.AddToQueue(ctx, entry1)
	require.NoError(t, err)
	err = client.AddToQueue(ctx, entry2)
	require.NoError(t, err)

	// First entry should be first
	isFirst, err := client.IsFirstInQueue(ctx, "first-entry")
	require.NoError(t, err)
	assert.True(t, isFirst)

	// Second entry should not be first
	isFirst, err = client.IsFirstInQueue(ctx, "second-entry")
	require.NoError(t, err)
	assert.False(t, isFirst)

	// After removing first entry, second should become first
	err = client.RemoveFromQueue(ctx, "first-entry")
	require.NoError(t, err)

	isFirst, err = client.IsFirstInQueue(ctx, "second-entry")
	require.NoError(t, err)
	assert.True(t, isFirst)
}

func TestQueueHeartbeat_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	initialTime := time.Now().Add(-5 * time.Minute)
	entry := &types.QueueEntry{
		ID:              "heartbeat-test",
		User:            "testuser",
		RequestedCount:  1,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: initialTime},
		LastHeartbeat:   types.FlexibleTime{Time: initialTime},
	}

	err := client.AddToQueue(ctx, entry)
	require.NoError(t, err)

	// Update heartbeat
	err = client.UpdateQueueEntryHeartbeat(ctx, entry.ID)
	require.NoError(t, err)

	// Get entry and verify heartbeat was updated
	retrieved, err := client.GetQueueEntry(ctx, entry.ID)
	require.NoError(t, err)

	// Heartbeat should be more recent than initial time
	assert.True(t, retrieved.LastHeartbeat.ToTime().After(initialTime))
}

func TestQueueStaleEntryCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 4)
	require.NoError(t, err)

	// Create a stale entry (heartbeat expired)
	staleTime := time.Now().Add(-5 * time.Minute) // Well past the 2-minute timeout
	staleEntry := &types.QueueEntry{
		ID:              "stale-entry",
		User:            "staleuser",
		RequestedCount:  2,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: staleTime},
		LastHeartbeat:   types.FlexibleTime{Time: staleTime},
	}

	// Create a fresh entry
	freshEntry := &types.QueueEntry{
		ID:              "fresh-entry",
		User:            "freshuser",
		RequestedCount:  1,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	err = client.AddToQueue(ctx, staleEntry)
	require.NoError(t, err)
	err = client.AddToQueue(ctx, freshEntry)
	require.NoError(t, err)

	// Verify both are in queue
	length, err := client.GetQueueLength(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, length)

	// Cleanup stale entries
	cleanedIDs, err := client.CleanupStaleQueueEntries(ctx)
	require.NoError(t, err)
	assert.Len(t, cleanedIDs, 1)
	assert.Contains(t, cleanedIDs, "stale-entry")

	// Verify only fresh entry remains
	length, err = client.GetQueueLength(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, length)

	// Stale entry should be gone
	retrieved, err := client.GetQueueEntry(ctx, "stale-entry")
	require.NoError(t, err)
	assert.Nil(t, retrieved)

	// Fresh entry should still exist
	retrieved, err = client.GetQueueEntry(ctx, "fresh-entry")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
}

func TestQueueEntry_PartialAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	entry := &types.QueueEntry{
		ID:              "partial-entry",
		User:            "testuser",
		RequestedCount:  4,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	err := client.AddToQueue(ctx, entry)
	require.NoError(t, err)

	// Update with partial allocation
	entry.AllocatedGPUs = []int{0, 1}
	err = client.UpdateQueueEntry(ctx, entry)
	require.NoError(t, err)

	// Verify update
	retrieved, err := client.GetQueueEntry(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, retrieved.AllocatedGPUs)

	// Update with more GPUs
	entry.AllocatedGPUs = []int{0, 1, 2, 3}
	err = client.UpdateQueueEntry(ctx, entry)
	require.NoError(t, err)

	// Verify update
	retrieved, err = client.GetQueueEntry(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2, 3}, retrieved.AllocatedGPUs)
}

func TestQueueStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	// Initialize GPU pool
	err := client.SetGPUCount(ctx, 4)
	require.NoError(t, err)

	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	engine := NewAllocationEngine(client, config)

	// Add entries with partial allocations
	entry1 := &types.QueueEntry{
		ID:              "status-entry-1",
		User:            "alice",
		RequestedCount:  3,
		AllocatedGPUs:   []int{0, 1},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	entry2 := &types.QueueEntry{
		ID:              "status-entry-2",
		User:            "bob",
		RequestedCount:  2,
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now().Add(time.Second)},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	err = client.AddToQueue(ctx, entry1)
	require.NoError(t, err)
	err = client.AddToQueue(ctx, entry2)
	require.NoError(t, err)

	// Get queue status
	status, err := engine.GetQueueStatus(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, status.TotalWaiting)
	assert.Equal(t, 5, status.TotalGPUsRequested) // 3 + 2
	assert.Equal(t, 2, status.TotalGPUsAllocated) // 2 from entry1

	assert.Len(t, status.Entries, 2)
	assert.Equal(t, "status-entry-1", status.Entries[0].ID)
	assert.Equal(t, "status-entry-2", status.Entries[1].ID)
}

func TestQueueEntry_RequestedIDs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := setupQueueTestRedis(t)
	ctx := context.Background()

	// Entry requesting specific GPU IDs
	entry := &types.QueueEntry{
		ID:              "specific-ids-entry",
		User:            "testuser",
		RequestedCount:  0, // When using RequestedIDs, count may be 0
		RequestedIDs:    []int{1, 3, 5},
		AllocatedGPUs:   []int{},
		ReservationType: types.ReservationTypeRun,
		EnqueueTime:     types.FlexibleTime{Time: time.Now()},
		LastHeartbeat:   types.FlexibleTime{Time: time.Now()},
	}

	err := client.AddToQueue(ctx, entry)
	require.NoError(t, err)

	// Verify RequestedIDs are preserved
	retrieved, err := client.GetQueueEntry(ctx, entry.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 3, 5}, retrieved.RequestedIDs)

	// Verify GetRequestedGPUCount works correctly
	assert.Equal(t, 3, retrieved.GetRequestedGPUCount())
}
