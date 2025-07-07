package gpu

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

// AllocateGPUs allocates GPUs using LRU strategy with race condition protection
func (ae *AllocationEngine) AllocateGPUs(ctx context.Context, request *types.AllocationRequest) ([]int, error) {
	// Validate the allocation request first
	if err := request.Validate(); err != nil {
		return nil, err
	}

	// Validate GPU availability using nvidia-smi
	usage, err := DetectGPUUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate GPU usage: %v", err)
	}

	// Get list of unreserved GPUs
	unreservedGPUs := GetUnreservedGPUs(ctx, usage, ae.config.MemoryThreshold)

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

// GetGPUStatus returns the current status of all GPUs with validation
func (ae *AllocationEngine) GetGPUStatus(ctx context.Context) ([]GPUStatusInfo, error) {
	gpuCount, err := ae.client.GetGPUCount(ctx)
	if err != nil {
		return nil, err
	}

	// Get actual GPU usage
	usage, err := DetectGPUUsage(ctx)
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
	ModelInfo       *ModelInfo `json:"model_info,omitempty"` // Detected model information
}

func (ae *AllocationEngine) buildGPUStatus(gpuID int, state *types.GPUState, usage *types.GPUUsage) GPUStatusInfo {
	status := GPUStatusInfo{GPUID: gpuID}

	// Check if GPU is reserved first - if it has a valid reservation, it's not unauthorized
	if state.User != "" {
		status.Status = "IN_USE"
		status.User = state.User
		status.ReservationType = state.Type
		status.Duration = time.Since(state.StartTime.ToTime())
		status.LastHeartbeat = state.LastHeartbeat.ToTime()
		status.ExpiryTime = state.ExpiryTime.ToTime()

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

			// Build process info string
			var processes []string
			for _, proc := range usage.Processes {
				processes = append(processes, fmt.Sprintf("PID %d (%s)", proc.PID, proc.ProcessName))
			}
			status.ProcessInfo = fmt.Sprintf("%dMB used by %s", usage.MemoryMB,
				strings.Join(processes, ", "))

			if len(processes) > 3 {
				status.ProcessInfo = fmt.Sprintf("%dMB used by %s and %d more",
					usage.MemoryMB, strings.Join(processes[:3], ", "), len(processes)-3)
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
