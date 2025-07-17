package gpu

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/russellb/canhazgpu/internal/types"
)

// AMDProvider implements the GPUProvider interface for AMD GPUs using amd-smi
type AMDProvider struct{}

// NewAMDProvider creates a new AMD GPU provider
func NewAMDProvider() *AMDProvider {
	return &AMDProvider{}
}

// Name returns the name of the provider
func (a *AMDProvider) Name() string {
	return "amd"
}

// IsAvailable checks if amd-smi is available on the system
func (a *AMDProvider) IsAvailable() bool {
	cmd := exec.Command("amd-smi", "--help")
	err := cmd.Run()
	return err == nil
}

// DetectGPUUsage queries AMD GPU usage via amd-smi
func (a *AMDProvider) DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	usage := make(map[int]*types.GPUUsage)

	// Query GPU memory usage
	memoryUsage, err := a.queryGPUMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query AMD GPU memory: %v", err)
	}

	// Query GPU processes
	processes, err := a.queryGPUProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query AMD GPU processes: %v", err)
	}

	// Combine memory usage and process information
	for gpuID, memoryMB := range memoryUsage {
		gpuUsage := &types.GPUUsage{
			GPUID:     gpuID,
			MemoryMB:  memoryMB,
			Processes: []types.GPUProcessInfo{},
			Users:     make(map[string]bool),
			Provider:  "AMD",
			Model:     "", // Leave blank for AMD GPUs
		}

		// Add processes for this GPU
		if gpuProcesses, exists := processes[gpuID]; exists {
			for _, proc := range gpuProcesses {
				gpuUsage.Processes = append(gpuUsage.Processes, proc)
				gpuUsage.Users[proc.User] = true
			}
		}

		usage[gpuID] = gpuUsage
	}

	return usage, nil
}

// GetGPUCount returns the number of AMD GPUs on the system
func (a *AMDProvider) GetGPUCount(ctx context.Context) (int, error) {
	cmd := exec.CommandContext(ctx, "amd-smi", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("amd-smi list failed: %v", err)
	}

	var listData []map[string]interface{}
	if err := json.Unmarshal(output, &listData); err != nil {
		return 0, fmt.Errorf("failed to parse amd-smi list output: %v", err)
	}

	// Count GPUs in the response
	count := 0
	for _, gpu := range listData {
		if _, ok := gpu["gpu"]; ok {
			count++
		}
	}

	return count, nil
}

// queryGPUMemory queries GPU memory usage via amd-smi
func (a *AMDProvider) queryGPUMemory(ctx context.Context) (map[int]int, error) {
	cmd := exec.CommandContext(ctx, "amd-smi", "metric", "-m", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("amd-smi metric failed: %v", err)
	}

	var metricData []map[string]interface{}
	if err := json.Unmarshal(output, &metricData); err != nil {
		return nil, fmt.Errorf("failed to parse amd-smi metric output: %v", err)
	}

	memory := make(map[int]int)

	// Parse GPU memory usage from JSON output
	for _, gpu := range metricData {
		if gpuIDVal, ok := gpu["gpu"].(float64); ok {
			gpuID := int(gpuIDVal)

			if memUsage, ok := gpu["mem_usage"].(map[string]interface{}); ok {
				if usedVram, ok := memUsage["used_vram"].(map[string]interface{}); ok {
					if memValue, ok := usedVram["value"].(float64); ok {
						// Convert to MB if needed
						memoryMB := int(memValue)
						if unit, ok := usedVram["unit"].(string); ok && unit == "GB" {
							memoryMB = int(memValue * 1024)
						}
						memory[gpuID] = memoryMB
					}
				}
			}
		}
	}

	return memory, nil
}

// queryGPUProcesses queries GPU processes via amd-smi
func (a *AMDProvider) queryGPUProcesses(ctx context.Context) (map[int][]types.GPUProcessInfo, error) {
	cmd := exec.CommandContext(ctx, "amd-smi", "process", "--json")
	output, err := cmd.Output()
	if err != nil {
		// amd-smi might return non-zero when no processes are found
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) == 0 {
			return make(map[int][]types.GPUProcessInfo), nil
		}
		return nil, fmt.Errorf("amd-smi process query failed: %v", err)
	}

	var processData []map[string]interface{}
	if err := json.Unmarshal(output, &processData); err != nil {
		return nil, fmt.Errorf("failed to parse amd-smi process output: %v", err)
	}

	processes := make(map[int][]types.GPUProcessInfo)

	// Parse process information from JSON output
	for _, gpu := range processData {
		if gpuIDVal, ok := gpu["gpu"].(float64); ok {
			gpuID := int(gpuIDVal)

			if procList, ok := gpu["process_list"].([]interface{}); ok {
				for _, procEntry := range procList {
					if proc, ok := procEntry.(map[string]interface{}); ok {
						// Check if this is the "No running processes detected" message
						if procInfo, ok := proc["process_info"].(string); ok {
							if procInfo == "No running processes detected" {
								continue
							}
						}

						// Extract the actual process_info object
						if procInfo, ok := proc["process_info"].(map[string]interface{}); ok {
							processInfo := a.parseProcessInfo(procInfo)
							if processInfo != nil {
								processes[gpuID] = append(processes[gpuID], *processInfo)
							}
						}
					}
				}
			}
		}
	}

	return processes, nil
}

// parseProcessInfo parses process information from amd-smi JSON output
func (a *AMDProvider) parseProcessInfo(proc map[string]interface{}) *types.GPUProcessInfo {
	processInfo := &types.GPUProcessInfo{}

	// Extract PID
	if pidVal, ok := proc["pid"].(float64); ok {
		processInfo.PID = int(pidVal)
	} else {
		return nil
	}

	// Extract process name
	if nameVal, ok := proc["name"].(string); ok {
		processInfo.ProcessName = nameVal
	}

	// Extract memory usage
	if memUsage, ok := proc["memory_usage"].(map[string]interface{}); ok {
		if vramMem, ok := memUsage["vram_mem"].(map[string]interface{}); ok {
			if memValue, ok := vramMem["value"].(float64); ok {
				memoryMB := int(memValue)
				if unit, ok := vramMem["unit"].(string); ok {
					switch unit {
					case "B":
						memoryMB = int(memValue / (1024 * 1024)) // Convert bytes to MB
					case "KB":
						memoryMB = int(memValue / 1024) // Convert KB to MB
					case "MB":
						memoryMB = int(memValue) // Already in MB
					case "GB":
						memoryMB = int(memValue * 1024) // Convert GB to MB
					}
				}
				processInfo.MemoryMB = memoryMB
			}
		}
	}

	// Get process owner
	user, err := getProcessOwner(processInfo.PID)
	if err != nil {
		user = "unknown"
	}
	processInfo.User = user

	return processInfo
}
