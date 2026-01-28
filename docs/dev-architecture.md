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
│ - status        │    │ - MRU-per-user  │    │ - Conflict check│
│ - admin         │    │ - Expiry        │    │ - Real-time scan│
│ - report        │    │ - Usage history │    │ - GPU processes │
│ - web           │    │ - Time tracking │    │ - Memory usage  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Core Components

### 1. Command Layer (`internal/cli/`)

The CLI interface built with Cobra framework:

```go
// internal/cli/root.go
var rootCmd = &cobra.Command{
    Use:   "canhazgpu",
    Short: "GPU reservation tool for shared development systems",
}

// internal/cli/run.go
var runCmd = &cobra.Command{
    Use:   "run",
    Short: "Reserve GPUs and run command",
    RunE:  runRun,
}

func init() {
    runCmd.Flags().IntVar(&gpuCount, "gpus", 1, "Number of GPUs to reserve")
}
```

**Key responsibilities:**
- Argument parsing and validation
- User interaction and error reporting
- Orchestrating lower-level components

### 2. State Management Layer (`internal/redis_client/`)

Redis-based distributed state management:

```go
// internal/redis_client/client.go
type Client struct {
    rdb *redis.Client
}

func NewClient(config *types.Config) *Client {
    rdb := redis.NewClient(&redis.Options{
        Addr: fmt.Sprintf("%s:%d", config.RedisHost, config.RedisPort),
        DB:   config.RedisDB,
    })
    return &Client{rdb: rdb}
}

// internal/types/types.go
type GPUState struct {
    User          string       `json:"user,omitempty"`
    StartTime     FlexibleTime `json:"start_time,omitempty"`
    LastHeartbeat FlexibleTime `json:"last_heartbeat,omitempty"`
    Type          string       `json:"type,omitempty"`
    ExpiryTime    FlexibleTime `json:"expiry_time,omitempty"`
    LastReleased  FlexibleTime `json:"last_released,omitempty"`
}
```

**Key responsibilities:**
- GPU state persistence in Redis
- Distributed locking for race condition prevention
- Heartbeat management for run-type reservations
- Expiry handling for manual reservations

### 3. Validation Layer (`internal/gpu/`)

Real-time GPU usage validation via GPU provider abstraction:

```go
// internal/gpu/provider.go
type GPUProvider interface {
    Name() string
    IsAvailable() bool
    DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error)
    GetGPUCount(ctx context.Context) (int, error)
}

// internal/gpu/nvidia_provider.go
func (p *NvidiaProvider) DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error) {
    // Query GPU processes via nvidia-smi
    cmd := exec.CommandContext(ctx, "nvidia-smi",
        "--query-compute-apps=pid,process_name,gpu_uuid,used_memory",
        "--format=csv,noheader")
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("nvidia-smi failed: %w", err)
    }
    return parseNvidiaProcessOutput(string(output))
}
```

**Key responsibilities:**
- Real-time GPU usage detection
- Process ownership identification
- Memory usage quantification
- Unreserved usage detection

### 4. Allocation Engine (`internal/redis_client/client.go`)

MRU-per-user GPU allocation with LRU fallback and race condition protection:

```go
// AtomicReserveGPUs reserves GPUs atomically using a Redis Lua script
func (c *Client) AtomicReserveGPUs(ctx context.Context, request *types.AllocationRequest) ([]int, error) {
    // Lua script for atomic allocation with MRU-per-user strategy
    luaScript := `
        local gpu_count = tonumber(ARGV[1])
        local requested = tonumber(ARGV[2])
        local user = ARGV[3]
        local reservation_type = ARGV[4]
        local current_time = tonumber(ARGV[5])

        -- Get available GPUs with MRU-per-user ranking
        local available_gpus = {}
        for i = 0, gpu_count - 1 do
            -- Check GPU availability and user's usage history
            -- Implementation details...
        end

        -- Allocate requested GPUs atomically
        local allocated = {}
        for i = 1, math.min(requested, #available_gpus) do
            -- Set GPU state with user and timestamp
        end

        return allocated
    `

    result, err := c.rdb.Eval(ctx, luaScript, nil, gpuCount, request.GPUCount,
        request.User, request.Type, time.Now().Unix()).Result()
    // Process result...
}
```

**Key responsibilities:**
- Atomic GPU allocation to prevent race conditions
- MRU-per-user (Most Recently Used per user) allocation strategy with LRU fallback
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

### 1. Modular Go Architecture

**Rationale:**
- Clear separation of concerns (CLI, GPU management, Redis, types)
- Single binary deployment via `go build`
- Strong typing and compile-time checks
- Easy to test individual components
- Self-contained tool with embedded web assets

**Trade-offs:**
- Requires Go toolchain for development
- More files to navigate than single-file approach
- Slightly more complex build process

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
- Harder to debug than Go code
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
```go
// AcquireAllocationLock acquires the global allocation lock with exponential backoff
func (c *Client) AcquireAllocationLock(ctx context.Context) error {
    for attempt := 0; attempt < 5; attempt++ {
        ok, err := c.rdb.SetNX(ctx, "canhazgpu:allocation_lock", "locked", 10*time.Second).Result()
        if err != nil {
            return fmt.Errorf("failed to acquire lock: %w", err)
        }
        if ok {
            return nil
        }

        // Exponential backoff with jitter
        sleepTime := time.Duration(1<<attempt)*time.Second + time.Duration(rand.Float64()*1000)*time.Millisecond
        time.Sleep(sleepTime)
    }
    return fmt.Errorf("failed to acquire allocation lock after 5 attempts")
}
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
**Go binary:** ~15-30MB per invocation
**System impact:** Minimal - short-lived processes

## Extension Points

### 1. Alternative Allocation Strategies

Current MRU-per-user allocation could be enhanced with:
- Thermal-aware allocation (prefer cooler GPUs)
- Performance-based allocation (prefer faster GPUs)
- Performance-based allocation
- User priority-based allocation

**Implementation:** Modify the Lua script in `AtomicReserveGPUs()` in `internal/redis_client/client.go`

### 2. GPU Provider System

The system supports multiple GPU providers through a unified interface:

**Available Providers:**
- **NVIDIA**: Uses nvidia-smi for NVIDIA GPU management
- **AMD**: Uses amd-smi (ROCm 5.7+) for AMD GPU management
- **Fake**: Simulated provider for development and testing without real GPUs

**Provider Architecture:**
```go
type GPUProvider interface {
    Name() string
    IsAvailable() bool
    DetectGPUUsage(ctx context.Context) (map[int]*types.GPUUsage, error)
    GetGPUCount(ctx context.Context) (int, error)
}
```

**Fake Provider Use Cases:**
- Development on laptops/desktops without GPUs
- CI/CD pipeline testing
- Testing reservation logic without hardware
- Demonstration and documentation

**Implementation:** See `internal/gpu/provider.go` and individual provider files

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
│   ├── allocation.go      # MRU-per-user allocation engine
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

#### 3. Enhanced MRU-per-User Implementation

**Improvements:**
- Efficient usage history queries (last 100 records)
- Per-user GPU preference tracking
- Proper RFC3339 timestamp parsing in Lua scripts
- Better handling of never-used GPUs (fallback to global LRU)
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