package gpu

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
)

type AllocationEngine struct {
	client *redis_client.Client
	config *types.Config
}

func NewAllocationEngine(client *redis_client.Client, config *types.Config) *AllocationEngine {
	return &AllocationEngine{
		client: client,
		config: config,
	}
}

func (ae *AllocationEngine) detectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	providerName, err := ae.client.GetAvailableProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached provider information: %v", err)
	}

	var pm *ProviderManager
	if providerName == "fake" {
		// For fake provider, get GPU count from Redis to configure the provider
		gpuCount, err := ae.client.GetGPUCount(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GPU count for fake provider: %v", err)
		}
		pm = NewProviderManagerWithFake(gpuCount)
	} else {
		pm = NewProviderManagerFromNames([]string{providerName})
	}

	return pm.DetectAllGPUUsageWithoutChecks(ctx)
}

// AllocateGPUs allocates GPUs using MRU-per-user strategy with race condition protection
func (ae *AllocationEngine) AllocateGPUs(ctx context.Context, request *types.AllocationRequest) ([]int, error) {
	// Validate the allocation request first
	if err := request.Validate(); err != nil {
		return nil, err
	}

	// Validate GPU availability using cached provider information
	usage, err := ae.detectGPUUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate GPU usage: %v", err)
	}

	// Get list of unreserved GPUs
	unreservedGPUs := GetUnreservedGPUs(ctx, usage, ae.config.MemoryThreshold)

	// If force flag is set, clear unreserved GPUs list to allow allocation
	if request.Force {
		unreservedGPUs = []int{}
	}

	// Acquire allocation lock
	if err := ae.client.AcquireAllocationLock(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err := ae.client.ReleaseAllocationLock(ctx); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: failed to release allocation lock: %v\n", err)
		}
	}()

	// Perform atomic allocation
	allocatedGPUs, err := ae.client.AtomicReserveGPUs(ctx, request, unreservedGPUs)
	if err != nil {
		// Check if it's an availability error and provide detailed message
		if err.Error() == "Not enough GPUs available" {
			gpuCount, _ := ae.client.GetGPUCount(ctx)
			available := gpuCount - len(unreservedGPUs)

			var unreservedMsg string
			if len(unreservedGPUs) > 0 {
				unreservedMsg = fmt.Sprintf(" (%d GPUs in use without reservation - run 'canhazgpu status' for details)", len(unreservedGPUs))
			}

			return nil, fmt.Errorf("not enough GPUs available. Requested: %d, Available: %d%s",
				request.GPUCount, available, unreservedMsg)
		}
		// For specific GPU ID errors, pass through the detailed error message
		return nil, err
	}

	return allocatedGPUs, nil
}

// ReleaseGPUs releases manually reserved GPUs for a user
func (ae *AllocationEngine) ReleaseGPUs(ctx context.Context, user string) ([]int, error) {
	gpuCount, err := ae.client.GetGPUCount(ctx)
	if err != nil {
		return nil, err
	}

	var releasedGPUs []int
	now := time.Now()

	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		state, err := ae.client.GetGPUState(ctx, gpuID)
		if err != nil {
			continue
		}

		// Only release manual reservations by this user
		if state.User == user && state.Type == types.ReservationTypeManual {
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

			if err := ae.client.RecordUsageHistory(ctx, usageRecord); err != nil {
				// Log error but don't fail the release
				fmt.Fprintf(os.Stderr, "Warning: failed to record usage history: %v\n", err)
			}

			// Mark as available with last_released timestamp
			availableState := &types.GPUState{
				LastReleased: types.FlexibleTime{Time: now},
			}

			if err := ae.client.SetGPUState(ctx, gpuID, availableState); err != nil {
				return nil, fmt.Errorf("failed to release GPU %d: %v", gpuID, err)
			}

			releasedGPUs = append(releasedGPUs, gpuID)
		}
	}

	return releasedGPUs, nil
}

// ReleaseSpecificGPUs releases specific GPUs owned by a user (both manual and run-type reservations)
func (ae *AllocationEngine) ReleaseSpecificGPUs(ctx context.Context, user string, gpuIDs []int) ([]int, error) {
	var releasedGPUs []int
	now := time.Now()

	for _, gpuID := range gpuIDs {
		state, err := ae.client.GetGPUState(ctx, gpuID)
		if err != nil {
			continue
		}

		// Release GPU if it's reserved by this user (either manual or run type)
		if state.User == user && (state.Type == types.ReservationTypeManual || state.Type == types.ReservationTypeRun) {
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
			if err := ae.client.RecordUsageHistory(ctx, usageRecord); err != nil {
				// Log error but don't fail the release
				fmt.Fprintf(os.Stderr, "Warning: failed to record usage history: %v\n", err)
			}

			// Mark as available with last_released timestamp
			availableState := &types.GPUState{
				LastReleased: types.FlexibleTime{Time: now},
			}
			if err := ae.client.SetGPUState(ctx, gpuID, availableState); err != nil {
				return nil, fmt.Errorf("failed to release GPU %d: %v", gpuID, err)
			}
			releasedGPUs = append(releasedGPUs, gpuID)
		}
	}

	return releasedGPUs, nil
}

// GetGPUStatus returns the current status of all GPUs with validation
func (ae *AllocationEngine) GetGPUStatus(ctx context.Context) ([]GPUStatusInfo, error) {
	gpuCount, err := ae.client.GetGPUCount(ctx)
	if err != nil {
		return nil, err
	}

	// Get actual GPU usage using cached provider information
	usage, err := ae.detectGPUUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate GPU usage: %v", err)
	}

	var statuses []GPUStatusInfo

	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		state, err := ae.client.GetGPUState(ctx, gpuID)
		if err != nil {
			statuses = append(statuses, GPUStatusInfo{
				GPUID:  gpuID,
				Status: "ERROR",
				Error:  fmt.Sprintf("Failed to get state: %v", err),
			})
			continue
		}

		status := ae.buildGPUStatus(gpuID, state, usage[gpuID])
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// GPUStatusInfo represents the status of a single GPU
type GPUStatusInfo struct {
	GPUID           int
	Status          string // "AVAILABLE", "IN_USE", "UNRESERVED", "ERROR"
	User            string
	ReservationType string
	Duration        time.Duration
	LastHeartbeat   time.Time
	ExpiryTime      time.Time
	LastReleased    time.Time
	ValidationInfo  string
	UnreservedUsers []string
	ProcessInfo     string
	Error           string
	ModelInfo       *ModelInfo `json:"model_info,omitempty"` // Detected AI model information
	Provider        string     `json:"provider,omitempty"`   // GPU provider (e.g., "NVIDIA", "AMD")
	GPUModel        string     `json:"gpu_model,omitempty"`  // GPU model (e.g., "H100", "RTX 4090")
	Note            string     `json:"note,omitempty"`       // Optional note describing the reservation purpose
}

func (ae *AllocationEngine) buildGPUStatus(gpuID int, state *types.GPUState, usage *types.GPUUsage) GPUStatusInfo {
	status := GPUStatusInfo{GPUID: gpuID}

	// Check if GPU is reserved first - if it has a valid reservation, it's not unauthorized
	if state.User != "" {
		status.Status = "IN_USE"
		// If user and actual_user differ, show "user (actual_user)" format
		if state.ActualUser != "" && state.User != state.ActualUser {
			status.User = fmt.Sprintf("%s (%s)", state.ActualUser, state.User)
		} else {
			status.User = state.User
		}
		status.ReservationType = state.Type
		status.Duration = time.Since(state.StartTime.ToTime())
		status.LastHeartbeat = state.LastHeartbeat.ToTime()
		status.ExpiryTime = state.ExpiryTime.ToTime()
		status.Note = state.Note

		// Build validation info
		if usage != nil && usage.MemoryMB > ae.config.MemoryThreshold {
			if len(usage.Processes) > 0 {
				status.ValidationInfo = fmt.Sprintf("[validated: %dMB, %d processes]",
					usage.MemoryMB, len(usage.Processes))
			} else {
				status.ValidationInfo = fmt.Sprintf("[validated: %dMB used]", usage.MemoryMB)
			}
		} else {
			status.ValidationInfo = "[validated: no usage detected]"
		}
	} else {
		// GPU has no reservation - check if it's being used without reservation
		if IsGPUInUnreservedUse(usage, ae.config.MemoryThreshold) {
			status.Status = "UNRESERVED"

			// Get users from processes
			var users []string
			for user := range usage.Users {
				users = append(users, user)
			}
			status.UnreservedUsers = users

			// Show memory usage without process details
			processCount := len(usage.Processes)
			if processCount == 1 {
				status.ProcessInfo = fmt.Sprintf("%dMB used by 1 process", usage.MemoryMB)
			} else {
				status.ProcessInfo = fmt.Sprintf("%dMB used by %d processes", usage.MemoryMB, processCount)
			}
		} else {
			status.Status = "AVAILABLE"
			status.LastReleased = state.LastReleased.ToTime()

			// Show memory usage for available GPUs
			if usage != nil {
				status.ValidationInfo = fmt.Sprintf("[validated: %dMB used]", usage.MemoryMB)
			}
		}
	}

	// Detect model information from processes if available
	if usage != nil && len(usage.Processes) > 0 {
		status.ModelInfo = DetectModelFromProcesses(usage.Processes)
	}

	// Add GPU provider and model information if available
	if usage != nil {
		status.Provider = usage.Provider
		status.GPUModel = usage.Model
	}

	return status
}

// CleanupExpiredReservations removes expired manual reservations
func (ae *AllocationEngine) CleanupExpiredReservations(ctx context.Context) error {
	gpuCount, err := ae.client.GetGPUCount(ctx)
	if err != nil {
		return err
	}

	now := time.Now()

	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		state, err := ae.client.GetGPUState(ctx, gpuID)
		if err != nil {
			continue
		}

		var shouldRelease bool
		var reason string

		// Check for expired manual reservations
		if state.Type == types.ReservationTypeManual &&
			!state.ExpiryTime.ToTime().IsZero() &&
			now.After(state.ExpiryTime.ToTime()) {
			shouldRelease = true
			reason = "expired"
		}

		// Check for stale heartbeats (run-type reservations)
		if state.Type == types.ReservationTypeRun &&
			!state.LastHeartbeat.ToTime().IsZero() &&
			now.Sub(state.LastHeartbeat.ToTime()) > types.HeartbeatTimeout {
			shouldRelease = true
			reason = "stale heartbeat"
		}

		if shouldRelease && state.User != "" {
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

			if err := ae.client.RecordUsageHistory(ctx, usageRecord); err != nil {
				// Log error but don't fail the cleanup
				fmt.Fprintf(os.Stderr, "Warning: failed to record usage history for %s: %v\n", reason, err)
			}

			// Release reservation
			availableState := &types.GPUState{
				LastReleased: types.FlexibleTime{Time: now},
			}
			if err := ae.client.SetGPUState(ctx, gpuID, availableState); err != nil {
				fmt.Printf("Warning: failed to set GPU %d state to available: %v\n", gpuID, err)
			}
		}
	}

	return nil
}

// QueuedAllocationRequest extends AllocationRequest with queue-specific options
type QueuedAllocationRequest struct {
	*types.AllocationRequest
	Blocking    bool           // If true, wait in queue when GPUs unavailable
	WaitTimeout *time.Duration // Max time to wait (nil = forever)
}

// QueuedAllocationResult represents the result of a queued allocation
type QueuedAllocationResult struct {
	AllocatedGPUs []int
	QueueEntry    *types.QueueEntry // Non-nil if allocation is still pending
	Error         error
}

// AllocateGPUsWithQueue allocates GPUs, optionally waiting in a queue if unavailable
func (ae *AllocationEngine) AllocateGPUsWithQueue(ctx context.Context, request *QueuedAllocationRequest) (*QueuedAllocationResult, error) {
	// First, try immediate allocation
	allocatedGPUs, err := ae.AllocateGPUs(ctx, request.AllocationRequest)
	if err == nil {
		return &QueuedAllocationResult{AllocatedGPUs: allocatedGPUs}, nil
	}

	// If not blocking, return the error immediately
	if !request.Blocking {
		return nil, err
	}

	// Create a queue entry
	queueEntry := ae.createQueueEntry(request)

	// Add to queue
	if err := ae.client.AddToQueue(ctx, queueEntry); err != nil {
		return nil, fmt.Errorf("failed to add to queue: %v", err)
	}

	// Start queue heartbeat
	queueHeartbeat := NewQueueHeartbeatManager(ae.client, queueEntry.ID)
	if err := queueHeartbeat.Start(); err != nil {
		ae.client.RemoveFromQueue(ctx, queueEntry.ID)
		return nil, fmt.Errorf("failed to start queue heartbeat: %v", err)
	}

	// Wait for GPUs
	result, err := ae.waitForGPUs(ctx, queueEntry, request, queueHeartbeat)
	if err != nil {
		// Cleanup on error
		queueHeartbeat.Stop()
		ae.cleanupQueueEntry(ctx, queueEntry)
		return nil, err
	}

	queueHeartbeat.Stop()
	return result, nil
}

// createQueueEntry creates a new queue entry from an allocation request
func (ae *AllocationEngine) createQueueEntry(request *QueuedAllocationRequest) *types.QueueEntry {
	now := time.Now()

	entry := &types.QueueEntry{
		ID:              uuid.New().String(),
		User:            request.User,
		ActualUser:      request.ActualUser,
		RequestedCount:  request.GPUCount,
		RequestedIDs:    request.GPUIDs,
		AllocatedGPUs:   []int{},
		ReservationType: request.ReservationType,
		Note:            request.Note,
		EnqueueTime:     types.FlexibleTime{Time: now},
		LastHeartbeat:   types.FlexibleTime{Time: now},
	}

	if request.ExpiryTime != nil {
		entry.ExpiryDuration = request.ExpiryTime.Sub(now)
	}

	if request.WaitTimeout != nil {
		waitTimeout := now.Add(*request.WaitTimeout)
		entry.WaitTimeout = &types.FlexibleTime{Time: waitTimeout}
	}

	return entry
}

// waitForGPUs polls for GPU availability and performs greedy allocation
func (ae *AllocationEngine) waitForGPUs(ctx context.Context, queueEntry *types.QueueEntry, request *QueuedAllocationRequest, heartbeat *QueueHeartbeatManager) (*QueuedAllocationResult, error) {
	ticker := time.NewTicker(types.QueuePollInterval)
	defer ticker.Stop()

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	position, _ := ae.client.GetQueuePosition(ctx, queueEntry.ID)
	fmt.Printf("Waiting for GPUs... (queue position: %d)\n", position+1)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-sigChan:
			return nil, fmt.Errorf("interrupted while waiting in queue")

		case <-ticker.C:
			// Cleanup stale queue entries
			if _, err := ae.client.CleanupStaleQueueEntries(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to cleanup stale queue entries: %v\n", err)
			}

			// Cleanup expired reservations
			if err := ae.CleanupExpiredReservations(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to cleanup expired reservations: %v\n", err)
			}

			// Check wait timeout
			if queueEntry.WaitTimeout != nil && time.Now().After(queueEntry.WaitTimeout.ToTime()) {
				return nil, fmt.Errorf("wait timeout exceeded")
			}

			// Check if we're first in queue
			isFirst, err := ae.client.IsFirstInQueue(ctx, queueEntry.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to check queue position: %v\n", err)
				continue
			}

			if !isFirst {
				// Update position display
				newPosition, _ := ae.client.GetQueuePosition(ctx, queueEntry.ID)
				if newPosition != position {
					position = newPosition
					fmt.Printf("Queue position updated: %d\n", position+1)
				}
				continue
			}

			// We're first in queue - try to allocate
			result, err := ae.tryAllocateForQueueEntry(ctx, queueEntry, request)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: allocation attempt failed: %v\n", err)
				continue
			}

			if result != nil {
				// Successfully allocated all GPUs
				// Remove from queue
				if err := ae.client.RemoveFromQueue(ctx, queueEntry.ID); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove from queue: %v\n", err)
				}
				return result, nil
			}

			// Still waiting for more GPUs
			entry, _ := ae.client.GetQueueEntry(ctx, queueEntry.ID)
			if entry != nil && len(entry.AllocatedGPUs) > len(queueEntry.AllocatedGPUs) {
				queueEntry = entry
				fmt.Printf("Partial allocation: %d/%d GPUs\n", len(queueEntry.AllocatedGPUs), queueEntry.GetRequestedGPUCount())
			}
		}
	}
}

// tryAllocateForQueueEntry attempts to allocate GPUs for the first queue entry
func (ae *AllocationEngine) tryAllocateForQueueEntry(ctx context.Context, queueEntry *types.QueueEntry, request *QueuedAllocationRequest) (*QueuedAllocationResult, error) {
	// Acquire lock
	if err := ae.client.AcquireAllocationLock(ctx); err != nil {
		return nil, err
	}
	defer ae.client.ReleaseAllocationLock(ctx)

	// Re-fetch the queue entry to get latest allocated GPUs
	entry, err := ae.client.GetQueueEntry(ctx, queueEntry.ID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("queue entry no longer exists")
	}

	// Check if already complete
	if entry.IsComplete() {
		return ae.finalizeAllocation(ctx, entry, request)
	}

	// Get available GPUs
	usage, err := ae.detectGPUUsage(ctx)
	if err != nil {
		return nil, err
	}

	unreservedGPUs := GetUnreservedGPUs(ctx, usage, ae.config.MemoryThreshold)
	if request.Force {
		unreservedGPUs = []int{}
	}

	// Find available GPUs
	gpuCount, err := ae.client.GetGPUCount(ctx)
	if err != nil {
		return nil, err
	}

	var availableGPUs []int
	for gpuID := 0; gpuID < gpuCount; gpuID++ {
		// Skip already allocated to this entry
		alreadyAllocated := false
		for _, allocatedID := range entry.AllocatedGPUs {
			if gpuID == allocatedID {
				alreadyAllocated = true
				break
			}
		}
		if alreadyAllocated {
			continue
		}

		// Skip unreserved GPUs
		isUnreserved := false
		for _, unreservedID := range unreservedGPUs {
			if gpuID == unreservedID {
				isUnreserved = true
				break
			}
		}
		if isUnreserved {
			continue
		}

		// Check if GPU is available
		state, err := ae.client.GetGPUState(ctx, gpuID)
		if err != nil {
			continue
		}

		if state.User == "" {
			// If specific IDs requested, only consider those
			if len(entry.RequestedIDs) > 0 {
				isRequested := false
				for _, reqID := range entry.RequestedIDs {
					if gpuID == reqID {
						isRequested = true
						break
					}
				}
				if !isRequested {
					continue
				}
			}

			availableGPUs = append(availableGPUs, gpuID)
		}
	}

	// No new GPUs available
	if len(availableGPUs) == 0 {
		return nil, nil
	}

	// Calculate how many more we need
	needed := entry.GetRequestedGPUCount() - len(entry.AllocatedGPUs)
	if needed > len(availableGPUs) {
		needed = len(availableGPUs)
	}

	// Allocate the available GPUs (greedy partial allocation)
	now := time.Now()
	for i := 0; i < needed; i++ {
		gpuID := availableGPUs[i]

		// Create reservation state with partial queue ID
		gpuState := &types.GPUState{
			User:           entry.User,
			ActualUser:     entry.ActualUser,
			StartTime:      types.FlexibleTime{Time: now},
			Type:           entry.ReservationType,
			Note:           entry.Note,
			PartialQueueID: entry.ID,
		}

		if entry.ReservationType == types.ReservationTypeRun {
			gpuState.LastHeartbeat = types.FlexibleTime{Time: now}
		} else if entry.ReservationType == types.ReservationTypeManual && entry.ExpiryDuration > 0 {
			gpuState.ExpiryTime = types.FlexibleTime{Time: now.Add(entry.ExpiryDuration)}
		}

		if err := ae.client.SetGPUState(ctx, gpuID, gpuState); err != nil {
			return nil, fmt.Errorf("failed to reserve GPU %d: %v", gpuID, err)
		}

		entry.AllocatedGPUs = append(entry.AllocatedGPUs, gpuID)
	}

	// Update queue entry
	if err := ae.client.UpdateQueueEntry(ctx, entry); err != nil {
		return nil, err
	}

	// Check if complete
	if entry.IsComplete() {
		return ae.finalizeAllocation(ctx, entry, request)
	}

	return nil, nil // Still waiting for more GPUs
}

// finalizeAllocation converts partial allocations to final reservations
func (ae *AllocationEngine) finalizeAllocation(ctx context.Context, entry *types.QueueEntry, request *QueuedAllocationRequest) (*QueuedAllocationResult, error) {
	now := time.Now()

	// Clear the partial queue ID from all allocated GPUs
	for _, gpuID := range entry.AllocatedGPUs {
		state, err := ae.client.GetGPUState(ctx, gpuID)
		if err != nil {
			continue
		}

		// Clear the partial queue ID to mark as fully allocated
		state.PartialQueueID = ""

		// Update expiry time for manual reservations (use the time of final allocation)
		if state.Type == types.ReservationTypeManual && entry.ExpiryDuration > 0 {
			state.ExpiryTime = types.FlexibleTime{Time: now.Add(entry.ExpiryDuration)}
		}

		if err := ae.client.SetGPUState(ctx, gpuID, state); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to finalize GPU %d: %v\n", gpuID, err)
		}
	}

	return &QueuedAllocationResult{
		AllocatedGPUs: entry.AllocatedGPUs,
	}, nil
}

// cleanupQueueEntry removes a queue entry and releases any partial allocations
func (ae *AllocationEngine) cleanupQueueEntry(ctx context.Context, entry *types.QueueEntry) {
	now := time.Now()

	// Release partial allocations
	for _, gpuID := range entry.AllocatedGPUs {
		availableState := &types.GPUState{
			LastReleased: types.FlexibleTime{Time: now},
		}
		if err := ae.client.SetGPUState(ctx, gpuID, availableState); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to release partial allocation for GPU %d: %v\n", gpuID, err)
		}
	}

	// Remove from queue
	if err := ae.client.RemoveFromQueue(ctx, entry.ID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove queue entry: %v\n", err)
	}
}

// GetQueueStatus returns the current queue status for display
func (ae *AllocationEngine) GetQueueStatus(ctx context.Context) (*types.QueueStatus, error) {
	return ae.client.GetQueueStatus(ctx)
}
