# LRU Allocation Strategy

canhazgpu uses a **Least Recently Used (LRU)** allocation strategy to ensure fair distribution of GPU resources over time. This approach promotes equitable access and can help with thermal management and hardware longevity.

## How LRU Works

### Basic Principle
When multiple GPUs are available for allocation, canhazgpu selects the GPU(s) that were **released longest ago**. This ensures that:

- All GPUs get used over time
- No single GPU is overworked while others sit idle
- Resource allocation is fair across time periods
- Thermal loads are distributed across hardware

### Timestamp Tracking
Each GPU maintains a `last_released` timestamp in Redis:

```json
{
  "last_released": 1672531200.123
}
```

When a GPU is released (either from `run` completion or manual `release`), this timestamp is updated to the current time.

## LRU in Action

### Example Scenario
Consider a system with 4 GPUs where you request 2 GPUs:

```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 5h 30m 15s                                           # Oldest release
1   available          free for 1h 45m 30s                                           # Recent release  
2   available          free for 3h 12m 45s                                           # Middle age
3   available          free for 2h 08m 12s                                           # Middle age
```

**LRU Ranking** (oldest first):
1. GPU 0 (5h 30m ago) ← Selected
2. GPU 2 (3h 12m ago) ← Selected  
3. GPU 3 (2h 08m ago)
4. GPU 1 (1h 45m ago)

**Result**: GPUs 0 and 2 are allocated.

### Allocation Output
```bash
❯ canhazgpu run --gpus 2 -- python train.py
Reserved 2 GPU(s): [0, 2] for command execution
# CUDA_VISIBLE_DEVICES=0,2 python train.py
```

## LRU with Unauthorized Usage

### Exclusion from LRU Pool
Unauthorized GPUs are automatically excluded from LRU consideration:

```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 4h                                                    
1   in use    bob                                           WITHOUT RESERVATION        1024MB used
2   available          free for 1h                                                    
3   available          free for 2h                                                    
```

**LRU Pool** (unreserved GPU 1 excluded):
1. GPU 0 (4h ago) ← Available for allocation
2. GPU 3 (2h ago) ← Available for allocation
3. GPU 2 (1h ago) ← Available for allocation

**Request 2 GPUs**: Would get GPUs 0 and 3.

### Error Handling
```bash
❯ canhazgpu run --gpus 4 -- python train.py
Error: Not enough GPUs available. Requested: 4, Available: 3 (1 GPUs in use without reservation - run 'canhazgpu status' for details)
```

The system shows you exactly why allocation failed.

## LRU Benefits

### Fair Resource Distribution
Without LRU, users might always get the same GPUs:
- GPU 0 gets used constantly
- GPU 7 never gets used
- Uneven wear and thermal stress

With LRU:
- All GPUs get used over time
- Fair rotation ensures equitable access
- Better long-term hardware health

### Thermal Management
**Problem without LRU:**
```bash
# Always allocating GPU 0
GPU 0: 85°C (constantly hot)
GPU 1: 35°C (always idle)
GPU 2: 35°C (always idle)
GPU 3: 35°C (always idle)
```

**With LRU distribution:**
```bash
# Heat distributed across GPUs
GPU 0: 65°C (used 2 hours ago)
GPU 1: 45°C (used 6 hours ago)  
GPU 2: 85°C (currently in use)
GPU 3: 55°C (used 4 hours ago)
```

### Usage Pattern Analytics
LRU timestamps provide valuable usage analytics:

```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 15m 30s                                               # Recently active
1   available          free for 8h 45m 12s                                            # Underutilized
2   in use    alice    2h 30m 0s   run     vllm-model       heartbeat 30s ago          # Currently active
3   available          free for 1h 20m 45s                                            # Normal usage
```

From this, you can identify:
- **Underutilized resources** (GPU 1)
- **Normal usage patterns** (GPUs 0, 3)
- **Current workloads** (GPU 2)

## Advanced LRU Scenarios

### Mixed Reservation Types
```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 2h                                                    
1   in use    bob      1h 0m 0s    run     transformers     heartbeat 15s ago          
2   in use    alice    30m 0s      manual                   expires in 3h 30m 0s      
3   available          free for 6h                                                    
```

**Available for LRU**: GPUs 0 and 3
**LRU order**: GPU 3 (6h ago), then GPU 0 (2h ago)

### New System Initialization
When GPUs are first initialized, they have no `last_released` timestamp:

```bash
❯ canhazgpu admin --gpus 4
Initialized 4 GPUs (IDs 0 to 7)

❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          never used                                                     
1   available          never used                                                     
2   available          never used                                                     
3   available          never used                                                     
```

**Initial LRU behavior**: 
- GPUs with no `last_released` timestamp are considered "oldest"
- Allocation order is deterministic (typically lowest ID first)
- After first use cycle, normal LRU takes over

### LRU After System Restart
LRU timestamps persist in Redis across system restarts:

```bash
# Before restart
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 1h 30m                                               
1   available          free for 4h 15m                                               

# After system restart
❯ canhazgpu status  
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 1h 35m                                               # Time continues
1   available          free for 4h 20m                                               # Time continues
```

## LRU Implementation Details

### Atomic LRU Selection
LRU selection happens atomically within Redis Lua scripts to prevent race conditions:

```lua
-- Simplified LRU logic (actual implementation in Redis Lua)
local available_gpus = {}
for i = 0, gpu_count - 1 do
    local gpu_data = redis.call('GET', 'canhazgpu:gpu:' .. i)
    if gpu_data == false or gpu_is_available(gpu_data) then
        -- Add to available list with timestamp
        table.insert(available_gpus, {id = i, last_released = get_timestamp(gpu_data)})
    end
end

-- Sort by last_released (oldest first)
table.sort(available_gpus, function(a, b) 
    return a.last_released < b.last_released 
end)

-- Select requested number of GPUs
local selected = {}
for i = 1, requested_count do
    if available_gpus[i] then
        table.insert(selected, available_gpus[i].id)
    end
end
```

### Performance Considerations
LRU calculation is efficient:
- **Time complexity**: O(n log n) where n is the number of available GPUs
- **Space complexity**: O(n) for sorting
- **Typical performance**: <1ms for systems with dozens of GPUs

## Monitoring LRU Effectiveness

### Usage Distribution Analysis
```bash
#!/bin/bash
# lru_analysis.sh - Analyze GPU usage distribution

echo "GPU Usage Distribution (last 24h):"
canhazgpu status | grep "available" | while read -r line; do
    GPU=$(echo "$line" | awk '{print $1}')
    TIME=$(echo "$line" | sed -n 's/.*free for \([^[:space:]]*\).*/\1/p')
    echo "GPU $GPU: $TIME"
done | sort -k3,3n
```

### Identifying Imbalances
```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 15m                                                   # Heavily used
1   available          free for 30m                                                   # Heavily used
2   available          free for 12h 45m                                               # Underutilized!
3   available          free for 45m                                                   # Normal usage
```

GPU 2 hasn't been used in 12+ hours - might indicate:
- Hardware issues with that GPU
- User preferences avoiding that GPU
- Configuration problems

## Customizing LRU Behavior

Currently, LRU is the only allocation strategy, but the system is designed to support alternatives:

### Potential Future Strategies
- **Round-robin**: Strict rotation regardless of release time
- **Random**: Random selection for load balancing
- **Thermal-aware**: Prefer cooler GPUs
- **Performance-based**: Prefer faster GPUs for specific workloads

### Configuration Extension Points
The LRU logic is centralized and could be made configurable:
```bash
# Potential future configuration
canhazgpu admin --allocation-strategy lru
canhazgpu admin --allocation-strategy round-robin
canhazgpu admin --allocation-strategy thermal-aware
```

## Best Practices with LRU

### For Users
- **Don't game the system**: Trust the LRU allocation for fairness
- **Release promptly**: Quick releases improve the LRU distribution
- **Monitor your usage patterns**: Use `status` to see resource distribution

### For Administrators
- **Monitor distribution**: Check for GPUs that are never used
- **Investigate imbalances**: GPUs with very old `last_released` times may have issues
- **Plan maintenance**: Use LRU data to schedule maintenance during low-usage periods

### For System Health
- **Thermal monitoring**: LRU helps distribute heat loads
- **Hardware longevity**: Even usage patterns extend hardware life
- **Performance consistency**: All GPUs stay active and ready

The LRU allocation strategy ensures that canhazgpu provides fair, efficient, and sustainable GPU resource management for your entire team.