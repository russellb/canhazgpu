package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "Empty string defaults to 8 hours",
			input:    "",
			expected: 8 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "Minutes format",
			input:    "30m",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "Hours format",
			input:    "2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "Days format",
			input:    "1d",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "Decimal hours",
			input:    "0.5h",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "Decimal days",
			input:    "0.5d",
			expected: 12 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "Large values",
			input:    "240m",
			expected: 4 * time.Hour,
			wantErr:  false,
		},
		{
			name:    "Invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "Invalid number",
			input:   "abcm",
			wantErr: true,
		},
		{
			name:    "Invalid decimal",
			input:   "abc.5h",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{
			name:     "Zero duration",
			input:    0,
			expected: "0h 0m 0s",
		},
		{
			name:     "Seconds only",
			input:    30 * time.Second,
			expected: "0h 0m 30s",
		},
		{
			name:     "Minutes and seconds",
			input:    5*time.Minute + 30*time.Second,
			expected: "0h 5m 30s",
		},
		{
			name:     "Hours, minutes, and seconds",
			input:    2*time.Hour + 30*time.Minute + 45*time.Second,
			expected: "2h 30m 45s",
		},
		{
			name:     "Large duration",
			input:    25*time.Hour + 90*time.Minute + 120*time.Second,
			expected: "26h 32m 0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		contains string
	}{
		{
			name:     "Zero time",
			input:    time.Time{},
			contains: "never",
		},
		{
			name:     "Future time",
			input:    now.Add(time.Hour),
			contains: "in the future",
		},
		{
			name:     "Past time",
			input:    now.Add(-2 * time.Hour),
			contains: "ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTimeAgo(tt.input)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestFormatTimeUntil(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		contains string
	}{
		{
			name:     "Zero time",
			input:    time.Time{},
			contains: "never",
		},
		{
			name:     "Past time",
			input:    now.Add(-time.Hour),
			contains: "expired",
		},
		{
			name:     "Future time",
			input:    now.Add(2 * time.Hour),
			contains: "in ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTimeUntil(tt.input)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "String shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "String equal to max",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "String longer than max",
			input:    "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "Very short max length",
			input:    "hello",
			maxLen:   3,
			expected: "hel",
		},
		{
			name:     "Max length of 1",
			input:    "hello",
			maxLen:   1,
			expected: "h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), tt.maxLen)
		})
	}
}

func TestFormatUserList(t *testing.T) {
	tests := []struct {
		name     string
		users    []string
		maxUsers int
		expected string
	}{
		{
			name:     "Empty list",
			users:    []string{},
			maxUsers: 3,
			expected: "unknown",
		},
		{
			name:     "Single user",
			users:    []string{"alice"},
			maxUsers: 3,
			expected: "user alice",
		},
		{
			name:     "Two users",
			users:    []string{"alice", "bob"},
			maxUsers: 3,
			expected: "users alice and bob",
		},
		{
			name:     "Three users",
			users:    []string{"alice", "bob", "charlie"},
			maxUsers: 3,
			expected: "users alice, bob and charlie",
		},
		{
			name:     "More users than max",
			users:    []string{"alice", "bob", "charlie", "david", "eve"},
			maxUsers: 3,
			expected: "users alice, bob, charlie and 2 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUserList(tt.users, tt.maxUsers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatProcessList(t *testing.T) {
	tests := []struct {
		name         string
		processes    []string
		maxProcesses int
		expected     string
	}{
		{
			name:         "Empty list",
			processes:    []string{},
			maxProcesses: 3,
			expected:     "",
		},
		{
			name:         "Few processes",
			processes:    []string{"python", "torch"},
			maxProcesses: 3,
			expected:     "python, torch",
		},
		{
			name:         "Many processes",
			processes:    []string{"python", "torch", "cuda", "tensorrt", "opencv"},
			maxProcesses: 3,
			expected:     "python, torch, cuda and 2 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatProcessList(tt.processes, tt.maxProcesses)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUsernameFromUID(t *testing.T) {
	tests := []struct {
		name      string
		uid       int
		wantError bool
	}{
		{
			name:      "Current user (UID 0 - root should exist)",
			uid:       0,
			wantError: false,
		},
		{
			name:      "Invalid negative UID",
			uid:       -1,
			wantError: true,
		},
		{
			name:      "Very large UID (likely non-existent)",
			uid:       999999,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, err := GetUsernameFromUID(tt.uid)

			if tt.wantError {
				assert.Error(t, err)
				assert.Empty(t, username)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, username)
			}
		})
	}
}
