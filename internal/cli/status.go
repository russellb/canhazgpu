package cli

import (
	"context"
	"fmt"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/utils"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current GPU allocation status",
	Long: `Show the current status of all GPUs including:
- Which GPUs are available
- Which GPUs are reserved and by whom
- GPU usage validation via nvidia-smi
- Unreserved usage detection`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(ctx context.Context) error {
	config := getConfig()
	client := redis_client.NewClient(config)
	defer client.Close()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine and get status
	engine := gpu.NewAllocationEngine(client)

	// Clean up expired reservations first
	if err := engine.CleanupExpiredReservations(ctx); err != nil {
		fmt.Printf("Warning: Failed to cleanup expired reservations: %v\n", err)
	}

	statuses, err := engine.GetGPUStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GPU status: %v", err)
	}

	// Display status for each GPU
	for _, status := range statuses {
		displayGPUStatus(status)
	}

	return nil
}

func displayGPUStatus(status gpu.GPUStatusInfo) {
	switch status.Status {
	case "AVAILABLE":
		if status.LastReleased.IsZero() {
			fmt.Printf("GPU %d: AVAILABLE (never used)", status.GPUID)
		} else {
			fmt.Printf("GPU %d: AVAILABLE (last released %s)",
				status.GPUID, utils.FormatTimeAgo(status.LastReleased))
		}
		if status.ValidationInfo != "" {
			fmt.Printf(" %s", status.ValidationInfo)
		}
		fmt.Println()

	case "IN_USE":
		fmt.Printf("GPU %d: IN USE by %s for %s",
			status.GPUID, status.User, utils.FormatDuration(status.Duration))

		if status.ReservationType == "run" {
			if !status.LastHeartbeat.IsZero() {
				fmt.Printf(" (run, last heartbeat %s)",
					utils.FormatTimeAgo(status.LastHeartbeat))
			} else {
				fmt.Printf(" (run)")
			}
		} else if status.ReservationType == "manual" {
			if !status.ExpiryTime.IsZero() {
				fmt.Printf(" (manual, expires %s)",
					utils.FormatTimeUntil(status.ExpiryTime))
			} else {
				fmt.Printf(" (manual)")
			}
		}

		if status.ValidationInfo != "" {
			fmt.Printf(" %s", status.ValidationInfo)
		}
		fmt.Println()

	case "UNRESERVED":
		userList := utils.FormatUserList(status.UnreservedUsers, 3)
		fmt.Printf("GPU %d: IN USE WITHOUT RESERVATION by %s - %s\n",
			status.GPUID, userList, status.ProcessInfo)

	case "ERROR":
		fmt.Printf("GPU %d: ERROR - %s\n", status.GPUID, status.Error)

	default:
		fmt.Printf("GPU %d: UNKNOWN STATUS\n", status.GPUID)
	}
}
