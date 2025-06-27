# Commands Overview

canhazgpu provides five main commands for GPU management:

```bash
❯ canhazgpu --help
Usage: canhazgpu [OPTIONS] COMMAND [ARGS]...

Commands:
  admin    Initialize GPU pool for this machine
  release  Release all manually reserved GPUs held by the current user
  reserve  Reserve GPUs manually for a specified duration
  run      Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set
  status   Show current GPU allocation status
```

## admin

Initialize and configure the GPU pool.

```bash
canhazgpu admin --gpus <count> [--force]
```

**Options:**
- `--gpus`: Number of GPUs available on this machine (required)
- `--force`: Force reinitialization even if already initialized

**Examples:**
```bash
# Initial setup
canhazgpu admin --gpus 8

# Change GPU count (requires --force)
canhazgpu admin --gpus 4 --force
```

!!! warning "Destructive Operation"
    Using `--force` will clear all existing reservations. Use with caution in production.

## status

Show current GPU allocation status with automatic validation.

```bash
canhazgpu status
```

**No options required** - validation is always enabled.

**Example Output:**
```bash
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, expires in 3h 15m 45s) [validated: no actual usage detected]
```

**Status Types:**
- `AVAILABLE`: GPU is free to use
- `IN USE`: GPU is properly reserved
- `IN USE WITHOUT RESERVATION`: GPU is being used without proper reservation

**Validation Info:**
- `[validated: XMB, Y processes]`: Confirms reservation matches actual usage
- `[validated: no actual usage detected]`: Reserved but no processes running
- `[validated: XMB used]`: Shows memory usage on available GPUs

## run

Reserve GPUs and run a command with automatic cleanup.

```bash
canhazgpu run --gpus <count> -- <command>
```

**Options:**
- `--gpus`: Number of GPUs to reserve (default: 1)

**Examples:**
```bash
# Single GPU training
canhazgpu run --gpus 1 -- python train.py

# Multi-GPU distributed training
canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py

# Complex command with arguments
canhazgpu run --gpus 1 -- python train.py --batch-size 32 --epochs 100
```

**Behavior:**
1. Validates actual GPU availability using nvidia-smi
2. Excludes GPUs that are in use without reservation
3. Reserves the requested number of GPUs using LRU allocation
4. Sets `CUDA_VISIBLE_DEVICES` to the allocated GPU IDs
5. Runs your command
6. Automatically releases GPUs when the command finishes
7. Maintains a heartbeat while running to keep the reservation active

**Error Handling:**
```bash
❯ canhazgpu run --gpus 2 -- python train.py
Error: Not enough GPUs available. Requested: 2, Available: 1 (1 GPUs in use without reservation - run 'canhazgpu status' for details)
```

## reserve

Manually reserve GPUs for a specified duration.

```bash
canhazgpu reserve [--gpus <count>] [--duration <time>]
```

**Options:**
- `--gpus`: Number of GPUs to reserve (default: 1)
- `--duration`: Duration to reserve GPUs (default: 8h)

**Duration Formats:**
- `30m`: 30 minutes
- `2h`: 2 hours
- `1d`: 1 day
- `0.5h`: 30 minutes (decimal values supported)

**Examples:**
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

**Use Cases:**
- Interactive development sessions
- Jupyter notebook workflows
- Preparing for batch jobs
- Blocking GPUs for maintenance

## release

Release all manually reserved GPUs held by the current user.

```bash
canhazgpu release
```

**No options required.**

**Examples:**
```bash
❯ canhazgpu release
Released 2 GPU(s): [1, 3]

❯ canhazgpu release  
No manually reserved GPUs found for current user
```

!!! note "Scope"
    This only releases manual reservations made with the `reserve` command. 
    It does not affect active `run` sessions.

## Command Interactions

### Validation and Conflicts

All allocation commands (`run` and `reserve`) automatically:

1. **Scan for unauthorized usage** using nvidia-smi
2. **Exclude unauthorized GPUs** from the available pool
3. **Provide detailed error messages** if insufficient GPUs remain

### LRU Allocation

When multiple GPUs are available, the system uses **Least Recently Used** allocation:

- GPUs that were released longest ago are allocated first
- Ensures fair distribution of GPU usage over time
- Helps with thermal management and hardware wear leveling

### Reservation Types

- **Run-type reservations**: Maintained by heartbeat, auto-released when process ends
- **Manual reservations**: Time-based expiry, require explicit release or timeout

### Status Integration

The `status` command shows comprehensive information about all reservation types and validates actual usage against reservations, making it easy to identify and resolve conflicts.