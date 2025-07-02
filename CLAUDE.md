# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is `canhazgpu`, a GPU reservation tool for single host shared development systems. It's a Go CLI application that uses Redis as a backend to coordinate GPU allocations across multiple users and processes, with comprehensive validation to detect and prevent unreserved GPU usage.

## Architecture

The tool is a Go application structured as a CLI with internal packages that implements seven main commands:
- `admin`: Initialize and configure the GPU pool with optional --force flag
- `status`: Show current GPU allocation status with automatic nvidia-smi validation
- `run`: Reserve GPU(s) and execute a command with `CUDA_VISIBLE_DEVICES` set
- `reserve`: Manually reserve GPU(s) for a specified duration 
- `release`: Release all manually reserved GPUs for the current user
- `report`: Generate GPU reservation reports showing historical reservation patterns by user
- `web`: Start a web server providing a dashboard for real-time monitoring and reports

### Core Components

- **Redis Integration**: Uses Redis (localhost:6379) for persistent state management with keys under `canhazgpu:` prefix
- **GPU Allocation Logic**: Tracks GPU state with JSON objects containing user, timestamps, heartbeat data, and reservation types
- **Heartbeat System**: Background goroutine sends periodic heartbeats (60s interval) to maintain run-type reservations
- **Auto-cleanup**: GPUs are automatically released when heartbeat expires (15 min timeout), manual reservations expire, or processes terminate
- **Unreserved Usage Detection**: nvidia-smi integration detects GPUs in use without proper reservations
- **User Accountability**: Process ownership detection identifies which users are running unreserved processes
- **LRU Allocation**: Least Recently Used strategy ensures fair GPU distribution over time
- **Race Condition Protection**: Redis-based distributed locking prevents allocation conflicts

## Development Commands

### Build and Installation

```bash
# Option 1: Install directly from GitHub (recommended for users)
go install github.com/russellb/canhazgpu@latest

# Option 2: Build from source
make build            # Build the Go binary to ./build/canhazgpu
make install          # Build and install to /usr/local/bin with bash completion
make test             # Run Go tests (when implemented)

# Option 3: Install from local source
go install .          # Installs to $GOPATH/bin or $HOME/go/bin
```

### Usage Examples
```bash
# Initialize GPU pool
./build/canhazgpu admin --gpus 8

# Force reinitialize with different count
./build/canhazgpu admin --gpus 4 --force

# Check status with automatic validation
./build/canhazgpu status

# Run command with GPU reservation
./build/canhazgpu run --gpus 1 -- python train.py

# Run command with timeout to prevent runaway processes
./build/canhazgpu run --gpus 1 --timeout 2h -- python train.py

# Manual GPU reservation
./build/canhazgpu reserve --gpus 2 --duration 4h

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
```

## Dependencies

- Go 1.23+ with modules:
  - `github.com/go-redis/redis/v8`: Redis client library
  - `github.com/spf13/cobra`: CLI framework
  - `github.com/spf13/viper`: Configuration management
- System requirements: 
  - Redis server running on localhost:6379
  - nvidia-smi command available for GPU validation
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
│   │   └── report.go               # report command implementation
│   ├── gpu/                        # GPU management logic
│   │   ├── allocation.go           # LRU allocation and coordination
│   │   ├── validation.go           # nvidia-smi integration and usage detection
│   │   └── heartbeat.go            # Background heartbeat system
│   ├── redis_client/               # Redis operations
│   │   └── client.go               # Redis client with Lua scripts
│   └── types/                      # Shared types and constants
│       └── types.go                # Config, GPUState, and other types
├── autocomplete_canhazgpu.sh       # Custom bash completion script
└── go.mod                          # Go module definition
```

### GPU Validation and Detection

- `DetectGPUUsage()` in `internal/gpu/validation.go`: Uses nvidia-smi to query actual GPU processes and memory usage
- `GetProcessOwner()` in `internal/gpu/validation.go`: Identifies process owners via /proc filesystem or ps command
- Unreserved usage detection excludes GPUs from allocation pool automatically
- Configurable memory threshold (default: 1024 MB) determines if GPU is considered "in use" via --memory-threshold flag

### Allocation Strategy

- `GetAvailableGPUsSortedByLRU()` in `internal/gpu/allocation.go`: LRU allocation with unreserved usage exclusion
- `AtomicReserveGPUs()` in `internal/redis_client/client.go`: Race-condition-safe reservation with Lua scripts
- Enhanced Redis Lua scripts validate unreserved usage list during atomic operations
- Detailed error messages when allocation fails due to unreserved usage

### Reservation Types

- **Run-type**: Maintained by heartbeat, auto-released when process ends
- **Manual-type**: Time-based expiry, explicit release required
- `LastReleased` timestamp tracking for LRU allocation decisions  
- Support for flexible duration formats (30m, 2h, 1d) via `ParseDuration()`

### Locking and Concurrency

- Global allocation lock (`AcquireAllocationLock`, `ReleaseAllocationLock`) prevents race conditions
- Exponential backoff with jitter for lock acquisition retries
- Atomic operations using Redis Lua scripts prevent partial allocation failures
- Lock timeout of 10 seconds with up to 5 retry attempts

### Status and Monitoring

- Real-time validation shows actual vs reserved GPU usage
- User accountability displays specific users running unreserved processes
- Validation info format: `[validated: XMB, Y processes]`
- "IN USE WITHOUT RESERVATION" status for unreserved usage

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
- `canhazgpu:allocation_lock`: Global allocation lock for race condition prevention
- `canhazgpu:usage_history:{timestamp}:{user}:{gpu_id}`: Historical usage records for reporting

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
- LRU allocation excludes GPUs in unreserved use
- Redis Lua scripts receive unreserved GPU lists for atomic validation
- Process ownership data enriches status display but not stored in Redis

### Reservation Tracking and Reporting

- Historical usage records automatically created when GPUs are released
- Records include user, GPU ID, start/end times, duration, and reservation type
- Usage data stored in Redis with 90-day expiration to prevent unbounded growth
- `report` command aggregates usage by user with configurable time windows
- Supports both historical completed usage and current in-progress reservations
