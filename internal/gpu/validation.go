package gpu

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/russellb/canhazgpu/internal/types"
	"github.com/russellb/canhazgpu/internal/utils"
)

// DetectGPUUsage queries nvidia-smi to get actual GPU usage
func DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	usage := make(map[int]*types.GPUUsage)

	// Query GPU memory usage
	memoryUsage, err := queryGPUMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query GPU memory: %v", err)
	}

	// Query GPU processes
	processes, err := queryGPUProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query GPU processes: %v", err)
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

// queryGPUMemory queries GPU memory usage via nvidia-smi
func queryGPUMemory(ctx context.Context) (map[int]int, error) {
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
func queryGPUProcesses(ctx context.Context) (map[int][]types.GPUProcessInfo, error) {
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
		gpuID, err := getGPUIDFromUUID(ctx, gpuUUID)
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
func getGPUIDFromUUID(ctx context.Context, uuid string) (int, error) {
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

// getProcessOwner determines the owner of a process
func getProcessOwner(pid int) (string, error) {
	// Try /proc filesystem first
	if user, err := getProcessOwnerFromProc(pid); err == nil {
		return user, nil
	}

	// Fallback to ps command
	return getProcessOwnerFromPS(pid)
}

// GetProcessOwner is the exported version for tests
func GetProcessOwner(pid int) (string, error) {
	return getProcessOwner(pid)
}

// getProcessOwnerFromProc reads process owner from /proc filesystem
func getProcessOwnerFromProc(pid int) (string, error) {
	statusFile := fmt.Sprintf("/proc/%d/status", pid)

	file, err := os.Open(statusFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				uid, err := strconv.Atoi(fields[1])
				if err != nil {
					return "", err
				}
				return utils.GetUsernameFromUID(uid)
			}
		}
	}

	return "", fmt.Errorf("could not find UID in %s", statusFile)
}

// getProcessOwnerFromPS uses ps command to get process owner
func getProcessOwnerFromPS(pid int) (string, error) {
	cmd := exec.Command("ps", "-o", "user=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	user := strings.TrimSpace(string(output))
	if user == "" {
		return "", fmt.Errorf("empty user from ps command")
	}

	return user, nil
}

// GetUnreservedGPUs returns list of GPU IDs that are in use without proper reservations
func GetUnreservedGPUs(ctx context.Context, usage map[int]*types.GPUUsage, memoryThreshold int) []int {
	var unreserved []int

	for gpuID, gpuUsage := range usage {
		if gpuUsage.MemoryMB > memoryThreshold {
			unreserved = append(unreserved, gpuID)
		}
	}

	return unreserved
}

// IsGPUInUnreservedUse checks if a specific GPU is in unreserved use
func IsGPUInUnreservedUse(usage *types.GPUUsage, memoryThreshold int) bool {
	return usage != nil && usage.MemoryMB > memoryThreshold
}

