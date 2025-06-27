# Manual GPU Reservations

The `reserve` command allows you to manually reserve GPUs for a specific duration without immediately running a command. This is useful for interactive development, planning work sessions, or blocking GPUs for maintenance.

## Basic Usage

```bash
canhazgpu reserve [--gpus <count>] [--duration <time>]
```

**Defaults:**
- `--gpus`: 1 GPU
- `--duration`: 8 hours

## Duration Formats

canhazgpu supports flexible duration formats:

| Format | Description | Example |
|--------|-------------|---------|
| `30m` | 30 minutes | `--duration 30m` |
| `2h` | 2 hours | `--duration 2h` |
| `1d` | 1 day | `--duration 1d` |
| `0.5h` | 30 minutes (decimal) | `--duration 0.5h` |
| `90m` | 90 minutes | `--duration 90m` |
| `3.5d` | 3.5 days | `--duration 3.5d` |

## Common Examples

### Quick Development Sessions
```bash
# Reserve 1 GPU for 2 hours
canhazgpu reserve --duration 2h

# Reserve 1 GPU for 30 minutes of testing
canhazgpu reserve --duration 30m
```

### Multi-GPU Development
```bash
# Reserve 2 GPUs for 4 hours
canhazgpu reserve --gpus 2 --duration 4h

# Reserve 4 GPUs for distributed development
canhazgpu reserve --gpus 4 --duration 6h
```

### Extended Work Sessions
```bash
# Full day development (8 hours, default)
canhazgpu reserve

# Multi-day project work
canhazgpu reserve --gpus 2 --duration 2d

# Week-long research sprint
canhazgpu reserve --gpus 1 --duration 7d
```

## Use Cases

### Interactive Development
Perfect for Jupyter notebooks, IPython sessions, or iterative model development:

```bash
# Reserve GPU for notebook session
canhazgpu reserve --duration 4h

# Start Jupyter (automatically uses reserved GPU)
jupyter notebook

# Your notebooks now have exclusive GPU access
# CUDA_VISIBLE_DEVICES is automatically set
```

### Batch Job Preparation
Reserve GPUs while you prepare and test your batch jobs:

```bash
# Reserve GPUs for job prep
canhazgpu reserve --gpus 2 --duration 2h

# Test your scripts
python test_distributed.py

# Run the actual job (using same GPUs)
python distributed_training.py

# Release when done
canhazgpu release
```

### Maintenance Windows
Block GPUs during system maintenance or updates:

```bash
# Block GPU during driver updates
canhazgpu reserve --gpus 8 --duration 1h

# Perform maintenance
sudo apt update && sudo apt upgrade nvidia-driver-*

# Release after maintenance
canhazgpu release
```

### Meeting and Presentation Prep
Ensure GPUs are available for demos and presentations:

```bash
# Reserve before important demo
canhazgpu reserve --gpus 1 --duration 3h

# Run demo applications
python demo_inference.py
jupyter notebook presentation.ipynb

# Release after presentation
canhazgpu release
```

## How Manual Reservations Work

### Allocation Process
1. **Validation**: Checks actual GPU usage with nvidia-smi
2. **Conflict Detection**: Excludes GPUs in unauthorized use
3. **LRU Selection**: Chooses least recently used GPUs
4. **Time-based Expiry**: Sets expiration time based on duration
5. **Persistent Storage**: Saves reservation in Redis

### Environment Setup
Unlike `run` commands, manual reservations don't automatically set environment variables. You need to check which GPUs were allocated:

```bash
# Reserve GPUs
❯ canhazgpu reserve --gpus 2 --duration 4h
Reserved 2 GPU(s): [1, 3] for 4h 0m 0s

# Check current allocations
❯ canhazgpu status
GPU 1: IN USE by alice for 0h 0m 30s (manual, expires in 3h 59m 30s)
GPU 3: IN USE by alice for 0h 0m 30s (manual, expires in 3h 59m 30s)

# Manually set CUDA_VISIBLE_DEVICES
export CUDA_VISIBLE_DEVICES=1,3
python your_script.py
```

### Expiration and Cleanup
Manual reservations automatically expire after the specified duration:

```bash
❯ canhazgpu status
GPU 1: IN USE by alice for 3h 58m 45s (manual, expires in 0h 1m 15s)

# After expiration
❯ canhazgpu status  
GPU 1: AVAILABLE (last released 0h 0m 5s ago)
```

## Releasing Reservations

### Manual Release
Release all your manual reservations immediately:

```bash
❯ canhazgpu release
Released 2 GPU(s): [1, 3]
```

### Checking Your Reservations
Use `status` to see your current reservations:

```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 1h 15m 30s ago)
GPU 1: IN USE by alice for 0h 45m 12s (manual, expires in 3h 14m 48s)  # Your reservation
GPU 2: IN USE by bob for 1h 30m 0s (run, last heartbeat 0h 0m 5s ago)
GPU 3: IN USE by alice for 0h 45m 12s (manual, expires in 3h 14m 48s)    # Your reservation
```

## Error Handling

### Insufficient GPUs
```bash
❯ canhazgpu reserve --gpus 4 --duration 2h
Error: Not enough GPUs available. Requested: 4, Available: 2 (2 GPUs in use without reservation - run 'canhazgpu status' for details)
```

Check the status and try again with fewer GPUs or wait for others to finish.

### Invalid Duration Format
```bash
❯ canhazgpu reserve --duration 2hours
Error: Invalid duration format. Use formats like: 30m, 2h, 1d, 0.5h
```

Use the supported duration formats listed above.

### Allocation Lock Contention
```bash
❯ canhazgpu reserve --gpus 2
Error: Failed to acquire allocation lock after 5 attempts
```

Multiple users are trying to allocate GPUs simultaneously. Wait a few seconds and try again.

## Best Practices

### Duration Planning
- **Start conservative**: Reserve for shorter periods initially
- **Extend if needed**: Use `reserve` again to extend (requires releasing first)
- **Plan for interruptions**: Don't reserve longer than you'll actually use

### Resource Efficiency
```bash
# Good: Reserve what you need
canhazgpu reserve --gpus 1 --duration 2h

# Wasteful: Over-reserving
canhazgpu reserve --gpus 8 --duration 24h  # Only if you really need this
```

### Team Coordination
- **Communicate**: Let teammates know about long reservations
- **Release early**: Use `canhazgpu release` when done early
- **Check conflicts**: Use `canhazgpu status` before making large reservations

### Development Workflow
```bash
# Efficient development cycle
canhazgpu reserve --duration 1h           # Start small
# ... work for 45 minutes ...
canhazgpu reserve --duration 30m          # Extend if needed (after releasing)
# ... finish work ...
canhazgpu release                         # Clean up immediately
```

## Integration Examples

### Shell Scripts
```bash
#!/bin/bash
set -e

echo "Reserving GPUs for data processing..."
canhazgpu reserve --gpus 2 --duration 3h

echo "Starting data processing pipeline..."
python preprocess.py
python feature_extraction.py
python model_training.py

echo "Releasing GPUs..."
canhazgpu release

echo "Processing complete!"
```

### Python Integration
```python
import subprocess
import os

def reserve_gpus(count=1, duration="2h"):
    """Reserve GPUs and return allocated GPU IDs"""
    result = subprocess.run([
        "canhazgpu", "reserve", 
        "--gpus", str(count),
        "--duration", duration
    ], capture_output=True, text=True)
    
    if result.returncode != 0:
        raise RuntimeError(f"GPU reservation failed: {result.stderr}")
    
    # Parse GPU IDs from output
    # "Reserved 2 GPU(s): [1, 3] for 2h 0m 0s"
    import re
    match = re.search(r'Reserved \d+ GPU\(s\): \[([^\]]+)\]', result.stdout)
    if match:
        gpu_ids = [int(x.strip()) for x in match.group(1).split(',')]
        os.environ['CUDA_VISIBLE_DEVICES'] = ','.join(map(str, gpu_ids))
        return gpu_ids
    return []

def release_gpus():
    """Release all manual reservations"""
    subprocess.run(["canhazgpu", "release"], check=True)

# Usage
try:
    gpu_ids = reserve_gpus(2, "1h")  
    print(f"Using GPUs: {gpu_ids}")
    
    # Your GPU work here
    import torch
    print(f"PyTorch sees {torch.cuda.device_count()} GPUs")
    
finally:
    release_gpus()
```

Manual reservations provide fine-grained control over GPU allocation, making them perfect for interactive development and planned work sessions.