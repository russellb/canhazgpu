# canhazgpu

***A GPU reservation tool for single host shared development systems***

In shared development environments with multiple GPUs, researchers and developers often face conflicts when trying to use GPUs simultaneously, leading to out-of-memory errors, failed training runs, and wasted time debugging resource conflicts. This utility provides a simple reservation system that coordinates GPU access across multiple users and processes on a single machine, ensuring exclusive access to requested GPUs while automatically handling cleanup when jobs complete or crash, thus eliminating the frustration of "GPU already in use" errors and enabling efficient collaborative development.

## Quick Example

```bash
# Initialize GPU pool
canhazgpu admin --gpus 8

# Check status 
canhazgpu status

# Run a training job with 2 GPUs
canhazgpu run --gpus 2 -- python train.py

# Reserve GPUs manually for 4 hours
canhazgpu reserve --gpus 1 --duration 4h

# Release manual reservations
canhazgpu release

# View usage reports
canhazgpu report --days 7

# Start web dashboard
canhazgpu web --port 8080
```

## Key Features

- ✅ **Race condition protection**: Uses Redis-based distributed locking
- ✅ **Manual reservations**: Reserve GPUs for specific durations  
- ✅ **Automatic cleanup**: GPUs auto-released when processes end or reservations expire
- ✅ **LRU allocation**: Fair distribution using least recently used strategy
- ✅ **Heartbeat monitoring**: Detects crashed processes and reclaims GPUs
- ✅ **Unreserved usage detection**: Identifies GPUs in use without proper reservations
- ✅ **User accountability**: Shows which users are running unreserved processes
- ✅ **Real-time validation**: Uses nvidia-smi to verify actual GPU usage
- ✅ **Smart allocation**: Automatically excludes unreserved GPUs from allocation
- ✅ **Usage reporting**: Track and analyze GPU usage patterns over time
- ✅ **Web dashboard**: Real-time monitoring interface with status and reports

## Status Display

```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, expires in 3h 15m 45s) [validated: no actual usage detected]
```

## Getting Started

1. **[Install dependencies](installation.md)** - Redis server and Go
2. **[Quick start guide](quickstart.md)** - Get up and running in minutes  
3. **[Configuration](configuration.md)** - Set defaults and customize behavior
4. **[Commands overview](commands.md)** - Learn all available commands
5. **[Administration setup](installation.md)** - Configure for your environment

## Use Cases

- **ML/AI Research Teams**: Coordinate GPU access across multiple researchers
- **Shared Workstations**: Prevent conflicts on multi-GPU development machines  
- **Training Pipelines**: Ensure exclusive GPU access for long-running jobs
- **Resource Monitoring**: Track unreserved GPU usage and enforce policies

## Documentation

### User Guides
- **[Installation](installation.md)** - Install dependencies
- **[Quick Start](quickstart.md)** - Get started in minutes
- **[Configuration](configuration.md)** - Configure defaults and customize behavior
- **[Commands Overview](commands.md)** - All available commands

### Detailed Usage
- **[Running Jobs](usage-run.md)** - GPU reservation with run command
- **[Manual Reservations](usage-reserve.md)** - Reserve GPUs manually  
- **[Status Monitoring](usage-status.md)** - Monitor GPU usage and reservations

### Key Features
- **[GPU Validation](features-validation.md)** - Real-time usage validation
- **[Unreserved Detection](features-unreserved.md)** - Find unauthorized GPU usage
- **[LRU Allocation](features-lru.md)** - Fair GPU distribution strategy

### Administration
- **[Installation Guide](installation.md)** - Dependencies and installation
- **[Troubleshooting](admin-troubleshooting.md)** - Common issues and solutions

### Development
- **[Architecture](dev-architecture.md)** - System design overview
- **[Contributing](dev-contributing.md)** - Contribution guidelines
- **[Testing](dev-testing.md)** - Testing procedures
- **[Release Process](dev-release.md)** - Release management