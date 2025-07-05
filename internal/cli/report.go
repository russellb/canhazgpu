package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
	"github.com/spf13/cobra"
)

var (
	reportDays int
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate GPU reservation reports",
	Long:  `Generate reports on GPU reservations over time, showing reservation data by user and aggregate totals.`,
	RunE:  runReport,
}

func init() {
	reportCmd.Flags().IntVarP(&reportDays, "days", "d", 30, "Number of days to include in the report")
	rootCmd.AddCommand(reportCmd)
}

func runReport(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Initialize Redis client
	config := getConfig()
	client := redis_client.NewClient(config)
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Printf("Warning: failed to close Redis client: %v\n", err)
		}
	}()

	// Test connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Calculate time range
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -reportDays)

	// Get historical usage data
	historicalRecords, err := client.GetUsageHistory(ctx, startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to get usage history: %v", err)
	}

	// Get current GPU states for in-progress usage
	ae := gpu.NewAllocationEngine(client, config)
	currentStatuses, err := ae.GetGPUStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current GPU status: %v", err)
	}

	// Add current usage to records
	currentRecords := getCurrentUsageRecords(currentStatuses, endTime)
	allRecords := append(historicalRecords, currentRecords...)

	// Generate and display report
	displayReport(allRecords, startTime, endTime)

	return nil
}

func getCurrentUsageRecords(statuses []gpu.GPUStatusInfo, now time.Time) []*types.UsageRecord {
	var records []*types.UsageRecord

	for _, status := range statuses {
		if status.Status == "IN_USE" && status.User != "" {
			// Calculate duration from start time to now
			duration := now.Sub(status.LastHeartbeat).Seconds()
			if status.ReservationType == types.ReservationTypeManual && !status.ExpiryTime.IsZero() {
				// For manual reservations, use the actual elapsed time
				duration = status.Duration.Seconds()
			}

			record := &types.UsageRecord{
				User:            status.User,
				GPUID:           status.GPUID,
				StartTime:       types.FlexibleTime{Time: status.LastHeartbeat.Add(-status.Duration)},
				EndTime:         types.FlexibleTime{Time: now},
				Duration:        duration,
				ReservationType: status.ReservationType,
			}
			records = append(records, record)
		}
	}

	return records
}

func displayReport(records []*types.UsageRecord, startTime, endTime time.Time) {
	// Aggregate usage by user
	userUsage := make(map[string]float64)
	userGPUHours := make(map[string]float64)
	userRunCount := make(map[string]int)
	userManualCount := make(map[string]int)

	var totalDuration float64

	for _, record := range records {
		userUsage[record.User] += record.Duration
		userGPUHours[record.User] += record.Duration / 3600.0
		totalDuration += record.Duration

		if record.ReservationType == types.ReservationTypeRun {
			userRunCount[record.User]++
		} else {
			userManualCount[record.User]++
		}
	}

	// Sort users by usage
	var users []string
	for user := range userUsage {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool {
		return userUsage[users[i]] > userUsage[users[j]]
	})

	// Display report header
	fmt.Printf("\n=== GPU Reservation Report ===\n")
	fmt.Printf("Period: %s to %s (%d days)\n",
		startTime.Format("2006-01-02"),
		endTime.Format("2006-01-02"),
		reportDays)
	fmt.Printf("\n")

	// Display per-user statistics
	fmt.Printf("%-20s %15s %15s %10s %10s\n",
		"User", "GPU Hours", "Percentage", "Run", "Manual")
	fmt.Printf("%s\n", strings.Repeat("-", 75))

	totalGPUHours := totalDuration / 3600.0
	for _, user := range users {
		percentage := (userUsage[user] / totalDuration) * 100
		fmt.Printf("%-20s %15.2f %14.1f%% %10d %10d\n",
			user,
			userGPUHours[user],
			percentage,
			userRunCount[user],
			userManualCount[user])
	}

	// Display summary
	fmt.Printf("%s\n", strings.Repeat("-", 75))
	fmt.Printf("%-20s %15.2f %14s %10d %10d\n",
		"TOTAL",
		totalGPUHours,
		"100.0%",
		len(records),
		0)

	fmt.Printf("\nTotal reservations: %d\n", len(records))
	fmt.Printf("Unique users: %d\n", len(users))
	fmt.Printf("\n")
}
