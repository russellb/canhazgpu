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
```

## Key Features

- ✅ **Race condition protection**: Uses Redis-based distributed locking
- ✅ **Manual reservations**: Reserve GPUs for specific durations  
- ✅ **Automatic cleanup**: GPUs auto-released when processes end or reservations expire
- ✅ **LRU allocation**: Fair distribution using least recently used strategy
- ✅ **Heartbeat monitoring**: Detects crashed processes and reclaims GPUs
- ✅ **Unauthorized usage detection**: Identifies GPUs in use without proper reservations
- ✅ **User accountability**: Shows which users are running unauthorized processes
- ✅ **Real-time validation**: Uses nvidia-smi to verify actual GPU usage
- ✅ **Smart allocation**: Automatically excludes unauthorized GPUs from allocation

## Status Display

```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, expires in 3h 15m 45s) [validated: no actual usage detected]
```

## Getting Started

1. **[Install dependencies](installation.md)** - Redis server and Python packages
2. **[Quick start guide](quickstart.md)** - Get up and running in minutes
3. **[Commands overview](commands.md)** - Learn all available commands
4. **[Administration setup](admin-setup.md)** - Configure for your environment

## Use Cases

- **ML/AI Research Teams**: Coordinate GPU access across multiple researchers
- **Shared Workstations**: Prevent conflicts on multi-GPU development machines  
- **Training Pipelines**: Ensure exclusive GPU access for long-running jobs
- **Resource Monitoring**: Track unauthorized GPU usage and enforce policies