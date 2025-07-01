# Contributing

Thank you for your interest in contributing to canhazgpu! This guide covers how to contribute code, report issues, and help improve the project.

## Getting Started

### 1. Development Environment Setup

**Clone the repository:**
```bash
git clone https://github.com/russellb/canhazgpu.git
cd canhazgpu
```

**Install dependencies:**
```bash
# System requirements
# - Go 1.23+
# - Redis server
# - NVIDIA drivers with nvidia-smi

# Go dependencies (automatic)
go mod download

# Development dependencies
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Documentation dependencies (optional)
pip install -r requirements-docs.txt
```

**Set up test environment:**
```bash
# Start Redis for testing
redis-server --daemonize yes

# Build the project
make build

# Verify environment
./build/canhazgpu --help
```

### 2. Development Workflow

**Create a feature branch:**
```bash
git checkout -b feature/your-feature-name
```

**Make your changes:**
- Edit Go source files in `internal/` directories
- Add tests for new functionality
- Update documentation if needed

**Run tests:**
```bash
# Build the project
make build

# Run tests
make test

# Linting and formatting
gofmt -s -w .
goimports -w .
golangci-lint run
```

**Commit your changes:**
```bash
git add .
git commit -m "feat: add your feature description"
```

### 3. Documentation Development

**Install documentation dependencies:**
```bash
make docs-deps
# or manually: pip install -r requirements-docs.txt  # for documentation only
```

**Build and preview documentation:**
```bash
# Build documentation
make docs

# Preview documentation locally
make docs-preview
# Opens http://127.0.0.1:8000 in your browser

# Clean build files
make docs-clean
```

## Code Style and Standards

### 1. Go Style Guide

Follow standard Go conventions with these specific guidelines:

**Formatting:** Use `gofmt` and `goimports` for automatic formatting
**Naming:** Use camelCase for unexported functions, PascalCase for exported functions
**Documentation:** Use Go doc comments starting with the function name
**Error handling:** Always handle errors explicitly, don't ignore them

**Example:**
```go
// AllocateGPUs reserves the specified number of GPUs for the given duration.
// It returns a slice of allocated GPU IDs or an error if allocation fails.
func AllocateGPUs(ctx context.Context, request *AllocationRequest) ([]int, error) {
    if request.GPUCount <= 0 {
        return nil, fmt.Errorf("gpu count must be positive, got %d", request.GPUCount)
    }
    
    // Acquire allocation lock to prevent race conditions
    if err := engine.client.AcquireAllocationLock(ctx); err != nil {
        return nil, fmt.Errorf("failed to acquire allocation lock: %w", err)
    }
    defer engine.client.ReleaseAllocationLock(ctx)
    
    // Implementation...
    return allocatedGPUs, nil
}
```

### 2. Code Formatting

Use standard Go formatting tools:
```bash
# Format code (automatically fixes formatting)
gofmt -s -w .

# Organize imports
goimports -w .

# Run both together
make format  # if you add this target to Makefile
```

### 3. Linting

Use golangci-lint for comprehensive linting:
```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run

# Run with specific linters
golangci-lint run --enable-all --disable wsl,nlreturn
```

### 4. Go Modules

Keep dependencies clean and up to date:
```bash
# Tidy modules (remove unused dependencies)
go mod tidy

# Update dependencies
go get -u ./...

# Verify dependencies
go mod verify
```

## Testing Guidelines

!!! info "Complete Testing Guide"
    For comprehensive testing information including test types, running tests, and debugging, see the [Testing Guide](dev-testing.md).

### 1. Test Structure

**Test organization:**
```
internal/
├── gpu/
│   ├── allocation.go
│   ├── allocation_test.go
│   ├── validation.go
│   └── validation_test.go
├── redis_client/
│   ├── client.go
│   └── client_test.go
└── types/
    ├── types.go
    └── types_test.go
```

### 2. Unit Testing

**Mock external dependencies:**
```go
package gpu

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// MockRedisClient implements the redis client interface for testing
type MockRedisClient struct {
    mock.Mock
}

func (m *MockRedisClient) GetGPUState(ctx context.Context, gpuID int) (*types.GPUState, error) {
    args := m.Called(ctx, gpuID)
    return args.Get(0).(*types.GPUState), args.Error(1)
}

func TestAllocateGPUs(t *testing.T) {
    // Setup
    mockClient := new(MockRedisClient)
    engine := &AllocationEngine{client: mockClient}
    
    // Mock expectations
    mockClient.On("GetGPUState", mock.Anything, 0).Return(&types.GPUState{}, nil)
    
    // Test
    result, err := engine.AllocateGPUs(context.Background(), &types.AllocationRequest{
        GPUCount: 1,
        User:     "testuser",
    })
    
    // Assertions
    assert.NoError(t, err)
    assert.Len(t, result, 1)
    mockClient.AssertExpectations(t)
}
```

**Test error conditions:**
```go
func TestAllocateGPUs_InsufficientGPUs(t *testing.T) {
    // Setup
    mockClient := new(MockRedisClient)
    engine := &AllocationEngine{client: mockClient}
    
    // Mock returning no available GPUs
    mockClient.On("GetGPUCount", mock.Anything).Return(2, nil)
    mockClient.On("GetGPUState", mock.Anything, mock.Anything).Return(&types.GPUState{User: "other"}, nil)
    
    // Test
    _, err := engine.AllocateGPUs(context.Background(), &types.AllocationRequest{
        GPUCount: 10,
        User:     "testuser",
    })
    
    // Assertions
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "Not enough GPUs available")
}
```

### 3. Integration Testing

**Redis integration tests:**
```go
package redis_client

import (
    "context"
    "testing"
    "github.com/go-redis/redis/v8"
    "github.com/stretchr/testify/assert"
    "github.com/russellb/canhazgpu/internal/types"
)

func setupTestRedis(t *testing.T) *Client {
    // Use test database (15) to avoid conflicts
    config := &types.Config{
        RedisHost: "localhost",
        RedisPort: 6379,
        RedisDB:   15,
    }
    
    client := NewClient(config)
    
    // Clean state before test
    client.rdb.FlushDB(context.Background())
    
    // Cleanup after test
    t.Cleanup(func() {
        client.rdb.FlushDB(context.Background())
        client.Close()
    })
    
    return client
}

func TestGPUStatePersistence(t *testing.T) {
    client := setupTestRedis(t)
    ctx := context.Background()
    
    // Set up initial state
    err := client.SetGPUCount(ctx, 4)
    assert.NoError(t, err)
    
    // Reserve a GPU by setting state
    state := &types.GPUState{
        User:      "testuser",
        StartTime: types.FlexibleTime{Time: time.Now()},
        Type:      "manual",
    }
    err = client.SetGPUState(ctx, 0, state)
    assert.NoError(t, err)
    
    // Verify state persistence
    retrievedState, err := client.GetGPUState(ctx, 0)
    assert.NoError(t, err)
    assert.Equal(t, "testuser", retrievedState.User)
}
```

### 4. Test Coverage

Aim for >90% test coverage:
```bash
# Run tests with coverage
go test -race -coverprofile=coverage.out ./...

# View coverage report
go tool cover -html=coverage.out

# Get coverage percentage
go tool cover -func=coverage.out

# Generate coverage for specific packages
go test -coverprofile=coverage.out ./internal/gpu/
go tool cover -html=coverage.out
```

## Feature Development

### 1. Adding New Commands

To add a new command:

1. **Add Cobra command file:**
```go
// internal/cli/newcommand.go
package cli

import (
    "github.com/spf13/cobra"
)

func init() {
    rootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
    Use:   "new",
    Short: "Command description",
    RunE: func(cmd *cobra.Command, args []string) error {
        option, _ := cmd.Flags().GetString("option")
        return runNewCommand(cmd.Context(), option)
    },
}

func init() {
    newCmd.Flags().String("option", "", "Option description")
}
```

2. **Implement core logic:**
```go
func runNewCommand(ctx context.Context, option string) error {
    if option == "" {
        return fmt.Errorf("option is required")
    }
    
    result, err := implementNewCommand(ctx, option)
    if err != nil {
        return fmt.Errorf("command failed: %w", err)
    }
    
    fmt.Printf("Success: %s\n", result)
    return nil
}

func implementNewCommand(ctx context.Context, option string) (string, error) {
    // Implementation
    return "result", nil
}
```

3. **Add tests:**
```go
func TestNewCommand(t *testing.T) {
    result, err := implementNewCommand(context.Background(), "test_value")
    assert.NoError(t, err)
    assert.Equal(t, "expected_result", result)
}
```

4. **Update documentation:**
- Add to `docs/commands.md`
- Update help text
- Add usage examples

### 2. Adding New Allocation Strategies

To add alternative allocation strategies:

1. **Create strategy function:**
```python
def get_available_gpus_thermal_aware(
    gpu_count: int, 
    requested: int,
    redis_client: redis.Redis
) -> List[int]:
    """Allocate GPUs based on thermal conditions.
    
    Args:
        gpu_count: Total number of GPUs
        requested: Number of GPUs requested
        redis_client: Redis connection
        
    Returns:
        List of GPU IDs sorted by thermal preference
    """
    # Query GPU temperatures
    temperatures = get_gpu_temperatures()
    
    # Sort by coolest first
    available = get_available_gpus(gpu_count, redis_client)
    return sorted(available, key=lambda gpu_id: temperatures[gpu_id])[:requested]
```

2. **Integrate with allocation logic:**
```python
# Add strategy selection parameter
def atomic_reserve_gpus(
    requested_gpus: int,
    user: str,
    reservation_type: str,
    strategy: str = "lru",
    expiry_time: Optional[float] = None
) -> List[int]:
    """Reserve GPUs using specified allocation strategy."""
    
    if strategy == "lru":
        available = get_available_gpus_sorted_by_lru(...)
    elif strategy == "thermal":
        available = get_available_gpus_thermal_aware(...)
    else:
        raise ValueError(f"Unknown strategy: {strategy}")
```

### 3. Adding New Validation Sources

To add support for AMD GPUs or other hardware:

1. **Create validation module:**
```python
def detect_amd_gpu_usage() -> Dict[int, Dict]:
    """Detect AMD GPU usage via rocm-smi."""
    try:
        result = subprocess.run([
            'rocm-smi', '--showuse', '--csv'
        ], capture_output=True, text=True, check=True)
        
        return parse_amd_usage(result.stdout)
    except (subprocess.CalledProcessError, FileNotFoundError):
        return {}
```

2. **Integrate with main validation:**
```python
def detect_gpu_usage() -> Dict[int, Dict]:
    """Detect GPU usage from all available sources."""
    usage = {}
    
    # NVIDIA GPUs
    nvidia_usage = detect_nvidia_gpu_usage()
    usage.update(nvidia_usage)
    
    # AMD GPUs  
    amd_usage = detect_amd_gpu_usage()
    usage.update(amd_usage)
    
    return usage
```

## Documentation

### 1. Code Documentation

**Docstring requirements:**
- All public functions must have docstrings
- Include parameter descriptions and types
- Document return values and exceptions
- Provide usage examples for complex functions

**Inline comments:**
- Explain complex logic
- Document assumptions and edge cases
- Reference external resources (RFCs, papers, etc.)

### 2. User Documentation

When adding features, update:
- `docs/commands.md` - Command reference
- `docs/usage-*.md` - Usage examples
- `docs/features-*.md` - Feature explanations
- `README.md` - If major feature

### 3. API Documentation

For internal APIs, document:
- Function contracts and invariants
- Error conditions and handling
- Performance characteristics
- Thread safety considerations

## Pull Request Process

### 1. Before Submitting

**Checklist:**
- [ ] Tests pass locally
- [ ] Code follows style guidelines
- [ ] Documentation updated
- [ ] Commit messages follow convention
- [ ] No merge conflicts with main branch

**Self-review:**
- Review your own changes first
- Check for debugging code or TODOs
- Verify all edge cases are handled
- Ensure error messages are helpful

### 2. Pull Request Format

**Title:** Use conventional commit format
```
feat: add thermal-aware GPU allocation
fix: handle Redis connection timeout gracefully  
docs: update installation instructions
```

**Description template:**
```markdown
## Summary
Brief description of the change and why it's needed.

## Changes
- Specific change 1
- Specific change 2
- Specific change 3

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Manual testing performed

## Breaking Changes
None / List any breaking changes

## Related Issues
Fixes #123
Relates to #456
```

### 3. Review Process

**What reviewers look for:**
- Code correctness and logic
- Test coverage and quality
- Performance implications
- Security considerations
- Documentation completeness
- Style and maintainability

**Addressing feedback:**
- Respond to all review comments
- Make requested changes promptly
- Ask questions if feedback is unclear
- Re-request review after changes

## Issue Reporting

### 1. Bug Reports

**Include in bug reports:**
- canhazgpu version
- Operating system and version
- Python version
- Redis version
- NVIDIA driver version
- Complete error messages
- Steps to reproduce
- Expected vs actual behavior

**Bug report template:**
```markdown
## Bug Description
Clear description of the bug

## Environment
- OS: Ubuntu 20.04
- Python: 3.8.10
- Redis: 6.0.16
- NVIDIA Driver: 470.129.06
- canhazgpu version: 1.0.0

## Steps to Reproduce
1. Step 1
2. Step 2
3. Step 3

## Expected Behavior
What should happen

## Actual Behavior
What actually happens

## Error Messages
```
Complete error output
```

## Additional Context
Any other relevant information
```

### 2. Feature Requests

**Include in feature requests:**
- Clear use case description
- Proposed solution (if any)
- Alternatives considered
- Impact on existing functionality
- Example usage

## Release Process

!!! info "Complete Release Guide"
    For detailed release procedures including goreleaser usage and troubleshooting, see the [Release Process Guide](dev-release.md).

### 1. Version Numbering

Follow semantic versioning (SemVer):
- **Major** (X.0.0): Breaking changes
- **Minor** (0.X.0): New features, backward compatible
- **Patch** (0.0.X): Bug fixes, backward compatible

### 2. Quick Release Steps

```bash
# Tag and push
git tag vX.Y.Z
git push origin vX.Y.Z

# Release with goreleaser
GITHUB_TOKEN=$(gh auth token) goreleaser --clean
```

## Community Guidelines

### 1. Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help newcomers learn and contribute
- Assume good intentions

### 2. Communication

- Use GitHub issues for bug reports and feature requests
- Use pull requests for code changes
- Tag maintainers for urgent issues
- Be patient with response times

### 3. Recognition

Contributors are recognized in:
- CONTRIBUTORS.md file
- Release notes
- Documentation acknowledgments

Thank you for contributing to canhazgpu! Your efforts help make GPU resource management better for everyone.