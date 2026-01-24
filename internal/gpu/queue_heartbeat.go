package gpu

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
)

// QueueHeartbeatManager manages heartbeats for queue entries
type QueueHeartbeatManager struct {
	client  *redis_client.Client
	queueID string
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewQueueHeartbeatManager creates a new queue heartbeat manager
func NewQueueHeartbeatManager(client *redis_client.Client, queueID string) *QueueHeartbeatManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &QueueHeartbeatManager{
		client:  client,
		queueID: queueID,
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
}

// Start begins sending heartbeats for the queue entry
func (qhm *QueueHeartbeatManager) Start() error {
	// Send initial heartbeat synchronously
	if err := qhm.sendHeartbeat(); err != nil {
		return fmt.Errorf("failed to send initial queue heartbeat: %w", err)
	}

	// Start background heartbeat loop
	go qhm.heartbeatLoop()
	return nil
}

// Stop stops the heartbeat manager
func (qhm *QueueHeartbeatManager) Stop() {
	qhm.cancel()
	<-qhm.done
}

// Wait blocks until the heartbeat manager is stopped
func (qhm *QueueHeartbeatManager) Wait() {
	<-qhm.done
}

// heartbeatLoop sends periodic heartbeats
func (qhm *QueueHeartbeatManager) heartbeatLoop() {
	defer close(qhm.done)

	ticker := time.NewTicker(types.QueueHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-qhm.ctx.Done():
			return
		case <-ticker.C:
			if err := qhm.sendHeartbeat(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to send queue heartbeat: %v\n", err)
			}
		}
	}
}

// sendHeartbeat updates the heartbeat timestamp for the queue entry
func (qhm *QueueHeartbeatManager) sendHeartbeat() error {
	return qhm.client.UpdateQueueEntryHeartbeat(qhm.ctx, qhm.queueID)
}
