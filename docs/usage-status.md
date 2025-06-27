# Status Monitoring

The `status` command provides real-time visibility into GPU allocation and usage across your system. It combines reservation tracking with actual GPU usage validation to give you a complete picture of resource utilization.

## Basic Usage

```bash
canhazgpu status
```

No options are required - the command automatically validates all GPUs and shows comprehensive status information.

## Status Output Explained

### Example Output
```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
GPU 3: IN USE by charlie for 1h 2m 15s (manual, expires in 3h 15m 45s) [validated: no actual usage detected]
GPU 4: IN USE WITHOUT RESERVATION by users alice, bob and charlie - 2048MB used by PID 12345 (python3), PID 23456 (pytorch) and 2 more
```

### Status Types

#### AVAILABLE
```bash
GPU 0: AVAILABLE (last released 0h 30m 15s ago) [validated: 45MB used]
```

- **Meaning**: GPU is free and can be allocated
- **Time info**: Shows when it was last released (for LRU allocation)
- **Validation**: Shows current memory usage (usually low baseline usage)

#### IN USE (Proper Reservations)
```bash
GPU 1: IN USE by alice for 0h 15m 30s (run, last heartbeat 0h 0m 5s ago) [validated: 8452MB, 1 processes]
```

**Components:**
- `by alice`: Username who reserved the GPU
- `for 0h 15m 30s`: How long it's been reserved
- `(run, ...)`: Reservation type and additional info
- `[validated: ...]`: Actual usage validation

**Reservation Types:**

**Run-type reservations:**
```bash
(run, last heartbeat 0h 0m 5s ago)
```
- Created by `canhazgpu run` command
- Maintained by periodic heartbeats
- Auto-released when process ends or heartbeat stops

**Manual reservations:**
```bash
(manual, expires in 3h 15m 45s)
```
- Created by `canhazgpu reserve` command  
- Time-based expiry
- Must be manually released or will expire

#### IN USE WITHOUT RESERVATION
```bash
GPU 2: IN USE WITHOUT RESERVATION by user bob - 1024MB used by PID 12345 (python3), PID 67890 (jupyter)
```

- **Meaning**: Someone is using the GPU without proper reservation
- **User identification**: Shows which user(s) are running unauthorized processes
- **Process details**: Lists PIDs and process names using the GPU
- **Impact**: This GPU will be excluded from allocation until usage stops

**Multiple unauthorized users:**
```bash
GPU 4: IN USE WITHOUT RESERVATION by users alice, bob and charlie - 2048MB used by PID 12345 (python3), PID 23456 (pytorch) and 2 more
```

### Validation Information

The `[validated: ...]` section shows actual GPU usage detected via nvidia-smi:

#### Confirms Proper Usage
```bash
[validated: 8452MB, 1 processes]
```
- GPU is reserved and actually being used
- Shows memory usage and process count
- Indicates healthy, proper resource utilization

#### No Actual Usage
```bash
[validated: no actual usage detected]
```
- GPU is reserved but no processes are running
- Might indicate:
  - Preparation phase before starting work
  - Finished work but reservation not yet released
  - Stale reservation that should be cleaned up

#### Baseline Usage Only
```bash
[validated: 45MB used]
```
- Available GPU with minimal background usage
- Normal baseline memory usage from GPU drivers
- Safe to allocate

## Monitoring Patterns

### Regular Health Checks
```bash
# Quick status check
canhazgpu status

# Monitor changes over time
watch -n 30 canhazgpu status

# Log status for analysis
canhazgpu status >> gpu_usage_log.txt
```

### Identifying Problems

#### Stale Reservations
```bash
GPU 3: IN USE by alice for 8h 45m 0s (manual, expires in 0h 15m 0s) [validated: no actual usage detected]
```
- Long reservation with no actual usage
- User likely forgot to release
- Will auto-expire soon

#### Heartbeat Issues
```bash
GPU 1: IN USE by bob for 2h 30m 0s (run, last heartbeat 0h 5m 30s ago) [validated: 8452MB, 1 processes]
```
- Last heartbeat was 5+ minutes ago (should be <1 minute)
- Possible network issues or process problems
- May auto-release due to heartbeat timeout

#### Unauthorized Usage Patterns
```bash
GPU 2: IN USE WITHOUT RESERVATION by user charlie - 12GB used by PID 12345 (python3)
GPU 5: IN USE WITHOUT RESERVATION by user charlie - 8GB used by PID 23456 (jupyter)
```
- Same user using multiple GPUs without reservation
- High memory usage indicates active workloads
- Should contact user to use proper reservation system

### Team Coordination

#### Planning Allocations
```bash
❯ canhazgpu status
GPU 0: AVAILABLE (last released 2h 0m 0s ago)     # Good candidate
GPU 1: AVAILABLE (last released 0h 30m 0s ago)    # Recently used
GPU 2: IN USE by alice for 0h 5m 0s (run, ...)    # Just started
GPU 3: IN USE by bob for 3h 45m 0s (manual, expires in 0h 15m 0s)  # Expiring soon
```

From this, you can see:
- GPU 0 is the best choice (LRU)
- GPU 1 was used recently
- GPU 3 will be available in 15 minutes

#### Resource Conflicts
```bash
❯ canhazgpu status
Available GPUs: 2 out of 8
In use with reservations: 4 GPUs  
Unauthorized usage: 2 GPUs
```

Clear indication that unauthorized usage is reducing available capacity.

## Advanced Monitoring

### Integration with Monitoring Systems

#### Prometheus/Grafana Integration
```bash
#!/bin/bash
# gpu_metrics.sh - Export metrics for monitoring

STATUS=$(canhazgpu status)

# Count GPU states
AVAILABLE=$(echo "$STATUS" | grep "AVAILABLE" | wc -l)
IN_USE=$(echo "$STATUS" | grep "IN USE by" | wc -l)
UNAUTHORIZED=$(echo "$STATUS" | grep "WITHOUT RESERVATION" | wc -l)

# Export metrics
echo "gpu_available $AVAILABLE"
echo "gpu_in_use $IN_USE"  
echo "gpu_unauthorized $UNAUTHORIZED"
echo "gpu_total $((AVAILABLE + IN_USE + UNAUTHORIZED))"
```

#### Log Analysis
```bash
# Capture status with timestamps
while true; do
    echo "$(date): $(canhazgpu status)" >> gpu_monitoring.log
    sleep 300  # Every 5 minutes
done

# Analyze usage patterns
grep "AVAILABLE" gpu_monitoring.log | wc -l
grep "WITHOUT RESERVATION" gpu_monitoring.log | cut -d: -f2- | sort | uniq -c
```

### Automated Alerts

#### Unauthorized Usage Detection
```bash
#!/bin/bash
# unauthorized_alert.sh

STATUS=$(canhazgpu status)
UNAUTHORIZED=$(echo "$STATUS" | grep "WITHOUT RESERVATION")

if [ -n "$UNAUTHORIZED" ]; then
    echo "ALERT: Unauthorized GPU usage detected!"
    echo "$UNAUTHORIZED"
    
    # Send notification (customize as needed)
    echo "$UNAUTHORIZED" | mail -s "GPU Policy Violation" admin@company.com
fi
```

#### Capacity Monitoring
```bash
#!/bin/bash
# capacity_alert.sh

STATUS=$(canhazgpu status)
AVAILABLE=$(echo "$STATUS" | grep "AVAILABLE" | wc -l)
TOTAL=$(echo "$STATUS" | wc -l)
UTILIZATION=$((100 * (TOTAL - AVAILABLE) / TOTAL))

if [ $UTILIZATION -gt 90 ]; then
    echo "WARNING: GPU utilization at ${UTILIZATION}%"
    canhazgpu status
fi
```

## Status Command Integration

### Shell Scripts
```bash
#!/bin/bash
# wait_for_gpu.sh - Wait until GPUs become available

echo "Waiting for GPUs to become available..."
while true; do
    AVAILABLE=$(canhazgpu status | grep "AVAILABLE" | wc -l)
    if [ $AVAILABLE -ge 2 ]; then
        echo "GPUs available! Starting job..."
        canhazgpu run --gpus 2 -- python train.py
        break
    fi
    echo "Only $AVAILABLE GPUs available, waiting..."
    sleep 60
done
```

### Python Integration
```python
import subprocess
import re
import time

def get_gpu_status():
    """Parse canhazgpu status output"""
    result = subprocess.run(['canhazgpu', 'status'], 
                          capture_output=True, text=True)
    
    if result.returncode != 0:
        raise RuntimeError(f"Status check failed: {result.stderr}")
    
    status = {}
    for line in result.stdout.strip().split('\n'):
        if line.startswith('GPU '):
            gpu_id = int(line.split(':')[0].split()[1])
            if 'AVAILABLE' in line:
                status[gpu_id] = 'available'
            elif 'WITHOUT RESERVATION' in line:
                status[gpu_id] = 'unauthorized'  
            elif 'IN USE' in line:
                status[gpu_id] = 'reserved'
    
    return status

def wait_for_gpus(count=1, timeout=3600):
    """Wait for specified number of GPUs to become available"""
    start_time = time.time()
    
    while time.time() - start_time < timeout:
        status = get_gpu_status()
        available = sum(1 for state in status.values() if state == 'available')
        
        if available >= count:
            return True
            
        print(f"Waiting for {count} GPUs... ({available} currently available)")
        time.sleep(30)
    
    return False

# Usage
if wait_for_gpus(2):
    print("GPUs available! Starting training...")
    subprocess.run(['canhazgpu', 'run', '--gpus', '2', '--', 'python', 'train.py'])
else:
    print("Timeout waiting for GPUs")
```

The `status` command is your primary tool for understanding GPU resource utilization, identifying conflicts, and coordinating with your team. Regular monitoring helps maintain efficient resource usage and prevents conflicts.