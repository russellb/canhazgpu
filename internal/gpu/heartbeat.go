package gpu

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
)

type HeartbeatManager struct {
	client        *redis_client.Client
	allocatedGPUs []int
	user          string
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
}

func NewHeartbeatManager(client *redis_client.Client, allocatedGPUs []int, user string) *HeartbeatManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &HeartbeatManager{
		client:        client,
		allocatedGPUs: allocatedGPUs,
		user:          user,
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
}

// Start begins sending heartbeats for the allocated GPUs
func (hm *HeartbeatManager) Start() error {
	// Send initial heartbeat synchronously before starting background tasks
	if err := hm.sendHeartbeat(); err != nil {
		return fmt.Errorf("failed to send initial heartbeat: %w", err)
	}

	// Now start background tasks
	go hm.heartbeatLoop()
	return nil
}

// Stop stops the heartbeat and releases GPUs
func (hm *HeartbeatManager) Stop() {
	hm.cancel()
	<-hm.done
	hm.releaseGPUs()
}

// Wait blocks until the heartbeat manager is stopped
func (hm *HeartbeatManager) Wait() {
	<-hm.done
}

// heartbeatLoop sends periodic heartbeats
func (hm *HeartbeatManager) heartbeatLoop() {
	defer close(hm.done)

	ticker := time.NewTicker(types.HeartbeatInterval)
	defer ticker.Stop()

	// Initial heartbeat already sent in Start(), so just loop
	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			if err := hm.sendHeartbeat(); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: Failed to send heartbeat: %v\n", err)
				fmt.Fprintf(os.Stderr, "GPU reservations may be at risk of expiring!\n")
			}
		}
	}
}

// sendHeartbeat updates the last_heartbeat timestamp for all allocated GPUs
func (hm *HeartbeatManager) sendHeartbeat() error {
	now := time.Now()

	for _, gpuID := range hm.allocatedGPUs {
		state, err := hm.client.GetGPUState(hm.ctx, gpuID)
		if err != nil {
			return fmt.Errorf("failed to get state for GPU %d: %v", gpuID, err)
		}

		// Only update if this is still our reservation
		if state.User == hm.user && state.Type == types.ReservationTypeRun {
			state.LastHeartbeat = types.FlexibleTime{Time: now}
			if err := hm.client.SetGPUState(hm.ctx, gpuID, state); err != nil {
				return fmt.Errorf("failed to update heartbeat for GPU %d: %v", gpuID, err)
			}
		} else if state.User != "" {
			// GPU is reserved by someone else - this is expected, skip silently
			continue
		} else {
			// GPU should be reserved by us but isn't - this is a problem!
			return fmt.Errorf("GPU %d reservation lost: expected user=%s, type=%s but found user=%s, type=%s",
				gpuID, hm.user, types.ReservationTypeRun, state.User, state.Type)
		}
	}

	return nil
}

// releaseGPUs releases all allocated GPUs when stopping
func (hm *HeartbeatManager) releaseGPUs() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()

	for _, gpuID := range hm.allocatedGPUs {
		state, err := hm.client.GetGPUState(ctx, gpuID)
		if err != nil {
			continue
		}

		// Only release if this is still our reservation
		if state.User == hm.user && state.Type == types.ReservationTypeRun {
			// Record usage history
			duration := now.Sub(state.StartTime.ToTime()).Seconds()
			usageRecord := &types.UsageRecord{
				User:            state.User,
				GPUID:           gpuID,
				StartTime:       state.StartTime,
				EndTime:         types.FlexibleTime{Time: now},
				Duration:        duration,
				ReservationType: state.Type,
			}

			if err := hm.client.RecordUsageHistory(ctx, usageRecord); err != nil {
				// Log error but don't fail the release
				fmt.Fprintf(os.Stderr, "Warning: failed to record usage history: %v\n", err)
			}

			// Release the GPU
			availableState := &types.GPUState{
				LastReleased: types.FlexibleTime{Time: now},
			}
			if err := hm.client.SetGPUState(ctx, gpuID, availableState); err != nil {
				fmt.Printf("Warning: failed to set GPU %d state to available: %v\n", gpuID, err)
			}
		}
	}
}
