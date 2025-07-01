# Commands Overview

canhazgpu provides seven main commands for GPU management:

```bash
❯ canhazgpu --help
Usage: canhazgpu [OPTIONS] COMMAND [ARGS]...

Commands:
  admin    Initialize GPU pool for this machine
  release  Release all manually reserved GPUs held by the current user
  report   Generate GPU usage reports
  reserve  Reserve GPUs manually for a specified duration
  run      Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set
  status   Show current GPU allocation status
  web      Start a web server for GPU status monitoring
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

**Important Note:**
Unlike the `run` command, `reserve` does NOT automatically set `CUDA_VISIBLE_DEVICES`. You must manually set it based on the GPU IDs shown in the output.

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

## report

Generate GPU reservation reports showing historical reservation patterns by user.

```bash
canhazgpu report [--days <num>]
```

**Options:**
- `--days`: Number of days to include in the report (default: 30)

**Examples:**
```bash
# Show reservations for the last 30 days (default)
canhazgpu report

# Show reservations for the last 7 days
canhazgpu report --days 7

# Show reservations for the last 24 hours
canhazgpu report --days 1
```

**Example Output:**
```bash
=== GPU Reservation Report ===
Period: 2025-05-31 to 2025-06-30 (30 days)

User                       GPU Hours      Percentage        Run     Manual
---------------------------------------------------------------------------
alice                          24.50          55.2%         12          8
bob                            15.25          34.4%          6         15
charlie                         4.60          10.4%          3          2
---------------------------------------------------------------------------
TOTAL                          44.35         100.0%         21         25

Total reservations: 46
Unique users: 3
```

**Report Features:**
- Shows GPU hours consumed by each user
- Percentage of total usage
- Breakdown by reservation type (run vs manual)
- Total statistics for the period
- Includes both completed and in-progress reservations

## web

Start a web server providing a dashboard for real-time monitoring and reports.

```bash
canhazgpu web [--port <port>] [--host <host>]
```

**Options:**
- `--port`: Port to run the web server on (default: 8080)
- `--host`: Host to bind the web server to (default: 0.0.0.0)

**Examples:**
```bash
# Start web server on default port 8080
canhazgpu web

# Start on a custom port
canhazgpu web --port 3000

# Bind to localhost only
canhazgpu web --host 127.0.0.1 --port 8080

# Run on a specific interface
canhazgpu web --host 192.168.1.100 --port 8888
```

![Web Dashboard Screenshot](images/web-screenshot.png)

The dashboard displays:
- System hostname in the header for easy identification
- GPU cards showing status, user, duration, and validation info
- Color-coded status badges (green=available, blue=in use, red=unreserved)
- Reservation report with usage statistics and visual bars
- Quick links to documentation and GitHub repository

**Dashboard Features:**
- **Real-time GPU Status**: Automatically refreshes every 30 seconds
- **Interactive Reservation Reports**: Customizable time periods (1-90 days)
- **Visual Design**: Dark theme with color-coded status indicators
- **Mobile Responsive**: Works on desktop and mobile devices
- **API Endpoints**: 
  - `/api/status` - Current GPU status as JSON
  - `/api/report?days=N` - Usage report as JSON

**Use Cases:**
- Team dashboards on shared displays
- Remote monitoring without SSH access
- Integration with monitoring systems via API
- Mobile access for on-the-go checks

## Command Interactions

### Validation and Conflicts

All allocation commands (`run` and `reserve`) automatically:

1. **Scan for unreserved usage** using nvidia-smi
2. **Exclude unreserved GPUs** from the available pool
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