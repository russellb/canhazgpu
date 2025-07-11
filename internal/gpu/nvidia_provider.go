package gpu

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/russellb/canhazgpu/internal/types"
)

// NVIDIAProvider implements the GPUProvider interface for NVIDIA GPUs using nvidia-smi
type NVIDIAProvider struct{}

// NewNVIDIAProvider creates a new NVIDIA GPU provider
func NewNVIDIAProvider() *NVIDIAProvider {
	return &NVIDIAProvider{}
}

// Name returns the name of the provider
func (n *NVIDIAProvider) Name() string {
	return "nvidia"
}

// IsAvailable checks if nvidia-smi is available on the system
func (n *NVIDIAProvider) IsAvailable() bool {
	cmd := exec.Command("nvidia-smi", "--help")
	err := cmd.Run()
	return err == nil
}

// DetectGPUUsage queries NVIDIA GPU usage via nvidia-smi
func (n *NVIDIAProvider) DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	usage := make(map[int]*types.GPUUsage)

	// Query GPU memory usage
	memoryUsage, err := n.queryGPUMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query NVIDIA GPU memory: %v", err)
	}

	// Query GPU processes
	processes, err := n.queryGPUProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query NVIDIA GPU processes: %v", err)
	}

	// Combine memory usage and process information
	for gpuID, memoryMB := range memoryUsage {
		gpuUsage := &types.GPUUsage{
			GPUID:     gpuID,
			MemoryMB:  memoryMB,
			Processes: []types.GPUProcessInfo{},
			Users:     make(map[string]bool),
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

// GetGPUCount returns the number of NVIDIA GPUs on the system
func (n *NVIDIAProvider) GetGPUCount(ctx context.Context) (int, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi", "-L")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("nvidia-smi -L failed: %v", err)
	}

	// Count the number of lines that start with "GPU"
	lines := strings.Split(string(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "GPU ") {
			count++
		}
	}

	return count, nil
}

// queryGPUMemory queries GPU memory usage via nvidia-smi
func (n *NVIDIAProvider) queryGPUMemory(ctx context.Context) (map[int]int, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=memory.used",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi failed: %v", err)
	}

	memory := make(map[int]int)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	gpuID := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		memoryMB, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse memory usage '%s': %v", line, err)
		}

		memory[gpuID] = memoryMB
		gpuID++
	}

	return memory, scanner.Err()
}

// queryGPUProcesses queries GPU processes via nvidia-smi
func (n *NVIDIAProvider) queryGPUProcesses(ctx context.Context) (map[int][]types.GPUProcessInfo, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-compute-apps=pid,process_name,gpu_uuid,used_memory",
		"--format=csv,noheader")

	output, err := cmd.Output()
	if err != nil {
		// nvidia-smi returns non-zero when no processes are found
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) == 0 {
			return make(map[int][]types.GPUProcessInfo), nil
		}
		return nil, fmt.Errorf("nvidia-smi processes query failed: %v", err)
	}

	processes := make(map[int][]types.GPUProcessInfo)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Split(line, ", ")
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}

		processName := strings.TrimSpace(fields[1])
		gpuUUID := strings.TrimSpace(fields[2])
		memoryStr := strings.TrimSpace(fields[3])

		// Parse memory (remove " MiB" suffix if present)
		memoryStr = strings.TrimSuffix(memoryStr, " MiB")
		memoryMB, err := strconv.Atoi(memoryStr)
		if err != nil {
			memoryMB = 0
		}

		// Get GPU ID from UUID (this is approximate - nvidia-smi doesn't always provide this mapping)
		gpuID, err := n.getGPUIDFromUUID(ctx, gpuUUID)
		if err != nil {
			// Fallback: try to extract from process info
			continue
		}

		// Get process owner
		user, err := getProcessOwner(pid)
		if err != nil {
			user = "unknown"
		}

		procInfo := types.GPUProcessInfo{
			PID:         pid,
			ProcessName: processName,
			User:        user,
			MemoryMB:    memoryMB,
		}

		processes[gpuID] = append(processes[gpuID], procInfo)
	}

	return processes, scanner.Err()
}

// getGPUIDFromUUID attempts to map GPU UUID to GPU ID
func (n *NVIDIAProvider) getGPUIDFromUUID(ctx context.Context, uuid string) (int, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,gpu_uuid",
		"--format=csv,noheader")

	output, err := cmd.Output()
	if err != nil {
		return -1, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Split(line, ", ")
		if len(fields) >= 2 {
			if strings.TrimSpace(fields[1]) == uuid {
				return strconv.Atoi(strings.TrimSpace(fields[0]))
			}
		}
	}

	return -1, fmt.Errorf("GPU UUID not found: %s", uuid)
} 