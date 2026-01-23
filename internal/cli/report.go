package cli

import (
	"encoding/json"
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
	reportDays       int
	reportJSONOutput bool
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate GPU reservation reports",
	Long:  `Generate reports on GPU reservations over time, showing reservation data by user and aggregate totals.`,
	RunE:  runReport,
}

func init() {
	reportCmd.Flags().IntVarP(&reportDays, "days", "d", 30, "Number of days to include in the report")
	reportCmd.Flags().BoolVarP(&reportJSONOutput, "json", "j", false, "Output report as JSON")
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
	if reportJSONOutput {
		displayReportJSON(allRecords, startTime, endTime)
	} else {
		displayReport(allRecords, startTime, endTime)
	}

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

// ReportJSON is the JSON output structure for the report command
type ReportJSON struct {
	Users             []ReportUserJSON `json:"users"`
	TotalGPUHours     float64          `json:"total_gpu_hours"`
	TotalReservations int              `json:"total_reservations"`
	UniqueUsers       int              `json:"unique_users"`
	StartDate         string           `json:"start_date"`
	EndDate           string           `json:"end_date"`
	Days              int              `json:"days"`
}

// ReportUserJSON is the JSON output structure for per-user report data
type ReportUserJSON struct {
	Name        string  `json:"name"`
	GPUHours    float64 `json:"gpu_hours"`
	Percentage  float64 `json:"percentage"`
	RunCount    int     `json:"run_count"`
	ManualCount int     `json:"manual_count"`
}

func displayReportJSON(records []*types.UsageRecord, startTime, endTime time.Time) {
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

	totalGPUHours := totalDuration / 3600.0

	// Build JSON output
	report := ReportJSON{
		TotalGPUHours:     totalGPUHours,
		TotalReservations: len(records),
		UniqueUsers:       len(users),
		StartDate:         startTime.Format("2006-01-02"),
		EndDate:           endTime.Format("2006-01-02"),
		Days:              reportDays,
	}

	for _, user := range users {
		percentage := 0.0
		if totalDuration > 0 {
			percentage = (userUsage[user] / totalDuration) * 100
		}
		report.Users = append(report.Users, ReportUserJSON{
			Name:        user,
			GPUHours:    userGPUHours[user],
			Percentage:  percentage,
			RunCount:    userRunCount[user],
			ManualCount: userManualCount[user],
		})
	}

	// Output JSON
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Printf("Error encoding JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonData))
}
