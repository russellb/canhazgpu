# Unreserved Usage Detection

One of canhazgpu's key features is detecting and handling GPUs that are being used without proper reservations. This prevents resource conflicts and enforces fair resource sharing policies.

## What is Unreserved Usage?

Unreserved usage occurs when:
- A GPU has active processes consuming >1GB of memory
- No proper reservation exists for that GPU in the system
- The GPU usage was not coordinated through canhazgpu

### Common Scenarios
```bash
# These create unreserved usage:
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
canhazgpu detects unreserved usage through:

1. **nvidia-smi Integration**: Queries actual GPU processes and memory usage
2. **Process Ownership Detection**: Identifies which users are running processes
3. **Memory Threshold**: Considers GPUs with >1GB usage as "in use"
4. **Cross-Reference**: Compares actual usage against Redis reservation database

### Detection Timing
Unreserved usage detection runs:
- **Before every allocation** (`run` and `reserve` commands)
- **During status checks** (`status` command)
- **Atomically during allocation** (within Redis transactions)

## Status Display

### Single Unreserved User
```bash
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
2   in use    bob                          mistralai/Mistral-7B-Instruct-v0.1           WITHOUT RESERVATION        1024MB used by PID 12345 (python3), PID 67890 (jupyter)
```

**Information shown:**
- `USER`: The user running unreserved processes (bob)
- `DETAILS`: Shows "WITHOUT RESERVATION" status
- `VALIDATION`: Total memory consumption and process details
- `PID 12345 (python3)`: Process ID and name
- `PID 67890 (jupyter)`: Additional processes (if any)

### Multiple Unreserved Users
```bash
GPU STATUS    USER            DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- --------------- ----------- ------- ---------------- -------------------------- ---------------------
3   in use    alice,bob,charlie                    meta-llama/Meta-Llama-3-8B-Instruct                    WITHOUT RESERVATION        2048MB used by PID 12345 (python3), PID 23456 (pytorch) and 2 more
```

**Information shown:**
- `USER`: All users with processes on this GPU (alice,bob,charlie)
- `DETAILS`: Shows "WITHOUT RESERVATION" status
- `VALIDATION`: Total memory consumption and process details
- `and 2 more`: Indicates additional processes (display truncated for readability)

### Process Details
The system attempts to show:
- **Process ID (PID)**: Unique identifier for the process
- **Process name**: Executable name (python3, jupyter, etc.)
- **User ownership**: Which user launched the process

## Impact on Allocation

### Automatic Exclusion
When unreserved usage is detected:

1. **Pre-allocation scan** identifies GPUs with unreserved usage
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
- `2 GPUs in use without reservation`: Number of GPUs excluded due to unreserved usage
- Suggests running `status` for detailed information

## Handling Unreserved Usage

### Investigation Steps

1. **Check detailed status**:
   ```bash
   canhazgpu status
   ```

2. **Identify unreserved users and processes**:
   ```bash
   GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
   --- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
   2   in use    bob                          mistralai/Mistral-7B-Instruct-v0.1           WITHOUT RESERVATION        1024MB used by PID 12345 (python3), PID 67890 (jupyter)
   ```

3. **Contact the user** to coordinate proper usage

4. **Verify process details** if needed:
   ```bash
   ps -p 12345 -o pid,ppid,user,command
   ```

### Resolution Options

#### Option 1: User Stops Unreserved Usage
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
# Wait for unreserved processes to finish naturally
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
# unreserved_reminder.sh
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
# unreserved_notify.sh
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

### Multi-GPU Unreserved Usage
```bash
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
1   in use    alice                        NousResearch/Nous-Hermes-2-Yi-34B                         WITHOUT RESERVATION        2048MB used by PID 11111 (python3)
2   in use    alice                        NousResearch/Nous-Hermes-2-Yi-34B                         WITHOUT RESERVATION        2048MB used by PID 11111 (python3)
5   in use    alice                        NousResearch/Nous-Hermes-2-Yi-34B                         WITHOUT RESERVATION        2048MB used by PID 11111 (python3)
```

Same process using multiple GPUs - user should reserve all needed GPUs properly.

### Mixed Usage Patterns
```bash
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   in use    bob      1h 30m 0s   run     microsoft/DialoGPT-large heartbeat 5s ago          8452MB, 1 processes
1   in use    alice                        teknium/OpenHermes-2.5-Mistral-7B                         WITHOUT RESERVATION        1024MB used by PID 22222 (jupyter)
2   available          free for 2h                                                    45MB used
3   in use    charlie  45m 0s      manual                   expires in 7h 15m 0s      no usage detected
```

Mix of proper usage, unreserved usage, and available GPUs.

### Container-Based Unreserved Usage
```bash
GPU STATUS    USER        DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- ----------- ----------- ------- ---------------- -------------------------- ---------------------
1   in use    root,alice                      codellama/CodeLlama-7b-Instruct-hf                      WITHOUT RESERVATION        3072MB used by PID 33333 (dockerd), PID 44444 (python3) and 1 more
```

Docker containers running with GPU access - users should coordinate container GPU usage through canhazgpu.

## System Integration

### Monitoring Integration
```python
def check_unreserved_usage():
    """Check for unreserved GPU usage and return details"""
    result = subprocess.run(['canhazgpu', 'status'], 
                          capture_output=True, text=True)
    
    unreserved = []
    for line in result.stdout.split('\n'):
        if 'WITHOUT RESERVATION' in line:
            # Parse user and memory info
            match = re.search(r'by users? ([^-]+) - (\d+)MB', line)
            if match:
                users = match.group(1).strip()
                memory = int(match.group(2))
                unreserved.append({
                    'users': users,
                    'memory_mb': memory,
                    'raw_line': line
                })
    
    return unreserved

# Usage in monitoring
violations = check_unreserved_usage()
if violations:
    for v in violations:
        logger.warning(f"Unreserved GPU usage: {v['users']} using {v['memory_mb']}MB")
```

### Automated Response
```bash
#!/bin/bash
# unreserved_response.sh

# Check for unreserved usage
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
- **Monitor regularly** - set up automated checks for unreserved usage
- **Educate users** - provide training on proper GPU reservation practices
- **Set clear policies** - document expected GPU usage procedures
- **Respond quickly** - address unreserved usage promptly to prevent conflicts

### For System Integration
- **Integrate with job schedulers** - ensure SLURM/PBS jobs use canhazgpu
- **Container orchestration** - configure Kubernetes/Docker to respect reservations
- **Development tools** - configure IDEs and notebooks to check for reservations

Unreserved usage detection ensures fair resource sharing and prevents the frustrating "GPU out of memory" errors that occur when multiple users unknowingly compete for the same resources.