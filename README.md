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

View current GPU allocation status:

```
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago)
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago)
GPU 2: AVAILABLE (last released 2h 45m 12s ago)
GPU 3: IN USE by bob for 1h 2m 15s (manual, expires in 3h 15m 45s)
```

The status shows:
- **AVAILABLE**: GPU is free to use, with time since last release
- **IN USE**: GPU is reserved, showing user, duration, and type:
  - `(run, ...)`: GPU reserved via `run` command with heartbeat info
  - `(manual, ...)`: GPU manually reserved with expiry time

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
- Reserves the requested number of GPUs
- Sets `CUDA_VISIBLE_DEVICES` to the allocated GPU IDs
- Runs your command
- Automatically releases GPUs when the command finishes
- Maintains a heartbeat while running to keep the reservation active

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

### Releasing Manual Reservations

Release all your manually reserved GPUs:

```bash
# Release all manual reservations for current user
canhazgpu release
```

Note: This only releases manual reservations, not active `run` sessions.

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
- Only supports NVIDIA GPUs (automatically sets `CUDA_VISIBLE_DEVICES`)

## Features

- ✅ **Race condition protection**: Uses Redis-based distributed locking
- ✅ **Manual reservations**: Reserve GPUs for specific durations
- ✅ **Automatic cleanup**: GPUs auto-released when processes end or reservations expire
- ✅ **LRU allocation**: Fair distribution using least recently used strategy
- ✅ **Heartbeat monitoring**: Detects crashed processes and reclaims GPUs
- ✅ **Flexible duration formats**: Support for minutes, hours, and days
