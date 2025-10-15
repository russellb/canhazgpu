package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
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
	noColorFlag bool
)

func init() {
	statusCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output status as JSON array")
	statusCmd.Flags().BoolVar(&showAll, "all", false, "Show status for all configured remote hosts")
	statusCmd.Flags().StringVarP(&remoteName, "remote", "r", "", "Show status for a specific remote host")
	statusCmd.Flags().BoolVarP(&showSummary, "summary", "s", false, "Show summary with GPU counts and availability")
	statusCmd.Flags().BoolVar(&noColorFlag, "no-color", false, "Disable colored output")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(ctx context.Context) error {
	config := getConfig()

	// Set color mode
	SetNoColor(noColorFlag)

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
		displaySingleHostSummary("localhost", statuses)
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
		displaySingleHostSummary(host, statuses)
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

	// For summary mode, collect all results first then display in table
	if showSummary {
		return runStatusAllHostsSummary(ctx, config)
	}

	// For table mode, display progressively
	// Get and display local status first
	localStatuses, localErr := getLocalStatus(ctx, config)

	if true { // Always table mode here
		fmt.Println()
		if localErr != nil {
			fmt.Printf("┌─ %s ─┐\n", FormatHost("localhost"))
			fmt.Printf("│ %s\n", FormatDim(fmt.Sprintf("ERROR: %v", localErr)))
			fmt.Println("└────────────┘")
		} else {
			fmt.Printf("┌─ %s ─┐\n", FormatHost("localhost"))
			displayGPUStatusTable(localStatuses)
		}
	}

	// Get and display each remote host progressively
	for _, host := range config.RemoteHosts {
		statuses, err := getRemoteStatus(ctx, host)

		fmt.Println()
		if err != nil {
			fmt.Printf("┌─ %s ─┐\n", FormatHost(host))
			fmt.Printf("│ %s\n", FormatDim(fmt.Sprintf("ERROR: %v", err)))
			fmt.Println("└────────────┘")
		} else {
			fmt.Printf("┌─ %s ─┐\n", FormatHost(host))
			displayGPUStatusTable(statuses)
		}
	}

	return nil
}

// runStatusAllHostsSummary collects all results then displays summary table
func runStatusAllHostsSummary(ctx context.Context, config *types.Config) error {
	if len(config.RemoteHosts) == 0 {
		return fmt.Errorf("no remote hosts configured in ~/.canhazgpu.yaml")
	}

	// Get local status first
	localStatuses, localErr := getLocalStatus(ctx, config)

	// Create table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.SeparateRows = false
	t.Style().Options.DrawBorder = false

	// Set header
	t.AppendHeader(table.Row{
		FormatHeader("HOST"),
		FormatHeader("TOTAL"),
		FormatHeader("GPU MODELS"),
		FormatHeader("AVAILABLE"),
		FormatHeader("IN USE"),
	})

	// Add localhost
	if localErr != nil {
		t.AppendRow(table.Row{
			FormatHost("localhost"),
			FormatDim("ERR"),
			FormatDim(fmt.Sprintf("ERROR: %v", localErr)),
			FormatDim("-"),
			FormatDim("-"),
		})
	} else {
		addSummaryRow(t, "localhost", localStatuses)
	}

	// Get and add each remote host
	for _, host := range config.RemoteHosts {
		statuses, err := getRemoteStatus(ctx, host)
		if err != nil {
			t.AppendRow(table.Row{
				FormatHost(host),
				FormatDim("ERR"),
				FormatDim(fmt.Sprintf("ERROR: %v", err)),
				FormatDim("-"),
				FormatDim("-"),
			})
		} else {
			addSummaryRow(t, host, statuses)
		}
	}

	fmt.Println()
	t.Render()
	fmt.Println()

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
	defer func() {
		_ = client.Close()
	}()

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

func displaySingleHostSummary(host string, statuses []gpu.GPUStatusInfo) {
	// Create table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.SeparateRows = false
	t.Style().Options.DrawBorder = false

	// Set header
	t.AppendHeader(table.Row{
		FormatHeader("HOST"),
		FormatHeader("TOTAL"),
		FormatHeader("GPU MODELS"),
		FormatHeader("AVAILABLE"),
		FormatHeader("IN USE"),
	})

	// Add single row
	addSummaryRow(t, host, statuses)

	fmt.Println()
	t.Render()
	fmt.Println()
}

func addSummaryRow(t table.Writer, host string, statuses []gpu.GPUStatusInfo) {
	totalGPUs := len(statuses)
	availableCount := 0
	inUseCount := 0

	// Count by GPU model type
	modelCounts := make(map[string]int)

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

	// Format available column: show checkmark if > 0, X if 0
	var availStr string
	if availableCount > 0 {
		availStr = fmt.Sprintf("%s %d", colorSuccess.Sprint("✓"), availableCount)
	} else {
		availStr = fmt.Sprintf("%s %d", colorError.Sprint("✗"), availableCount)
	}

	// Format in-use column: just the number, no symbol
	inUseStr := fmt.Sprintf("%d", inUseCount)

	t.AppendRow(table.Row{
		FormatHost(host),
		FormatMetric(totalGPUs),
		FormatDim(modelsStr),
		availStr,
		inUseStr,
	})
}

func displayGPUStatusTable(statuses []gpu.GPUStatusInfo) {
	// Check if any GPU has model information
	hasModels := false
	for _, status := range statuses {
		if status.ModelInfo != nil && status.ModelInfo.Model != "" {
			hasModels = true
			break
		}
	}

	// Create table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)

	// Set style
	t.SetStyle(table.StyleLight)
	t.Style().Options.SeparateRows = false
	t.Style().Options.DrawBorder = false

	// Set header
	if hasModels {
		t.AppendHeader(table.Row{
			FormatHeader("GPU"), FormatHeader("STATUS"), FormatHeader("USER"),
			FormatHeader("DURATION"), FormatHeader("TYPE"), FormatHeader("DETAILS"),
			FormatHeader("VALIDATION"), FormatHeader("MODEL"),
		})
	} else {
		t.AppendHeader(table.Row{
			FormatHeader("GPU"), FormatHeader("STATUS"), FormatHeader("USER"),
			FormatHeader("DURATION"), FormatHeader("TYPE"), FormatHeader("DETAILS"),
			FormatHeader("VALIDATION"),
		})
	}

	// Add rows
	for _, status := range statuses {
		addGPUStatusRow(t, status, hasModels)
	}

	t.Render()
}

func addGPUStatusRow(t table.Writer, status gpu.GPUStatusInfo, includeModel bool) {
	gpuID := fmt.Sprintf("%d", status.GPUID)

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
			t.AppendRow(table.Row{
				gpuID, FormatStatus("AVAILABLE"), FormatDim("-"), FormatDim("-"), FormatDim("-"),
				details, FormatDim(validation), model,
			})
		} else {
			t.AppendRow(table.Row{
				gpuID, FormatStatus("AVAILABLE"), FormatDim("-"), FormatDim("-"), FormatDim("-"),
				details, FormatDim(validation),
			})
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
			t.AppendRow(table.Row{
				gpuID, FormatStatus("IN_USE"), user, duration, reservationType, details, FormatDim(validation), model,
			})
		} else {
			t.AppendRow(table.Row{
				gpuID, FormatStatus("IN_USE"), user, duration, reservationType, details, FormatDim(validation),
			})
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
			t.AppendRow(table.Row{
				gpuID, FormatStatus("UNRESERVED"), userList, FormatDim("-"), FormatDim("-"),
				details, FormatDim("-"), model,
			})
		} else {
			t.AppendRow(table.Row{
				gpuID, FormatStatus("UNRESERVED"), userList, FormatDim("-"), FormatDim("-"),
				details, FormatDim("-"),
			})
		}

	case "ERROR":
		if includeModel {
			t.AppendRow(table.Row{
				gpuID, FormatStatus("ERROR"), FormatDim("-"), FormatDim("-"), FormatDim("-"),
				status.Error, FormatDim("-"), FormatDim("-"),
			})
		} else {
			t.AppendRow(table.Row{
				gpuID, FormatStatus("ERROR"), FormatDim("-"), FormatDim("-"), FormatDim("-"),
				status.Error, FormatDim("-"),
			})
		}

	default:
		if includeModel {
			t.AppendRow(table.Row{
				gpuID, "UNKNOWN", FormatDim("-"), FormatDim("-"), FormatDim("-"),
				"unknown status", FormatDim("-"), FormatDim("-"),
			})
		} else {
			t.AppendRow(table.Row{
				gpuID, "UNKNOWN", FormatDim("-"), FormatDim("-"), FormatDim("-"),
				"unknown status", FormatDim("-"),
			})
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
