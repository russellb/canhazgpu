# GPU Validation

canhazgpu integrates with nvidia-smi to provide real-time validation of GPU usage, ensuring that reservations match actual resource utilization and detecting unreserved usage.

## How Validation Works

### nvidia-smi Integration
The system uses nvidia-smi to query actual GPU processes and memory usage:

```bash
# canhazgpu internally runs commands like:
nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits
nvidia-smi --query-compute-apps=pid,process_name,gpu_uuid,used_memory --format=csv,noheader
```

### Process Owner Detection
For each GPU process, canhazgpu identifies the owner:

1. **Primary method**: Read `/proc/{pid}/status` to get the UID
2. **Fallback method**: Use `ps -o uid= -p {pid}` if `/proc` is unavailable
3. **Username resolution**: Convert UID to username using system user database

### Memory Threshold
GPUs with more than the configured memory threshold are considered "in use" (default: **1GB = 1024 MB**):

- **Below threshold**: Baseline GPU driver usage, considered available
- **Above threshold**: Active workload detected, GPU marked as in use

The threshold can be customized using the `--memory-threshold` flag:

```bash
# Use a lower threshold (512 MB) to detect lighter GPU usage
canhazgpu status --memory-threshold 512

# Use a higher threshold (2 GB) to ignore smaller allocations
canhazgpu status --memory-threshold 2048

# Apply to allocation commands as well
canhazgpu run --memory-threshold 512 --gpus 1 -- python train.py
```

## Validation Output

### Status Display
The validation information appears in brackets in the status output:

```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, ...) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, ...) [validated: no actual usage detected]
```

### Validation States

#### Normal Available GPU
```bash
[validated: 45MB used]
```
- Low memory usage indicates GPU is available
- Only driver baseline memory consumption
- Safe to allocate

#### Confirmed Reservation Usage
```bash
[validated: 8452MB, 1 processes]
```
- High memory usage confirms GPU is actively used
- Process count shows number of applications using GPU
- Validates that reservation matches actual usage

#### No Usage Detected
```bash
[validated: no actual usage detected]
```
- GPU is reserved but not actually being used
- Could indicate:
  - Preparation phase (normal)
  - Finished work but reservation not released
  - Stale reservation (should be cleaned up)

#### Unauthorized Usage Detail
```bash
by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
```
- Shows specific user running unreserved processes
- Lists PIDs and process names
- Memory usage quantifies the unreserved resource consumption

## Validation Benefits

### Prevents Double Allocation
Without validation, you might have scenarios like:
- GPU 1 is "AVAILABLE" according to Redis
- But someone is actually using GPU 1 without reservation
- New reservation could conflict with existing usage

With validation:
- GPU 1 would show "IN USE WITHOUT RESERVATION"
- GPU 1 is automatically excluded from allocation
- Prevents conflicts and out-of-memory errors

### User Accountability
```bash
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3)
GPU 5: IN USE WITHOUT RESERVATION by user charlie - 8GB used by PID 23456 (jupyter)
```

Clear identification of which users need to be contacted about policy compliance.

### Resource Optimization
```bash
GPU 3: IN USE by alice for 8h 0m 0s (manual, expires in 0h 30m 0s) [validated: no actual usage detected]
```

Identifies stale reservations that could be released early to improve resource availability.

## Validation in Allocation

### Pre-Allocation Scanning
Before any GPU allocation, canhazgpu:

1. **Scans all GPUs** using nvidia-smi
2. **Identifies unreserved usage** and excludes those GPUs
3. **Updates available GPU pool** with only truly available GPUs
4. **Proceeds with allocation** using the validated pool

### Error Messages with Context
```bash
❯ canhazgpu run --gpus 3 -- python train.py
Error: Not enough GPUs available. Requested: 3, Available: 1 (2 GPUs in use without reservation - run 'canhazgpu status' for details)
```

The error message indicates:
- You requested 3 GPUs
- Only 1 is actually available
- 2 GPUs are being used without proper reservations
- Suggests checking status for more details

## Validation Edge Cases

### Multiple Users Per GPU
```bash
GPU 4: IN USE WITHOUT RESERVATION by users alice, bob and charlie - 2048MB used by PID 12345 (python3), PID 23456 (pytorch) and 2 more
```

When multiple users have processes on the same GPU:
- All usernames are listed
- Process details may be truncated for readability
- Total memory usage is shown

### Process Information Limitations
Sometimes process details may be limited:
```bash
by user unknown - 1024MB used by PID 12345 (unknown process)
```

This can happen when:
- Process owner cannot be determined (permission issues)
- Process name is not available
- Process terminates between detection and information gathering

### High Memory Baseline
```bash
[validated: 850MB used]
```

Some systems may have higher baseline GPU memory usage due to:
- Multiple GPU monitoring tools
- Persistent CUDA contexts
- Background ML services

The 1GB threshold can be adjusted if needed (currently hardcoded).

## Validation Performance

### Caching and Efficiency
- Validation runs only during allocation and status commands
- nvidia-smi queries are batched for efficiency
- Process information is gathered in parallel where possible

### Impact on System Performance
- Minimal overhead: validation takes typically <1 second
- No continuous monitoring or background processes
- GPU validation doesn't interfere with actual GPU workloads

## Integration with Other Features

### LRU Allocation
Validation integrates with LRU (Least Recently Used) allocation:
- Only validated available GPUs are considered for LRU ranking
- Unauthorized GPUs are excluded from LRU pool
- Last release timestamps are preserved across validation

### Race Condition Protection
Validation is integrated into the atomic allocation process:
- Redis Lua scripts receive unreserved GPU lists
- Allocation fails if requested GPUs become unreserved during allocation
- Ensures consistent state between validation and allocation

### Heartbeat System
Validation complements the heartbeat system:
- Heartbeat tracks reservation liveness
- Validation tracks actual GPU usage
- Together they provide complete resource lifecycle management

## Troubleshooting Validation

### nvidia-smi Not Available
```bash
Error: nvidia-smi command not found
```

Ensure NVIDIA drivers are properly installed:
```bash
# Test nvidia-smi availability
nvidia-smi

# If not available, install NVIDIA drivers
sudo apt install nvidia-driver-*  # Ubuntu
```

### Permission Issues
```bash
Warning: Could not determine owner for PID 12345
```

This may occur when:
- `/proc` filesystem has restricted access
- User lacks permissions to query process information
- Process terminates between detection and query

The system will still function but with less detailed process information.

### Memory Reporting Discrepancies
Different tools may report slightly different GPU memory usage:
- nvidia-smi vs. CUDA runtime memory reports
- Shared memory vs. process-specific memory
- Memory allocated vs. memory actually used

canhazgpu uses nvidia-smi reporting for consistency across all processes.

GPU validation ensures that canhazgpu maintains accurate, real-time awareness of GPU resource utilization, preventing conflicts and enabling efficient resource sharing.