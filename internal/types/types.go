package types

import (
	"fmt"
	"strconv"
	"time"
)

// GPUState represents the state of a GPU in Redis
type GPUState struct {
	User          string       `json:"user,omitempty"`
	StartTime     FlexibleTime `json:"start_time,omitempty"`
	LastHeartbeat FlexibleTime `json:"last_heartbeat,omitempty"`
	Type          string       `json:"type,omitempty"` // "run" or "manual"
	ExpiryTime    FlexibleTime `json:"expiry_time,omitempty"`
	LastReleased  FlexibleTime `json:"last_released,omitempty"`
}

// FlexibleTime handles both Unix timestamps and RFC3339 time strings
type FlexibleTime struct {
	time.Time
}

func (ft *FlexibleTime) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	// Remove quotes if present
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}

	// Try parsing as Unix timestamp first (Python compatibility)
	if timestamp, err := strconv.ParseFloat(s, 64); err == nil {
		ft.Time = time.Unix(int64(timestamp), 0)
		return nil
	}

	// Try parsing as RFC3339 string
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		ft.Time = t
		return nil
	}

	// Try parsing as Unix timestamp string
	if timestamp, err := strconv.ParseInt(s, 10, 64); err == nil {
		ft.Time = time.Unix(timestamp, 0)
		return nil
	}

	return fmt.Errorf("cannot parse time: %s", s)
}

func (ft FlexibleTime) MarshalJSON() ([]byte, error) {
	if ft.IsZero() {
		return []byte("null"), nil
	}
	return ft.Time.MarshalJSON()
}

// ToTime converts FlexibleTime to time.Time
func (ft FlexibleTime) ToTime() time.Time {
	return ft.Time
}

// GPUUsage represents actual GPU usage detected via nvidia-smi
type GPUUsage struct {
	GPUID     int              `json:"gpu_id"`
	MemoryMB  int              `json:"memory_mb"`
	Processes []GPUProcessInfo `json:"processes"`
	Users     map[string]bool  `json:"users"`
}

// GPUProcessInfo represents a process using a GPU
type GPUProcessInfo struct {
	PID         int    `json:"pid"`
	ProcessName string `json:"process_name"`
	User        string `json:"user"`
	MemoryMB    int    `json:"memory_mb"`
}

// AllocationRequest represents a request to allocate GPUs
type AllocationRequest struct {
	GPUCount        int   // Number of GPUs to allocate (ignored if GPUIDs is specified)
	GPUIDs          []int // Specific GPU IDs to allocate (mutually exclusive with GPUCount)
	User            string
	ReservationType string
	ExpiryTime      *time.Time
}

// Validate checks if the allocation request is valid
func (ar *AllocationRequest) Validate() error {
	// Check that either GPUCount or GPUIDs is specified, but not both
	hasGPUCount := ar.GPUCount > 0
	hasGPUIDs := len(ar.GPUIDs) > 0

	if !hasGPUCount && !hasGPUIDs {
		return fmt.Errorf("either gpu count or specific gpu ids must be specified")
	}

	if hasGPUCount && hasGPUIDs {
		// Allow if GPUCount matches the number of GPU IDs
		if ar.GPUCount == len(ar.GPUIDs) {
			// This is fine - user specified matching count and IDs
		} else if ar.GPUCount == 1 {
			// Allow if GPUCount is 1 (likely the default) regardless of GPU ID count
		} else {
			return fmt.Errorf("conflicting gpu count (%d) and gpu ids (count: %d)", ar.GPUCount, len(ar.GPUIDs))
		}
	}

	if hasGPUCount && ar.GPUCount <= 0 {
		return fmt.Errorf("gpu count must be positive, got %d", ar.GPUCount)
	}

	if hasGPUIDs {
		// Check for duplicate GPU IDs
		seen := make(map[int]bool)
		for _, id := range ar.GPUIDs {
			if id < 0 {
				return fmt.Errorf("gpu id must be non-negative, got %d", id)
			}
			if seen[id] {
				return fmt.Errorf("duplicate gpu id: %d", id)
			}
			seen[id] = true
		}
	}

	if ar.User == "" {
		return fmt.Errorf("user cannot be empty")
	}

	if ar.ReservationType != ReservationTypeRun && ar.ReservationType != ReservationTypeManual {
		return fmt.Errorf("invalid reservation type: %s", ar.ReservationType)
	}

	return nil
}

// AllocationResult represents the result of a GPU allocation
type AllocationResult struct {
	AllocatedGPUs []int
	Error         error
}

// UsageRecord represents a historical GPU usage record
type UsageRecord struct {
	User            string       `json:"user"`
	GPUID           int          `json:"gpu_id"`
	StartTime       FlexibleTime `json:"start_time"`
	EndTime         FlexibleTime `json:"end_time"`
	Duration        float64      `json:"duration_seconds"`
	ReservationType string       `json:"reservation_type"`
}

// Config represents the application configuration
type Config struct {
	RedisHost       string
	RedisPort       int
	RedisDB         int
	MemoryThreshold int
}

// Constants
const (
	ReservationTypeRun    = "run"
	ReservationTypeManual = "manual"

	RedisKeyPrefix         = "canhazgpu:"
	RedisKeyGPUCount       = RedisKeyPrefix + "gpu_count"
	RedisKeyProvider       = RedisKeyPrefix + "provider"
	RedisKeyAllocationLock = RedisKeyPrefix + "allocation_lock"
	RedisKeyUsageHistory   = RedisKeyPrefix + "usage_history:"

	HeartbeatInterval = 60 * time.Second
	HeartbeatTimeout  = 15 * time.Minute
	LockTimeout       = 10 * time.Second
	MaxLockRetries    = 5

	MemoryThresholdMB = 1024
)
