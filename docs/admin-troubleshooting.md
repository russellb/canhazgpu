# Troubleshooting

This guide covers common issues and their solutions when using canhazgpu in production environments.

## Redis Connection Issues

### Redis Server Not Running
**Symptoms:**
```bash
❯ canhazgpu status
redis.exceptions.ConnectionError: Error 111 connecting to 127.0.0.1:6379. Connection refused.
```

**Solutions:**
```bash
# Check Redis status
sudo systemctl status redis-server

# Start Redis if not running
sudo systemctl start redis-server
sudo systemctl enable redis-server

# Verify Redis is accessible
redis-cli ping
# Should return: PONG
```

**Alternative Redis installations:**
```bash
# If using different Redis installation
ps aux | grep redis
netstat -tlnp | grep 6379

# Check Redis configuration
sudo cat /etc/redis/redis.conf | grep -E "^(bind|port)"
```

### Redis Permission Issues
**Symptoms:**
```bash
❯ canhazgpu status
redis.exceptions.ResponseError: NOAUTH Authentication required.
```

**Solutions:**
```bash
# Check if Redis requires authentication
redis-cli
127.0.0.1:6379> ping
(error) NOAUTH Authentication required.

# Option 1: Disable AUTH for localhost (recommended for canhazgpu)
sudo vim /etc/redis/redis.conf
# Comment out: # requirepass your_password_here
sudo systemctl restart redis-server

# Option 2: Configure canhazgpu with AUTH (requires code modification)
# Currently not supported - disable AUTH instead
```

### Redis Memory Issues
**Symptoms:**
```bash
❯ canhazgpu reserve
redis.exceptions.ResponseError: OOM command not allowed when used memory > 'maxmemory'.
```

**Solutions:**
```bash
# Check Redis memory usage
redis-cli info memory

# Increase maxmemory in /etc/redis/redis.conf
maxmemory 512mb  # Increase as needed

# Or disable maxmemory limit
# maxmemory 0

sudo systemctl restart redis-server
```

## GPU Provider Issues

### Wrong Provider Cached
**Issue:** System using wrong GPU provider after switching hardware

**Solution:**
```bash
# Check current cached provider
redis-cli get "canhazgpu:provider"

# Re-initialize with correct provider
canhazgpu admin --gpus 8 --provider nvidia --force
# OR
canhazgpu admin --gpus 8 --provider amd --force

# Let system auto-detect
canhazgpu admin --gpus 8 --force
```

### Multiple GPU Vendors
**Issue:** System has both NVIDIA and AMD GPUs

**Current Limitation:** canhazgpu currently supports single provider per system

**Workaround:** Use the provider for the GPUs you want to manage:
```bash
# Use NVIDIA provider for NVIDIA GPUs
canhazgpu admin --gpus 4 --provider nvidia

# Use AMD provider for AMD GPUs  
canhazgpu admin --gpus 2 --provider amd
```

## NVIDIA GPU Issues

### nvidia-smi Not Available
**Symptoms:**
```bash
❯ canhazgpu status
nvidia-smi: command not found
```

**Solutions:**
```bash
# Check if NVIDIA drivers are installed
lspci | grep -i nvidia

# Install NVIDIA drivers (Ubuntu/Debian)
sudo apt update
sudo apt install nvidia-driver-470  # or latest version

# Install NVIDIA drivers (CentOS/RHEL/Fedora)
sudo dnf install nvidia-driver

# Verify installation
nvidia-smi
```

### NVIDIA Driver Version Issues
**Symptoms:**
```bash
❯ nvidia-smi
NVIDIA-SMI has failed because it couldn't communicate with the NVIDIA driver.
```

**Solutions:**
```bash
# Check driver status
sudo dmesg | grep nvidia
lsmod | grep nvidia

# Restart NVIDIA services
sudo systemctl restart nvidia-persistenced
sudo modprobe -r nvidia_uvm nvidia_drm nvidia_modeset nvidia
sudo modprobe nvidia

# If still failing, reinstall drivers
sudo apt purge nvidia-*
sudo apt autoremove
sudo apt install nvidia-driver-470
sudo reboot
```

### NVIDIA GPU Detection Issues
**Symptoms:**
```bash
❯ nvidia-smi
No devices were found
```

**Solutions:**
```bash
# Check hardware detection
lspci | grep -i nvidia

# Check if GPUs are disabled in BIOS
# Reboot and check BIOS settings

# Check if GPUs are in compute mode
nvidia-smi -q -d COMPUTE

# Reset GPU state if needed
sudo nvidia-smi -r
```

## AMD GPU Issues

### amd-smi Not Available
**Error:**
```bash
amd-smi: command not found
```

**Solution:**
```bash
# Check if AMD GPUs are present
lspci | grep -i amd

# Test amd-smi availability
amd-smi list

# Install ROCm and amd-smi (Ubuntu/Debian)
sudo apt update
sudo apt install rocm-dev amd-smi-lib

# Install ROCm (CentOS/RHEL/Fedora)
sudo dnf install rocm-dev amd-smi-lib
```

**Verify installation:**
```bash
amd-smi list
```

### AMD Driver Communication Error
**Error:**
```bash
❯ amd-smi list
Failed to initialize ROCm
```

**Solution:**
Check ROCm installation and permissions:
```bash
# Check ROCm installation
ls /opt/rocm/

# Check user permissions
groups $USER
# Should include 'render' and 'video' groups

# Add user to groups if needed
sudo usermod -a -G render,video $USER
# Log out and log back in

# Restart ROCm services
sudo systemctl restart rocm-smi
```

## Allocation Problems

### Not Enough GPUs Available
**Symptoms:**
```bash
❯ canhazgpu run --gpus 2 -- python train.py
Error: Not enough GPUs available. Requested: 2, Available: 1 (1 GPUs in use without reservation - run 'canhazgpu status' for details)
```

**Diagnosis:**
```bash
# Check detailed status
canhazgpu status

# Look for unreserved usage
canhazgpu status | grep "UNRESERVED"

# Check actual GPU processes
nvidia-smi
amd-smi list
```

**Solutions:**
1. **Contact unreserved users:**
   - Identify users from status output
   - Ask them to use proper reservations

2. **Wait for reservations to expire:**
   - Check manual reservation expiry times
   - Wait for run-type reservations to complete

3. **Reduce GPU request:**
   ```bash
   canhazgpu run --gpus 1 -- python train.py
   ```

### Allocation Lock Timeouts
**Symptoms:**
```bash
❯ canhazgpu reserve --gpus 2
Error: Failed to acquire allocation lock after 5 attempts
```

**Causes:**
- High contention (multiple users allocating simultaneously)
- Stale locks from crashed processes
- Redis performance issues

**Solutions:**
```bash
# Wait and retry
sleep 10
canhazgpu reserve --gpus 2

# Check for stale locks in Redis
redis-cli
127.0.0.1:6379> GET canhazgpu:allocation_lock
127.0.0.1:6379> DEL canhazgpu:allocation_lock  # If stale

# Check Redis performance
redis-cli --latency -i 1
```

### GPU State Corruption
**Symptoms:**
```bash
❯ canhazgpu status
Error: GPU state corrupted for GPU 2
```

**Solutions:**
```bash
# Check Redis data
redis-cli
127.0.0.1:6379> GET canhazgpu:gpu:2
127.0.0.1:6379> DEL canhazgpu:gpu:2  # Clear corrupted state

# Reinitialize GPU pool if needed
canhazgpu admin --gpus 8 --force

# This will clear all reservations - warn users first
```

## Process and Heartbeat Issues

### Stale Heartbeats
**Symptoms:**
```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
1   in use    alice    3h 0m 0s    run                      heartbeat 15m 30s ago     
```

**Analysis:**
- Heartbeat should update every ~60 seconds
- Heartbeats >5 minutes old indicate problems
- GPU will auto-release after 15 minutes without heartbeat

**Solutions:**
```bash
# Check if process is still running
ps aux | grep alice | grep python

# If process died, wait for auto-cleanup (15 min timeout)
# If process is stuck, user should kill it

# Manual cleanup (admin only, if urgent)
redis-cli
127.0.0.1:6379> DEL canhazgpu:gpu:1
```

### Orphaned Processes
**Symptoms:**
```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
0   available          free for 5m                                                    2048MB, 1 processes
```

- GPU shows as available but has active processes
- Usually indicates process started outside canhazgpu

**Solutions:**
```bash
# Identify the process
nvidia-smi
amd-smi list

# Contact process owner
ps -o user,pid,command -p <PID>

# Ask them to:
# 1. Stop the process, or
# 2. Create proper reservation
```

## Permission and Access Issues

### User Cannot Run canhazgpu
**Symptoms:**
```bash
❯ canhazgpu status
bash: canhazgpu: command not found
```

**Solutions:**
```bash
# Check if installed
which canhazgpu
ls -la /usr/local/bin/canhazgpu

# Check PATH
echo $PATH

# Install if missing
sudo cp canhazgpu /usr/local/bin/
sudo chmod +x /usr/local/bin/canhazgpu
```

### Process Owner Detection Fails
**Symptoms:**
```bash
❯ canhazgpu status
GPU STATUS    USER     DURATION    TYPE    MODEL            DETAILS                    VALIDATION
--- --------- -------- ----------- ------- ---------------- -------------------------- ---------------------
2   in use    unknown                                       WITHOUT RESERVATION        1024MB used by PID 12345 (unknown process)
```

**Solutions:**
```bash
# Check /proc filesystem access
ls -la /proc/12345/

# Check ps command availability
ps -o user,command -p 12345

# If still failing, process may have terminated
# Wait for next status check
```

### Redis Access Denied
**Symptoms:**
```bash
❯ canhazgpu status
redis.exceptions.ResponseError: DENIED Redis is running in protected mode
```

**Solutions:**
```bash
# Check Redis configuration
sudo cat /etc/redis/redis.conf | grep protected-mode

# Option 1: Disable protected mode (if Redis is localhost-only)
sudo vim /etc/redis/redis.conf
# Set: protected-mode no

# Option 2: Set bind address (recommended)
# Set: bind 127.0.0.1

sudo systemctl restart redis-server
```

## Performance Issues

### Slow Status Commands
**Symptoms:**
- `canhazgpu status` takes >5 seconds
- High latency in GPU allocation

**Diagnosis:**
```bash
# Time the status command
time canhazgpu status

# Check nvidia-smi or amd-smi performance
time nvidia-smi
time amd-smi list

# Check Redis performance
redis-cli --latency -i 1
```

**Solutions:**
```bash
# Optimize Redis
sudo vim /etc/redis/redis.conf
# Add: tcp-keepalive 60
# Add: timeout 0

# Check system resources
htop
iostat -x 1

# Check for disk I/O issues
sudo iotop
```

### High Memory Usage
**Symptoms:**
- System running out of memory
- Redis using excessive memory

**Solutions:**
```bash
# Check Redis memory usage
redis-cli info memory

# Set memory limit in /etc/redis/redis.conf
maxmemory 256mb
maxmemory-policy allkeys-lru

# Monitor system memory
free -h
ps aux --sort=-%mem | head -20
```

## Data Recovery

### Lost GPU Reservations
**Symptoms:**
- All GPUs show as available after system restart
- Users lose their reservations

**Recovery:**
```bash
# Check if Redis data persisted
redis-cli
127.0.0.1:6379> KEYS canhazgpu:*

# If no data, check for Redis backup
ls -la /var/lib/redis/

# If backup exists, restore it
redis-cli FLUSHALL
cat dump.rdb | redis-cli --pipe

# If no backup, reinitialize
canhazgpu admin --gpus 8 --force
```

### Corrupted Redis Database
**Symptoms:**
```bash
❯ canhazgpu status
redis.exceptions.ResponseError: WRONGTYPE Operation against a key holding the wrong kind of value
```

**Recovery:**
```bash
# Backup current state (if possible)
redis-cli --rdb /tmp/corrupted-backup.rdb

# Clear corrupted data
redis-cli FLUSHALL

# Reinitialize canhazgpu
canhazgpu admin --gpus 8

# Notify users about data loss
```

## Preventive Measures

### Regular Health Checks
```bash
#!/bin/bash
# /usr/local/bin/canhazgpu-healthcheck.sh

# Check all components
redis-cli ping > /dev/null || echo "ERROR: Redis down"
nvidia-smi > /dev/null || echo "ERROR: NVIDIA drivers down"
canhazgpu status > /dev/null || echo "ERROR: canhazgpu failing"

# Check for common issues
STALE_HEARTBEATS=$(canhazgpu status | grep "last heartbeat" | grep -E "[5-9]m|[1-9][0-9]m" | wc -l)
if [ $STALE_HEARTBEATS -gt 0 ]; then
    echo "WARNING: $STALE_HEARTBEATS stale heartbeats detected"
fi

UNAUTHORIZED=$(canhazgpu status | grep "WITHOUT RESERVATION" | wc -l)
if [ $UNAUTHORIZED -gt 0 ]; then
    echo "WARNING: $UNAUTHORIZED unreserved GPU usage detected"
fi
```

### Monitoring Scripts
```bash
# /usr/local/bin/canhazgpu-monitor.sh
#!/bin/bash

while true; do
    echo "$(date): $(canhazgpu status | wc -l) GPUs, $(canhazgpu status | grep AVAILABLE | wc -l) available"
    sleep 300  # Every 5 minutes
done >> /var/log/canhazgpu-monitor.log
```

### Backup Procedures
```bash
#!/bin/bash
# Daily Redis backup
redis-cli --rdb /backup/canhazgpu-$(date +%Y%m%d).rdb

# Keep last 7 days
find /backup/ -name "canhazgpu-*.rdb" -mtime +7 -delete
```

## Getting Help

### Debug Information Collection
When reporting issues, collect:

```bash
# System information
uname -a
lsb_release -a

# NVIDIA GPU information
nvidia-smi
lspci | grep -i nvidia

# AMD GPU information
amd-smi list
lspci | grep -i amd

# Redis information
redis-cli info
redis-cli config get '*'

# canhazgpu state
canhazgpu status
redis-cli keys 'canhazgpu:*'

# Process information
ps aux | grep -E "(redis|nvidia|python)"
```

### Log Files to Check
- `/var/log/redis/redis-server.log`
- `/var/log/syslog` (or `/var/log/messages`)
- `dmesg` output for hardware issues
- Any custom monitoring logs

This troubleshooting guide covers the most common issues encountered in production deployments of canhazgpu. Most problems can be resolved by following these systematic approaches.