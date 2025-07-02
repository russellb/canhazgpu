package utils

import (
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"time"
)

// GetUsernameFromUID converts a UID to username
func GetUsernameFromUID(uid int) (string, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// ParseDuration parses duration strings like "30m", "2h", "1d"
func ParseDuration(duration string) (time.Duration, error) {
	if duration == "" {
		return 8 * time.Hour, nil // Default 8 hours
	}

	// Handle decimal values like "0.5h"
	if strings.Contains(duration, ".") {
		if strings.HasSuffix(duration, "h") {
			hoursStr := strings.TrimSuffix(duration, "h")
			hours, err := strconv.ParseFloat(hoursStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid duration format: %s", duration)
			}
			return time.Duration(hours * float64(time.Hour)), nil
		}
		if strings.HasSuffix(duration, "d") {
			daysStr := strings.TrimSuffix(duration, "d")
			days, err := strconv.ParseFloat(daysStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid duration format: %s", duration)
			}
			return time.Duration(days * 24 * float64(time.Hour)), nil
		}
	}

	// Handle integer values
	if strings.HasSuffix(duration, "s") {
		secondsStr := strings.TrimSuffix(duration, "s")
		seconds, err := strconv.Atoi(secondsStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", duration)
		}
		return time.Duration(seconds) * time.Second, nil
	}

	if strings.HasSuffix(duration, "m") {
		minutesStr := strings.TrimSuffix(duration, "m")
		minutes, err := strconv.Atoi(minutesStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", duration)
		}
		return time.Duration(minutes) * time.Minute, nil
	}

	if strings.HasSuffix(duration, "h") {
		hoursStr := strings.TrimSuffix(duration, "h")
		hours, err := strconv.Atoi(hoursStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", duration)
		}
		return time.Duration(hours) * time.Hour, nil
	}

	if strings.HasSuffix(duration, "d") {
		daysStr := strings.TrimSuffix(duration, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", duration)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return 0, fmt.Errorf("invalid duration format: %s (use formats like 30s, 30m, 2h, 1d)", duration)
}

// FormatDuration formats a duration into human readable format
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("0h 0m %ds", int(d.Seconds()))
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
}

// FormatTime formats a time.Time into relative format like "2h 30m 15s ago"
func FormatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	d := time.Since(t)
	if d < 0 {
		return "in the future"
	}

	return FormatDuration(d) + " ago"
}

// FormatTimeUntil formats a time.Time into relative format like "in 2h 30m 15s"
func FormatTimeUntil(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	d := time.Until(t)
	if d < 0 {
		return "expired"
	}

	return "in " + FormatDuration(d)
}

// TruncateString truncates a string to maxLen characters
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FormatUserList formats a list of users for display
func FormatUserList(users []string, maxUsers int) string {
	if len(users) == 0 {
		return "unknown"
	}

	if len(users) == 1 {
		return "user " + users[0]
	}

	if len(users) <= maxUsers {
		if len(users) == 2 {
			return "users " + users[0] + " and " + users[1]
		}
		return "users " + strings.Join(users[:len(users)-1], ", ") + " and " + users[len(users)-1]
	}

	displayed := users[:maxUsers]
	remaining := len(users) - maxUsers
	return "users " + strings.Join(displayed, ", ") + fmt.Sprintf(" and %d more", remaining)
}

// FormatProcessList formats a list of processes for display
func FormatProcessList(processes []string, maxProcesses int) string {
	if len(processes) == 0 {
		return ""
	}

	if len(processes) <= maxProcesses {
		return strings.Join(processes, ", ")
	}

	displayed := processes[:maxProcesses]
	remaining := len(processes) - maxProcesses
	return strings.Join(displayed, ", ") + fmt.Sprintf(" and %d more", remaining)
}
