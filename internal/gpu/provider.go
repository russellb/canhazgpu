package gpu

import (
	"context"
	"fmt"

	"github.com/russellb/canhazgpu/internal/types"
)

// GPUProvider defines the interface for GPU providers (NVIDIA, AMD, etc.)
type GPUProvider interface {
	// Name returns the name of the provider (e.g., "nvidia", "amd")
	Name() string
	
	// IsAvailable checks if the provider's tools are available on the system
	IsAvailable() bool
	
	// DetectGPUUsage queries the GPU usage for all GPUs managed by this provider
	DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error)
	
	// GetGPUCount returns the number of GPUs managed by this provider
	GetGPUCount(ctx context.Context) (int, error)
}

// ProviderManager manages multiple GPU providers
type ProviderManager struct {
	providers []GPUProvider
}

// NewProviderManager creates a new provider manager
func NewProviderManager() *ProviderManager {
	return &ProviderManager{
		providers: []GPUProvider{
			NewNVIDIAProvider(),
			NewAMDProvider(),
		},
	}
}

// NewProviderManagerFromNames creates a provider manager with only the specified providers
func NewProviderManagerFromNames(providerNames []string) *ProviderManager {
	var providers []GPUProvider
	
	for _, name := range providerNames {
		switch name {
		case "nvidia":
			providers = append(providers, NewNVIDIAProvider())
		case "amd":
			providers = append(providers, NewAMDProvider())
		}
	}
	
	return &ProviderManager{
		providers: providers,
	}
}

// GetAvailableProviders returns all available providers on the system
func (pm *ProviderManager) GetAvailableProviders() []GPUProvider {
	var available []GPUProvider
	for _, provider := range pm.providers {
		if provider.IsAvailable() {
			available = append(available, provider)
		}
	}
	return available
}

// DetectAllGPUUsage detects usage from the available provider
func (pm *ProviderManager) DetectAllGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
	availableProviders := pm.GetAvailableProviders()
	
	if len(availableProviders) == 0 {
		return make(map[int]*types.GPUUsage), nil
	}
	
	// Use the first (and only) available provider
	provider := availableProviders[0]
	return provider.DetectGPUUsage(ctx)
}

// DetectAllGPUUsageWithoutChecks detects usage from the provider without availability checks
// This is used when provider availability is already cached in Redis
func (pm *ProviderManager) DetectAllGPUUsageWithoutChecks(ctx context.Context) (map[int]*types.GPUUsage, error) {
	if len(pm.providers) == 0 {
		return nil, fmt.Errorf("no GPU providers configured in ProviderManager")
	}
	
	// Use the first (and only) provider
	provider := pm.providers[0]
	return provider.DetectGPUUsage(ctx)
}

// GetTotalGPUCount returns the total number of GPUs from the available provider
func (pm *ProviderManager) GetTotalGPUCount(ctx context.Context) (int, error) {
	availableProviders := pm.GetAvailableProviders()
	
	if len(availableProviders) == 0 {
		return 0, nil
	}
	
	// Use the first (and only) available provider
	provider := availableProviders[0]
	return provider.GetGPUCount(ctx)
} 