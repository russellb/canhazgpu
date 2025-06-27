# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is `canhazgpu`, a GPU reservation tool for single host shared development systems. It's a Go CLI application that uses Redis as a backend to coordinate GPU allocations across multiple users and processes, with comprehensive validation to detect and prevent unauthorized GPU usage.

## Architecture

The tool is a Go application structured as a CLI with internal packages that implements five main commands:
- `admin`: Initialize and configure the GPU pool with optional --force flag
- `status`: Show current GPU allocation status with automatic nvidia-smi validation
- `run`: Reserve GPU(s) and execute a command with `CUDA_VISIBLE_DEVICES` set
- `reserve`: Manually reserve GPU(s) for a specified duration 
- `release`: Release all manually reserved GPUs for the current user

### Core Components

- **Redis Integration**: Uses Redis (localhost:6379) for persistent state management with keys under `canhazgpu:` prefix
- **GPU Allocation Logic**: Tracks GPU state with JSON objects containing user, timestamps, heartbeat data, and reservation types
- **Heartbeat System**: Background goroutine sends periodic heartbeats (60s interval) to maintain run-type reservations
- **Auto-cleanup**: GPUs are automatically released when heartbeat expires (15 min timeout), manual reservations expire, or processes terminate
- **Unauthorized Usage Detection**: nvidia-smi integration detects GPUs in use without proper reservations
- **User Accountability**: Process ownership detection identifies which users are running unauthorized processes
- **LRU Allocation**: Least Recently Used strategy ensures fair GPU distribution over time
- **Race Condition Protection**: Redis-based distributed locking prevents allocation conflicts

## Development Commands

### Build and Installation
```bash
make build            # Build the Go binary to ./build/canhazgpu
make install          # Build and install to /usr/local/bin with bash completion
make test             # Run Go tests (when implemented)
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

# Manual GPU reservation
./build/canhazgpu reserve --gpus 2 --duration 4h

# Release manual reservations
./build/canhazgpu release
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
```
├── main.go                          # Entry point
├── internal/
│   ├── cli/                        # Cobra CLI commands
│   │   ├── root.go                 # Root command and global config
│   │   ├── admin.go                # admin command implementation
│   │   ├── status.go               # status command implementation  
│   │   ├── run.go                  # run command implementation
│   │   ├── reserve.go              # reserve command implementation
│   │   └── release.go              # release command implementation
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
- Unauthorized usage detection excludes GPUs from allocation pool automatically
- Memory threshold of 100MB determines if GPU is considered "in use"

### Allocation Strategy
- `GetAvailableGPUsSortedByLRU()` in `internal/gpu/allocation.go`: LRU allocation with unauthorized usage exclusion
- `AtomicReserveGPUs()` in `internal/redis_client/client.go`: Race-condition-safe reservation with Lua scripts
- Enhanced Redis Lua scripts validate unauthorized usage list during atomic operations
- Detailed error messages when allocation fails due to unauthorized usage

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
- User accountability displays specific users running unauthorized processes
- Validation info format: `[validated: XMB, Y processes]`
- "IN USE WITHOUT RESERVATION" status for unauthorized usage

### Time Handling
- `FlexibleTime` type in `internal/types/types.go` handles both Unix timestamps (Python compatibility) and RFC3339 strings (Go native)
- Ensures backward compatibility with existing Python-created Redis data

## Redis Schema

### Core Keys
- `canhazgpu:gpu_count`: Total number of available GPUs
- `canhazgpu:allocation_lock`: Global allocation lock for race condition prevention

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
- Unauthorized usage detection runs during allocation
- LRU allocation excludes GPUs in unauthorized use
- Redis Lua scripts receive unauthorized GPU lists for atomic validation
- Process ownership data enriches status display but not stored in Redis