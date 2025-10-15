package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/russellb/canhazgpu/internal/gpu"
	"github.com/russellb/canhazgpu/internal/redis_client"
	"github.com/russellb/canhazgpu/internal/types"
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
- Unreserved usage detection

Remote host status:
- Use --remote/-r <address> to check status on a specific remote host
- Use --all to check status on all configured remote hosts
- Configure remote hosts in ~/.canhazgpu.yaml under remote_hosts

Summary mode:
- Use --summary or -s to show a condensed summary
- Works with local, --remote, or --all modes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd.Context())
	},
}

var (
	jsonOutput  bool
	showAll     bool
	remoteName  string
	showSummary bool
)

func init() {
	statusCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output status as JSON array")
	statusCmd.Flags().BoolVar(&showAll, "all", false, "Show status for all configured remote hosts")
	statusCmd.Flags().StringVarP(&remoteName, "remote", "r", "", "Show status for a specific remote host")
	statusCmd.Flags().BoolVarP(&showSummary, "summary", "s", false, "Show summary with GPU counts and availability")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(ctx context.Context) error {
	config := getConfig()

	// Validate flags
	if showAll && remoteName != "" {
		return fmt.Errorf("cannot use --all and --remote together")
	}

	// Determine execution mode
	if showAll {
		return runStatusAllHosts(ctx, config)
	} else if remoteName != "" {
		return runStatusRemoteHost(ctx, remoteName)
	} else {
		return runStatusLocal(ctx, config)
	}
}

func runStatusLocal(ctx context.Context, config *types.Config) error {
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
	if showSummary {
		displaySummary("localhost", statuses)
	} else if jsonOutput {
		return displayGPUStatusJSON(statuses)
	} else {
		displayGPUStatusTable(statuses)
	}

	return nil
}

func runStatusRemoteHost(ctx context.Context, host string) error {
	statuses, err := getRemoteStatus(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to get status from %s: %v", host, err)
	}

	if showSummary {
		displaySummary(host, statuses)
	} else if jsonOutput {
		return displayGPUStatusJSON(statuses)
	} else {
		fmt.Printf("Status for %s:\n", host)
		displayGPUStatusTable(statuses)
	}

	return nil
}

func runStatusAllHosts(ctx context.Context, config *types.Config) error {
	if len(config.RemoteHosts) == 0 {
		return fmt.Errorf("no remote hosts configured in ~/.canhazgpu.yaml")
	}

	// JSON mode needs to collect all results first
	if jsonOutput {
		return runStatusAllHostsJSON(ctx, config)
	}

	// For table and summary modes, display progressively
	// Get and display local status first
	localStatuses, localErr := getLocalStatus(ctx, config)

	if showSummary {
		// Print header for summary table
		fmt.Printf("%-20s %5s  %-30s %9s  %6s\n",
			"HOST", "TOTAL", "GPU_MODELS", "AVAILABLE", "IN_USE")
		fmt.Printf("%-20s %5s  %-30s %9s  %6s\n",
			"--------------------", "-----", "------------------------------", "---------", "------")

		if localErr != nil {
			fmt.Printf("%-20s ERROR: %v\n", "localhost", localErr)
		} else {
			displaySummary("localhost", localStatuses)
		}
	} else {
		fmt.Printf("=== %s ===\n", "localhost")
		if localErr != nil {
			fmt.Printf("ERROR: %v\n", localErr)
		} else {
			displayGPUStatusTable(localStatuses)
		}
	}

	// Get and display each remote host progressively
	for _, host := range config.RemoteHosts {
		statuses, err := getRemoteStatus(ctx, host)

		if showSummary {
			if err != nil {
				fmt.Printf("%s: ERROR - %v\n", host, err)
			} else {
				displaySummary(host, statuses)
			}
		} else {
			fmt.Println() // Blank line between hosts
			fmt.Printf("=== %s ===\n", host)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
			} else {
				displayGPUStatusTable(statuses)
			}
		}
	}

	return nil
}

// runStatusAllHostsJSON collects all results then outputs JSON
func runStatusAllHostsJSON(ctx context.Context, config *types.Config) error {
	// Get local status first
	localStatuses, localErr := getLocalStatus(ctx, config)

	// Collect results from all hosts
	type hostResult struct {
		host     string
		statuses []gpu.GPUStatusInfo
		err      error
	}

	results := make([]hostResult, 0, len(config.RemoteHosts)+1)

	// Add localhost result
	results = append(results, hostResult{
		host:     "localhost",
		statuses: localStatuses,
		err:      localErr,
	})

	// Get status from each remote host
	for _, host := range config.RemoteHosts {
		statuses, err := getRemoteStatus(ctx, host)
		results = append(results, hostResult{
			host:     host,
			statuses: statuses,
			err:      err,
		})
	}

	// Output all statuses as JSON
	allStatuses := make(map[string]interface{})
	for _, result := range results {
		if result.err != nil {
			allStatuses[result.host] = map[string]string{"error": result.err.Error()}
		} else {
			allStatuses[result.host] = result.statuses
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(allStatuses)
}

func getLocalStatus(ctx context.Context, config *types.Config) ([]gpu.GPUStatusInfo, error) {
	client := redis_client.NewClient(config)
	defer client.Close()

	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	engine := gpu.NewAllocationEngine(client, config)

	// Cleanup expired reservations
	_ = engine.CleanupExpiredReservations(ctx)

	return engine.GetGPUStatus(ctx)
}

func getRemoteStatus(ctx context.Context, host string) ([]gpu.GPUStatusInfo, error) {
	// Execute remote status command with JSON output
	stdout, stderr, err := utils.ExecuteRemoteCanHazGPU(ctx, host, []string{"status", "--json"})
	if err != nil {
		if stderr != "" {
			return nil, fmt.Errorf("%v: %s", err, stderr)
		}
		return nil, err
	}

	// Parse JSON output
	var statuses []JSONGPUStatus
	if err := json.Unmarshal([]byte(stdout), &statuses); err != nil {
		return nil, fmt.Errorf("failed to parse JSON output: %v", err)
	}

	// Convert to GPUStatusInfo
	result := make([]gpu.GPUStatusInfo, len(statuses))
	for i, s := range statuses {
		result[i] = convertJSONToStatusInfo(s)
	}

	// Check if any GPU is missing model info (older canhazgpu version)
	needsGPUModel := false
	for _, s := range result {
		if s.GPUModel == "" {
			needsGPUModel = true
			break
		}
	}

	// If GPU model is missing, try to get it directly via nvidia-smi or amd-smi
	if needsGPUModel {
		gpuModel := getRemoteGPUModel(ctx, host)
		if gpuModel != "" {
			// Apply the model to all GPUs (assumes homogeneous system)
			for i := range result {
				if result[i].GPUModel == "" {
					result[i].GPUModel = gpuModel
				}
			}
		}
	}

	return result, nil
}

// getRemoteGPUModel tries to detect GPU model directly via nvidia-smi or amd-smi
func getRemoteGPUModel(ctx context.Context, host string) string {
	// Try nvidia-smi first
	stdout, _, err := utils.ExecuteRemoteCommand(ctx, host,
		"nvidia-smi --query-gpu=gpu_name --format=csv,noheader,nounits | head -1")
	if err == nil && stdout != "" {
		return strings.TrimSpace(stdout)
	}

	// Try amd-smi
	stdout, _, err = utils.ExecuteRemoteCommand(ctx, host,
		"amd-smi list --json 2>/dev/null | jq -r '.[0].name' 2>/dev/null")
	if err == nil && stdout != "" && stdout != "null" {
		return strings.TrimSpace(stdout)
	}

	return ""
}

func convertJSONToStatusInfo(j JSONGPUStatus) gpu.GPUStatusInfo {
	status := gpu.GPUStatusInfo{
		GPUID:  j.GPUID,
		Status: j.Status,
		User:   j.User,
	}

	// Parse duration if present
	if j.Duration != "" {
		// Duration is formatted as "Xh Ym Zs", we'll store it as-is for display
		// but we need a time.Duration for the struct
		// For remote status, we don't have the exact duration, so we'll estimate
		status.Duration = 0 // Will use details field instead
	}

	status.ReservationType = strings.ToLower(j.ReservationType)
	status.ValidationInfo = j.ValidationInfo
	status.ProcessInfo = j.ProcessInfo
	status.UnreservedUsers = j.UnreservedUsers
	status.Error = j.Error

	if j.LastReleased != nil {
		status.LastReleased = *j.LastReleased
	}
	if j.LastHeartbeat != nil {
		status.LastHeartbeat = *j.LastHeartbeat
	}
	if j.ExpiryTime != nil {
		status.ExpiryTime = *j.ExpiryTime
	}

	if j.ModelInfo != nil {
		status.ModelInfo = &gpu.ModelInfo{
			Provider: j.ModelInfo.Provider,
			Model:    j.ModelInfo.Model,
		}
	}

	status.GPUModel = j.GPUModel

	return status
}

type hostSummary struct {
	host       string
	totalGPUs  string
	gpuModels  string
	available  string
	inUse      string
	unreserved string
}

func displaySummary(host string, statuses []gpu.GPUStatusInfo) {
	totalGPUs := len(statuses)
	availableCount := 0
	inUseCount := 0

	// Count by GPU model type
	modelCounts := make(map[string]int)
	availableByModel := make(map[string]int)

	for _, status := range statuses {
		// Get GPU model from provider info
		gpuModel := ""
		if status.GPUModel != "" {
			gpuModel = status.GPUModel
		}

		if gpuModel != "" {
			modelCounts[gpuModel]++
		}

		switch status.Status {
		case "AVAILABLE":
			availableCount++
			if gpuModel != "" {
				availableByModel[gpuModel]++
			}
		case "IN_USE", "UNRESERVED":
			// Combine reserved and unreserved usage into IN_USE
			inUseCount++
		}
	}

	// Build GPU models string
	var modelsStr string
	if len(modelCounts) == 0 {
		modelsStr = "-"
	} else if len(modelCounts) == 1 {
		for model := range modelCounts {
			modelsStr = model
		}
	} else {
		models := make([]string, 0, len(modelCounts))
		for model := range modelCounts {
			models = append(models, model)
		}
		sort.Strings(models)
		var parts []string
		for _, model := range models {
			parts = append(parts, fmt.Sprintf("%d %s", modelCounts[model], model))
		}
		modelsStr = strings.Join(parts, ", ")
	}

	fmt.Printf("%-20s %5d  %-30s %9d  %6d\n",
		host, totalGPUs, modelsStr, availableCount, inUseCount)
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
	GPUModel        string         `json:"gpu_model,omitempty"`
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

		// Add GPU model if present
		if status.GPUModel != "" {
			jsonStatus.GPUModel = status.GPUModel
		}

		jsonStatuses[i] = jsonStatus
	}

	// Output as pretty-printed JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonStatuses)
}
