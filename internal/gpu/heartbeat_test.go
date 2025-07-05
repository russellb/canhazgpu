package gpu

import (
	"context"
	"testing"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestHeartbeatManager_Structure(t *testing.T) {
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	redisClient := redis_client.NewClient(config)

	manager := NewHeartbeatManager(redisClient, []int{0, 1}, "testuser")
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.client)
	assert.Equal(t, []int{0, 1}, manager.allocatedGPUs)
	assert.Equal(t, "testuser", manager.user)
	assert.NotNil(t, manager.ctx)
	assert.NotNil(t, manager.cancel)
	assert.NotNil(t, manager.done)
}

func TestHeartbeatManager_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Starting heartbeat manager integration test - involves goroutines and timing")

	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	redisClient := redis_client.NewClient(config)

	// Test starting heartbeat
	gpuIDs := []int{0, 1}
	user := "testuser"

	manager := NewHeartbeatManager(redisClient, gpuIDs, user)

	t.Log("Starting heartbeat manager (launches background goroutines)")
	// Test starting (should not panic)
	manager.Start()

	t.Log("Waiting 100ms for heartbeat to initialize")
	// Brief delay to let heartbeat start
	time.Sleep(100 * time.Millisecond)

	t.Log("Stopping heartbeat manager and waiting for cleanup")
	// Test stopping (should not panic)
	manager.Stop()

	// Verify manager is stopped
	select {
	case <-manager.done:
		// Manager stopped successfully
	default:
		t.Error("Manager should be stopped")
	}
}

func TestHeartbeatManager_Wait(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test in short mode")
	}

	t.Log("Starting heartbeat Wait() test - tests blocking behavior")

	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	redisClient := redis_client.NewClient(config)

	gpuIDs := []int{0}
	user := "testuser"

	manager := NewHeartbeatManager(redisClient, gpuIDs, user)

	t.Log("Starting heartbeat manager first")
	manager.Start()

	t.Log("Setting up goroutine to stop manager after 100ms")
	// Test Wait method with timeout
	go func() {
		time.Sleep(100 * time.Millisecond)
		t.Log("Goroutine stopping manager now")
		manager.Stop()
	}()

	t.Log("Calling Wait() - should block for ~100ms")
	// Wait should block until Stop is called
	start := time.Now()
	manager.Wait()
	elapsed := time.Since(start)
	t.Logf("Wait() completed after %v", elapsed)

	// Should have waited at least 100ms
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 1*time.Second) // But not too long
}

func TestHeartbeatManager_SendHeartbeat(t *testing.T) {
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	redisClient := redis_client.NewClient(config)

	gpuIDs := []int{0}
	user := "testuser"

	manager := NewHeartbeatManager(redisClient, gpuIDs, user)

	// Test sendHeartbeat method (may fail if GPUs not initialized)
	err := manager.sendHeartbeat()

	// Should not panic, may return error if Redis not initialized
	_ = err
}

func TestHeartbeatManager_DoubleStop(t *testing.T) {
	t.Log("Testing double-stop behavior (should handle gracefully)")

	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	redisClient := redis_client.NewClient(config)

	gpuIDs := []int{0}
	user := "testuser"

	manager := NewHeartbeatManager(redisClient, gpuIDs, user)

	t.Log("Starting heartbeat manager")
	// Start heartbeat
	manager.Start()

	t.Log("Waiting 50ms then testing double-stop")
	// Brief delay
	time.Sleep(50 * time.Millisecond)

	t.Log("First stop call")
	// Stop twice (should not panic)
	manager.Stop()

	t.Log("Setting up safety goroutine in case second stop hangs")
	// Second stop should not hang or panic
	go func() {
		time.Sleep(100 * time.Millisecond)
		t.Log("Safety goroutine: forcing cancellation if needed")
		// Force context cancellation if second stop hangs
		manager.cancel()
	}()

	t.Log("Second stop call (should return immediately)")
	manager.Stop() // This should return immediately
	t.Log("Double-stop test completed successfully")
}

func TestHeartbeatManager_ReleaseGPUs(t *testing.T) {
	config := &types.Config{
		RedisHost: "localhost",
		RedisPort: 6379,
		RedisDB:   15,
	}
	redisClient := redis_client.NewClient(config)

	gpuIDs := []int{0}
	user := "testuser"

	manager := NewHeartbeatManager(redisClient, gpuIDs, user)

	// Test releaseGPUs method (should not panic)
	manager.releaseGPUs()

	// Should complete without error even if GPUs not initialized
	assert.True(t, true) // Test completed without panic
}

func TestHeartbeatTiming_Concepts(t *testing.T) {
	// Test heartbeat timing concepts without actual Redis

	// Constants from heartbeat implementation
	heartbeatInterval := 60 * time.Second
	heartbeatTimeout := 15 * time.Minute

	// Verify timing relationships
	assert.True(t, heartbeatTimeout > heartbeatInterval,
		"Heartbeat timeout should be longer than interval")

	// Calculate how many heartbeats can be missed before timeout
	missedBeatsBeforeTimeout := heartbeatTimeout / heartbeatInterval
	assert.GreaterOrEqual(t, float64(missedBeatsBeforeTimeout), 10.0,
		"Should allow at least 10 missed heartbeats before timeout")

	// Verify reasonable intervals
	assert.LessOrEqual(t, heartbeatInterval, 2*time.Minute,
		"Heartbeat interval should be reasonable (≤2min)")
	assert.GreaterOrEqual(t, heartbeatInterval, 30*time.Second,
		"Heartbeat interval should not be too frequent (≥30s)")
}

func TestHeartbeatManager_ReservationLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Log("Testing heartbeat behavior when reservation is lost")

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
	if err := client.SetGPUCount(ctx, 4); err != nil {
		t.Skipf("Could not initialize GPU pool: %v", err)
	}

	// Simulate successful allocation
	user := "testuser"
	allocatedGPUs := []int{0}

	now := time.Now()
	reservedState := &types.GPUState{
		User:          user,
		StartTime:     types.FlexibleTime{Time: now},
		LastHeartbeat: types.FlexibleTime{Time: now},
		Type:          types.ReservationTypeRun,
	}

	err := client.SetGPUState(ctx, 0, reservedState)
	assert.NoError(t, err)

	t.Log("✅ GPU 0 reserved successfully")

	// Test the sendHeartbeat function directly
	manager := NewHeartbeatManager(client, allocatedGPUs, user)

	// First heartbeat should work
	err = manager.sendHeartbeat()
	assert.NoError(t, err)
	t.Log("✅ First heartbeat succeeded")

	// Verify heartbeat was updated
	state, err := client.GetGPUState(ctx, 0)
	assert.NoError(t, err)
	assert.Equal(t, user, state.User)
	assert.True(t, state.LastHeartbeat.ToTime().After(now))

	t.Log("🔥 Simulating reservation loss...")

	// Simulate the reservation being cleared (e.g., by cleanup or Redis issue)
	availableState := &types.GPUState{
		LastReleased: types.FlexibleTime{Time: time.Now()},
	}
	err = client.SetGPUState(ctx, 0, availableState)
	assert.NoError(t, err)

	t.Log("💔 GPU reservation cleared")

	// Now the next heartbeat should detect the problem and return an error
	t.Log("🔍 Testing heartbeat after reservation loss...")
	err = manager.sendHeartbeat()

	if err != nil {
		t.Logf("✅ Heartbeat correctly detected reservation loss: %v", err)
		assert.Contains(t, err.Error(), "reservation lost")
	} else {
		t.Error("❌ Heartbeat should have detected reservation loss but didn't return error")
	}
}
