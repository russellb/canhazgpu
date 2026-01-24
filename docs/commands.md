# Commands Overview

canhazgpu provides seven main commands for GPU management:

```bash
❯ canhazgpu --help
Usage: canhazgpu [OPTIONS] COMMAND [ARGS]...

Commands:
  admin    Initialize GPU pool for this machine
  release  Release manually reserved GPUs held by the current user
  report   Generate GPU usage reports
  reserve  Reserve GPUs manually for a specified duration
  run      Reserve GPUs and run a command with CUDA_VISIBLE_DEVICES set
  status   Show current GPU allocation status
  web      Start a web server for GPU status monitoring
```

## Global Flags

All commands support these global configuration flags:

- `--config`: Path to configuration file (default: `$HOME/.canhazgpu.yaml`)
- `--redis-host`: Redis server hostname (default: localhost)
- `--redis-port`: Redis server port (default: 6379)
- `--redis-db`: Redis database number (default: 0)
- `--memory-threshold`: Memory threshold in MB to consider a GPU as "in use" (default: 1024)

**Configuration Methods:**

1. **Command-line flags** (highest priority)
2. **Environment variables** with `CANHAZGPU_` prefix
3. **Configuration files** in YAML, JSON, or TOML format
4. **Built-in defaults** (lowest priority)

**Examples:**
```bash
# Use command-line flags
canhazgpu status --redis-host redis.example.com --redis-port 6380

# Use environment variables
export CANHAZGPU_MEMORY_THRESHOLD=512
export CANHAZGPU_REDIS_HOST=redis.example.com
canhazgpu status

# Use a configuration file
canhazgpu --config /path/to/config.yaml status
canhazgpu --config config.json run --gpus 2 -- python train.py
```

**Configuration File Examples:**

YAML format (`~/.canhazgpu.yaml`):
```yaml
redis:
  host: redis.example.com
  port: 6379
  db: 0
memory:
  threshold: 512
```

JSON format:
```json
{
  "redis": {
    "host": "redis.example.com",
    "port": 6379,
    "db": 0
  },
  "memory": {
    "threshold": 512
  }
}
```

TOML format:
```toml
[redis]
host = "redis.example.com"
port = 6379
db = 0

[memory]
threshold = 512
```

The `--memory-threshold` flag controls when a GPU is considered "in use without reservation". GPUs using more than this amount of memory will be excluded from allocation and flagged as unreserved usage.

## admin

Initialize and configure the GPU pool.

```bash
canhazgpu admin --gpus <count> [--force] [--provider <type>]
```

**Options:**
- `--gpus`: Number of GPUs available on this machine (required)
- `--force`: Force reinitialization even if already initialized
- `--provider`: GPU provider type (`nvidia`, `amd`, or `fake`). Auto-detected if not specified.

**Examples:**
```bash
# Initial setup (auto-detects provider)
canhazgpu admin --gpus 8

# Explicitly specify NVIDIA provider
canhazgpu admin --gpus 8 --provider nvidia

# Use AMD GPUs
canhazgpu admin --gpus 4 --provider amd

# Use fake provider for development/testing (no real GPUs required)
canhazgpu admin --gpus 4 --provider fake

# Change GPU count (requires --force)
canhazgpu admin --gpus 4 --force
```

!!! tip "Fake Provider for Development"
    Use `--provider fake` to develop and test canhazgpu on systems without actual GPUs.
    The fake provider simulates GPU behavior without requiring nvidia-smi or amd-smi.

!!! warning "Destructive Operation"
    Using `--force` will clear all existing reservations. Use with caution in production.

## status

Show current GPU allocation status with automatic validation.

```bash
# Table output (default)
canhazgpu status

# JSON output for programmatic use
canhazgpu status --json
canhazgpu status -j
```

**Options:**
- `-j, --json`: Output status as JSON array instead of table format

**[→ Detailed Status Guide](usage-status.md)**

**Examples:**
```bash
# Standard status check
canhazgpu status

# JSON output for scripts and APIs
canhazgpu status --json

# Use a lower threshold to detect lighter GPU usage
canhazgpu status --memory-threshold 512

# Use a higher threshold to ignore small allocations
canhazgpu status --memory-threshold 2048

# Combine JSON with memory threshold
canhazgpu status --json --memory-threshold 512
```

!!! note "Global Memory Threshold"
    The `--memory-threshold` flag is a global option that affects GPU usage detection across all commands. It can be set in your [configuration file](configuration.md) or used with any command that performs GPU validation.

**Table Output Example:**
```bash
GPU  STATUS      USER      DURATION     TYPE    MODEL                    DETAILS                   VALIDATION
---  ------      ----      --------     ----    -----                    -------                   ----------
0    AVAILABLE   -         -            -       -                        free for 0h 30m 15s      45MB used
1    IN_USE      alice     0h 15m 30s   RUN     meta-llama/Llama-2-7b-chat-hf  heartbeat 0h 0m 5s ago    8452MB, 1 processes
2    UNRESERVED  user bob  -            -       codellama/CodeLlama-7b-Instruct-hf        1024MB used by PID 12345 (python3), PID 67890 (jupyter)  -
3    IN_USE      charlie   1h 2m 15s    MANUAL  -                        expires in 3h 15m 45s    no usage detected
```

**JSON Output Example:**
```json
[
  {
    "gpu_id": 0,
    "status": "AVAILABLE",
    "details": "free for 0h 30m 15s",
    "validation": "45MB used"
  },
  {
    "gpu_id": 1,
    "status": "IN_USE",
    "user": "alice",
    "duration": "0h 15m 30s",
    "type": "RUN",
    "details": "heartbeat 0h 0m 5s ago",
    "validation": "8452MB, 1 processes",
    "model": {
      "provider": "meta-llama",
      "model": "meta-llama/Llama-2-7b-chat-hf"
    }
  },
  {
    "gpu_id": 2,
    "status": "UNRESERVED",
    "details": "WITHOUT RESERVATION",
    "unreserved_users": ["bob"],
    "process_info": "1024MB used by PID 12345 (python3), PID 67890 (jupyter)",
    "model": {
      "provider": "codellama",
      "model": "codellama/CodeLlama-7b-Instruct-hf"
    }
  },
  {
    "gpu_id": 3,
    "status": "IN_USE",
    "user": "charlie",
    "duration": "1h 2m 15s",
    "type": "MANUAL",
    "details": "expires in 3h 15m 45s",
    "validation": "no usage detected"
  }
]
```

**Status Types:**
- `AVAILABLE`: GPU is free to use
- `IN_USE`: GPU is properly reserved
- `UNRESERVED`: GPU is being used without proper reservation

**Table Columns:**
- `GPU`: GPU ID number
- `STATUS`: Current state (AVAILABLE, IN_USE, UNRESERVED, ERROR)
- `USER`: Username who reserved the GPU (or who is using it unreserved)
- `DURATION`: How long the GPU has been reserved
- `TYPE`: Reservation type (RUN, MANUAL)
- `MODEL`: Detected AI model (if any)
- `DETAILS`: Additional information (heartbeat, expiry, process info)
- `VALIDATION`: Actual GPU usage validation (memory, process count)

## run

Reserve GPUs and run a command with automatic cleanup.

```bash
canhazgpu run [--gpus <count> | --gpu-ids <ids>] [--timeout <duration>] -- <command>
```

**[→ Detailed Run Guide](usage-run.md)**

**Options:**
- `--gpus`: Number of GPUs to reserve (default: 1)
- `--gpu-ids`: Specific GPU IDs to reserve (comma-separated, e.g., 1,3,5)
- `--timeout`: Maximum time to run command before killing it (default: none)

!!! note "GPU Selection Options"
    You can use `--gpus` alone, `--gpu-ids` alone, or both together if:
    - `--gpus` matches the number of GPU IDs specified, or
    - `--gpus` is 1 (the default value)
    
    If specific GPU IDs are requested and any are not available, the entire reservation will fail.

**Timeout formats:**
- `30s` (30 seconds)
- `30m` (30 minutes)
- `2h` (2 hours)  
- `1d` (1 day)
- `0.5h` (30 minutes with decimal)

**Examples:**
```bash
# Single GPU training
canhazgpu run --gpus 1 -- python train.py

# Multi-GPU distributed training
canhazgpu run --gpus 2 -- python -m torch.distributed.launch train.py

# Reserve specific GPU IDs
canhazgpu run --gpu-ids 1,3 -- python train.py

# Complex command with arguments
canhazgpu run --gpus 1 -- python train.py --batch-size 32 --epochs 100

# Training with timeout to prevent runaway processes
canhazgpu run --gpus 1 --timeout 2h -- python train.py

# Short timeout for testing
canhazgpu run --gpus 1 --timeout 30m -- python test_model.py
```

**Behavior:**
1. Validates actual GPU availability using nvidia-smi
2. Excludes GPUs that are in use without reservation
3. Reserves the requested number of GPUs using MRU-per-user allocation (with LRU fallback)
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
canhazgpu reserve [--gpus <count> | --gpu-ids <ids>] [--duration <time>]
```

**[→ Detailed Reserve Guide](usage-reserve.md)**

**Options:**
- `--gpus`: Number of GPUs to reserve (default: 1)
- `--gpu-ids`: Specific GPU IDs to reserve (comma-separated, e.g., 1,3,5)
- `--duration`: Duration to reserve GPUs (default: 8h)

!!! note "GPU Selection Options"
    You can use `--gpus` alone, `--gpu-ids` alone, or both together if:
    - `--gpus` matches the number of GPU IDs specified, or
    - `--gpus` is 1 (the default value)
    
    If specific GPU IDs are requested and any are not available, the entire reservation will fail.

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

# Reserve specific GPU IDs
canhazgpu reserve --gpu-ids 0,2 --duration 2h

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

Release manually reserved GPUs held by the current user.

```bash
canhazgpu release [--gpu-ids <ids>]
```

**[→ Detailed Release Guide](usage-release.md)**

**Options:**
- `-G, --gpu-ids`: Specific GPU IDs to release (comma-separated, e.g., 1,3,5)

**Examples:**
```bash
# Release all manually reserved GPUs
❯ canhazgpu release
Released 2 GPU(s): [1, 3]

# Release specific GPUs
❯ canhazgpu release --gpu-ids 1,3
Released 2 GPU(s): [1, 3]

❯ canhazgpu release  
No manually reserved GPUs found for current user
```

!!! note "Scope"
    By default, releases all manually reserved GPUs. With `--gpu-ids`, can release specific GPUs including both manual reservations (from `reserve` command) and run-type reservations (from `run` command).

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
canhazgpu web [--port <port>] [--host <host>] [--demo]
```

**Options:**
- `--port, -p`: Port to run the web server on (default: 8080)
- `--host`: Host to bind the web server to (default: 0.0.0.0)
- `--demo`: Run in demo mode with simulated data (no Redis required)

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

# Run in demo mode for testing
canhazgpu web --demo
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
- **Visual Design**: Dark/light theme toggle with color-coded status indicators
- **Mobile Responsive**: Works on desktop and mobile devices
- **Multi-Host Support**: View all configured remote hosts in one dashboard
- **API Endpoints**:
  - `/api/status` - Current GPU status as JSON
  - `/api/hosts` - List of configured hosts
  - `/api/hosts/status` - Status for all hosts (multi-host view)
  - `/api/hosts/status?host=<name>` - Status for a specific host
  - `/api/report?days=N` - Usage report as JSON

### Multi-Host Support

When remote hosts are configured in `~/.canhazgpu.yaml`, the web dashboard automatically enables a multi-host view:

```yaml
# ~/.canhazgpu.yaml
remote_hosts:
  - gpu-server-1
  - gpu-server-2
  - workstation-3
```

**Multi-Host Features:**
- **Hosts Overview**: Summary cards showing GPU availability for each host
- **Click to Expand**: Select a host to view detailed GPU status
- **Parallel Fetching**: Status is fetched from all hosts concurrently
- **Error Handling**: Failed hosts show error messages without blocking others
- **Graceful Degradation**: If localhost Redis is unavailable but remote hosts are configured, the dashboard continues with remote hosts only

**Single-Host Behavior:**
When no remote hosts are configured, the dashboard shows the traditional single-host view with GPU cards and reservation reports.

**Use Cases:**
- Team dashboards on shared displays
- Remote monitoring without SSH access
- Multi-system GPU cluster monitoring
- Integration with monitoring systems via API
- Mobile access for on-the-go checks

## Command Interactions

### Validation and Conflicts

All allocation commands (`run` and `reserve`) automatically:

1. **Scan for unreserved usage** using nvidia-smi
2. **Exclude unreserved GPUs** from the available pool
3. **Provide detailed error messages** if insufficient GPUs remain

### MRU-per-User Allocation

When multiple GPUs are available, the system uses **Most Recently Used per User** allocation:

- Prioritizes GPUs that you have used most recently (based on your usage history)
- Falls back to global LRU (Least Recently Used) for GPUs you haven't used
- Provides GPU affinity for better cache locality and workflow continuity
- Ensures fair distribution across all users while respecting individual preferences

### Reservation Types

- **Run-type reservations**: Maintained by heartbeat, auto-released when process ends
- **Manual reservations**: Time-based expiry, require explicit release or timeout

### Status Integration

The `status` command shows comprehensive information about all reservation types and validates actual usage against reservations, making it easy to identify and resolve conflicts.