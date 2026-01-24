# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is `canhazgpu`, a GPU reservation tool for single host shared development systems. It's a Go CLI application that uses Redis as a backend to coordinate GPU allocations across multiple users and processes, with comprehensive validation to detect and prevent unreserved GPU usage. The system supports both NVIDIA and AMD GPUs through a unified provider abstraction.

## Architecture

The tool is a Go application structured as a CLI with internal packages that implements eight main commands:
- `admin`: Initialize and configure the GPU pool with optional --force flag and --provider selection
- `status`: Show current GPU allocation status with automatic provider-specific validation
- `run`: Reserve GPU(s) and execute a command with `CUDA_VISIBLE_DEVICES` set (blocks if GPUs unavailable)
- `reserve`: Manually reserve GPU(s) for a specified duration (blocks if GPUs unavailable)
- `release`: Release all manually reserved GPUs for the current user
- `report`: Generate GPU reservation reports showing historical reservation patterns by user
- `queue`: Show the GPU reservation queue with wait times and allocation progress
- `web`: Start a web server providing a dashboard for real-time monitoring and reports

### Core Components

- **Redis Integration**: Uses Redis (localhost:6379) for persistent state management with keys under `canhazgpu:` prefix
- **GPU Provider System**: Unified abstraction supporting both NVIDIA (nvidia-smi) and AMD (amd-smi) GPUs
- **GPU Allocation Logic**: Tracks GPU state with JSON objects containing user, timestamps, heartbeat data, and reservation types
- **Heartbeat System**: Background goroutine sends periodic heartbeats (60s interval) to maintain run-type reservations
- **Auto-cleanup**: GPUs are automatically released when heartbeat expires (15 min timeout), manual reservations expire, or processes terminate
- **Unreserved Usage Detection**: Provider-specific integration detects GPUs in use without proper reservations
- **User Accountability**: Process ownership detection identifies which users are running unreserved processes
- **MRU-per-User Allocation**: Most Recently Used per user strategy provides GPU affinity with LRU fallback for fair distribution
- **Specific GPU Reservation**: Users can reserve exact GPU IDs (e.g., --gpu-ids 1,3) when specific hardware is needed
- **Race Condition Protection**: Redis-based distributed locking prevents allocation conflicts
- **Fair Queueing System**: FCFS (First Come First Served) queue with greedy partial allocation for first-in-queue, heartbeat-based stale entry cleanup, and queue status monitoring

## Development Commands

### Build and Installation

```bash
# Option 1: Install directly from GitHub (recommended for users)
go install github.com/russellb/canhazgpu@latest

# Option 2: Build from source
make build            # Build the Go binary to ./build/canhazgpu
make install          # Build and install to /usr/local/bin with bash completion
make test-short       # Run Go tests

# Option 3: Install from local source
go install .          # Installs to $GOPATH/bin or $HOME/go/bin

# Optional: Create short alias symlink (after installing to /usr/local/bin)
sudo ln -s /usr/local/bin/canhazgpu /usr/local/bin/chg
```

### Usage Examples
```bash
# Initialize GPU pool (auto-detects provider)
./build/canhazgpu admin --gpus 8

# Initialize with specific provider
./build/canhazgpu admin --gpus 4 --provider nvidia
./build/canhazgpu admin --gpus 2 --provider amd

# Force reinitialize with different count
./build/canhazgpu admin --gpus 4 --force

# Check status with automatic provider-specific validation
./build/canhazgpu status

# Get JSON output for programmatic use
./build/canhazgpu status --json

# Run command with GPU reservation (by count)
./build/canhazgpu run --gpus 1 -- python train.py

# Run command with specific GPU IDs
./build/canhazgpu run --gpu-ids 1,3 -- python train.py

# Run command with timeout to prevent runaway processes
./build/canhazgpu run --gpus 1 --timeout 2h -- python train.py

# Manual GPU reservation (by count)
./build/canhazgpu reserve --gpus 2 --duration 4h

# Manual reservation of specific GPU IDs
./build/canhazgpu reserve --gpu-ids 0,2,4 --duration 2h

# Release manual reservations
./build/canhazgpu release

# Generate reservation report for last 7 days
./build/canhazgpu report --days 7

# Customize memory threshold for GPU usage detection (default: 1024 MB)
./build/canhazgpu status --memory-threshold 512
./build/canhazgpu run --memory-threshold 2048 --gpus 1 -- python train.py

# Use a configuration file
./build/canhazgpu --config /path/to/config.yaml status
./build/canhazgpu --config config.json run --gpus 2 -- python train.py

# Queueing: wait for GPUs if unavailable (default behavior)
./build/canhazgpu run --gpus 4 -- python train.py  # Waits in queue until GPUs available

# Fail immediately if GPUs are unavailable (no queueing)
./build/canhazgpu run --nonblock --gpus 4 -- python train.py

# Wait up to 30 minutes for GPUs, then fail
./build/canhazgpu run --wait 30m --gpus 4 -- python train.py

# Check the reservation queue
./build/canhazgpu queue
./build/canhazgpu queue --json
```

### GPU Provider Examples
```bash
# NVIDIA GPU setup
canhazgpu admin --gpus $(nvidia-smi --list-gpus | wc -l)

# AMD GPU setup
canhazgpu admin --gpus $(amd-smi list --json | jq 'length')

# Fake GPU setup (for development/testing without real GPUs)
canhazgpu admin --gpus 4 --provider fake

# Check provider availability
nvidia-smi --help >/dev/null 2>&1 && echo "NVIDIA available" || echo "NVIDIA not available"
amd-smi --help >/dev/null 2>&1 && echo "AMD available" || echo "AMD not available"

# View cached provider information
redis-cli get "canhazgpu:provider"
```

## Dependencies

- Go 1.23+ with modules:
  - `github.com/go-redis/redis/v8`: Redis client library
  - `github.com/spf13/cobra`: CLI framework
  - `github.com/spf13/viper`: Configuration management
- System requirements: 
  - Redis server running on localhost:6379
  - **For NVIDIA GPUs**: nvidia-smi command available
  - **For AMD GPUs**: amd-smi command available (ROCm 5.7+)
  - Access to /proc filesystem or ps command for user detection

## Key Implementation Details

### Project Structure

```text
├── main.go                          # Entry point
├── internal/
│   ├── cli/                        # Cobra CLI commands
│   │   ├── root.go                 # Root command and global config
│   │   ├── admin.go                # admin command implementation
│   │   ├── status.go               # status command implementation  
│   │   ├── run.go                  # run command implementation
│   │   ├── reserve.go              # reserve command implementation
│   │   ├── release.go              # release command implementation
│   │   ├── report.go               # report command implementation
│   │   └── queue.go                # queue command implementation
│   ├── gpu/                        # GPU management logic
│   │   ├── allocation.go           # MRU-per-user allocation and coordination
│   │   ├── validation.go           # GPU usage validation and process detection
│   │   ├── model_detection.go      # AI model detection from process commands
│   │   ├── heartbeat.go            # Background heartbeat system
│   │   ├── queue_heartbeat.go      # Queue-specific heartbeat system
│   │   ├── provider.go             # GPU provider interface and manager
│   │   ├── nvidia_provider.go      # NVIDIA GPU provider implementation
│   │   ├── amd_provider.go         # AMD GPU provider implementation
│   │   └── fake_provider.go        # Fake GPU provider for development/testing
│   ├── redis_client/               # Redis operations
│   │   └── client.go               # Redis client with Lua scripts
│   ├── types/                      # Shared types and constants
│   │   └── types.go                # Config, GPUState, and other types
│   └── utils/                      # Utility functions
│       └── utils.go                # Common utility functions
├── autocomplete_canhazgpu.sh       # Custom bash completion script
└── go.mod                          # Go module definition
```

### GPU Provider System

- **Provider Interface**: `GPUProvider` interface in `internal/gpu/provider.go` defines standard operations
- **NVIDIA Provider**: `nvidia_provider.go` implements NVIDIA GPU management using nvidia-smi commands
- **AMD Provider**: `amd_provider.go` implements AMD GPU management using amd-smi commands
- **Fake Provider**: `fake_provider.go` simulates GPU behavior for development and testing without real hardware
- **Provider Manager**: `ProviderManager` in `provider.go` handles provider detection, caching, and instance management
- **Auto-detection**: Automatically detects available providers during system initialization

### GPU Validation and Detection

- `DetectAllGPUUsage()` in `internal/gpu/provider.go`: Uses provider-specific commands to query actual GPU processes and memory usage
- `GetProcessOwner()` in `internal/gpu/validation.go`: Identifies process owners via /proc filesystem or ps command
- Unreserved usage detection excludes GPUs from allocation pool automatically
- Configurable memory threshold (default: 1024 MB) determines if GPU is considered "in use" via --memory-threshold flag
- **Model Detection**: `DetectModelFromProcesses()` in `internal/gpu/model_detection.go` identifies running AI models:
  - **vLLM-specific detection**: Recognizes vLLM serve commands with positional or --model arguments
  - **Generic --model detection**: Detects `--model value` or `--model=value` patterns in any command
  - **Examples**: Supports `python train.py --model meta-llama/Llama-2-7b-chat-hf`, `inference-server --model=mistralai/Mistral-7B-Instruct-v0.1`
  - **Provider extraction**: Automatically extracts provider names (e.g., "openai", "meta-llama") from model identifiers

### Allocation Strategy

- `AtomicReserveGPUs()` in `internal/redis_client/client.go`: MRU-per-user allocation with unreserved usage exclusion and LRU fallback
- `AtomicReserveGPUs()` in `internal/redis_client/client.go`: Race-condition-safe reservation with Lua scripts
- Enhanced Redis Lua scripts validate unreserved usage list during atomic operations
- Detailed error messages when allocation fails due to unreserved usage

### Reservation Types

- **Run-type**: Maintained by heartbeat, auto-released when process ends
- **Manual-type**: Time-based expiry, explicit release required
- `LastReleased` timestamp tracking for global LRU fallback allocation
- Usage history tracking for MRU-per-user preferences  
- Support for flexible duration formats (30m, 2h, 1d) via `ParseDuration()`

### Locking and Concurrency

- Global allocation lock (`AcquireAllocationLock`, `ReleaseAllocationLock`) prevents race conditions
- Exponential backoff with jitter for lock acquisition retries
- Atomic operations using Redis Lua scripts prevent partial allocation failures
- Lock timeout of 10 seconds with up to 5 retry attempts

### Status and Monitoring

- Real-time validation shows actual vs reserved GPU usage
- User accountability displays specific users running unreserved processes
- Table format with GPU, STATUS, USER, DURATION, TYPE, MODEL, DETAILS, VALIDATION columns
- Validation info format: `XMB, Y processes` (no prefix)
- "UNRESERVED" status for unreserved usage
- Model detection displays identified AI models in MODEL column
- Provider-specific validation using appropriate GPU management tools

### Time Handling

- `FlexibleTime` type in `internal/types/types.go` handles both Unix timestamps (Python compatibility) and RFC3339 strings (Go native)
- Ensures backward compatibility with existing Python-created Redis data

### Configuration Management

- Configuration via command-line flags, configuration files, and environment variables
- Supports YAML, JSON, and TOML configuration file formats
- Default config file location: `$HOME/.canhazgpu.yaml`
- Environment variables with `CANHAZGPU_` prefix (e.g., `CANHAZGPU_MEMORY_THRESHOLD=512`)
- Priority order: CLI flags > environment variables > config file > defaults

## Redis Schema

### Core Keys

- `canhazgpu:gpu_count`: Total number of available GPUs
- `canhazgpu:provider`: Cached GPU provider type ("nvidia", "amd", or "fake")
- `canhazgpu:allocation_lock`: Global allocation lock for race condition prevention
- `canhazgpu:usage_history:{timestamp}:{user}:{gpu_id}`: Historical usage records for reporting

### Queue Keys

- `canhazgpu:queue`: Sorted set of queue entries (score = enqueue timestamp)
- `canhazgpu:queue:entry:{id}`: JSON object for each queue entry details

### GPU State Objects (`canhazgpu:gpu:{id}`)

Available state: `{'last_released': timestamp}` or `{}`

Reserved state:

```json
{
  "user": "username",
  "start_time": timestamp,
  "last_heartbeat": timestamp,
  "type": "run|manual",
  "expiry_time": timestamp  // Only for manual reservations
}
```

### Validation Integration

- Unreserved usage detection runs during allocation
- MRU-per-user allocation excludes GPUs in unreserved use
- Redis Lua scripts receive unreserved GPU lists for atomic validation
- Process ownership data enriches status display but not stored in Redis

### Reservation Tracking and Reporting

- Historical usage records automatically created when GPUs are released
- Records include user, GPU ID, start/end times, duration, and reservation type
- Usage data stored in Redis with 90-day expiration to prevent unbounded growth
- `report` command aggregates usage by user with configurable time windows
- Supports both historical completed usage and current in-progress reservations

## GPU Provider Support

### NVIDIA GPUs
- **Requirements**: nvidia-smi command available
- **Driver Support**: CUDA drivers with nvidia-smi
- **Process Detection**: Uses `nvidia-smi --query-compute-apps` for process information
- **Memory Detection**: Uses `nvidia-smi --query-gpu` for memory usage
- **Validation**: Real-time validation of GPU usage and process ownership

### AMD GPUs
- **Requirements**: amd-smi command available (ROCm 5.7+)
- **Driver Support**: ROCm drivers with amd-smi
- **Process Detection**: Uses `amd-smi process --json` for process information
- **Memory Detection**: Uses `amd-smi metric --json` for memory usage
- **Validation**: Real-time validation of GPU usage and process ownership

### Fake GPUs (Development/Testing)
- **Requirements**: None - no real GPU hardware or drivers needed
- **Use Cases**: Development on laptops, CI/CD testing, demonstrations
- **Process Detection**: Returns empty process list (no real processes)
- **Memory Detection**: Returns 0 MB usage for all GPUs
- **Validation**: All GPUs shown as available with "Fake GPU" model name
- **Setup**: `canhazgpu admin --gpus 4 --provider fake`

### Provider Selection
- **Auto-detection**: Automatically detects available providers during initialization
- **Manual Selection**: Use `--provider nvidia`, `--provider amd`, or `--provider fake`
- **Single Provider**: System assumes only one GPU provider type per system

## Documentation

- Documentation is in the docs/ directory. All user facing features should be documented.
