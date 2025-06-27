package gpu

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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
func (hm *HeartbeatManager) Start() {
	go hm.heartbeatLoop()
	go hm.signalHandler()
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

	// Send initial heartbeat
	hm.sendHeartbeat()

	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			if err := hm.sendHeartbeat(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to send heartbeat: %v\n", err)
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
			state.LastHeartbeat = types.FlexibleTime{now}
			if err := hm.client.SetGPUState(hm.ctx, gpuID, state); err != nil {
				return fmt.Errorf("failed to update heartbeat for GPU %d: %v", gpuID, err)
			}
		}
	}

	return nil
}

// signalHandler listens for termination signals and stops the heartbeat
func (hm *HeartbeatManager) signalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		hm.cancel()
	case <-hm.ctx.Done():
		return
	}
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
			availableState := &types.GPUState{
				LastReleased: types.FlexibleTime{now},
			}
			hm.client.SetGPUState(ctx, gpuID, availableState)
		}
	}
}
