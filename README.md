# canhazgpu

***An ok-ish GPU reservation tool for single host shared development systems***

In shared development environments with multiple GPUs, researchers and
developers often face conflicts when trying to use GPUs simultaneously, leading
to out-of-memory errors, failed training runs, and wasted time debugging
resource conflicts. This utility provides a simple reservation system that
coordinates GPU access across multiple users and processes on a single machine,
ensuring exclusive access to requested GPUs while automatically handling cleanup
when jobs complete or crash, thus eliminating the frustration of "GPU already in
use" errors and enabling efficient collaborative development.

## Usage Overview

```
❯ canhazgpu
Usage: canhazgpu [OPTIONS] COMMAND [ARGS]...

Options:
  --help  Show this message and exit.

Commands:
  admin    Initialize GPU pool for this machine
  release  Release all manually reserved GPUs held by the current user
  reserve  Reserve GPUs manually for a specified duration
  run      Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set
  status   Show current GPU allocation status
```

## Getting Started

First, initialize the system to tell it how many GPUs to track:

```
❯ canhazgpu admin --gpus 8
Initialized 8 GPUs (IDs 0 to 7)
```

If already initialized, you can change the number of GPUs with `--force`:

```
❯ canhazgpu admin --gpus 4 --force
Force reinitializing: clearing 8 existing GPUs...
Reinitialized 4 GPUs (IDs 0 to 3)
```

## Checking Status

View current GPU allocation status with automatic validation:

```
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, expires in 3h 15m 45s) [validated: no actual usage detected]
```

The status shows:
- **AVAILABLE**: GPU is free to use, with time since last release
- **IN USE**: GPU is properly reserved, showing user, duration, and type:
  - `(run, ...)`: GPU reserved via `run` command with heartbeat info
  - `(manual, ...)`: GPU manually reserved with expiry time
- **IN USE WITHOUT RESERVATION**: GPU is being used without proper reservation, showing:
  - Which user(s) are running unauthorized processes
  - Memory usage and process details
  - Suggests running `canhazgpu status` for full details
- **Validation info**: Shows actual GPU usage detected via nvidia-smi:
  - `[validated: XMB, Y processes]`: Confirms reservation matches actual usage
  - `[validated: no actual usage detected]`: Reserved but no processes running
  - `[validated: XMB used]`: Shows memory usage on "available" GPUs

## Usage Modes

### Running Commands with GPU Reservation

Reserve GPUs and run a command with `CUDA_VISIBLE_DEVICES` automatically set:

```bash
# Reserve 1 GPU and run a command
canhazgpu run --gpus 1 -- python train.py

# Reserve 2 GPUs for multi-GPU training
canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py

# Run vllm with a single GPU
canhazgpu run --gpus 1 -- vllm serve my/model
```

The `run` command:
- Validates actual GPU availability using nvidia-smi
- Excludes GPUs that are in use without reservation
- Reserves the requested number of GPUs using LRU allocation
- Sets `CUDA_VISIBLE_DEVICES` to the allocated GPU IDs
- Runs your command
- Automatically releases GPUs when the command finishes
- Maintains a heartbeat while running to keep the reservation active

If allocation fails due to unauthorized usage:
```bash
❯ canhazgpu run --gpus 2 -- python train.py
Error: Not enough GPUs available. Requested: 2, Available: 1 (1 GPUs in use without reservation - run 'canhazgpu status' for details)
```

### Manual GPU Reservation

Reserve GPUs for a specific duration without running a command:

```bash
# Reserve 1 GPU for 8 hours (default)
canhazgpu reserve

# Reserve 2 GPUs for 4 hours
canhazgpu reserve --gpus 2 --duration 4h

# Reserve 1 GPU for 30 minutes
canhazgpu reserve --duration 30m

# Reserve 1 GPU for 2 days
canhazgpu reserve --gpus 1 --duration 2d
```

Duration formats supported:
- `30m` - 30 minutes
- `2h` - 2 hours  
- `1d` - 1 day
- `0.5h` - 30 minutes (decimal values supported)

The `reserve` command also validates GPU availability and excludes unauthorized usage:
```bash
❯ canhazgpu reserve --gpus 2 --duration 4h
Error: Not enough GPUs available. Requested: 2, Available: 1 (1 GPUs in use without reservation - run 'canhazgpu status' for details)
```

### Releasing Manual Reservations

Release all your manually reserved GPUs:

```bash
# Release all manual reservations for current user
canhazgpu release
```

Note: This only releases manual reservations, not active `run` sessions.

## Unauthorized Usage Detection

The system automatically detects and prevents allocation of GPUs that are in use without proper reservations:

### Detection Methods
- **nvidia-smi integration**: Queries actual GPU processes and memory usage
- **Process ownership**: Identifies which users are running unauthorized processes
- **Real-time validation**: Checks during every allocation attempt
- **Memory threshold**: Considers GPUs with >100MB usage as "in use"

### Status Display Examples

**Single unauthorized user:**
```bash
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
```

**Multiple unauthorized users:**
```bash
GPU 3: IN USE WITHOUT RESERVATION by users alice, bob and charlie - 2048MB used by PID 12345 (python3), PID 23456 (pytorch) and 2 more
```

**Validation mismatch (reserved but unused):**
```bash
GPU 4: IN USE by alice for 1h 30m 0s (manual, expires in 2h 30m 0s) [validated: no actual usage detected]
```

### Allocation Protection
When requesting GPUs, the system:
1. Scans for unauthorized usage using nvidia-smi
2. Excludes those GPUs from the available pool
3. Provides detailed error messages if insufficient GPUs remain
4. Suggests running `canhazgpu status` to see unauthorized usage details

## GPU Allocation Strategy

The system uses a **Least Recently Used (LRU)** allocation strategy:

- When multiple GPUs are available, it selects the GPU(s) that were released longest ago
- This ensures fair distribution of GPU usage over time
- Helps with thermal management and hardware wear leveling
- Tracks `last_released` timestamp for each GPU

For example, if you request 2 GPUs and GPUs 1, 2, and 5 are available:
- GPU 1 was last released 3 hours ago
- GPU 2 was last released 1 hour ago  
- GPU 5 was last released 2 hours ago

The system will allocate GPUs 1 and 5 (oldest releases first).

## Requirements

- Python 3.6+
- `redis` Python library (`pip install redis`)
- `click` Python library (`pip install click`)
- Redis server running on localhost:6379
- NVIDIA GPUs with nvidia-smi available (automatically sets `CUDA_VISIBLE_DEVICES`)
- For user detection: access to `/proc` filesystem or `ps` command

## Features

- ✅ **Race condition protection**: Uses Redis-based distributed locking
- ✅ **Manual reservations**: Reserve GPUs for specific durations
- ✅ **Automatic cleanup**: GPUs auto-released when processes end or reservations expire
- ✅ **LRU allocation**: Fair distribution using least recently used strategy
- ✅ **Heartbeat monitoring**: Detects crashed processes and reclaims GPUs
- ✅ **Flexible duration formats**: Support for minutes, hours, and days
- ✅ **Unauthorized usage detection**: Identifies GPUs in use without proper reservations
- ✅ **User accountability**: Shows which users are running unauthorized processes
- ✅ **Real-time validation**: Uses nvidia-smi to verify actual GPU usage
- ✅ **Smart allocation**: Automatically excludes unauthorized GPUs from allocation
- ✅ **Detailed error messages**: Clear feedback when GPUs unavailable due to unauthorized usage
