# Unauthorized Usage Detection

One of canhazgpu's key features is detecting and handling GPUs that are being used without proper reservations. This prevents resource conflicts and enforces fair resource sharing policies.

## What is Unauthorized Usage?

Unauthorized usage occurs when:
- A GPU has active processes consuming >100MB of memory
- No proper reservation exists for that GPU in the system
- The GPU usage was not coordinated through canhazgpu

### Common Scenarios
```bash
# These create unauthorized usage:
CUDA_VISIBLE_DEVICES=0,1 python train.py           # Direct GPU access
jupyter notebook                                    # Jupyter using default GPUs
docker run --gpus all pytorch/pytorch python       # Container with GPU access
python -c "import torch; torch.cuda.set_device(2)" # Explicit GPU selection
```

```bash
# These are proper usage:
canhazgpu run --gpus 2 -- python train.py          # Proper reservation
canhazgpu reserve --gpus 1 && jupyter notebook     # Manual reservation first
```

## Detection Methods

### Real-Time Scanning
canhazgpu detects unauthorized usage through:

1. **nvidia-smi Integration**: Queries actual GPU processes and memory usage
2. **Process Ownership Detection**: Identifies which users are running processes
3. **Memory Threshold**: Considers GPUs with >100MB usage as "in use"
4. **Cross-Reference**: Compares actual usage against Redis reservation database

### Detection Timing
Unauthorized usage detection runs:
- **Before every allocation** (`run` and `reserve` commands)
- **During status checks** (`status` command)
- **Atomically during allocation** (within Redis transactions)

## Status Display

### Single Unauthorized User
```bash
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
```

**Information shown:**
- `user bob`: The user running unauthorized processes
- `1024MB used`: Total memory consumption on this GPU
- `PID 12345 (python3)`: Process ID and name
- `PID 67890 (jupyter)`: Additional processes (if any)

### Multiple Unauthorized Users
```bash
GPU 3: IN USE WITHOUT RESERVATION by users alice, bob and charlie - 2048MB used by PID 12345 (python3), PID 23456 (pytorch) and 2 more
```

**Information shown:**
- `users alice, bob and charlie`: All users with processes on this GPU
- `2048MB used`: Total memory consumption
- `and 2 more`: Indicates additional processes (display truncated for readability)

### Process Details
The system attempts to show:
- **Process ID (PID)**: Unique identifier for the process
- **Process name**: Executable name (python3, jupyter, etc.)
- **User ownership**: Which user launched the process

## Impact on Allocation

### Automatic Exclusion
When unauthorized usage is detected:

1. **Pre-allocation scan** identifies GPUs with unauthorized usage
2. **Exclusion from available pool** removes those GPUs from allocation candidates
3. **LRU calculation** operates only on truly available GPUs
4. **Error reporting** provides detailed feedback about unavailable resources

### Error Messages
```bash
❯ canhazgpu run --gpus 3 -- python train.py
Error: Not enough GPUs available. Requested: 3, Available: 1 (2 GPUs in use without reservation - run 'canhazgpu status' for details)
```

**Message breakdown:**
- `Requested: 3`: Number of GPUs you asked for
- `Available: 1`: Number of GPUs actually available for allocation
- `2 GPUs in use without reservation`: Number of GPUs excluded due to unauthorized usage
- Suggests running `status` for detailed information

## Handling Unauthorized Usage

### Investigation Steps

1. **Check detailed status**:
   ```bash
   canhazgpu status
   ```

2. **Identify unauthorized users and processes**:
   ```bash
   GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
   ```

3. **Contact the user** to coordinate proper usage

4. **Verify process details** if needed:
   ```bash
   ps -p 12345 -o pid,ppid,user,command
   ```

### Resolution Options

#### Option 1: User Stops Unauthorized Usage
```bash
# User bob stops their processes
kill 12345 67890

# Or gracefully shuts down
# Ctrl+C in their terminal, close Jupyter, etc.
```

#### Option 2: User Creates Proper Reservation
```bash
# User bob creates a proper reservation
canhazgpu reserve --gpus 1 --duration 4h

# Then continues their work
# GPU will now show as properly reserved
```

#### Option 3: Wait for Completion
```bash
# Wait for unauthorized processes to finish naturally
# Then GPU will become available again
```

## User Education and Policies

### Training Users
Help users understand proper GPU usage:

```bash
# Wrong way
CUDA_VISIBLE_DEVICES=0 python train.py

# Right way  
canhazgpu run --gpus 1 -- python train.py
```

### Policy Enforcement Examples

#### Gentle Reminder
```bash
#!/bin/bash
# unauthorized_reminder.sh
STATUS=$(canhazgpu status)
UNAUTHORIZED=$(echo "$STATUS" | grep "WITHOUT RESERVATION")

if [ -n "$UNAUTHORIZED" ]; then
    echo "Reminder: Please use canhazgpu for GPU reservations"
    echo "$UNAUTHORIZED"
fi
```

#### Automated Notification
```bash
#!/bin/bash
# unauthorized_notify.sh
STATUS=$(canhazgpu status)
UNAUTHORIZED=$(echo "$STATUS" | grep "WITHOUT RESERVATION")

if [ -n "$UNAUTHORIZED" ]; then
    # Extract usernames and send notifications
    USERS=$(echo "$UNAUTHORIZED" | sed -n 's/.*by users\? \([^-]*\) -.*/\1/p')
    for USER in $USERS; do
        echo "Please use canhazgpu for GPU reservations" | wall -n "$USER"
    done
fi
```

## Advanced Detection Scenarios

### Multi-GPU Unauthorized Usage
```bash
GPU 1: IN USE WITHOUT RESERVATION by user alice - 2048MB used by PID 11111 (python3)
GPU 2: IN USE WITHOUT RESERVATION by user alice - 2048MB used by PID 11111 (python3)
GPU 5: IN USE WITHOUT RESERVATION by user alice - 2048MB used by PID 11111 (python3)
```

Same process using multiple GPUs - user should reserve all needed GPUs properly.

### Mixed Usage Patterns
```bash
GPU 0: IN USE by bob for 1h 30m 0s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 1: IN USE WITHOUT RESERVATION by user alice - 1024MB used by PID 22222 (jupyter)
GPU 2: AVAILABLE (last released 2h 0m 0s ago) [validated: 45MB used]
GPU 3: IN USE by charlie for 0h 45m 0s (manual, expires in 7h 15m 0s) [validated: no actual usage detected]
```

Mix of proper usage, unauthorized usage, and available GPUs.

### Container-Based Unauthorized Usage
```bash
GPU 1: IN USE WITHOUT RESERVATION by users root, alice - 3072MB used by PID 33333 (dockerd), PID 44444 (python3) and 1 more
```

Docker containers running with GPU access - users should coordinate container GPU usage through canhazgpu.

## System Integration

### Monitoring Integration
```python
def check_unauthorized_usage():
    """Check for unauthorized GPU usage and return details"""
    result = subprocess.run(['canhazgpu', 'status'], 
                          capture_output=True, text=True)
    
    unauthorized = []
    for line in result.stdout.split('\n'):
        if 'WITHOUT RESERVATION' in line:
            # Parse user and memory info
            match = re.search(r'by users? ([^-]+) - (\d+)MB', line)
            if match:
                users = match.group(1).strip()
                memory = int(match.group(2))
                unauthorized.append({
                    'users': users,
                    'memory_mb': memory,
                    'raw_line': line
                })
    
    return unauthorized

# Usage in monitoring
violations = check_unauthorized_usage()
if violations:
    for v in violations:
        logger.warning(f"Unauthorized GPU usage: {v['users']} using {v['memory_mb']}MB")
```

### Automated Response
```bash
#!/bin/bash
# unauthorized_response.sh

# Check for unauthorized usage
UNAUTHORIZED=$(canhazgpu status | grep "WITHOUT RESERVATION")

if [ -n "$UNAUTHORIZED" ]; then
    # Log the violation
    echo "$(date): $UNAUTHORIZED" >> /var/log/gpu_violations.log
    
    # Extract and notify users
    echo "$UNAUTHORIZED" | while read -r line; do
        USERS=$(echo "$line" | sed -n 's/.*by users\? \([^-]*\) -.*/\1/p')
        for USER in $USERS; do
            # Send notification to user
            echo "GPU Policy Violation: Please use 'canhazgpu reserve' for GPU access" | \
                mail -s "GPU Usage Policy" "$USER@company.com"
        done
    done
    
    # Alert administrators
    echo "$UNAUTHORIZED" | mail -s "GPU Policy Violations Detected" admin@company.com
fi
```

## Best Practices

### For Users
- **Always use canhazgpu** for GPU access
- **Check status first** with `canhazgpu status` before starting work
- **Reserve appropriately** - don't over-reserve, but don't skip reservations
- **Clean up** - release manual reservations when done

### For Administrators
- **Monitor regularly** - set up automated checks for unauthorized usage
- **Educate users** - provide training on proper GPU reservation practices
- **Set clear policies** - document expected GPU usage procedures
- **Respond quickly** - address unauthorized usage promptly to prevent conflicts

### For System Integration
- **Integrate with job schedulers** - ensure SLURM/PBS jobs use canhazgpu
- **Container orchestration** - configure Kubernetes/Docker to respect reservations
- **Development tools** - configure IDEs and notebooks to check for reservations

Unauthorized usage detection ensures fair resource sharing and prevents the frustrating "GPU out of memory" errors that occur when multiple users unknowingly compete for the same resources.