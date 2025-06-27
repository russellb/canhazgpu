# canhazgpu

***A GPU reservation tool for single host shared development systems***

In shared development environments with multiple GPUs, researchers and developers often face conflicts when trying to use GPUs simultaneously, leading to out-of-memory errors, failed training runs, and wasted time debugging resource conflicts. This utility provides a simple reservation system that coordinates GPU access across multiple users and processes on a single machine.

## Quick Start

```bash
# Initialize GPU pool
canhazgpu admin --gpus 8

# Check current status
canhazgpu status

# Run vLLM with an automatic 2 GPU reservation
canhazgpu run --gpus 2 -- vllm serve my/model --tensor-parallel-size 2

# Reserve a single GPU manually for development
canhazgpu reserve --gpus 1 --duration 4h

# Release manual reservations when done
canhazgpu release
```

## Key Features

- **Race condition protection**: Uses Redis-based distributed locking
- **Automatic cleanup**: GPUs auto-released when processes end or reservations expire
- **LRU allocation**: Fair distribution using least recently used strategy
- **Unauthorized usage detection**: Identifies GPUs in use without proper reservations
- **Real-time validation**: Uses nvidia-smi to verify actual GPU usage
- **Flexible reservations**: Support for both command execution and manual reservations

## Usage Examples

### Running Commands with GPU Reservation
The most common usage - automatically reserves GPUs, runs your command, and cleans up:

```bash
# Single GPU training
canhazgpu run --gpus 1 -- python train.py

# Multi-GPU distributed training  
canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py

# Serve a model with vLLM
canhazgpu run --gpus 1 -- vllm serve microsoft/DialoGPT-medium
```

### Manual GPU Reservations
Reserve GPUs for interactive development or planned work sessions:

```bash
# Reserve 1 GPU for 8 hours (default)
canhazgpu reserve

# Reserve 2 GPUs for 4 hours
canhazgpu reserve --gpus 2 --duration 4h

# Reserve for shorter sessions
canhazgpu reserve --duration 30m  # 30 minutes
canhazgpu reserve --duration 2d   # 2 days

# Release when finished
canhazgpu release
```

### Status Monitoring
Check current GPU allocation and detect conflicts:

```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, expires in 3h 15m 45s) [validated: no actual usage detected]
```

## Documentation

For detailed usage, configuration, and administration:

**📚 [Full Documentation](http://blog.russellbryant.net/canhazgpu/)**

- **[Installation Guide](http://blog.russellbryant.net/canhazgpu/installation/)** - Setup and dependencies
- **[Quick Start](http://blog.russellbryant.net/canhazgpu/quickstart/)** - Get up and running
- **[Usage Guide](http://blog.russellbryant.net/canhazgpu/usage-run/)** - Detailed command examples
- **[Administration](http://blog.russellbryant.net/canhazgpu/admin-setup/)** - Production setup and monitoring
- **[Troubleshooting](http://blog.russellbryant.net/canhazgpu/admin-troubleshooting/)** - Common issues and solutions

## Requirements

- **Go 1.23+** (for building from source)
- **Redis server** running on localhost:6379
- **NVIDIA GPUs** with nvidia-smi available
- **System access** to `/proc` filesystem or `ps` command

## Installation

```bash
# Option 1: Build from source
git clone https://github.com/russellb/canhazgpu.git
cd canhazgpu
make install

# Option 2: Download pre-built binary (when available)
wget https://github.com/russellb/canhazgpu/releases/latest/download/canhazgpu
chmod +x canhazgpu
sudo cp canhazgpu /usr/local/bin/

# Install bash completion (optional)
wget https://raw.githubusercontent.com/russellb/canhazgpu/main/autocomplete_canhazgpu.sh
sudo cp autocomplete_canhazgpu.sh /etc/bash_completion.d/

# Initialize GPU pool
canhazgpu admin --gpus $(nvidia-smi -L | wc -l)
```

## How It Works

1. **Validation**: Uses nvidia-smi to detect actual GPU usage and identify conflicts
2. **Coordination**: Uses Redis for distributed state management and race condition prevention  
3. **Allocation**: LRU (Least Recently Used) strategy ensures fair resource distribution
4. **Monitoring**: Heartbeat system tracks active reservations and handles cleanup
5. **Enforcement**: Automatically excludes unauthorized GPU usage from allocation

## Contributing

See the [Contributing Guide](http://blog.russellbryant.net/canhazgpu/dev-contributing/) for development setup, coding standards, and how to submit contributions.
