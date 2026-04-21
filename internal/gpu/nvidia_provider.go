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

// DetectGPUUsage queries NVIDIA GPU usage via nvidia-smi.
// Runs GPU info and process queries in parallel to minimize nvidia-smi overhead.
func (n *NVIDIAProvider) DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	gpuInfo, err := n.queryGPUInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query NVIDIA GPU info: %v", err)
	}

	uuidMap := make(map[string]int, len(gpuInfo))
	for _, info := range gpuInfo {
		uuidMap[info.uuid] = info.index
	}

	processes, err := n.queryGPUProcesses(ctx, uuidMap)
	if err != nil {
		return nil, fmt.Errorf("failed to query NVIDIA GPU processes: %v", err)
	}

	usage := make(map[int]*types.GPUUsage)
	for _, info := range gpuInfo {
		gpuUsage := &types.GPUUsage{
			GPUID:     info.index,
			MemoryMB:  info.memoryMB,
			Processes: []types.GPUProcessInfo{},
			Users:     make(map[string]bool),
			Provider:  "NVIDIA",
			Model:     info.model,
		}

		if gpuProcesses, exists := processes[info.index]; exists {
			for _, proc := range gpuProcesses {
				gpuUsage.Processes = append(gpuUsage.Processes, proc)
				gpuUsage.Users[proc.User] = true
			}
		}

		usage[info.index] = gpuUsage
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

type gpuInfoEntry struct {
	index    int
	uuid     string
	model    string
	memoryMB int
}

// queryGPUInfo queries GPU index, UUID, model name, and memory usage in a single nvidia-smi call.
func (n *NVIDIAProvider) queryGPUInfo(ctx context.Context) ([]gpuInfoEntry, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,gpu_uuid,name,memory.used",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi failed: %v", err)
	}

	var entries []gpuInfoEntry
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

		index, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}

		uuid := strings.TrimSpace(fields[1])

		model := strings.TrimSpace(fields[2])
		model = strings.TrimPrefix(model, "NVIDIA ")

		memoryMB, err := strconv.Atoi(strings.TrimSpace(fields[3]))
		if err != nil {
			continue
		}

		entries = append(entries, gpuInfoEntry{
			index:    index,
			uuid:     uuid,
			model:    model,
			memoryMB: memoryMB,
		})
	}

	return entries, scanner.Err()
}

// queryGPUProcesses queries GPU processes via nvidia-smi, using a pre-built UUID-to-index map.
func (n *NVIDIAProvider) queryGPUProcesses(ctx context.Context, uuidMap map[string]int) (map[int][]types.GPUProcessInfo, error) {
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

		gpuID, ok := uuidMap[gpuUUID]
		if !ok {
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
