package cli

import (
	"context"
	"encoding/json"
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

var jsonOutput bool

func init() {
	statusCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output status as JSON array")
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

	// Display status in requested format
	if jsonOutput {
		return displayGPUStatusJSON(statuses)
	} else {
		displayGPUStatusTable(statuses)
	}

	return nil
}

func displayGPUStatusTable(statuses []gpu.GPUStatusInfo) {
	// Create a new tabwriter for aligned columns
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		_ = w.Flush()
	}()

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
		_, _ = fmt.Fprintln(w, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION\tMODEL")
		_, _ = fmt.Fprintln(w, "---\t------\t----\t--------\t----\t-------\t----------\t-----")
	} else {
		_, _ = fmt.Fprintln(w, "GPU\tSTATUS\tUSER\tDURATION\tTYPE\tDETAILS\tVALIDATION")
		_, _ = fmt.Fprintln(w, "---\t------\t----\t--------\t----\t-------\t----------")
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
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "AVAILABLE", "-", "-", "-", details, validation, model)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
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
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "IN_USE", user, duration, reservationType, details, validation, model)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
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
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNRESERVED", userList, "-", "-", details, "-", model)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNRESERVED", userList, "-", "-", details, "-")
		}

	case "ERROR":
		if includeModel {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "ERROR", "-", "-", "-", status.Error, "-", "-")
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "ERROR", "-", "-", "-", status.Error, "-")
		}

	default:
		if includeModel {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNKNOWN", "-", "-", "-", "unknown status", "-", "-")
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				gpu, "UNKNOWN", "-", "-", "-", "unknown status", "-")
		}
	}
}

// JSONGPUStatus represents a GPU status for JSON output
type JSONGPUStatus struct {
	GPUID           int            `json:"gpu_id"`
	Status          string         `json:"status"`
	User            string         `json:"user,omitempty"`
	Duration        string         `json:"duration,omitempty"`
	ReservationType string         `json:"type,omitempty"`
	Details         string         `json:"details,omitempty"`
	ValidationInfo  string         `json:"validation,omitempty"`
	ModelInfo       *JSONModelInfo `json:"model,omitempty"`
	LastReleased    *time.Time     `json:"last_released,omitempty"`
	LastHeartbeat   *time.Time     `json:"last_heartbeat,omitempty"`
	ExpiryTime      *time.Time     `json:"expiry_time,omitempty"`
	UnreservedUsers []string       `json:"unreserved_users,omitempty"`
	ProcessInfo     string         `json:"process_info,omitempty"`
	Error           string         `json:"error,omitempty"`
}

// JSONModelInfo represents model information for JSON output
type JSONModelInfo struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
}

func displayGPUStatusJSON(statuses []gpu.GPUStatusInfo) error {
	jsonStatuses := make([]JSONGPUStatus, len(statuses))

	for i, status := range statuses {
		jsonStatus := JSONGPUStatus{
			GPUID:  status.GPUID,
			Status: status.Status,
		}

		// Add optional fields based on status
		if status.User != "" {
			jsonStatus.User = status.User
		}

		if status.Duration > 0 {
			jsonStatus.Duration = utils.FormatDuration(status.Duration)
		}

		if status.ReservationType != "" {
			jsonStatus.ReservationType = strings.ToUpper(status.ReservationType)
		}

		// Add details based on status type
		switch status.Status {
		case "AVAILABLE":
			if status.LastReleased.IsZero() {
				jsonStatus.Details = "never used"
			} else {
				jsonStatus.Details = fmt.Sprintf("free for %s", utils.FormatDuration(time.Since(status.LastReleased)))
				jsonStatus.LastReleased = &status.LastReleased
			}

		case "IN_USE":
			switch status.ReservationType {
			case "run":
				if !status.LastHeartbeat.IsZero() {
					jsonStatus.Details = fmt.Sprintf("heartbeat %s", utils.FormatTimeAgo(status.LastHeartbeat))
					jsonStatus.LastHeartbeat = &status.LastHeartbeat
				} else {
					jsonStatus.Details = "active"
				}
			case "manual":
				if !status.ExpiryTime.IsZero() {
					jsonStatus.Details = fmt.Sprintf("expires %s", utils.FormatTimeUntil(status.ExpiryTime))
					jsonStatus.ExpiryTime = &status.ExpiryTime
				} else {
					jsonStatus.Details = "manual reservation"
				}
			}

		case "UNRESERVED":
			jsonStatus.Details = "WITHOUT RESERVATION"
			if len(status.UnreservedUsers) > 0 {
				jsonStatus.UnreservedUsers = status.UnreservedUsers
			}
			if status.ProcessInfo != "" {
				jsonStatus.ProcessInfo = status.ProcessInfo
			}

		case "ERROR":
			if status.Error != "" {
				jsonStatus.Error = status.Error
				jsonStatus.Details = status.Error
			}

		default:
			jsonStatus.Details = "unknown status"
		}

		// Clean and add validation info
		if status.ValidationInfo != "" {
			validation := strings.TrimSpace(strings.Trim(status.ValidationInfo, "[]"))
			validation = strings.TrimPrefix(validation, "validated: ")
			jsonStatus.ValidationInfo = validation
		}

		// Add model info if present
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			jsonStatus.ModelInfo = &JSONModelInfo{
				Provider: status.ModelInfo.Provider,
				Model:    status.ModelInfo.Model,
			}
		}

		jsonStatuses[i] = jsonStatus
	}

	// Output as pretty-printed JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonStatuses)
}
