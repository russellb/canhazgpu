package gpu

import (
	"os/exec"
	"testing"
)

// isNvidiaSmiAvailable checks if nvidia-smi command is available
// This is used by tests to skip tests that require nvidia-smi when it's not present
func isNvidiaSmiAvailable() bool {
	_, err := exec.LookPath("nvidia-smi")
	return err == nil
}

// isAmdSmiAvailable checks if amd-smi command is available
// This is used by tests to skip tests that require amd-smi when it's not present
func isAmdSmiAvailable() bool {
	_, err := exec.LookPath("amd-smi")
	return err == nil
}

// isAnyGPUProviderAvailable checks if any GPU provider is available
func isAnyGPUProviderAvailable() bool {
	return isNvidiaSmiAvailable() || isAmdSmiAvailable()
}

// TestIsNvidiaSmiAvailable tests the helper function itself
func TestIsNvidiaSmiAvailable(t *testing.T) {
	available := isNvidiaSmiAvailable()
	t.Logf("nvidia-smi availability: %v", available)

	// This test just documents the current state, doesn't assert a specific value
	// since it depends on the test environment
}

// TestIsAmdSmiAvailable tests the helper function itself
func TestIsAmdSmiAvailable(t *testing.T) {
	available := isAmdSmiAvailable()
	t.Logf("amd-smi availability: %v", available)

	// This test just documents the current state, doesn't assert a specific value
	// since it depends on the test environment
}
