package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

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
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Printf("Warning: failed to close Redis client: %v\n", err)
		}
	}()

	// Test Redis connection
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Redis: %v", err)
	}

	// Create allocation engine and get status
	engine := gpu.NewAllocationEngine(client, config)

	// Clean up expired reservations first
	if err := engine.CleanupExpiredReservations(ctx); err != nil {
		fmt.Printf("Warning: Failed to cleanup expired reservations: %v\n", err)
	}

	statuses, err := engine.GetGPUStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GPU status: %v", err)
	}

	// Display status in table format
	displayGPUStatusTable(statuses)

	return nil
}

func displayGPUStatusTable(statuses []gpu.GPUStatusInfo) {
	// Create a new tabwriter for aligned columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Check if any GPU has model information
	hasModels := false
	for _, status := range statuses {
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			hasModels = true
			break
		}
	}

	// Print header - exclude MODEL column if no models detected
	if hasModels {
		fmt.Fprintln(w, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION\tMODEL")
		fmt.Fprintln(w, "---\t------\t----\t--------\t----\t-------\t----------\t-----")
	} else {
		fmt.Fprintln(w, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION")
		fmt.Fprintln(w, "---\t------\t----\t--------\t----\t-------\t----------")
	}

	// Print each GPU status
	for _, status := range statuses {
		displaySingleGPUStatus(w, status, hasModels)
	}
}

func displaySingleGPUStatus(w *tabwriter.Writer, status gpu.GPUStatusInfo, includeModel bool) {
	gpu := fmt.Sprintf("%d", status.GPUID)

	switch status.Status {
	case "AVAILABLE":
		var details string
		if status.LastReleased.IsZero() {
			details = "never used"
		} else {
			details = fmt.Sprintf("free for %s", utils.FormatDuration(time.Since(status.LastReleased)))
		}

		// Clean validation info
		validation := strings.TrimSpace(strings.Trim(status.ValidationInfo, "[]"))
		validation = strings.TrimPrefix(validation, "validated: ")

		// Set model info
		model := "-"
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			model = status.ModelInfo.Model
		}

		if includeModel {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "AVAILABLE", "-", "-", "-", details, validation, model)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "AVAILABLE", "-", "-", "-", details, validation)
		}

	case "IN_USE":
		user := status.User
		duration := utils.FormatDuration(status.Duration)
		reservationType := strings.ToUpper(status.ReservationType)

		var details string
		switch status.ReservationType {
		case "run":
			if !status.LastHeartbeat.IsZero() {
				details = fmt.Sprintf("heartbeat %s", utils.FormatTimeAgo(status.LastHeartbeat))
			} else {
				details = "active"
			}
		case "manual":
			if !status.ExpiryTime.IsZero() {
				details = fmt.Sprintf("expires %s", utils.FormatTimeUntil(status.ExpiryTime))
			} else {
				details = "manual reservation"
			}
		}

		// Clean validation info
		validation := strings.TrimSpace(strings.Trim(status.ValidationInfo, "[]"))
		validation = strings.TrimPrefix(validation, "validated: ")

		// Set model info
		model := "-"
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			model = status.ModelInfo.Model
		}

		if includeModel {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "IN_USE", user, duration, reservationType, details, validation, model)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "IN_USE", user, duration, reservationType, details, validation)
		}

	case "UNRESERVED":
		userList := utils.FormatUserList(status.UnreservedUsers, 2)
		details := status.ProcessInfo

		// Set model info
		model := "-"
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			model = status.ModelInfo.Model
		}

		if includeModel {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNRESERVED", userList, "-", "-", details, "-", model)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNRESERVED", userList, "-", "-", details, "-")
		}

	case "ERROR":
		if includeModel {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "ERROR", "-", "-", "-", status.Error, "-", "-")
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "ERROR", "-", "-", "-", status.Error, "-")
		}

	default:
		if includeModel {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNKNOWN", "-", "-", "-", "unknown status", "-", "-")
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNKNOWN", "-", "-", "-", "unknown status", "-")
		}
	}
}
