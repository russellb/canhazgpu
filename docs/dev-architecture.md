# Architecture

canhazgpu is designed as a Go CLI application that uses Redis for distributed coordination and nvidia-smi for GPU validation. This document describes the internal architecture and design decisions.

## High-Level Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   User CLI      │    │   Redis Store   │    │   GPU Hardware  │
│   (canhazgpu)   │◄──►│   (localhost)   │    │   (nvidia-smi)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         ▲                       ▲                       ▲
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ Command Layer   │    │ State Layer     │    │ Validation Layer│
│ - run           │    │ - GPU tracking  │    │ - Usage detection│
│ - reserve       │    │ - Heartbeats    │    │ - Process owner │
│ - release       │    │ - Locking       │    │ - Memory usage  │
│ - status        │    │ - LRU tracking  │    │ - Conflict check│
│ - admin         │    │ - Expiry        │    │ - Real-time scan│
│ - report        │    │ - Usage history │    │ - GPU processes │
│ - web           │    │ - Time tracking │    │ - Memory usage  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Core Components

### 1. Command Layer (`canhazgpu:lines 700-800`)

The CLI interface built with Click framework:

```python
@click.group()
def main():
    """GPU reservation tool for shared development systems"""
    pass

@main.command()
@click.option('--gpus', default=1, help='Number of GPUs to reserve')
@click.option('--', 'command', help='Command to run')
def run(gpus, command):
    """Reserve GPUs and run command"""
    # Implementation
```

**Key responsibilities:**
- Argument parsing and validation
- User interaction and error reporting
- Orchestrating lower-level components

### 2. State Management Layer (`canhazgpu:lines 200-400`)

Redis-based distributed state management:

```python
def get_redis_client():
    """Get Redis client with connection pooling"""
    return redis.Redis(host='localhost', port=6379, db=0, decode_responses=True)

class GPUState:
    """GPU state representation"""
    def __init__(self, gpu_id):
        self.gpu_id = gpu_id
        self.redis_key = f"canhazgpu:gpu:{gpu_id}"
    
    def is_available(self):
        """Check if GPU is available for allocation"""
        # Implementation
```

**Key responsibilities:**
- GPU state persistence in Redis
- Distributed locking for race condition prevention
- Heartbeat management for run-type reservations
- Expiry handling for manual reservations

### 3. Validation Layer (`canhazgpu:lines 98-200`)

Real-time GPU usage validation via nvidia-smi:

```python
def detect_gpu_usage():
    """Detect actual GPU usage via nvidia-smi"""
    try:
        # Query GPU memory usage
        memory_result = subprocess.run([
            'nvidia-smi', '--query-gpu=memory.used', 
            '--format=csv,noheader,nounits'
        ], capture_output=True, text=True, check=True)
        
        # Query GPU processes
        process_result = subprocess.run([
            'nvidia-smi', '--query-compute-apps=pid,process_name,gpu_uuid,used_memory',
            '--format=csv,noheader'
        ], capture_output=True, text=True, check=True)
        
        # Process and return usage data
        return parse_gpu_usage(memory_result.stdout, process_result.stdout)
    except subprocess.CalledProcessError:
        return {}
```

**Key responsibilities:**
- Real-time GPU usage detection
- Process ownership identification
- Memory usage quantification
- Unreserved usage detection

### 4. Allocation Engine (`canhazgpu:lines 303-444`)

LRU-based GPU allocation with race condition protection:

```python
def atomic_reserve_gpus(requested_gpus, user, reservation_type, expiry_time=None):
    """Atomically reserve GPUs using Redis Lua script"""
    
    # Lua script for atomic allocation
    lua_script = '''
        local gpu_count = tonumber(ARGV[1])
        local requested = tonumber(ARGV[2])
        local user = ARGV[3]
        local reservation_type = ARGV[4]
        local current_time = tonumber(ARGV[5])
        local expiry_time = ARGV[6] ~= "None" and tonumber(ARGV[6]) or nil
        
        -- Get available GPUs with LRU ranking
        local available_gpus = {}
        for i = 0, gpu_count - 1 do
            -- Check GPU availability and LRU ranking
            -- Implementation details...
        end
        
        -- Allocate requested GPUs
        local allocated = {}
        for i = 1, math.min(requested, #available_gpus) do
            -- Atomic allocation logic
            -- Implementation details...
        end
        
        return allocated
    '''
    
    # Execute Lua script atomically
    return redis_client.eval(lua_script, 0, gpu_count, requested_gpus, user, 
                            reservation_type, current_time, expiry_time)
```

**Key responsibilities:**
- Atomic GPU allocation to prevent race conditions
- LRU (Least Recently Used) allocation strategy
- Integration with validation layer for unreserved usage exclusion
- Rollback on partial allocation failures

## Data Flow

### 1. GPU Reservation Flow (`run` command)

```
User Request
     ↓
Command Parsing
     ↓
Pre-allocation Validation ─────► nvidia-smi Query
     ↓                          ↓
Available GPU Detection ←───── Process Ownership
     ↓
Allocation Lock Acquisition
     ↓
Atomic GPU Reservation ────────► Redis Lua Script
     ↓                          ↓
Environment Setup ←──────────── GPU IDs Assigned
     ↓
Command Execution ─────────────► Background Heartbeat
     ↓                          ↓
Automatic Cleanup ←──────────── Process Termination
```

### 2. Status Reporting Flow

```
Status Request
     ↓
Redis State Query ─────────────► GPU Reservations
     ↓                          ↓
Validation Scan ←──────────────── Current State
     ↓
nvidia-smi Query ──────────────► Actual Usage
     ↓                          ↓
Process Analysis ←─────────────── User Identification
     ↓
Status Aggregation
     ↓
Formatted Output
```

## Key Design Decisions

### 1. Single-File Architecture

**Rationale:**
- Simplifies deployment and distribution
- Reduces dependencies and complexity
- Easy to audit and modify
- Self-contained tool

**Trade-offs:**
- Larger file size (~800 lines)
- Less modular than multi-file architecture
- Harder to unit test individual components

### 2. Redis for State Management

**Rationale:**
- Provides distributed coordination
- Atomic operations via Lua scripts
- Persistent storage across restarts
- High performance for concurrent access

**Trade-offs:**
- Additional dependency (Redis server)
- Network dependency (though localhost)
- Requires Redis administration knowledge

### 3. nvidia-smi Integration

**Rationale:**
- Universal availability on NVIDIA systems
- Comprehensive GPU information
- Real-time process detection
- Standard tool for GPU monitoring

**Trade-offs:**
- Subprocess overhead for each query
- Parsing text output (not structured API)
- Dependency on NVIDIA driver stack

### 4. Lua Scripts for Atomicity

**Rationale:**
- Prevents race conditions in allocation
- Ensures consistent state updates
- Eliminates time-of-check-time-of-use bugs
- Leverages Redis's atomic execution

**Trade-offs:**
- Complex logic embedded in Lua strings
- Harder to debug than Python code
- Limited error handling within Lua

## State Schema

### Redis Key Structure

```
canhazgpu:gpu_count              # Total GPU count (integer)
canhazgpu:allocation_lock        # Global allocation lock (string)
canhazgpu:gpu:{id}              # Individual GPU state (JSON)
```

### GPU State Object

**Available GPU:**
```json
{
  "last_released": 1672531200.123
}
```

**Reserved GPU:**
```json
{
  "user": "alice",
  "start_time": 1672531200.123,
  "last_heartbeat": 1672531260.456,
  "type": "run",
  "expiry_time": null
}
```

**Manual Reservation:**
```json
{
  "user": "bob", 
  "start_time": 1672531200.123,
  "type": "manual",
  "expiry_time": 1672559600.789
}
```

## Concurrency and Race Conditions

### 1. Allocation Race Conditions

**Problem:**
Multiple users requesting GPUs simultaneously could cause:
- Double allocation of same GPU
- Inconsistent state updates
- Partial allocations

**Solution:**
```python
def acquire_allocation_lock():
    """Acquire global allocation lock with exponential backoff"""
    for attempt in range(5):
        if redis_client.set("canhazgpu:allocation_lock", "locked", nx=True, ex=10):
            return True
        
        # Exponential backoff with jitter
        sleep_time = (2 ** attempt) + random.uniform(0, 1)
        time.sleep(sleep_time)
    
    return False
```

### 2. Heartbeat Race Conditions

**Problem:**
Heartbeat updates could conflict with allocation/release operations.

**Solution:**
- Heartbeats use separate Redis operations
- Allocation operations check heartbeat freshness
- Auto-cleanup handles stale heartbeats

### 3. Validation Race Conditions

**Problem:**
GPU usage could change between validation and allocation.

**Solution:**
- Validation integrated into atomic Lua scripts
- Unreserved usage lists passed to allocation logic
- Re-validation on allocation failure

## Performance Characteristics

### 1. Command Performance

**Typical latencies:**
- `status`: 100-500ms (depends on GPU count)
- `reserve`: 50-200ms (depends on contention)
- `run`: 100-300ms (plus command startup)
- `release`: 50-100ms

**Bottlenecks:**
- nvidia-smi subprocess calls
- Redis network round trips
- Lua script execution

### 2. Scalability Limits

**GPU count:** Tested up to 64 GPUs per system
**Concurrent users:** Handles 10+ simultaneous allocations
**Allocation frequency:** Supports high-frequency allocation patterns

**Scaling factors:**
- Linear with GPU count for status operations
- Constant time for individual GPU operations
- Contention increases with user count

### 3. Memory Usage

**Redis memory:** ~1KB per GPU + ~10KB overhead
**Python process:** ~20-50MB per invocation
**System impact:** Minimal - short-lived processes

## Extension Points

### 1. Alternative Allocation Strategies

Current LRU allocation could be replaced with:
- Round-robin allocation
- Thermal-aware allocation
- Performance-based allocation
- User priority-based allocation

**Implementation:** Modify `get_available_gpus_sorted_by_lru()` function

### 2. Additional Validation Sources

Beyond nvidia-smi, could integrate:
- ROCm tools for AMD GPUs
- Intel GPU tools
- Container runtime APIs
- Custom monitoring tools

**Implementation:** Extend `detect_gpu_usage()` function

### 3. Alternative State Backends

Redis could be replaced with:
- Database backends (PostgreSQL, SQLite)
- Distributed systems (etcd, Consul)
- File-based locking
- Cloud-based coordination

**Implementation:** Replace Redis client with abstract interface

### 4. Notification Systems

Could add notifications for:
- Allocation conflicts
- Unreserved usage
- Reservation expiry
- System health issues

**Implementation:** Add notification hooks to key operations

## Go Implementation Architecture

The Go implementation follows a modular architecture with clear separation of concerns:

### Package Structure

```
internal/
├── cli/                    # Command implementations (Cobra)
│   ├── root.go            # Root command and global config
│   ├── admin.go           # GPU pool initialization
│   ├── status.go          # Status display
│   ├── run.go             # Run with GPU reservation
│   ├── reserve.go         # Manual reservation
│   ├── release.go         # Release reservations
│   ├── report.go          # Usage reporting
│   └── web.go             # Web dashboard server
├── gpu/                    # GPU management logic
│   ├── allocation.go      # LRU allocation engine
│   ├── validation.go      # nvidia-smi integration
│   └── heartbeat.go       # Heartbeat manager
├── redis_client/          # Redis operations
│   └── client.go          # Redis client with Lua scripts
└── types/                 # Shared types
    └── types.go           # Config, state, and domain types
```

### New Features in Go Implementation

#### 1. Usage Tracking and Reporting

**Architecture:**
- Usage records created when GPUs are released
- Stored in Redis with key pattern `canhazgpu:usage_history:*`
- 90-day expiration to prevent unbounded growth
- Report aggregation includes both historical and current usage

**Implementation:**
```go
type UsageRecord struct {
    User            string
    GPUID           int
    StartTime       FlexibleTime
    EndTime         FlexibleTime
    Duration        float64
    ReservationType string
}
```

#### 2. Web Dashboard

**Architecture:**
- Embedded HTML/CSS/JS using Go's embed package
- RESTful API endpoints for status and reports
- Real-time updates with auto-refresh
- Responsive design for mobile access

**API Endpoints:**
- `GET /` - Dashboard UI
- `GET /api/status` - Current GPU status (JSON)
- `GET /api/report?days=N` - Usage report (JSON)

**Key Design Decisions:**
- Single binary deployment (UI embedded)
- No external dependencies for web UI
- Dark theme for developer-friendly interface
- Progressive enhancement approach

#### 3. Enhanced LRU Implementation

**Improvements:**
- Proper RFC3339 timestamp parsing in Lua scripts
- Better handling of never-used GPUs (last_released = 0)
- Atomic operations prevent allocation races

### Performance Optimizations

1. **Concurrent Operations**: Command implementations use goroutines where beneficial
2. **Connection Pooling**: Redis client maintains persistent connections
3. **Embedded Resources**: Web assets compiled into binary
4. **Efficient Serialization**: JSON marshaling optimized for common paths

### Security Considerations

1. **Web Server**: Configurable bind address for network isolation
2. **No Authentication**: Designed for trusted environments
3. **Read-Only Web**: Dashboard cannot modify state
4. **Process Validation**: Uses /proc filesystem for ownership detection

## Testing Strategy

### 1. Unit Testing Approach

**Mockable components:**
- Redis client interactions
- nvidia-smi subprocess calls
- System time functions
- Process ownership detection

**Test categories:**
- State management operations
- Allocation logic validation
- Error handling scenarios
- Race condition simulation

### 2. Integration Testing

**Test scenarios:**
- Multi-user allocation conflicts
- System restart recovery
- Hardware failure simulation
- Network partition handling

### 3. Performance Testing

**Load testing:**
- Concurrent allocation stress tests
- High-frequency operation patterns
- Large GPU count scenarios
- Memory leak detection

This architecture provides a robust, scalable foundation for GPU resource management while maintaining simplicity and ease of deployment.