package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexibleTime_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "Unix timestamp as integer",
			input:    `1640995200`,
			expected: time.Unix(1640995200, 0),
			wantErr:  false,
		},
		{
			name:     "Unix timestamp as float",
			input:    `1640995200.123`,
			expected: time.Unix(1640995200, 0),
			wantErr:  false,
		},
		{
			name:     "RFC3339 string",
			input:    `"2022-01-01T00:00:00Z"`,
			expected: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "RFC3339 string with timezone",
			input:    `"2022-01-01T12:30:45-05:00"`,
			expected: time.Date(2022, 1, 1, 17, 30, 45, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:    "Invalid JSON",
			input:   `invalid`,
			wantErr: true,
		},
		{
			name:    "Invalid time format",
			input:   `"not-a-time"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexibleTime
			err := json.Unmarshal([]byte(tt.input), &ft)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, tt.expected.Equal(ft.Time),
				"Expected %v, got %v", tt.expected, ft.Time)
		})
	}
}

func TestFlexibleTime_MarshalJSON(t *testing.T) {
	ft := FlexibleTime{Time: time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)}
	data, err := json.Marshal(ft)
	require.NoError(t, err)
	assert.Equal(t, `"2022-01-01T00:00:00Z"`, string(data))
}

func TestFlexibleTime_ToTime(t *testing.T) {
	now := time.Now()
	ft := FlexibleTime{Time: now}
	assert.Equal(t, now, ft.ToTime())
}

func TestGPUState_JSONSerialization(t *testing.T) {
	originalTime := time.Date(2022, 1, 1, 12, 0, 0, 0, time.UTC)

	state := &GPUState{
		User:          "testuser",
		StartTime:     FlexibleTime{Time: originalTime},
		LastHeartbeat: FlexibleTime{Time: originalTime.Add(time.Minute)},
		Type:          "run",
		ExpiryTime:    FlexibleTime{Time: originalTime.Add(time.Hour)},
		LastReleased:  FlexibleTime{Time: originalTime.Add(-time.Hour)},
	}

	// Marshal to JSON
	data, err := json.Marshal(state)
	require.NoError(t, err)

	// Unmarshal back
	var restored GPUState
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify all fields
	assert.Equal(t, state.User, restored.User)
	assert.Equal(t, state.Type, restored.Type)
	assert.True(t, state.StartTime.Time.Equal(restored.StartTime.Time))
	assert.True(t, state.LastHeartbeat.Time.Equal(restored.LastHeartbeat.Time))
	assert.True(t, state.LastReleased.Time.Equal(restored.LastReleased.Time))
	assert.True(t, state.ExpiryTime.Time.Equal(restored.ExpiryTime.Time))
}

func TestAllocationRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request *AllocationRequest
		valid   bool
	}{
		{
			name: "Valid run-type request",
			request: &AllocationRequest{
				GPUCount:        2,
				User:            "testuser",
				ReservationType: "run",
			},
			valid: true,
		},
		{
			name: "Valid manual-type request",
			request: &AllocationRequest{
				GPUCount:        1,
				User:            "testuser",
				ReservationType: "manual",
				ExpiryTime:      &time.Time{},
			},
			valid: true,
		},
		{
			name: "Invalid GPU count",
			request: &AllocationRequest{
				GPUCount:        0,
				User:            "testuser",
				ReservationType: "run",
			},
			valid: false,
		},
		{
			name: "Empty user",
			request: &AllocationRequest{
				GPUCount:        1,
				User:            "",
				ReservationType: "run",
			},
			valid: false,
		},
		{
			name: "Invalid reservation type",
			request: &AllocationRequest{
				GPUCount:        1,
				User:            "testuser",
				ReservationType: "invalid",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	config := &Config{}

	// Should have sensible defaults when not set
	assert.Equal(t, "", config.RedisHost) // Will be set by viper
	assert.Equal(t, 0, config.RedisPort)  // Will be set by viper
	assert.Equal(t, 0, config.RedisDB)    // Will be set by viper
}

func TestConstants(t *testing.T) {
	// Verify important constants are set correctly
	assert.Equal(t, "canhazgpu:", RedisKeyPrefix)
	assert.Equal(t, "canhazgpu:gpu_count", RedisKeyGPUCount)
	assert.Equal(t, "canhazgpu:allocation_lock", RedisKeyAllocationLock)

	// Verify timing constants are reasonable
	assert.Equal(t, 60*time.Second, HeartbeatInterval)
	assert.Equal(t, 15*time.Minute, HeartbeatTimeout)
	assert.Equal(t, 10*time.Second, LockTimeout)
	assert.Equal(t, 5, MaxLockRetries)
}
