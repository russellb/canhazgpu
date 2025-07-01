# canhazgpu

***A GPU reservation tool for single host shared development systems***

In shared development environments with multiple GPUs, researchers and developers often face conflicts when trying to use GPUs simultaneously, leading to out-of-memory errors, failed training runs, and wasted time debugging resource conflicts. This utility provides a simple reservation system that coordinates GPU access across multiple users and processes on a single machine.

### Who this is for

You peacefully share a host but want a helper to avoid accidental conflicts.

- You have a single host with NVIDIA GPUs shared by multiple users
- You all log in and run commands manually for development and/or testing
- You can still talk to each other about playing nice and sharing your (GPU) toys

### Who this is NOT for

If your needs are more than this, you probably want something more powerful like Kubernetes.

- You want to manage resources across a cluster
- You want to set resource usage limits or other policies
- You want to support workload priorities and preemption

## Quick Start

```bash
# Start Redis server listening on localhost:6379
# This is the default configuration in most cases.

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

# Generate usage reports
canhazgpu report --days 7

# Start web dashboard
canhazgpu web --port 8080
```

## Key Features

- **Race condition protection**: Uses Redis-based distributed locking
- **Automatic cleanup**: GPUs auto-released when processes end or reservations expire
- **LRU allocation**: Fair distribution using least recently used strategy
- **Unreserved usage detection**: Identifies GPUs in use without proper reservations
- **Real-time validation**: Uses nvidia-smi to verify actual GPU usage
- **Flexible reservations**: Support for both command execution and manual reservations
- **Usage reporting**: Track and analyze GPU usage patterns over time by user
- **Web dashboard**: Real-time monitoring interface with status and usage reports

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
# Option 1: Install directly from GitHub (recommended)
go install github.com/russellb/canhazgpu@latest

# Option 2: Build from source
git clone https://github.com/russellb/canhazgpu.git
cd canhazgpu
make install

# Option 3: Download pre-built binary (when available)
wget https://github.com/russellb/canhazgpu/releases/latest/download/canhazgpu
chmod +x canhazgpu
sudo cp canhazgpu /usr/local/bin/

# Install bash completion (optional but recommended)
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
5. **Enforcement**: Automatically excludes unreserved GPU usage from allocation

## Contributing

See the [Contributing Guide](http://blog.russellbryant.net/canhazgpu/dev-contributing/) for development setup, coding standards, and how to submit contributions.
