package gpu

import (
	"context"

	"github.com/russellb/canhazgpu/internal/types"
)

// FakeProvider implements the GPUProvider interface for development and testing
// on systems without actual GPUs
type FakeProvider struct {
	gpuCount int
}

// NewFakeProvider creates a new fake GPU provider with the specified GPU count
func NewFakeProvider(gpuCount int) *FakeProvider {
	return &FakeProvider{
		gpuCount: gpuCount,
	}
}

// Name returns the name of the provider
func (f *FakeProvider) Name() string {
	return "fake"
}

// IsAvailable always returns true since this is a fake provider for testing
func (f *FakeProvider) IsAvailable() bool {
	return true
}

// DetectGPUUsage returns empty usage data for all fake GPUs
func (f *FakeProvider) DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	usage := make(map[int]*types.GPUUsage)

	for gpuID := range f.gpuCount {
		usage[gpuID] = &types.GPUUsage{
			GPUID:     gpuID,
			MemoryMB:  0,
			Processes: []types.GPUProcessInfo{},
			Users:     make(map[string]bool),
			Provider:  "Fake",
			Model:     "Fake GPU",
		}
	}

	return usage, nil
}

// GetGPUCount returns the configured number of fake GPUs
func (f *FakeProvider) GetGPUCount(ctx context.Context) (int, error) {
	return f.gpuCount, nil
}

// SetGPUCount allows updating the GPU count (useful when loading from Redis)
func (f *FakeProvider) SetGPUCount(count int) {
	f.gpuCount = count
}
