# Administration Setup

This guide covers setting up and configuring canhazgpu for production use in shared development environments.

## Initial System Setup

### 1. Install Dependencies

**System packages:**
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install redis-server

# CentOS/RHEL/Fedora  
sudo dnf install redis

# macOS
brew install redis
```

**Go installation:**
```bash
# Download and install Go 1.23+ from https://golang.org/dl/
# Or use package manager:

# Ubuntu (via snap)
sudo snap install go --classic

# macOS
brew install go
```

**NVIDIA drivers:**
```bash
# Verify nvidia-smi is available
nvidia-smi

# If not installed, install appropriate NVIDIA drivers
sudo apt install nvidia-driver-470  # Ubuntu example
```

### 2. Redis Configuration

**Basic Redis setup:**
```bash
# Start Redis service
sudo systemctl start redis-server
sudo systemctl enable redis-server

# Verify Redis is running
redis-cli ping
# Should return: PONG
```

**Production Redis configuration** (`/etc/redis/redis.conf`):
```ini
# Basic security
bind 127.0.0.1
protected-mode yes
port 6379

# Memory management
maxmemory 256mb
maxmemory-policy allkeys-lru

# Persistence (optional - GPU state can be rebuilt)
save 900 1
save 300 10
save 60 10000

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log
```

**Restart Redis after configuration changes:**
```bash
sudo systemctl restart redis-server
```

### 3. Install canhazgpu

**System-wide installation:**
```bash
# Download and install canhazgpu
wget https://raw.githubusercontent.com/russellb/canhazgpu/main/canhazgpu
chmod +x canhazgpu
sudo cp canhazgpu /usr/local/bin/

# Download and install bash completion (required for 'canhazgpu run' completion)
wget https://raw.githubusercontent.com/russellb/canhazgpu/main/autocomplete_canhazgpu.sh
sudo cp autocomplete_canhazgpu.sh /etc/bash_completion.d/

# Ensure proper permissions for bash completion
sudo chmod 644 /etc/bash_completion.d/autocomplete_canhazgpu.sh
```

**Verify installation:**
```bash
canhazgpu --help

# Test bash completion (optional)
# Start a new bash session and test:
# canhazgpu <TAB><TAB>
# Should show available commands
```

## GPU Pool Initialization

### 1. Determine GPU Count
```bash
# Check available GPUs
nvidia-smi -L
# or
nvidia-smi --query-gpu=index,name --format=csv
```

### 2. Initialize the Pool
```bash
# Initialize with the correct number of GPUs
canhazgpu admin --gpus 8

# Verify initialization
canhazgpu status
```

### 3. Test Basic Functionality
```bash
# Test reservation
canhazgpu reserve --duration 5m

# Check status
canhazgpu status

# Test run command
canhazgpu run --gpus 1 -- nvidia-smi

# Release manual reservations
canhazgpu release
```

## User Management

### 1. User Access
All users who need GPU access should:
- Have the `canhazgpu` command available in their PATH
- Be able to run `nvidia-smi`
- Have access to the Redis server (localhost:6379)
- Have bash completion available (if installed system-wide)

### 2. Group-Based Permissions (Optional)
```bash
# Create a GPU users group
sudo groupadd gpuusers

# Add users to the group
sudo usermod -a -G gpuusers alice
sudo usermod -a -G gpuusers bob

# Create a wrapper script for group enforcement (optional)
sudo tee /usr/local/bin/canhazgpu-wrapper << 'EOF'
#!/bin/bash
if ! groups | grep -q gpuusers; then
    echo "Error: You must be in the 'gpuusers' group to use GPU resources"
    exit 1
fi
exec /usr/local/bin/canhazgpu "$@"
EOF

sudo chmod +x /usr/local/bin/canhazgpu-wrapper
```

## Monitoring and Maintenance

### 1. System Health Checks

**Daily health check script:**
```bash
#!/bin/bash
# /usr/local/bin/canhazgpu-healthcheck.sh

set -e

echo "=== canhazgpu Health Check $(date) ==="

# Check Redis connectivity
echo "Checking Redis..."
redis-cli ping > /dev/null || {
    echo "ERROR: Redis is not responding"
    exit 1
}

# Check nvidia-smi
echo "Checking NVIDIA drivers..."
nvidia-smi > /dev/null || {
    echo "ERROR: nvidia-smi is not working"
    exit 1
}

# Check canhazgpu basic functionality
echo "Checking canhazgpu status..."
canhazgpu status > /dev/null || {
    echo "ERROR: canhazgpu status failed"
    exit 1
}

# Check for unauthorized usage
UNAUTHORIZED=$(canhazgpu status | grep "WITHOUT RESERVATION" || true)
if [ -n "$UNAUTHORIZED" ]; then
    echo "WARNING: Unauthorized GPU usage detected:"
    echo "$UNAUTHORIZED"
fi

# Check for stale reservations
STALE=$(canhazgpu status | grep "no actual usage detected" || true)
if [ -n "$STALE" ]; then
    echo "INFO: Potential stale reservations:"
    echo "$STALE"
fi

echo "Health check completed successfully"
```

**Schedule health checks:**
```bash
# Add to crontab
sudo crontab -e

# Run health check daily at 9 AM
0 9 * * * /usr/local/bin/canhazgpu-healthcheck.sh >> /var/log/canhazgpu-health.log 2>&1
```

### 2. Usage Monitoring

**Usage statistics script:**
```bash
#!/bin/bash
# /usr/local/bin/canhazgpu-stats.sh

echo "=== GPU Usage Statistics $(date) ==="

STATUS=$(canhazgpu status)

# Count different states
TOTAL=$(echo "$STATUS" | wc -l)
AVAILABLE=$(echo "$STATUS" | grep "AVAILABLE" | wc -l)
IN_USE=$(echo "$STATUS" | grep "IN USE by" | wc -l)
UNAUTHORIZED=$(echo "$STATUS" | grep "WITHOUT RESERVATION" | wc -l)

echo "Total GPUs: $TOTAL"
echo "Available: $AVAILABLE"
echo "In Use (Reserved): $IN_USE"
echo "Unauthorized Usage: $UNAUTHORIZED"
echo "Utilization: $(( 100 * (IN_USE + UNAUTHORIZED) / TOTAL ))%"

if [ $UNAUTHORIZED -gt 0 ]; then
    echo ""
    echo "Unauthorized Usage Details:"
    echo "$STATUS" | grep "WITHOUT RESERVATION"
fi
```

### 3. Automated Cleanup

**Stale reservation cleanup:**
```bash
#!/bin/bash
# /usr/local/bin/canhazgpu-cleanup.sh

# Find manual reservations with no actual usage for >1 hour
STATUS=$(canhazgpu status)
STALE=$(echo "$STATUS" | grep "manual.*no actual usage detected" | grep -E "[2-9][0-9]h|[0-9]{3,}h")

if [ -n "$STALE" ]; then
    echo "Found stale reservations (>1h with no usage):"
    echo "$STALE"
    
    # Log for manual review - don't auto-release without admin approval
    echo "$(date): $STALE" >> /var/log/canhazgpu-stale.log
    
    # Optional: Send notification to admin
    echo "$STALE" | mail -s "Stale GPU Reservations Detected" admin@company.com
fi
```

## Security Configuration

### 1. Redis Security
```bash
# /etc/redis/redis.conf
bind 127.0.0.1                    # Only localhost access
protected-mode yes                # Enable protected mode
# requirepass your_password_here  # Optional password protection

# Disable dangerous commands
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command CONFIG ""
```

### 2. File Permissions
```bash
# Ensure proper permissions
sudo chown root:root /usr/local/bin/canhazgpu
sudo chmod 755 /usr/local/bin/canhazgpu

# Protect Redis data directory
sudo chown redis:redis /var/lib/redis
sudo chmod 700 /var/lib/redis
```

### 3. Network Security
Since canhazgpu uses local Redis, ensure:
- Redis is bound only to localhost (127.0.0.1)
- Firewall blocks external access to port 6379
- No Redis AUTH if not needed (localhost only)

## Configuration Changes

### 1. Changing GPU Count
```bash
# When adding/removing GPUs from the system
canhazgpu admin --gpus 12 --force

# This will clear all existing reservations
# Notify users before making this change
```

### 2. Redis Migration
```bash
# Backup current state
redis-cli --rdb /backup/canhazgpu-backup.rdb

# After Redis migration/reinstall
redis-cli --rdb /backup/canhazgpu-backup.rdb | redis-cli --pipe

# Verify data integrity
canhazgpu status
```

### 3. System Upgrades
Before system upgrades:
```bash
# Warn users
wall "System maintenance in 15 minutes - GPU reservations will be cleared"

# Export current state for reference
canhazgpu status > /tmp/gpu-state-before-upgrade.txt

# After upgrade, reinitialize if needed
canhazgpu admin --gpus 8 --force
```

## Troubleshooting

### Common Issues

**Redis connection errors:**
```bash
# Check Redis status
sudo systemctl status redis-server

# Check Redis logs
sudo tail -f /var/log/redis/redis-server.log

# Test Redis connectivity
redis-cli ping
```

**Permission errors:**
```bash
# Check file permissions
ls -la /usr/local/bin/canhazgpu

# Check Redis permissions
sudo -u redis redis-cli ping
```

**NVIDIA driver issues:**
```bash
# Check driver status
nvidia-smi

# Check driver version
cat /proc/driver/nvidia/version

# Restart nvidia services if needed
sudo systemctl restart nvidia-persistenced
```

### Log Analysis
```bash
# System logs
sudo journalctl -u redis-server
sudo journalctl | grep canhazgpu

# Application logs (if configured)
tail -f /var/log/canhazgpu-health.log
tail -f /var/log/canhazgpu-stale.log
```

## Production Deployment Checklist

- [ ] Redis server installed and configured
- [ ] NVIDIA drivers working (`nvidia-smi` functional)
- [ ] Go 1.23+ installed
- [ ] canhazgpu installed system-wide
- [ ] Bash completion script installed
- [ ] GPU pool initialized with correct count
- [ ] Basic functionality tested
- [ ] Bash completion verified for users
- [ ] Health check script configured
- [ ] Monitoring script set up
- [ ] User access verified
- [ ] Security configuration applied
- [ ] Backup/recovery procedures documented
- [ ] User training materials prepared

## Performance Optimization

### 1. Redis Performance
```bash
# Redis performance tuning in /etc/redis/redis.conf
tcp-keepalive 60
timeout 0
tcp-backlog 511

# Disable unused features for better performance
save ""  # Disable RDB snapshots if persistence not needed
```

### 2. System Performance
```bash
# Optimize for rapid GPU queries
echo 'vm.swappiness=1' >> /etc/sysctl.conf
echo 'vm.dirty_ratio=5' >> /etc/sysctl.conf
sysctl -p
```

### 3. Monitoring Performance
```bash
# Monitor Redis performance
redis-cli --latency -i 1

# Monitor system resources
htop
iostat -x 1
```

With proper administration setup, canhazgpu provides reliable, secure, and efficient GPU resource management for your entire development team.