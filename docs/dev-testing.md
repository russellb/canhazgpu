# Testing

This guide explains how to run tests for canhazgpu and understand the testing infrastructure.

## Test Organization

### Test Packages

- **`internal/types/`** - Core data types and validation
- **`internal/utils/`** - Utility functions (parsing, formatting)
- **`internal/cli/`** - Command-line interface structure
- **`internal/redis_client/`** - Redis operations and concurrency
- **`internal/gpu/`** - GPU allocation, heartbeat, and validation

### Test Types

#### Unit Tests (Fast)
- Run with: `make test-short`
- Duration: < 1 second
- Dependencies: None
- Tests core logic, validation, data structures

#### Integration Tests (Slower)
- Run with: `make test` or `make test-integration`
- Duration: 5-30 seconds per test
- Dependencies: Redis server, nvidia-smi (optional)
- Tests real system interactions

## Running Tests

### Quick Testing (Recommended for Development)
```bash
make test-short
```
- Skips integration tests
- Fast feedback loop
- No external dependencies required

### Full Testing (CI/Release)
```bash
make test
```
- Includes integration tests
- Tests Redis connectivity
- Tests nvidia-smi integration
- May take 30+ seconds

### Coverage Reports
```bash
make test-coverage
```
- Generates `coverage.html` report
- Shows line-by-line coverage

### Integration Only
```bash
make test-integration
```
- Runs only integration tests
- Useful for testing external dependencies

## Understanding Test Timing

### Expected Slow Tests

When running full tests (`make test`), these tests may take time:

1. **Redis Concurrency Tests** (3-5 seconds)
   - `TestClient_AllocationLock_Concurrency`
   - Tests distributed locking with timeouts
   - Logs: Shows lock acquisition timing

2. **GPU Validation Tests** (5-10 seconds)
   - `TestDetectGPUUsage_Integration` 
   - Calls nvidia-smi command
   - Logs: Indicates nvidia-smi availability

3. **Heartbeat Manager Tests** (1-3 seconds)
   - `TestHeartbeatManager_Wait`
   - `TestHeartbeatManager_StartStop`
   - Tests goroutine lifecycle
   - Logs: Shows goroutine timing

4. **GPU Allocation Tests** (2-10 seconds)
   - `TestAllocationEngine_AllocateGPUs_Structure`
   - Combines Redis + nvidia-smi validation
   - Logs: Indicates each phase

### Test Logging

Integration tests include verbose logging to explain timing:

```
=== RUN   TestClient_AllocationLock_Concurrency
    client_test.go:149: Starting concurrency test - testing lock contention (may take up to 5 seconds)
    client_test.go:154: First client acquired lock successfully
    client_test.go:169: Second client attempting to acquire lock (should timeout/fail)
    client_test.go:174: Second lock attempt took 3.5s and failed as expected
```

## Test Dependencies

### Required for Integration Tests

1. **Redis Server** (localhost:6379)
   - Used for state management tests
   - Tests automatically skip if unavailable
   - Uses database 15 (test database)

2. **nvidia-smi** (optional)
   - Used for GPU detection tests
   - Tests gracefully handle missing command
   - Expected to fail on non-GPU systems

### Test Database Isolation

- Tests use Redis database 15
- Automatically cleaned before/after tests
- Won't affect production data (database 0)

## Debugging Failing Tests

### Redis Connection Issues
```
SKIP: Redis not available for testing: dial tcp :6379: connect: connection refused
```
**Solution**: Start Redis server or run `make test-short`

### nvidia-smi Not Found
```
nvidia-smi not available or failed: exec: "nvidia-smi": executable file not found
```
**Solution**: This is expected on non-GPU systems - test will pass

### GPU Memory Threshold Understanding
The system uses a >100MB threshold (not ≥100MB) for unreserved GPU usage:
- GPUs using exactly 100MB: Considered authorized
- GPUs using >100MB: Considered unreserved
- Integration tests log actual GPU usage for verification

### Command Failure Cleanup
The `run` command properly cleans up GPUs even when the executed command fails:
- Successful commands: GPUs cleaned up via defer mechanism
- Failed commands: GPUs explicitly cleaned up before process exit
- Signal termination: GPUs cleaned up via signal handler
- Process crash: GPUs auto-released after heartbeat timeout (15 min)

### Test Timeouts
```
Command timed out after 2m
```
**Solution**: Check Redis connectivity or run `make test-short`

## Writing New Tests

### Test Structure
```go
func TestExample_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    t.Log("Starting integration test - may take X seconds")
    // ... test logic
}
```

### Integration Test Guidelines

1. **Use `testing.Short()` checks** for slow tests
2. **Add informative logging** with `t.Log()`
3. **Handle missing dependencies gracefully**
4. **Use timeouts** for network operations
5. **Clean up resources** in test cleanup

### Unit Test Guidelines

1. **Test public APIs** and exported functions
2. **Use table-driven tests** for multiple cases
3. **Test error conditions** and edge cases
4. **Keep tests fast** (< 100ms each)
5. **Avoid external dependencies**

## CI/CD Integration

### GitHub Actions Example
```yaml
# Fast checks
- name: Run unit tests
  run: make test-short

# Full validation (with Redis)
- name: Run integration tests  
  run: make test
  env:
    REDIS_URL: redis://localhost:6379
```

### Local Development Workflow
```bash
# During development
make test-short

# Before committing
make test

# Before releasing
make test-coverage
```

This testing infrastructure ensures reliable GPU allocation while providing fast feedback during development.