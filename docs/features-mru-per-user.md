# MRU-per-User Allocation Strategy

canhazgpu uses a **Most Recently Used per User (MRU-per-user)** allocation strategy to provide better GPU affinity for individual users while maintaining fair distribution across the team. This approach gives users preference for GPUs they've recently used, which can improve performance through cache locality and familiarity.

## How MRU-per-User Works

### Basic Principle
When allocating GPUs, canhazgpu prioritizes GPUs that **you** have used most recently, based on your usage history. If you haven't used any available GPUs, it falls back to a global LRU (Least Recently Used) strategy.

This ensures:
- Users get consistent GPU assignments when possible
- Better cache locality and potential performance improvements
- Fair distribution when users have no history with available GPUs
- All GPUs still get used over time

### Priority System
The allocation follows this priority order:

1. **User's Most Recent GPUs** - GPUs you used most recently are allocated first
2. **User's Older GPUs** - Other GPUs from your history, by recency
3. **Global LRU Fallback** - GPUs you've never used, selected by global LRU (oldest released first)

### Usage History Tracking
Each time you release a GPU, a usage record is stored in Redis with:
- User name
- GPU ID
- Start and end timestamps
- Duration
- Reservation type

The system queries your recent usage history (last 100 records) to determine which GPUs to prefer for you.

## MRU-per-User in Action

### Example Scenario - Alice's Workflow
Alice has been working with GPUs 1 and 2 recently:

```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 5h 30m
1   available          free for 30m                                                     # Alice used recently
2   available          free for 45m                                                     # Alice used before GPU 1
3   available          free for 2h 15m
```

**Alice's Usage History**:
- GPU 1: Used 30 min ago
- GPU 2: Used 45 min ago
- GPU 0, 3: Never used by Alice

**When Alice requests 2 GPUs**:
1. GPU 1 (her most recent) ← Selected
2. GPU 2 (her second most recent) ← Selected

**Result**: Alice gets GPUs 1 and 2, maintaining her workflow continuity.

### Example Scenario - Bob's First Request
Bob is a new user with no usage history:

```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 6h
1   available          free for 30m
2   available          free for 45m
3   available          free for 2h 15m
```

**Bob's Usage History**: None

**When Bob requests 2 GPUs** (falls back to global LRU):
1. GPU 0 (oldest global release) ← Selected
2. GPU 3 (second oldest) ← Selected

**Result**: Bob gets GPUs 0 and 3 using LRU fallback.

## MRU Benefits

### GPU Affinity
**Scenario**: Alice is developing a model and making iterative changes.

Without MRU-per-user (random or strict LRU):
```bash
Run 1: Gets GPUs 0, 3
Run 2: Gets GPUs 1, 4  # Different GPUs, cache cold
Run 3: Gets GPUs 2, 5  # Different GPUs again
```

With MRU-per-user:
```bash
Run 1: Gets GPUs 0, 3
Run 2: Gets GPUs 0, 3  # Same GPUs, cache warm!
Run 3: Gets GPUs 0, 3  # Consistent assignment
```

### Cache Locality Benefits
Modern GPUs have caches that can benefit from reusing the same GPU:
- **L2 cache** may retain useful data
- **Model weights** might be partially cached
- **Kernel compilations** might be cached by the driver

### Workflow Continuity
Users working on specific projects get consistent GPU assignments:
- Easier debugging (same hardware behavior)
- Better performance monitoring (comparing runs on same GPU)
- Reduced variability in experiments

### Fair Distribution Still Maintained
MRU-per-user doesn't create unfairness:
- If your preferred GPU is busy, you get another
- New users get LRU allocation (fair distribution)
- Global LRU fallback ensures all GPUs get used

## Advanced Scenarios

### Mixed User Workloads

**System state**:
- Alice frequently uses GPUs 1, 2
- Bob frequently uses GPUs 3, 4
- GPU 0, 5, 6, 7 are available

**Alice requests 2 GPUs**:
1. Checks availability of GPUs 1, 2 (her recent GPUs)
2. Both available → Gets GPUs 1, 2

**Bob requests 2 GPUs**:
1. Checks availability of GPUs 3, 4 (his recent GPUs)
2. Both available → Gets GPUs 3, 4

**Charlie (new user) requests 2 GPUs**:
1. No history → Falls back to global LRU
2. Gets GPUs 0, 5 (oldest available by global LRU)

**Result**: Each user gets their preferred GPUs when possible!

### Handling Conflicts

If multiple users want the same GPU:

```bash
# Alice and Bob both recently used GPU 1
# GPU 1 becomes available
# Alice requests first → Gets GPU 1
# Bob requests second → Gets GPU 2 (his next preference) or LRU fallback
```

The system respects reservation order - first request wins.

### With Unreserved Usage

Unreserved GPUs are excluded from ALL allocation strategies:

```bash
❯ canhazgpu status
GPU STATUS      USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- ----------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available            free for 2h
1   UNRESERVED  bob                                           1024MB used by 1 process
2   available            free for 1h
3   available            free for 3h
```

**Alice's history**: GPUs 1, 2, 3 (in that order)

**When Alice requests 2 GPUs**:
1. GPU 1 is her first preference, but it's UNRESERVED → Skip
2. GPU 2 is her second preference → Select
3. GPU 3 is her third preference → Select

**Result**: Alice gets GPUs 2 and 3.

## Implementation Details

### Efficient History Querying
The system queries Redis for recent usage efficiently:
- Queries last 100 usage records from sorted set
- Filters for current user's records
- Tracks most recent timestamp per GPU
- All done atomically in a Lua script

### Atomic Selection
MRU selection happens atomically within Redis Lua scripts:

```lua
-- Query usage history for this user
local user_gpu_history = {}
local recent_records = redis.call('ZREVRANGE', 'canhazgpu:usage_history_sorted', 0, 99, 'WITHSCORES')

-- Build user's GPU preference map
for each record in recent_records do
    if record.user == current_user then
        user_gpu_history[gpu_id] = timestamp
    end
end

-- Sort available GPUs by MRU-per-user strategy
table.sort(available_gpus, function(a, b)
    -- If both have user history, prefer more recent
    if a.user_last_used > 0 and b.user_last_used > 0 then
        return a.user_last_used > b.user_last_used
    end
    -- If only one has user history, prefer it
    if a.user_last_used > 0 then return true end
    if b.user_last_used > 0 then return false end
    -- Neither has history, use global LRU
    return a.last_released < b.last_released
end)
```

### Performance
- **Time complexity**: O(n log n) for sorting available GPUs
- **History query**: O(log m + 100) where m is total history size
- **Typical performance**: <2ms for systems with dozens of GPUs

## Monitoring MRU Effectiveness

### Check Your GPU Affinity
```bash
❯ canhazgpu report --days 7 | grep $(whoami)
```

You should see patterns in your GPU usage - if MRU is working well, you'll repeatedly use the same GPUs.

### System-Wide Distribution
```bash
❯ canhazgpu report --days 7
```

Even with MRU-per-user, you should see all GPUs being used across all users (the LRU fallback ensures this).

## Best Practices

### For Users
- **Rely on MRU**: The system will give you your preferred GPUs when available
- **Don't request specific IDs unnecessarily**: Let MRU handle assignment
- **Release promptly**: Helps you and others get preferred GPUs

### For Administrators
- **Monitor distribution**: Ensure no GPUs are consistently idle
- **Check affinity patterns**: Users should have consistent assignments
- **Investigate anomalies**: If a user never gets their preferred GPU, investigate why

### Transition from LRU
If you're migrating from an LRU-only system:
- **No configuration changes needed**: MRU-per-user is automatic
- **Existing history is used**: Past usage data informs MRU decisions
- **Graceful fallback**: Users without history get LRU allocation

The MRU-per-user allocation strategy provides better user experience through GPU affinity while maintaining the fair distribution and efficiency that LRU provides.
