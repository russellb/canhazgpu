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
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

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
