package gpu

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
)

// consecutiveFailures tracks heartbeat failures to trigger reconnection
const maxFailuresBeforeReconnect = 2

var ErrReservationLost = errors.New("GPU reservation lost")

type HeartbeatManager struct {
	client                *redis_client.Client
	allocatedGPUs         []int
	user                  string
	ctx                   context.Context
	cancel                context.CancelFunc
	done                  chan struct{}
	reservationLost       chan struct{}
	reservationLostOnce   sync.Once
	consecutiveFailures   int
	lastSuccessfulHeartbeat time.Time
}

func NewHeartbeatManager(client *redis_client.Client, allocatedGPUs []int, user string) *HeartbeatManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &HeartbeatManager{
		client:                  client,
		allocatedGPUs:           allocatedGPUs,
		user:                    user,
		ctx:                     ctx,
		cancel:                  cancel,
		done:                    make(chan struct{}),
		reservationLost:         make(chan struct{}),
		lastSuccessfulHeartbeat: time.Now(),
	}
}

// Start begins sending heartbeats for the allocated GPUs
func (hm *HeartbeatManager) Start() error {
	// Send initial heartbeat synchronously before starting background tasks
	if err := hm.sendHeartbeat(); err != nil {
		return fmt.Errorf("failed to send initial heartbeat: %w", err)
	}

	hm.lastSuccessfulHeartbeat = time.Now()

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

// ReservationLost returns a channel that is closed when the reservation is
// irrecoverably lost (taken by another user or cleaned up due to stale heartbeat).
func (hm *HeartbeatManager) ReservationLost() <-chan struct{} {
	return hm.reservationLost
}

func (hm *HeartbeatManager) signalReservationLost() {
	hm.reservationLostOnce.Do(func() {
		close(hm.reservationLost)
	})
}

// heartbeatLoop sends periodic heartbeats with connection health checking
func (hm *HeartbeatManager) heartbeatLoop() {
	defer close(hm.done)

	ticker := time.NewTicker(types.HeartbeatInterval)
	defer ticker.Stop()

	// Health check runs more frequently than heartbeats to detect connection
	// problems early. If we only checked at heartbeat time (60s), a dead
	// connection could go unnoticed for up to 60s before we even attempt
	// a reconnect, eating into the 5-minute timeout budget.
	healthTicker := time.NewTicker(types.HealthCheckInterval)
	defer healthTicker.Stop()

	// Initial heartbeat already sent in Start(), so just loop
	for {
		select {
		case <-hm.ctx.Done():
			return

		case <-healthTicker.C:
			hm.checkConnectionHealth()

		case <-ticker.C:
			if err := hm.sendHeartbeat(); err != nil {
				hm.consecutiveFailures++

				if errors.Is(err, ErrReservationLost) {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					hm.signalReservationLost()
					return
				}

				fmt.Fprintf(os.Stderr, "ERROR: Failed to send heartbeat (attempt %d): %v\n",
					hm.consecutiveFailures, err)

				// Try to recover by reconnecting if we've had multiple failures
				if hm.consecutiveFailures >= maxFailuresBeforeReconnect {
					hm.attemptReconnect()
				}

				if time.Since(hm.lastSuccessfulHeartbeat) >= types.HeartbeatTimeout {
					fmt.Fprintf(os.Stderr, "ERROR: No successful heartbeat for %s, reservation is lost\n",
						types.HeartbeatTimeout)
					hm.signalReservationLost()
					return
				}

				fmt.Fprintf(os.Stderr, "GPU reservations may be at risk of expiring!\n")
			} else {
				if hm.consecutiveFailures > 0 {
					fmt.Fprintf(os.Stderr, "Heartbeat recovered after %d failed attempts\n",
						hm.consecutiveFailures)
				}
				hm.consecutiveFailures = 0
				hm.lastSuccessfulHeartbeat = time.Now()
			}
		}
	}
}

// checkConnectionHealth proactively verifies the Redis connection is alive.
// If the connection is dead, it attempts to reconnect before the next heartbeat.
func (hm *HeartbeatManager) checkConnectionHealth() {
	if err := hm.client.HealthCheck(hm.ctx); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Redis health check failed: %v\n", err)
		hm.attemptReconnect()
	}
}

// attemptReconnect tries to re-establish the Redis connection.
func (hm *HeartbeatManager) attemptReconnect() {
	fmt.Fprintf(os.Stderr, "Attempting Redis reconnection...\n")
	if err := hm.client.Reconnect(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Redis reconnection failed: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Redis reconnection successful\n")
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
		} else {
			return fmt.Errorf("%w: GPU %d expected user=%s type=%s, found user=%q type=%q",
				ErrReservationLost, gpuID, hm.user, types.ReservationTypeRun, state.User, state.Type)
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
