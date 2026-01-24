package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeProvider_Name(t *testing.T) {
	provider := NewFakeProvider(4)
	assert.Equal(t, "fake", provider.Name())
}

func TestFakeProvider_IsAvailable(t *testing.T) {
	provider := NewFakeProvider(4)
	// Fake provider is always available
	assert.True(t, provider.IsAvailable())
}

func TestFakeProvider_GetGPUCount(t *testing.T) {
	tests := []struct {
		name     string
		gpuCount int
	}{
		{"zero GPUs", 0},
		{"single GPU", 1},
		{"multiple GPUs", 4},
		{"many GPUs", 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewFakeProvider(tt.gpuCount)
			count, err := provider.GetGPUCount(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.gpuCount, count)
		})
	}
}

func TestFakeProvider_DetectGPUUsage(t *testing.T) {
	tests := []struct {
		name     string
		gpuCount int
	}{
		{"zero GPUs", 0},
		{"single GPU", 1},
		{"multiple GPUs", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewFakeProvider(tt.gpuCount)
			usage, err := provider.DetectGPUUsage(context.Background())
			require.NoError(t, err)

			// Should have correct number of entries
			assert.Len(t, usage, tt.gpuCount)

			// Each GPU should have empty usage
			for i := 0; i < tt.gpuCount; i++ {
				gpuUsage, exists := usage[i]
				assert.True(t, exists, "GPU %d should exist", i)
				assert.Equal(t, i, gpuUsage.GPUID)
				assert.Equal(t, 0, gpuUsage.MemoryMB)
				assert.Empty(t, gpuUsage.Processes)
				assert.Empty(t, gpuUsage.Users)
				assert.Equal(t, "Fake", gpuUsage.Provider)
				assert.Equal(t, "Fake GPU", gpuUsage.Model)
			}
		})
	}
}

func TestFakeProvider_SetGPUCount(t *testing.T) {
	provider := NewFakeProvider(2)

	// Initial count
	count, err := provider.GetGPUCount(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Update count
	provider.SetGPUCount(8)
	count, err = provider.GetGPUCount(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 8, count)

	// Verify DetectGPUUsage reflects new count
	usage, err := provider.DetectGPUUsage(context.Background())
	require.NoError(t, err)
	assert.Len(t, usage, 8)
}

func TestNewProviderManagerWithFake(t *testing.T) {
	pm := NewProviderManagerWithFake(4)
	require.NotNil(t, pm)

	// Should have exactly one provider
	available := pm.GetAvailableProviders()
	require.Len(t, available, 1)
	assert.Equal(t, "fake", available[0].Name())

	// Should be able to get GPU count
	count, err := pm.GetTotalGPUCount(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}

func TestNewProviderManagerFromNames_Fake(t *testing.T) {
	pm := NewProviderManagerFromNames([]string{"fake"})
	require.NotNil(t, pm)

	// Should have exactly one provider (with default 0 GPUs)
	available := pm.GetAvailableProviders()
	require.Len(t, available, 1)
	assert.Equal(t, "fake", available[0].Name())
}
